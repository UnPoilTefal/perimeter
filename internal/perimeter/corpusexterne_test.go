package perimeter

import (
	"path/filepath"
	"testing"
)

// #85 — corpus_externes: declare des corpus de documentation qui n'adoptent
// pas le schema perimeter (vault Obsidian, catalogue produit) : ni role, ni
// validation fonctionnelle sur leur champ type.
const avecCorpusExterne = `
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
  memoire:  { adapter: files,  reliability: declared, endpoint: notes, probe: { cmd: "true" } }
  depots:   { adapter: git,    reliability: declared, probe: { cmd: "true" } }
  cluster:  { adapter: http,   reliability: measured, probe: { cmd: "true" } }
corpus_externes:
  - { chemin: "../vault-homelab", type: obsidian }
  - { chemin: "../catalogue-produit", type: catalogue }
`

func TestLeRegistreParseLesCorpusExternes(t *testing.T) {
	reg, err := Load(write(t, avecCorpusExterne))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(reg.CorpusExternes) != 2 {
		t.Fatalf("attendu 2 corpus externes, obtenu %d", len(reg.CorpusExternes))
	}
	if reg.CorpusExternes[0].Chemin != "../vault-homelab" || reg.CorpusExternes[0].Type != "obsidian" {
		t.Errorf("premier corpus externe inattendu : %+v", reg.CorpusExternes[0])
	}
	if reg.CorpusExternes[1].Type != "catalogue" {
		t.Errorf("second corpus externe inattendu : %+v", reg.CorpusExternes[1])
	}
	if is := reg.Check(); len(is) != 0 {
		t.Errorf("un corpus externe ne doit pourvoir aucun role, Check() doit rester coherent : %v", is)
	}
}

func TestCorpusExterneTypeEstTexteLibre(t *testing.T) {
	// N'importe quel texte doit passer le schema — jamais une enumeration
	// fermee (decision actee en grilling, #85).
	body := `
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
  memoire:  { adapter: files,  reliability: declared, endpoint: notes, probe: { cmd: "true" } }
  depots:   { adapter: git,    reliability: declared, probe: { cmd: "true" } }
  cluster:  { adapter: http,   reliability: measured, probe: { cmd: "true" } }
corpus_externes:
  - { chemin: "../n-importe-quoi", type: "un texte totalement libre, jamais valide" }
`
	if _, err := Load(write(t, body)); err != nil {
		t.Fatalf("un type texte libre doit toujours charger : %v", err)
	}
}

func TestResoudreCorpusExterneRelatifAuRegistre(t *testing.T) {
	regPath := write(t, avecCorpusExterne)
	reg, err := Load(regPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	got := reg.ResoudreCorpusExterne(reg.CorpusExternes[0])
	want := filepath.Join(filepath.Dir(regPath), "../vault-homelab")
	if got != want {
		t.Errorf("ResoudreCorpusExterne() = %q, attendu %q", got, want)
	}
}
