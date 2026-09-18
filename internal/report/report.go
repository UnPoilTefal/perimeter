// Package report normalise les constats produits par les commandes et leurs
// formats de sortie : humain pour le terminal, JSON pour un agent, annotations
// GitHub Actions pour la CI.
package report

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
)

// Severity ordonne les constats. Error fait echouer, Warn seulement en --strict.
type Severity string

const (
	Error Severity = "error"
	Warn  Severity = "warn"
	Info  Severity = "info"
)

// Finding est un constat unitaire attache a un fichier.
type Finding struct {
	Rule     string   `json:"rule"`
	Severity Severity `json:"severity"`
	File     string   `json:"file"`
	Message  string   `json:"message"`
	// Hint dit quoi faire, pas seulement ce qui ne va pas.
	Hint string `json:"hint,omitempty"`
}

// Agregable declare qu'un controle porte une remediation unique : une seule
// commande repare toutes ses occurrences a la fois, quelle que soit l'identite
// des unites concernees. C'est cette propriete — et non le nombre
// d'occurrences — qui autorise a remplacer N constats de notes par un constat
// de processus.
//
// Un controle qui ne la declare pas n'est jamais replie, meme a 100 % :
// « atomicity » atteint tout un corpus sans qu'aucune commande ne le repare,
// parce que chaque note demande son propre arbitrage. Replier ses constats
// perdrait la seule information qu'ils portent, l'identite des notes.
type Agregable struct {
	// Constat enonce le fait de processus, au singulier.
	Constat string `json:"constat"`
	// Fix est la commande qui le repare, en entier. Le controle ne la
	// declare que si la politique du corpus l'autorise : on ne suggere
	// jamais une commande qui refusera de s'executer.
	Fix string `json:"fix"`
	// Examinees est la taille du perimetre que le controle a parcouru —
	// le denominateur du taux. C'est le controle qui le declare : il seul
	// sait ce qu'il pouvait atteindre, qui n'est pas toujours le corpus
	// entier.
	Examinees int `json:"examinees"`
}

const (
	// SeuilAgregation est la part du perimetre examine a partir de laquelle
	// l'identite des unites concernees cesse d'etre l'information.
	//
	// 0,9 et non 1 : calibre sur des mesures, pas sur une intuition. Le taux
	// le plus haut observe pour un controle qu'on veut detaille est 29 %
	// (24 atomicity sur 82 notes) ; 90 % en laisse un facteur trois. Et
	// exiger la totalite serait tenir le repli a un fil : sur le corpus
	// mesure, une seule accroche corrigee a la main fait retomber 116
	// constats — le bruit revient pour une raison qui n'en est pas une.
	SeuilAgregation = 0.9
	// PlancherAgregation est le nombre d'occurrences en deca duquel on ne
	// replie pas. Une liste de neuf lignes ne noie rien ; et 100 % d'une
	// poignee n'est pas la preuve d'un processus, c'est une coincidence.
	PlancherAgregation = 10
)

// Processus est le constat rendu a la place des N constats d'unites.
type Processus struct {
	Rule       string   `json:"rule"`
	Severity   Severity `json:"severity"`
	File       string   `json:"file"`
	Constat    string   `json:"constat"`
	Fix        string   `json:"fix"`
	Concernees int      `json:"concernees"`
	Examinees  int      `json:"examinees"`
}

// Rendu decrit comment ecrire un resultat.
type Rendu struct {
	Format string
	Color  bool
	// Verbose rend le detail par unite des controles replies. Le repli est
	// une decision d'affichage : il ne retire rien du resultat, ni des
	// severites, ni du code de sortie.
	Verbose bool
}

// Result agrege les constats d'une execution.
type Result struct {
	Command  string         `json:"command"`
	Root     string         `json:"root"`
	Notes    int            `json:"notes"`
	Findings []Finding      `json:"findings"`
	Stats    map[string]any `json:"stats,omitempty"`
	// Agregables porte les declarations des controles, par regle. Rendu
	// au travers de « processus », qui y ajoute la mesure.
	Agregables map[string]Agregable `json:"-"`
}

