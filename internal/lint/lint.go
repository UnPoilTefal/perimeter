// Package lint applique les regles structurelles d'un corpus de memoire.
//
// Le pari de l'outil : la memoire pourrit par defaut, et elle pourrit en
// silence. Chaque regle ici correspond a un mode de pourrissement observe —
// une note invisible au routeur, une description qui ne permet pas de decider
// du rappel, un fait qui n'a plus de preuve.
package lint

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/UnPoilTefal/perimeter/internal/corpus"
	"github.com/UnPoilTefal/perimeter/internal/report"
	"github.com/UnPoilTefal/perimeter/schema"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

// printer rend les messages du validateur. La bibliotheque exige un
// imprimeur non nil : lui passer nil fait paniquer.
var printer = message.NewPrinter(language.English)

// Options pilote une execution du lint.
type Options struct {
	Now     time.Time
	Disable []string
}

func (o Options) disabled(rule string) bool {
	for _, d := range o.Disable {
		if d == rule {
			return true
		}
	}
	return false
}

// Run applique toutes les regles actives au corpus.
func Run(c *corpus.Corpus, opt Options) (*report.Result, error) {
	if opt.Now.IsZero() {
		opt.Now = time.Now()
	}
	res := &report.Result{Command: "lint", Root: c.Root, Notes: len(c.Notes), Stats: map[string]any{}}

	compiled, err := compileSchema()
	if err != nil {
		return nil, err
	}

	rules := []struct {
		name string
		fn   func(*corpus.Corpus, Options, *report.Result)
	}{
		{"parse", ruleParse},
		{"schema", func(c *corpus.Corpus, o Options, r *report.Result) { ruleSchema(c, compiled, r) }},
		{"name-match", ruleNameMatch},
		{"index-orphan", ruleIndexOrphan},
		{"index-dangling", ruleIndexDangling},
		{"index-drift", ruleIndexDrift},
		{"wikilink", ruleWikilink},
		{"description", ruleDescription},
		{"atomicity", ruleAtomicity},
		{"ownership", ruleOwnership},
		{"staleness", ruleStaleness},
		{"secret", ruleSecret},
	}
	for _, r := range rules {
		if opt.disabled(r.name) {
			continue
		}
		r.fn(c, opt, res)
	}
	return res, nil
}

func compileSchema() (*jsonschema.Schema, error) {
	doc, err := jsonschema.UnmarshalJSON(strings.NewReader(string(schema.Memory)))
	if err != nil {
		return nil, err
	}
	cmp := jsonschema.NewCompiler()
	if err := cmp.AddResource("memory.schema.json", doc); err != nil {
		return nil, err
	}
	return cmp.Compile("memory.schema.json")
}

// ruleParse rapporte les notes dont le frontmatter est illisible. Une note
// non analysable est invisible a toutes les autres regles, donc c'est une
// erreur et pas un avertissement.
func ruleParse(c *corpus.Corpus, _ Options, res *report.Result) {
	for _, n := range c.Notes {
		if n.ParseErr != nil {
			res.Add(report.Finding{
				Rule: "parse", Severity: report.Error, File: n.Rel,
				Message: n.ParseErr.Error(),
				Hint:    "toute note doit ouvrir sur un bloc --- ... --- en YAML",
			})
		}
	}
}

func ruleSchema(c *corpus.Corpus, sch *jsonschema.Schema, res *report.Result) {
	for _, n := range c.Notes {
		if n.ParseErr != nil || n.Any == nil {
			continue
		}
		if err := sch.Validate(normalize(n.Any)); err != nil {
			for _, line := range schemaMessages(err) {
				res.Add(report.Finding{
					Rule: "schema", Severity: report.Error, File: n.Rel, Message: line,
					Hint: "frontmatter non conforme a schema/memory.schema.json",
				})
			}
		}
	}
}

