// Package coverage agrege, en une seule vue, l'etat de tout le perimetre
// d'un utilisateur : la couverture des six roles du registre (deja rendue
// par internal/perimeter.Registry.Check), la sante du corpus scheme (deja
// rendue par internal/lint), et la fiabilite de chaque corpus externe
// declare (deja rendue par internal/audit.Scan). Aucune de ces categories
// n'est une nouvelle detection — ce paquet ne fait que les compter et les
// agreger en un verdict, sur le meme modele qu'internal/advisory.Compute.
//
// Compute est une fonction pure, sans lecture disque : l'appelant charge le
// registre, le resultat de lint, et le scan de chaque corpus externe
// declare.
package coverage

import (
	"github.com/UnPoilTefal/perimeter/internal/audit"
	"github.com/UnPoilTefal/perimeter/internal/perimeter"
	"github.com/UnPoilTefal/perimeter/internal/report"
)

// CorpusExterne est le scan d'un corpus externe declare, deja charge par
// l'appelant. Introuvable et Erreur distinguent deux causes bien
// differentes : un chemin absent sur le disque n'est pas un fichier illisible
// en cours de scan — les deux bloquent, mais un rapport qui les confondrait
// orienterait l'utilisateur vers le mauvais correctif.
type CorpusExterne struct {
	Declaration perimeter.CorpusExterne
	Scan        audit.ScanResult
	Introuvable bool
	Erreur      string
}

// Report est la vue agregee de couverture rendue par Compute.
type Report struct {
	RoleIssues []perimeter.Issue
	// CorpusErreur porte l'echec de resolution du corpus scheme quand un
	// role est pourvu mais que sa source ne l'est pas univoquement (ex.
	// #72 : deux sources qualifiees pour le meme role) — distinct du cas
	// legitime ou aucun role n'est pourvu du tout, qui laisse CorpusErreur
	// vide et LintErrors a zero sans rien signaler.
	CorpusErreur   string
	LintErrors     int
	CorpusExternes []CorpusExterne
}

// Bloquant dit si le rapport porte au moins un probleme qui doit faire
// echouer « perctl coverage » : un role non pourvu ou non sondable, une
// erreur de resolution ou de lint sur le corpus scheme, ou un corpus
// externe declare mais introuvable ou en echec de scan.
//
// Les callouts a valider/invalides d'un corpus externe restent de
// l'information, jamais un motif d'echec a eux seuls (decision prise en
// implementation, spec #85) : c'est la meme discipline que l'avis ambiant
// ailleurs sur ce projet, qui ne bloque jamais la sous-commande reellement
// demandee.
func (r Report) Bloquant() bool {
	if len(r.RoleIssues) > 0 || r.CorpusErreur != "" || r.LintErrors > 0 {
		return true
	}
	for _, ce := range r.CorpusExternes {
		if ce.Introuvable || ce.Erreur != "" {
			return true
		}
	}
	return false
}

// Compute agrege les detecteurs deja existants en un rapport de couverture.
func Compute(roleIssues []perimeter.Issue, corpusErreur string, lintRes *report.Result, corpusExternes []CorpusExterne) Report {
	rep := Report{RoleIssues: roleIssues, CorpusErreur: corpusErreur, CorpusExternes: corpusExternes}
	if lintRes != nil {
		rep.LintErrors = lintRes.Count(report.Error)
	}
	return rep
}
