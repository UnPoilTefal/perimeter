package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/UnPoilTefal/perimeter/internal/reliability"
)

// stderrDe joue une commande en captant sa sortie d'erreur, et en detournant
// sa sortie standard vers un fichier temporaire pour ne pas polluer le
// journal de test — seul le contenu de stderr nous interesse ici.
func stderrDe(t *testing.T, f func() error) (string, error) {
	t.Helper()
	fichierOut := filepath.Join(t.TempDir(), "stdout.txt")
	hOut, err := os.Create(fichierOut)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = hOut.Close() }()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	ancienOut, ancienErr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = hOut, w
	errCmd := f()
	os.Stdout, os.Stderr = ancienOut, ancienErr
	_ = w.Close()

	buf := make([]byte, 8192)
	n, _ := r.Read(buf)
	_ = r.Close()
	return string(buf[:n]), errCmd
}

// P1 — un perimetre sain (registre complet, aucun lien casse, aucun index de
// fiabilite) ne produit aucun avis ambiant : le silence est le cas normal.
func TestAvisAmbiantSilencieuxSurUnPerimetreSain(t *testing.T) {
	regPath := posePerimetre(t, filepath.Join(t.TempDir(), "perimeter.yml"))

	out, errCmd := stderrDe(t, func() error {
		return cmdVerify([]string{"--perimeter", regPath})
	})
	if errCmd != nil {
		t.Fatalf("verify : %v", errCmd)
	}
	if strings.Contains(out, "avis ambiant") {
		t.Errorf("un perimetre sain ne doit produire aucun avis ambiant, stderr : %q", out)
	}
}

// P2 — la garantie centrale du chantier #73 : un echec interne du calcul de
// l'avis ambiant (ici, un index de fiabilite illisible) ne doit jamais
// empecher la sous-commande reellement demandee de s'executer jusqu'au bout,
// et ne doit jamais se confondre avec le silence du cas sain.
func TestAvisAmbiantIndisponibleNeBloquePasLaCommandeReelle(t *testing.T) {
	regPath := posePerimetre(t, filepath.Join(t.TempDir(), "perimeter.yml"))
	indexPath := reliability.IndexPath(regPath)
	if err := os.WriteFile(indexPath, []byte("{ceci n'est pas du json}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	out, errCmd := stderrDe(t, func() error {
		return cmdVerify([]string{"--perimeter", regPath})
	})
	if errCmd != nil {
		t.Fatalf("un echec de calcul de l'avis ambiant a bloque verify : %v", errCmd)
	}
	if !strings.Contains(out, "avis ambiant indisponible") {
		t.Errorf("attendu une ligne \"avis ambiant indisponible\", obtenu stderr : %q", out)
	}
}