func schemaMessages(err error) []string {
	ve, ok := err.(*jsonschema.ValidationError)
	if !ok {
		return []string{err.Error()}
	}
	var out []string
	var walk func(e *jsonschema.ValidationError)
	walk = func(e *jsonschema.ValidationError) {
		if len(e.Causes) == 0 {
			loc := e.InstanceLocation
			path := "/"
			if len(loc) > 0 {
				path = "/" + strings.Join(loc, "/")
			}
			out = append(out, fmt.Sprintf("%s : %s", path, e.ErrorKind.LocalizedString(printer)))
			return
		}
		for _, cause := range e.Causes {
			walk(cause)
		}
	}
	walk(ve)
	if len(out) > 4 {
		out = append(out[:4], fmt.Sprintf("… et %d autres", len(out)-4))
	}
	return out
}

// normalize convertit les cles YAML en chaines, ce que le validateur exige.
func normalize(v any) any {
	switch t := v.(type) {
	case map[string]any:
		m := make(map[string]any, len(t))
		for k, vv := range t {
			m[k] = normalize(vv)
		}
		return m
	case map[any]any:
		m := make(map[string]any, len(t))
		for k, vv := range t {
			m[fmt.Sprint(k)] = normalize(vv)
		}
		return m
	case []any:
		s := make([]any, len(t))
		for i, vv := range t {
			s[i] = normalize(vv)
		}
		return s
	case int:
		return float64(t)
	case time.Time:
		return t.Format(time.RFC3339)
	default:
		return v
	}
}

// ruleNameMatch : le champ name est l'identifiant utilise par les wikilinks.
// S'il diverge du nom de fichier, les liens pointent dans le vide.
func ruleNameMatch(c *corpus.Corpus, _ Options, res *report.Result) {
	for _, n := range c.Notes {
		if n.ParseErr != nil || n.Name == "" {
			continue
		}
		want := strings.TrimSuffix(n.Base, ".md")
		if n.Name != want {
			res.Add(report.Finding{
				Rule: "name-match", Severity: report.Error, File: n.Rel,
				Message: fmt.Sprintf("name: %q ne correspond pas au fichier (%q attendu)", n.Name, want),
				Hint:    "le name est la cle des liens [[…]] : il doit egaler le nom de fichier",
			})
		}
	}
}

// ruleIndexOrphan est la regle centrale du lot 1 : l'index est le routeur du
// rappel, donc une note absente de l'index est ecrite mais inaccessible.
func ruleIndexOrphan(c *corpus.Corpus, _ Options, res *report.Result) {
	if !c.HasIndex {
		return
	}
	cited := map[string]bool{}
	for _, t := range c.IndexTargets {
		cited[t] = true
	}
	orphans := 0
	for _, n := range c.Notes {
		if !cited[n.Rel] {
			orphans++
			res.Add(report.Finding{
				Rule: "index-orphan", Severity: report.Error, File: n.Rel,
				Message: "absente de l'index",
				Hint:    "l'index est le routeur du rappel : une note non citee est ecrite mais jamais lue",
			})
		}
	}
	res.Stats["index_orphans"] = orphans
}

func ruleIndexDangling(c *corpus.Corpus, _ Options, res *report.Result) {
	if !c.HasIndex {
		return
	}
	known := c.ByRel()
	seen := map[string]bool{}
	for _, t := range c.IndexTargets {
		if _, ok := known[t]; !ok {
			res.Add(report.Finding{
				Rule: "index-dangling", Severity: report.Error, File: c.Config.Corpus.Index,
				Message: fmt.Sprintf("l'index cite %s, qui n'existe pas", t),
				Hint:    "supprimer l'entree, ou restaurer la note",
			})
		}
		if seen[t] {
			res.Add(report.Finding{
				Rule: "index-dangling", Severity: report.Warn, File: c.Config.Corpus.Index,
				Message: fmt.Sprintf("%s est cite plusieurs fois", t),
			})
		}
		seen[t] = true
	}
}

