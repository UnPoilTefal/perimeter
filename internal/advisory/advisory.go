// Package advisory calcule l'avis ambiant : un resume passif, agrege par
// categorie, de ce que des detecteurs deja existants savent — la coherence
// du registre (Registry.Check), deux regles de lint qui signalent une
// derive structurelle du corpus (wikilink, staleness-budget), et la
// fiabilite des faits deja confirmes (reliability.Check). Aucune de ces
// categories n'est une nouvelle detection : ce paquet ne fait que les
// compter et les rendre lisibles en une ligne.
//
// Compute est une fonction pure, sans lecture disque : l'appelant charge
// le registre, le resultat de lint et l'index de fiabilite, et decide
// lui-meme de l'etat "indisponible" si l'un de ces chargements echoue —
// c'est cette decision-la, prise a l'accroche CLI, qui garantit que
// l'avis ambiant ne bloque jamais la sous-commande reellement demandee.
package advisory

import (
	"fmt"
	"time"

	"github.com/UnPoilTefal/perimeter/internal/perimeter"
	"github.com/UnPoilTefal/perimeter/internal/reliability"
	"github.com/UnPoilTefal/perimeter/internal/report"
)

// Advisory est l'avis ambiant. Zero partout, Unavailable faux : rien a
// signaler, le seul etat qui reste silencieux.
type Advisory struct {
	// Registre compte les Issue rendues par Registry.Check() — role non
	// pourvu, source absente, sonde manquante, credential suspect.
	Registre int
	// Liens compte les constats de la regle de lint "wikilink".
	Liens int
	// Peremption compte le constat agrege de la regle "staleness-budget" —
	// jamais les constats par-note "staleness", qui sont un nit editorial,
	// pas un signal de derive du perimetre.
	Peremption int
	// Faits compte les references confirmees dont reliability.Check rend
	// un verdict Stale ou Drifted.
	Faits int
	// Unavailable dit que le calcul lui-meme a echoue (registre, lint ou
	// index de fiabilite illisibles) — jamais confondu avec Empty().
	Unavailable bool
	Cause       string
}

// Empty dit si l'avis n'a rien a signaler. Un avis indisponible n'est
// jamais vide : le confondre avec le cas sain reproduirait exactement la
// degradation silencieuse que le reste du projet refuse deja.
func (a Advisory) Empty() bool {
	return !a.Unavailable && a.Registre == 0 && a.Liens == 0 && a.Peremption == 0 && a.Faits == 0
}

// Compute agrege les trois detecteurs en un avis. reg peut etre nil (un
// corpus resolu sans registre) : Registre et Faits retombent alors a
// zero, faute d'un registre pour les porter. entries peut etre nil quand
// aucun index de fiabilite n'existe encore.
func Compute(reg *perimeter.Registry, lintRes *report.Result, entries []reliability.Entry, root string, unprobedBudgetDays int, now time.Time) Advisory {
	a := Advisory{}

	if lintRes != nil {
		for _, f := range lintRes.Findings {
			switch f.Rule {
			case "wikilink":
				a.Liens++
			case "staleness-budget":
				a.Peremption++
			}
		}
	}

	if reg == nil {
		return a
	}
	a.Registre = len(reg.Check())

	seen := map[string]bool{}
	for _, e := range entries {
		if seen[e.Ref] {
			continue
		}
		seen[e.Ref] = true
		v := reliability.Check(entries, e.Ref, root, unprobedBudgetDays, now)
		if v.Stale || v.Drifted {
			a.Faits++
		}
	}
	return a
}

// Ligne rend la ligne stderr de l'avis ambiant, ou une chaine vide quand
// il n'y a rien a signaler — a l'appelant de ne rien ecrire dans ce cas.
func (a Advisory) Ligne() string {
	if a.Unavailable {
		return "avis ambiant indisponible : " + a.Cause
	}
	parts := a.categories()
	if len(parts) == 0 {
		return ""
	}
	out := "avis ambiant : "
	for i, p := range parts {
		if i > 0 {
			out += ", "
		}
		out += p
	}
	return out + " — perctl lint / perctl reliability check pour le detail"
}

// Warnings rend le meme contenu que Ligne, sous forme d'un tableau — le
// champ "warnings" ajoute aux sorties JSON qui le supportent deja. Rend
// nil quand l'avis est vide, pour que le champ s'omette proprement
// (`omitempty`).
func (a Advisory) Warnings() []string {
	if a.Unavailable {
		return []string{"avis ambiant indisponible : " + a.Cause}
	}
	parts := a.categories()
	if len(parts) == 0 {
		return nil
	}
	return parts
}

func (a Advisory) categories() []string {
	var parts []string
	if a.Registre > 0 {
		parts = append(parts, fmt.Sprintf("%d probleme(s) de registre", a.Registre))
	}
	if a.Liens > 0 {
		parts = append(parts, fmt.Sprintf("%d lien(s) casse(s)", a.Liens))
	}
	if a.Peremption > 0 {
		parts = append(parts, "corpus au-dela du plafond de peremption")
	}
	if a.Faits > 0 {
		parts = append(parts, fmt.Sprintf("%d fait(s) perime(s) ou derive(s)", a.Faits))
	}
	return parts
}