// DeclareAgregable enregistre qu'un controle porte une remediation unique.
func (r *Result) DeclareAgregable(rule string, a Agregable) {
	if r.Agregables == nil {
		r.Agregables = map[string]Agregable{}
	}
	r.Agregables[rule] = a
}

// Add enregistre un constat.
func (r *Result) Add(f Finding) { r.Findings = append(r.Findings, f) }

// Count compte les constats d'une severite.
func (r *Result) Count(s Severity) int {
	n := 0
	for _, f := range r.Findings {
		if f.Severity == s {
			n++
		}
	}
	return n
}

// ExitCode rend 1 s'il y a de quoi echouer, 0 sinon.
func (r *Result) ExitCode(strict bool) int {
	if r.Count(Error) > 0 || (strict && r.Count(Warn) > 0) {
		return 1
	}
	return 0
}

func (r *Result) sorted() []Finding {
	out := append([]Finding(nil), r.Findings...)
	rank := map[Severity]int{Error: 0, Warn: 1, Info: 2}
	sort.SliceStable(out, func(i, j int) bool {
		if rank[out[i].Severity] != rank[out[j].Severity] {
			return rank[out[i].Severity] < rank[out[j].Severity]
		}
		if out[i].Rule != out[j].Rule {
			return out[i].Rule < out[j].Rule
		}
		return out[i].File < out[j].File
	})
	return out
}

