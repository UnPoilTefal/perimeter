package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/UnPoilTefal/perimeter/internal/perimeter"
)

const registreSonde = `version: 1
roles:
  intention.spec:        { source: specs }
  intention.tickets:     { source: tickets }
  contrainte.decisions:  { source: adr }
  contrainte.memoire:    { source: memoire }
  etat.declare:          { source: depots }
  etat.reel:             { source: cluster }
sources:
  specs:    { adapter: files,  reliability: measured, probe: { cmd: "true" } }
  tickets:  { adapter: github, reliability: measured, endpoint: "org/produit",
              probe: { cmd: "echo org/produit", expect_stdout: "^org/produit$" } }
  adr:      { adapter: git,    reliability: declared, probe: { cmd: "true" } }
  memoire:  { adapter: files,  reliability: declared, probe: { cmd: "true" } }
  depots:   { adapter: git,    reliability: declared, probe: { cmd: "true" } }
  cluster:  { adapter: http,   reliability: measured, probe: { cmd: "%s" } }
`

func registrePourSondes(t *testing.T, sondeCluster string) *perimeter.Registry {
	t.Helper()
	p := filepath.Join(t.TempDir(), perimeter.File)
	ecrire(t, p, strings.Replace(registreSonde, "%s", sondeCluster, 1))
	reg, err := perimeter.Load(p)
	if err != nil {
		t.Fatal(err)
	}
	return reg
}

// Test d'acceptation de #56, au niveau de la commande : un registre dont une
// seule sonde echoue rend une sortie et un code de retour differents du meme
// registre dont toutes les sondes passent. Aucun corpus n'entre ici — c'est
// tout l'interet : le rejeu ne depend plus de l'etat des preuves de notes.
func TestPerimeterProbeDistingueUneSondeCassee(t *testing.T) {
	var sain, casse strings.Builder
	codeSain, err := sonderSources(registrePourSondes(t, "true"), true, 0, &sain)
	if err != nil {
		t.Fatal(err)
	}
	codeCasse, err := sonderSources(registrePourSondes(t, "exit 1"), true, 0, &casse)
	if err != nil {
		t.Fatal(err)
	}

	if codeSain != 0 {
		t.Errorf("toutes les sondes passent : code 0 attendu, obtenu %d\n%s", codeSain, sain.String())
	}
	if codeCasse == 0 {
		t.Errorf("une sonde en echec doit rendre un code non nul\n%s", casse.String())
	}
	if sain.String() == casse.String() {
		t.Fatalf("sortie identique pour les deux registres :\n%s", sain.String())
	}
	if !strings.Contains(casse.String(), "cluster") {
		t.Errorf("la source en echec doit etre nommee :\n%s", casse.String())
	}
}

// Les sondes sont des commandes declarees dans un fichier de configuration.
// Sans --allow-exec, on refuse explicitement — on ne se tait pas.
func TestPerimeterProbeRefuseSansAllowExec(t *testing.T) {
	var out strings.Builder
	_, err := sonderSources(registrePourSondes(t, "true"), false, 0, &out)
	if err == nil {
		t.Fatal("l'execution devait etre refusee sans --allow-exec")
	}
	if !strings.Contains(err.Error(), "--allow-exec") {
		t.Errorf("le refus doit nommer le drapeau manquant, obtenu : %v", err)
	}
	if out.String() != "" {
		t.Errorf("rien ne doit etre rejoue ni rendu : %q", out.String())
	}
}
