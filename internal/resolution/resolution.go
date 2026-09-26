// Package resolution decide quel corpus et quel registre analyser, selon la
// precedence commune a toutes les sous-commandes perctl : un chemin explicite
// dit *quel corpus*, un --perimeter explicite dit *quelle politique* — les
// deux se combinent. Sans chemin, le registre est cherche dans cet ordre :
// --perimeter, la variable PERIMETER, la remontee d'arborescence, puis
// l'emplacement utilisateur.
//
// Ni flag ni os.Exit ici : l'appelant (l'accroche CLI) decide seul du code de
// sortie et de l'affichage. Resolve rend ses notes ("PERIMETER ignoree") en
// warnings plutot que de les imprimer, pour rester appelable depuis un test
// sans detourner de flux global.
package resolution

import (
	"fmt"
	"os"

	"github.com/UnPoilTefal/perimeter/internal/corpus"
	"github.com/UnPoilTefal/perimeter/internal/perimeter"
)

const (
	// DesignationDrapeau est le conseil affiche quand la sous-commande
	// designe un registre par --perimeter — le cas courant.
	DesignationDrapeau = "indiquer un chemin avec --perimeter"
	// DesignationArgument est le conseil affiche par « perctl perimeter »,
	// seule sous-commande dont le positionnel EST le registre.
	DesignationArgument = "donner le registre en argument : « perctl perimeter <registre> »"
)

// ResolveRegistryPath cherche le chemin d'un registre — PERIMETER, la
// remontee d'arborescence depuis depuis, puis l'emplacement utilisateur — et
// rend, a defaut, le diagnostic qui dit ou l'outil a cherche. designation
// varie selon l'appelant : ce n'est pas le meme moyen qui le nomme selon que
// le positionnel designe un corpus ou le registre lui-meme.
func ResolveRegistryPath(depuis, designation string) (string, error) {
	found, provenance, ok := perimeter.Resoudre(depuis, perimeter.DossierUtilisateur())
	if ok {
		return found, nil
	}
	// Une piste explicite qui ne repond pas n'est pas ignoree : retomber en
	// silence sur un autre registre ferait travailler sur un perimetre que
	// l'utilisateur n'a pas nomme.
	if provenance == perimeter.EnvVar {
		return "", fmt.Errorf("%s pointe %q, qui n'existe pas — corriger la variable, ou l'effacer pour laisser la recherche se faire",
			perimeter.EnvVar, os.Getenv(perimeter.EnvVar))
	}
	return "", fmt.Errorf("%s", perimeter.Introuvable(depuis, perimeter.DossierUtilisateur(), designation))
}

// Resolve rend le corpus a analyser, et le registre qui a fourni sa
// politique (nil si aucun ne s'applique).
func Resolve(path, regPath string) (*corpus.Corpus, *perimeter.Registry, []string, error) {
	if path != "" {
		var warnings []string
		if regPath == "" {
			// Un registre seulement ambiant — PERIMETER, ou trouve en
			// remontant — ne s'applique pas a un corpus designe a la main :
			// il ferait piloter n'importe quel repertoire analyse au passage
			// par la politique d'un autre perimetre.
			if env := os.Getenv(perimeter.EnvVar); env != "" {
				warnings = append(warnings, fmt.Sprintf(
					"note : %s est definie, mais un chemin est donne — la politique du registre n'est pas appliquee\n      la demander explicitement : --perimeter %s",
					perimeter.EnvVar, env))
			}
			c, err := corpus.Load(path)
			return c, nil, warnings, err
		}
		reg, err := perimeter.Load(regPath)
		if err != nil {
			return nil, nil, nil, err
		}
		_, _, policy, err := reg.CorpusSource()
		if err != nil {
			return nil, nil, nil, err
		}
		c, err := corpus.LoadWith(path, corpus.ConfigFromPolicy(policy))
		return c, reg, nil, err
	}
	if regPath == "" {
		found, err := ResolveRegistryPath(".", DesignationDrapeau)
		if err != nil {
			return nil, nil, nil, err
		}
		regPath = found
	}
	reg, err := perimeter.Load(regPath)
	if err != nil {
		return nil, nil, nil, err
	}
	nom, root, policy, err := reg.CorpusSource()
	if err != nil {
		return nil, nil, nil, err
	}
	// Un corpus declare mais absent est le premier mur que rencontre un
	// nouvel utilisateur, juste apres un « perctl init » reussi. L'erreur de
	// la bibliotheque standard est exacte et muette sur la cause.
	if st, errStat := os.Stat(root); errStat != nil || !st.IsDir() {
		return nil, nil, nil, fmt.Errorf("%s", perimeter.CheminAbsent(nom, "contrainte.memoire", root))
	}
	c, err := corpus.LoadWith(root, corpus.ConfigFromPolicy(policy))
	return c, reg, nil, err
}
