package main

import (
	"os"
	"testing"
)

// ecrire est un helper partage par plusieurs fichiers de test de ce package.
// La resolution registre/corpus elle-meme est testee dans
// internal/resolution ; les tests d'integration CLI restants (par ex.
// perimetre_resolution_test.go) continuent d'utiliser ce helper pour poser
// leurs fixtures.
func ecrire(t *testing.T, p, contenu string) {
	t.Helper()
	if err := os.WriteFile(p, []byte(contenu), 0o644); err != nil {
		t.Fatal(err)
	}
}
