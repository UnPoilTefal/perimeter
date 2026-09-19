# perimeter — conventions agent

Conventions de workflow pour développer perimeter. Le modèle du produit
(axiome, six rôles, pas de score) vit dans le `README.md` — ce fichier ne
parle que de comment on travaille dessus.

⚠️ Basculé de `superpowers` à `mattpocock-skills` le 2026-09-19. Précondition
posée par `/setup-matt-pocock-skills` : `docs/agents/issue-tracker.md` et
`docs/agents/domain.md`, plus le bloc `## Agent skills` dans `CLAUDE.md`.

## Avant tout code : idée → spec → tickets

Toute tâche au-delà d'un correctif trivial suit `mattpocock-skills:grill-with-docs`
(interview qui affine l'idée, alimente `CONTEXT.md`/`docs/adr/` au fil de
l'eau) puis `mattpocock-skills:to-spec`. Si le chantier tient sur plusieurs
sessions, enchaîner sur `mattpocock-skills:to-tickets` (tickets tracer-bullet
avec dépendances de blocage) avant la moindre ligne de code ; sinon,
`/implement` directement dans la même fenêtre de contexte.

Le spec issu de `to-spec` est une **issue GitHub sur ce repo** — jamais un
fichier `docs/specs/*.md` : aucun précédent ici, le tracker est la source
canonique. Format observé sur #59-62 et #68 : constat mesuré, proposition,
raisonnement, ce que ça remplace ou dont ça dépend.

## Implémentation : quelle boucle TDD

- `mattpocock-skills:tdd` par défaut — boucle rouge-vert, une tâche du plan
  à la fois, pilotée par `/implement`.
- Consulter `mattpocock-skills:codebase-design` en même temps que `tdd`, pas
  en session séparée, dès qu'une tâche ouvre une **nouvelle frontière de
  package** (un nouveau dossier sous `internal/`) : elle négocie les seams
  avant le premier test.

Le refactor reste hors de la boucle rouge-vert dans les deux cas : il
attend la revue de fin d'issue, jamais mêlé à l'implémentation.

## Avant de fermer une issue

1. Preuve d'exécution fraîche avant toute affirmation de succès. Pas
   d'équivalent `mattpocock-skills` identifié pour ce réflexe
   (`superpowers:verification-before-completion` n'a pas de remplaçant dans
   la carte de `/ask-matt`) — c'est une règle tenue à la main, pas un skill
   à invoquer.
2. `mattpocock-skills:code-review` contre l'issue d'origine (l'issue *est*
   le spec) — axe Standards (baseline de smells Fowler ; pas de
   `CODING_STANDARDS.md` ici, donc rien d'autre à charger) et axe Spec.
   Sert aussi bien à l'audit automatique de fin d'issue qu'à une relecture
   humaine explicite d'une branche/PR — un seul skill, les deux usages.
3. Merge à la main. Même gap qu'au point 1 : aucun skill `mattpocock-skills`
   ne couvre `superpowers:finishing-a-development-branch`.

## Sur déclencheur précis

- `mattpocock-skills:research` avant d'écrire un connecteur (ex. #62) :
  ancrer la syntaxe exacte d'une CLI tierce plutôt que la deviner.
- `mattpocock-skills:grilling` en pressure-test d'un design juste avant
  `to-spec`, sur les chantiers à fort enjeu (#68 en est un exemple).
- `mattpocock-skills:domain-modeling` pour tenir `CONTEXT.md` à jour dès
  qu'un concept du modèle change (rôle, invariant, statut de fiabilité).
  `grill-with-docs` le crée déjà organiquement dès le premier chantier qui
  touche le modèle ; `domain-modeling` sert à l'affiner ensuite, pas à le
  créer rétroactivement pour son propre compte.
