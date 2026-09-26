# perimeter

Outillage qui répond à « un agent peut-il agir sur ce périmètre ? » — lint du
corpus, preuves rejouées, registre de sources, porte de readiness.

## Language

**Avis ambiant**:
Résumé passif, agrégé par catégorie, des verdicts déjà produits par les
détecteurs existants (cohérence du registre, lint du corpus, fiabilité des
faits confirmés). Surfacé sur toute sous-commande via la résolution
partagée du registre, sans exécuter de sonde ni introduire de nouvelle
détection — il ne fait que rendre visible ce que `lint`/`reliability
check`/`perimeter` savaient déjà. À la demande, sur tout le périmètre plutôt
qu'en passager d'une sous-commande : voir [[Coverage]].
_Avoid_: Dérive (déjà pris par `reliability.Drifted`, un verdict précis sur
une référence, pas un résumé), Finding (le constat unitaire de lint),
Issue (le constat de cohérence du registre), canal B (raccourci de
conversation, jamais écrit ailleurs que là)

**Corpus externe**:
Un corpus de documentation (vault Obsidian, catalogue produit, etc.) qui
n'adopte pas le schéma perimeter — pas de registre, pas de rôle. Déclaré de
façon minimale dans une section `corpus_externes:` du registre (un chemin et
un type texte libre, purement descriptif, jamais fonctionnel). Le mécanisme
qui le concerne (callouts `[!verification]-`, `perctl audit`) fonctionne sur
des notes brutes, sans jamais imposer l'adoption du schéma perimeter.
Alimente [[Coverage]].
_Avoid_: Corpus (tout court — déjà pris par le corpus schémé chargé depuis un
registre, un concept distinct), Registre (le corpus externe n'en a pas, par
définition)

**Coverage**:
La vue agrégée, à la demande, de l'état de tout le périmètre d'un
utilisateur : couverture des six rôles du registre, santé du corpus, et
fiabilité — index de fiabilité schémé et callouts de chaque [[Corpus
externe]] déclaré. Porte un verdict exploitable en CI, pas seulement de
l'information — à la différence de l'[[Avis ambiant]], passager silencieux
de chaque sous-commande, coverage est une commande dédiée qu'on invoque
explicitement pour juger l'ensemble du périmètre (`perctl coverage`,
[#85](https://github.com/UnPoilTefal/perimeter/issues/85)).
_Avoid_: Status (nom envisagé puis écarté — trop générique, chaque outil CLI
lui donne un sens différent), Couverture (le nom retenu reste anglais, comme
les autres sous-commandes : `perimeter`, `readiness`, `reliability`)
