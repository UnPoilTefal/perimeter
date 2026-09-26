package main

import (
	"flag"
	"fmt"
	"os"
	"sort"

	"github.com/UnPoilTefal/perimeter/internal/audit"
	"github.com/UnPoilTefal/perimeter/internal/corpus"
	"github.com/UnPoilTefal/perimeter/internal/coverage"
	"github.com/UnPoilTefal/perimeter/internal/lint"
	"github.com/UnPoilTefal/perimeter/internal/perimeter"
	"github.com/UnPoilTefal/perimeter/internal/report"
	"github.com/UnPoilTefal/perimeter/internal/resolution"
)

// cmdCoverage agrege en une seule vue l'etat de tout le perimetre declare :
// couverture des six roles, sante du corpus scheme, et fiabilite de chaque
// corpus externe (#85). Resout son registre par la meme recherche ambiante
// que lint/verify — pas le modele isole de gate/readiness, qui diverge pour
// une raison propre a leur metier.
func cmdCoverage(args []string) error {
	fs := flag.NewFlagSet("coverage", flag.ExitOnError)
	regFlag := fs.String("perimeter", "", "registre a utiliser (defaut : recherche en remontant)")
	_ = fs.Parse(trimPositional(args))

	regPath := *regFlag
	if regPath == "" {
		found, err := resolution.ResolveRegistryPath(".", resolution.DesignationDrapeau)
		if err != nil {
			return err
		}
		regPath = found
	}
	reg, err := perimeter.Load(regPath)
	if err != nil {
		return err
	}

	roleIssues := reg.Check()
	lintRes, corpusErreur := resoudreCorpusScheme(reg)

	var externes []coverage.CorpusExterne
	for _, ce := range reg.CorpusExternes {
		externes = append(externes, scannerCorpusExterne(reg, ce))
	}

	rep := coverage.Compute(roleIssues, corpusErreur, lintRes, externes)
	ecrireCoverage(os.Stdout, regPath, rep)

	if rep.Bloquant() {
		return fail(1)
	}
	return nil
}

// resoudreCorpusScheme tente de charger et de linter le corpus scheme du
// registre. Un role contrainte.memoire non pourvu est legitime — un
// registre peut se dedier aux seuls corpus externes (#85) — et laisse
// lintRes et corpusErreur vides tous les deux, silencieusement.
//
// Toute autre defaillance (source non univoque comme #72, chemin introuvable,
// chargement ou lint en echec) doit se voir : la confondre avec le cas
// legitime ferait passer un corpus reellement casse pour un perimetre qui
// n'en declare simplement pas.
func resoudreCorpusScheme(reg *perimeter.Registry) (lintRes *report.Result, corpusErreur string) {
	if _, ok := reg.RoleMap[perimeter.CorpusRole].First(); !ok {
		return nil, ""
	}
	_, root, policy, err := reg.CorpusSource()
	if err != nil {
		return nil, err.Error()
	}
	if st, errStat := os.Stat(root); errStat != nil || !st.IsDir() {
		return nil, fmt.Sprintf("corpus introuvable : %s", root)
	}
	c, err := corpus.LoadWith(root, corpus.ConfigFromPolicy(policy))
	if err != nil {
		return nil, err.Error()
	}
	res, err := lint.Run(c, lint.Options{})
	if err != nil {
		return nil, err.Error()
	}
	return res, ""
}

// scannerCorpusExterne rend l'etat d'un corpus externe declare. Introuvable
// (chemin absent) et Erreur (scan en echec sur un chemin qui existe) sont
// deux causes distinctes, jamais confondues.
func scannerCorpusExterne(reg *perimeter.Registry, ce perimeter.CorpusExterne) coverage.CorpusExterne {
	root := reg.ResoudreCorpusExterne(ce)
	entry := coverage.CorpusExterne{Declaration: ce}
	st, errStat := os.Stat(root)
	if errStat != nil {
		entry.Introuvable = true
		return entry
	}
	if !st.IsDir() {
		entry.Erreur = fmt.Sprintf("%s n'est pas un dossier", root)
		return entry
	}
	scan, err := audit.Scan(root)
	if err != nil {
		entry.Erreur = err.Error()
		return entry
	}
	entry.Scan = scan
	return entry
}

