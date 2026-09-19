// Package claim type les assertions d'une note et propose la sonde qui les
// prouverait.
//
// Le probleme qu'il resout est un chiffre : sur le corpus de developpement,
// deux notes sur cent une portent une preuve. Le mecanisme est le
// differenciateur du produit, et il reste inutilise tant qu'il faut ecrire
// chaque sonde a la main.
//
// Deux regles gouvernent la proposition, et elles viennent toutes deux d'une
// erreur constatee :
//
//   - On n'infere jamais depuis le sens du texte, seulement depuis des signaux
//     structurels ancres. Un motif lache (« mot/mot » pour un depot) produit
//     80 % de faux positifs — mesure sur le corpus reel.
//   - On ne construit jamais une sonde a partir d'une commande citee dans une
//     note. On n'execute pas ce qu'on trouve ecrit : seules des formes connues
//     et en lecture seule sont proposees.
package claim

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/UnPoilTefal/perimeter/internal/corpus"
	"github.com/UnPoilTefal/perimeter/internal/perimeter"
)

// Confidence dit d'ou vient le signal, donc ce que vaut la proposition.
type Confidence string

const (
	// FromRegistry : la note cite une source declaree. La sonde emprunte
	// l'interrogation de cette source, donc elle mesure le bon systeme.
	FromRegistry Confidence = "registre"
	// Structural : un signal reconnaissable sans le registre. La sonde est
	// plausible, elle demande une relecture plus attentive.
	Structural Confidence = "structurel"
)

// Kind est le type d'assertion. Il porte la demi-vie : un chemin se perime
// plus vite qu'une convention.
type Kind struct {
	Name         string
	HalfLifeDays int
}

var (
	KindIssue      = Kind{"issue", 30}
	KindPath       = Kind{"chemin", 30}
	KindEndpoint   = Kind{"endpoint", 60}
	KindRepo       = Kind{"depot", 60}
	KindHost       = Kind{"hote", 90}
	KindVersion    = Kind{"version", 60}
	KindConvention = Kind{"convention", 120}
)

// Proposal est une sonde candidate pour une note.
type Proposal struct {
	Note       string
	Kind       Kind
	Confidence Confidence
	Source     string // source du registre qui la sert, si ancree
	Match      string // le signal qui l'a declenchee
	Cmd        string
	Expect     string
	Why        string
	// Motif et Consequence portent un caveat de maturite sur la sonde
	// elle-meme — meme forme que Remarque dans internal/perimeter/maturite.go.
	// Vides quand la sonde prouve la revendication ; "sonde-d-existence"
	// quand elle ne prouve que la presence de ce qu'elle sonde.
	Motif       string
	Consequence string
}

// Result agrege une passe de proposition.
type Result struct {
	Total      int
	DejaPreuve int
	Proposals  []Proposal
	SansSignal []string
}

// Coverage rend la part de notes pour lesquelles une sonde existe ou est
// proposable.
func (r *Result) Coverage() float64 {
	if r.Total == 0 {
		return 0
	}
	return float64(r.DejaPreuve+len(r.Proposals)) / float64(r.Total)
}

// pathRe et consorts : signaux structurels ancres. Chacun exige un prefixe
// reconnaissable — jamais un motif generique.
var (
	pathRe = regexp.MustCompile(`(?:^|[\s` + "`" + `(])((?:/(?:Users|opt|volume1|etc|Applications)/|~/)[\w./\-]{4,})`)
	urlRe  = regexp.MustCompile(`https?://[\w.\-]+\.[a-z]{2,}(?:/[\w./\-]*)?`)
	// Quatre octets pleins, obligatoires. L'ancien motif rendait les octets
	// du milieu optionnels et prenait « 10.5.67 » — une version d'OS — pour
	// une adresse.
	// Chaque branche porte son compte d'octets : « 192.168 » en contient deja
	// deux, « 10 » un seul. L'ancien motif rendait les octets du milieu
	// optionnels et prenait « 10.5.67 » — une version d'OS — pour une adresse.
	ipRe       = regexp.MustCompile(`\b(?:192\.168\.\d{1,3}\.\d{1,3}|10\.\d{1,3}\.\d{1,3}\.\d{1,3})\b`)
	semverRe   = regexp.MustCompile(`\bv\d+\.\d+\.\d+\b`)
	trailingRe = regexp.MustCompile(`[.,;:)` + "`" + `]+$`)

	// absenceRe reconnait une note qui enonce une absence. Seuls des
	// marqueurs francs : « jamais » ou « aucun » sont trop larges, ils
	// visaient 24 notes sur 101 dont la plupart n'enoncent aucune absence.
	absenceRe = regexp.MustCompile(`(?i)n'exist(?:e|ent) plus|(?:a|ont) ete supprim|(?:a|ont) été supprim|n'est plus present|supprime du poste|supprimé du poste`)

	phraseSep = regexp.MustCompile(`[.!?]\s|\n`)
)