// ruleIndexDrift compare l'accroche portee par l'index a la description de la
// note. L'accroche est de l'etat derive : quand elle diverge, le routeur
// annonce autre chose que ce que la note dit, et le rappel se fait sur une
// information fausse. « perctl index --sync » regenere.
func ruleIndexDrift(c *corpus.Corpus, _ Options, res *report.Result) {
	if !c.HasIndex {
		return
	}
	// En regime authored, l'accroche est ecrite pour un lecteur humain :
	// elle n'a aucune raison de reprendre la description, et la comparer
	// produit un constat par entree — du bruit qui noie le reste.
	if c.IndexHookAuthore() {
		res.Stats["index_drift"] = 0
		return
	}
	byRel := c.ByRel()
	drift, examinees := 0, 0
	for _, e := range c.IndexEntries {
		n, ok := byRel[e.Target]
		if !ok || n.ParseErr != nil || e.Hook == "" {
			continue
		}
		examinees++
		if e.Hook == n.IndexHook() {
			continue
		}
		drift++
		res.Add(report.Finding{
			Rule: "index-drift", Severity: report.Warn, File: c.Config.Corpus.Index,
			Message: fmt.Sprintf("l.%d %s : l'accroche ne reprend pas la description", e.Line, e.Target),
			Hint:    "l'accroche d'index est derivee de la description : regenerer avec « perctl index --sync »",
		})
	}
	res.Stats["index_drift"] = drift

	// Une seule commande regenere toutes les accroches d'un coup : la
	// remediation ne depend pas de l'identite des notes, donc quand la
	// derive atteint tout l'index, le constat est celui du processus qui
	// l'ecrit, pas celui de chaque note. Le denominateur est le nombre
	// d'entrees confrontees, pas le corpus : une note absente de l'index
	// est un constat d'un autre controle.
	//
	// La suggestion passe par la porte qui garde la commande elle-meme :
	// si « index --sync » refuse d'ecrire ici, on ne le propose pas.
	// L'invariant tient alors par construction, et non par le fait que la
	// regle se taise deja en regime authored.
	if drift > 0 && corpus.SyncIndexAutorise(c) == nil {
		res.DeclareAgregable("index-drift", report.Agregable{
			Constat:   "l'index n'est pas synchronise",
			Fix:       "perctl index --sync",
			Examinees: examinees,
		})
	}
}

var wikilink = regexp.MustCompile(`\[\[([^\]|#]+)`)

var hasWord = regexp.MustCompile(`[\p{L}\p{N}]`)

// ruleWikilink verifie l'integrite des liens, y compris ceux qui sortent du
// corpus : une note de memoire cite couramment un wiki ou un runbook, et un
// renommage cote voisin casse le lien sans que rien ne le signale.
func ruleWikilink(c *corpus.Corpus, _ Options, res *report.Result) {
	known := c.ByName()
	external := c.ExternalStems()
	illisibles := stemsIllisibles(c)
	seen := map[string]bool{}

	// entrants compte, par note illisible, les liens qui la citent ; ordre
	// preserve la sequence de decouverte pour un rapport stable.
	entrants := map[string]int{}
	var ordre []string

	for _, n := range c.Notes {
		if n.ParseErr != nil {
			continue
		}
		for _, m := range wikilink.FindAllStringSubmatch(n.Body, -1) {
			target := strings.TrimSpace(m[1])
			if target == "" || !hasWord.MatchString(target) {
				continue
			}
			if ignoredLink(c, target) {
				continue
			}
			if _, ok := known[target]; ok {
				continue
			}
			if _, ok := external[target]; ok {
				continue
			}
			key := n.Rel + "\x00" + target
			if seen[key] {
				continue
			}
			seen[key] = true

			// La cible existe, mais son frontmatter ne s'analyse pas : son
			// name est vide, donc elle est absente de ByName. Le defaut
			// appartient a cette note-la, pas a chacune de celles qui la
			// citent — sinon une seule note en defaut produit autant
			// d'alertes qu'elle a de liens entrants, et aucune ne designe
			// la cause.
			if rel, ok := illisibles[target]; ok {
				if entrants[rel] == 0 {
					ordre = append(ordre, rel)
				}
				entrants[rel]++
				continue
			}

			res.Add(report.Finding{
				Rule: "wikilink", Severity: report.Warn, File: n.Rel,
				Message: fmt.Sprintf("[[%s]] n'a pas de cible", target),
				Hint:    "lien casse, ou note a ecrire ; declarer les corpus voisins dans links.external_roots",
			})
		}
	}

	for _, rel := range ordre {
		res.Add(report.Finding{
			Rule: "wikilink", Severity: report.Warn, File: rel,
			Message: fmt.Sprintf("existe mais ne s'analyse pas : %d lien(s) entrant(s) ne se resolvent pas", entrants[rel]),
			Hint:    "corriger le frontmatter de cette note resout ces liens d'un coup ; la cause est le constat parse ci-dessus",
		})
	}
}

