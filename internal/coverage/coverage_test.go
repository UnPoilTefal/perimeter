package coverage

import (
	"testing"

	"github.com/UnPoilTefal/perimeter/internal/audit"
	"github.com/UnPoilTefal/perimeter/internal/perimeter"
	"github.com/UnPoilTefal/perimeter/internal/report"
)

func TestComputeSainNEstPasBloquant(t *testing.T) {
	rep := Compute(nil, "", nil, nil)
	if rep.Bloquant() {
		t.Error("rien a signaler : Bloquant() doit etre faux")
	}
}

func TestComputeRoleNonPourvuEstBloquant(t *testing.T) {
	issues := []perimeter.Issue{{Role: "contrainte.memoire", Message: "role non pourvu"}}
	rep := Compute(issues, "", nil, nil)
	if !rep.Bloquant() {
		t.Error("un role non pourvu doit rendre Bloquant() vrai")
	}
}

func TestComputeErreurDeLintEstBloquante(t *testing.T) {
	lintRes := &report.Result{}
	lintRes.Add(report.Finding{Rule: "schema", Severity: report.Error, Message: "note invalide"})
	rep := Compute(nil, "", lintRes, nil)
	if !rep.Bloquant() {
		t.Error("une erreur de lint doit rendre Bloquant() vrai")
	}
}

func TestComputeCorpusExterneIntrouvableEstBloquant(t *testing.T) {
	ce := []CorpusExterne{{Declaration: perimeter.CorpusExterne{Chemin: "../nexiste-pas"}, Introuvable: true}}
	rep := Compute(nil, "", nil, ce)
	if !rep.Bloquant() {
		t.Error("un corpus externe introuvable doit rendre Bloquant() vrai")
	}
}

// #85 (revue de code) — un role est bien pourvu (Registry.Check() muet),
// mais CorpusSource() echoue quand meme (ex. deux sources qualifiees pour
// le meme role, cf. #72) : ce cas ne doit jamais retomber en silence sur
// « tout va bien », il doit rendre Bloquant() vrai comme un role manquant.
func TestComputeCorpusErreurEstBloquante(t *testing.T) {
	rep := Compute(nil, "corpus non univoque", nil, nil)
	if !rep.Bloquant() {
		t.Error("une erreur de resolution du corpus doit rendre Bloquant() vrai")
	}
}

// Un corpus externe dont le SCAN echoue (fichier illisible en cours de
// route) est distinct d'un chemin introuvable — les deux doivent bloquer,
// mais ne doivent pas se faire passer l'un pour l'autre dans le rapport.
func TestComputeCorpusExterneErreurDeScanEstBloquante(t *testing.T) {
	ce := []CorpusExterne{{Declaration: perimeter.CorpusExterne{Chemin: "../vault"}, Erreur: "fichier illisible"}}
	rep := Compute(nil, "", nil, ce)
	if !rep.Bloquant() {
		t.Error("une erreur de scan doit rendre Bloquant() vrai")
	}
}

// Decision actee en implementation (spec #85, question laissee ouverte) :
// des callouts a valider/invalides restent de l'information, jamais un
// motif d'echec a eux seuls — coherent avec l'avis ambiant ailleurs sur ce
// projet, qui ne bloque jamais la sous-commande reellement demandee.
func TestComputeCalloutsEnAttenteNeSontPasBloquants(t *testing.T) {
	ce := []CorpusExterne{{
		Declaration: perimeter.CorpusExterne{Chemin: "../vault", Type: "obsidian"},
		Scan: audit.ScanResult{
			Notes:   3,
			Comptes: map[audit.Etat]int{audit.AValider: 2, audit.Invalide: 1},
		},
	}}
	rep := Compute(nil, "", nil, ce)
	if rep.Bloquant() {
		t.Error("des callouts a valider/invalides ne doivent pas, a eux seuls, rendre Bloquant() vrai")
	}
}

func TestComputeLintErrorsRefleteLeDecompte(t *testing.T) {
	lintRes := &report.Result{}
	lintRes.Add(report.Finding{Rule: "schema", Severity: report.Error})
	lintRes.Add(report.Finding{Rule: "schema", Severity: report.Error})
	lintRes.Add(report.Finding{Rule: "staleness", Severity: report.Warn})
	rep := Compute(nil, "", lintRes, nil)
	if rep.LintErrors != 2 {
		t.Errorf("LintErrors = %d, attendu 2 (les avertissements ne comptent pas)", rep.LintErrors)
	}
}
