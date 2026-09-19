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
check`/`perimeter` savaient déjà.
_Avoid_: Dérive (déjà pris par `reliability.Drifted`, un verdict précis sur
une référence, pas un résumé), Finding (le constat unitaire de lint),
Issue (le constat de cohérence du registre), canal B (raccourci de
conversation, jamais écrit ailleurs que là)