// stemsIllisibles indexe par nom de fichier, extension retiree, les notes
// dont le frontmatter ne s'analyse pas. Leur champ name est inaccessible,
// donc le stem est la seule cle disponible — et la regle name-match garantit
// que les deux coincident sur une note saine.
func stemsIllisibles(c *corpus.Corpus) map[string]string {
	m := map[string]string{}
	for _, n := range c.Notes {
		if n.ParseErr == nil {
			continue
		}
		m[strings.TrimSuffix(filepath.Base(n.Rel), ".md")] = n.Rel
	}
	return m
}

func ignoredLink(c *corpus.Corpus, target string) bool {
	for _, p := range c.Config.Links.IgnorePrefixes {
		if p != "" && strings.HasPrefix(target, p) {
			return true
		}
	}
	return false
}

// descriptions trop generiques pour permettre une decision de rappel.
var vagueDesc = regexp.MustCompile(`(?i)^(notes?|infos?|divers|contexte|a propos|about|todo|wip)\b`)

// ruleDescription : c'est la description, pas le corps, que l'agent lit pour
// decider si la note est pertinente. Une mauvaise description rend la note
// aussi inutile qu'une note absente de l'index.
func ruleDescription(c *corpus.Corpus, _ Options, res *report.Result) {
	for _, n := range c.Notes {
		if n.ParseErr != nil || n.Desc == "" {
			continue
		}
		slug := strings.ReplaceAll(strings.TrimSuffix(n.Base, ".md"), "-", " ")
		if strings.EqualFold(strings.TrimSpace(n.Desc), slug) {
			res.Add(report.Finding{
				Rule: "description", Severity: report.Warn, File: n.Rel,
				Message: "la description ne fait que repeter le nom",
				Hint:    "la description doit enoncer le fait, pas le sujet : c'est elle qui declenche le rappel",
			})
			continue
		}
		if vagueDesc.MatchString(n.Desc) {
			res.Add(report.Finding{
				Rule: "description", Severity: report.Warn, File: n.Rel,
				Message: fmt.Sprintf("description trop generique : %q", truncate(n.Desc, 60)),
				Hint:    "la description doit enoncer le fait, pas le sujet : c'est elle qui declenche le rappel",
			})
		}
	}
}

// ruleAtomicity : un fait par fichier. Sans cette contrainte, le
// dedoublonnage et la peremption deviennent impraticables.
func ruleAtomicity(c *corpus.Corpus, _ Options, res *report.Result) {
	max := c.Config.Policy.MaxBodyWords
	for _, n := range c.Notes {
		if n.ParseErr != nil || n.BodyWords <= max {
			continue
		}
		res.Add(report.Finding{
			Rule: "atomicity", Severity: report.Warn, File: n.Rel,
			Message: fmt.Sprintf("%d mots (budget %d)", n.BodyWords, max),
			Hint:    "un fait par note : au-dela du budget, scinder — une note large ne se perime jamais proprement",
		})
	}
}

// ruleOwnership : sans proprietaire designe, personne ne supprime jamais rien.
func ruleOwnership(c *corpus.Corpus, _ Options, res *report.Result) {
	if !c.Config.Policy.RequireOwner {
		return
	}
	for _, n := range c.Notes {
		if n.ParseErr != nil || n.Metadata.Owner != "" {
			continue
		}
		res.Add(report.Finding{
			Rule: "ownership", Severity: report.Warn, File: n.Rel,
			Message: "metadata.owner absent",
			Hint:    "sans proprietaire, la note ne sera jamais relue ni supprimee",
		})
	}
}