// Processus rend, par regle, les controles dont les constats se replient en un
// fait de processus : ceux qui ont declare une remediation unique, et qui
// atteignent une part du perimetre telle que l'identite des unites n'est plus
// l'information.
//
// Le repli n'est qu'un rendu : les constats restent tous dans Findings, donc
// les severites, les comptes et le code de sortie sont inchanges.
func (r *Result) Processus() []Processus {
	if len(r.Agregables) == 0 {
		return nil
	}
	premier := map[string]Finding{}
	compte := map[string]int{}
	for _, f := range r.Findings {
		if _, vu := premier[f.Rule]; !vu {
			premier[f.Rule] = f
		}
		compte[f.Rule]++
	}
	var out []Processus
	for rule, a := range r.Agregables {
		n := compte[rule]
		if n < PlancherAgregation || a.Examinees <= 0 {
			continue
		}
		if float64(n)/float64(a.Examinees) < SeuilAgregation {
			continue
		}
		f := premier[rule]
		out = append(out, Processus{
			Rule: rule, Severity: f.Severity, File: f.File,
			Constat: a.Constat, Fix: a.Fix,
			Concernees: n, Examinees: a.Examinees,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Rule < out[j].Rule })
	return out
}

// replies indexe par regle les constats de processus a rendre.
func (r *Result) replies(verbose bool) map[string]Processus {
	m := map[string]Processus{}
	if verbose {
		return m
	}
	for _, p := range r.Processus() {
		m[p.Rule] = p
	}
	return m
}

// Write rend le resultat dans le format demande.
func (r *Result) Write(w io.Writer, o Rendu) error {
	switch o.Format {
	case "json":
		// Un agent consomme le detail : les constats sont tous la, et
		// « processus » s'y ajoute plutot qu'il ne s'y substitue.
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(struct {
			*Result
			Processus []Processus `json:"processus,omitempty"`
		}{r, r.Processus()})
	case "github":
		// Une annotation par note pour un seul fait, c'est le bruit de
		// l'issue a l'endroit ou il coute le plus cher : la relecture
		// d'une pull request. Le repli vaut donc ici aussi.
		replies := r.replies(o.Verbose)
		for _, p := range r.Processus() {
			if _, ok := replies[p.Rule]; !ok {
				continue
			}
			if _, err := fmt.Fprintf(w, "::%s file=%s,title=%s::%s (%d/%d notes) — %s\n",
				niveau(p.Severity), p.File, p.Rule, p.Constat, p.Concernees, p.Examinees, p.Fix); err != nil {
				return err
			}
		}
		for _, f := range r.sorted() {
			if _, replie := replies[f.Rule]; replie {
				continue
			}
			msg := f.Message
			if f.Hint != "" {
				msg += " — " + f.Hint
			}
			if _, err := fmt.Fprintf(w, "::%s file=%s,title=%s::%s\n", niveau(f.Severity), f.File, f.Rule, strings.ReplaceAll(msg, "\n", " ")); err != nil {
				return err
			}
		}
		return nil
	default:
		return r.writeHuman(w, o.Color, o.Verbose)
	}
}

func niveau(s Severity) string {
	if s == Error {
		return "error"
	}
	return "warning"
}

type palette struct{ red, yellow, dim, bold, reset string }

func newPalette(on bool) palette {
	if !on {
		return palette{}
	}
	return palette{red: "\x1b[31m", yellow: "\x1b[33m", dim: "\x1b[2m", bold: "\x1b[1m", reset: "\x1b[0m"}
}

func (r *Result) writeHuman(w io.Writer, color, verbose bool) error {
	p := newPalette(color)
	if len(r.Findings) == 0 {
		_, err := fmt.Fprintf(w, "%s✓%s %d notes, aucun constat\n", p.bold, p.reset, r.Notes)
		return err
	}

	replies := r.replies(verbose)

	byRule := map[string][]Finding{}
	var order []string
	for _, f := range r.sorted() {
		if _, seen := byRule[f.Rule]; !seen {
			order = append(order, f.Rule)
		}
		byRule[f.Rule] = append(byRule[f.Rule], f)
	}

	for _, rule := range order {
		fs := byRule[rule]
		mark, col := "!", p.yellow
		if fs[0].Severity == Error {
			mark, col = "x", p.red
		}
		// Un controle replie dit le fait une fois, avec la mesure qui le
		// fonde et la commande qui le repare. Le detail des notes n'est
		// pas perdu : « --verbose » et « --format json » le rendent.
		if pr, replie := replies[rule]; replie {
			fmt.Fprintf(w, "\n%s%s %s%s : %s %s(%d/%d notes)%s\n", col, mark, rule, p.reset, pr.Constat, p.dim, pr.Concernees, pr.Examinees, p.reset) //nolint:errcheck // sortie terminal
			fmt.Fprintf(w, "  → %s\n", pr.Fix)                                                                                                        //nolint:errcheck // sortie terminal
			continue
		}
		fmt.Fprintf(w, "\n%s%s %s%s %s(%d)%s\n", col, mark, rule, p.reset, p.dim, len(fs), p.reset) //nolint:errcheck // sortie terminal
		if fs[0].Hint != "" {
			fmt.Fprintf(w, "  %s%s%s\n", p.dim, fs[0].Hint, p.reset) //nolint:errcheck // sortie terminal
		}
		for _, f := range fs {
			fmt.Fprintf(w, "    %s  %s\n", f.File, f.Message) //nolint:errcheck // sortie terminal
		}
	}

	fmt.Fprintf(w, "\n%s%d notes%s — %s%d erreurs%s, %s%d avertissements%s\n", //nolint:errcheck // sortie terminal
		p.bold, r.Notes, p.reset, p.red, r.Count(Error), p.reset, p.yellow, r.Count(Warn), p.reset)

	// Les comptes restent ceux des constats, pas des blocs affiches : un
	// repli ne fait pas disparaitre d'avertissement, et --strict continue
	// d'echouer dessus. Reste a dire ou est passe le detail.
	if n := len(replies); n > 0 {
		s := ""
		if n > 1 {
			s = "s"
		}
		_, err := fmt.Fprintf(w, "%s%d controle%s replie%s en constat de processus — « lint --verbose » pour le detail par note%s\n",
			p.dim, n, s, s, p.reset)
		return err
	}
	return nil
}
