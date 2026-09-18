package lint

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/UnPoilTefal/perimeter/internal/corpus"
	"github.com/UnPoilTefal/perimeter/internal/report"
)

// Ce fichier tient les tests d'acceptation de #58. Ils sont ecrits sur la
// sortie rendue, pas sur la liste des constats : c'est la sortie standard qui
// est le defaut — les constats, eux, sont corrects depuis le debut.

// corpusDerivant rend un corpus de n notes dont les d premieres portent une
// accroche d'index qui ne reprend pas leur description. C'est la forme du
// corpus mesure : un index ecrit par un processus qui ne derive pas les
// accroches, donc une divergence par entree, et aucune fautive en propre.
//
// Les trois premieres notes depassent en plus le budget de mots : il faut un
// second controle, bruyant lui aussi, pour verifier qu'il survit au repli du
// premier.
func corpusDerivant(t *testing.T, n, d int) *corpus.Corpus {
	t.Helper()
	files := map[string]string{}
	var index strings.Builder
	index.WriteString("# Index\n\n")
	for i := 0; i < n; i++ {
		nom := fmt.Sprintf("reference-note-%02d", i)
		desc := fmt.Sprintf("Le fait numero %d, enonce assez precisement pour decider du rappel", i)
		corps := "Un fait.\n"
		if i < 3 {
			corps = strings.Repeat("mot ", 500) + "\n"
		}
		files[nom+".md"] = "---\nname: " + nom + "\ndescription: \"" + desc + "\"\n" +
			"metadata:\n  type: reference\n  modified: 2026-09-01\n---\n\n" + corps
		accroche := desc
		if i < d {
			accroche = fmt.Sprintf("reformule a la main pour l'humain qui parcourt (%d)", i)
		}
		fmt.Fprintf(&index, "- [Reference: note %02d](%s.md) — %s\n", i, nom, accroche)
	}
	files["MEMORY.md"] = index.String()
	return corpusTemp(t, files)
}

func rendu(t *testing.T, c *corpus.Corpus) string {
	t.Helper()
	res, err := Run(c, Options{})
	if err != nil {
		t.Fatalf("lint : %v", err)
	}
	var b bytes.Buffer
	if err := res.Write(&b, "human", false); err != nil {
		t.Fatalf("rendu : %v", err)
	}
	return b.String()
}

// lignesDe rend les lignes de detail d'un controle : celles qui nomment un
// fichier sous son bloc.
func lignesDe(sortie, regle string) []string {
	var out []string
	dans := false
	for _, l := range strings.Split(sortie, "\n") {
		switch {
		case strings.HasPrefix(l, "! ") || strings.HasPrefix(l, "x "):
			dans = strings.HasPrefix(l, "! "+regle) || strings.HasPrefix(l, "x "+regle)
		case dans && strings.HasPrefix(l, "    "):
			out = append(out, strings.TrimSpace(l))
		}
	}
	return out
}

// A1 — le test d'acceptation de l'issue. Un controle qui atteint la totalite
// du corpus rend UN constat portant N/N, et les constats des autres controles
// restent visibles en sortie standard.
func TestControleTotalRendUnConstatDeProcessus(t *testing.T) {
	sortie := rendu(t, corpusDerivant(t, 12, 12))

	if n := len(lignesDe(sortie, "index-drift")); n != 0 {
		t.Errorf("index-drift atteint 12/12 : le detail par note doit se replier, %d lignes rendues", n)
	}
	if !strings.Contains(sortie, "12/12 notes") {
		t.Errorf("le constat doit porter la mesure « 12/12 notes », sortie :\n%s", sortie)
	}
	if !strings.Contains(sortie, "perctl index --sync") {
		t.Errorf("le constat doit porter sa remediation, sortie :\n%s", sortie)
	}
	// Le fond de l'issue : le controle bruyant ne doit pas emporter l'autre.
	if n := len(lignesDe(sortie, "atomicity")); n != 3 {
		t.Errorf("atomicity : 3 notes trop larges attendues en sortie standard, %d rendues\n%s", n, sortie)
	}
}

// A2 — le complement, au moins aussi important. Quand le controle ne touche
// qu'une minorite du corpus, l'identite des notes EST l'information : le
// detail reste.
func TestControleMinoritaireGardeLeDetailParNote(t *testing.T) {
	sortie := rendu(t, corpusDerivant(t, 12, 2))

	if n := len(lignesDe(sortie, "index-drift")); n != 2 {
		t.Errorf("index-drift touche 2 notes sur 12 : 2 lignes de detail attendues, %d rendues\n%s", n, sortie)
	}
	if strings.Contains(sortie, "2/12 notes") {
		t.Errorf("une minorite ne se replie pas en constat de processus, sortie :\n%s", sortie)
	}
}

// A3 — le taux seul ne suffit pas. atomicity atteint ici 100 % du corpus, et
// doit malgre tout rester detaille : sa remediation n'est pas unique, chaque
// note demande son propre arbitrage. C'est la remediation, pas le nombre, qui
// autorise le repli.
func TestControleSansRemediationUniqueResteDetailleA100(t *testing.T) {
	files := map[string]string{}
	var index strings.Builder
	index.WriteString("# Index\n\n")
	for i := 0; i < 12; i++ {
		nom := fmt.Sprintf("reference-large-%02d", i)
		desc := fmt.Sprintf("Le fait numero %d, enonce assez precisement pour decider du rappel", i)
		files[nom+".md"] = "---\nname: " + nom + "\ndescription: \"" + desc + "\"\n" +
			"metadata:\n  type: reference\n  modified: 2026-09-01\n---\n\n" + strings.Repeat("mot ", 500) + "\n"
		fmt.Fprintf(&index, "- [Reference: large %02d](%s.md) — %s\n", i, nom, desc)
	}
	files["MEMORY.md"] = index.String()

	sortie := rendu(t, corpusTemp(t, files))
	if n := len(lignesDe(sortie, "atomicity")); n != 12 {
		t.Errorf("atomicity a 12/12 n'a pas de remediation unique : 12 lignes attendues, %d rendues\n%s", n, sortie)
	}
	if strings.Contains(sortie, "12/12 notes") {
		t.Errorf("atomicity ne doit pas se replier en constat de processus, sortie :\n%s", sortie)
	}
}

// A4 — en regime authored, index-drift ne se declenche pas : il ne doit donc
// suggerer aucune remediation. « index --sync » y refuse d'ecrire, et on ne
// propose jamais une commande que la politique du corpus interdit.
func TestAucuneRemediationSuggereeEnRegimeAuthored(t *testing.T) {
	c := corpusDerivant(t, 12, 12)
	c.Config.Policy.IndexHook = corpus.IndexHookAuthored

	sortie := rendu(t, c)
	if strings.Contains(sortie, "index --sync") {
		t.Errorf("regime authored : « index --sync » refuse d'ecrire, il ne doit pas etre suggere\n%s", sortie)
	}
	if strings.Contains(sortie, "index-drift") {
		t.Errorf("regime authored : index-drift ne se declenche pas\n%s", sortie)
	}
	// Le reste du rapport n'est pas affecte.
	if n := len(lignesDe(sortie, "atomicity")); n != 3 {
		t.Errorf("atomicity doit rester rendu en authored : 3 lignes attendues, %d\n%s", n, sortie)
	}
}

var _ = report.Warn
