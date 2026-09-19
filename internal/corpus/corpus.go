package corpus

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/UnPoilTefal/perimeter/internal/perimeter"
)

// Le corpus n'a pas de fichier de configuration propre : sa politique vit dans
// la source qui le porte, au registre de perimetre. Une equipe n'ecrit qu'un
// seul fichier.

// Config decrit un corpus. Les valeurs par defaut correspondent a la
// disposition d'un repertoire de memoire Claude Code (notes a plat, index
// MEMORY.md), pour qu'un corpus existant marche sans configuration.
type Config struct {
	Version int `yaml:"version"`

	Corpus struct {
		Path    string   `yaml:"path"`
		Index   string   `yaml:"index"`
		Exclude []string `yaml:"exclude"`
	} `yaml:"corpus"`

	Policy struct {
		Types        []string `yaml:"types"`
		MaxBodyWords int      `yaml:"max_body_words"`
		Staleness    struct {
			ReviewAfterDays         int     `yaml:"review_after_days"`
			MaxStaleRatio           float64 `yaml:"max_stale_ratio"`
			ProbedReviewAfterDays   int     `yaml:"probed_review_after_days"`
			UnprobedReviewAfterDays int     `yaml:"unprobed_review_after_days"`
		} `yaml:"staleness"`
		RequireOwner bool `yaml:"require_owner"`

		// IndexHook declare qui ecrit l'accroche d'index. « derived » :
		// elle reprend la description, et toute divergence est une
		// derive. « authored » : elle est ecrite a la main pour un
		// lecteur humain, et n'a aucune raison de coincider.
		//
		// Les deux lecteurs different — la description sert a l'agent
		// qui decide s'il ouvre la note, l'accroche sert a l'humain qui
		// parcourt l'index. Les forcer egales optimise pour un seul des
		// deux ; le choix appartient a l'equipe, rien ne permet de le
		// deduire du corpus.
		IndexHook string `yaml:"index_hook"`

		// RequireDate fait d'une note sans date une erreur et non un
		// avertissement. Une note qu'aucune date ne rattache au temps ne
		// peut pas vieillir : sa peremption est invisible quel que soit
		// le budget de relecture. Mesure sur un corpus reel : 18 % des
		// notes etaient dans ce cas, et l'avertissement ne les avait
		// jamais fait corriger.
		RequireDate bool `yaml:"require_date"`
	} `yaml:"policy"`

	// Links declare les corpus voisins vers lesquels un lien est legitime.
	// Un corpus de memoire n'est jamais seul : il cite un wiki, des
	// runbooks, des commandes. Sans cette declaration, l'outil ne peut pas
	// distinguer un lien externe valide d'un lien casse.
	Links struct {
		IgnorePrefixes []string `yaml:"ignore_prefixes"`
		ExternalRoots  []string `yaml:"external_roots"`
	} `yaml:"links"`

	Verify struct {
		TimeoutSeconds int `yaml:"timeout_seconds"`
	} `yaml:"verify"`
}

// DefaultConfig rend la configuration appliquee quand le registre ne declare
// aucune politique de corpus. Tout ce qui est fixe ici est un defaut : un
// registre n'a pas a le reecrire pour l'obtenir.
func DefaultConfig() *Config {
	c := &Config{Version: 1}
	c.Corpus.Path = "."
	c.Corpus.Index = "MEMORY.md"
	c.Policy.Types = []string{"user", "feedback", "project", "reference", "decision"}
	c.Policy.MaxBodyWords = 400
	c.Policy.Staleness.ReviewAfterDays = 180
	c.Policy.Staleness.MaxStaleRatio = 0.15
	c.Policy.IndexHook = IndexHookDerived
	c.Policy.RequireDate = true
	// Un lien commencant par / designe une commande ou une skill, pas une note.
	c.Links.IgnorePrefixes = []string{"/"}
	c.Verify.TimeoutSeconds = 30
	return c
}

// Corpus est un ensemble de notes charge depuis le disque.
type Corpus struct {
	Root      string
	Config    *Config
	Notes     []*Note
	IndexPath string
	// IndexTargets liste les fichiers cites par l'index, dans l'ordre.
	IndexTargets []string
	// IndexEntries decrit les entrees completes reconnues par l'outil.
	IndexEntries []IndexEntry
	HasIndex     bool
}

// IndexEntry est une ligne d'index de la forme "- [titre](fichier.md) — accroche".
// L'accroche est de l'etat derive : elle doit reproduire la description de la
// note, faute de quoi le routeur finit par contredire ce qu'il route.
type IndexEntry struct {
	Line   int
	Raw    string
	Title  string
	Target string
	Hook   string
}

