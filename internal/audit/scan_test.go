package audit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func ecrireFichier(t *testing.T, chemin, contenu string) {
	t.Helper()
	if err := os.WriteFile(chemin, []byte(contenu), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestScanCompteLesCalloutsParEtat(t *testing.T) {
	dir := t.TempDir()

	c1 := Callout{Date: "2026-09-01", Origine: "x", Etat: AValider, Citation: "fait 1"}
	c2 := Callout{Date: "2026-09-02", Origine: "y", Etat: Confirme, Citation: "fait 2"}
	c3 := Callout{Date: "2026-09-03", Origine: "z", Etat: AValider, Citation: "fait 3"}

	ecrireFichier(t, filepath.Join(dir, "un.md"), "titre\nfait 1\n"+Render(c1)+"suite\nfait 2\n"+Render(c2))
	ecrireFichier(t, filepath.Join(dir, "deux.md"), "titre\nfait 3\n"+Render(c3))

	res, err := Scan(dir)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if res.JamaisAudite {
		t.Error("des callouts existent, JamaisAudite doit etre faux")
	}
	if res.Notes != 2 {
		t.Errorf("Notes = %d, attendu 2", res.Notes)
	}
	if res.Comptes[AValider] != 2 {
		t.Errorf("Comptes[AValider] = %d, attendu 2", res.Comptes[AValider])
	}
	if res.Comptes[Confirme] != 1 {
		t.Errorf("Comptes[Confirme] = %d, attendu 1", res.Comptes[Confirme])
	}
	if res.Comptes[Invalide] != 0 {
		t.Errorf("Comptes[Invalide] = %d, attendu 0", res.Comptes[Invalide])
	}
}

func TestScanJamaisAuditeSansCallout(t *testing.T) {
	dir := t.TempDir()
	ecrireFichier(t, filepath.Join(dir, "un.md"), "titre\nun fait quelconque\n")
	ecrireFichier(t, filepath.Join(dir, "deux.md"), "titre\nun autre fait\n")

	res, err := Scan(dir)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if !res.JamaisAudite {
		t.Error("aucun callout dans le corpus, JamaisAudite doit etre vrai")
	}
	if res.Notes != 2 {
		t.Errorf("Notes = %d, attendu 2", res.Notes)
	}
}

func TestScanIgnoreLesSousDossiers(t *testing.T) {
	dir := t.TempDir()
	ecrireFichier(t, filepath.Join(dir, "un.md"), "titre\nfait\n")
	sousDossier := filepath.Join(dir, "sous")
	if err := os.MkdirAll(sousDossier, 0o755); err != nil {
		t.Fatal(err)
	}
	c := Callout{Date: "2026-09-01", Origine: "x", Etat: Invalide, Citation: "fait cache"}
	ecrireFichier(t, filepath.Join(sousDossier, "cache.md"), "titre\nfait cache\n"+Render(c))

	res, err := Scan(dir)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if res.Notes != 1 {
		t.Errorf("Notes = %d, attendu 1 (sous-dossier ignore)", res.Notes)
	}
	if !res.JamaisAudite {
		t.Error("le seul callout existant est dans un sous-dossier ignore : JamaisAudite doit rester vrai")
	}
}

func TestScanToleresLesFinsDeLigneCRLF(t *testing.T) {
	dir := t.TempDir()
	c := Callout{Date: "2026-09-01", Origine: "x", Etat: AValider, Citation: "fait"}
	rendu := strings.ReplaceAll(Render(c), "\n", "\r\n")
	ecrireFichier(t, filepath.Join(dir, "un.md"), "titre\r\nfait\r\n"+rendu)

	res, err := Scan(dir)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if res.Comptes[AValider] != 1 {
		t.Errorf("Comptes[AValider] = %d, attendu 1 (CRLF doit etre tolere)", res.Comptes[AValider])
	}
	if res.JamaisAudite {
		t.Error("un callout CRLF valide ne doit pas compter comme jamais audite")
	}
}

func TestScanCheminInexistant(t *testing.T) {
	if _, err := Scan(filepath.Join(t.TempDir(), "nexiste-pas")); err == nil {
		t.Error("un chemin inexistant doit rendre une erreur")
	}
}
