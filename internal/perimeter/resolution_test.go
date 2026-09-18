package perimeter

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func poseRegistre(t *testing.T, dir string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, File)
	if err := os.WriteFile(p, []byte("version: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// R1 — l'ordre de resolution, du plus explicite au plus implicite. Chaque
// niveau l'emporte sur le suivant : sans ca, un defaut utilisateur masquerait
// le registre du depot ou l'on se trouve.
func TestOrdreDeResolution(t *testing.T) {
	racine := t.TempDir()
	depot := filepath.Join(racine, "depot")
	maison := filepath.Join(racine, "config")
	viaDepot := poseRegistre(t, depot)
	viaMaison := poseRegistre(t, maison)
	viaEnv := poseRegistre(t, filepath.Join(racine, "env"))

	t.Setenv(EnvVar, viaEnv)
	got, src, ok := Resoudre(depot, maison)
	if !ok || got != viaEnv {
		t.Fatalf("la variable d'environnement doit primer : %q (%s)", got, src)
	}
	if src != "PERIMETER" {
		t.Errorf("la provenance doit etre nommee, obtenu %q", src)
	}

	t.Setenv(EnvVar, "")
	got, src, ok = Resoudre(depot, maison)
	if !ok || got != viaDepot {
		t.Fatalf("a defaut, la remontee d'arborescence : %q (%s)", got, src)
	}

	// Le depot n'a plus de registre : l'emplacement utilisateur prend le relais.
	if err := os.Remove(viaDepot); err != nil {
		t.Fatal(err)
	}
	got, src, ok = Resoudre(depot, maison)
	if !ok || got != viaMaison {
		t.Fatalf("en dernier, l'emplacement utilisateur : %q (%s)", got, src)
	}
	if src == "" {
		t.Error("la provenance doit rester nommee")
	}
}

// R2 — le cas de la tour de controle : on travaille depuis un repertoire qui
// ne contient aucun registre, et le registre unique vit ailleurs. C'est
// l'usage qui n'etait pas servi.
func TestTourDeControleTrouveLeRegistreUtilisateur(t *testing.T) {
	racine := t.TempDir()
	travail := filepath.Join(racine, "sources", "un-depot")
	if err := os.MkdirAll(travail, 0o755); err != nil {
		t.Fatal(err)
	}
	maison := filepath.Join(racine, "config")
	attendu := poseRegistre(t, maison)

	t.Setenv(EnvVar, "")
	got, _, ok := Resoudre(travail, maison)
	if !ok || got != attendu {
		t.Fatalf("depuis un repertoire sans registre, l'emplacement utilisateur doit repondre : %q", got)
	}
}

// R3 — quand rien ne repond, dire ou l'on a cherche. Sans ca le diagnostic
// reste entierement a la charge de l'utilisateur, ce qui est precisement ce
// qui a fait vivre le defaut si longtemps.
func TestLEchecNommeLesEndroitsCherches(t *testing.T) {
	racine := t.TempDir()
	travail := filepath.Join(racine, "vide")
	if err := os.MkdirAll(travail, 0o755); err != nil {
		t.Fatal(err)
	}
	maison := filepath.Join(racine, "config")

	t.Setenv(EnvVar, "")
	if _, _, ok := Resoudre(travail, maison); ok {
		t.Fatal("aucun registre ne doit etre trouve")
	}
	msg := Introuvable(travail, maison, "indiquer un chemin avec --perimeter")
	for _, attendu := range []string{"PERIMETER", travail, maison} {
		if !strings.Contains(msg, attendu) {
			t.Errorf("le message doit citer %q, obtenu :\n%s", attendu, msg)
		}
	}
}

// R4 — un chemin donne par la variable mais inexistant ne doit pas etre
// ignore en silence : l'utilisateur a exprime une intention, la taire le
// renverrait vers un autre registre sans qu'il le sache.
func TestUnePisteExpliciteIntrouvableNeRetombePasEnSilence(t *testing.T) {
	racine := t.TempDir()
	depot := filepath.Join(racine, "depot")
	poseRegistre(t, depot)

	t.Setenv(EnvVar, filepath.Join(racine, "nexiste-pas.yml"))
	if _, _, ok := Resoudre(depot, ""); ok {
		t.Error("PERIMETER pointe un fichier absent : il faut le signaler, pas retomber sur le depot")
	}
}