// ruleStaleness compare chaque note a son echeance de relecture, et le corpus
// entier au budget de peremption tolere.
func ruleStaleness(c *corpus.Corpus, opt Options, res *report.Result) {
	budget := c.Config.Policy.Staleness.ReviewAfterDays
	stale, dated := 0, 0
	for _, n := range c.Notes {
		if n.ParseErr != nil {
			continue
		}
		due, ok := n.ReviewDue(budget)
		if !ok {
			// Une note qu'aucune date ne rattache au temps est hors de
			// portee de toute peremption. Un avertissement ne l'a jamais
			// fait corriger : sur un corpus reel, 18 % des notes etaient
			// dans ce cas, et le signal se noyait dans le bruit.
			severite := report.Warn
			if c.Config.Policy.RequireDate {
				severite = report.Error
			}
			res.Add(report.Finding{
				Rule: "staleness", Severity: severite, File: n.Rel,
				Message: "aucune date exploitable (ni modified, ni verified_at, ni review_after)",
				Hint:    "sans date, la peremption d'une note est invisible — poser modified, ou declarer policy.require_date: false",
			})
			continue
		}
		dated++
		if opt.Now.After(due) {
			stale++
			res.Add(report.Finding{
				Rule: "staleness", Severity: report.Warn, File: n.Rel,
				Message: fmt.Sprintf("a relire depuis le %s (%d jours)", due.Format("2006-01-02"), int(opt.Now.Sub(due).Hours()/24)),
				Hint:    fmt.Sprintf("budget de relecture : %d jours — relire, puis mettre a jour modified ou review_after", budget),
			})
		}
	}
	ratio := 0.0
	if dated > 0 {
		ratio = float64(stale) / float64(dated)
	}
	res.Stats["stale"] = stale
	res.Stats["stale_ratio"] = ratio
	if max := c.Config.Policy.Staleness.MaxStaleRatio; max > 0 && ratio > max {
		res.Add(report.Finding{
			Rule: "staleness-budget", Severity: report.Error, File: c.Config.Corpus.Index,
			Message: fmt.Sprintf("%.0f%% du corpus est perime (plafond %.0f%%)", ratio*100, max*100),
			Hint:    "le corpus derive plus vite qu'il n'est relu : purger ou relire avant d'ajouter",
		})
	}
}

// secretPatterns attrape les fuites les plus courantes. Ce n'est pas un
// remplacement de gitleaks en CI, c'est un garde-fou avant le commit.
var secretPatterns = []struct {
	name string
	re   *regexp.Regexp
}{
	{"token GitHub", regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9]{20,}`)},
	{"token GitLab", regexp.MustCompile(`\bglpat-[A-Za-z0-9_\-]{16,}`)},
	{"cle privee", regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----`)},
	{"cle AWS", regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`)},
	{"JWT", regexp.MustCompile(`\beyJ[A-Za-z0-9_\-]{10,}\.[A-Za-z0-9_\-]{10,}\.[A-Za-z0-9_\-]{10,}`)},
	{"mot de passe en clair", regexp.MustCompile(`(?i)\b(password|passwd|secret|api[_-]?key|token)\s*[:=]\s*["']?[A-Za-z0-9/+=_\-]{12,}`)},
}

// ruleSecret : la memoire est un vecteur de fuite particulierement mauvais,
// parce qu'elle est rechargee et recopiee dans chaque session.
func ruleSecret(c *corpus.Corpus, _ Options, res *report.Result) {
	for _, n := range c.Notes {
		hay := n.Body + "\n" + n.Desc
		for _, p := range secretPatterns {
			if p.re.MatchString(hay) {
				res.Add(report.Finding{
					Rule: "secret", Severity: report.Error, File: n.Rel,
					Message: fmt.Sprintf("ressemble a un secret : %s", p.name),
					Hint:    "jamais de secret en memoire : elle est rechargee et recopiee a chaque session",
				})
			}
		}
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