// zonePertinente rend la partie d'une note ou un signal vaut revendication :
// la description et le premier paragraphe.
//
// Mesure sur le corpus de reference : sans cette contrainte, 45 notes portent
// un signal et la precision apres tri humain tombe a 20 %. Avec, 21 notes et
// ~38 %. Le cout est reel — une bonne proposition sur neuf est perdue, celle
// dont le chemin n'apparait qu'au troisieme paragraphe — mais 24 propositions
// de bruit disparaissent.
func zonePertinente(n *corpus.Note) string {
	body := strings.TrimSpace(n.Body)
	if i := strings.Index(body, "\n\n"); i > 0 {
		body = body[:i]
	}
	return n.Desc + "\n" + body
}

// enonceUneAbsence dit si la phrase portant le signal affirme que la chose
// n'existe plus. Une telle note appelle une sonde inversee : « test -e » sur
// un chemin supprime echouerait alors que la note est vraie.
//
// La detection est locale a la phrase, pas a la note : « le chemin X n'existe
// plus » peut arriver au troisieme paragraphe, loin de la description.
func enonceUneAbsence(body, signal string) bool {
	i := strings.Index(body, signal)
	if i < 0 {
		return false
	}
	debut := 0
	if locs := phraseSep.FindAllStringIndex(body[:i], -1); len(locs) > 0 {
		debut = locs[len(locs)-1][1]
	}
	fin := len(body)
	if loc := phraseSep.FindStringIndex(body[i:]); loc != nil {
		fin = i + loc[1]
	}
	return absenceRe.MatchString(body[debut:fin])
}

// Propose examine un corpus et rend les sondes candidates.
func Propose(c *corpus.Corpus, reg *perimeter.Registry) *Result {
	res := &Result{}
	for _, n := range c.Notes {
		if n.ParseErr != nil {
			continue
		}
		res.Total++
		if len(n.Metadata.Verify) > 0 {
			res.DejaPreuve++
			continue
		}
		if p, ok := proposeFor(n, reg); ok {
			res.Proposals = append(res.Proposals, p)
			continue
		}
		res.SansSignal = append(res.SansSignal, n.Rel)
	}
	sort.Slice(res.Proposals, func(i, j int) bool {
		if res.Proposals[i].Confidence != res.Proposals[j].Confidence {
			return res.Proposals[i].Confidence == FromRegistry
		}
		return res.Proposals[i].Note < res.Proposals[j].Note
	})
	return res
}

func proposeFor(n *corpus.Note, reg *perimeter.Registry) (Proposal, bool) {
	body := n.Body
	// Le registre d'abord : il fournit le vocabulaire, donc les ancres sures.
	if reg != nil {
		if p, ok := fromRegistry(n, body, reg); ok {
			return p, true
		}
	}
	return fromStructure(n, zonePertinente(n), n.Body)
}

// fromRegistry cherche, pour chaque source declaree, une ancre qui lui soit
// propre. Ne pas deviner ce qu'est un depot : demander au registre.
func fromRegistry(n *corpus.Note, body string, reg *perimeter.Registry) (Proposal, bool) {
	for _, name := range reg.SourceNames() {
		s := reg.Sources[name]
		if s.Endpoint == "" {
			continue
		}
		if !ancrable(s.Endpoint) {
			continue
		}
		for _, a := range anchorsFor(s.Adapter, s.Endpoint) {
			m := a.re.FindStringSubmatch(body)
			if m == nil {
				continue
			}
			p := Proposal{
				Note: n.Rel, Kind: a.kind, Confidence: FromRegistry,
				Source: name, Match: strings.TrimSpace(m[0]),
				Why: fmt.Sprintf("cite la source %q du registre", name),
			}
			// Une source qui declare une interrogation parametree se prete
			// directement : la sonde emprunte son chemin, pas une commande
			// reinventee.
			if s.Query != nil && len(m) > 1 && m[1] != "" {
				p.Cmd = fmt.Sprintf("source: %s\n  arg: %q", name, m[1])
				p.Expect = ""
				return p, true
			}
			p.Cmd = fmt.Sprintf("source: %s", name)
			return p, true
		}
	}
	return Proposal{}, false
}

// substantiel exige une suite d'au moins trois caracteres de mot.
var substantiel = regexp.MustCompile(`[\p{L}\p{N}]{3,}`)

// ancrable ecarte les endpoints degeneres. Un « . » ou un « / » donne une
// ancre qui matche tout : mesure sur le corpus reel, c'est la premiere cause
// de faux positif, devant les motifs laches.
func ancrable(endpoint string) bool {
	return len(endpoint) >= 4 && substantiel.MatchString(endpoint)
}

type anchor struct {
	re   *regexp.Regexp
	kind Kind
}

// anchorsFor rend les ancres propres a un adaptateur. Jamais le mot nu : sur
// un corpus homelab, « homelab » apparait partout et matcherait tout.
func anchorsFor(adapter, endpoint string) []anchor {
	switch adapter {
	case "github", "gitlab":
		repo := endpoint
		if i := strings.LastIndex(endpoint, "/"); i >= 0 {
			repo = endpoint[i+1:]
		}
		return []anchor{
			{regexp.MustCompile(`\b` + regexp.QuoteMeta(repo) + `#(\d+)\b`), KindIssue},
			{regexp.MustCompile(regexp.QuoteMeta(endpoint)), KindRepo},
		}
	case "files", "git":
		return []anchor{{regexp.MustCompile(regexp.QuoteMeta(endpoint)), KindPath}}
	default:
		return []anchor{{regexp.MustCompile(regexp.QuoteMeta(endpoint)), KindEndpoint}}
	}
}

