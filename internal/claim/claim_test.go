package claim

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/UnPoilTefal/perimeter/internal/corpus"
	"github.com/UnPoilTefal/perimeter/internal/perimeter"
)

const registre = `
version: 1
roles:
  intention.spec:        { source: vault }
  intention.tickets:     { source: tickets }
  contrainte.decisions:  { source: vault }
  contrainte.memoire:    { source: memoire }
  etat.declare:          { source: depot }
  etat.reel:             { source: cluster }
sources:
  vault:    { adapter: files,  endpoint: "/srv/vault", reliability: declared, probe: { cmd: "true" } }
  tickets:
    adapter: github
    endpoint: "Org/produit"
    reliability: measured
    probe: { cmd: "true" }
    query: { cmd: "gh api repos/${endpoint}/issues/${arg} --jq .state" }
  tickets-jira:
    adapter: jira
    endpoint: "PROJ"
    reliability: measured
    probe: { cmd: "true" }
    query: { cmd: "acli jira workitem view ${arg}" }
  tickets-jira-court: { adapter: jira, endpoint: "OPS", reliability: measured, probe: { cmd: "true" }, query: { cmd: "acli jira workitem view ${arg}" } }
  memoire:  { adapter: files,  endpoint: ".",          reliability: declared, probe: { cmd: "true" } }
  depot:    { adapter: git,    endpoint: "/srv/repo",  reliability: declared, probe: { cmd: "true" } }
  cluster:  { adapter: shell,  endpoint: "ctx@app",    reliability: measured, probe: { cmd: "true" } }
`

