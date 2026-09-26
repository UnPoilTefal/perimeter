package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/UnPoilTefal/perimeter/internal/audit"
	"github.com/UnPoilTefal/perimeter/internal/coverage"
	"github.com/UnPoilTefal/perimeter/internal/perimeter"
)

// #85 — chemin heureux : registre complet, aucun corpus externe declare.
// Le chemin qui echoue (Bloquant()) se teste dans internal/coverage : le
// faire passer par cmdCoverage tuerait le binaire de test, fail() appelant
// os.Exit — meme raison que pour cmdGate (voir preconditions_test.go).
func TestCoverageRegistreSainSansCorpusExterne(t *testing.T) {
	regPath := posePerimetre(t, filepath.Join(t.TempDir(), "perimeter.yml"))

	sortie, err := sortieDe(t, func() error {
		return cmdCoverage([]string{"--perimeter", regPath})
	})
	if err != nil {
		t.Fatalf("coverage: %v", err)
	}
	if !strings.Contains(sortie, "six roles pourvus") {
		t.Errorf("attendu la confirmation des six roles, obtenu : %q", sortie)
	}
}

// #85 (revue de code) — un role pourvu mais dont la source n'est pas
// univoque (deux sources qualifiees pour contrainte.memoire, cf. #72) ne
// doit jamais retomber en silence sur « lintRes = nil » : c'est distinct du
// cas legitime ou aucun role n'est pourvu du tout.
func TestResoudreCorpusSchemeSignaleUnCorpusNonUnivoque(t *testing.T) {
	dir := t.TempDir()
	regPath := filepath.Join(dir, "perimeter.yml")
	memoire := filepath.Join(dir, "memoire")
	if err := os.MkdirAll(memoire, 0o755); err != nil {
		t.Fatal(err)
	}
	ecrire(t, regPath, `version: 1
roles:
  intention.spec:       { source: m }
  intention.tickets:    { source: m }
  contrainte.decisions: { source: m }
  contrainte.memoire:
    - { source: m,  qualifier: prod }
    - { source: m2, qualifier: recette }
  etat.declare:         { source: m }
  etat.reel:            { source: m }
sources:
  m:
    adapter: files
    reliability: declared
    endpoint: "`+memoire+`"
    probe: { cmd: "true" }
  m2:
    adapter: files
    reliability: declared
    endpoint: "`+memoire+`"
    probe: { cmd: "true" }
`)
	reg, err := perimeter.Load(regPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	lintRes, corpusErreur := resoudreCorpusScheme(reg)
	if lintRes != nil {
		t.Errorf("lintRes attendu nil quand la resolution echoue, obtenu %+v", lintRes)
	}
	if corpusErreur == "" {
		t.Error("attendu un message d'erreur pour un corpus non univoque, obtenu vide")
	}
}

// Le cas legitime : aucun role n'est pourvu du tout (registre dedie aux
// seuls corpus externes) — ni erreur, ni lint, silencieusement.
func TestResoudreCorpusSchemeSansAucunRolePourvu(t *testing.T) {
	regPath := filepath.Join(t.TempDir(), "perimeter.yml")
	ecrire(t, regPath, "version: 1\nroles: {}\nsources:\n  s: { adapter: files, reliability: declared, probe: { cmd: \"true\" } }\ncorpus_externes:\n  - { chemin: \"../vault\", type: obsidian }\n")
	reg, err := perimeter.Load(regPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	lintRes, corpusErreur := resoudreCorpusScheme(reg)
	if lintRes != nil || corpusErreur != "" {
		t.Errorf("aucun role pourvu est legitime : attendu (nil, \"\"), obtenu (%+v, %q)", lintRes, corpusErreur)
	}
}

// Un corpus externe dont le repertoire existe mais dont le scan echoue
// (fichier illisible) doit se signaler comme une erreur de scan, jamais
// comme un chemin introuvable — les deux causes appellent des correctifs
// differents.
// #85 (revue de code, 2e passe) — un chemin qui existe mais n'est pas un
// dossier (une note pointee au lieu de son dossier) n'est pas « introuvable »
// : le confondre orienterait l'utilisateur vers le mauvais correctif (creer
// un chemin absent, plutot que corriger un chemin mal designe).
func TestScannerCorpusExterneCheminPasUnDossier(t *testing.T) {
	dir := t.TempDir()
	regPath := filepath.Join(dir, "perimeter.yml")
	ecrire(t, regPath, "version: 1\nroles: {}\nsources:\n  s: { adapter: files, reliability: declared, probe: { cmd: \"true\" } }\n")
	reg, err := perimeter.Load(regPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	fichier := filepath.Join(dir, "pas-un-dossier.md")
	ecrire(t, fichier, "contenu")

	entry := scannerCorpusExterne(reg, perimeter.CorpusExterne{Chemin: "pas-un-dossier.md", Type: "obsidian"})
	if entry.Introuvable {
		t.Error("le chemin existe (c'est un fichier, pas un dossier) : Introuvable doit rester faux")
	}
	if entry.Erreur == "" {
		t.Error("attendu un message d'erreur distinct pour un chemin qui n'est pas un dossier")
	}
}

func TestScannerCorpusExterneDistingueErreurEtIntrouvable(t *testing.T) {
	dir := t.TempDir()
	regPath := filepath.Join(dir, "perimeter.yml")
	ecrire(t, regPath, "version: 1\nroles: {}\nsources:\n  s: { adapter: files, reliability: declared, probe: { cmd: \"true\" } }\n")
	reg, err := perimeter.Load(regPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	vault := filepath.Join(dir, "vault")
	if err := os.MkdirAll(vault, 0o755); err != nil {
		t.Fatal(err)
	}
	fichier := filepath.Join(vault, "illisible.md")
	ecrire(t, fichier, "contenu")
	if err := os.Chmod(fichier, 0o000); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chmod(fichier, 0o644) }()

	entry := scannerCorpusExterne(reg, perimeter.CorpusExterne{Chemin: "vault", Type: "obsidian"})
	if entry.Introuvable {
		t.Error("le repertoire existe : Introuvable doit rester faux")
	}
	if entry.Erreur == "" {
		t.Error("attendu un message d'erreur de scan, obtenu vide")
	}
}

// #85 (revue de code) — un etat mal recopie a la main (faute de frappe) ne
// doit pas disparaitre du rapport, meme s'il ne correspond a aucune des
// trois constantes reconnues : audit.Scan le compte deja sous sa propre
// cle, ecrireCoverage ne doit pas le filtrer en silence.
func TestEcrireComptesAfficheLesEtatsNonReconnus(t *testing.T) {
	fichier := filepath.Join(t.TempDir(), "sortie.txt")
	h, err := os.Create(fichier)
	if err != nil {
		t.Fatal(err)
	}
	ecrireComptes(h, map[audit.Etat]int{audit.AValider: 1, "à confirmer": 1})
	_ = h.Close()
	raw, err := os.ReadFile(fichier)
	if err != nil {
		t.Fatal(err)
	}
	out := string(raw)
	if !strings.Contains(out, "à confirmer") {
		t.Errorf("un etat non reconnu doit quand meme apparaitre dans le rapport, obtenu : %q", out)
	}
}

// #85 (revue de code, 2e passe) — quand contrainte.memoire renvoie a une
// source absente, Registry.Check() le rapporte deja comme un constat sur le
// registre ; resoudreCorpusScheme rapporterait la meme cause via
// CorpusErreur. ecrireCoverage ne doit pas montrer deux fois la meme
// defaillance — on construit le rapport a la main plutot que de passer par
// cmdCoverage, dont l'echec appellerait fail() -> os.Exit et tuerait le
// process de test (meme raison que pour cmdGate).
func TestCoverageNAffichePasDeuxFoisLaMemeDefaillance(t *testing.T) {
	rep := coverage.Report{
		RoleIssues: []perimeter.Issue{{
			Role:    perimeter.CorpusRole,
			Message: `renvoie a la source "manquante", absente du registre`,
		}},
		CorpusErreur: `perimeter.yml : le role "contrainte.memoire" renvoie a la source "manquante", absente`,
	}

	fichier := filepath.Join(t.TempDir(), "sortie.txt")
	h, err := os.Create(fichier)
	if err != nil {
		t.Fatal(err)
	}
	ecrireCoverage(h, "perimeter.yml", rep)
	_ = h.Close()
	raw, err := os.ReadFile(fichier)
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(raw), "manquante"); n != 1 {
		t.Errorf("la source manquante doit apparaitre une seule fois dans le rapport, obtenu %d fois :\n%s", n, raw)
	}
}

func TestCoverageRapporteUnCorpusExterneJamaisAudite(t *testing.T) {
	dir := t.TempDir()
	regPath := posePerimetre(t, filepath.Join(dir, "registre", "perimeter.yml"))
	vault := filepath.Join(dir, "vault")
	if err := os.MkdirAll(vault, 0o755); err != nil {
		t.Fatal(err)
	}
	ecrire(t, filepath.Join(vault, "note.md"), "# Une note\n\nUn fait quelconque.\n")

	raw, err := os.ReadFile(regPath)
	if err != nil {
		t.Fatal(err)
	}
	corpusExternesBloc := "\ncorpus_externes:\n  - { chemin: \"" + vault + "\", type: obsidian }\n"
	if err := os.WriteFile(regPath, append(raw, []byte(corpusExternesBloc)...), 0o644); err != nil {
		t.Fatal(err)
	}

	sortie, err := sortieDe(t, func() error {
		return cmdCoverage([]string{"--perimeter", regPath})
	})
	if err != nil {
		t.Fatalf("coverage: %v", err)
	}
	if !strings.Contains(sortie, "jamais audite") {
		t.Errorf("attendu le signal « jamais audite », obtenu : %q", sortie)
	}
}
