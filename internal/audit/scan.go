package audit

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// etatLigneRe reconnait la ligne « **etat**: <valeur> » d'un callout deja
// rendu par Render — la seule ligne dont Scan a besoin pour compter.
var etatLigneRe = regexp.MustCompile(`^> \*\*état\*\*: (.+)$`)

// ScanResult est le decompte des callouts de verification trouves dans un
// corpus externe.
type ScanResult struct {
	// Notes est le nombre de fichiers .md inspectes, qu'ils portent ou non
	// un callout — sert a distinguer un corpus vide d'un corpus non trouve.
	Notes int
	// Comptes indexe le nombre de callouts par etat rencontre. Une valeur
	// d'etat qui ne correspond a aucune des trois constantes connues est
	// quand meme comptee sous sa propre cle : un etat mal recopie a la main
	// doit se voir, pas disparaitre en silence.
	Comptes map[Etat]int
	// JamaisAudite est vrai quand aucun callout n'existe nulle part dans le
	// corpus — distinct d'un decompte a zero dans un etat donne.
	JamaisAudite bool
}

// Scan compte les callouts de verification dans les notes Markdown d'un
// repertoire de corpus externe. Non recursif : le vault homelab observe
// lors de #76 est plat, et une recursion n'apporterait rien qu'un terrain
// reel n'a pas demande.
func Scan(dir string) (ScanResult, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ScanResult{}, err
	}
	res := ScanResult{Comptes: map[Etat]int{}}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		res.Notes++
		raw, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return ScanResult{}, err
		}
		for _, l := range strings.Split(string(raw), "\n") {
			// TrimRight, pas TrimSpace : une note CRLF ne doit pas casser la
			// reconnaissance de la ligne, mais un espace final dans la
			// valeur elle-meme (rare, mais possible sur une saisie a la
			// main) doit rester visible tel quel pour ne pas masquer une
			// faute de frappe.
			l = strings.TrimRight(l, "\r")
			if m := etatLigneRe.FindStringSubmatch(l); m != nil {
				res.Comptes[Etat(m[1])]++
			}
		}
	}
	total := 0
	for _, n := range res.Comptes {
		total += n
	}
	res.JamaisAudite = total == 0
	return res, nil
}
