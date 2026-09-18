package report

import (
	"strings"
	"testing"
)

// resultat rend un resultat de n constats pour une regle agregable declaree
// sur un perimetre de examinees unites.
func resultat(n, examinees int) *Result {
	r := &Result{Command: "lint", Notes: examinees}
	for i := 0; i < n; i++ {
		r.Add(Finding{Rule: "index-drift", Severity: Warn, File: "MEMORY.md", Message: "derive"})
	}
	r.DeclareAgregable("index-drift", Agregable{
		Constat: "l'index n'est pas synchronise", Fix: "perctl index --sync", Examinees: examinees,
	})
	return r
}

func humain(t *testing.T, r *Result, verbose bool) string {
	t.Helper()
	var b strings.Builder
	if err := r.Write(&b, Rendu{Format: "human", Verbose: verbose}); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

// S1 — le seuil, aux deux bords. 90 % replie, 89 % detaille. Le calibrage est
// mesure (voir SeuilAgregation) ; ce test empeche qu'il derive sans qu'on le
// decide.
func TestSeuilAgregationAuxDeuxBords(t *testing.T) {
	for _, cas := range []struct {
		n, examinees int
		replie       bool
	}{
		{100, 100, true}, // le cas de l'issue
		{90, 100, true},  // pile au seuil
		{89, 100, false}, // juste en dessous
		{105, 116, true}, // corpus mesure, onze accroches corrigees a la main
		{23, 116, false}, // le taux d'atomicity mesure : jamais replie
	} {
		sortie := humain(t, resultat(cas.n, cas.examinees), false)
		replie := strings.Contains(sortie, "l'index n'est pas synchronise")
		if replie != cas.replie {
			t.Errorf("%d/%d : repli attendu %v, obtenu %v\n%s", cas.n, cas.examinees, cas.replie, replie, sortie)
		}
	}
}

// S2 — le plancher. Une liste de neuf lignes ne noie rien, et 100 % d'une
// poignee n'est pas la preuve d'un processus.
func TestPlancherAgregation(t *testing.T) {
	if sortie := humain(t, resultat(9, 9), false); strings.Contains(sortie, "9/9") {
		t.Errorf("9 constats sur 9 : sous le plancher, le detail doit rester\n%s", sortie)
	}
	if sortie := humain(t, resultat(10, 10), false); !strings.Contains(sortie, "10/10") {
		t.Errorf("10 constats sur 10 : le repli doit s'appliquer\n%s", sortie)
	}
}

// S3 — « --verbose » rend le detail, sans rien changer d'autre.
func TestVerboseRendLeDetail(t *testing.T) {
	r := resultat(12, 12)
	replie := humain(t, r, false)
	detaille := humain(t, r, true)

	if strings.Count(detaille, "MEMORY.md") != 12 {
		t.Errorf("--verbose doit rendre les 12 lignes de detail\n%s", detaille)
	}
	if strings.Contains(detaille, "12/12 notes") {
		t.Errorf("--verbose ne rend pas le constat de processus\n%s", detaille)
	}
	// Le repli est un rendu : il ne retire aucun constat.
	for _, s := range []string{replie, detaille} {
		if !strings.Contains(s, "12 avertissements") {
			t.Errorf("les comptes doivent rester ceux des constats\n%s", s)
		}
	}
}

// S4 — le repli n'a aucun effet sur les severites ni sur le code de sortie :
// un corpus qui echouait en --strict continue d'echouer.
func TestRepliNeChangePasLeCodeDeSortie(t *testing.T) {
	r := resultat(12, 12)
	if r.Count(Warn) != 12 {
		t.Errorf("12 avertissements attendus, %d comptes", r.Count(Warn))
	}
	if r.ExitCode(true) != 1 {
		t.Error("--strict doit continuer d'echouer sur un controle replie")
	}
}

// S5 — un controle qui n'a rien declare n'est jamais replie, quel que soit son
// taux. C'est la reponse au « le taux seul ne suffit pas » : la remediation
// unique est la condition, le taux n'est que le declencheur.
func TestSansDeclarationAucunRepli(t *testing.T) {
	r := &Result{Command: "lint", Notes: 50}
	for i := 0; i < 50; i++ {
		r.Add(Finding{Rule: "atomicity", Severity: Warn, File: "n.md", Message: "900 mots (budget 400)"})
	}
	if p := r.Processus(); len(p) != 0 {
		t.Errorf("aucune remediation unique declaree : aucun repli attendu, obtenu %v", p)
	}
	if n := strings.Count(humain(t, r, false), "900 mots"); n != 50 {
		t.Errorf("les 50 lignes de detail doivent rester, %d rendues", n)
	}
}

// S6 — les formats machine. JSON garde tout le detail et ajoute le constat de
// processus ; les annotations GitHub replient, parce qu'une annotation par
// note pour un seul fait est le bruit de l'issue en pleine relecture de PR.
func TestFormatsMachine(t *testing.T) {
	r := resultat(12, 12)

	var js strings.Builder
	if err := r.Write(&js, Rendu{Format: "json"}); err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(js.String(), `"rule": "index-drift"`); n != 13 {
		t.Errorf("json : 12 constats + 1 processus attendus, %d occurrences", n)
	}
	if !strings.Contains(js.String(), `"processus"`) {
		t.Errorf("json : le constat de processus doit etre expose\n%s", js.String())
	}

	var gh strings.Builder
	if err := r.Write(&gh, Rendu{Format: "github"}); err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(gh.String(), "::warning"); n != 1 {
		t.Errorf("github : une seule annotation attendue, %d rendues\n%s", n, gh.String())
	}
	if !strings.Contains(gh.String(), "perctl index --sync") {
		t.Errorf("github : l'annotation doit porter la remediation\n%s", gh.String())
	}
}