func ecrireCoverage(w *os.File, regPath string, rep coverage.Report) {
	fmt.Fprintf(w, "%s\n\n", regPath) //nolint:errcheck // sortie terminal

	if len(rep.RoleIssues) == 0 {
		fmt.Fprintln(w, "✓ six roles pourvus") //nolint:errcheck // sortie terminal
	} else {
		fmt.Fprintf(w, "%d constat(s) sur le registre :\n", len(rep.RoleIssues)) //nolint:errcheck // sortie terminal
		for _, i := range rep.RoleIssues {
			fmt.Fprintf(w, "  - %s\n", i) //nolint:errcheck // sortie terminal
		}
	}

	// Registry.Check() rapporte deja toute defaillance de contrainte.memoire
	// qu'il sait detecter (role non pourvu, source absente...) : ne pas
	// repeter la meme cause via CorpusErreur, qui n'existe que pour les cas
	// que Check() ne voit pas (ex. #72 : plusieurs sources qualifiees).
	corpusDejaSignale := false
	for _, i := range rep.RoleIssues {
		if i.Role == perimeter.CorpusRole {
			corpusDejaSignale = true
			break
		}
	}
	if rep.CorpusErreur != "" && !corpusDejaSignale {
		fmt.Fprintf(w, "x corpus : %s\n", rep.CorpusErreur) //nolint:errcheck // sortie terminal
	} else if rep.LintErrors > 0 {
		fmt.Fprintf(w, "%d erreur(s) de lint sur le corpus\n", rep.LintErrors) //nolint:errcheck // sortie terminal
	}

	for _, ce := range rep.CorpusExternes {
		fmt.Fprintf(w, "\ncorpus externe %s (%s)\n", ce.Declaration.Chemin, ce.Declaration.Type) //nolint:errcheck // sortie terminal
		switch {
		case ce.Introuvable:
			fmt.Fprintln(w, "  x introuvable sur le disque") //nolint:errcheck // sortie terminal
		case ce.Erreur != "":
			fmt.Fprintf(w, "  x erreur de lecture : %s\n", ce.Erreur) //nolint:errcheck // sortie terminal
		case ce.Scan.JamaisAudite:
			fmt.Fprintf(w, "  ! jamais audite (%d note(s))\n", ce.Scan.Notes) //nolint:errcheck // sortie terminal
		default:
			ecrireComptes(w, ce.Scan.Comptes)
		}
	}
}

// ecrireComptes affiche chaque etat rencontre, connu ou non. Un etat qui ne
// correspond a aucune des trois constantes reconnues (faute de frappe dans
// un callout ecrit a la main) doit se voir, jamais disparaitre en silence —
// c'est deja l'invariant tenu par audit.Scan, ecrireCoverage ne doit pas le
// defaire en filtrant sur une liste fermee.
func ecrireComptes(w *os.File, comptes map[audit.Etat]int) {
	connus := []audit.Etat{audit.AValider, audit.Confirme, audit.Invalide}
	vus := map[audit.Etat]bool{}
	for _, e := range connus {
		vus[e] = true
		if n := comptes[e]; n > 0 {
			fmt.Fprintf(w, "  %s: %d\n", e, n) //nolint:errcheck // sortie terminal
		}
	}
	var autres []string
	for e := range comptes {
		if !vus[e] {
			autres = append(autres, string(e))
		}
	}
	sort.Strings(autres)
	for _, e := range autres {
		fmt.Fprintf(w, "  %s (état non reconnu): %d\n", e, comptes[audit.Etat(e)]) //nolint:errcheck // sortie terminal
	}
}
