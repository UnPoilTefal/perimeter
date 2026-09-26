package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/UnPoilTefal/perimeter/internal/readiness"
)

// #72 — preconditions() chargeait le corpus a plat (corpus.Load), jamais
// selon la politique declaree au registre (corpus.LoadWith +
// ConfigFromPolicy, comme resolveCorpus le fait partout ailleurs). Consequence :
// un budget de relecture plus strict que le defaut, declare au registre,
// n'atteignait jamais « gate ».
func TestPreconditionsRespecteLeBudgetDeStalenessDuRegistre(t *testing.T) {
	dir := t.TempDir()
	regPath := filepath.Join(dir, "perimeter.yml")
	corpusDir := filepath.Join(dir, "memoire")
	if err := os.MkdirAll(corpusDir, 0o755); err != nil {
		t.Fatal(err)
	}

	ecrire(t, filepath.Join(corpusDir, "MEMORY.md"),
		"# Index\n\n- [Reference: un](reference-un.md) — le piege, formule pour l'humain\n")
	// Modifie il y a 25 jours (par rapport au 2026-09-26 de cet environnement) :
	// perime sous un budget de 5 jours declare au registre, pas sous le defaut
	// (180 jours) que preconditions() applique aujourd'hui a tort.
	ecrire(t, filepath.Join(corpusDir, "reference-un.md"),
		"---\nname: reference-un\ndescription: \"Un fait a surveiller\"\n"+
			"metadata:\n  type: reference\n  modified: 2026-09-01\n---\n\nUn fait.\n")
	ecrire(t, regPath, `version: 1
roles:
  intention.spec:       { source: m }
  intention.tickets:    { source: m }
  contrainte.decisions: { source: m }
  contrainte.memoire:   { source: m }
  etat.declare:         { source: m }
  etat.reel:            { source: m }
sources:
  m:
    adapter: files
    reliability: declared
    endpoint: "`+corpusDir+`"
    probe:
      cmd: "true"
    corpus:
      index: MEMORY.md
      index_hook: authored
      staleness: { unprobed_review_after_days: 5 }
`)

	a := &readiness.Assessment{Spec: "test", Facts: []string{"reference-un"}}
	pre, err := preconditions(regPath, corpusDir, a)
	if err != nil {
		t.Fatalf("preconditions: %v", err)
	}
	for _, p := range pre {
		if p.Code == "fait-perime" {
			return
		}
	}
	t.Errorf("attendu une precondition fait-perime (budget de 5 jours declare au registre), obtenu %+v", pre)
}
