package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/UnPoilTefal/perimeter/internal/perimeter"
)

// Tests d'acceptation de #66. « perimeter » est la sous-commande dont le
// metier est d'inspecter le registre, et c'etait la seule a ne pas savoir le
// trouver : elle ouvrait perimeter.yml dans le repertoire courant pendant que
// lint et verify empruntaient l'ordre de resolution de #43.
//
// Ils s'observent de l'exterieur, par la sortie de la commande : c'est la
// premiere ligne de « perimeter » qui nomme le registre retenu, et c'est donc
// la seule facon de juger *lequel* a ete choisi plutot que de constater qu'il
// y en a eu un.

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

// posePerimetre ecrit un registre a l'emplacement voulu, avec le corpus qu'il
// pilote, et rend le chemin du registre. Le corpus est minimal : ce qui est
// juge ici est la resolution du registre, jamais le contenu du corpus.
func posePerimetre(t *testing.T, regPath string) string {
	t.Helper()
	corpusDir := filepath.Join(filepath.Dir(regPath), "memoire")
	if err := os.MkdirAll(corpusDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Une accroche redigee a la main : legitime en regime authored, qui est
	// celui que declare le gabarit.
	ecrire(t, filepath.Join(corpusDir, "MEMORY.md"),
		"# Index\n\n- [Reference: un](reference-un.md) — le piege, formule pour l'humain\n")
	ecrire(t, filepath.Join(corpusDir, "reference-un.md"),
		"---\nname: reference-un\ndescription: \"Le port 9 de l'UDM Pro est le WAN2, indiscernable d'un port LAN libre\"\n"+
			"metadata:\n  type: reference\n  modified: 2026-09-01\n---\n\nUn fait.\n")
	ecrire(t, regPath, strings.Replace(gabaritRegistre, "%s", corpusDir, 1))
	return regPath
}

// sortieDe joue une commande en captant sa sortie standard. « perimeter »
// ecrit le registre retenu sur sa premiere ligne ; sans cette capture, on ne
// pourrait verifier que l'absence d'erreur — ce qui laisserait passer un
// registre autre que celui de lint.
func sortieDe(t *testing.T, f func() error) (string, error) {
	t.Helper()
	fichier := filepath.Join(t.TempDir(), "sortie.txt")
	h, err := os.Create(fichier)
	if err != nil {
		t.Fatal(err)
	}
	ancien := os.Stdout
	os.Stdout = h
	errCmd := f()
	os.Stdout = ancien
	if err := h.Close(); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(fichier)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw), errCmd
}

// registreDeLint rend le registre que retiennent lint et verify depuis le
// repertoire courant. Il passe par le chemin de production, pas par une
// reimplementation de la resolution : comparer « perimeter » a une copie de
// la regle ne prouverait rien sur la regle appliquee.
func registreDeLint(t *testing.T) string {
	t.Helper()
	_, reg, err := resolveCorpus("", "")
	if err != nil {
		t.Fatalf("lint ne resout pas de registre ici : %v", err)
	}
	return reg.Path
}

