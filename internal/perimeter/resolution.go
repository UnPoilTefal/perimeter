package perimeter

import (
	"fmt"
	"os"
	"path/filepath"
)

// EnvVar nomme le registre a utiliser, quand la remontee d'arborescence ne
// convient pas : session d'agent, CI, ou tour de controle.
const EnvVar = "PERIMETER"

// Resoudre trouve le registre a utiliser, du plus explicite au plus implicite,
// et rend la provenance retenue pour que l'utilisateur sache d'ou vient le
// fichier sur lequel l'outil travaille.
//
// L'ordre compte. La remontee d'arborescence doit primer sur l'emplacement
// utilisateur : sinon un registre personnel masquerait celui du depot ou l'on
// se trouve, et une commande lancee dans un projet travaillerait ailleurs sans
// le dire.
//
// A l'inverse, l'emplacement utilisateur est indispensable a l'usage « tour de
// controle » : un operateur travaille depuis un repertoire central et rayonne
// vers plusieurs depots, dont aucun ne porte le registre. La remontee ne
// trouve alors rien, et c'est le cas qui n'etait pas servi.
func Resoudre(depuis, utilisateur string) (chemin, provenance string, ok bool) {
	// Une piste explicite qui ne repond pas n'est pas ignoree : l'utilisateur
	// a exprime une intention, retomber ailleurs en silence le ferait
	// travailler sur un autre perimetre sans qu'il le sache.
	if env := os.Getenv(EnvVar); env != "" {
		if estFichier(env) {
			return env, EnvVar, true
		}
		return "", EnvVar, false
	}
	if depuis != "" {
		if trouve, ok := Find(depuis); ok {
			return trouve, "remontee depuis " + depuis, true
		}
	}
	if utilisateur != "" {
		candidat := filepath.Join(utilisateur, File)
		if estFichier(candidat) {
			return candidat, "emplacement utilisateur", true
		}
	}
	return "", "", false
}

// Introuvable dit ou l'outil a cherche. Un message qui se contente de
// constater l'absence laisse tout le diagnostic a l'utilisateur — c'est ce qui
// a fait vivre le defaut sans qu'il soit vu.
//
// designation dit comment nommer un registre a la main sur la sous-commande
// qui appelle : ce n'est pas partout le meme argument. Conseiller un drapeau
// qui n'existe pas la ou l'on est renverrait sur « flag provided but not
// defined » — un diagnostic qui envoie dans le mur en vaut a peine un.
func Introuvable(depuis, utilisateur, designation string) string {
	msg := fmt.Sprintf("aucun %s trouve. Cherche, dans cet ordre :\n", File)
	msg += fmt.Sprintf("  1. la variable d'environnement %s (%s)\n", EnvVar, valeurOuVide(os.Getenv(EnvVar)))
	if depuis != "" {
		msg += fmt.Sprintf("  2. en remontant depuis %s\n", depuis)
	}
	if utilisateur != "" {
		msg += fmt.Sprintf("  3. %s\n", filepath.Join(utilisateur, File))
	}
	msg += "\n" + designation + ", exporter " + EnvVar + ", ou ecrire un registre avec « perctl init »"
	return msg
}

// DossierUtilisateur rend l'emplacement du registre par defaut d'un operateur.
// Il respecte XDG_CONFIG_HOME, et rend une chaine vide si rien ne permet de le
// situer — auquel cas ce niveau de resolution est simplement absent.
func DossierUtilisateur() string {
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(x, "perimeter")
	}
	maison, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(maison, ".config", "perimeter")
}

func estFichier(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

func valeurOuVide(v string) string {
	if v == "" {
		return "non definie"
	}
	return v
}
