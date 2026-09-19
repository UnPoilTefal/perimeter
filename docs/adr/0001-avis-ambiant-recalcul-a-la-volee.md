# Avis ambiant : recalcul à la volée, pas de cache

Le résumé ambiant (rôles/sources incohérents, liens cassés, ratio de
péremption, faits dérivés) doit être lu en synchrone à chaque sous-commande
perctl, sans en dégrader la latence. On a décidé de le recalculer à chaque
appel plutôt que de le stocker dans un fichier de cache tenu à jour par un
passage séparé (CI ou local) : la résolution du registre, le lint
structurel et la lecture de `perimeter.reliability.jsonl` n'exécutent
aucune sonde tierce (`--allow-exec` reste requis ailleurs pour ça) — rien
ne justifie de mettre en cache un calcul qui n'a pas de coût d'exécution
externe.

## Considered Options

Un cache central alimenté soit par CI (sources avec remote et webhook),
soit localement (sources sans remote, comme un vault Obsidian) a été
envisagé et écarté : ça aurait exigé de trancher, par source, *qui*
rafraîchit et *où* atterrit le résultat, avant même d'avoir produit un
seul octet de valeur. Le recalcul à la volée dissout la question entière.

## Consequences

Si un futur détecteur nécessite une sonde coûteuse ou une exécution
externe pour nourrir l'avis ambiant, cette décision ne tient plus telle
quelle — il faudra alors réintroduire une forme de cache, et la question
CI/local écartée ici redeviendrait pertinente.
