// perctl outille un perimetre : l'ensemble de ce qu'un agent doit connaitre
// pour agir sans se tromper de premisse.
//
// Il verifie la structure du corpus de memoire (lint), rejoue les preuves
// attachees aux faits et aux sources (verify), valide le registre des briques
// qui servent le perimetre (perimeter), et derive le verdict de readiness
// d'une specification (gate).
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/UnPoilTefal/perimeter/internal/advisory"
	"github.com/UnPoilTefal/perimeter/internal/ciguard"
	"github.com/UnPoilTefal/perimeter/internal/claim"
	"github.com/UnPoilTefal/perimeter/internal/corpus"
	"github.com/UnPoilTefal/perimeter/internal/draft"
	"github.com/UnPoilTefal/perimeter/internal/harvest"
	"github.com/UnPoilTefal/perimeter/internal/lint"
	"github.com/UnPoilTefal/perimeter/internal/perimeter"
	"github.com/UnPoilTefal/perimeter/internal/probediff"
	"github.com/UnPoilTefal/perimeter/internal/readiness"
	"github.com/UnPoilTefal/perimeter/internal/reliability"
	"github.com/UnPoilTefal/perimeter/internal/report"
	"github.com/UnPoilTefal/perimeter/internal/resolution"
	"github.com/UnPoilTefal/perimeter/internal/verify"
	"github.com/UnPoilTefal/perimeter/schema"
	"golang.org/x/term"
)

// version est renseignee au build (-ldflags "-X main.version=…").
var version = "dev"

