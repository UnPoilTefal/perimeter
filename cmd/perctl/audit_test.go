package main

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var auditNow = time.Date(2026, 9, 26, 0, 0, 0, 0, time.UTC)

func TestParseAuditRefAvecLigne(t *testing.T) {
	path, lineNo, hasLine := parseAuditRef("Homelab/Réseau.md#L42")
	if !hasLine || path != "Homelab/Réseau.md" || lineNo != 42 {
		t.Errorf("parseAuditRef() = %q, %d, %v — attendu %q, 42, true", path, lineNo, hasLine, "Homelab/Réseau.md")
	}
}

func TestParseAuditRefSansLigne(t *testing.T) {
	path, _, hasLine := parseAuditRef("Homelab/Réseau.md")
	if hasLine || path != "Homelab/Réseau.md" {
		t.Errorf("parseAuditRef() = %q, hasLine=%v — attendu hasLine=false", path, hasLine)
	}
}

func TestAuditRefDirectEcritLeCallout(t *testing.T) {
	path := filepath.Join(t.TempDir(), "note.md")
	if err := os.WriteFile(path, []byte("titre\noutil X retenu\nsuite\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := auditRefDirect(path, 2, "sélection humaine", auditNow); err != nil {
		t.Fatalf("auditRefDirect: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, attendu := range []string{"[!verification]-", "**date**: 2026-09-26", "**état**: à valider", `"outil X retenu"`} {
		if !strings.Contains(string(got), attendu) {
			t.Errorf("fichier ecrit ne contient pas %q :\n%s", attendu, got)
		}
	}
}

func TestAuditFichierInteractifUneLigne(t *testing.T) {
	path := filepath.Join(t.TempDir(), "note.md")
	if err := os.WriteFile(path, []byte("titre\noutil X retenu\nsuite\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	in := bufio.NewReader(strings.NewReader("2\nsélection humaine\n\n"))
	var out bytes.Buffer
	if err := auditFichierInteractif(in, &out, path, "", auditNow); err != nil {
		t.Fatalf("auditFichierInteractif: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "**origine**: sélection humaine") {
		t.Errorf("fichier ecrit ne contient pas l'origine saisie :\n%s", got)
	}
}

func TestAuditRefDirectCalloutDejaPresentNAbortePas(t *testing.T) {
	path := filepath.Join(t.TempDir(), "note.md")
	if err := os.WriteFile(path, []byte("titre\noutil X retenu\nsuite\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := auditRefDirect(path, 2, "premiere passe", auditNow); err != nil {
		t.Fatalf("auditRefDirect (premiere passe): %v", err)
	}
	avant, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if err := auditRefDirect(path, 2, "seconde passe", auditNow); err != nil {
		t.Fatalf("auditRefDirect (seconde passe, callout deja present) : erreur inattendue %v", err)
	}

	apres, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(avant) != string(apres) {
		t.Errorf("le fichier a change alors qu'un callout existait deja :\navant : %s\napres : %s", avant, apres)
	}
	if strings.Count(string(apres), "[!verification]-") != 1 {
		t.Errorf("un second callout a ete empile :\n%s", apres)
	}
}

func TestAuditFichierInteractifIgnoreLigneDejaMarqueeSansAbandonnerLesAutres(t *testing.T) {
	path := filepath.Join(t.TempDir(), "note.md")
	if err := os.WriteFile(path, []byte("l1\nl2\nl3\nl4\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Une premiere passe marque la ligne 2.
	in := bufio.NewReader(strings.NewReader("2\norigine-l2\n\n"))
	var out bytes.Buffer
	if err := auditFichierInteractif(in, &out, path, "", auditNow); err != nil {
		t.Fatalf("auditFichierInteractif (premiere passe): %v", err)
	}

	// La premiere passe a decale « l4 » : son numero de ligne se lit dans le
	// fichier tel qu'il est maintenant, pas dans la numerotation d'origine.
	apresPremierePasse, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	l4Ligne := 0
	for i, l := range strings.Split(string(apresPremierePasse), "\n") {
		if l == "l4" {
			l4Ligne = i + 1
		}
	}
	if l4Ligne == 0 {
		t.Fatalf("« l4 » introuvable apres la premiere passe :\n%s", apresPremierePasse)
	}

	// Seconde passe : re-designer la ligne 2 (deja marquee) et la ligne de
	// « l4 » dans la meme invocation — l4 doit quand meme s'ecrire, la
	// ligne 2 doit etre signalee et ignoree, pas planter le tout.
	in2 := bufio.NewReader(strings.NewReader(fmt.Sprintf("2\norigine-l2-bis\n%d\norigine-l4\n\n", l4Ligne)))
	out.Reset()
	if err := auditFichierInteractif(in2, &out, path, "", auditNow); err != nil {
		t.Fatalf("auditFichierInteractif (seconde passe): %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), `"l4"`) {
		t.Errorf("la ligne 4, non encore marquee, aurait du recevoir son callout :\n%s", got)
	}
	if strings.Count(string(got), "[!verification]-") != 2 {
		t.Errorf("attendu exactement 2 callouts (l2 de la premiere passe, l4 de la seconde) :\n%s", got)
	}
	if !strings.Contains(out.String(), "déjà") {
		t.Errorf("la sortie devrait signaler que la ligne 2 est deja marquee, obtenu : %s", out.String())
	}
}

func TestAuditFichierInteractifPlusieursLignesPreserveLesNumeros(t *testing.T) {
	// Deux lignes marquees dans la meme note : la premiere insertion (a la
	// ligne la plus basse) ne doit pas decaler le numero vise par la seconde.
	path := filepath.Join(t.TempDir(), "note.md")
	if err := os.WriteFile(path, []byte("l1\nl2\nl3\nl4\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	in := bufio.NewReader(strings.NewReader("2\norigine-l2\n4\norigine-l4\n\n"))
	var out bytes.Buffer
	if err := auditFichierInteractif(in, &out, path, "", auditNow); err != nil {
		t.Fatalf("auditFichierInteractif: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	s := string(got)
	i2 := strings.Index(s, `"l2"`)
	i4 := strings.Index(s, `"l4"`)
	if i2 < 0 || i4 < 0 {
		t.Fatalf("les deux citations attendues sont absentes :\n%s", s)
	}
	if i2 >= i4 {
		t.Errorf("citation « l2 » attendue avant « l4 » dans le fichier ecrit :\n%s", s)
	}
	if !strings.Contains(s, "origine-l2") || !strings.Contains(s, "origine-l4") {
		t.Errorf("les deux origines saisies sont attendues :\n%s", s)
	}
}
