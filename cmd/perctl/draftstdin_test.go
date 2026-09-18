package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// « perctl draft » sans argument lit l'entree standard. Deux usages s'y
// croisent, et ils demandent l'inverse l'un de l'autre : en pipe, l'outil ne
// doit rien dire de plus — la sortie est peut-etre redirigee, et un mot ajoute
// la salirait ; au clavier, il doit annoncer ce qu'il attend, sinon il parait
// fige et le premier reflexe est Ctrl-C.

// P1 — stdin non-tty : rien sur stderr. Les trois formes que prend un stdin
// redirige sont prises, parce que c'est la non-regression qui compte ici : le
// pipe est le chemin normal de la commande.
func TestStdinNonTtyNAnnonceRien(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()
	if _, err := w.WriteString("---\nname: brouillon\n---\n"); err != nil {
		t.Fatal(err)
	}
	_ = w.Close()

	fichier := filepath.Join(t.TempDir(), "brouillon.md")
	if err := os.WriteFile(fichier, []byte("un brouillon"), 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(fichier)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()

	vide, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = vide.Close() }()

	for nom, in := range map[string]*os.File{"pipe": r, "fichier": f, "/dev/null": vide} {
		var errBuf bytes.Buffer
		annoncerLectureStdin(in, &errBuf)
		if errBuf.Len() != 0 {
			t.Errorf("stdin %s : rien ne doit paraitre sur stderr, obtenu %q", nom, errBuf.String())
		}
	}
}

// P2 — stdin tty : l'annonce parait sur stderr, avant l'attente.
//
// Un vrai terminal ne s'obtient pas dans un « go test » ordinaire : il faut un
// pty. On prend donc le terminal de controle quand la session en a un — sous
// « script -q /dev/null go test ./... », par exemple — et on passe la main
// sinon. Le job dogfood couvre ce meme cas de bout en bout, sous pty.
func TestStdinTtyAnnonceLAttente(t *testing.T) {
	tty, err := os.Open("/dev/tty")
	if err != nil {
		t.Skip("pas de terminal de controle ici — cas couvert de bout en bout par le job dogfood")
	}
	defer func() { _ = tty.Close() }()

	var errBuf bytes.Buffer
	annoncerLectureStdin(tty, &errBuf)

	ligne := errBuf.String()
	if ligne == "" {
		t.Fatal("stdin tty : l'attente doit etre annoncee sur stderr, rien n'a ete ecrit")
	}
	for _, attendu := range []string{"entree standard", "Ctrl-D", "Ctrl-C"} {
		if !strings.Contains(ligne, attendu) {
			t.Errorf("l'annonce doit dire %q, obtenu %q", attendu, ligne)
		}
	}
}
