package verify

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/UnPoilTefal/perimeter/internal/corpus"
	"github.com/UnPoilTefal/perimeter/internal/perimeter"
	"github.com/UnPoilTefal/perimeter/internal/report"
)

// noteSansPreuve est la situation de quelqu'un qui vient de poser son premier
// registre : des faits, aucune preuve attachee — et c'est precisement le
// moment ou il veut savoir si ses sources repondent.
const noteSansPreuve = `---
name: reference-sans-preuve
description: "Un fait pose sans preuve attachee, comme au premier jour d'un corpus"
metadata:
  type: reference
  modified: 2026-09-13
---

Corps.
`

// registreCasse est le meme registre, une seule sonde remplacee par un echec.
var registreCasse = strings.Replace(registreTest,
	`cluster:  { adapter: http,   reliability: measured, probe: { cmd: "false" } }`,
	`cluster:  { adapter: http,   reliability: measured, probe: { cmd: "exit 1" } }`, 1)

// registreSain est le meme registre, toutes sondes passantes.
var registreSain = strings.Replace(registreTest,
	`cluster:  { adapter: http,   reliability: measured, probe: { cmd: "false" } }`,
	`cluster:  { adapter: http,   reliability: measured, probe: { cmd: "true" } }`, 1)

func corpusSansPreuve(t *testing.T) *corpus.Corpus {
	t.Helper()
	return load(t, corpusAvec(t, map[string]string{"reference-sans-preuve.md": noteSansPreuve}))
}

// Test d'acceptation de #56 — un registre dont une seule sonde echoue produit
// une sortie et un code de retour differents du meme registre dont toutes les
// sondes passent, sur un corpus sans aucune preuve de note.
//
// Sans cela, « ✓ registre coherent » peut decrire un registre dont aucune
// source n'est joignable : une garde acceptee, sans effet, et muette.
func TestSondesObservablesSurUnCorpusSansPreuve(t *testing.T) {
	sain, _, err := Run(corpusSansPreuve(t), Options{
		AllowExec: true, ProbeSources: true, Registry: registreDepuis(t, registreSain),
	})
	if err != nil {
		t.Fatal(err)
	}
	casse, _, err := Run(corpusSansPreuve(t), Options{
		AllowExec: true, ProbeSources: true, Registry: registreDepuis(t, registreCasse),
	})
	if err != nil {
		t.Fatal(err)
	}

	if sain.ExitCode(false) != 0 {
		t.Errorf("toutes les sondes passent : code de retour attendu 0, obtenu %d (%+v)",
			sain.ExitCode(false), sain.Findings)
	}
	if casse.ExitCode(false) == 0 {
		t.Errorf("une sonde en echec doit faire sortir non nul, obtenu %d", casse.ExitCode(false))
	}
	if rendu(t, sain) == rendu(t, casse) {
		t.Error("les deux registres rendent la meme sortie : une sonde cassee est invisible")
	}
	if ko, _ := casse.Stats["sources_ko"].(int); ko != 1 {
		t.Errorf("1 source en echec attendue, compte : %v", casse.Stats["sources_ko"])
	}
}

// Les sondes sont des commandes vivant dans un fichier de configuration : les
// jouer reste soumis a --allow-exec, et le refus doit etre dit, pas tu.
func TestSondesRefuseesSansAutorisationDExecution(t *testing.T) {
	_, _, err := Run(corpusSansPreuve(t), Options{
		AllowExec: false, ProbeSources: true, Registry: registreDepuis(t, registreSain),
	})
	if !errors.Is(err, ErrExecRefused) {
		t.Fatalf("execution attendue refusee, obtenu : %v", err)
	}
}

// registreDepuis charge un registre ecrit a la volee.
func registreDepuis(t *testing.T, contenu string) *perimeter.Registry {
	t.Helper()
	p := filepath.Join(t.TempDir(), perimeter.File)
	if err := os.WriteFile(p, []byte(contenu), 0o644); err != nil {
		t.Fatal(err)
	}
	reg, err := perimeter.Load(p)
	if err != nil {
		t.Fatal(err)
	}
	return reg
}

// rendu rend la sortie humaine d'un resultat : c'est ce que l'utilisateur
// compare entre deux executions.
func rendu(t *testing.T, res *report.Result) string {
	t.Helper()
	var b strings.Builder
	if err := res.Write(&b, report.Rendu{Format: "human"}); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

// Le rapport de sondes est ce qu'un humain lit : il doit distinguer les deux
// registres a l'oeil, et porter le verdict dans son code de retour.
func TestRapportDeSondesDistingueLesDeuxRegistres(t *testing.T) {
	sain := Sonder(registreDepuis(t, registreSain), time.Second*5)
	casse := Sonder(registreDepuis(t, registreCasse), time.Second*5)

	if sain.ExitCode() != 0 || sain.Echecs() != 0 {
		t.Errorf("toutes les sondes passent : %d echec(s), code %d", sain.Echecs(), sain.ExitCode())
	}
	if casse.ExitCode() == 0 || casse.Echecs() != 1 {
		t.Errorf("une sonde en echec attendue : %d echec(s), code %d", casse.Echecs(), casse.ExitCode())
	}

	var a, b strings.Builder
	if err := sain.Ecrire(&a); err != nil {
		t.Fatal(err)
	}
	if err := casse.Ecrire(&b); err != nil {
		t.Fatal(err)
	}
	if a.String() == b.String() {
		t.Fatal("les deux rapports sont identiques")
	}
	// Une sortie utile nomme la source, ce qu'elle a repondu, et ce qu'on
	// attendait d'elle — sans quoi on sait qu'il faut chercher, pas ou.
	for _, attendu := range []string{"cluster", "exit 1", "sonde   :", "sortie  :", "attendu :"} {
		if !strings.Contains(b.String(), attendu) {
			t.Errorf("le rapport d'echec doit contenir %q :\n%s", attendu, b.String())
		}
	}
	if !strings.Contains(a.String(), "6 sondes rejouees") {
		t.Errorf("le rapport doit dire combien de sondes ont ete rejouees :\n%s", a.String())
	}
}
