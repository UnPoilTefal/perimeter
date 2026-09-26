package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/UnPoilTefal/perimeter/internal/audit"
)

// auditRefRe reconnait une reference ancree a une ligne : "chemin#L42" — la
// meme convention que internal/reliability, reprise ici pour designer une
// ligne precise sans repeter la question a l'utilisateur.
var auditRefRe = regexp.MustCompile(`^(.+)#L(\d+)$`)

// parseAuditRef separe le chemin de son ancre de ligne optionnelle. Sans
// ancre, hasLine est faux : l'appelant retombe sur le mode interactif, ligne
// par ligne.
func parseAuditRef(chemin string) (path string, lineNo int, hasLine bool) {
	m := auditRefRe.FindStringSubmatch(chemin)
	if m == nil {
		return chemin, 0, false
	}
	n, err := strconv.Atoi(m[2])
	if err != nil {
		return chemin, 0, false
	}
	return m[1], n, true
}

// cmdAudit ecrit un callout « [!verification]- » (etat « a valider ») dans une
// note non-schemee, sur les faits que l'utilisateur designe lui-meme — pas de
// seuil temporel ni de signal de contenu automatique en V1 (#78) : le vault
// audite n'a pas de note a l'abandon, la derive touche des faits ponctuels
// dans des notes actives, pas des fichiers oublies.
func cmdAudit(args []string) error {
	fs := flag.NewFlagSet("audit", flag.ExitOnError)
	origine := fs.String("origine", "", "origine du doute (sonde rejouee, ou motif) — requise avec une reference #Lnn, sinon demandee en mode interactif")
	chemin := positional(args)
	_ = fs.Parse(trimPositional(args))
	if chemin == "" {
		return fmt.Errorf("usage : perctl audit <chemin>[#Lnn] [--origine ...]")
	}

	path, lineNo, hasLine := parseAuditRef(chemin)
	now := time.Now()

	if hasLine {
		if *origine == "" {
			return fmt.Errorf("--origine est requis avec une reference de ligne (%s)", chemin)
		}
		return auditRefDirect(path, lineNo, *origine, now)
	}

	st, err := os.Stat(path)
	if err != nil {
		return err
	}
	r := bufio.NewReader(os.Stdin)
	if st.IsDir() {
		return auditDossierInteractif(r, os.Stdout, path, *origine, now)
	}
	return auditFichierInteractif(r, os.Stdout, path, *origine, now)
}

// auditRefDirect ecrit un callout a la ligne donnee, sans interaction — le
// chemin le plus direct quand l'utilisateur connait deja la ligne exacte,
// par exemple depuis un lien Obsidian ou un « chemin#L42 » recopie a la main.
func auditRefDirect(path string, lineNo int, origine string, now time.Time) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	out, err := audit.InsertAtLine(string(raw), lineNo, now.Format("2006-01-02"), origine, audit.AValider)
	if errors.Is(err, audit.ErrCalloutExiste) {
		// Idempotent plutot que planter : relancer la commande sur une ligne
		// deja marquee est une erreur d'usage benigne, pas un echec — et un
		// second callout ecraserait un etat deja tranche par un humain.
		fmt.Printf("%s#L%d : un callout est déjà présent — rien écrit\n", path, lineNo)
		return nil
	}
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(out), 0o644); err != nil {
		return err
	}
	fmt.Printf("%s#L%d : callout inséré\n", path, lineNo)
	return nil
}

// designation est une ligne pointee par l'utilisateur en mode interactif, en
// attente d'ecriture.
type designation struct {
	lineNo  int
	origine string
}

// auditFichierInteractif mene le dialogue sur une seule note : l'utilisateur
// pointe une ligne a la fois (Entree pour terminer), et le fichier n'est
// ecrit qu'une fois — en une seule passe, du plus grand numero de ligne au
// plus petit, pour qu'une insertion ne decale jamais les numeros de ligne
// que l'utilisateur vise ensuite dans la meme note.
func auditFichierInteractif(r *bufio.Reader, w io.Writer, path, origineDefaut string, now time.Time) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "%s\n", path) //nolint:errcheck // sortie terminal

	var pointes []designation
	for {
		fmt.Fprint(w, "  numéro de ligne à marquer (Entrée pour passer au suivant) : ") //nolint:errcheck // sortie terminal
		s, _ := r.ReadString('\n')
		s = strings.TrimSpace(s)
		if s == "" {
			break
		}
		n, err := strconv.Atoi(s)
		if err != nil {
			fmt.Fprintf(w, "  nombre attendu, pas %q\n", s) //nolint:errcheck // sortie terminal
			continue
		}
		origine := origineDefaut
		if origine == "" {
			fmt.Fprint(w, "  origine du doute : ") //nolint:errcheck // sortie terminal
			o, _ := r.ReadString('\n')
			origine = strings.TrimSpace(o)
		}
		pointes = append(pointes, designation{lineNo: n, origine: origine})
	}
	if len(pointes) == 0 {
		return nil
	}
	sort.Slice(pointes, func(i, j int) bool { return pointes[i].lineNo > pointes[j].lineNo })

	content := string(raw)
	date := now.Format("2006-01-02")
	ecrits := 0
	for _, p := range pointes {
		out, err := audit.InsertAtLine(content, p.lineNo, date, p.origine, audit.AValider)
		if errors.Is(err, audit.ErrCalloutExiste) {
			// Une ligne deja marquee ne doit pas faire echouer les autres
			// designations de la meme note : c'est une erreur d'usage
			// benigne, pas un motif d'abandon du reste du lot.
			fmt.Fprintf(w, "  ligne %d : un callout est déjà présent — ignorée\n", p.lineNo) //nolint:errcheck // sortie terminal
			continue
		}
		if err != nil {
			return fmt.Errorf("%s : %w", path, err)
		}
		content = out
		ecrits++
	}
	if ecrits == 0 {
		return nil
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return err
	}
	fmt.Fprintf(w, "  %d callout(s) écrit(s) dans %s\n", ecrits, path) //nolint:errcheck // sortie terminal
	return nil
}

// auditDossierInteractif applique auditFichierInteractif a chaque note
// directement sous le dossier — le vault vise est plat (44 notes, aucun
// sous-dossier), une recursion n'apporterait rien qu'un terrain reel n'a pas
// demande.
func auditDossierInteractif(r *bufio.Reader, w io.Writer, dir, origineDefaut string, now time.Time) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		if err := auditFichierInteractif(r, w, filepath.Join(dir, e.Name()), origineDefaut, now); err != nil {
			return err
		}
	}
	return nil
}
