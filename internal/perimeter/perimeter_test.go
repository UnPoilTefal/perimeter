package perimeter

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// write depose un registre temporaire et rend son chemin.
func write(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), File)
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

const complet = `
version: 1
roles:
  intention.spec:        { source: specs }
  intention.tickets:     { source: tickets }
  contrainte.decisions:  { source: adr }
  contrainte.memoire:    { source: memoire }
  etat.declare:          { source: depots }
  etat.reel:             { source: cluster }
sources:
  specs:    { adapter: files,  reliability: measured, probe: { cmd: "true" } }
  tickets:  { adapter: github, reliability: measured, endpoint: "org/produit", credential: "env:GITHUB_TOKEN",
              probe: { cmd: "echo ${endpoint}" }, query: { cmd: "echo ${endpoint} ${arg}" } }
  adr:      { adapter: git,    reliability: declared, probe: { cmd: "true" } }
  memoire:  { adapter: files,  reliability: declared, probe: { cmd: "true" } }
  depots:   { adapter: git,    reliability: declared, probe: { cmd: "true" } }
  cluster:  { adapter: http,   reliability: measured, probe: { cmd: "true" } }
`

func issueText(is []Issue) string {
	var b strings.Builder
	for _, i := range is {
		b.WriteString(i.String())
		b.WriteString("\n")
	}
	return b.String()
}

// Test d'acceptation 1 — un role non pourvu est une carence nommee, pas un
// silence. C'est la condition E1 du demarrage.
func TestRoleNonPourvuEstUnConstat(t *testing.T) {
	body := strings.Replace(complet, "  etat.reel:             { source: cluster }\n", "", 1)
	reg, err := Load(write(t, body))
	if err != nil {
		t.Fatalf("chargement : %v", err)
	}
	got := issueText(reg.Check())
	if !strings.Contains(got, "etat.reel") || !strings.Contains(got, "non pourvu") {
		t.Errorf("le role manquant devait etre nomme, obtenu :\n%s", got)
	}
}

func TestRoleRenvoyantAUneSourceAbsente(t *testing.T) {
	body := strings.Replace(complet, "etat.reel:             { source: cluster }", "etat.reel:             { source: fantome }", 1)
	reg, err := Load(write(t, body))
	if err != nil {
		t.Fatal(err)
	}
	got := issueText(reg.Check())
	if !strings.Contains(got, "fantome") {
		t.Errorf("la source inexistante devait etre nommee, obtenu :\n%s", got)
	}
}

func TestRegistreCompletNeProduitAucunConstat(t *testing.T) {
	reg, err := Load(write(t, complet))
	if err != nil {
		t.Fatal(err)
	}
	if is := reg.Check(); len(is) != 0 {
		t.Errorf("registre complet, constats inattendus :\n%s", issueText(is))
	}
}

// Test d'acceptation 2 — une source sans sonde se degrade en silence.
func TestSourceSansSondeEstRefusee(t *testing.T) {
	body := strings.Replace(complet, `specs:    { adapter: files,  reliability: measured, probe: { cmd: "true" } }`,
		`specs:    { adapter: files,  reliability: measured }`, 1)
	if _, err := Load(write(t, body)); err == nil {
		t.Fatal("une source sans sonde doit etre refusee par le schema")
	}
}

// Test d'acceptation 6 — un adaptateur inconnu echoue nommement.
func TestAdaptateurInconnuEstNomme(t *testing.T) {
	body := strings.Replace(complet, "adapter: http,   reliability: measured", "adapter: telepathie, reliability: measured", 1)
	_, err := Load(write(t, body))
	if err == nil {
		t.Fatal("un adaptateur inconnu doit etre refuse")
	}
	if !strings.Contains(err.Error(), "adapter") {
		t.Errorf("l'erreur doit designer le champ fautif, obtenu : %v", err)
	}
}

// Test d'acceptation 4 — le registre ne porte que des references de
// credential, jamais des valeurs.
func TestCredentialSansSchemeEstRefuse(t *testing.T) {
	body := strings.Replace(complet, `credential: "env:GITHUB_TOKEN"`, `credential: "ghp_AAAABBBBCCCCDDDDEEEEFFFFGGGG1234"`, 1)
	if _, err := Load(write(t, body)); err == nil {
		t.Fatal("un credential sans scheme de reference doit etre refuse")
	}
}

