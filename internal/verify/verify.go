// Package verify rejoue les preuves attachees aux notes.
//
// C'est la boucle qui distingue un corpus de memoire d'un wiki : un fait qui
// porte une commande de verification cesse d'etre une affirmation datee et
// devient une assertion testable. Une note qui echoue n'est pas supprimee —
// elle est signalee, parce que c'est un humain qui decide si c'est le fait ou
// le monde qui a change.
//
// Consequence de securite assumee : les commandes sont du code executable
// vivant dans des fichiers de documentation. L'execution est donc refusee par
// defaut et exige AllowExec, et un corpus partage doit proteger ses notes par
// CODEOWNERS au meme titre que sa CI.
package verify

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/UnPoilTefal/perimeter/internal/corpus"
	"github.com/UnPoilTefal/perimeter/internal/perimeter"
	"github.com/UnPoilTefal/perimeter/internal/report"
	"gopkg.in/yaml.v3"
)

// ErrExecRefused est rendue quand des preuves existent mais que l'execution
// n'a pas ete autorisee explicitement.
var ErrExecRefused = errors.New("execution refusee : relancer avec --allow-exec")

// Options pilote une execution de verify.
type Options struct {
	AllowExec bool
	Write     bool
	Timeout   time.Duration
	Only      string // sous-chaine filtrant les notes par nom
	Now       time.Time

	// Registry, quand il est fourni, resout les preuves qui renvoient a une
	// source declaree. Sans lui, ces preuves echouent nommement plutot que
	// d'etre silencieusement ignorees : une preuve non jouee ne prouve rien,
	// et le taire donnerait un verdict faussement vert.
	Registry *perimeter.Registry

	// ProbeSources rejoue aussi la sonde de chaque source declaree. Une
	// source injoignable invalide toutes les preuves qui en dependent.
	ProbeSources bool
}

// Outcome est le resultat pour une note.
type Outcome struct {
	Note   string        `json:"note"`
	Status string        `json:"status"`
	Checks []CheckResult `json:"checks,omitempty"`
}

// CheckResult est le resultat d'une commande.
type CheckResult struct {
	Cmd      string `json:"cmd"`
	Status   string `json:"status"`
	ExitCode int    `json:"exit_code"`
	WantExit int    `json:"want_exit"`
	Stdout   string `json:"stdout,omitempty"`
	Reason   string `json:"reason,omitempty"`
}

// Run rejoue les preuves du corpus.
func Run(c *corpus.Corpus, opt Options) (*report.Result, []Outcome, error) {
	if opt.Now.IsZero() {
		opt.Now = time.Now()
	}
	if opt.Timeout <= 0 {
		opt.Timeout = time.Duration(c.Config.Verify.TimeoutSeconds) * time.Second
	}

	res := &report.Result{Command: "verify", Root: c.Root, Notes: len(c.Notes), Stats: map[string]any{}}
	var outcomes []Outcome

	var withChecks []*corpus.Note
	for _, n := range c.Notes {
		if n.ParseErr != nil || len(n.Metadata.Verify) == 0 {
			continue
		}
		if opt.Only != "" && !strings.Contains(n.Name, opt.Only) {
			continue
		}
		withChecks = append(withChecks, n)
	}

	viaSource := 0
	for _, n := range withChecks {
		for _, c := range n.Metadata.Verify {
			if c.ViaSource() {
				viaSource++
			}
		}
	}
	res.Stats["verifiable"] = len(withChecks)
	res.Stats["checks_via_source"] = viaSource
	res.Stats["coverage"] = coverage(c, len(withChecks))

	// Le rejeu des sondes ne depend pas de l'etat des preuves de notes : un
	// corpus qui n'en porte aucune est justement celui de quelqu'un qui vient
	// de poser son registre, et qui veut savoir si ses sources repondent.
	// Rendre la main ici laissait « --probe-sources » inerte et muet.
	sonder := opt.ProbeSources && opt.Registry != nil
	if len(withChecks) == 0 && !sonder {
		return res, outcomes, nil
	}
	if !opt.AllowExec {
		return res, outcomes, ErrExecRefused
	}

	if sonder {
		outcomes = append(outcomes, probeSources(opt, res)...)
	}

	pass, fail := 0, 0
	for _, n := range withChecks {
		out := Outcome{Note: n.Rel, Status: "pass"}
		for _, chk := range n.Metadata.Verify {
			cr := runResolved(chk, opt)
			out.Checks = append(out.Checks, cr)
			if cr.Status != "pass" && out.Status == "pass" {
				out.Status = cr.Status
			}
		}
		outcomes = append(outcomes, out)

		switch out.Status {
		case "pass":
			pass++
		default:
			fail++
			for _, cr := range out.Checks {
				if cr.Status == "pass" {
					continue
				}
				res.Add(report.Finding{
					Rule: "verify", Severity: report.Error, File: n.Rel,
					Message: fmt.Sprintf("%s — %s", truncate(cr.Cmd, 70), cr.Reason),
					Hint:    "la preuve ne tient plus : relire le fait, puis corriger la note ou le monde",
				})
			}
		}

		if opt.Write {
			if err := writeStatus(n, out.Status, opt.Now); err != nil {
				res.Add(report.Finding{
					Rule: "verify-write", Severity: report.Warn, File: n.Rel,
					Message: fmt.Sprintf("mise a jour impossible : %v", err),
				})
			}
		}
	}
	res.Stats["pass"] = pass
	res.Stats["fail"] = fail
	return res, outcomes, nil
}

