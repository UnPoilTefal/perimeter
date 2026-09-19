package reliability

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func ecrire(t *testing.T, dir string, lignes ...string) string {
	t.Helper()
	p := filepath.Join(dir, "perimeter.reliability.jsonl")
	body := ""
	for _, l := range lignes {
		body += l + "\n"
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// IndexPath place toujours l'index a cote du registre, jamais dans le corpus.
func TestIndexPathEstACoteDuRegistre(t *testing.T) {
	got := IndexPath("/perimetre/equipe/perimeter.yml")
	want := "/perimetre/equipe/perimeter.reliability.jsonl"
	if got != want {
		t.Errorf("chemin attendu %q, obtenu %q", want, got)
	}
}

// Un index absent n'est pas une erreur : un perimetre qui n'a encore rien
// confirme a un index vide, pas casse.
func TestUnIndexAbsentRendUneListeVide(t *testing.T) {
	entries, err := Load(filepath.Join(t.TempDir(), "absent.jsonl"))
	if err != nil {
		t.Fatalf("un index absent ne doit pas etre une erreur : %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("liste vide attendue, obtenu %d entrees", len(entries))
	}
}

// L'index est un journal : l'etat courant d'une reference est sa derniere
// ligne, jamais une reecriture en place.
func TestLEtatCourantEstLaDerniereLigne(t *testing.T) {
	dir := t.TempDir()
	p := ecrire(t, dir,
		`{"ref":"a.md#L1","status":"confirme-humain","confirmed_at":"2026-01-01"}`,
		`{"ref":"a.md#L1","status":"confirme-humain","confirmed_at":"2026-09-01"}`,
	)
	entries, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	last, ok := Latest(entries, "a.md#L1")
	if !ok {
		t.Fatal("une entree etait attendue")
	}
	if last.ConfirmedAt != "2026-09-01" {
		t.Errorf("la derniere confirmation attendue 2026-09-01, obtenue %s", last.ConfirmedAt)
	}
}

// Sans aucune entree, le verdict est sans-signal — etat legitime, pas une erreur.
func TestSansEntreeLeVerdictEstSansSignal(t *testing.T) {
	v := Check(nil, "note.md#L1", t.TempDir(), 180, time.Now())
	if v.Status != StatusSansSignal {
		t.Errorf("statut attendu sans-signal, obtenu %s", v.Status)
	}
	if v.Found {
		t.Error("Found ne doit pas etre vrai sans entree")
	}
}

// Une confirmation humaine hors de la fenetre de fraicheur est signalee stale.
func TestUneConfirmationHumaineHorsFenetreEstStale(t *testing.T) {
	entries := []Entry{{Ref: "note.md#L1", Status: StatusConfirmeHumain, ConfirmedAt: "2026-01-01"}}
	now := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC) // 243 jours plus tard
	v := Check(entries, "note.md#L1", t.TempDir(), 180, now)
	if !v.Stale {
		t.Error("243 jours > budget non-sonde de 180 jours : stale attendu")
	}
}

// Une confirmation humaine recente, dans la fenetre, n'est pas stale.
func TestUneConfirmationHumaineRecenteNEstPasStale(t *testing.T) {
	entries := []Entry{{Ref: "note.md#L1", Status: StatusConfirmeHumain, ConfirmedAt: "2026-08-15"}}
	now := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC) // 17 jours plus tard
	v := Check(entries, "note.md#L1", t.TempDir(), 180, now)
	if v.Stale {
		t.Error("17 jours < budget non-sonde de 180 jours : stale non attendu")
	}
}

// Le contenu vise par une reference a ligne a change depuis la confirmation :
// l'empreinte ne correspond plus, l'annotation s'invalide automatiquement.
func TestUneReferenceDontLeContenuAChangeEstDrifted(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "note.md"), []byte("ligne 1\nle port 9 est le WAN2\nligne 3\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	empreinteInitiale, ok, err := HashLine(dir, "note.md#L2")
	if err != nil || !ok {
		t.Fatalf("empreinte initiale attendue, err=%v ok=%v", err, ok)
	}
	entries := []Entry{{Ref: "note.md#L2", Status: StatusConfirmeHumain, ConfirmedAt: "2026-09-01", ClaimHash: empreinteInitiale}}

	// Le contenu de la ligne 2 change.
	if err := os.WriteFile(filepath.Join(dir, "note.md"), []byte("ligne 1\nle port 9 est un LAN libre\nligne 3\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	v := Check(entries, "note.md#L2", dir, 180, time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC))
	if !v.Drifted {
		t.Error("le contenu de la ligne 2 a change : Drifted attendu")
	}
}

// Le contenu inchange ne declenche pas de derive, meme avec une empreinte enregistree.
func TestUneReferenceInchangeeNEstPasDrifted(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "note.md"), []byte("ligne 1\nle port 9 est le WAN2\nligne 3\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	empreinte, ok, err := HashLine(dir, "note.md#L2")
	if err != nil || !ok {
		t.Fatalf("empreinte attendue, err=%v ok=%v", err, ok)
	}
	entries := []Entry{{Ref: "note.md#L2", Status: StatusConfirmeHumain, ConfirmedAt: "2026-09-01", ClaimHash: empreinte}}
	v := Check(entries, "note.md#L2", dir, 180, time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC))
	if v.Drifted {
		t.Error("le contenu n'a pas change : Drifted non attendu")
	}
}