const usage = `perctl — savoir si un agent peut agir sur un perimetre

  perctl lint   [chemin]   verifie la structure du corpus
  perctl verify [chemin]   rejoue les preuves attachees aux faits
  perctl index  [chemin]   compare l'index au corpus (--fix pour completer)
  perctl perimeter [reg]   valide le registre des sources du perimetre
                           (sans [reg] : meme recherche que lint ;
                            --probe --allow-exec : rejoue les sondes declarees)
  perctl gate <evaluation> derive le verdict de readiness d'une specification
  perctl readiness         etat de sortie de demarrage, et regime qui en decoule
  perctl propose [chemin]  propose une sonde pour les notes qui n'en portent pas
  perctl draft  [fichier]  juge un brouillon avant de l'ecrire (« - » ou rien : entree standard)
  perctl harvest           propose des candidats depuis les traces deja la (--allow-exec)
  perctl reliability check <ref>
                           verdict de fiabilite pour une reference du perimetre
  perctl audit <chemin>[#Lnn] [--origine ...]
                           marque un fait douteux dans une note non-schemee
                           (« [!verification]- » ; sans #Lnn, mode interactif
                            ligne par ligne, sur un fichier ou un dossier)
  perctl init   [chemin]   ecrit un perimeter.yml : le registre du perimetre
  perctl schema            ecrit le JSON Schema sur la sortie standard
  perctl version

Un chemin designe le corpus, un --perimeter la politique : les deux se
combinent. Un registre seulement ambiant (PERIMETER, ou trouve en remontant)
ne s'applique pas a un corpus designe par un chemin.

Le chemin vaut « . » par defaut. Detail des regles et mode d'emploi : README.md

Le registre est cherche, dans cet ordre : --perimeter, la variable PERIMETER,
la remontee d'arborescence depuis le repertoire courant, puis
~/.config/perimeter/perimeter.yml — ce dernier niveau sert l'usage « tour de
controle », ou l'on travaille depuis un repertoire central sans registre.
Toutes les sous-commandes empruntent cette recherche, « perimeter » comprise ;
elle seule n'a pas de --perimeter, son chemin positionnel etant deja le
registre.
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "lint":
		err = cmdLint(os.Args[2:])
	case "verify":
		err = cmdVerify(os.Args[2:])
	case "index":
		err = cmdIndex(os.Args[2:])
	case "perimeter":
		err = cmdPerimeter(os.Args[2:])
	case "gate":
		err = cmdGate(os.Args[2:])
	case "readiness":
		err = cmdReadiness(os.Args[2:])
	case "propose":
		err = cmdPropose(os.Args[2:])
	case "draft":
		err = cmdDraft(os.Args[2:])
	case "harvest":
		err = cmdHarvest(os.Args[2:])
	case "reliability":
		err = cmdReliability(os.Args[2:])
	case "audit":
		err = cmdAudit(os.Args[2:])
	case "init":
		err = cmdInit(os.Args[2:])
	case "schema":
		_, err = os.Stdout.Write(schema.Memory)
	case "version":
		fmt.Printf("perctl %s\n", version)
	case "-h", "--help", "help":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "commande inconnue : %s\n\n%s", os.Args[1], usage)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "perctl: %v\n", err)
		os.Exit(1)
	}
}

// fail termine avec un code sans message supplementaire : la sortie a deja
// tout dit.
func fail(code int) error {
	if code == 0 {
		return nil
	}
	os.Exit(code)
	return nil
}

// positional rend le chemin donne en argument, ou la chaine vide. La
// distinction compte : sans argument, on resout le corpus depuis le registre ;
// avec, on analyse le repertoire pointe tel quel.
func positional(args []string) string {
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		return args[0]
	}
	return ""
}

// resolveCorpus rend le corpus a analyser, en affichant immediatement toute
// note renvoyee par la resolution (ex. PERIMETER ignoree) — la logique de
// precedence elle-meme vit dans internal/resolution, deep et testable sans
// flag ni os.Exit.
func resolveCorpus(path, regPath string) (*corpus.Corpus, *perimeter.Registry, error) {
	c, reg, warnings, err := resolution.Resolve(path, regPath)
	for _, w := range warnings {
		fmt.Fprintln(os.Stderr, w+"\n") //nolint:errcheck // sortie terminal
	}
	return c, reg, err
}

// avisAmbiant calcule et affiche l'avis ambiant sur w juste apres qu'une
// sous-commande a resolu son registre/corpus, et rend son contenu pour les
// sorties JSON qui le supportent deja. reuse porte le report.Result deja
// calcule par l'appelant (cmdLint) pour eviter un second passage de lint ;
// nil partout ailleurs.
//
// Les echecs de chargement (lint ou index de fiabilite) sont convertis ici
// en avis "indisponible" — jamais remontes comme l'erreur de la
// sous-commande elle-meme : l'avis ambiant ne doit jamais devenir un point
// de panne pour le reste de l'outil.
func avisAmbiant(w io.Writer, reg *perimeter.Registry, c *corpus.Corpus, reuse *report.Result) []string {
	afficher := func(a advisory.Advisory) []string {
		if ligne := a.Ligne(); ligne != "" {
			fmt.Fprintln(w, ligne) //nolint:errcheck // sortie terminal
		}
		return a.Warnings()
	}

	// c est optionnel : « perctl gate » peut ne recevoir qu'un registre, sans
	// --corpus. Sans corpus, il n'y a rien a linter, et lintRes reste nil —
	// Compute sait deja s'en passer.
	var lintRes *report.Result
	var scope string
	if reuse != nil {
		lintRes = reuse
	} else if c != nil {
		var err error
		lintRes, err = lint.Run(c, lint.Options{})
		if err != nil {
			return afficher(advisory.Advisory{Unavailable: true, Cause: err.Error()})
		}
	}
	if c != nil {
		scope = c.Root
	}

	var entries []reliability.Entry
	var root string
	budget := 0
	if reg != nil {
		var err error
		entries, err = reliability.Load(reliability.IndexPath(reg.Path))
		if err != nil {
			return afficher(advisory.Advisory{Unavailable: true, Cause: err.Error()})
		}
		root = filepath.Dir(reg.Path)
		if c != nil {
			budget = c.Config.UnprobedBudget()
		}
	}

	return afficher(advisory.Compute(reg, lintRes, entries, root, budget, time.Now(), scope))
}

func target(args []string) string {
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		return args[0]
	}
	return "."
}

func isTTY() bool { return term.IsTerminal(int(os.Stdout.Fd())) }

func emit(res *report.Result, format string, verbose bool) error {
	return res.Write(os.Stdout, report.Rendu{Format: format, Color: format == "human" && isTTY(), Verbose: verbose})
}

func cmdLint(args []string) error {
	fs := flag.NewFlagSet("lint", flag.ExitOnError)
	format := fs.String("format", "human", "human | json | github")
	strict := fs.Bool("strict", false, "traite les avertissements comme des erreurs")
	disable := fs.String("disable", "", "regles a desactiver, separees par des virgules")
	verbose := fs.Bool("verbose", false, "detaille par note les controles replies en constat de processus")
	regPath := fs.String("perimeter", "", "registre a utiliser (defaut : recherche en remontant)")
	path := positional(args)
	_ = fs.Parse(trimPositional(args))

	c, reg, err := resolveCorpus(path, *regPath)
	if err != nil {
		return err
	}
	res, err := lint.Run(c, lint.Options{Disable: splitList(*disable)})
	if err != nil {
		return err
	}
	// lint a deja calcule ce dont l'avis ambiant a besoin : le lui passer
	// evite un second passage identique sur le meme corpus.
	avisAmbiant(os.Stderr, reg, c, res)
	if err := emit(res, *format, *verbose); err != nil {
		return err
	}
	return fail(res.ExitCode(*strict))
}

func cmdVerify(args []string) error {
	fs := flag.NewFlagSet("verify", flag.ExitOnError)
	format := fs.String("format", "human", "human | json | github")
	allow := fs.Bool("allow-exec", false, "autorise l'execution des commandes declarees (obligatoire)")
	write := fs.Bool("write", false, "inscrit verified_at et verify_status dans les notes")
	only := fs.String("only", "", "ne verifie que les notes dont le nom contient cette chaine")
	untrusted := fs.Bool("allow-exec-untrusted", false, "executer meme dans un contexte ou le contenu vient de l'exterieur")
	diffRef := fs.String("diff-probes", "", "lister les preuves qui ont change depuis cette reference git, sans rien executer")
	timeout := fs.Duration("timeout", 0, "delai par commande (defaut : celui du corpus)")
	regPath := fs.String("perimeter", "", "registre a utiliser (defaut : recherche en remontant)")
	probes := fs.Bool("probe-sources", false, "rejoue aussi la sonde de chaque source declaree")
	path := positional(args)
	_ = fs.Parse(trimPositional(args))

	c, reg, err := resolveCorpus(path, *regPath)
	if err != nil {
		return err
	}
	avisAmbiant(os.Stderr, reg, c, nil)

	if *diffRef != "" {
		return listerPreuvesModifiees(c, *diffRef)
	}

	// Sans registre, il n'y a aucune sonde a rejouer. L'accepter en silence
	// rendait le drapeau inoperant sans le dire.
	if *probes && reg == nil {
		return fmt.Errorf("--probe-sources demande un registre : l'indiquer avec --perimeter <registre>")
	}

	// Les preuves sont du shell declare dans des fichiers markdown. Les jouer
	// sur une contribution venue de l'exterieur revient a executer du code
	// arbitraire. La regle existait dans la documentation ; elle est ici.
	if *allow && !*untrusted {
		if v := ciguard.AssessDefault(); !v.Trusted {
			fmt.Fprintf(os.Stderr, "\n  contexte : %s\n  %s\n\n  Les preuves sont du code declare dans des fichiers markdown : les jouer\n  ici executerait du contenu venu de l'exterieur. Passer outre demande\n  --allow-exec-untrusted, et de savoir pourquoi.\n\n", v.Context, v.Reason) //nolint:errcheck // sortie terminal
			return fmt.Errorf("execution refusee dans ce contexte")
		}
	}

	res, outcomes, err := verify.Run(c, verify.Options{
		AllowExec: *allow, Write: *write, Only: *only, Timeout: *timeout,
		Registry: reg, ProbeSources: *probes,
	})
	if errors.Is(err, verify.ErrExecRefused) {
		n, _ := res.Stats["verifiable"].(int)
		if n == 0 && *probes {
			fmt.Fprintf(os.Stderr, "\n  Les sondes sont des commandes declarees dans %s : les\n  rejouer execute ce que ce fichier contient.\n\n", reg.Path) //nolint:errcheck // sortie terminal
		} else {
			fmt.Fprintf(os.Stderr, "\n  Ces commandes sont du code executable declare dans des fichiers\n  markdown : ne les lancer que sur un corpus dont on relit les\n  contributions (CODEOWNERS)\n\n") //nolint:errcheck // sortie terminal
		}
		// Le refus peut venir des seules sondes : un corpus sans preuve n'en
		// porte aucune, et annoncer « 0 notes portent des preuves » ferait
		// passer le refus pour une absence de travail.
		if n == 0 && *probes {
			return fmt.Errorf("%d sources declarent une sonde : %w", len(reg.Sources), err)
		}
		return fmt.Errorf("%d notes portent des preuves : %w", n, err)
	}
	if err != nil {
		return err
	}
	if *format == "json" {
		res.Stats["outcomes"] = outcomes
	}
	if err := emit(res, *format, false); err != nil {
		return err
	}
	if *format == "human" {
		v, _ := res.Stats["verifiable"].(int)
		cov, _ := res.Stats["coverage"].(float64)
		fmt.Printf("%d notes portent une preuve (%.0f%% du corpus)\n", v, cov*100)
		// Sans ce compte, un rejeu ou tout passe ne se distingue pas d'un
		// rejeu qui n'a pas eu lieu — c'est exactement ce qui rendait le
		// drapeau muet.
		if ok, joue := res.Stats["sources_ok"].(int); joue {
			ko, _ := res.Stats["sources_ko"].(int)
			fmt.Printf("%d sondes de source rejouees, %d en echec\n", ok+ko, ko)
			fmt.Println("  le detail par source : perctl perimeter " + reg.Path + " --probe --allow-exec")
		}
	}
	return fail(res.ExitCode(false))
}

func cmdIndex(args []string) error {
	fs := flag.NewFlagSet("index", flag.ExitOnError)
	fix := fs.Bool("fix", false, "ajoute a l'index les notes manquantes")
	sync := fs.Bool("sync", false, "regenere les accroches a partir des descriptions")
	regPath := fs.String("perimeter", "", "registre a utiliser (defaut : recherche en remontant)")
	path := positional(args)
	_ = fs.Parse(trimPositional(args))

	c, reg, err := resolveCorpus(path, *regPath)
	if err != nil {
		return err
	}
	avisAmbiant(os.Stderr, reg, c, nil)
	if !c.HasIndex {
		return fmt.Errorf("aucun index lisible en %s", filepath.Join(c.Root, c.Config.Corpus.Index))
	}
	cited := map[string]bool{}
	for _, t := range c.IndexTargets {
		cited[t] = true
	}
	var missing []*corpus.Note
	for _, n := range c.Notes {
		if !cited[n.Rel] {
			missing = append(missing, n)
		}
	}
	if *sync {
		if err := corpus.SyncIndexAutorise(c); err != nil {
			return err
		}
		return syncIndex(c)
	}
	if len(missing) == 0 {
		fmt.Printf("index complet : %d notes, %d entrees\n", len(c.Notes), len(c.IndexTargets))
		return nil
	}
	if !*fix {
		fmt.Printf("%d notes absentes de l'index :\n", len(missing))
		for _, n := range missing {
			fmt.Printf("  %s\n", n.Rel)
		}
		fmt.Println("\nrelancer avec --fix pour les ajouter (les intitules restent a relire)")
		return fail(1)
	}

	raw, err := os.ReadFile(c.IndexPath)
	if err != nil {
		return err
	}
	var b strings.Builder
	b.Write(raw)
	if !strings.HasSuffix(string(raw), "\n") {
		b.WriteString("\n")
	}
	for _, n := range missing {
		b.WriteString(n.IndexLine("") + "\n")
	}
	if err := os.WriteFile(c.IndexPath, []byte(b.String()), 0o644); err != nil {
		return err
	}
	suite := "a relire : le titre et l'accroche viennent du frontmatter"
	if c.IndexHookAuthore() {
		suite = "l'accroche generee n'est qu'un point de depart : ce corpus les ecrit a la main"
	}
	fmt.Printf("%d entrees ajoutees a %s — %s\n", len(missing), c.Config.Corpus.Index, suite)
	return nil
}

// syncIndex reecrit les accroches de l'index a partir des descriptions, en
// conservant les intitules, qui eux sont ecrits a la main.
func syncIndex(c *corpus.Corpus) error {
	raw, err := os.ReadFile(c.IndexPath)
	if err != nil {
		return err
	}
	byRel := c.ByRel()
	lines := strings.Split(string(raw), "\n")
	changed := 0
	for _, e := range c.IndexEntries {
		n, ok := byRel[e.Target]
		if !ok || n.ParseErr != nil {
			continue
		}
		want := n.IndexLine(e.Title)
		if lines[e.Line-1] == want {
			continue
		}
		lines[e.Line-1] = want
		changed++
	}
	if changed == 0 {
		fmt.Println("accroches deja alignees sur les descriptions")
		return nil
	}
	if err := os.WriteFile(c.IndexPath, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
		return err
	}
	fmt.Printf("%d accroches regenerees dans %s\n", changed, c.Config.Corpus.Index)
	return nil
}

// cmdPerimeter valide le registre : couverture des roles, sources
// referencees, adaptateurs connus, credentials en reference. C'est la
// condition d'entree du demarrage — tant qu'elle n'est pas tenue, le
// perimetre n'est pas decrit.
func cmdPerimeter(args []string) error {
	fs := flag.NewFlagSet("perimeter", flag.ExitOnError)
	probe := fs.Bool("probe", false, "rejoue la sonde de chaque source declaree (--allow-exec obligatoire)")
	allow := fs.Bool("allow-exec", false, "autorise l'execution des sondes declarees au registre")
	untrusted := fs.Bool("allow-exec-untrusted", false, "executer meme dans un contexte ou le contenu vient de l'exterieur")
	timeout := fs.Duration("timeout", 0, "delai par sonde (defaut : celui du registre)")
	// Le registre est ici le sujet de la commande, pas un modificateur : il se
	// donne en argument, et cette sous-commande n'a donc pas de --perimeter,
	// qui dirait deux fois la meme chose. L'aide le dit, plutot que de laisser
	// « flag provided but not defined » l'expliquer.
	fs.Usage = func() {
		aide := fmt.Sprintf("perctl perimeter [registre] [options]\n\n"+
			"Le chemin donne en argument EST le registre : pas de --perimeter ici, il\n"+
			"dirait la meme chose deux fois. Sans argument, le registre est cherche\n"+
			"comme pour lint et verify : %s, remontee d'arborescence, puis\n"+
			"emplacement utilisateur.\n\n", perimeter.EnvVar)
		fmt.Fprint(fs.Output(), aide) //nolint:errcheck // sortie terminal
		fs.PrintDefaults()
	}
	root := positional(args)
	_ = fs.Parse(trimPositional(args))
	// Un chemin positionnel dit *quel registre* — il prime, exactement comme
	// un chemin dit *quel corpus* pour lint (#54). Sans lui, la sous-commande
	// dont le metier est d'inspecter le registre emprunte l'ordre de
	// resolution des autres, au lieu d'ouvrir perimeter.yml la ou elle est
	// lancee.
	if root == "" {
		found, err := resolution.ResolveRegistryPath(".", resolution.DesignationArgument)
		if err != nil {
			return err
		}
		root = found
	}

	reg, err := perimeter.Load(root)
	if err != nil {
		return err
	}
	// Pas d'avis ambiant ici, deliberement : cette sous-commande affiche deja
	// les issues de Registry.Check() ci-dessous, c'est son sujet. Le repeter
	// en avis ambiant redirait la meme chose deux fois dans la meme sortie.

	// Le rejeu des sondes est la seule chose qui distingue un registre
	// coherent d'un registre joignable. Il vit ici parce que c'est le
	// registre qu'il eprouve, et qu'il ne demande aucun corpus.
	if *probe {
		if *allow && !*untrusted {
			if err := refuserContexteNonSur(); err != nil {
				return err
			}
		}
		code, err := sonderSources(reg, *allow, *timeout, os.Stdout)
		if err != nil {
			return err
		}
		return fail(code)
	}

	issues := reg.Check()

	fmt.Printf("%s — %d sources, %d roles sur %d pourvus\n",
		root, len(reg.Sources), len(Roles(reg)), len(perimeter.Roles))

	if len(issues) > 0 {
		fmt.Printf("\n%d constats :\n", len(issues))
		for _, i := range issues {
			fmt.Printf("  - %s\n", i)
		}
		return fail(1)
	}

	fmt.Println("\n✓ registre coherent : chaque role est pourvu, et chaque source declare une sonde")
	fmt.Println("  declarer une sonde n'est pas la passer — les rejouer :")
	fmt.Println("    perctl perimeter " + root + " --probe --allow-exec")

	// La coherence est binaire, la maturite ne l'est pas. Ces remarques
	// n'invalident rien : un registre qui en porte reste utilisable, mais on
	// en tirera moins. Les rendre bloquantes rendrait invalide tout registre
	// existant et ferait desactiver la verification en entier.
	if rs := reg.Maturite(); len(rs) > 0 {
		fmt.Printf("\n%d remarque(s) de maturite — le registre reste valide :\n", len(rs))
		for _, r := range rs {
			fmt.Printf("  ~ [%s] %s\n", r.Motif, r)
		}
	}
	return nil
}

// sonderSources rejoue les sondes du registre et rend le code de sortie qui
// en decoule. Il rend un code plutot que d'en sortir lui-meme, pour que le
// verdict soit eprouvable autrement qu'en lancant un processus.
func sonderSources(reg *perimeter.Registry, allowExec bool, timeout time.Duration, w io.Writer) (int, error) {
	// Les sondes sont du shell declare dans un fichier de configuration.
	// L'autorisation est donc exigee ici comme pour les preuves de notes, et
	// son absence est dite — se taire rendrait la commande inerte, ce qui est
	// precisement le defaut qu'on corrige.
	if !allowExec {
		fmt.Fprintf(os.Stderr, "\n  Les sondes sont des commandes declarees dans %s : les rejouer\n  execute ce que ce fichier contient.\n\n", reg.Path) //nolint:errcheck // sortie terminal
		return 0, fmt.Errorf("%d sources declarent une sonde : relancer avec --allow-exec pour les rejouer", len(reg.Sources))
	}
	rap := verify.Sonder(reg, timeout)
	if err := rap.Ecrire(w); err != nil {
		return 0, err
	}
	return rap.ExitCode(), nil
}

// refuserContexteNonSur applique la meme garde que verify : rejouer du shell
// declare dans un depot sur une contribution venue de l'exterieur revient a
// executer du code arbitraire.
func refuserContexteNonSur() error {
	v := ciguard.AssessDefault()
	if v.Trusted {
		return nil
	}
	fmt.Fprintf(os.Stderr, "\n  contexte : %s\n  %s\n\n  Les sondes sont du code declare dans un fichier de configuration : les\n  jouer ici executerait du contenu venu de l'exterieur. Passer outre demande\n  --allow-exec-untrusted, et de savoir pourquoi.\n\n", v.Context, v.Reason) //nolint:errcheck // sortie terminal
	return fmt.Errorf("execution refusee dans ce contexte")
}

// Roles rend les roles effectivement pourvus par une source existante.
func Roles(reg *perimeter.Registry) []string {
	var filled []string
	for _, role := range perimeter.Roles {
		b, ok := reg.RoleMap[role].First()
		if !ok {
			continue
		}
		if _, ok := reg.Sources[b.Source]; ok {
			filled = append(filled, role)
		}
	}
	return filled
}

// cmdGate derive le verdict d'une evaluation de readiness et consigne le
// passage. Le code de sortie porte le verdict : 0 produire, 1 instruire,
// 2 rendre la main — pour qu'une automatisation puisse s'y brancher.
func cmdGate(args []string) error {
	fs := flag.NewFlagSet("gate", flag.ExitOnError)
	regPath := fs.String("perimeter", "", "registre des sources, pour verifier la couverture des roles")
	corpusPath := fs.String("corpus", "", "corpus de memoire, pour verifier la fraicheur des faits mobilises")
	ledger := fs.String("ledger", readiness.LedgerFile, "journal des passages")
	noRecord := fs.Bool("no-record", false, "ne consigne pas ce passage")
	allow := fs.Bool("allow-exec", false, "rejoue les preuves de resolution des carences comblees")
	untrusted := fs.Bool("allow-exec-untrusted", false, "executer meme dans un contexte ou le contenu vient de l'exterieur")
	path := target(args)
	_ = fs.Parse(trimPositional(args))
	if path == "." {
		return fmt.Errorf("indiquer le fichier d'evaluation a passer")
	}

	a, err := readiness.Load(path)
	if err != nil {
		return err
	}
	if issues := a.Coherence(); len(issues) > 0 {
		fmt.Fprintln(os.Stderr, "evaluation incoherente :") //nolint:errcheck // sortie terminal
		for _, i := range issues {
			fmt.Fprintf(os.Stderr, "  - %s\n", i) //nolint:errcheck // sortie terminal
		}
		return fmt.Errorf("%d incoherence(s) : une carence omise rendrait le verdict faussement vert", len(issues))
	}

	pre, err := preconditions(*regPath, *corpusPath, a)
	if err != nil {
		return err
	}
	avisAmbiantGate(os.Stderr, *regPath, *corpusPath)

	if *allow {
		if !*untrusted {
			if v := ciguard.AssessDefault(); !v.Trusted {
				fmt.Fprintf(os.Stderr, "\n  contexte : %s\n  %s\n\n", v.Context, v.Reason) //nolint:errcheck // sortie terminal
				return fmt.Errorf("rejeu des preuves de resolution refuse dans ce contexte")
			}
		}
		if echecs := rejouerResolutions(a, *regPath); len(echecs) > 0 {
			for _, e := range echecs {
				pre = append(pre, readiness.Precondition{Code: "resolution-non-tenue", Message: e})
			}
		}
	}

	d := a.Derive(pre)

	fmt.Printf("%s\n\n  verdict : %s\n", d.Spec, strings.ToUpper(string(d.Verdict)))
	for _, r := range d.Reasons {
		fmt.Printf("  · %s\n", r)
	}
	if d.Escalation != "" {
		fmt.Printf("\n  escalade classee : %s\n", d.Escalation)
	}
	if d.ResolutionsNonProuvees > 0 && !*allow {
		fmt.Printf("\n  %d resolution(s) sans preuve rejouable — « perctl gate … --allow-exec » les verifierait\n", d.ResolutionsNonProuvees)
	}

	if !*noRecord {
		e := readiness.Entry{
			At: time.Now().UTC(), Spec: a.Spec, Verdict: d.Verdict,
			Escalation: d.Escalation, Assessment: filepath.Base(path),
			ResolutionsNonProuvees: d.ResolutionsNonProuvees,
		}
		if err := readiness.Append(*ledger, e); err != nil {
			return fmt.Errorf("journal : %w", err)
		}
		fmt.Printf("\n  consigne dans %s\n", *ledger)
	}

	switch d.Verdict {
	case readiness.Produire:
		return nil
	case readiness.Instruire:
		return fail(1)
	default:
		return fail(2)
	}
}

// avisAmbiantGate adapte l'avis ambiant au modele de resolution propre a
// « gate » : --perimeter et --corpus y sont optionnels et independants,
// sans la recherche ambiante des autres sous-commandes — preconditions()
// vient deja de les charger une fois avec succes, donc un second
// chargement ici n'a pas de raison d'echouer differemment ; s'il echoue
// quand meme, il se rapporte comme "indisponible" plutot qu'en silence.
func avisAmbiantGate(w io.Writer, regPath, corpusPath string) {
	var reg *perimeter.Registry
	var c *corpus.Corpus
	if regPath != "" {
		var err error
		if reg, err = perimeter.Load(regPath); err != nil {
			fmt.Fprintln(w, (advisory.Advisory{Unavailable: true, Cause: err.Error()}).Ligne()) //nolint:errcheck // sortie terminal
			return
		}
	}
	if corpusPath != "" {
		var err error
		if c, err = corpus.Load(corpusPath); err != nil {
			fmt.Fprintln(w, (advisory.Advisory{Unavailable: true, Cause: err.Error()}).Ligne()) //nolint:errcheck // sortie terminal
			return
		}
	}
	avisAmbiant(w, reg, c, nil)
}

// preconditions rassemble ce que l'outil sait verifier seul : la couverture
// du perimetre, et la fraicheur des faits sur lesquels l'evaluation s'appuie.
// S'appuyer sur un fait dont la preuve ne tient plus, c'est agir sur une
// premisse fausse — exactement ce que la porte existe pour empecher.
func preconditions(regPath, corpusPath string, a *readiness.Assessment) ([]readiness.Precondition, error) {
	var pre []readiness.Precondition

	if regPath != "" {
		reg, err := perimeter.Load(regPath)
		if err != nil {
			return nil, err
		}
		if issues := reg.Check(); len(issues) > 0 {
			pre = append(pre, readiness.Precondition{
				Code:    "perimetre",
				Message: fmt.Sprintf("%d constat(s) sur le registre : le perimetre n'est pas entierement decrit", len(issues)),
				Hint:    "perctl perimeter " + regPath,
			})
		}
	}

	if corpusPath == "" || len(a.Facts) == 0 {
		return pre, nil
	}
	c, err := corpus.Load(corpusPath)
	if err != nil {
		return nil, err
	}
	byName := c.ByName()
	now := time.Now()
	for _, name := range a.Facts {
		n, ok := byName[name]
		if !ok {
			pre = append(pre, readiness.Precondition{
				Code:    "fait-absent",
				Message: fmt.Sprintf("le fait %q n'existe pas dans le corpus", name),
			})
			continue
		}
		if n.Metadata.VerifyStatus == "fail" {
			pre = append(pre, readiness.Precondition{
				Code:    "preuve-en-echec",
				Message: fmt.Sprintf("le fait %q porte une preuve en echec", name),
				Hint:    "relire le fait avant de s'y appuyer",
			})
			continue
		}
		if due, ok := n.ReviewDue(c.Config.StalenessBudget(n)); ok && now.After(due) {
			pre = append(pre, readiness.Precondition{
				Code:    "fait-perime",
				Message: fmt.Sprintf("le fait %q est a relire depuis le %s", name, due.Format("2006-01-02")),
			})
		}
	}
	return pre, nil
}

// cmdReadiness rend l'etat de sortie de demarrage. Le regime n'est pas un
// reglage : il se deduit du journal, donc il ne peut pas pourrir.
// rejouerResolutions verifie que les carences declarees comblees le sont
// vraiment. Une resolution qui ne tient pas devient une precondition, donc
// elle empeche « produire » — au meme titre qu'un fait dont la preuve a lache.
func rejouerResolutions(a *readiness.Assessment, regPath string) []string {
	var reg *perimeter.Registry
	if regPath != "" {
		reg, _ = perimeter.Load(regPath)
	}
	var echecs []string
	for _, def := range a.Deficiencies {
		if !def.Resolved || !def.Prouvee() {
			continue
		}
		chk := *def.ResolvedProof
		if chk.ViaSource() {
			if reg == nil {
				echecs = append(echecs, fmt.Sprintf("carence %q : sa preuve renvoie a une source, mais aucun registre n'est charge", def.ID))
				continue
			}
			cmd, err := reg.Resolve(chk.Source, chk.Arg)
			if err != nil {
				echecs = append(echecs, fmt.Sprintf("carence %q : %v", def.ID, err))
				continue
			}
			resolu := corpus.Check{Cmd: cmd.Cmd, ExpectExit: cmd.ExpectExit, ExpectStdout: cmd.ExpectStdout}
			if chk.ExpectStdout != "" {
				resolu.ExpectStdout = chk.ExpectStdout
			}
			chk = resolu
		}
		if r := verify.RunCheck(chk, 30*time.Second); r.Status != "pass" {
			echecs = append(echecs, fmt.Sprintf("carence %q declaree resolue, mais sa preuve ne tient pas : %s", def.ID, r.Reason))
		}
	}
	return echecs
}

func cmdReadiness(args []string) error {
	fs := flag.NewFlagSet("readiness", flag.ExitOnError)
	ledger := fs.String("ledger", readiness.LedgerFile, "journal des passages")
	window := fs.Int("window", readiness.DefaultWindow, "nombre de passages consecutifs juges")
	_ = fs.Parse(trimPositional(args))

	entries, err := readiness.ReadLedger(*ledger)
	if err != nil {
		return err
	}
	m := readiness.Assess(entries, *window)

	phase := "demarrage"
	if !m.Bootstrap {
		phase = "regime etabli"
	}
	fmt.Printf("%s — %d passage(s) consignes\n\n", *ledger, len(entries))
	fmt.Printf("  phase  : %s\n", phase)
	fmt.Printf("  regime : %s\n", m.Mode)
	fmt.Printf("  %s\n", m.Reason)
	if m.NonProuvees > 0 {
		fmt.Printf("\n  ⚠ %d resolution(s) affirmee(s) sans preuve sur la fenetre\n", m.NonProuvees)
		fmt.Printf("  Une porte se contourne en reclassant une ambiguite d'intention en carence\n")
		fmt.Printf("  mesurable, puis en la declarant resolue. C'est le signal qui le montre.\n")
	}

	if len(entries) > 0 {
		fmt.Printf("\n  derniers passages :\n")
		from := len(entries) - m.Window
		if from < 0 {
			from = 0
		}
		for _, e := range entries[from:] {
			esc := ""
			if e.Escalation != "" {
				esc = " (" + e.Escalation + ")"
			}
			fmt.Printf("    %s  %-14s%s  %s\n", e.At.Format("2006-01-02"), e.Verdict, esc, e.Spec)
		}
	}
	return nil
}

// cmdInit ecrit le registre du perimetre. C'est le premier contact d'une
// equipe avec l'outil : il pose les six roles, propose une brique pour chacun,
// et ecrit des sondes qui fonctionnent — sinon « simple » reste un vœu.
// cmdPropose examine les notes sans preuve et propose la sonde qui les
// prouverait. Sans --write, il n'ecrit rien : c'est une proposition a relire,
// jamais une ecriture. Avec, la sonde est inseree en commentaire — une note ne
// devient jamais verifiable sans qu'un humain l'ait decommentee.
// listerPreuvesModifiees rend les notes dont le bloc de preuve a bouge depuis
// une reference. Rien n'est execute : c'est fait pour tourner sur une pull
// request, la ou l'execution est justement interdite.
func listerPreuvesModifiees(c *corpus.Corpus, ref string) error {
	racine, err := depotDe(c.Root)
	if err != nil {
		return err
	}
	lire := func(rel string) ([]byte, error) {
		chemin, err := filepath.Rel(racine, filepath.Join(c.Root, rel))
		if err != nil {
			return nil, err
		}
		cmd := exec.Command("git", "-C", racine, "show", ref+":"+chemin)
		out, err := cmd.Output()
		if err != nil {
			// git echoue aussi bien pour un fichier absent que pour une
			// reference inconnue : on distingue en interrogeant la reference.
			if verifierRef(racine, ref) != nil {
				return nil, fmt.Errorf("reference %q inconnue dans %s", ref, racine)
			}
			return nil, os.ErrNotExist
		}
		return out, nil
	}

	changes, err := probediff.Changed(c, lire)
	if err != nil {
		return err
	}
	if len(changes) == 0 {
		fmt.Printf("aucune preuve modifiee depuis %s\n", ref)
		return nil
	}
	fmt.Printf("%d note(s) dont les preuves ont change depuis %s :\n\n", len(changes), ref)
	total := 0
	for _, ch := range changes {
		total += ch.Count()
		fmt.Printf("  %s — %s\n", ch.Note, ch.Resume())
		for _, a := range ch.Ajoutees {
			fmt.Printf("    + %s\n", a)
		}
		for _, r := range ch.Retirees {
			fmt.Printf("    - %s\n", r)
		}
	}
	fmt.Printf("\n%d preuve(s) touchee(s) — c'est la seule portion du diff qui execute du code.\n", total)
	return nil
}

func depotDe(dir string) (string, error) {
	out, err := exec.Command("git", "-C", dir, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", fmt.Errorf("%s n'est pas dans un depot git : --diff-probes compare a une reference git", dir)
	}
	return strings.TrimSpace(string(out)), nil
}

func verifierRef(racine, ref string) error {
	return exec.Command("git", "-C", racine, "rev-parse", "--verify", "--quiet", ref+"^{commit}").Run()
}

func cmdPropose(args []string) error {
	fs := flag.NewFlagSet("propose", flag.ExitOnError)
	regPath := fs.String("perimeter", "", "registre a utiliser (defaut : recherche en remontant)")
	write := fs.Bool("write", false, "inserer les sondes proposees, en commentaire")
	only := fs.String("confidence", "", "ne garder qu'un niveau : registre | structurel")
	path := positional(args)
	_ = fs.Parse(trimPositional(args))

	c, reg, err := resolveCorpus(path, *regPath)
	if err != nil {
		return err
	}
	avisAmbiant(os.Stderr, reg, c, nil)
	// Une commande qui modifie des fichiers doit dire lesquels et ou. Sans
	// cela, un registre au chemin absolu fait ecrire ailleurs que la ou l'on
	// croit etre — constate, et corrige ici.
	fmt.Printf("corpus : %s\n", c.Root)
	if reg != nil {
		fmt.Printf("registre : %s\n", reg.Path)
	}
	fmt.Println()

	res := claim.Propose(c, reg)

	var kept []claim.Proposal
	for _, p := range res.Proposals {
		if *only == "" || string(p.Confidence) == *only {
			kept = append(kept, p)
		}
	}

	fmt.Printf("%d notes — %d portent deja une preuve, %d proposables, %d sans signal\n",
		res.Total, res.DejaPreuve, len(res.Proposals), len(res.SansSignal))
	fmt.Printf("couverture atteignable : %.0f %%\n", res.Coverage()*100)

	var courant claim.Confidence
	for _, p := range kept {
		if p.Confidence != courant {
			courant = p.Confidence
			fmt.Printf("\n· confiance %s\n", courant)
		}
		fmt.Printf("    %-46s %-10s %s\n", p.Note, p.Kind.Name, p.Why)
		if p.Cmd != "" {
			fmt.Printf("      %s\n", strings.ReplaceAll(p.Cmd, "\n  ", "\n      "))
			if p.Expect != "" {
				fmt.Printf("      %s\n", p.Expect)
			}
		}
		if p.Motif != "" {
			fmt.Printf("      ~ [%s] %s\n", p.Motif, p.Consequence)
		}
	}

	if !*write {
		if len(kept) > 0 {
			fmt.Printf("\nrien n'a ete ecrit — relancer avec --write pour inserer ces sondes en commentaire\n")
		}
		return nil
	}
	n, err := claim.Write(c, kept)
	if err != nil {
		return err
	}
	fmt.Printf("\n%d notes completees dans %s — les sondes sont en commentaire, a decommenter apres relecture\n", n, c.Root)
	return nil
}

func cmdInit(args []string) error {
	fs := flag.NewFlagSet("init", flag.ExitOnError)
	auto := fs.Bool("non-interactive", false, "ecrire un gabarit commente sans poser de questions")
	out := fs.String("o", perimeter.File, "fichier a ecrire")
	_ = fs.Parse(trimPositional(args))

	if _, err := os.Stat(*out); err == nil {
		return fmt.Errorf("%s existe deja — le supprimer ou choisir un autre fichier avec -o", *out)
	}

	var body string
	if *auto {
		body = perimeter.Template()
	} else {
		var err error
		if body, err = askRegistry(os.Stdin); err != nil {
			return err
		}
	}
	if err := os.WriteFile(*out, []byte(body), 0o644); err != nil {
		return err
	}
	fmt.Printf("\necrit : %s\n", *out)
	fmt.Printf("verifier avec : perctl perimeter %s\n", *out)
	return nil
}

// askRegistry mene le dialogue et rend le registre correspondant.
func askRegistry(in io.Reader) (string, error) {
	r := bufio.NewReader(in)
	ask := func(q, def string) string {
		fmt.Printf("  %s\n    [%s] ", q, def)
		line, err := r.ReadString('\n')
		if err != nil && strings.TrimSpace(line) == "" {
			return def
		}
		if v := strings.TrimSpace(line); v != "" {
			return v
		}
		return def
	}

	fmt.Println("Registre du perimetre — six roles a pourvoir.")
	fmt.Println("Entree pour accepter la proposition entre crochets.")
	fmt.Println()

	var roles, sources strings.Builder
	roles.WriteString("roles:\n")
	sources.WriteString("\nsources:\n")

	for _, h := range perimeter.RoleHints {
		fmt.Printf("· %s\n", h.Role)
		adapter := ask("Quel type de brique ? ("+strings.Join(perimeter.Adapters, ", ")+")", h.Adapter)
		endpoint := ask(h.Question, h.Endpoint)
		name := strings.ReplaceAll(strings.SplitN(h.Role, ".", 2)[1], "_", "-")
		fmt.Println()

		fmt.Fprintf(&roles, "  %-22s { source: %s }\n", h.Role+":", name)
		fmt.Fprintf(&sources, "  %s:\n    adapter: %s\n    endpoint: %q\n    reliability: %s\n",
			name, adapter, endpoint, perimeter.ReliabilityFor(adapter))
		sources.WriteString(perimeter.ProbeFor(adapter, endpoint))
		if h.Role == perimeter.CorpusRole {
			sources.WriteString(perimeter.CorpusPolicyBloc)
		}
	}
	return "# Registre du perimetre — https://github.com/UnPoilTefal/perimeter\n" +
		"# Une source est referencee et sondee, jamais recopiee.\n" +
		"version: 1\n\n" + roles.String() + sources.String(), nil
}

func trimPositional(args []string) []string {
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		return args[1:]
	}
	return args
}

func splitList(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}

// cmdDraft juge un brouillon de note contre le corpus, sans jamais l'ecrire.
// C'est le pendant outil du portillon d'ecriture : il ne repond pas aux trois
// questions du portillon — ce sont des jugements — mais il repond a « ce nom
// est-il pris », « est-ce que ca existe deja », « la note est-elle conforme ».
func cmdDraft(args []string) error {
	fs := flag.NewFlagSet("draft", flag.ExitOnError)
	corpusPath := fs.String("corpus", "", "corpus de reference (defaut : celui du registre)")
	regPath := fs.String("perimeter", "", "registre a utiliser (defaut : recherche en remontant)")
	voisins := fs.Int("voisins", 0, "nombre de notes proches rendues (defaut 3)")
	format := fs.String("format", "human", "human | json")
	chemin := positional(args)
	_ = fs.Parse(trimPositional(args))

	raw, rel, err := lireBrouillon(chemin)
	if err != nil {
		return err
	}
	n, err := corpus.ParseNote(rel, raw)
	if err != nil {
		return err
	}
	// Un brouillon vit dans un fichier temporaire, ou sur l'entree standard :
	// le chemin ou il se trouve ne dit rien du nom qu'il doit porter. C'est
	// le champ name qui fait foi — sinon name-match se declenche a tort sur
	// chaque brouillon, et la collision n'est pas detectable.
	if n.ParseErr == nil && n.Name != "" {
		n, _ = corpus.ParseNote(n.Name+".md", raw)
	}

	c, reg, err := resolveCorpus(*corpusPath, *regPath)
	if err != nil {
		return err
	}
	warnings := avisAmbiant(os.Stderr, reg, c, nil)

	v, err := draft.Check(c, n, reg, draft.Options{Voisins: *voisins})
	if err != nil {
		return err
	}

	if *format == "json" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		sortie := draftJSON(n, v)
		sortie.Warnings = warnings
		if err := enc.Encode(sortie); err != nil {
			return err
		}
	} else if err := ecrireVerdict(os.Stdout, c, n, v); err != nil {
		return err
	}
	if v.Bloquant() {
		return fail(1)
	}
	return nil
}

func lireBrouillon(chemin string) ([]byte, string, error) {
	if chemin == "" || chemin == "-" {
		annoncerLectureStdin(os.Stdin, os.Stderr)
		raw, err := io.ReadAll(os.Stdin)
		if err != nil {
			return nil, "", err
		}
		if len(bytes.TrimSpace(raw)) == 0 {
			return nil, "", fmt.Errorf("brouillon vide sur l'entree standard — passer un fichier, ou ecrire la note sur stdin")
		}
		return raw, "brouillon.md", nil
	}
	raw, err := os.ReadFile(chemin)
	if err != nil {
		return nil, "", err
	}
	return raw, filepath.Base(chemin), nil
}

// annoncerLectureStdin dit ce que la commande attend quand elle l'attend d'un
// clavier. Sans ce mot, « perctl draft » sans argument parait fige : il lit
// l'entree standard, sans rien afficher, et le premier reflexe est Ctrl-C.
//
// Rien n'est dit quand l'entree vient d'un pipe ou d'un fichier — c'est le
// chemin normal de la commande, et il n'y a la personne a prevenir. L'annonce
// part sur stderr : une sortie redirigee doit rester le seul verdict.
func annoncerLectureStdin(in *os.File, w io.Writer) {
	if !term.IsTerminal(int(in.Fd())) {
		return
	}
	fmt.Fprintln(w, "lecture du brouillon sur l'entree standard — Ctrl-D pour juger, Ctrl-C pour annuler") //nolint:errcheck // sortie terminal
}

// sortie retient la premiere erreur d'ecriture et se tait ensuite : un
// verdict s'ecrit d'un bloc, le decouper en controles d'erreur le rendrait
// illisible pour rien.
type sortie struct {
	w   io.Writer
	err error
}

func (s *sortie) f(format string, a ...any) {
	if s.err != nil {
		return
	}
	_, s.err = fmt.Fprintf(s.w, format, a...)
}

func ecrireVerdict(w io.Writer, c *corpus.Corpus, n *corpus.Note, v *draft.Verdict) error {
	o := &sortie{w: w}
	o.f("brouillon : %s\ncorpus    : %s (%d notes)\n\n", n.Rel, c.Root, len(c.Notes))
	if n.ParseErr == nil && n.Name == "" {
		o.f("! nom       le champ name est vide : impossible de dire ou ecrire la note\n\n")
	}

	if v.Collision != nil {
		o.f("x collision  %s porte deja ce nom\n", v.Collision.Rel)
		o.f("             « %s »\n", v.Collision.Desc)
		o.f("             completer cette note, ou choisir un autre nom\n\n")
	}

	for _, f := range v.Findings {
		marque := "!"
		if f.Severity == report.Error {
			marque = "x"
		}
		o.f("%s %-10s %s\n", marque, f.Rule, f.Message)
		if f.Hint != "" {
			o.f("             %s\n", f.Hint)
		}
	}
	if len(v.Findings) > 0 {
		o.f("\n")
	}

	if len(v.Voisins) > 0 {
		o.f("est-ce que ca existe deja ? %d note(s) proche(s) — a juger, ce n'est pas un verdict\n", len(v.Voisins))
		for _, vo := range v.Voisins {
			o.f("  %.2f  %s\n", vo.Score, vo.Note.Rel)
			o.f("        « %s »\n", vo.Note.Desc)
			o.f("        en commun : %s\n", strings.Join(vo.Termes, ", "))
		}
		o.f("\n")
	}

	if v.Verify != nil {
		o.f("preuve proposable (%s) : %s\n", v.Verify.Confidence, v.Verify.Cmd)
		if v.Verify.Why != "" {
			o.f("        %s\n", v.Verify.Why)
		}
		if v.Verify.Motif != "" {
			o.f("        ~ [%s] %s\n", v.Verify.Motif, v.Verify.Consequence)
		}
		o.f("\n")
	}

	if v.Bloquant() {
		o.f("-> ne pas ecrire en l'etat\n")
	} else {
		o.f("-> rien ne s'oppose a l'ecriture cote outil\n")
	}
	o.f("   restent les trois questions du portillon, qui ne se mecanisent pas :\n")
	o.f("   non re-derivable ? non ephemere ? comptera dans trois mois ?\n")
	return o.err
}

type draftSortie struct {
	Brouillon string           `json:"brouillon"`
	Bloquant  bool             `json:"bloquant"`
	Collision string           `json:"collision,omitempty"`
	Findings  []report.Finding `json:"findings"`
	Voisins   []draftVoisin    `json:"voisins"`
	Verify    *claim.Proposal  `json:"verify,omitempty"`
	Warnings  []string         `json:"warnings,omitempty"`
}

type draftVoisin struct {
	Note   string   `json:"note"`
	Score  float64  `json:"score"`
	Termes []string `json:"termes"`
}

func draftJSON(n *corpus.Note, v *draft.Verdict) draftSortie {
	out := draftSortie{Brouillon: n.Rel, Bloquant: v.Bloquant(), Findings: v.Findings, Verify: v.Verify}
	if out.Findings == nil {
		out.Findings = []report.Finding{}
	}
	if v.Collision != nil {
		out.Collision = v.Collision.Rel
	}
	out.Voisins = []draftVoisin{}
	for _, vo := range v.Voisins {
		out.Voisins = append(out.Voisins, draftVoisin{Note: vo.Note.Rel, Score: vo.Score, Termes: vo.Termes})
	}
	return out
}

// cmdHarvest amorce un corpus depuis les traces qu'une equipe possede deja.
// Il ne produit que des propositions a relire — jamais une ecriture.
func cmdHarvest(args []string) error {
	fs := flag.NewFlagSet("harvest", flag.ExitOnError)
	regPath := fs.String("perimeter", "", "registre a utiliser (defaut : recherche en remontant)")
	depuis := fs.String("depuis", "", "borne l'historique lu (revision ou date, au sens de git)")
	limite := fs.Int("limite", 0, "plafond de candidats rendus (defaut 25)")
	voisins := fs.Int("voisins", 0, "notes proches rendues par candidat (defaut 3)")
	allow := fs.Bool("allow-exec", false, "autoriser l'execution des commandes de lecture")
	format := fs.String("format", "human", "human | json")
	_ = fs.Parse(trimPositional(args))

	c, reg, err := resolveCorpus("", *regPath)
	if err != nil {
		return err
	}
	if reg == nil {
		return fmt.Errorf("harvest a besoin d'un registre : il ne lit que des sources declarees")
	}
	warnings := avisAmbiant(os.Stderr, reg, c, nil)

	res, err := harvest.Run(c, reg, harvest.Options{
		AllowExec: *allow, Depuis: *depuis, Limite: *limite, Voisins: *voisins,
	})
	if err != nil {
		return err
	}
	if *format == "json" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(harvestSortie{Result: res, Warnings: warnings})
	}
	return ecrireMoisson(os.Stdout, c, res)
}

// harvestSortie ajoute l'avis ambiant a la sortie JSON de harvest, sans
// toucher au paquet harvest lui-meme : ses champs sont promus par
// l'embedding, warnings s'ajoute a cote.
type harvestSortie struct {
	*harvest.Result
	Warnings []string `json:"warnings,omitempty"`
}

// cmdReliability route vers les sous-commandes de l'index de fiabilite.
func cmdReliability(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("sous-commande attendue : check")
	}
	switch args[0] {
	case "check":
		return cmdReliabilityCheck(args[1:])
	default:
		return fmt.Errorf("sous-commande inconnue : %s", args[0])
	}
}

// cmdReliabilityCheck rend le verdict de fiabilite d'une reference — le
// chemin de lecture qu'un agent consulte avant de reutiliser un fait.
func cmdReliabilityCheck(args []string) error {
	fs := flag.NewFlagSet("reliability check", flag.ExitOnError)
	regPath := fs.String("perimeter", "", "registre a utiliser (defaut : recherche en remontant)")
	ref := positional(args)
	_ = fs.Parse(trimPositional(args))
	if ref == "" {
		return fmt.Errorf("usage : perctl reliability check <ref>")
	}

	c, reg, err := resolveCorpus("", *regPath)
	if err != nil {
		return err
	}
	if reg == nil {
		return fmt.Errorf("aucun registre trouve — voir « perctl init »")
	}
	avisAmbiant(os.Stderr, reg, c, nil)

	entries, err := reliability.Load(reliability.IndexPath(reg.Path))
	if err != nil {
		return err
	}
	v := reliability.Check(entries, ref, filepath.Dir(reg.Path), c.Config.UnprobedBudget(), time.Now())

	fmt.Printf("%s : %s\n", ref, v.Status)
	fmt.Printf("  %s\n", v.Detail)
	if v.Stale || v.Drifted {
		return fail(1)
	}
	return nil
}

func ecrireMoisson(w io.Writer, c *corpus.Corpus, res *harvest.Result) error {
	o := &sortie{w: w}
	o.f("sources : %s — %d traces lues\ncorpus  : %s (%d notes)\n", strings.Join(res.Sources, ", "), res.Lus, c.Root, len(c.Notes))
	for _, ig := range res.Ignorees {
		o.f("! ignoree : %s — %s\n", ig.Source, ig.Raison)
	}
	o.f("\n")
	o.f("%d trace(s) retenue(s)", res.Retenus)
	if res.Retenus > len(res.Candidats) {
		o.f(", %d rendue(s) — relancer avec --limite pour le reste", len(res.Candidats))
	}
	o.f("\n\n")

	for _, cand := range res.Candidats {
		o.f("  [%s] %s  %s\n", cand.Qualifier, cand.Ref, cand.Titre)
		o.f("    signal : %s\n", cand.Signal)
		if len(cand.Voisins) > 0 {
			o.f("    deja dans le corpus ? a juger, ce n'est pas un verdict :\n")
			for _, v := range cand.Voisins {
				o.f("      %.2f  %s\n", v.Score, v.Note.Rel)
			}
		}
		o.f("\n")
	}
	o.f("tous les candidats sont en trust: proposed — rien n'a ete ecrit.\n")
	o.f("les trois questions du portillon restent a poser sur chacun :\n")
	o.f("non re-derivable ? non ephemere ? comptera dans trois mois ?\n")
	return o.err
}
