# perimeter

Savoir si un agent peut agir — sur un homelab, une équipe, un produit.

Un **périmètre**, c'est l'ensemble de ce qu'un agent doit connaître pour travailler sans se
tromper de prémisse : une mémoire de ce qui n'est pas re-dérivable, un registre des briques qui
font autorité, et une porte qui décide si une spécification est prête.

Ce qu'un agent tient pour acquis pourrit par défaut, et pourrit en silence. `perimeter` traite
cette connaissance comme du code : schéma, CI, revue, et surtout **des faits qui portent leur
propre preuve**.

```
perctl lint       # la structure du corpus tient-elle ?
perctl verify     # les faits et les sources sont-ils encore vrais ?
perctl perimeter  # chaque rôle est-il pourvu et sondable ?
perctl gate       # cette spécification est-elle prête ?
```

---

## L'axiome

> **La mémoire ne stocke que ce qui n'est pas re-dérivable depuis la source de vérité.**

Ce qui est dans le code, l'état d'infrastructure, le système vivant ou l'historique git
se redemande à l'exécution. Ce qui reste tient dans peu de place et ne pourrit presque
pas : **le pourquoi et les pièges**. C'est cette frontière qui borne la taille du corpus
et qui rend la péremption gérable.

Corollaire pratique : une note qui commence par « le service X écoute sur le port Y »
est probablement à jeter. Une note qui dit « le port 9 de cette gateway est le WAN2,
indiscernable d'un port LAN libre dans l'interface » ne se re-dérive pas — elle a coûté
une panne.

## Les cinq registres

Un corpus mélange cinq choses qui n'ont ni le même cycle de vie, ni le même relecteur.
Les confondre dans un wiki unique est le mode d'échec le plus courant.

| Registre | Contenu | Durée de vie |
|---|---|---|
| `user` | qui est l'utilisateur, son contexte | lente |
| `feedback` | comment on attend que l'agent travaille | lente |
| `reference` | un fait contre-intuitif, un piège | stable, **vérifiable** |
| `project` | un chantier en cours, son état | **courte** |
| `decision` | un choix et son pourquoi (ADR) | immuable |

`project` est le registre le plus périssable : c'est là que la pourriture commence.

---

## Installation

```bash
brew install UnPoilTefal/tap/perimeter
```

<details>
<summary>Autres canaux</summary>

```bash
# Binaires : https://github.com/UnPoilTefal/perimeter/releases

# Depuis les sources (suppose une chaîne Go)
go install github.com/UnPoilTefal/perimeter/cmd/perctl@latest

# En CI, sans installer Go
docker run --rm -v "$PWD:/w" -w /w ghcr.io/unpoiltefal/perimeter:latest lint memory/
```

</details>

## Démarrage

```bash
cd <votre corpus>
perctl init      # écrit perimeter.yml, le registre du périmètre
perctl lint      # premier état des lieux
```

Un corpus est un répertoire de notes markdown à frontmatter YAML, plus un index qui
sert de routeur. La disposition par défaut correspond à celle d'un répertoire de
mémoire Claude Code : les notes à plat, `MEMORY.md` en index.

### Anatomie d'une note

```markdown
---
name: reference-udm-port-9-est-wan2
description: "Le port 9 de l'UDM Pro est le WAN2, indiscernable d'un port LAN libre
  dans l'interface : y brancher un équipement le place sur le WAN"
metadata:
  type: reference
  trust: canon
  owner: "@equipe-reseau"
  source: "https://github.com/org/repo/issues/24"
  modified: 2026-09-13
  verify:
    - cmd: "curl -s ... | jq -r '.ports[8].name'"
      expect_stdout: "WAN2"
      note: "le port 9 est toujours déclaré WAN côté gateway"
---

Un fait, une note. Le corps explique ce que la description annonce,
et lie les notes voisines avec [[reference-autre-note]].
```

