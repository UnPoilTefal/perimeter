package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/UnPoilTefal/perimeter/internal/perimeter"
)

// corpusEtRegistre depose un corpus et un registre qui le pilote avec une
// politique reconnaissable. Le gabarit est celui de posePerimetre : une seule
// definition, pour que les deux familles de tests parlent du meme registre.
func corpusEtRegistre(t *testing.T) (corpusDir, regPath string) {
	t.Helper()
	regPath = posePerimetre(t, filepath.Join(t.TempDir(), perimeter.File))
	return filepath.Join(filepath.Dir(regPath), "memoire"), regPath
}

func ecrire(t *testing.T, p, contenu string) {
	t.Helper()
	if err := os.WriteFile(p, []byte(contenu), 0o644); err != nil {
		t.Fatal(err)
	}
}

// P1 — un chemin dit quel corpus, un --perimeter explicite dit quelle
// politique. Les deux ne s'excluent pas, et les combiner etait jusqu'ici
// accepte puis ignore en silence.
func TestCheminEtRegistreExpliciteCoexistent(t *testing.T) {
	corpusDir, regPath := corpusEtRegistre(t)

	c, reg, err := resolveCorpus(corpusDir, regPath)
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
}

// P2 — un registre seulement ambiant ne s'applique pas a un corpus designe a
// la main. Sans cette limite, PERIMETER exporte dans un shell ferait appliquer
// la politique d'un perimetre a n'importe quel repertoire analyse au passage.
func TestUnRegistreAmbiantNeSAppliquePasAUnCheminDonne(t *testing.T) {
	corpusDir, regPath := corpusEtRegistre(t)
	t.Setenv(perimeter.EnvVar, regPath)

	c, reg, err := resolveCorpus(corpusDir, "")
	if err != nil {
		t.Fatalf("resolution : %v", err)
	}
	if reg != nil {
		t.Error("un registre ambiant ne doit pas piloter un corpus designe explicitement")
	}
	if got := c.Config.Policy.IndexHook; got != "derived" {
		t.Errorf("les defauts doivent s'appliquer, index_hook = %q", got)
	}
}

// P3 — sans chemin, la resolution habituelle reste intacte.
func TestSansCheminLaResolutionResteInchangee(t *testing.T) {
	_, regPath := corpusEtRegistre(t)
	t.Setenv(perimeter.EnvVar, regPath)

	_, reg, err := resolveCorpus("", "")
	if err != nil {
		t.Fatalf("resolution : %v", err)
	}
	if reg == nil {
		t.Fatal("sans chemin, le registre ambiant doit repondre")
	}
}
