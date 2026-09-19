package advisory

import (
	"testing"
	"time"

	"github.com/UnPoilTefal/perimeter/internal/perimeter"
	"github.com/UnPoilTefal/perimeter/internal/reliability"
	"github.com/UnPoilTefal/perimeter/internal/report"
)

var now = time.Date(2026, 9, 19, 0, 0, 0, 0, time.UTC)

// registreComplet fournit les six roles, chacun rattache a une source
// existante — le cas sain pour Registry.Check().
func registreComplet() *perimeter.Registry {
	sources := map[string]perimeter.Source{}
	roles := map[string]perimeter.Bindings{}
	for _, role := range perimeter.Roles {
		name := role + "-source"
		sources[name] = perimeter.Source{
			Adapter: "files",
			Probe:   &perimeter.Command{Cmd: "true"},
		}
		roles[role] = perimeter.Bindings{{Source: name}}
	}
	return &perimeter.Registry{Path: "perimeter.yml", RoleMap: roles, Sources: sources}
}

func TestEmptyQuandRienASignaler(t *testing.T) {
	a := Compute(registreComplet(), &report.Result{}, nil, t.TempDir(), 30, now)
	if !a.Empty() {
		t.Errorf("attendu Empty(), obtenu %+v", a)
	}
}

func TestLiensCompteLesConstatsWikilink(t *testing.T) {
	res := &report.Result{Findings: []report.Finding{
		{Rule: "wikilink", File: "a.md"},
		{Rule: "wikilink", File: "b.md"},
		{Rule: "description", File: "c.md"}, // nit editorial, ne doit pas compter
	}}
	a := Compute(nil, res, nil, "", 0, now)
	if a.Liens != 2 {
		t.Errorf("Liens = %d, attendu 2", a.Liens)
	}
	if a.Registre != 0 || a.Faits != 0 {
		t.Errorf("sans registre, Registre et Faits doivent rester a zero, obtenu %+v", a)
	}
}

func TestPeremptionCompteLeConstatAgregeMasPasLeParNote(t *testing.T) {
	res := &report.Result{Findings: []report.Finding{
		{Rule: "staleness", File: "a.md"},        // par-note, exclu (nit editorial)
		{Rule: "staleness", File: "b.md"},        // idem
		{Rule: "staleness-budget", File: "idx"}, // agrege, seul celui-ci compte
	}}
	a := Compute(nil, res, nil, "", 0, now)
	if a.Peremption != 1 {
		t.Errorf("Peremption = %d, attendu 1 (seul staleness-budget compte, pas les 2 staleness par-note)", a.Peremption)
	}
}

func TestRegistreCompteLesIssuesDeCoherence(t *testing.T) {
	// Un seul role pourvu sur six : Registry.Check() rapporte les cinq
	// manquants.
	reg := &perimeter.Registry{
		Path:    "perimeter.yml",
		RoleMap: map[string]perimeter.Bindings{perimeter.Roles[0]: {{Source: "s"}}},
		Sources: map[string]perimeter.Source{"s": {Adapter: "files", Probe: &perimeter.Command{Cmd: "true"}}},
	}
	a := Compute(reg, &report.Result{}, nil, t.TempDir(), 30, now)
	if a.Registre != len(perimeter.Roles)-1 {
		t.Errorf("Registre = %d, attendu %d (cinq roles non pourvus)", a.Registre, len(perimeter.Roles)-1)
	}
}

func TestFaitsCompteStaleEtDeriveSansDoublonParRef(t *testing.T) {
	entries := []reliability.Entry{
		// Stale : confirme par un humain il y a longtemps, budget depasse.
		{Ref: "note.md#L1", Status: reliability.StatusConfirmeHumain, ConfirmedAt: "2020-01-01"},
		// Meme ref confirmee deux fois : Latest() ne doit compter qu'une fois.
		{Ref: "note.md#L1", Status: reliability.StatusConfirmeHumain, ConfirmedAt: "2020-01-02"},
		// Derive : l'ancre pointe vers un fichier absent du root de test.
		{Ref: "absent.md#L1", Status: reliability.StatusPreuveScriptee, ClaimHash: "sha256:aaaa"},
		// Sain : preuve scriptee, sans ancre de ligne a rehacher.
		{Ref: "sain.md", Status: reliability.StatusPreuveScriptee},
	}
	a := Compute(registreComplet(), &report.Result{}, entries, t.TempDir(), 30, now)
	if a.Faits != 2 {
		t.Errorf("Faits = %d, attendu 2 (une ref stale, une ref derivee, la ref saine exclue, pas de doublon)", a.Faits)
	}
}

func TestFaitsResteAZeroSansRegistre(t *testing.T) {
	entries := []reliability.Entry{
		{Ref: "note.md#L1", Status: reliability.StatusConfirmeHumain, ConfirmedAt: "2020-01-01"},
	}
	a := Compute(nil, &report.Result{}, entries, t.TempDir(), 30, now)
	if a.Faits != 0 {
		t.Errorf("sans registre, Faits doit rester a zero (rien ne dit ou vit l'index), obtenu %d", a.Faits)
	}
}

func TestLigneAvisIndisponible(t *testing.T) {
	a := Advisory{Unavailable: true, Cause: "index illisible"}
	got := a.Ligne()
	want := "avis ambiant indisponible : index illisible"
	if got != want {
		t.Errorf("Ligne() = %q, attendu %q", got, want)
	}
	if a.Empty() {
		t.Error("un avis indisponible n'est pas un avis vide : il doit rester visible")
	}
}

func TestLigneEtWarningsEnumerentLesCategoriesNonNulles(t *testing.T) {
	a := Advisory{Registre: 2, Liens: 1}
	w := a.Warnings()
	if len(w) != 2 {
		t.Fatalf("Warnings() = %v, attendu 2 entrees", w)
	}
	ligne := a.Ligne()
	if ligne == "" {
		t.Error("Ligne() ne doit pas etre vide quand l'avis ne l'est pas")
	}
}

func TestWarningsRendNilQuandVide(t *testing.T) {
	a := Advisory{}
	if w := a.Warnings(); w != nil {
		t.Errorf("Warnings() = %v, attendu nil sur un avis vide", w)
	}
}
