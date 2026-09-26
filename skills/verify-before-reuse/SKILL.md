---
name: verify-before-reuse
description: Vérifier la fiabilité d'un fait avant de le reprendre comme prémisse, quand la session référence explicitement une note d'un corpus audité (chemin lu, wikilink cité). À invoquer avant de construire sur ce fait, jamais sur un simple air de déjà-vu.
---

# verify-before-reuse

Ce skill décide **s'il faut construire sur un fait**, avant de le faire. Il se
déclenche uniquement quand la session référence **explicitement** un chemin
ou un lien vers une note d'un corpus audité (un fichier lu, un wikilink cité,
« d'après `<note>` ») — jamais sur un signal flou du genre « ça ressemble à
une décision passée ». Ce dernier est le même genre de bruit que le signal de
contenu déjà écarté ailleurs sur ce projet (voir `harvest`) : se limiter à une
référence traçable, pas à une impression.

Corpus schémés (registre `perimeter.yml`) et corpus non-schémés (vault
Obsidian, catalogue produit sans schéma perimeter) ne partagent pas le même
mécanisme de fiabilité :

- Un corpus **schémé** est vérifié par `perctl reliability check <ref>` — un
  index JSONL externe, ancré par empreinte de ligne.
- Un corpus **non-schémé** n'a ni registre ni index : le seul marqueur de
  fiabilité est un callout `[!verification]-` inséré **dans le texte de la
  note elle-même**, écrit par `perctl audit` ou à la main. Ce skill couvre ce
  second cas — il ne remplace ni n'étend `perctl reliability check`.

## Marche à suivre

1. Lire la note référencée (ou la section pertinente) et chercher un callout
   `> [!verification]-` juste après la ligne qui porte le fait visé.
2. Lire son champ `**état**` :

   | état trouvé | action |
   |---|---|
   | aucun callout | poursuivre normalement — jamais audité n'est un état légitime, pas une erreur |
   | `confirmé` | poursuivre normalement |
   | `à valider` | avertir l'utilisateur du doute (citer le callout : date, origine) et attendre sa confirmation avant de construire sur ce fait |
   | `invalidé` | bloquer — refuser de prendre ce fait pour prémisse tant que l'utilisateur n'a pas explicitement tranché |

3. Ne jamais faire passer soi-même un callout de `à valider` à `confirmé` ou
   `invalidé` : cette transition n'appartient qu'à un humain, en éditant le
   callout à la main dans la note — exactement comme une proposition d'agent
   ailleurs sur ce projet arrive en `proposed`, jamais en `canon`.
