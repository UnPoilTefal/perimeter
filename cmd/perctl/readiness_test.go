package main

import (
	"path/filepath"
	"strings"
	"testing"
)

// #75 — « readiness » a le meme modele de resolution isole que « gate »
// (--perimeter/--corpus optionnels et independants, sans recherche
// ambiante) : l'avis ambiant doit s'y accrocher aussi.
func TestReadinessAccrocheLAvisAmbiant(t *testing.T) {
	dir := t.TempDir()
	regPath := filepath.Join(dir, "perimeter.yml")
	// Un seul role pourvu sur six : Registry.Check() rapporte les cinq
	// manquants, exactement le fixture deja eprouve par advisory_test.go.
	ecrire(t, regPath, "version: 1\nroles:\n  intention.spec: { source: s }\nsources:\n  s:\n    adapter: files\n    reliability: declared\n    probe: { cmd: \"true\" }\n")

	out, err := stderrDe(t, func() error {
		return cmdReadiness([]string{"--perimeter", regPath, "--ledger", filepath.Join(dir, "ledger.jsonl")})
	})
	if err != nil {
		t.Fatalf("readiness: %v", err)
	}
	if !strings.Contains(out, "avis ambiant") || !strings.Contains(out, "probleme(s) de registre") {
		t.Errorf("readiness avec un registre incomplet doit produire un avis ambiant, stderr : %q", out)
	}
}

// Sans --perimeter, aucun avis ambiant ne doit apparaitre — le mode le plus
// courant (readiness sans argument) ne doit rien changer d'observable.
func TestReadinessSansPerimeterResteSilencieux(t *testing.T) {
	dir := t.TempDir()
	out, err := stderrDe(t, func() error {
		return cmdReadiness([]string{"--ledger", filepath.Join(dir, "ledger.jsonl")})
	})
	if err != nil {
		t.Fatalf("readiness: %v", err)
	}
	if strings.Contains(out, "avis ambiant") {
		t.Errorf("sans --perimeter, aucun avis ambiant n'est attendu, stderr : %q", out)
	}
}