Le schéma complet : [`schema/memory.schema.json`](schema/memory.schema.json)
(`perctl schema` l'écrit sur la sortie standard, pour votre éditeur).

---

## `perctl lint` — la structure

| Règle | Sévérité | Ce qu'elle empêche |
|---|---|---|
| `parse` | erreur | une note illisible est invisible à tout le reste |
| `schema` | erreur | frontmatter non conforme |
| `name-match` | erreur | `name` divergent du fichier : les liens pointent dans le vide |
| `index-orphan` | erreur | **une note absente de l'index est écrite mais jamais lue** |
| `index-dangling` | erreur | l'index cite une note supprimée |
| `index-drift` | avert. | l'accroche d'index contredit la description qu'elle route *(régime `derived` seulement)* |
| `wikilink` | avert. | lien cassé, y compris vers un corpus voisin |
| `description` | avert. | une description qui ne permet pas de décider du rappel |
| `atomicity` | avert. | une note trop large ne se périme jamais proprement |
| `ownership` | avert. | sans propriétaire, personne ne supprime jamais rien |
| `staleness` | avert. | une note dont l'échéance de relecture est passée |
| `staleness` | **erreur** | une note sans aucune date — voir `policy.require_date` |
| `staleness-budget` | erreur | le corpus dérive plus vite qu'il n'est relu |
| `secret` | erreur | la mémoire est rechargée à chaque session : c'est un vecteur de fuite |

### Qui écrit l'accroche d'index — `policy.index_hook`

L'index porte, pour chaque note, une **accroche** : la phrase après le tiret. Deux
régimes, et le choix appartient à l'équipe.

| | `derived` *(défaut)* | `authored` |
|---|---|---|
| L'accroche | reprend la description | est rédigée à la main |
| `index-drift` | signale toute divergence | ne s'applique pas |
| `index --sync` | régénère | **refuse** |
| `index --fix` | ajoute les manquantes | ajoute les manquantes, à relire |

La raison d'être des deux : **description et accroche servent deux lecteurs.** La
description dit à l'agent s'il doit ouvrir la note ; l'accroche aide l'humain qui
parcourt l'index à s'orienter. Les forcer égales optimise pour un seul des deux.

En `derived`, l'accroche est de l'état dérivé et toute divergence est une dérive — le
mode de panne réel est une accroche qui finit par **contredire** la note qu'elle route.
En `authored`, elle est du contenu à part entière : la comparer produirait un
avertissement par entrée, et ce bruit noie les constats utiles.

⚠️ En `authored`, `index --sync` **refuse** au lieu de régénérer. Taire la règle sans
désarmer la commande laisserait la configuration dire une chose et l'outil rester
capable du contraire — un seul `--sync` écraserait toutes les accroches d'un coup.

Rien dans un corpus ne permet de déduire le régime : c'est une intention, donc elle se
déclare. Une valeur inconnue est **refusée** par le schéma, jamais rabattue sur un
défaut — sinon une faute de frappe couperait la règle en silence.

`index-orphan` est la règle qui justifie l'outil à elle seule. L'index est le routeur du
rappel : une note qui n'y figure pas a été écrite, relue, commitée — et ne sera jamais
lue par l'agent. Rien ne le signale sans outillage. Sur le corpus qui a servi à
développer `perimeter`, 11 notes sur 98 étaient dans ce cas.

### Un contrôle qui touche tout le corpus est un constat de processus

Un contrôle qui parle à chaque ligne cesse d'être lu, **et emporte les autres avec
lui**. Sur un corpus mesuré de 116 notes, `index-drift` remontait 116 avertissements —
un par note — qui reléguaient hors de l'écran les 23 `atomicity`, les seuls à demander
un arbitrage.

Or 116 occurrences ne veulent pas dire « 116 notes sont mauvaises ». Elles veulent dire
**« le processus qui écrit l'index ne dérive pas les accroches »** : un seul fait,
répété, avec une remédiation unique. Il est donc rendu une fois, avec sa mesure et sa
commande :

```
! index-drift : l'index n'est pas synchronisé (116/116 notes)
  → perctl index --sync
```

**Ce qui rend un constat repliable n'est pas son nombre, c'est sa remédiation.** Un
contrôle n'est replié que s'il porte *une* commande qui répare toutes ses occurrences à
la fois, indépendamment de l'identité des notes. `atomicity` n'en a pas : chaque note
trop large demande son propre arbitrage. Même à 100 %, il reste détaillé — replier ses
constats perdrait la seule information qu'ils portent.

Le taux n'est que le déclencheur : à partir de **90 %** du périmètre examiné, et d'au
moins **10 occurrences**. En dessous, l'identité des notes *est* l'information, et le
détail reste. `--verbose` et `--format json` le rendent dans tous les cas.

⚠️ Une remédiation n'est jamais suggérée si la politique du corpus l'interdit. En
régime `authored`, `index --sync` refuse d'écrire : `index-drift` ne s'y déclenche pas,
et la suggestion passe par la porte qui garde la commande elle-même.

```bash
perctl lint                      # sortie terminal
perctl lint --verbose            # détaille les contrôles repliés
perctl lint --format github      # annotations GitHub Actions
perctl lint --format json        # pour un agent
perctl lint --strict             # les avertissements font échouer
perctl lint --disable atomicity,staleness
```

### L'index est de l'état dérivé

L'accroche portée par l'index doit reproduire la description de la note. Quand elle est
maintenue à la main en parallèle, elle finit par annoncer autre chose que ce que la note
dit — et le rappel se fait sur une information fausse.

```bash
perctl index           # quelles notes manquent à l'index
perctl index --fix     # les ajouter
perctl index --sync    # régénérer les accroches depuis les descriptions
```

### Proposer les sondes plutôt que les attendre

Le mécanisme de preuve ne sert à rien tant qu'il faut écrire chaque sonde à la main. Sur le
corpus de développement, **2 notes sur 101** en portaient une.

```bash
perctl propose            # ce qu'il écrirait
perctl propose --write    # l'écrit, en commentaire
```

`propose` type chaque assertion à partir de **signaux structurels ancrés**, jamais du sens du
texte, et propose la sonde correspondante :

| Confiance | Signal | Sonde proposée |
|---|---|---|
| `registre` | la note cite une source déclarée — `produit#412` | `source: tickets`, `arg: "412"` |
| `structurel` | chemin absolu, URL, hôte, version | `test -e …`, `curl -sfI …` |

L'ancrage au registre est le bon : **ne pas deviner ce qu'est un dépôt, demander au registre.**
Un motif générique (`mot/mot`) produit 80 % de faux positifs, et un endpoint dégénéré (`.`)
donne une ancre qui matche tout — les deux mesurés, les deux écartés par le code.

**Un signal ne compte que s'il est proche de la revendication** : dans la `description` ou le
premier paragraphe. Au-delà, c'est du contexte. Sans cette contrainte, 45 notes d'un corpus de
101 portaient un signal pour une précision de 20 % après tri humain ; avec, 21 notes et ~38 %.
Le coût est assumé — une bonne proposition sur neuf est perdue, celle dont le chemin n'apparaît
qu'au troisième paragraphe.

**Une note qui énonce une absence appelle une sonde inversée.** « Ce chemin n'existe plus »
se prouve par `! test -e`, pas par `test -e` — qui échouerait alors que la note dit vrai. La
détection est locale à la phrase portant le signal, et n'accepte que des marqueurs francs :
« jamais » et « aucun » visaient 24 notes sur 101, dont la plupart n'énoncent aucune absence.

**Une note sans date est une erreur, pas un avertissement** — `policy.require_date`, vrai par
défaut. Une note qu'aucune date ne rattache au temps est hors de portée de toute péremption :
aucun budget, aucune demi-vie ne l'atteindra jamais. Mesure sur un corpus réel : **18 % des
notes** étaient dans ce cas, et l'avertissement ne les avait jamais fait corriger — il se noyait
dans le bruit.

Trois champs valent : `modified`, `verified_at`, `review_after`. Un seul suffit — exiger nommément
`modified` rejetterait une note prouvée la veille par `verify`.

⚠️ Un corpus existant qui ne peut pas s'y plier tout de suite déclare `require_date: false` plutôt
que de désactiver `staleness` en entier, ce qui lui ferait perdre aussi la détection de péremption.
**Combler les dates d'abord, durcir ensuite** : l'inverse fait basculer d'un coup toutes les notes
sans date en erreur, et la règle est désactivée avant d'avoir servi.

Le budget de relecture est **unique pour tout le corpus** — `policy.staleness.review_after_days`.
Une demi-vie par type, un chemin se périmant plus vite qu'une convention, se défend en principe
mais n'est pas implémentée : la différencier demanderait des observations qu'aucun corpus n'a
encore produites. Suivi en [#38](https://github.com/UnPoilTefal/perimeter/issues/38).

**Deux règles de sûreté, chacune née d'une erreur constatée :**

- Une sonde n'est **jamais** construite à partir d'une commande citée dans une note. On
  n'exécute pas ce qu'on trouve écrit.
- Les sondes sont écrites **en commentaire**. Une note ne devient vérifiable que lorsqu'un
  humain a relu et décommenté — même discipline que pour la mémoire : une proposition d'agent
  arrive en `proposed`, jamais en `canon`.

`propose` annonce toujours **le corpus et le registre sur lesquels il agit**. Un registre au
chemin absolu fait travailler ailleurs que là où l'on croit être.

---

## `perctl verify` — les faits

C'est la boucle qui sépare un corpus de mémoire d'un wiki. Un fait qui porte une commande
de vérification cesse d'être une affirmation datée : il devient une assertion testable.

```yaml
metadata:
  verify:
    - cmd: "gh api repos/org/repo/rulesets --jq '.[].name'"
      expect_stdout: "protect-main"
      note: "la protection de branche tient toujours"
```

```bash
perctl verify --allow-exec           # rejoue les preuves
perctl verify --allow-exec --write   # inscrit verified_at et verify_status
perctl verify --allow-exec --only authentik
```

Une preuve en échec **ne supprime pas la note** : elle la signale. C'est un humain qui
tranche entre « le fait a changé » et « le monde a changé » — les deux se corrigent
différemment.

`verify` rapporte aussi le **taux de couverture** : quelle proportion du corpus porte une
preuve. C'est la métrique de santé la plus utile, et elle monte lentement — commencez par
les faits qui vous ont déjà coûté un incident.

### Sécurité : `verify` exécute du code

Les commandes sont du shell déclaré dans des fichiers de documentation. C'est délibéré —
c'est ce qui rend la preuve exécutable — et c'est une surface d'attaque.

- L'exécution est **refusée par défaut**. `--allow-exec` est obligatoire.
- Un corpus partagé doit protéger ses notes par `CODEOWNERS`, au même titre que sa CI.
- Le job CI qui lance `verify` doit avoir des permissions minimales et **ne jamais tourner
  sur une pull request venant d'un fork**.
- N'écrivez que des commandes en lecture seule et idempotentes.

`perctl lint` ne lance jamais rien : la CI de pull request peut l'utiliser sans réserve.

### Le garde-fou est dans l'outil, pas dans la documentation

`--allow-exec` **refuse de s'exécuter** quand il reconnaît une contribution venue de l'extérieur :
une pull request GitHub ou une merge request GitLab dont la source n'est pas le dépôt lui-même.

Le doute ne profite pas à l'exécution : sur un événement de pull request, il faut pouvoir
*confirmer* que la source est le dépôt. Charge d'événement illisible, dépôts non nommés, variable
absente — tout cela vaut refus.

```
contexte : GitHub Actions, pull request
pull request issue de tiers/produit, distinct de org/produit : le contenu vient de l'extérieur
```

Passer outre demande `--allow-exec-untrusted`, qu'il faut écrire sciemment.

Hors intégration continue, ou sur un `push`, un `schedule`, un déclenchement manuel, rien ne
change. Une CI non reconnue sans indication d'événement n'est pas refusée : on n'invente pas un
risque, et on ne casse pas les travaux nocturnes.

### Voir seulement ce qui exécute du code

```bash
perctl verify <corpus> --diff-probes main
```

`CODEOWNERS` fait relire le **fichier entier**. Cette commande ne rapporte que les notes dont le
**bloc de preuve** a changé — la seule portion du diff qui finira par exécuter quelque chose. Elle
n'exécute rien, donc elle est faite pour tourner là où l'exécution est justement interdite.

Une note dont seul le texte a changé n'apparaît pas. Reformuler le commentaire d'une preuve non
plus : c'est la commande et son attente qui comptent.

---

## Le registre de périmètre

Un corpus de mémoire ne vit pas seul. Autour de lui, une équipe a des tickets, des décisions,
des dépôts, un état réel — chacun avec son accès, ses identifiants, et surtout **son régime de
fiabilité**. Le registre déclare quelle brique concrète sert quel rôle.

```yaml
# perimeter.yml
version: 1

roles:
  intention.spec:        { source: specs }
  intention.tickets:     { source: tickets }
  contrainte.decisions:  { source: adr }
  contrainte.memoire:    { source: memoire }
  etat.declare:          { source: depots }
  etat.reel:             { source: cluster }

sources:
  tickets:
    adapter: github
    endpoint: org/produit
    reliability: measured
    credential: env:GITHUB_TOKEN      # une référence, jamais la valeur
    probe:
      cmd: "gh api repos/${endpoint} --jq .full_name"
      expect_stdout: "^org/produit$"
    query:
      cmd: "gh api repos/${endpoint}/issues/${arg} --jq .state"
```

```bash
perctl perimeter perimeter.yml                     # chaque rôle est-il pourvu, chaque source sondable ?
perctl perimeter perimeter.yml --probe --allow-exec  # et chaque sonde passe-t-elle vraiment ?
```

### Un rôle porte une source, ou plusieurs

Une brique opérée sur plusieurs environnements a **autant d'états réels que d'environnements**.
N'en déclarer qu'un revient à mentir par omission sur les autres.

```yaml
roles:
  etat.declare:  { source: depots }          # la forme courte reste la norme
  etat.reel:
    - { source: prod,    qualifier: production }
    - { source: recette, qualifier: recette }
```

Dès qu'un rôle porte plusieurs sources, chacune doit être **qualifiée** — sinon rien ne dit
laquelle une preuve interroge.

Le corpus de mémoire fait exception : `contrainte.memoire` n'accepte qu'une source. **Un corpus
est singulier** ; plusieurs corpus demandent plusieurs périmètres.

### ⚠ L'angle mort du modèle à deux vérités

Comparer le déclaré au réel suppose que l'**intention**, elle, est stable. Quand elle ne l'est
pas — une cible technique encore en arbitrage, écrite dans la spécification puis remplacée avant
que rien n'ait bougé — déclaré et réel sont **parfaitement convergents** : ils décrivent tous
deux l'état d'avant.

> **La convergence entre le déclaré et le réel peut masquer que les deux sont périmés par
> rapport à l'intention.**

La porte ne voit aucun écart et rassure à tort. Ce n'est pas un rôle qui manque — c'est une
**carence d'intention**, donc un arrêt légitime, qu'il revient à l'auteur de l'évaluation de
classer comme telle.

Aucun mécanisme ne la détecte aujourd'hui : elle est invisible à un outil qui ne compare que
deux états.

### Les six rôles ne sont pas configurables

`intention.spec`, `intention.tickets`, `contrainte.decisions`, `contrainte.memoire`,
`etat.declare`, `etat.reel`. Un périmètre qui n'en pourvoit pas un a une **carence**, pas une
préférence — et le registre la nomme plutôt que de la laisser passer. C'est le premier signal
qu'un périmètre n'est pas encore décrit.

### Les sources portent leur propre sonde

C'est l'axiome remonté d'un cran. Sans sonde, une boîte qui n'a vérifié que ses *faits* finit
par affirmer sereinement qu'aucun ticket ne contredit — parce que son jeton a expiré trois
semaines plus tôt.

**Déclarer une sonde n'est pas la passer.** `perctl perimeter` vérifie que chaque source en
déclare une ; seul le rejeu dit qu'elle y arrive :

```bash
perctl perimeter perimeter.yml --probe --allow-exec
```

```
perimeter.yml — 6 sources, 6 sondes rejouees

  ✓ adr      code de sortie 0, aucune sortie
  ✗ cluster  code de sortie 1, attendu 0
      sonde   : curl -sf -o /dev/null -w '%{http_code}' https://api.exemple.internal/healthz
      sortie  : (aucune sortie)
      attendu : code de sortie 0, et sortie conforme a /^200$/
  ✓ depots   code de sortie 0, aucune sortie
  ✓ memoire  code de sortie 0, aucune sortie
  ✓ specs    code de sortie 0, aucune sortie
  ✓ tickets  org/produit

1 sonde sur 6 en echec
  tant qu'une source ne repond pas, tout ce qui s'y adosse ne prouve rien
```

Le **code de retour est non nul dès qu'une sonde échoue** : la commande se branche telle quelle
en CI et en pre-commit. Elle ne demande **aucun corpus** — c'est le registre qu'elle éprouve, et
c'est ce qui la rend utilisable le jour où l'on pose son premier `perimeter.yml`, avant d'avoir
attaché la moindre preuve à une note.

Le même rejeu s'ajoute à une vérification de corpus, pour porter sur **les faits, les sources,
et le registre** en une passe :

```bash
perctl verify <corpus> --perimeter perimeter.yml --probe-sources --allow-exec
```

`--allow-exec` est exigé dans les deux cas : une sonde est du shell déclaré dans un fichier de
configuration. Sans le drapeau, la commande refuse explicitement plutôt que de ne rien faire.

### Une preuve peut renvoyer à une source

```yaml
metadata:
  verify:
    - source: tickets          # l'interrogation est résolue par le registre
      arg: "412"
      expect_stdout: "^closed$"
```

Préférable à une commande écrite en dur dès qu'un périmètre est déclaré : le fait survit au
changement d'outil, la commande non. Changer de traqueur de tickets devient une ligne du
registre au lieu d'une reprise de toutes les notes.

### Un chemin et un registre se combinent

Un chemin positionnel dit **quel corpus**, `--perimeter` dit **quelle politique**. Les deux ne
s'excluent pas :

```bash
perctl lint                              # corpus et politique viennent du registre
perctl lint <chemin> --perimeter <reg>   # ce corpus-la, avec cette politique-la
perctl lint <chemin>                     # usage ad hoc : les defauts s'appliquent
```

⚠️ **Un registre seulement ambiant ne s'applique pas à un corpus désigné à la main.** Si
`PERIMETER` est exporté dans le shell, analyser un répertoire quelconque au passage ne doit pas lui
appliquer la politique d'un autre périmètre. L'outil le dit alors plutôt que de choisir en silence :

```
note : PERIMETER est definie, mais un chemin est donne —
       la politique du registre n'est pas appliquee
```

L'écart est loin d'être cosmétique. Sur un corpus réel, `lint <chemin>` rendait **142**
avertissements contre **24** avec la politique — mêmes notes, verdicts incomparables.

### La maturité du registre — au-delà de valide / invalide

`perctl perimeter` rend deux choses distinctes, et il ne faut pas les confondre.

**La cohérence est binaire** : chaque rôle est pourvu, chaque source est sondable, aucun
identifiant n'est en clair. Elle passe ou elle échoue.

Elle se lit **sur le fichier**, sans rien exécuter — donc un registre cohérent peut décrire des
sources dont aucune ne répond. C'est `--probe` qui tranche cette question-là, et lui seul.

**La maturité ne l'est pas.** Un registre peut être parfaitement valide et pourtant ne pas
permettre grand-chose. Les remarques de maturité le disent **sans invalider** :

| motif | ce qu'il signale |
|---|---|
| `etat-reel-non-mesure` | `etat.reel` pointe une source `declared` — le réel n'est pas constaté, l'écart entre les deux vérités devient inobservable |
| `familles-confondues` | une source sert `intention` **et** `etat` — comparer ce qui est voulu à ce qui est n'apprend plus rien |
| `sonde-d-existence` | la sonde se limite à `test -d` ou `true` — elle prouve que la source est là, pas qu'elle dit ce qu'on attend d'elle |

⚠️ **Ces remarques ne sont pas des erreurs, et c'est délibéré.** Les rendre bloquantes
rendrait invalide tout registre existant, et ferait désactiver la vérification en entier.

⚠️ **Il n'y a pas de score.** Ce projet a remplacé les scores de confiance par des tests
falsifiables, et a constaté trois fois qu'un signal lexical *classe sans classifier*. Un
« registre à 78 % » inviterait à optimiser le chiffre plutôt que le périmètre. Chaque remarque
porte un motif nommable et **sa conséquence** — et le silence quand il n'y a rien à dire.

C'est aussi ce qui donne enfin un rôle au champ `reliability`, jusque-là exigé par le schéma et
consulté nulle part.

### Deux invariants, appliqués par le schéma et non par la discipline

**Aucun identifiant en clair.** Le champ `credential` n'accepte qu'une référence
(`env:` `file:` `keychain:` `cmd:` `op:`), résolue à l'exécution. Une valeur déguisée en
référence est signalée à la validation.

**Aucune recopie de source.** Le schéma n'offre aucun champ de cache, de miroir ou de
synchronisation locale : en déclarer un est une erreur de validation. Ce qui est dérivable est
cherché au moment de l'évaluation, jamais mémorisé — sinon on recrée exactement la péremption
qu'on cherche à supprimer.

### Compatibilité

Un corpus sans registre garde **exactement** son comportement : `perctl lint` et
`perctl verify` fonctionnent comme avant. Le registre est additif.

Une preuve qui renvoie à une source alors qu'aucun registre n'est chargé **échoue nommément**
plutôt que d'être ignorée — une preuve non jouée ne prouve rien, et la taire donnerait un
verdict faussement vert.

---

## La porte de readiness

Une spécification n'est pas prête parce qu'on se sent confiant. Elle est prête quand on sait
écrire les tests qui diront qu'elle est satisfaite.

> **L'incapacité à écrire le test est le signal.**

On ne sait pas exprimer l'assertion d'une intention qu'on n'a pas comprise — et le blocage est
localisé : on sait *quel* test résiste, donc quelle partie de la spécification est trouble. Le
même artefact sert deux fois : **porte** avant de produire, **critère d'acceptation** après.

### Le partage du travail

`perctl` ne juge pas si une spécification est comprise — c'est un binaire, il ne sait pas lire.
Il fournit **le cadre et le registre des passages** ; l'agent fournit le jugement. C'est la même
division que pour les preuves : quelqu'un écrit la sonde, l'outil la rejoue.

Le verdict n'est donc **jamais déclaré** — le schéma n'accepte aucun champ `verdict`. Il est
dérivé de ce qui a été consigné.

```yaml
# evaluation.yml
version: 1
spec: "issue #131 — alerter quand une source ne répond plus"

tests:
  - id: sonde-en-echec-signale
    assertion: "une source dont la sonde échoue produit un constat nommant la source"
    status: writable
  - id: seuil-de-bruit
    assertion: "une source en échec depuis moins que le seuil toléré n'alerte pas"
    status: blocked
    blocked_by: seuil-inconnu

deficiencies:
  - id: seuil-inconnu
    classification: mesurable
    statement: "le seuil de tolérance n'est pas connu — il se lit dans l'historique du journal"

facts:
  - reference-index-est-le-routeur
```

```bash
perctl gate evaluation.yml --perimeter perimeter.yml --corpus memory/
```

### Trois classifications, trois issues

| Classification | Issue | Régime |
|---|---|---|
| `mesurable` | **instruire** — l'agent va la combler seul | normal |
| `connaissance` | **rendre la main** — le savoir n'est dans aucune source déclarée | démarrage |
| `intention` | **rendre la main** — seul arrêt légitime en régime établi | permanent |

Le critère n'est pas « suis-je incertain » mais **« est-ce réductible par déduction »**. Tout ce
qui est mesurable, l'agent va le chercher ; seule l'ambiguïté d'intention justifie de rendre la
main. Le code de sortie porte le verdict — `0` produire, `1` instruire, `2` rendre la main — pour
qu'une automatisation s'y branche.

### Ce que l'outil vérifie seul

Deux préconditions mécaniques, qui empêchent `produire` sans jamais faire rendre la main (par
construction, l'agent peut aller les régler) :

- **la couverture du périmètre** — un rôle non pourvu, c'est E1 non tenue ;
- **la fraîcheur des faits mobilisés** — s'appuyer sur un fait dont la preuve ne tient plus,
  c'est agir sur une prémisse fausse, exactement ce que la porte existe pour empêcher.

Et une cohérence : un test bloqué doit désigner une carence existante, une carence doit bloquer
quelque chose ou être résolue. Sans ça, omettre de classer ce qui bloque produirait un verdict
faussement vert.

### Une résolution s'affirme, ou elle se prouve

Le verdict dérive de la classification des carences, et cette classification est **déclarée par
l'auteur de l'évaluation**. Un agent qui cherche `produire` peut reclasser une ambiguïté
d'intention en carence mesurable, la déclarer résolue, et rien dans la structure du document ne
le trahit.

```yaml
deficiencies:
  - id: seuil-inconnu
    classification: mesurable
    statement: "le seuil n'est pas connu — il se lit dans l'historique"
    resolved: true
    resolved_by: "lu dans l'historique : 3 échecs consécutifs"   # obligatoire
    resolved_proof:                                              # ce qui la rend vérifiable
      cmd: "…"
      expect_stdout: "^3$"
```

`resolved_by` est **obligatoire** dès que `resolved` vaut `true` : une résolution qui ne dit pas
ce qui a été fait n'est vérifiable par personne, pas même par un humain.

`resolved_proof` la rend rejouable. *« Je suis allé mesurer X »* se prouve par la mesure — c'est
l'axiome du produit appliqué à sa propre porte.

```bash
perctl gate evaluation.yml --allow-exec    # rejoue les preuves de résolution
```

Une résolution dont la preuve ne tient pas **empêche `produire`**, au même titre qu'un fait dont
la preuve a lâché. Le garde-fou d'exécution s'applique ici aussi.

Une résolution seulement affirmée ne bloque pas — mais elle est **comptée**, et le compte va au
journal. C'est l'accumulation qui révèle qu'une porte est contournée par reclassement :

```
⚠ 3 résolution(s) affirmée(s) sans preuve sur la fenêtre
```

### Sortir du démarrage

Chaque passage est consigné dans un journal **en ajout seul** — on ne réécrit pas l'histoire des
verdicts, sinon la mesure de maturité devient déclarative et ne vaut plus rien.

```bash
perctl readiness
```

Le démarrage prend fin quand, sur une fenêtre de passages consécutifs, **plus aucune escalade ne
porte sur une carence de connaissance** — seulement sur l'intention. La mesure est gratuite : la
porte classe déjà ses escalades. L'instrument qui évalue les spécifications évalue aussi sa
propre maturité.

**Le régime n'est pas un réglage.** Il se déduit du journal : interactif tant que le démarrage
dure, par lot ensuite. Aucun drapeau à positionner, rien qui pourrisse.

Si le journal se remplit sans jamais tenir la condition, ce n'est pas une boucle à laisser
tourner — c'est un diagnostic : *les sources déclarées ne suffisent pas à couvrir le périmètre*.

---

## Mode d'emploi

### Le portillon d'écriture

Trois questions avant d'écrire quoi que ce soit. Une seule réponse négative, on n'écrit pas.

1. **Est-ce non re-dérivable ?** Si un appel d'outil donne la réponse, ce n'est pas de la mémoire.
2. **Est-ce non éphémère ?** Si c'est vrai cette semaine seulement, c'est une issue, pas une mémoire.
3. **Est-ce que ça comptera dans trois mois ?** Sinon, le coût de relecture dépasse la valeur.

Plus une contrainte dure : **un fait par fichier**. C'est ce qui rend le dédoublonnage et
la péremption praticables.

### Amorcer un corpus — `perctl harvest`

Une équipe qui adopte l'outil part de zéro, là où un corpus organique met **deux ans** à se
constituer. `harvest` lit les traces déjà là et en propose des candidats — il **n'écrit jamais**,
et tout candidat arrive en `trust: proposed`.

```bash
perctl harvest --allow-exec --limite 25
```

**La porte de périmètre est structurelle** : `harvest` ne lit que la source rattachée à
`etat.declare` dans le registre. Un candidat vient donc d'une source déclarée, par construction.
C'est important pour la suite : un transcript de session d'agent, lui, enregistre **tout ce qui
s'est dit devant l'agent** — périmètre ou non. Mesuré sur un corpus réel, il contenait du personnel
sans rapport avec le périmètre technique. Sur un corpus d'équipe ce n'est pas du bruit, c'est une
fuite — et la règle `secret` ne l'attraperait pas, elle cherche des identifiants, pas du hors-sujet.
Cette source demandera donc une porte explicite, elle n'est pas lue aujourd'hui.

**Le filtrage est volontairement agressif.** Un candidat doit porter **deux** signaux : une
formulation qui dit ce qui *ne marche pas*, et une explication. Mesure sur 263 commits d'un dépôt
réel : 69 corps portent un piège, 38 un pourquoi, **20 les deux**. Un harvest qui proposerait tout
ce qu'il trouve reproduirait le problème qu'il prétend résoudre.

La raison du double signal tient à l'axiome : un message qui décrit ce que le commit **fait** se
redemande au diff. Ce qui ne se redérive pas, c'est le piège et le pourquoi.

⚠️ Le voisinage rendu par candidat **classe, il ne tranche pas** — voir `draft` ci-dessous. Sur 20
candidats réels, un seuil à 0,15 marquait un vrai doublon, un faux, et en manquait un troisième
dont la note sortait pourtant en tête. Aucun marqueur binaire n'est donc rendu.

### Où vit le registre — `PERIMETER` et la tour de contrôle

Le registre est cherché dans cet ordre, du plus explicite au plus implicite :

| | |
|---|---|
| 1 | `--perimeter <chemin>` |
| 2 | la variable d'environnement **`PERIMETER`** |
| 3 | la remontée d'arborescence depuis le répertoire courant, comme `.git` |
| 4 | `~/.config/perimeter/perimeter.yml` *(ou `$XDG_CONFIG_HOME/perimeter/`)* |

Le niveau 3 sert le modèle **un projet, un périmètre** : une équipe pose son registre à la
racine du dépôt, et toute commande lancée dedans le trouve.

Le niveau 4 sert le modèle **tour de contrôle** : un opérateur travaille depuis un répertoire
central et rayonne vers plusieurs dépôts, dont aucun ne porte le registre. La remontée ne trouve
alors rien — et c'est l'usage qui n'était pas servi.

⚠️ **L'ordre compte.** La remontée prime sur l'emplacement utilisateur : sinon un registre
personnel masquerait celui du dépôt où l'on se trouve, et une commande lancée dans un projet
travaillerait ailleurs sans le dire.

⚠️ **Une piste explicite qui ne répond pas n'est pas ignorée.** Si `PERIMETER` pointe un fichier
absent, l'outil le signale au lieu de retomber sur un autre registre : l'utilisateur a exprimé une
intention, la taire le ferait travailler sur un autre périmètre sans le savoir.

Quand rien ne répond, le message dit **où l'outil a cherché** — un constat d'absence qui laisse le
diagnostic à l'utilisateur est précisément ce qui fait vivre ce genre de défaut sans qu'il soit vu.

### Vérifier un brouillon — `perctl draft`

Le portillon ne se mécanise pas : les trois questions sont des jugements, et un
outil qui prétendrait y répondre donnerait une fausse assurance. `perctl draft`
fait ce qui est mécanique, et **n'écrit jamais**.

```bash
perctl draft brouillon.md          # ou : ... | perctl draft -
```

| Ce qu'il rend | |
|---|---|
| **collision** | une note porte déjà ce `name` — la compléter, pas en créer une seconde |
| **constats** | schéma, registre, description, longueur, secret |
| **voisinage** | « est-ce que ça existe déjà ? » — liste courte ordonnée |
| **preuve** | un bloc `verify` proposé quand la note énonce un fait testable |

Le nom du fichier est déduit du champ `name`, pas du chemin : un brouillon vit
dans un fichier temporaire, et c'est le `name` qui dit où la note doit aller.

⚠️ **Le voisinage classe, il ne tranche pas.** Mesure sur un corpus réel de 104
notes : les vrais recouvrements sortent bien en tête, mais l'échelle absolue ne
veut rien dire — le meilleur voisin médian vaut 0,106, soit du bruit. D'où un
**plafond** et non un seuil : le coût de relecture est connu d'avance, et le
verdict reste à l'humain ou à l'agent.

Le skill correspondant — le portillon complet, portable vers un harnais d'agent —
est dans [`skills/remember/SKILL.md`](skills/remember/SKILL.md).

### Écrire la description

C'est la description, pas le corps, que l'agent lit pour décider si la note est
pertinente. Une mauvaise description rend la note aussi inutile qu'une note absente de
l'index.

- Énoncer **le fait**, pas le sujet. `« Renovate retargète en interne la dernière
  version, ce qui fige une PR en pending »` et non `« notes sur Renovate »`.
- Y mettre les mots qu'on emploierait en cherchant, pas le vocabulaire canonique.
- Une phrase. Si deux sont nécessaires, la note n'est pas atomique.

### Le rituel

| Quand | Quoi |
|---|---|
| À chaque PR | `perctl lint --format github` |
| Chaque nuit | `perctl verify --allow-exec --write`, PR d'écart si échec |
| Chaque mois | relire les `project` — c'est le registre qui pourrit |
| Chaque trimestre | purger ce qui n'a jamais été rappelé |

---

## En équipe

Quatre choses qui ne servent à rien quand on est seul, et sans lesquelles le corpus
d'une équipe meurt en six mois.

**Niveaux de confiance.** `trust: canon | proposed | personal`. `canon` a été revu en PR
par le propriétaire du domaine ; `proposed` a été écrit par un agent et ne l'a pas été ;
`personal` n'a qu'une portée individuelle. L'agent doit savoir dans quel niveau il puise.
Sans ça, l'hypothèse fausse d'une personne devient vérité d'équipe en trois semaines.

**Propriété.** `owner` sur chaque note, plus `CODEOWNERS` sur le répertoire. Activer
`policy.require_owner`. Sans propriétaire désigné, personne ne supprime jamais rien, et
le corpus meurt par accumulation.

**Cloisonnement.** La règle `secret` est un garde-fou avant le commit, pas un scanner :
doublez-la de `gitleaks` en CI. Et fixez explicitement ce qui n'a pas le droit d'entrer —
client, données personnelles, périmètre.

**Une métrique.** *Temps jusqu'à la première réponse correcte* pour un nouvel arrivant.
Humain ou agent, c'est la même mesure : l'onboarding d'une personne et celui d'un agent
sont le même problème, et un corpus de mémoire le résout une fois pour les deux.

---

## Corpus voisins

Une mémoire n'est jamais seule : elle cite un wiki, des runbooks, des commandes. Sans
déclaration, l'outil ne peut pas distinguer un lien externe valide d'un lien cassé.

```yaml
links:
  ignore_prefixes: ["/"]        # [[/deploy]] désigne une commande, pas une note
  external_roots:
    - ~/wiki                    # résolution par titre de fichier, à la Obsidian
```

Déclarer les voisins transforme le contrôle de liens en **contrôle d'intégrité
inter-corpus** : un renommage côté wiki casse le lien, et le lint le dit.

## Standards

`perimeter` ne réinvente rien de ce qui existe déjà :

- [`AGENTS.md`](https://agents.md) pour les directives — la mémoire ne remplace pas les instructions.
- [Agent Skills](https://code.claude.com/docs/en/skills) (`SKILL.md`) pour le procédural — une procédure s'exécute, elle ne se mémorise pas.
- [MCP](https://modelcontextprotocol.io) pour exposer la recherche à d'autres agents que celui qui a écrit.
- [ADR / MADR](https://adr.github.io) pour les décisions — le registre `decision` en reprend l'intention.
- [Diátaxis](https://diataxis.fr) pour la documentation humaine adjacente.
- JSON Schema pour le frontmatter, versionné et publié.

Et volontairement **pas** de base vectorielle. Sur du markdown atomique, un index et un
`grep` suffisent jusqu'à plusieurs milliers de notes. Le goulot d'étranglement n'est
jamais la récupération, c'est la curation : une recherche sémantique sur un corpus non
curé retrouve plus vite des informations fausses.

## Licence

MIT