// probeSources rejoue la sonde de chaque source declaree. C'est l'axiome
// applique un cran au-dessus : une boite qui ne verifie que ses faits finit
// par affirmer sereinement qu'aucun ticket ne contredit, parce que son jeton
// a expire trois semaines plus tot.
func probeSources(opt Options, res *report.Result) []Outcome {
	rap := Sonder(opt.Registry, opt.Timeout)
	var outcomes []Outcome
	ok := 0
	for _, s := range rap.Sondes {
		cr := CheckResult{
			Cmd: "sonde " + s.Source, Status: s.Status,
			ExitCode: s.ExitCode, WantExit: s.WantExit,
			Stdout: s.Stdout, Reason: s.Reason,
		}
		outcomes = append(outcomes, Outcome{
			Note: "source:" + s.Source, Status: s.Status, Checks: []CheckResult{cr},
		})
		if s.Passe() {
			ok++
			continue
		}
		res.Add(report.Finding{
			Rule: "source", Severity: report.Error, File: opt.Registry.Path,
			Message: fmt.Sprintf("source %s : %s", s.Source, s.Reason),
			Hint:    "tant que la source ne repond pas, les preuves qui en dependent ne prouvent rien",
		})
	}
	res.Stats["sources_ok"] = ok
	res.Stats["sources_ko"] = rap.Echecs()
	return outcomes
}

func coverage(c *corpus.Corpus, verifiable int) float64 {
	total := 0
	for _, n := range c.Notes {
		if n.ParseErr == nil {
			total++
		}
	}
	if total == 0 {
		return 0
	}
	return float64(verifiable) / float64(total)
}

// runResolved resout une preuve avant de l'executer : une preuve adossee a
// une source emprunte l'interrogation declaree au registre, ce qui la rend
// independante de l'outil concret.
func runResolved(chk corpus.Check, opt Options) CheckResult {
	if !chk.ViaSource() {
		return runCheck(chk, opt.Timeout)
	}
	label := fmt.Sprintf("source %s", chk.Source)
	if chk.Arg != "" {
		label += " (" + chk.Arg + ")"
	}
	if opt.Registry == nil {
		return CheckResult{
			Cmd: label, Status: "error", ExitCode: -1,
			Reason: "aucun registre de perimetre charge : relancer avec --perimeter",
		}
	}
	cmd, err := opt.Registry.Resolve(chk.Source, chk.Arg)
	if err != nil {
		return CheckResult{Cmd: label, Status: "error", ExitCode: -1, Reason: err.Error()}
	}
	// L'attente declaree dans la note prime sur celle de la source : c'est le
	// fait qui est verifie, pas la source.
	resolved := corpus.Check{Cmd: cmd.Cmd, ExpectExit: cmd.ExpectExit, ExpectStdout: cmd.ExpectStdout}
	if chk.ExpectExit != nil {
		resolved.ExpectExit = chk.ExpectExit
	}
	if chk.ExpectStdout != "" {
		resolved.ExpectStdout = chk.ExpectStdout
	}
	cr := runCheck(resolved, opt.Timeout)
	cr.Cmd = label
	return cr
}

