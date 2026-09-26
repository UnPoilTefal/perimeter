// Package audit marque, en place dans une note Markdown non-schemee (vault
// Obsidian, corpus d'equipe sans registre perimeter), un fait douteux ou
// invalide, pour confirmation ou invalidation humaine — jamais un score, un
// etat categoriel confirme par un humain, jamais par un agent seul.
//
// Ce mecanisme est deliberement separe de internal/reliability : celui-ci
// tient un index JSONL externe ancre par empreinte de ligne pour des corpus
// schemes (perimeter.yml). Un corpus non-scheme n'a ni registre ni index —
// le seul marqueur de fiabilite est le callout insere dans le texte de la
// note elle-meme.
package audit

import (
	"errors"
	"fmt"
	"strings"
)

// Etat est le statut categoriel d'un fait signale.
type Etat string

const (
	// AValider : le fait est signale douteux, en attente de confirmation
	// humaine.
	AValider Etat = "à valider"
	// Confirme : un humain a confirme le fait contre une source.
	Confirme Etat = "confirmé"
	// Invalide : un humain a invalide le fait — a ne plus prendre pour
	// premisse.
	Invalide Etat = "invalidé"
)

// calloutMarque est la premiere ligne d'un callout deja rendu — elle sert a
// detecter qu'un fait porte deja un marqueur, sans reparser tout le bloc.
const calloutMarque = "> [!verification]-"

// ErrCalloutExiste signale qu'un callout suit deja la ligne visee. Un second
// callout empile perdrait un etat deja tranche par un humain (confirme ou
// invalide) — l'invariant du format (#77) est un seul callout par fait,
// jamais empile.
var ErrCalloutExiste = errors.New("un callout de verification existe deja pour cette ligne")

// Callout est le contenu d'un marqueur de verification insere dans une note.
//
// Citation porte le fait cite, sans les guillemets qui l'entourent dans le
// rendu : un second ancrage a cote de la position dans le fichier, qui
// survit a un reordonnancement du texte.
type Callout struct {
	Date     string
	Origine  string
	Etat     Etat
	Citation string
	// ResoluLe est optionnel — renseigne seulement quand Etat n'est plus
	// AValider, la date a laquelle un humain a tranche.
	ResoluLe string
}

// aplatir remplace les retours a la ligne d'un champ par des espaces — un
// champ du callout tient sur une seule ligne de blockquote ; un retour a la
// ligne brut y sortirait du prefixe « > » et casserait le bloc pour le
// parseur Markdown comme pour skills/verify-before-reuse.
func aplatir(s string) string {
	s = strings.ReplaceAll(s, "\r\n", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	return s
}

// Render rend le callout Obsidian correspondant, pliable par defaut. Chaque
// ligne commence par « > » : c'est un blockquote Obsidian, pas un bloc de
// code — il doit rester lisible et editable a la main dans l'editeur.
func Render(c Callout) string {
	s := calloutMarque + "\n"
	s += fmt.Sprintf("> **date**: %s\n", aplatir(c.Date))
	s += fmt.Sprintf("> **origine**: %s\n", aplatir(c.Origine))
	s += fmt.Sprintf("> **état**: %s\n", c.Etat)
	if c.ResoluLe != "" {
		s += fmt.Sprintf("> **résolu_le**: %s\n", aplatir(c.ResoluLe))
	}
	s += fmt.Sprintf("> %q\n", aplatir(c.Citation))
	return s
}

// lignesDe decoupe le contenu en lignes, en retenant separement si le
// fichier se terminait par un retour a la ligne — Split laisse sinon une
// entree finale vide qui n'est pas une ligne du fichier.
func lignesDe(content string) (lignes []string, finAvecRetour bool) {
	finAvecRetour = strings.HasSuffix(content, "\n")
	lignes = strings.Split(content, "\n")
	if finAvecRetour {
		lignes = lignes[:len(lignes)-1]
	}
	return lignes, finAvecRetour
}

// verifierLigne rend une erreur explicite si lineNo ne designe aucune ligne
// de lignes — une insertion silencieuse au mauvais endroit annoterait le
// mauvais fait, ce qui est pire que ne rien annoter.
func verifierLigne(lignes []string, lineNo int) error {
	if lineNo < 1 {
		return fmt.Errorf("numero de ligne invalide : %d", lineNo)
	}
	if lineNo > len(lignes) {
		return fmt.Errorf("numero de ligne invalide : %d (le fichier en compte %d)", lineNo, len(lignes))
	}
	return nil
}

// Insert insere le callout rendu juste apres la ligne lineNo (1-indexee) du
// contenu donne, et rend le contenu complet resultant. Le reste du fichier —
// avant et apres la ligne visee — est preserve tel quel.
func Insert(content string, lineNo int, c Callout) (string, error) {
	lignes, _ := lignesDe(content)
	if err := verifierLigne(lignes, lineNo); err != nil {
		return "", err
	}
	if lineNo < len(lignes) && strings.TrimSpace(lignes[lineNo]) == calloutMarque {
		return "", ErrCalloutExiste
	}

	var b strings.Builder
	for i, l := range lignes {
		b.WriteString(l)
		b.WriteString("\n")
		if i+1 == lineNo {
			b.WriteString(Render(c))
		}
	}
	return b.String(), nil
}

// InsertAtLine construit le callout a partir du seul texte de la ligne visee
// — l'appelant n'a pas a extraire lui-meme la citation — puis l'insere avec
// Insert. Memes bornes, memes erreurs — y compris ErrCalloutExiste.
func InsertAtLine(content string, lineNo int, date, origine string, etat Etat) (string, error) {
	lignes, _ := lignesDe(content)
	if err := verifierLigne(lignes, lineNo); err != nil {
		return "", err
	}

	c := Callout{
		Date:     date,
		Origine:  origine,
		Etat:     etat,
		Citation: strings.TrimSpace(lignes[lineNo-1]),
	}
	return Insert(content, lineNo, c)
}
