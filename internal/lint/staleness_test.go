package lint

import (
	"testing"
	"time"
)

// noteAvecSonde et noteSansSonde partagent la meme date de modification :
// seule la sondabilite doit expliquer un verdict different.
func noteAvecSonde(nom string) string {
	return "---\nname: " + nom + "\ndescription: \"une description assez longue pour ne pas declencher les autres regles\"\n" +
		"metadata:\n  type: reference\n  modified: 2026-01-01\n  verify:\n    - cmd: \"true\"\n---\n\nUn fait sondable.\n"
}

func noteSansSonde(nom string) string {
	return "---\nname: " + nom + "\ndescription: \"une description assez longue pour ne pas declencher les autres regles\"\n" +
		"metadata:\n  type: reference\n  modified: 2026-01-01\n---\n\nUn fait non-sondable.\n"
}

// Test d'acceptation — a duree ecoulee egale (45 jours), une note sondable et
// une note non-sondable recoivent un verdict de staleness different : le
// palier sondable (30j) est deja depasse, le palier non-sondable (180j) non.
func TestLeVerdictDeStalenessDependDeLaSondabilite(t *testing.T) {
	c := corpusTemp(t, map[string]string{
		"sondable.md":     noteAvecSonde("sondable"),
		"non-sondable.md": noteSansSonde("non-sondable"),
	})
	c.Config.Policy.Staleness.ProbedReviewAfterDays = 30
	c.Config.Policy.Staleness.UnprobedReviewAfterDays = 180

	now := time.Date(2026, 2, 15, 0, 0, 0, 0, time.UTC) // 45 jours apres modified
	res, err := Run(c, Options{Now: now})
	if err != nil {
		t.Fatal(err)
	}
	got := findings(res, "staleness")

	perimee := map[string]bool{}
	for _, f := range got {
		perimee[f.File] = true
	}
	if !perimee["sondable.md"] {
		t.Error("la note sondable devrait etre perimee (45j > palier sondable 30j)")
	}
	if perimee["non-sondable.md"] {
		t.Error("la note non-sondable ne devrait pas etre perimee (45j < palier non-sondable 180j)")
	}
}