// RunCheck rejoue une preuve isolee. Expose pour que la porte de readiness
// puisse verifier la preuve d'une resolution sans reimplementer l'execution
// ni ses garde-fous.
func RunCheck(chk corpus.Check, timeout time.Duration) CheckResult {
	return runCheck(chk, timeout)
}

func runCheck(chk corpus.Check, timeout time.Duration) CheckResult {
	cr := CheckResult{Cmd: chk.Cmd, WantExit: chk.WantExit()}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", chk.Cmd)
	cmd.Env = os.Environ()
	isolateProcessGroup(cmd)
	// Filet de securite : meme si un descendant survit a l'annulation, la
	// lecture du resultat doit rendre la main.
	cmd.WaitDelay = 2 * time.Second
	out, err := cmd.Output()
	stdout := strings.TrimSpace(string(out))
	cr.Stdout = truncate(stdout, 200)

	switch {
	case ctx.Err() == context.DeadlineExceeded:
		cr.Status, cr.Reason, cr.ExitCode = "error", fmt.Sprintf("delai depasse (%s)", timeout), -1
		return cr
	case err != nil:
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			cr.ExitCode = ee.ExitCode()
		} else {
			cr.Status, cr.Reason, cr.ExitCode = "error", err.Error(), -1
			return cr
		}
	}

	if cr.ExitCode != cr.WantExit {
		cr.Status = "fail"
		cr.Reason = fmt.Sprintf("code de sortie %d, attendu %d", cr.ExitCode, cr.WantExit)
		return cr
	}
	if chk.ExpectStdout != "" {
		re, err := regexp.Compile(chk.ExpectStdout)
		if err != nil {
			cr.Status, cr.Reason = "error", fmt.Sprintf("expect_stdout invalide : %v", err)
			return cr
		}
		if !re.MatchString(stdout) {
			cr.Status = "fail"
			cr.Reason = fmt.Sprintf("sortie %q ne satisfait pas /%s/", truncate(stdout, 60), chk.ExpectStdout)
			return cr
		}
	}
	cr.Status = "pass"
	return cr
}

// writeStatus met a jour verified_at et verify_status en place. On passe par
// l'arbre YAML plutot que par un re-marshal complet, pour ne pas reordonner
// les cles ni perdre les commentaires d'une note ecrite a la main.
func writeStatus(n *corpus.Note, status string, now time.Time) error {
	if n.Root == nil {
		return errors.New("frontmatter non analysable")
	}
	meta := mappingValue(n.Root, "metadata")
	if meta == nil {
		return errors.New("bloc metadata absent")
	}
	setScalar(meta, "verified_at", now.UTC().Format(time.RFC3339))
	setScalar(meta, "verify_status", status)

	var buf strings.Builder
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(n.Root); err != nil {
		return err
	}
	if err := enc.Close(); err != nil {
		return err
	}

	body := n.Body
	if body != "" {
		body += "\n"
	}
	content := "---\n" + buf.String() + "---\n\n" + body

	info, err := os.Stat(n.Path)
	mode := os.FileMode(0o644)
	if err == nil {
		mode = info.Mode()
	}
	return os.WriteFile(n.Path, []byte(content), mode)
}

func mappingValue(m *yaml.Node, key string) *yaml.Node {
	if m == nil || m.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	return nil
}

func setScalar(m *yaml.Node, key, value string) {
	if m.Kind != yaml.MappingNode {
		return
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			m.Content[i+1].Kind = yaml.ScalarNode
			m.Content[i+1].Tag = "!!str"
			m.Content[i+1].Value = value
			return
		}
	}
	m.Content = append(m.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value},
	)
}

func truncate(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