func TestCredentialAvecSchemeMaisValeurEstSignale(t *testing.T) {
	body := strings.Replace(complet, `credential: "env:GITHUB_TOKEN"`, `credential: "env:ghp_AAAABBBBCCCCDDDDEEEEFFFFGGGG1234"`, 1)
	reg, err := Load(write(t, body))
	if err != nil {
		t.Fatalf("le scheme est valide, le chargement doit passer : %v", err)
	}
	got := issueText(reg.Check())
	if !strings.Contains(got, "reference") {
		t.Errorf("une valeur deguisee en reference devait etre signalee, obtenu :\n%s", got)
	}
}

// Test d'acceptation 5 — la boite ne recopie aucune source. L'invariant est
// applique par le schema : il n'existe aucun champ ou declarer une copie.
func TestChampDeCopieLocaleEstRefuseParLeSchema(t *testing.T) {
	for _, champ := range []string{"cache", "mirror", "sync", "local_copy"} {
		body := strings.Replace(complet, `adr:      { adapter: git,    reliability: declared, probe: { cmd: "true" } }`,
			`adr:      { adapter: git, reliability: declared, `+champ+`: /tmp/adr, probe: { cmd: "true" } }`, 1)
		if _, err := Load(write(t, body)); err == nil {
			t.Errorf("le champ %q declare une copie locale et doit etre refuse", champ)
		}
	}
}

func TestSourceOrphelineEstSignalee(t *testing.T) {
	body := strings.TrimRight(complet, "\n") + "\n  orpheline: { adapter: files, reliability: declared, probe: { cmd: \"true\" } }\n"
	reg, err := Load(write(t, body))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(issueText(reg.Check()), "orpheline") {
		t.Error("une source rattachee a aucun role devait etre signalee")
	}
}

// Resolve substitue endpoint et arg, et refuse nommement ce qu'il ne sait pas faire.
func TestResolveSubstitueEndpointEtArg(t *testing.T) {
	reg, err := Load(write(t, complet))
	if err != nil {
		t.Fatal(err)
	}
	c, err := reg.Resolve("tickets", "42")
	if err != nil {
		t.Fatal(err)
	}
	if c.Cmd != "echo org/produit 42" {
		t.Errorf("substitution attendue \"echo org/produit 42\", obtenue %q", c.Cmd)
	}
}

func TestResolveSourceInconnueEstNommee(t *testing.T) {
	reg, _ := Load(write(t, complet))
	_, err := reg.Resolve("fantome", "")
	if err == nil || !strings.Contains(err.Error(), "fantome") {
		t.Errorf("la source inconnue doit etre nommee, obtenu : %v", err)
	}
}

func TestResolveSansInterrogationEstNomme(t *testing.T) {
	reg, _ := Load(write(t, complet))
	_, err := reg.Resolve("adr", "")
	if err == nil || !strings.Contains(err.Error(), "adr") {
		t.Errorf("une source sans query doit etre refusee nommement, obtenu : %v", err)
	}
}

func TestProbeSubstitueEndpoint(t *testing.T) {
	reg, _ := Load(write(t, complet))
	c, err := reg.ProbeOf("tickets")
	if err != nil {
		t.Fatal(err)
	}
	if c.Cmd != "echo org/produit" {
		t.Errorf("sonde attendue \"echo org/produit\", obtenue %q", c.Cmd)
	}
}

// L'exemple livre doit rester valide : c'est la documentation executable du format.
func TestExempleLivreEstCoherent(t *testing.T) {
	reg, err := Load(filepath.Join("..", "..", "examples", "perimeter.yml"))
	if err != nil {
		t.Fatalf("l'exemple doit charger : %v", err)
	}
	if is := reg.Check(); len(is) != 0 {
		t.Errorf("l'exemple doit etre coherent :\n%s", issueText(is))
	}
}

// --- configuration unique : le registre porte la politique du corpus ---

