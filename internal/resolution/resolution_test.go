package resolution

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/UnPoilTefal/perimeter/internal/perimeter"
)

const gabaritRegistre = `version: 1
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
    endpoint: "%s"
    probe:
      cmd: "true"
    corpus:
      index: MEMORY.md
      index_hook: authored
`

func ecrire(t *testing.T, p, contenu string) {
	t.Helper()
	if err := os.WriteFile(p, []byte(contenu), 0o644); err != nil {
		t.Fatal(err)
	}
}

// posePerimetre ecrit un registre a l'emplacement voulu, avec le corpus qu'il
// pilote, et rend le chemin du registre.
func posePerimetre(t *testing.T, regPath string) string {
	t.Helper()
	corpusDir := filepath.Join(filepath.Dir(regPath), "memoire")
	if err := os.MkdirAll(corpusDir, 0o755); err != nil {
		t.Fatal(err)
	}
	ecrire(t, filepath.Join(corpusDir, "MEMORY.md"),
		"# Index\n\n- [Reference: un](reference-un.md) — le piege, formule pour l'humain\n")
	ecrire(t, filepath.Join(corpusDir, "reference-un.md"),
		"---\nname: reference-un\ndescription: \"Le port 9 de l'UDM Pro est le WAN2, indiscernable d'un port LAN libre\"\n"+
			"metadata:\n  type: reference\n  modified: 2026-09-01\n---\n\nUn fait.\n")
	if err := os.MkdirAll(filepath.Dir(regPath), 0o755); err != nil {
		t.Fatal(err)
	}
	ecrire(t, regPath, strings.Replace(gabaritRegistre, "%s", corpusDir, 1))
	return regPath
}

func corpusEtRegistre(t *testing.T) (corpusDir, regPath string) {
	t.Helper()
	regPath = posePerimetre(t, filepath.Join(t.TempDir(), perimeter.File))
	return filepath.Join(filepath.Dir(regPath), "memoire"), regPath
}

// P1 — un chemin dit quel corpus, un --perimeter explicite dit quelle
// politique. Les deux ne s'excluent pas.
func TestCheminEtRegistreExpliciteCoexistent(t *testing.T) {
	corpusDir, regPath := corpusEtRegistre(t)

	c, reg, warnings, err := Resolve(corpusDir, regPath)
	if err != nil {
		t.Fatalf("resolution : %v", err)
	}
	if reg == nil {
		t.Fatal("un --perimeter explicite doit etre honore meme avec un chemin")
	}
	if got := c.Config.Policy.IndexHook; got != "authored" {
		t.Errorf("la politique du registre doit piloter le corpus, index_hook = %q", got)
	}
	if c.Root != corpusDir {
		t.Errorf("le corpus doit rester celui du chemin donne, obtenu %s", c.Root)
	}
	if len(warnings) != 0 {
		t.Errorf("aucune note attendue ici, obtenu %v", warnings)
	}
}

// P2 — un registre seulement ambiant ne s'applique pas a un corpus designe a
// la main, et le dit via une note plutot que de l'appliquer en silence.
func TestUnRegistreAmbiantNeSAppliquePasAUnCheminDonne(t *testing.T) {
	corpusDir, regPath := corpusEtRegistre(t)
	t.Setenv(perimeter.EnvVar, regPath)

	c, reg, warnings, err := Resolve(corpusDir, "")
	if err != nil {
		t.Fatalf("resolution : %v", err)
	}
	if reg != nil {
		t.Error("un registre ambiant ne doit pas piloter un corpus designe explicitement")
	}
	if got := c.Config.Policy.IndexHook; got != "derived" {
		t.Errorf("les defauts doivent s'appliquer, index_hook = %q", got)
	}
	if len(warnings) != 1 {
		t.Fatalf("attendu une note sur %s ignoree, obtenu %v", perimeter.EnvVar, warnings)
	}
}

// P3 — sans chemin, la resolution habituelle reste intacte.
func TestSansCheminLaResolutionResteInchangee(t *testing.T) {
	_, regPath := corpusEtRegistre(t)
	t.Setenv(perimeter.EnvVar, regPath)

	_, reg, _, err := Resolve("", "")
	if err != nil {
		t.Fatalf("resolution : %v", err)
	}
	if reg == nil {
		t.Fatal("sans chemin, le registre ambiant doit repondre")
	}
}
