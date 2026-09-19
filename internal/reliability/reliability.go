// Package reliability tient l'index de fiabilite d'un perimetre : un journal
// append-only qui annote un fait, ou qu'il vive, sans jamais le recopier ni
// modifier le fichier qui le porte.
//
// Ancre par empreinte, pas par contenu : chaque entree porte le hash de la
// ligne visee, pas la ligne elle-meme. Si le texte change, l'empreinte ne
// correspond plus — l'annotation s'invalide automatiquement, sans sonde
// dediee par type de source.
//
// Statut categoriel, jamais un score : "il n'y a pas de score" est un
// principe canon de ce projet, etabli trois fois sur la detection de
// recouvrement lexical. Une confirmation est preuve-scriptee, confirmee par
// un humain, ou sans signal — jamais un pourcentage de confiance.
package reliability

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// IndexFile est le nom du fichier d'index, toujours a cote du registre.
const IndexFile = "perimeter.reliability.jsonl"

// Status est le statut categoriel d'un fait. Derive, jamais stocke comme un
// champ independant de ce qui le prouve.
type Status string

const (
	// StatusPreuveScriptee : une sonde scriptee existe et a ete rejouee.
	StatusPreuveScriptee Status = "preuve-scriptee"
	// StatusConfirmeHumain : aucune sonde exploitable, mais un humain a
	// tranche contre une source du registre — jamais un agent seul.
	StatusConfirmeHumain Status = "confirme-humain"
	// StatusSansSignal : ni l'un ni l'autre. Etat legitime, pas une erreur.
	StatusSansSignal Status = "sans-signal"
)

// Entry est une ligne de l'index : une confirmation, jamais une recopie de
// la source qu'elle atteste.
type Entry struct {
	Ref             string `json:"ref"`
	ClaimHash       string `json:"claim_hash,omitempty"`
	Status          Status `json:"status"`
	ConfirmedAt     string `json:"confirmed_at,omitempty"`
	ConfirmedSource string `json:"confirmed_source,omitempty"`
	ConfirmedBy     string `json:"confirmed_by,omitempty"`
}

// IndexPath rend le chemin de l'index pour un registre donne — toujours a
// cote de perimeter.yml, jamais dans le corpus qu'il annote.
func IndexPath(registryPath string) string {
	return filepath.Join(filepath.Dir(registryPath), IndexFile)
}

// Load lit l'index. Un fichier absent n'est pas une erreur : un perimetre
// qui n'a encore rien confirme a un index vide, pas casse.
func Load(path string) ([]Entry, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []Entry
	for i, line := range strings.Split(strings.TrimRight(string(raw), "\n"), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var e Entry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			return nil, fmt.Errorf("%s:%d : ligne illisible : %w", path, i+1, err)
		}
		out = append(out, e)
	}
	return out, nil
}

// Latest rend la derniere entree pour une reference — l'index est un
// journal, jamais reecrit en place : l'etat courant est sa derniere ligne.
func Latest(entries []Entry, ref string) (Entry, bool) {
	var last Entry
	found := false
	for _, e := range entries {
		if e.Ref == ref {
			last = e
			found = true
		}
	}
	return last, found
}

// lineRefRe reconnait une reference ancree a une ligne : "chemin#L42". Sans
// cette forme, rien ne peut etre rehache — le verdict porte alors sur le
// statut et la fraicheur seules, jamais sur une supposition de format.
var lineRefRe = regexp.MustCompile(`^(.+)#L(\d+)$`)

// HashLine rend l'empreinte de la ligne visee par une reference — jamais son
// contenu : c'est une empreinte de changement, pas une recopie de la source.
// ok=false signifie « rien a comparer » (reference sans ancre de ligne),
// jamais une erreur.
func HashLine(root, ref string) (hash string, ok bool, err error) {
	m := lineRefRe.FindStringSubmatch(ref)
	if m == nil {
		return "", false, nil
	}
	lineNo, convErr := strconv.Atoi(m[2])
	if convErr != nil {
		return "", false, nil
	}
	f, openErr := os.Open(filepath.Join(root, m[1]))
	if errors.Is(openErr, os.ErrNotExist) {
		return "", true, nil // le fichier a disparu : rien a comparer, pas une erreur
	}
	if openErr != nil {
		return "", false, openErr
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for i := 1; sc.Scan(); i++ {
		if i == lineNo {
			sum := sha256.Sum256(sc.Bytes())
			return "sha256:" + hex.EncodeToString(sum[:]), true, nil
		}
	}
	return "", true, nil // la ligne n'existe plus : rien a comparer
}

// Verdict est ce qu'un agent recoit de Check : le statut, et s'il faut le
// prendre pour argent comptant ou revalider. L'agent decide a partir de ce
// seul verdict — jamais en jugeant lui-meme le fait.
type Verdict struct {
	Status  Status
	Found   bool
	Stale   bool
	Drifted bool
	Detail  string
}

// Check rend le verdict combine pour une reference : statut, fraicheur (vis
// a vis du palier non-sonde de la Tache 1), et derive du contenu vise. Root
// resout les references relatives (par convention, le repertoire du
// registre — la meme "tour de controle" qui sert deja a localiser l'index).
func Check(entries []Entry, ref, root string, unprobedBudgetDays int, now time.Time) Verdict {
	e, found := Latest(entries, ref)
	if !found {
		return Verdict{Status: StatusSansSignal, Detail: "aucune confirmation enregistree"}
	}
	v := Verdict{Status: e.Status, Found: true}

	if e.Status == StatusConfirmeHumain && unprobedBudgetDays > 0 && e.ConfirmedAt != "" {
		if t, err := time.Parse("2006-01-02", e.ConfirmedAt); err == nil {
			if now.After(t.AddDate(0, 0, unprobedBudgetDays)) {
				v.Stale = true
			}
		}
	}

	if e.ClaimHash != "" {
		if hash, ok, err := HashLine(root, e.Ref); err == nil && ok && hash != "" && hash != e.ClaimHash {
			v.Drifted = true
		}
	}

	switch {
	case v.Drifted:
		v.Detail = "le contenu vise a change depuis la confirmation — revalider"
	case v.Stale:
		v.Detail = fmt.Sprintf("confirme le %s, hors fenetre de fraicheur — revalider", e.ConfirmedAt)
	default:
		v.Detail = "a jour"
	}
	return v
}