// ConfigFromPolicy traduit la politique declaree au registre en configuration
// de corpus. Un champ absent retombe sur le defaut, pour qu'un registre
// minimal reste utilisable.
func ConfigFromPolicy(p *perimeter.CorpusPolicy) *Config {
	cfg := DefaultConfig()
	if p == nil {
		return cfg
	}
	if p.Index != nil {
		cfg.Corpus.Index = *p.Index
	}
	if p.Exclude != nil {
		cfg.Corpus.Exclude = p.Exclude
	}
	if len(p.Types) > 0 {
		cfg.Policy.Types = p.Types
	}
	if p.MaxBodyWords > 0 {
		cfg.Policy.MaxBodyWords = p.MaxBodyWords
	}
	cfg.Policy.RequireOwner = p.RequireOwner
	if p.IndexHook != "" {
		cfg.Policy.IndexHook = p.IndexHook
	}
	// Pointeur cote registre : un booleen absent ne se distingue pas d'un
	// booleen a false, et le defaut est ici true.
	if p.RequireDate != nil {
		cfg.Policy.RequireDate = *p.RequireDate
	}
	if p.Staleness.ReviewAfterDays > 0 {
		cfg.Policy.Staleness.ReviewAfterDays = p.Staleness.ReviewAfterDays
	}
	if p.Staleness.MaxStaleRatio > 0 {
		cfg.Policy.Staleness.MaxStaleRatio = p.Staleness.MaxStaleRatio
	}
	if p.Staleness.ProbedReviewAfterDays > 0 {
		cfg.Policy.Staleness.ProbedReviewAfterDays = p.Staleness.ProbedReviewAfterDays
	}
	if p.Staleness.UnprobedReviewAfterDays > 0 {
		cfg.Policy.Staleness.UnprobedReviewAfterDays = p.Staleness.UnprobedReviewAfterDays
	}
	if p.Links.IgnorePrefixes != nil {
		cfg.Links.IgnorePrefixes = p.Links.IgnorePrefixes
	}
	if p.Links.ExternalRoots != nil {
		cfg.Links.ExternalRoots = p.Links.ExternalRoots
	}
	if p.VerifyTimeoutSeconds > 0 {
		cfg.Verify.TimeoutSeconds = p.VerifyTimeoutSeconds
	}
	return cfg
}

// ProbedBudget rend le budget de relecture d'une note sondable : rejouer sa
// sonde ne coute rien, donc ce palier reste serre. Sans reglage explicite,
// review_after_days sert d'alias retrocompatible.
func (c *Config) ProbedBudget() int {
	if c.Policy.Staleness.ProbedReviewAfterDays > 0 {
		return c.Policy.Staleness.ProbedReviewAfterDays
	}
	return c.Policy.Staleness.ReviewAfterDays
}

// UnprobedBudget rend le budget de relecture d'une note non-sondable :
// revalider coute un humain, donc ce palier reste large par defaut. Sans
// reglage explicite, review_after_days sert d'alias retrocompatible.
func (c *Config) UnprobedBudget() int {
	if c.Policy.Staleness.UnprobedReviewAfterDays > 0 {
		return c.Policy.Staleness.UnprobedReviewAfterDays
	}
	return c.Policy.Staleness.ReviewAfterDays
}

// StalenessBudget route vers le palier applicable a une note precise : une
// sonde scriptee (metadata.verify non vide) releve du palier sondable,
// tout le reste du palier non-sondable.
func (c *Config) StalenessBudget(n *Note) int {
	if len(n.Metadata.Verify) > 0 {
		return c.ProbedBudget()
	}
	return c.UnprobedBudget()
}

// indexLink capture les cibles markdown d'un lien : [titre](fichier.md)
var indexLink = regexp.MustCompile(`\]\(([^)\s]+\.md)\)`)

// indexEntry reconnait une entree complete : puce, titre, cible, accroche.
var indexEntry = regexp.MustCompile(`^\s*[-*]\s*\[([^\]]*)\]\(([^)\s]+\.md)\)\s*(?:[—-]\s*(.*))?$`)

// Load charge le corpus enracine en root.
// Load charge un corpus avec la politique par defaut. C'est l'usage ad hoc :
// pointer un repertoire sans avoir declare de perimetre.
func Load(root string) (*Corpus, error) {
	return LoadWith(root, DefaultConfig())
}