func note(t *testing.T, dir, name, body string) {
	t.Helper()
	s := "---\nname: " + name +
		"\ndescription: \"une description assez longue pour satisfaire le schema du corpus\"\nmetadata:\n  type: reference\n  modified: 2026-09-01\n---\n\n" + body + "\n"
	if err := os.WriteFile(filepath.Join(dir, name+".md"), []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}

func setup(t *testing.T, notes map[string]string) (*corpus.Corpus, *perimeter.Registry) {
	t.Helper()
	dir := t.TempDir()
	for n, b := range notes {
		note(t, dir, n, b)
	}
	rp := filepath.Join(dir, perimeter.File)
	if err := os.WriteFile(rp, []byte(registre), 0o644); err != nil {
		t.Fatal(err)
	}
	reg, err := perimeter.Load(rp)
	if err != nil {
		t.Fatal(err)
	}
	c, err := corpus.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	return c, reg
}

func find(r *Result, note string) (Proposal, bool) {
	for _, p := range r.Proposals {
		if p.Note == note+".md" {
			return p, true
		}
	}
	return Proposal{}, false
}

// Test 1 — le typage vient de signaux structurels ancres, jamais du sens.
func TestAncrageAuRegistrePrimeSurLeStructurel(t *testing.T) {
	c, reg := setup(t, map[string]string{
		"reference-issue": "Corrige en produit#412, voir aussi /Users/x/ailleurs.md",
	})
	r := Propose(c, reg)
	p, ok := find(r, "reference-issue")
	if !ok {
		t.Fatal("une proposition etait attendue")
	}
	if p.Confidence != FromRegistry || p.Source != "tickets" {
		t.Errorf("ancrage registre attendu, obtenu %s/%s", p.Confidence, p.Source)
	}
	if !strings.Contains(p.Cmd, `arg: "412"`) {
		t.Errorf("le numero doit etre repris comme argument : %q", p.Cmd)
	}
}

// La cle de ticket porte le prefixe de projet declare par l'endpoint — pas
// un motif jira generique, qui ancrerait n'importe quel projet.
func TestUneCleDeTicketJiraSAncreAuRegistre(t *testing.T) {
	c, reg := setup(t, map[string]string{
		"reference-jira": "PROJ-1234 est une Feature du T4, toujours en cours.",
	})
	r := Propose(c, reg)
	p, ok := find(r, "reference-jira")
	if !ok {
		t.Fatal("une proposition etait attendue")
	}
	if p.Confidence != FromRegistry || p.Source != "tickets-jira" {
		t.Errorf("ancrage registre tickets-jira attendu, obtenu %s/%s", p.Confidence, p.Source)
	}
	if !strings.Contains(p.Cmd, `arg: "PROJ-1234"`) {
		t.Errorf("la cle complete doit etre reprise comme argument : %q", p.Cmd)
	}
}

// Le plancher de longueur d'ancrable protege les sources qui matchent une
// sous-chaine nue, mais l'ancre jira exige deja un « -<numero> » final : une
// cle de projet courte (OPS, HL...) doit s'ancrer malgre elle.
func TestUneCleDeProjetJiraCourteSAncreAuRegistre(t *testing.T) {
	c, reg := setup(t, map[string]string{
		"reference-jira-court": "OPS-7 est en cours de qualification.",
	})
	r := Propose(c, reg)
	p, ok := find(r, "reference-jira-court")
	if !ok {
		t.Fatal("une proposition etait attendue")
	}
	if p.Confidence != FromRegistry || p.Source != "tickets-jira-court" {
		t.Errorf("ancrage registre tickets-jira-court attendu, obtenu %s/%s", p.Confidence, p.Source)
	}
	if !strings.Contains(p.Cmd, `arg: "OPS-7"`) {
		t.Errorf("la cle complete doit etre reprise comme argument : %q", p.Cmd)
	}
}

// Une cle d'un autre projet ne doit jamais s'ancrer sur la source « PROJ » :
// l'ancre doit porter le prefixe exact de l'endpoint declare, pas un motif
// jira générique qui matcherait n'importe quel projet.
func TestUneCleDunAutreProjetNAncrePas(t *testing.T) {
	c, reg := setup(t, map[string]string{
		"reference-autre-projet": "AUTRE-99 concerne un produit different.",
	})
	r := Propose(c, reg)
	if p, ok := find(r, "reference-autre-projet"); ok {
		t.Errorf("aucune proposition attendue pour un projet different, obtenu %+v", p)
	}
}

// Le mot nu ne doit jamais ancrer : sur un corpus homelab, « produit »
// apparaitrait partout.
func TestLeMotNuNAncrePas(t *testing.T) {
	c, reg := setup(t, map[string]string{
		"reference-mot-nu": "On parle du produit en general, sans rien de precis.",
	})
	r := Propose(c, reg)
	if p, ok := find(r, "reference-mot-nu"); ok {
		t.Errorf("aucune proposition attendue, obtenue %s/%s sur %q", p.Confidence, p.Kind.Name, p.Match)
	}
}

// Test 2 — on ne construit jamais une sonde depuis une commande citee.
func TestUneCommandeCiteeNeDevientJamaisUneSonde(t *testing.T) {
	c, reg := setup(t, map[string]string{
		"reference-cmd": "Le correctif : `rm -rf /tmp/cache && ./deploy.sh --force`.",
	})
	r := Propose(c, reg)
	for _, p := range r.Proposals {
		for _, interdit := range []string{"rm -rf", "deploy.sh", "--force"} {
			if strings.Contains(p.Cmd, interdit) {
				t.Errorf("une commande citee a fuite dans une sonde : %q", p.Cmd)
			}
		}
	}
}

// Test 5 — une note qui porte deja une preuve n'est pas touchee.
func TestNoteDejaProuveeEstIgnoree(t *testing.T) {
	dir := t.TempDir()
	s := "---\nname: reference-prouvee\ndescription: \"une note portant deja sa propre preuve rejouable\"\nmetadata:\n  type: reference\n  modified: 2026-09-01\n  verify:\n    - cmd: \"true\"\n---\n\nCite produit#1.\n"
	if err := os.WriteFile(filepath.Join(dir, "reference-prouvee.md"), []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := corpus.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	r := Propose(c, nil)
	if r.DejaPreuve != 1 {
		t.Errorf("1 note deja prouvee attendue, comptee %d", r.DejaPreuve)
	}
	if len(r.Proposals) != 0 {
		t.Errorf("aucune proposition ne doit viser une note deja prouvee : %+v", r.Proposals)
	}
}

// Test 6 — sans registre, on retombe sur le structurel, jamais sur rien.
func TestSansRegistreLeStructurelPrend(t *testing.T) {
	c, _ := setup(t, map[string]string{
		"reference-chemin": "Le fichier ~/.config/Brewfile declare les paquets.",
	})
	r := Propose(c, nil)
	p, ok := find(r, "reference-chemin")
	if !ok {
		t.Fatal("une proposition structurelle etait attendue")
	}
	if p.Confidence != Structural || p.Kind != KindPath {
		t.Errorf("chemin/structurel attendu, obtenu %s/%s", p.Kind.Name, p.Confidence)
	}
	if !strings.Contains(p.Cmd, "test -e") {
		t.Errorf("sonde en lecture seule attendue : %q", p.Cmd)
	}
}

func TestNoteSansSignalEstListeeSansProposition(t *testing.T) {
	c, reg := setup(t, map[string]string{
		"feedback-gout": "Preferer les phrases courtes aux phrases longues.",
	})
	r := Propose(c, reg)
	if len(r.SansSignal) != 1 {
		t.Errorf("1 note sans signal attendue, obtenue %d", len(r.SansSignal))
	}
	if len(r.Proposals) != 0 {
		t.Errorf("aucune proposition ne doit etre inventee : %+v", r.Proposals)
	}
}

// Test 3 — les sondes sont ecrites en commentaire, jamais actives.
func TestLesSondesSontEcritesEnCommentaire(t *testing.T) {
	c, reg := setup(t, map[string]string{
		"reference-issue": "Corrige en produit#412.",
	})
	r := Propose(c, reg)
	n, err := Write(c, r.Proposals)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("1 note completee attendue, obtenue %d", n)
	}
	raw, err := os.ReadFile(filepath.Join(c.Root, "reference-issue.md"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	if !strings.Contains(s, "# sonde proposee") {
		t.Fatal("le bloc propose est absent")
	}
	for _, l := range strings.Split(s, "\n") {
		if strings.Contains(l, "verify:") && !strings.Contains(l, "#") {
			t.Errorf("une sonde est devenue active sans relecture : %q", l)
		}
	}
	// La note reste analysable, et toujours sans preuve.
	c2, err := corpus.Load(c.Root)
	if err != nil {
		t.Fatal(err)
	}
	for _, note := range c2.Notes {
		if note.ParseErr != nil {
			t.Fatalf("la note reecrite n'est plus analysable : %v", note.ParseErr)
		}
		if len(note.Metadata.Verify) != 0 {
			t.Error("la note ne doit pas etre devenue verifiable")
		}
	}
}

func TestEcrireDeuxFoisNeDoublePas(t *testing.T) {
	c, reg := setup(t, map[string]string{"reference-issue": "Corrige en produit#412."})
	r := Propose(c, reg)
	if _, err := Write(c, r.Proposals); err != nil {
		t.Fatal(err)
	}
	c2, _ := corpus.Load(c.Root)
	n, err := Write(c2, r.Proposals)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("une seconde passe ne doit rien reecrire, obtenu %d", n)
	}
}

// Chaque type porte une demi-vie : un chemin se perime plus vite qu'une
// convention.
func TestChaqueTypePorteUneDemiVie(t *testing.T) {
	for _, k := range []Kind{KindIssue, KindPath, KindEndpoint, KindRepo, KindHost, KindVersion, KindConvention} {
		if k.HalfLifeDays <= 0 {
			t.Errorf("le type %q n'a pas de demi-vie", k.Name)
		}
	}
	if KindPath.HalfLifeDays >= KindConvention.HalfLifeDays {
		t.Error("un chemin doit se perimer plus vite qu'une convention")
	}
}

func TestCouvertureCompteLesDeuxSources(t *testing.T) {
	c, reg := setup(t, map[string]string{
		"reference-issue":  "Corrige en produit#412.",
		"reference-chemin": "Voir ~/.config/Brewfile.",
		"feedback-gout":    "Preferer les phrases courtes.",
	})
	r := Propose(c, reg)
	if r.Total != 3 || len(r.Proposals) != 2 || len(r.SansSignal) != 1 {
		t.Fatalf("attendu 3/2/1, obtenu %d/%d/%d", r.Total, len(r.Proposals), len(r.SansSignal))
	}
	if cov := r.Coverage(); cov < 0.66 || cov > 0.67 {
		t.Errorf("couverture attendue ~0.667, obtenue %.3f", cov)
	}
}

// --- proximite du signal et sonde inversee (#21) ---

// Un signal loin de la revendication est du contexte, pas une assertion.
// Mesure sur le corpus de reference : sans cette contrainte, 45 notes portent
// un signal pour une precision de 20 % apres tri ; avec, 21 notes et ~38 %.
func TestUnSignalLoinDeLaRevendicationEstIgnore(t *testing.T) {
	loin := "Premier paragraphe sans aucun signal exploitable.\n\n" +
		"Deuxieme paragraphe qui cite ~/.config/Brewfile au passage."
	c, reg := setup(t, map[string]string{"reference-loin": loin})
	r := Propose(c, reg)
	if p, ok := find(r, "reference-loin"); ok {
		t.Errorf("aucune proposition attendue, obtenue %s sur %q", p.Kind.Name, p.Match)
	}
	if len(r.SansSignal) != 1 {
		t.Errorf("la note devait etre listee sans signal, obtenu %d", len(r.SansSignal))
	}
}

func TestUnSignalDansLePremierParagrapheCompte(t *testing.T) {
	c, reg := setup(t, map[string]string{
		"reference-proche": "Le fichier ~/.config/Brewfile declare les paquets.\n\nSuite sans signal.",
	})
	r := Propose(c, reg)
	if _, ok := find(r, "reference-proche"); !ok {
		t.Error("un signal dans le premier paragraphe doit fonder une proposition")
	}
}

// La description compte aussi : c'est la phrase qui enonce le fait.
func TestUnSignalDansLaDescriptionCompte(t *testing.T) {
	dir := t.TempDir()
	s := "---\nname: reference-desc\ndescription: \"Le fichier ~/.config/Brewfile declare tous les paquets installes\"\nmetadata:\n  type: reference\n  modified: 2026-09-01\n---\n\nParagraphe sans aucun signal.\n"
	if err := os.WriteFile(filepath.Join(dir, "reference-desc.md"), []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := corpus.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(Propose(c, nil).Proposals) != 1 {
		t.Error("un signal dans la description doit fonder une proposition")
	}
}

// Une note qui enonce une absence appelle une sonde inversee. « test -e » sur
// un chemin supprime echouerait alors meme que la note dit vrai.
func TestUneNoteQuiEnonceUneAbsenceInverseLaSonde(t *testing.T) {
	c, reg := setup(t, map[string]string{
		"reference-disparu": "Le chemin ~/Developer/sources/ancien n'existe plus. Il a ete supprime le 2026-08-15.",
	})
	p, ok := find(Propose(c, reg), "reference-disparu")
	if !ok {
		t.Fatal("une proposition etait attendue")
	}
	if !strings.HasPrefix(p.Cmd, `cmd: "! test -e`) {
		t.Errorf("sonde inversee attendue, obtenue %q", p.Cmd)
	}
	if !strings.Contains(p.Why, "inversee") {
		t.Errorf("la raison doit dire pourquoi : %q", p.Why)
	}
}

func TestUnePresenceNInversePas(t *testing.T) {
	c, reg := setup(t, map[string]string{
		"reference-present": "Le fichier ~/.config/Brewfile declare les paquets installes.",
	})
	p, _ := find(Propose(c, reg), "reference-present")
	if strings.Contains(p.Cmd, "! test") {
		t.Errorf("aucune inversion attendue : %q", p.Cmd)
	}
}

// La detection d'absence est locale a la phrase : une absence enoncee ailleurs
// dans la note ne doit pas inverser une sonde qui porte sur autre chose.
func TestLAbsenceEstLocaleALaPhrase(t *testing.T) {
	c, reg := setup(t, map[string]string{
		"reference-mixte": "Le fichier ~/.config/Brewfile declare les paquets. " +
			"L'ancien repertoire de builds n'existe plus.",
	})
	p, ok := find(Propose(c, reg), "reference-mixte")
	if !ok {
		t.Fatal("une proposition etait attendue")
	}
	if strings.Contains(p.Cmd, "! test") {
		t.Errorf("l'absence porte sur une autre phrase, pas d'inversion : %q", p.Cmd)
	}
}

// « jamais » et « aucun » visaient 24 notes sur 101 : trop large pour fonder
// une inversion, volontairement hors des marqueurs retenus.
func TestLesMarqueursLargesNInversentPas(t *testing.T) {
	for _, phrase := range []string{
		"Le fichier ~/.config/Brewfile ne doit jamais etre edite a la main.",
		"Aucun paquet n'est installe hors de ~/.config/Brewfile.",
	} {
		c, reg := setup(t, map[string]string{"reference-large": phrase})
		p, ok := find(Propose(c, reg), "reference-large")
		if !ok {
			t.Fatalf("proposition attendue pour %q", phrase)
		}
		if strings.Contains(p.Cmd, "! test") {
			t.Errorf("marqueur trop large, pas d'inversion attendue : %q", p.Cmd)
		}
	}
}

// --- motif d'adresse (#23) ---

// Un numero de version n'est pas une adresse. L'ancien motif rendait les
// octets du milieu optionnels et prenait « 10.5.67 » — la version d'OS d'une
// gateway — pour un hote a sonder.
func TestUnNumeroDeVersionNEstPasUneAdresse(t *testing.T) {
	for _, phrase := range []string{
		"Le bug de type 65 n'existe plus en 10.5.67.",
		"La version 10.5 corrige le comportement.",
		"Passage en v1.13.4 du schematic.",
	} {
		c, reg := setup(t, map[string]string{"reference-version": phrase})
		if p, ok := find(Propose(c, reg), "reference-version"); ok && p.Kind == KindHost {
			t.Errorf("%q : pris pour un hote (%q)", phrase, p.Match)
		}
	}
}

func TestUneVraieAdresseEstReconnue(t *testing.T) {
	c, reg := setup(t, map[string]string{
		"reference-hote": "Le NAS repond sur 192.168.40.50 en SSH.",
	})
	p, ok := find(Propose(c, reg), "reference-hote")
	if !ok || p.Kind != KindHost {
		t.Fatalf("hote attendu, obtenu %+v", p)
	}
	if p.Match != "192.168.40.50" {
		t.Errorf("adresse attendue 192.168.40.50, obtenue %q", p.Match)
	}
}

// Une adresse de reseau n'est pas une machine : la sonder echouerait
// systematiquement. Cas reel du corpus : le CIDR des pods Calico.
func TestUneAdresseDeReseauNEstPasUnHote(t *testing.T) {
	c, reg := setup(t, map[string]string{
		"reference-cidr": "Les pods vivent dans 10.244.0.0 par defaut.",
	})
	if p, ok := find(Propose(c, reg), "reference-cidr"); ok && p.Kind == KindHost {
		t.Errorf("une adresse de reseau ne doit pas fonder une sonde d'hote : %q", p.Match)
	}
}

func TestUnOctetHorsBorneEstEcarte(t *testing.T) {
	c, reg := setup(t, map[string]string{
		"reference-borne": "La valeur 192.168.999.1 n'est pas une adresse valide.",
	})
	if p, ok := find(Propose(c, reg), "reference-borne"); ok && p.Kind == KindHost {
		t.Errorf("octet hors borne accepte : %q", p.Match)
	}
}

// Une vraie adresse dans la meme zone qu'une version doit quand meme etre vue.
func TestUneAdresseCoexisteAvecUneVersion(t *testing.T) {
	c, reg := setup(t, map[string]string{
		"reference-mixte": "En 10.5.67, la gateway 192.168.40.254 intercepte le port 53.",
	})
	p, ok := find(Propose(c, reg), "reference-mixte")
	if !ok || p.Match != "192.168.40.254" {
		t.Errorf("l'adresse reelle devait etre retenue, obtenu %+v", p)
	}
}

// La sonde d'existence sur un chemin ne prouve que sa presence, jamais la
// revendication de contenu que la note porte reellement — meme motif que
// « sonde-d-existence » deja applique aux sources dans Maturite().
func TestUnCheminEstSignaleCommeSondeDExistence(t *testing.T) {
	c, reg := setup(t, map[string]string{
		"reference-chemin": "Le catalogue vit dans /Users/x/dev/sources/catalogue, une fiche par brique.",
	})
	r := Propose(c, reg)
	p, ok := find(r, "reference-chemin")
	if !ok {
		t.Fatal("une proposition etait attendue")
	}
	if p.Motif != "sonde-d-existence" {
		t.Errorf("motif sonde-d-existence attendu, obtenu %q", p.Motif)
	}
	if p.Consequence == "" {
		t.Error("la consequence doit expliquer ce que la sonde ne prouve pas")
	}
}

// Une note qui enonce une absence appelle une sonde inversee, qui prouve
// exactement la revendication (« ce chemin n'existe plus ») — pas de caveat
// a poser ici, contrairement au cas positif ci-dessus.
func TestUneAbsenceDeCheminNEstPasUneSondeDExistence(t *testing.T) {
	c, reg := setup(t, map[string]string{
		"reference-absence": "Le chemin /Users/x/dev/sources/ancien-repo a ete supprime du poste.",
	})
	r := Propose(c, reg)
	p, ok := find(r, "reference-absence")
	if !ok {
		t.Fatal("une proposition etait attendue")
	}
	if p.Motif != "" {
		t.Errorf("une sonde inversee prouve la revendication, aucun motif attendu, obtenu %q", p.Motif)
	}
	if !strings.HasPrefix(p.Cmd, `cmd: "! test -e`) {
		t.Errorf("sonde inversee attendue, obtenu %q", p.Cmd)
	}
}

// Le caveat doit survivre a l'ecriture en commentaire : un humain qui relit
// la note doit voir que la sonde ne prouve qu'une presence, pas la revendication.
func TestLaSondeDExistenceEstSignaleeDansLeCommentaire(t *testing.T) {
	c, reg := setup(t, map[string]string{
		"reference-chemin": "Le catalogue vit dans /Users/x/dev/sources/catalogue, une fiche par brique.",
	})
	r := Propose(c, reg)
	if _, err := Write(c, r.Proposals); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(c.Root, "reference-chemin.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "sonde-d-existence") {
		t.Errorf("le commentaire doit citer le motif sonde-d-existence :\n%s", raw)
	}
}