const avecCorpus = `
version: 1
roles:
  intention.spec:        { source: specs }
  intention.tickets:     { source: tickets }
  contrainte.decisions:  { source: adr }
  contrainte.memoire:    { source: memoire }
  etat.declare:          { source: depots }
  etat.reel:             { source: cluster }
sources:
  specs:    { adapter: files,  reliability: measured, probe: { cmd: "true" } }
  tickets:  { adapter: github, reliability: measured, probe: { cmd: "true" } }
  adr:      { adapter: git,    reliability: declared, probe: { cmd: "true" } }
  memoire:
    adapter: files
    endpoint: notes
    reliability: declared
    probe: { cmd: "true" }
    corpus:
      index: INDEX.md
      max_body_words: 250
      require_owner: true
      staleness: { review_after_days: 90, max_stale_ratio: 0.2, probed_review_after_days: 15, unprobed_review_after_days: 200 }
      links: { ignore_prefixes: ["/", "@"] }
  depots:   { adapter: git,    reliability: declared, probe: { cmd: "true" } }
  cluster:  { adapter: http,   reliability: measured, probe: { cmd: "true" } }
`

// Test d'acceptation 1 — la politique du corpus vit au registre.
func TestLeRegistrePorteLaPolitiqueDuCorpus(t *testing.T) {
	reg, err := Load(write(t, avecCorpus))
	if err != nil {
		t.Fatal(err)
	}
	if is := reg.Check(); len(is) != 0 {
		t.Fatalf("registre attendu coherent :\n%s", issueText(is))
	}
	name, root, pol, err := reg.CorpusSource()
	if err != nil {
		t.Fatal(err)
	}
	if name != "memoire" {
		t.Errorf("source attendue \"memoire\", obtenue %q", name)
	}
	// Le chemin est resolu relativement au registre, pas au repertoire courant.
	if want := filepath.Join(filepath.Dir(reg.Path), "notes"); root != want {
		t.Errorf("racine attendue %q, obtenue %q", want, root)
	}
	if pol == nil || pol.Index == nil || *pol.Index != "INDEX.md" || pol.MaxBodyWords != 250 || !pol.RequireOwner {
		t.Errorf("politique mal lue : %+v", pol)
	}
	if pol.Staleness.ReviewAfterDays != 90 || len(pol.Links.IgnorePrefixes) != 2 {
		t.Errorf("sous-blocs de politique mal lus : %+v", pol)
	}
	if pol.Staleness.ProbedReviewAfterDays != 15 || pol.Staleness.UnprobedReviewAfterDays != 200 {
		t.Errorf("paliers de staleness mal lus : %+v", pol.Staleness)
	}
}

// Test d'acceptation 5 — une politique invalide est refusee, pas ignoree.
func TestPolitiqueDeCorpusInvalideEstRefusee(t *testing.T) {
	body := strings.Replace(avecCorpus, "max_body_words: 250", "max_body_words: -3", 1)
	if _, err := Load(write(t, body)); err == nil {
		t.Fatal("un budget de mots negatif doit etre refuse par le schema")
	}
	body = strings.Replace(avecCorpus, "index: INDEX.md", "indice: INDEX.md", 1)
	if _, err := Load(write(t, body)); err == nil {
		t.Fatal("un champ inconnu dans la politique doit etre refuse")
	}
}

func TestRoleDeCorpusNonPourvuEstNomme(t *testing.T) {
	body := strings.Replace(avecCorpus, "  contrainte.memoire:    { source: memoire }\n", "", 1)
	reg, err := Load(write(t, body))
	if err != nil {
		t.Fatal(err)
	}
	_, _, _, err = reg.CorpusSource()
	if err == nil || !strings.Contains(err.Error(), CorpusRole) {
		t.Errorf("le role manquant doit etre nomme, obtenu : %v", err)
	}
}

// Test d'acceptation 3 — le registre se trouve en remontant l'arborescence.
func TestFindRemonteLArborescence(t *testing.T) {
	racine := t.TempDir()
	if err := os.WriteFile(filepath.Join(racine, File), []byte(avecCorpus), 0o644); err != nil {
		t.Fatal(err)
	}
	profond := filepath.Join(racine, "a", "b", "c")
	if err := os.MkdirAll(profond, 0o755); err != nil {
		t.Fatal(err)
	}
	got, ok := Find(profond)
	if !ok {
		t.Fatal("le registre devait etre trouve en remontant")
	}
	// TempDir peut passer par un lien symbolique : comparer les cibles.
	want, _ := filepath.EvalSymlinks(filepath.Join(racine, File))
	gotr, _ := filepath.EvalSymlinks(got)
	if gotr != want {
		t.Errorf("registre attendu %q, obtenu %q", want, gotr)
	}
}