// LoadWith charge un corpus avec une politique donnee, typiquement celle
// declaree au registre de perimetre.
func LoadWith(root string, cfg *Config) (*Corpus, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		cfg = DefaultConfig()
	}
	notesRoot := filepath.Join(abs, cfg.Corpus.Path)
	c := &Corpus{Root: notesRoot, Config: cfg}

	if cfg.Corpus.Index != "" {
		c.IndexPath = filepath.Join(notesRoot, cfg.Corpus.Index)
		if raw, err := os.ReadFile(c.IndexPath); err == nil {
			c.HasIndex = true
			for _, m := range indexLink.FindAllStringSubmatch(string(raw), -1) {
				c.IndexTargets = append(c.IndexTargets, m[1])
			}
			for i, line := range strings.Split(string(raw), "\n") {
				if m := indexEntry.FindStringSubmatch(line); m != nil {
					c.IndexEntries = append(c.IndexEntries, IndexEntry{
						Line: i + 1, Raw: line, Title: m[1], Target: m[2],
						Hook: strings.TrimSpace(m[3]),
					})
				}
			}
		}
	}

	excluded := func(rel string) bool {
		if cfg.Corpus.Index != "" && rel == cfg.Corpus.Index {
			return true
		}
		for _, pat := range cfg.Corpus.Exclude {
			if ok, _ := filepath.Match(pat, rel); ok {
				return true
			}
			if ok, _ := filepath.Match(pat, filepath.Base(rel)); ok {
				return true
			}
		}
		return false
	}

	err = filepath.WalkDir(notesRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if name := d.Name(); path != notesRoot && (strings.HasPrefix(name, ".") || name == "node_modules") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".md") {
			return nil
		}
		rel, _ := filepath.Rel(notesRoot, path)
		if excluded(rel) {
			return nil
		}
		n, err := LoadNote(notesRoot, path)
		if err != nil {
			return err
		}
		c.Notes = append(c.Notes, n)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(c.Notes, func(i, j int) bool { return c.Notes[i].Rel < c.Notes[j].Rel })
	return c, nil
}

// ExternalStems rend l'ensemble des titres de notes atteignables dans les
// corpus voisins declares, indexes comme le fait un vault : par nom de
// fichier sans extension, quel que soit le sous-repertoire.
func (c *Corpus) ExternalStems() map[string]string {
	stems := map[string]string{}
	for _, root := range c.Config.Links.ExternalRoots {
		root = expandHome(root)
		if !filepath.IsAbs(root) {
			root = filepath.Join(c.Root, root)
		}
		_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				if path != root && strings.HasPrefix(d.Name(), ".") {
					return filepath.SkipDir
				}
				return nil
			}
			if strings.HasSuffix(d.Name(), ".md") {
				stems[strings.TrimSuffix(d.Name(), ".md")] = path
			}
			return nil
		})
	}
	return stems
}

func expandHome(p string) string {
	if strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, p[2:])
		}
	}
	return p
}

// ByRel indexe les notes par chemin relatif.
func (c *Corpus) ByRel() map[string]*Note {
	m := make(map[string]*Note, len(c.Notes))
	for _, n := range c.Notes {
		m[n.Rel] = n
	}
	return m
}

// ByName indexe les notes par champ name.
func (c *Corpus) ByName() map[string]*Note {
	m := make(map[string]*Note, len(c.Notes))
	for _, n := range c.Notes {
		if n.Name != "" {
			m[n.Name] = n
		}
	}
	return m
}

// Regimes d'accroche d'index. Voir Config.Policy.IndexHook.
const (
	IndexHookDerived  = "derived"
	IndexHookAuthored = "authored"
)

// IndexHookAuthore dit si les accroches de ce corpus sont ecrites a la main.
func (c *Corpus) IndexHookAuthore() bool {
	return c.Config != nil && c.Config.Policy.IndexHook == IndexHookAuthored
}

// SyncIndexAutorise garde la regeneration des accroches. En regime authored
// elle detruirait du travail humain : « perctl index --sync » reecrirait
// d'un coup toutes les accroches a partir des descriptions.
//
// Taire la regle sans desarmer la commande ne suffirait pas — la config
// dirait une chose et l'outil resterait capable du contraire.
func SyncIndexAutorise(c *Corpus) error {
	if c.IndexHookAuthore() {
		return fmt.Errorf("policy.index_hook vaut \"authored\" : les accroches sont ecrites a la main, les regenerer les ecraserait")
	}
	return nil
}

// politiqueAvecIndex sert les tests : elle construit une politique dont seule
// la cle index varie, nil valant « absente ».
func politiqueAvecIndex(index *string) *perimeter.CorpusPolicy {
	return &perimeter.CorpusPolicy{Index: index}
}
