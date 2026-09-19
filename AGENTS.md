# perimeter — conventions agent

Conventions de workflow pour développer perimeter. Le modèle du produit
(axiome, six rôles, pas de score) vit dans le `README.md` — ce fichier ne
parle que de comment on travaille dessus.

## Avant tout code : spec puis plan

Toute tâche au-delà d'un correctif trivial suit `superpowers:brainstorming`
puis `superpowers:writing-plans` avant la moindre ligne de code.

Le spec issu du brainstorming est une **issue GitHub sur ce repo** — jamais
un fichier `docs/specs/*.md` : aucun précédent ici, le tracker est la
source canonique. Format observé sur #59-62 et #68 : constat mesuré,
proposition, raisonnement, ce que ça remplace ou dont ça dépend.

## Implémentation : quelle boucle TDD

- `superpowers:test-driven-development` par défaut — boucle rouge-vert
  stricte, une tâche du plan à la fois.
- `mattpocock-skills:tdd` quand la tâche ouvre une **nouvelle frontière de
  package** (un nouveau dossier sous `internal/`) : elle négocie les seams
  avant le premier test. Consulter `mattpocock-skills:codebase-design`
  pour le vocabulaire d'interface au même moment, pas en session séparée.

Le refactor reste hors de la boucle rouge-vert dans les deux cas : il
attend la revue de fin d'issue, jamais mêlé à l'implémentation.

## Avant de fermer une issue

1. `superpowers:verification-before-completion` — preuve d'exécution
   fraîche avant toute affirmation de succès.
2. `mattpocock-skills:code-review` contre l'issue d'origine (l'issue *est*
   le spec) — axe Standards (baseline de smells Fowler ; pas de
   `CODING_STANDARDS.md` ici, donc rien d'autre à charger) et axe Spec.
3. `superpowers:finishing-a-development-branch` pour le merge.

`superpowers:requesting-code-review` / `receiving-code-review` s'ajoutent
seulement quand un humain relit réellement le diff — l'audit ci-dessus est
automatique et ne les remplace pas.

## Sur déclencheur précis

- `mattpocock-skills:research` avant d'écrire un connecteur (ex. #62) :
  ancrer la syntaxe exacte d'une CLI tierce plutôt que la deviner.
- `mattpocock-skills:grilling` en pressure-test d'un design juste avant
  `writing-plans`, sur les chantiers à fort enjeu (#68 en est un exemple).
- `mattpocock-skills:domain-modeling` pour tenir un `CONTEXT.md` à jour dès
  qu'un concept du modèle change (rôle, invariant, statut de fiabilité).
  N'existe pas encore ici — à créer via ce skill au prochain chantier qui
  touche le modèle, pas rétroactivement pour son propre compte.
