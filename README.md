# Celay

Celay affiche tes discussions Codex locales. Clique sur un titre pour **télécharger la discussion en Markdown** ou **programmer l’envoi d’un message** dans cette même discussion.

## Télécharger et lancer

1. Télécharge l’archive de ton système dans les [versions de Celay](https://github.com/mertioz/celay/releases/latest).
2. Extrais-la, puis lance le programme indiqué ci-dessous. L’interface s’ouvre dans ton navigateur.
3. Clique sur une discussion pour afficher les deux options.

| Système | Télécharger | Lancer après extraction |
| --- | --- | --- |
| Windows 10 ou 11, PC Intel ou AMD | [Windows x64](https://github.com/mertioz/celay/releases/latest/download/Celay-Windows-x64.zip) | Ouvre `Celay.exe` |
| Windows 10 ou 11, PC ARM 64 bits | [Windows ARM64](https://github.com/mertioz/celay/releases/latest/download/Celay-Windows-ARM64.zip) | Ouvre `Celay.exe` |
| macOS 12 ou ultérieur, Mac Apple Silicon | [Mac Apple Silicon](https://github.com/mertioz/celay/releases/latest/download/Celay-macOS-Apple-Silicon.zip) | Ouvre `Celay.app` |
| macOS 12 ou ultérieur, Mac Intel | [Mac Intel](https://github.com/mertioz/celay/releases/latest/download/Celay-macOS-Intel.zip) | Ouvre `Celay.app` |
| Linux x86-64, PC Intel ou AMD | [Linux x86-64](https://github.com/mertioz/celay/releases/latest/download/Celay-Linux-x86_64.tar.gz) | Lance `./celay` dans le dossier extrait |
| Linux ARM 64 bits | [Linux ARM64](https://github.com/mertioz/celay/releases/latest/download/Celay-Linux-ARM64.tar.gz) | Lance `./celay` dans le dossier extrait |

L’[archive complète](https://github.com/mertioz/celay/releases/latest/download/Celay-Windows-macOS-Linux.zip) regroupe les six éditions. Sous Linux, si le navigateur ne s’ouvre pas automatiquement, copie l’adresse affichée dans le terminal. L’application macOS est distribuée sans notarisation Apple. Au premier lancement, macOS peut demander de l’autoriser dans **Réglages Système → Confidentialité et sécurité**. [Aide Apple](https://support.apple.com/fr-fr/102445)

## Programmer des messages

1. Sélectionne une discussion et clique sur **Envoyer un message**.
2. Écris ton prompt, choisis une date et une heure locales, puis clique sur **Programmer l’envoi**.
3. Répète l’opération pour ajouter autant de messages que tu veux, dans la même discussion ou dans d’autres. La liste affiche leur statut et permet d’annuler les envois en attente.

Pour envoyer, Celay utilise Codex installé et connecté sur ta machine. Tu peux indiquer le chemin absolu de l’exécutable avec la variable `CELAY_CODEX` si la détection automatique échoue. L’envoi reprend la discussion existante et utilise les réglages de Codex. Celay décline les demandes d’autorisation interactives. Pour programmer un message dans une archive, désarchive-la d’abord dans Codex. [Documentation Codex App Server](https://learn.chatgpt.com/docs/app-server)

Tu peux fermer l’onglet. Pour respecter l’heure choisie, laisse Celay en marche et l’ordinateur éveillé. Sous Linux, laisse aussi le terminal qui a lancé Celay ouvert. Après un redémarrage, relance Celay pour reprendre les envois en attente, y compris ceux dont l’heure est passée.

Les messages d’une même discussion passent l’un après l’autre. Si un autre processus Codex garde la discussion occupée, Celay attend qu’elle soit libérée avant d’envoyer le message.

## Télécharger une discussion

Clique sur **Télécharger**. Le navigateur enregistre le fichier `.md` dans son dossier de téléchargement. Les discussions archivées restent téléchargeables et les images jointes sont signalées dans le texte.

Celay lit l’historique dans le dossier `.codex` du compte courant ou celui indiqué par `CODEX_HOME`. Il enregistre la file d’envoi séparément, dans le dossier de configuration local de Celay. Le service de l’interface est limité à l’ordinateur où il tourne.

## Compiler

Avec Python 3.10+ et Go 1.26, exécute `python build.py`. Le script compile les six éditions et prépare les archives dans `versions`. Pour compiler uniquement pour la machine actuelle, lance `go build` depuis `src/portable`.

## Licence

Le code est sous [licence MIT](LICENSE). Les archives incluent les avis de licence des dépendances.
