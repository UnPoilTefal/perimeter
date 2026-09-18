package verify

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/UnPoilTefal/perimeter/internal/corpus"
	"github.com/UnPoilTefal/perimeter/internal/perimeter"
)

func load(t *testing.T, root string) *corpus.Corpus {
	t.Helper()
	c, err := corpus.Load(root)
	if err != nil {
		t.Fatalf("chargement : %v", err)
	}
	return c
}

// copyCorpus travaille sur une copie : verify --write modifie les fichiers.
func copyCorpus(t *testing.T) string {
	t.Helper()
	dst := t.TempDir()
	entries, err := os.ReadDir("testdata/corpus")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		raw, err := os.ReadFile(filepath.Join("testdata/corpus", e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dst, e.Name()), raw, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dst
}

// L'execution est le comportement le plus dangereux de l'outil : elle lance
// du shell declare dans des fichiers markdown. Elle doit etre refusee tant
// qu'elle n'est pas demandee explicitement.
func TestExecutionRefuseeSansAutorisation(t *testing.T) {
	c := load(t, "testdata/corpus")
	_, _, err := Run(c, Options{AllowExec: false})
	if !errors.Is(err, ErrExecRefused) {
		t.Fatalf("execution attendue refusee, obtenu : %v", err)
	}
}

func TestPreuveVraiePasse(t *testing.T) {
	c := load(t, "testdata/corpus")
	_, outcomes, err := Run(c, Options{AllowExec: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, o := range outcomes {
		if o.Note == "reference-preuve-vraie.md" && o.Status != "pass" {
			t.Errorf("la preuve devait passer, statut : %s (%+v)", o.Status, o.Checks)
		}
	}
}

func TestPreuveFausseEchoue(t *testing.T) {
	c := load(t, "testdata/corpus")
	res, outcomes, err := Run(c, Options{AllowExec: true})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, o := range outcomes {
		if o.Note == "reference-preuve-fausse.md" {
			found = true
			if o.Status == "pass" {
				t.Error("la preuve ne tient plus, elle ne devait pas passer")
			}
		}
	}
	if !found {
		t.Fatal("la note temoin n'a pas ete verifiee")
	}
	if res.ExitCode(false) != 1 {
		t.Error("une preuve en echec doit faire sortir en 1")
	}
}

func TestCouvertureEstMesuree(t *testing.T) {
	c := load(t, "testdata/corpus")
	res, _, err := Run(c, Options{AllowExec: true})
	if err != nil {
		t.Fatal(err)
	}
	if n := res.Stats["verifiable"].(int); n != 2 {
		t.Errorf("2 notes portent une preuve, compte : %d", n)
	}
	// 2 notes verifiables sur 3 notes lisibles.
	if cov := res.Stats["coverage"].(float64); cov < 0.66 || cov > 0.67 {
		t.Errorf("couverture attendue ~0.667, obtenue %.3f", cov)
	}
}

func TestTimeoutEstApplique(t *testing.T) {
	dir := t.TempDir()
	note := `---
name: reference-lente
description: "Une note dont la commande de verification ne rend jamais la main"
metadata:
  type: reference
  verify:
    - cmd: "sh -c 'sleep 30' & wait"
---

Corps.
`
	if err := os.WriteFile(filepath.Join(dir, "reference-lente.md"), []byte(note), 0o644); err != nil {
		t.Fatal(err)
	}
	c := load(t, dir)
	start := time.Now()
	_, outcomes, err := Run(c, Options{AllowExec: true, Timeout: 300 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Errorf("le delai n'a pas ete applique : %s", elapsed)
	}
	if outcomes[0].Status != "error" {
		t.Errorf("un depassement de delai doit donner error, obtenu %s", outcomes[0].Status)
	}
}

// --write doit inscrire le resultat sans reordonner ni perdre le reste du
// frontmatter : une note ecrite a la main ne doit pas etre remaniee.
func TestWriteInscritLeResultatEnPreservantLeFrontmatter(t *testing.T) {
	dir := copyCorpus(t)
	c := load(t, dir)
	now := time.Date(2026, 9, 13, 8, 0, 0, 0, time.UTC)
	if _, _, err := Run(c, Options{AllowExec: true, Write: true, Now: now}); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(filepath.Join(dir, "reference-preuve-vraie.md"))
	if err != nil {
		t.Fatal(err)
	}
	got := string(raw)
	for _, want := range []string{
		"verified_at: \"2026-09-13T08:00:00Z\"",
		"verify_status: pass",
		"name: reference-preuve-vraie",
		"la version d'image declaree est bien celle en service",
		"Corps.",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("le fichier reecrit ne contient pas %q\n---\n%s", want, got)
		}
	}
	// L'ordre d'origine doit tenir : name avant description avant metadata.
	if strings.Index(got, "name:") > strings.Index(got, "description:") {
		t.Error("l'ordre des cles du frontmatter n'a pas ete preserve")
	}

	// La note relue doit rester valide et porter le nouveau statut.
	c2 := load(t, dir)
	for _, n := range c2.Notes {
		if n.Name == "reference-preuve-vraie" {
			if n.ParseErr != nil {
				t.Fatalf("la note reecrite n'est plus analysable : %v", n.ParseErr)
			}
			if n.Metadata.VerifyStatus != "pass" {
				t.Errorf("verify_status relu : %q", n.Metadata.VerifyStatus)
			}
		}
	}
}

func TestOnlyFiltreLesNotes(t *testing.T) {
	c := load(t, "testdata/corpus")
	res, outcomes, err := Run(c, Options{AllowExec: true, Only: "vraie"})
	if err != nil {
		t.Fatal(err)
	}
	if len(outcomes) != 1 || outcomes[0].Note != "reference-preuve-vraie.md" {
		t.Errorf("--only n'a pas filtre : %+v", outcomes)
	}
	if res.ExitCode(false) != 0 {
		t.Error("seule la note qui passe etait selectionnee")
	}
}

// --- preuves adossees a une source du registre ---

const registreTest = `
version: 1
roles:
  intention.spec:        { source: specs }
  intention.tickets:     { source: tickets }
  contrainte.decisions:  { source: adr }
  contrainte.memoire:    { source: memoire }
  etat.declare:          { source: depots }
  etat.reel:             { source: cluster }
sources:
  specs:    { adapter: files,  reliability: measured, probe: { cmd: "true" } }
  tickets:  { adapter: github, reliability: measured, endpoint: "org/produit",
              probe: { cmd: "echo org/produit" },
              query: { cmd: "echo etat-${arg}" } }
  adr:      { adapter: git,    reliability: declared, probe: { cmd: "true" } }
  memoire:  { adapter: files,  reliability: declared, probe: { cmd: "true" } }
  depots:   { adapter: git,    reliability: declared, probe: { cmd: "true" } }
  cluster:  { adapter: http,   reliability: measured, probe: { cmd: "false" } }
`

const noteViaSource = `---
name: reference-via-source
description: "Un fait dont la preuve passe par une source declaree plutot que par une commande ecrite en dur"
metadata:
  type: reference
  modified: 2026-09-13
  verify:
    - source: tickets
      arg: "ouvert"
      expect_stdout: "^etat-ouvert$"
---

Corps.
`

// corpusAvec depose un corpus minimal et rend sa racine.
func corpusAvec(t *testing.T, notes map[string]string) string {
	t.Helper()
	// Aucun fichier de configuration depose ici : depuis la v0.3.0 la
	// politique du corpus vit dans le registre, et un .corpus.yml n'est plus
	// lu. Ce helper en ecrivait encore un — le test passait donc pour une
	// autre raison que celle qu'il croyait poser.
	dir := t.TempDir()
	for name, body := range notes {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func registre(t *testing.T) *perimeter.Registry {
	t.Helper()
	return registreDepuis(t, registreTest)
}

// Test d'acceptation 3 — une preuve peut renvoyer a une source declaree,
// l'interrogation etant resolue par le registre.
func TestPreuveAdosseeAUneSourcePasse(t *testing.T) {
	c := load(t, corpusAvec(t, map[string]string{"reference-via-source.md": noteViaSource}))
	res, outcomes, err := Run(c, Options{AllowExec: true, Registry: registre(t)})
	if err != nil {
		t.Fatal(err)
	}
	if len(outcomes) != 1 || outcomes[0].Status != "pass" {
		t.Fatalf("la preuve devait passer : %+v", outcomes)
	}
	if n := res.Stats["checks_via_source"].(int); n != 1 {
		t.Errorf("1 preuve adossee a une source attendue, compte : %d", n)
	}
}

// Test d'acceptation 7 — sans registre, une preuve adossee a une source
// echoue nommement. La taire donnerait un verdict faussement vert.
func TestPreuveAdosseeSansRegistreEchoueNommement(t *testing.T) {
	c := load(t, corpusAvec(t, map[string]string{"reference-via-source.md": noteViaSource}))
	_, outcomes, err := Run(c, Options{AllowExec: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(outcomes) != 1 || outcomes[0].Status == "pass" {
		t.Fatalf("sans registre la preuve ne doit pas passer : %+v", outcomes)
	}
	if r := outcomes[0].Checks[0].Reason; !strings.Contains(r, "perimeter") {
		t.Errorf("la raison doit indiquer le registre manquant, obtenue : %q", r)
	}
}

func TestSourceInconnueDansUnePreuveEstNommee(t *testing.T) {
	note := strings.Replace(noteViaSource, "source: tickets", "source: fantome", 1)
	c := load(t, corpusAvec(t, map[string]string{"reference-via-source.md": note}))
	_, outcomes, err := Run(c, Options{AllowExec: true, Registry: registre(t)})
	if err != nil {
		t.Fatal(err)
	}
	if r := outcomes[0].Checks[0].Reason; !strings.Contains(r, "fantome") {
		t.Errorf("la source inconnue doit etre nommee, obtenue : %q", r)
	}
}

// Test d'acceptation 2 — la sonde d'une source est rejouee au meme titre
// qu'un fait : une source injoignable est un ecart rapporte.
func TestSondeDeSourceEnEchecEstRapportee(t *testing.T) {
	c := load(t, corpusAvec(t, map[string]string{"reference-via-source.md": noteViaSource}))
	res, _, err := Run(c, Options{AllowExec: true, Registry: registre(t), ProbeSources: true})
	if err != nil {
		t.Fatal(err)
	}
	// La source "cluster" sonde avec "false" : elle doit ressortir.
	var vu bool
	for _, f := range res.Findings {
		if f.Rule == "source" && strings.Contains(f.Message, "cluster") {
			vu = true
		}
	}
	if !vu {
		t.Errorf("la sonde en echec devait etre rapportee, constats : %+v", res.Findings)
	}
	if ko := res.Stats["sources_ko"].(int); ko != 1 {
		t.Errorf("1 source en echec attendue, compte : %d", ko)
	}
	if ok := res.Stats["sources_ok"].(int); ok != 5 {
		t.Errorf("5 sources saines attendues, compte : %d", ok)
	}
}

// Test d'acceptation 7 — un corpus sans registre garde exactement son
// comportement : la rework est additive, pas destructive.
func TestCorpusExistantInchangeSansRegistre(t *testing.T) {
	c := load(t, "testdata/corpus")
	res, outcomes, err := Run(c, Options{AllowExec: true})
	if err != nil {
		t.Fatal(err)
	}
	if n := res.Stats["verifiable"].(int); n != 2 {
		t.Errorf("2 notes verifiables attendues comme avant, compte : %d", n)
	}
	if n := res.Stats["checks_via_source"].(int); n != 0 {
		t.Errorf("aucune preuve adossee a une source dans ce corpus, compte : %d", n)
	}
	if len(outcomes) != 2 {
		t.Errorf("2 resultats attendus, obtenu %d", len(outcomes))
	}
}