// premierHote rend la premiere adresse de la zone qui designe un hote
// joignable. Une adresse est ecartee si un octet depasse 255, ou si le dernier
// vaut 0 : « 10.244.0.0 » est un plan d'adressage de pods, pas une machine, et
// la sonder produirait une sonde vouee a echouer.
func premierHote(zone string) string {
	for _, m := range ipRe.FindAllString(zone, -1) {
		octets := strings.Split(m, ".")
		if len(octets) != 4 {
			continue
		}
		valide := true
		for i, o := range octets {
			n, err := strconv.Atoi(o)
			if err != nil || n > 255 || (i == 3 && n == 0) {
				valide = false
				break
			}
		}
		if valide {
			return m
		}
	}
	return ""
}

// fromStructure propose depuis un signal reconnaissable sans le registre. La
// sonde reste en lecture seule et d'une forme connue.
func fromStructure(n *corpus.Note, zone, body string) (Proposal, bool) {
	if m := pathRe.FindStringSubmatch(zone); m != nil {
		path := trailingRe.ReplaceAllString(m[1], "")
		if enonceUneAbsence(body, path) {
			return Proposal{
				Note: n.Rel, Kind: KindPath, Confidence: Structural, Match: path,
				Cmd: fmt.Sprintf("cmd: %q", "! test -e "+path),
				Why: "enonce que ce chemin n'existe plus — sonde inversee",
			}, true
		}
		return Proposal{
			Note: n.Rel, Kind: KindPath, Confidence: Structural, Match: path,
			Cmd:         fmt.Sprintf("cmd: %q", "test -e "+path),
			Why:         "mentionne un chemin absolu",
			Motif:       "sonde-d-existence",
			Consequence: "prouve que le chemin est la, pas ce que la note affirme a son sujet",
		}, true
	}
	if m := urlRe.FindString(zone); m != "" {
		u := trailingRe.ReplaceAllString(m, "")
		return Proposal{
			Note: n.Rel, Kind: KindEndpoint, Confidence: Structural, Match: u,
			Cmd:    fmt.Sprintf("cmd: %q", "curl -sfI -o /dev/null -w '%{http_code}' "+u),
			Expect: `expect_stdout: "^[23]"`,
			Why:    "mentionne une adresse HTTP",
		}, true
	}
	if m := premierHote(zone); m != "" {
		return Proposal{
			Note: n.Rel, Kind: KindHost, Confidence: Structural, Match: m,
			Cmd: fmt.Sprintf("cmd: %q", "ping -c1 -W2 "+m+" >/dev/null"),
			Why: "mentionne un hote du reseau interne",
		}, true
	}
	if m := semverRe.FindString(zone); m != "" {
		return Proposal{
			Note: n.Rel, Kind: KindVersion, Confidence: Structural, Match: m,
			Why: "mentionne une version — la sonde depend de ce qui la porte, a completer",
		}, true
	}
	return Proposal{}, false
}

// Write insere les sondes proposees dans les notes, en commentaire.
//
// Jamais actives : une note ne devient verifiable que lorsqu'un humain a relu
// la sonde et l'a decommentee. C'est la meme discipline que pour la memoire
// elle-meme — une proposition d'agent arrive en « proposed », pas en « canon ».
func Write(c *corpus.Corpus, proposals []Proposal) (int, error) {
	byRel := c.ByRel()
	done := 0
	for _, p := range proposals {
		n, ok := byRel[p.Note]
		if !ok || n.ParseErr != nil {
			continue
		}
		raw, err := os.ReadFile(n.Path)
		if err != nil {
			return done, err
		}
		s := string(raw)
		if strings.Contains(s, "# sonde proposee") {
			continue // deja propose, on ne double pas
		}
		var b strings.Builder
		b.WriteString("  # sonde proposee par « perctl propose » — relire, puis decommenter\n")
		fmt.Fprintf(&b, "  # type %s · confiance %s · %s\n", p.Kind.Name, p.Confidence, p.Why)
		if p.Motif != "" {
			fmt.Fprintf(&b, "  # ~ [%s] %s\n", p.Motif, p.Consequence)
		}
		b.WriteString("  # verify:\n")
		for _, line := range strings.Split(p.Cmd, "\n") {
			fmt.Fprintf(&b, "  #   - %s\n", strings.TrimSpace(line))
		}
		if p.Expect != "" {
			fmt.Fprintf(&b, "  #     %s\n", p.Expect)
		}

		// Inserer juste avant la cloture du frontmatter.
		idx := strings.Index(s[4:], "\n---\n")
		if idx < 0 {
			continue
		}
		cut := idx + 5
		out := s[:cut] + b.String() + s[cut:]
		if err := os.WriteFile(n.Path, []byte(out), 0o644); err != nil {
			return done, err
		}
		done++
	}
	return done, nil
}