func TestFindEchoueSansRegistre(t *testing.T) {
	if _, ok := Find(t.TempDir()); ok {
		t.Error("aucun registre ne doit etre trouve dans un repertoire vide")
	}
}

// Tests d'acceptation 6 et 8 — le gabarit d'init est valide et coherent.
func TestGabaritDInitEstValideEtCoherent(t *testing.T) {
	reg, err := Load(write(t, Template()))
	if err != nil {
		t.Fatalf("le gabarit doit charger : %v", err)
	}
	if is := reg.Check(); len(is) != 0 {
		t.Errorf("le gabarit doit passer la validation :\n%s", issueText(is))
	}
	if _, _, pol, err := reg.CorpusSource(); err != nil || pol == nil {
		t.Errorf("le gabarit doit declarer une politique de corpus : %v / %+v", err, pol)
	}
	for _, h := range RoleHints {
		if _, ok := reg.RoleMap[h.Role]; !ok {
			t.Errorf("le gabarit ne pourvoit pas %q", h.Role)
		}
	}
}

func TestChaqueAdaptateurProposeUneSonde(t *testing.T) {
	for _, a := range Adapters {
		if !strings.Contains(ProbeFor(a, "x"), "cmd:") {
			t.Errorf("l'adaptateur %q doit proposer une sonde", a)
		}
	}
}

// --- plusieurs sources pour un role (#27) ---

const multiEnv = `
version: 1
roles:
  intention.spec:        { source: specs }
  intention.tickets:     { source: tickets }
  contrainte.decisions:  { source: adr }
  contrainte.memoire:    { source: memoire }
  etat.declare:          { source: depots }
  etat.reel:
    - { source: prod,    qualifier: production }
    - { source: recette, qualifier: recette }
sources:
  specs:    { adapter: files,  reliability: measured, probe: { cmd: "true" } }
  tickets:  { adapter: github, reliability: measured, probe: { cmd: "true" } }
  adr:      { adapter: git,    reliability: declared, probe: { cmd: "true" } }
  memoire:  { adapter: files,  endpoint: notes, reliability: declared, probe: { cmd: "true" } }
  depots:   { adapter: git,    reliability: declared, probe: { cmd: "true" } }
  prod:     { adapter: http,   reliability: measured, probe: { cmd: "true" } }
  recette:  { adapter: http,   reliability: measured, probe: { cmd: "true" } }
`

// Une brique operee sur plusieurs environnements a plusieurs etats reels.
func TestUnRolePeutPorterPlusieursSources(t *testing.T) {
	reg, err := Load(write(t, multiEnv))
	if err != nil {
		t.Fatal(err)
	}
	if is := reg.Check(); len(is) != 0 {
		t.Fatalf("registre attendu coherent :\n%s", issueText(is))
	}
	if n := len(reg.RoleMap["etat.reel"]); n != 2 {
		t.Errorf("2 sources attendues pour etat.reel, obtenu %d", n)
	}
}

// La forme courte reste la norme : la plupart des roles n'ont qu'une source.
func TestLaFormeCourteResteAcceptee(t *testing.T) {
	reg, err := Load(write(t, avecCorpus))
	if err != nil {
		t.Fatal(err)
	}
	b, ok := reg.RoleMap["etat.reel"].First()
	if !ok || b.Source != "cluster" {
		t.Errorf("forme courte mal lue : %+v", reg.RoleMap["etat.reel"])
	}
}

// Sans qualificatif, rien ne dit laquelle des instances une preuve interroge.
func TestPlusieursSourcesSansQualificatifSontSignalees(t *testing.T) {
	body := strings.Replace(multiEnv, "{ source: prod,    qualifier: production }", "{ source: prod }", 1)
	reg, err := Load(write(t, body))
	if err != nil {
		t.Fatal(err)
	}
	got := issueText(reg.Check())
	if !strings.Contains(got, "qualifiee") {
		t.Errorf("le defaut de qualificatif devait etre signale :\n%s", got)
	}
}

