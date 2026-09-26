package audit

import (
	"errors"
	"strings"
	"testing"
)

func TestRenderAValider(t *testing.T) {
	c := Callout{
		Date:     "2026-09-26",
		Origine:  `sonde rejouée (ex. ` + "`kubectl get ...`" + `) ou "seuil temporel >30j"`,
		Etat:     AValider,
		Citation: "citation courte du fait visé",
	}
	want := "> [!verification]-\n" +
		"> **date**: 2026-09-26\n" +
		"> **origine**: sonde rejouée (ex. `kubectl get ...`) ou \"seuil temporel >30j\"\n" +
		"> **état**: à valider\n" +
		"> \"citation courte du fait visé\"\n"

	got := Render(c)
	if got != want {
		t.Errorf("Render() =\n%q\nwant\n%q", got, want)
	}
}

func TestInsertAuMilieu(t *testing.T) {
	content := "ligne 1\nligne 2\nligne 3\n"
	c := Callout{Date: "2026-09-26", Origine: "test", Etat: AValider, Citation: "ligne 2"}

	got, err := Insert(content, 2, c)
	if err != nil {
		t.Fatalf("Insert: %v", err)
	}
	want := "ligne 1\nligne 2\n" + Render(c) + "ligne 3\n"
	if got != want {
		t.Errorf("Insert() =\n%q\nwant\n%q", got, want)
	}
}

func TestInsertSurDerniereLigne(t *testing.T) {
	content := "ligne 1\nligne 2\n"
	c := Callout{Date: "2026-09-26", Origine: "test", Etat: AValider, Citation: "ligne 2"}

	got, err := Insert(content, 2, c)
	if err != nil {
		t.Fatalf("Insert: %v", err)
	}
	want := "ligne 1\nligne 2\n" + Render(c)
	if got != want {
		t.Errorf("Insert() =\n%q\nwant\n%q", got, want)
	}
}

func TestInsertSansRetourALaLigneFinal(t *testing.T) {
	content := "ligne 1\nligne 2"
	c := Callout{Date: "2026-09-26", Origine: "test", Etat: AValider, Citation: "ligne 2"}

	got, err := Insert(content, 2, c)
	if err != nil {
		t.Fatalf("Insert: %v", err)
	}
	want := "ligne 1\nligne 2\n" + Render(c)
	if got != want {
		t.Errorf("Insert() =\n%q\nwant\n%q", got, want)
	}
}

func TestInsertRefuseUnSecondCallout(t *testing.T) {
	c := Callout{Date: "2026-09-26", Origine: "premiere passe", Etat: AValider, Citation: "ligne 2"}
	content, err := Insert("ligne 1\nligne 2\nligne 3\n", 2, c)
	if err != nil {
		t.Fatalf("Insert (premiere passe): %v", err)
	}

	c2 := Callout{Date: "2026-09-27", Origine: "seconde passe", Etat: AValider, Citation: "ligne 2"}
	_, err = Insert(content, 2, c2)
	if !errors.Is(err, ErrCalloutExiste) {
		t.Fatalf("Insert (seconde passe sur la meme ligne) : erreur %v, attendu ErrCalloutExiste", err)
	}
}

func TestInsertLigneHorsBornes(t *testing.T) {
	content := "ligne 1\nligne 2\n"
	c := Callout{Date: "2026-09-26", Origine: "test", Etat: AValider, Citation: "x"}

	for _, lineNo := range []int{0, -1, 3} {
		if _, err := Insert(content, lineNo, c); err == nil {
			t.Errorf("Insert(lineNo=%d) : erreur attendue, rien reçu", lineNo)
		}
	}
}

func TestRenderOrigineMultiligneResteDansLeBlockquote(t *testing.T) {
	c := Callout{
		Date:     "2026-09-26",
		Origine:  "ligne1\nligne2",
		Etat:     AValider,
		Citation: "fait",
	}
	for _, l := range strings.Split(strings.TrimSuffix(Render(c), "\n"), "\n") {
		if !strings.HasPrefix(l, "> ") {
			t.Errorf("ligne du callout sans prefixe « > » (origine multi-lignes mal aplatie) : %q", l)
		}
	}
}

func TestInsertAtLineExtraitLaCitation(t *testing.T) {
	content := "titre\noutil X retenu\nsuite\n"

	got, err := InsertAtLine(content, 2, "2026-09-26", "sélection humaine", AValider)
	if err != nil {
		t.Fatalf("InsertAtLine: %v", err)
	}
	c := Callout{Date: "2026-09-26", Origine: "sélection humaine", Etat: AValider, Citation: "outil X retenu"}
	want := "titre\noutil X retenu\n" + Render(c) + "suite\n"
	if got != want {
		t.Errorf("InsertAtLine() =\n%q\nwant\n%q", got, want)
	}
}

func TestInsertAtLineLigneHorsBornes(t *testing.T) {
	content := "ligne 1\nligne 2\n"
	if _, err := InsertAtLine(content, 0, "2026-09-26", "test", AValider); err == nil {
		t.Errorf("InsertAtLine(lineNo=0) : erreur attendue, rien reçu")
	}
	if _, err := InsertAtLine(content, 5, "2026-09-26", "test", AValider); err == nil {
		t.Errorf("InsertAtLine(lineNo=5) : erreur attendue, rien reçu")
	}
}

func TestInsertAtLineGuillemetsInternes(t *testing.T) {
	content := "note noté \"outil X retenu\" en 2026\n"

	got, err := InsertAtLine(content, 1, "2026-09-26", "test", AValider)
	if err != nil {
		t.Fatalf("InsertAtLine: %v", err)
	}
	for _, l := range strings.Split(strings.TrimSuffix(got, "\n"), "\n")[1:] {
		if !strings.HasPrefix(l, "> ") {
			t.Errorf("ligne du callout sans prefixe « > » : %q", l)
		}
	}
}

func TestRenderConfirmeAvecResoluLe(t *testing.T) {
	c := Callout{
		Date:     "2026-09-20",
		Origine:  "seuil temporel >30j",
		Etat:     Confirme,
		Citation: "outil X retenu",
		ResoluLe: "2026-09-26",
	}
	want := "> [!verification]-\n" +
		"> **date**: 2026-09-20\n" +
		"> **origine**: seuil temporel >30j\n" +
		"> **état**: confirmé\n" +
		"> **résolu_le**: 2026-09-26\n" +
		"> \"outil X retenu\"\n"

	got := Render(c)
	if got != want {
		t.Errorf("Render() =\n%q\nwant\n%q", got, want)
	}
}
