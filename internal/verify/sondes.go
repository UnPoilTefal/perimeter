package verify

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/UnPoilTefal/perimeter/internal/corpus"
	"github.com/UnPoilTefal/perimeter/internal/perimeter"
)

// Sonde est le rejeu de la sonde d'une source : ce que la source a repondu,
// et — quand elle a mal repondu — ce qu'on attendait d'elle.
//
// Declarer une sonde n'est pas la passer. Un registre coherent dit que chaque
// source *annonce* comment se prouver ; seul le rejeu dit qu'elle y arrive.
type Sonde struct {
	Source       string `json:"source"`
	Cmd          string `json:"cmd"`
	Status       string `json:"status"`
	ExitCode     int    `json:"exit_code"`
	WantExit     int    `json:"want_exit"`
	Stdout       string `json:"stdout,omitempty"`
	ExpectStdout string `json:"expect_stdout,omitempty"`
	Reason       string `json:"reason,omitempty"`
}

// Passe dit si la source a repondu comme elle l'annonce.
func (s Sonde) Passe() bool { return s.Status == "pass" }

// Attendu formule ce qu'il aurait fallu observer. C'est la moitie utile d'un
// echec : sans elle, on sait que la source a mal repondu, pas ce qui aurait
// ete une bonne reponse.
func (s Sonde) Attendu() string {
	parts := []string{fmt.Sprintf("code de sortie %d", s.WantExit)}
	if s.ExpectStdout != "" {
		parts = append(parts, fmt.Sprintf("sortie conforme a /%s/", s.ExpectStdout))
	}
	return strings.Join(parts, ", et ")
}

// RapportSondes est le rejeu de toutes les sondes d'un registre.
type RapportSondes struct {
	Registre string  `json:"registre"`
	Sondes   []Sonde `json:"sondes"`
}

// Echecs compte les sources qui n'ont pas repondu comme annonce.
func (r RapportSondes) Echecs() int {
	n := 0
	for _, s := range r.Sondes {
		if !s.Passe() {
			n++
		}
	}
	return n
}

// ExitCode porte le verdict : non nul des qu'une sonde echoue, pour que ce
// soit branchable en CI et en pre-commit.
func (r RapportSondes) ExitCode() int {
	if r.Echecs() > 0 {
		return 1
	}
	return 0
}

// Ecrire rend le rapport pour un humain : une ligne par source, et le detail
// de ce qui manque sous celles qui echouent.
func (r RapportSondes) Ecrire(w io.Writer) error {
	var b strings.Builder
	fmt.Fprintf(&b, "%s — %d sources, %d sondes rejouees\n\n", r.Registre, len(r.Sondes), len(r.Sondes))

	largeur := 0
	for _, s := range r.Sondes {
		if len(s.Source) > largeur {
			largeur = len(s.Source)
		}
	}
	for _, s := range r.Sondes {
		if s.Passe() {
			fmt.Fprintf(&b, "  ✓ %-*s  %s\n", largeur, s.Source, repondu(s))
			continue
		}
		fmt.Fprintf(&b, "  ✗ %-*s  %s\n", largeur, s.Source, s.Reason)
		if s.Cmd != "" {
			fmt.Fprintf(&b, "      sonde   : %s\n", s.Cmd)
			fmt.Fprintf(&b, "      sortie  : %s\n", observee(s.Stdout))
			fmt.Fprintf(&b, "      attendu : %s\n", s.Attendu())
		}
	}

	if ko := r.Echecs(); ko > 0 {
		fmt.Fprintf(&b, "\n%d sonde%s sur %d en echec\n", ko, pluriel(ko), len(r.Sondes))
		fmt.Fprint(&b, "  tant qu'une source ne repond pas, tout ce qui s'y adosse ne prouve rien\n")
	} else if len(r.Sondes) > 0 {
		fmt.Fprint(&b, "\n✓ toutes les sources repondent comme elles l'annoncent\n")
	}
	_, err := io.WriteString(w, b.String())
	return err
}

func observee(stdout string) string {
	if strings.TrimSpace(stdout) == "" {
		return "(aucune sortie)"
	}
	return stdout
}

// repondu resume ce qu'une source qui passe a rendu. Beaucoup de sondes sont
// muettes et ne prouvent que par leur code de sortie : le dire vaut mieux que
// d'aligner des lignes vides.
func repondu(s Sonde) string {
	if strings.TrimSpace(s.Stdout) == "" {
		return fmt.Sprintf("code de sortie %d, aucune sortie", s.ExitCode)
	}
	return s.Stdout
}

func pluriel(n int) string {
	if n > 1 {
		return "s"
	}
	return ""
}

// Sonder rejoue la sonde de chaque source declaree, dans l'ordre des noms.
//
// Une source dont la sonde est inexploitable compte comme un echec plutot que
// comme une absence : c'est exactement le silence que ce rejeu existe pour
// rompre. Le schema rend la sonde obligatoire, ce cas ne devrait donc pas se
// produire — le traiter evite qu'une evolution du schema le rende muet.
func Sonder(reg *perimeter.Registry, timeout time.Duration) RapportSondes {
	if timeout <= 0 {
		timeout = TimeoutDe(reg)
	}
	rap := RapportSondes{Registre: reg.Path}
	for _, name := range reg.SourceNames() {
		cmd, err := reg.ProbeOf(name)
		if err != nil {
			rap.Sondes = append(rap.Sondes, Sonde{
				Source: name, Status: "error", ExitCode: -1, Reason: err.Error(),
			})
			continue
		}
		cr := runCheck(corpus.Check{
			Cmd: cmd.Cmd, ExpectExit: cmd.ExpectExit, ExpectStdout: cmd.ExpectStdout,
		}, timeout)
		rap.Sondes = append(rap.Sondes, Sonde{
			Source: name, Cmd: cmd.Cmd, Status: cr.Status,
			ExitCode: cr.ExitCode, WantExit: cr.WantExit,
			Stdout: cr.Stdout, ExpectStdout: cmd.ExpectStdout, Reason: cr.Reason,
		})
	}
	return rap
}

// TimeoutDe rend le delai par sonde. Il vient de la politique de corpus
// declaree au registre quand il y en a une : le registre reste le seul
// fichier qu'une equipe ecrit, y compris pour ce qui gouverne ses sondes.
func TimeoutDe(reg *perimeter.Registry) time.Duration {
	cfg := corpus.DefaultConfig()
	if reg != nil {
		if _, _, policy, err := reg.CorpusSource(); err == nil {
			cfg = corpus.ConfigFromPolicy(policy)
		}
	}
	return time.Duration(cfg.Verify.TimeoutSeconds) * time.Second
}