func TestLaMemeSourceDeuxFoisEstSignalee(t *testing.T) {
	body := strings.Replace(multiEnv, "{ source: recette, qualifier: recette }", "{ source: prod, qualifier: recette }", 1)
	reg, err := Load(write(t, body))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(issueText(reg.Check()), "deux fois") {
		t.Error("rattacher deux fois la meme source devait etre signale")
	}
}

// Une source vide est refusee par le schema, avant meme la coherence : c'est
// la bonne couche, l'erreur est de forme et non de sens.
func TestUneSourceVideEstRefuseeParLeSchema(t *testing.T) {
	body := strings.Replace(multiEnv, `  etat.reel:
    - { source: prod,    qualifier: production }
    - { source: recette, qualifier: recette }`, `  etat.reel:
    - { source: "" }`, 1)
	if _, err := Load(write(t, body)); err == nil {
		t.Fatal("une source vide doit etre refusee par le schema")
	}
}

// Un role absent du registre reste non pourvu, et c'est la coherence qui le dit.
func TestUnRoleAbsentResteNonPourvu(t *testing.T) {
	body := strings.Replace(multiEnv, `  etat.reel:
    - { source: prod,    qualifier: production }
    - { source: recette, qualifier: recette }
`, "", 1)
	reg, err := Load(write(t, body))
	if err != nil {
		t.Fatal(err)
	}
	got := issueText(reg.Check())
	if !strings.Contains(got, "etat.reel") || !strings.Contains(got, "non pourvu") {
		t.Errorf("le role manquant devait etre nomme :\n%s", got)
	}
}

// Un corpus est singulier : plusieurs corpus demandent plusieurs perimetres.
func TestPlusieursCorpusSontRefuses(t *testing.T) {
	body := strings.Replace(multiEnv, "  contrainte.memoire:    { source: memoire }", `  contrainte.memoire:
    - { source: memoire, qualifier: un }
    - { source: depots,  qualifier: deux }`, 1)
	reg, err := Load(write(t, body))
	if err != nil {
		t.Fatal(err)
	}
	_, _, _, err = reg.CorpusSource()
	if err == nil || !strings.Contains(err.Error(), "singulier") {
		t.Errorf("plusieurs corpus doivent etre refuses nommement, obtenu : %v", err)
	}
}

// Une source rattachee a l'une des instances n'est pas orpheline.
func TestUneSourceDUneInstanceNEstPasOrpheline(t *testing.T) {
	reg, _ := Load(write(t, multiEnv))
	if strings.Contains(issueText(reg.Check()), "orpheline") {
		t.Error("les sources d'un role multiple sont bien rattachees")
	}
	if strings.Contains(issueText(reg.Check()), "aucun role") {
		t.Error("aucune source ne doit etre vue comme non rattachee")
	}
}

// index_hook n'accepte que deux regimes. Une faute de frappe qui passerait
// pour « derived » silencerait index-drift sans que personne ne le voie : le
// schema doit la refuser, pas lui donner un defaut.
func TestIndexHookRefuseUneValeurInconnue(t *testing.T) {
	gabarit := `memoire:  { adapter: files,  reliability: declared, probe: { cmd: "true" }, corpus: { index: MEMORY.md, index_hook: %s } }`

	for _, bon := range []string{"derived", "authored"} {
		body := strings.Replace(complet,
			`memoire:  { adapter: files,  reliability: declared, probe: { cmd: "true" } }`,
			fmt.Sprintf(gabarit, bon), 1)
		if _, err := Load(write(t, body)); err != nil {
			t.Errorf("index_hook: %s est un regime valide, refuse : %v", bon, err)
		}
	}

	for _, mauvais := range []string{"redigee", "manual", "Derived", "true"} {
		body := strings.Replace(complet,
			`memoire:  { adapter: files,  reliability: declared, probe: { cmd: "true" } }`,
			fmt.Sprintf(gabarit, mauvais), 1)
		if _, err := Load(write(t, body)); err == nil {
			t.Errorf("index_hook: %s n'est pas un regime connu et doit etre refuse", mauvais)
		}
	}
}