// A1 — sans PERIMETER, sans chemin positionnel, et avec un registre a
// l'emplacement utilisateur : « perimeter » doit retenir le meme registre que
// « lint ». C'est le parcours exact du rapport — un registre ramene a
// l'emplacement par defaut, donc la variable retiree, et « perimeter » qui
// cesse de fonctionner pendant que les autres continuent.
func TestPerimeterRetientLeMemeRegistreQueLint(t *testing.T) {
	racine := t.TempDir()
	xdg := filepath.Join(racine, "xdg")
	utilisateur := posePerimetre(t, filepath.Join(xdg, "perimeter", perimeter.File))
	travail := filepath.Join(racine, "tour-de-controle")
	if err := os.MkdirAll(travail, 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("XDG_CONFIG_HOME", xdg)
	t.Setenv(perimeter.EnvVar, "")
	t.Chdir(travail)

	attendu := registreDeLint(t)
	if attendu != utilisateur {
		t.Fatalf("lint devait retenir %s, obtenu %s", utilisateur, attendu)
	}

	sortie, err := sortieDe(t, func() error { return cmdPerimeter(nil) })
	if err != nil {
		t.Fatalf("perimeter sans chemin doit trouver le registre, obtenu : %v", err)
	}
	if !strings.HasPrefix(sortie, attendu+" ") {
		t.Errorf("perimeter doit retenir le registre de lint (%s), sortie :\n%s", attendu, sortie)
	}
}

// A2 — la garde a ne pas manquer. Un test qui verifie seulement que
// « perimeter » trouve *un* registre serait vert alors meme qu'il en
// trouverait un autre que lint. Deux registres repondent ici — l'un par
// remontee d'arborescence, l'autre a l'emplacement utilisateur — et l'ordre
// entre les deux doit etre le meme des deux cotes : c'est cet ordre, et pas
// seulement le fait de trouver, qui doit etre partage.
func TestPerimeterEtLintDepartagentDeuxRegistresPareillement(t *testing.T) {
	racine := t.TempDir()
	xdg := filepath.Join(racine, "xdg")
	posePerimetre(t, filepath.Join(xdg, "perimeter", perimeter.File))
	depot := filepath.Join(racine, "depot")
	remontee := posePerimetre(t, filepath.Join(depot, perimeter.File))
	travail := filepath.Join(depot, "sous", "dossier")
	if err := os.MkdirAll(travail, 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("XDG_CONFIG_HOME", xdg)
	t.Setenv(perimeter.EnvVar, "")
	t.Chdir(travail)

	attendu := registreDeLint(t)
	if attendu != remontee {
		t.Fatalf("la remontee doit primer sur l'emplacement utilisateur, lint a retenu %s", attendu)
	}

	sortie, err := sortieDe(t, func() error { return cmdPerimeter(nil) })
	if err != nil {
		t.Fatalf("perimeter sans chemin doit trouver le registre, obtenu : %v", err)
	}
	if !strings.HasPrefix(sortie, attendu+" ") {
		t.Errorf("perimeter a retenu un autre registre que lint (%s), sortie :\n%s", attendu, sortie)
	}
}

// A3 — sans registre du tout, la sortie liste les emplacements cherches.
// Aucune erreur « open » ne doit plus sortir de ce chemin : c'est la classe
// de defaut que les diagnostics de premier contact (#52) visaient, et a
// laquelle « perimeter » avait echappe.
func TestPerimeterSansRegistreListeLesEmplacementsCherches(t *testing.T) {
	racine := t.TempDir()
	travail := filepath.Join(racine, "vide")
	if err := os.MkdirAll(travail, 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("XDG_CONFIG_HOME", filepath.Join(racine, "xdg-sans-registre"))
	t.Setenv(perimeter.EnvVar, "")
	t.Chdir(travail)

	_, err := sortieDe(t, func() error { return cmdPerimeter(nil) })
	if err == nil {
		t.Fatal("sans registre, la commande doit echouer")
	}
	msg := err.Error()
	for _, attendu := range []string{"aucun " + perimeter.File, perimeter.EnvVar, "perctl init"} {
		if !strings.Contains(msg, attendu) {
			t.Errorf("le diagnostic doit citer %q, obtenu :\n%s", attendu, msg)
		}
	}
	if strings.Contains(msg, "no such file or directory") {
		t.Errorf("l'erreur brute de open remonte encore :\n%s", msg)
	}
	// Le conseil doit designer un moyen qui existe ici. Sur cette
	// sous-commande le registre se donne en argument : conseiller un drapeau
	// absent enverrait l'utilisateur sur « flag provided but not defined ».
	if strings.Contains(msg, "--perimeter") {
		t.Errorf("« perimeter » n'a pas de --perimeter : ne pas le conseiller :\n%s", msg)
	}
}

// A4 — une piste explicite qui ne repond pas se dit, et ne retombe pas en
// silence sur un autre registre. Un registre valide est pose a l'emplacement
// utilisateur : s'il etait retenu, l'utilisateur travaillerait sur un
// perimetre qu'il n'a pas nomme.
func TestPerimeterVariableCasseeNeRetombePasEnSilence(t *testing.T) {
	racine := t.TempDir()
	xdg := filepath.Join(racine, "xdg")
	posePerimetre(t, filepath.Join(xdg, "perimeter", perimeter.File))
	travail := filepath.Join(racine, "tour-de-controle")
	if err := os.MkdirAll(travail, 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("XDG_CONFIG_HOME", xdg)
	t.Setenv(perimeter.EnvVar, filepath.Join(racine, "nexiste-pas.yml"))
	t.Chdir(travail)

	sortie, err := sortieDe(t, func() error { return cmdPerimeter(nil) })
	if err == nil {
		t.Fatalf("un %s pointant un fichier absent doit etre signale, sortie :\n%s", perimeter.EnvVar, sortie)
	}
	msg := err.Error()
	for _, attendu := range []string{perimeter.EnvVar, "nexiste-pas.yml", "effacer"} {
		if !strings.Contains(msg, attendu) {
			t.Errorf("le message doit citer %q, obtenu :\n%s", attendu, msg)
		}
	}
	if strings.Contains(msg, "no such file or directory") {
		t.Errorf("l'erreur brute de open remonte encore :\n%s", msg)
	}
	if sortie != "" {
		t.Errorf("aucun registre ne doit avoir ete inspecte, sortie :\n%s", sortie)
	}
}

// A5 — le chemin positionnel reste prioritaire : il dit *quel registre*,
// exactement comme un chemin dit *quel corpus* pour lint (#54). La variable
// designe ici un autre registre, et ne doit pas l'emporter.
func TestUnCheminPositionnelPrimeSurLaResolution(t *testing.T) {
	racine := t.TempDir()
	ambiant := posePerimetre(t, filepath.Join(racine, "ambiant", perimeter.File))
	designe := posePerimetre(t, filepath.Join(racine, "designe", perimeter.File))

	t.Setenv("XDG_CONFIG_HOME", filepath.Join(racine, "xdg-sans-registre"))
	t.Setenv(perimeter.EnvVar, ambiant)
	t.Chdir(racine)

	sortie, err := sortieDe(t, func() error { return cmdPerimeter([]string{designe}) })
	if err != nil {
		t.Fatalf("perimeter <registre> : %v", err)
	}
	if !strings.HasPrefix(sortie, designe+" ") {
		t.Errorf("le chemin donne doit primer sur %s, sortie :\n%s", perimeter.EnvVar, sortie)
	}
}
