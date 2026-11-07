# Celay

Celay est une application de bureau pour tes discussions Codex locales. Clique sur un titre pour **télécharger la discussion en Markdown** ou **programmer l’envoi d’un message** dans cette même discussion.

## Télécharger et lancer

1. Télécharge l’archive de ton système dans les [versions de Celay](https://github.com/mertioz/celay/releases/latest).
2. Extrais l’archive si nécessaire, puis lance le programme indiqué ci-dessous. Celay s’ouvre dans sa propre fenêtre.
3. Clique sur une discussion pour afficher les deux options.

| Système | Télécharger | Lancer après extraction |
| --- | --- | --- |
| Windows 10 ou 11, PC Intel ou AMD | [Windows x64](https://github.com/mertioz/celay/releases/latest/download/Celay-Windows-x64.zip) | Ouvre `Celay.exe` |
| Windows 10 ou 11, PC ARM 64 bits | [Windows ARM64](https://github.com/mertioz/celay/releases/latest/download/Celay-Windows-ARM64.zip) | Ouvre `Celay.exe` |
| macOS 12 ou ultérieur, Mac Apple Silicon | [Mac Apple Silicon](https://github.com/mertioz/celay/releases/latest/download/Celay-macOS-Apple-Silicon.zip) | Ouvre `Celay.app` |
| macOS 12 ou ultérieur, Mac Intel | [Mac Intel](https://github.com/mertioz/celay/releases/latest/download/Celay-macOS-Intel.zip) | Ouvre `Celay.app` |
| Linux x86-64, PC Intel ou AMD | [AppImage x86-64](https://github.com/mertioz/celay/releases/latest/download/Celay-Linux-x86_64.AppImage) | Autorise l’exécution du fichier, puis ouvre-le |
| Linux ARM 64 bits | [AppImage ARM64](https://github.com/mertioz/celay/releases/latest/download/Celay-Linux-ARM64.AppImage) | Autorise l’exécution du fichier, puis ouvre-le |

L’[archive complète](https://github.com/mertioz/celay/releases/latest/download/Celay-Windows-macOS-Linux.zip) regroupe les six éditions. Sous Linux, tu peux rendre l’AppImage exécutable dans les propriétés du fichier ou avec `chmod +x Celay-Linux-*.AppImage`. Les versions proposent aussi des archives `.tar.gz` avec un exécutable `celay` pour les systèmes sans prise en charge d’AppImage.

L’application macOS est distribuée sans notarisation Apple. Au premier lancement, macOS peut demander de l’autoriser dans **Réglages Système → Confidentialité et sécurité**. [Aide Apple](https://support.apple.com/fr-fr/102445)

Pour passer de Celay 2.0 à la version bureau, quitte d’abord l’ancienne version. Les envois enregistrés sont repris par la nouvelle.

## Programmer des messages

1. Sélectionne une discussion et clique sur **Envoyer un message**.
2. Écris ton prompt, choisis une date et une heure locales, puis clique sur **Programmer l’envoi**.
3. Répète l’opération pour ajouter autant de messages que tu veux, dans la même discussion ou dans d’autres. Pour retirer un envoi, sélectionne-le dans **Envois programmés**, puis clique sur **Supprimer**. Un message retiré de la file en attente ne partira pas. Pour un envoi déjà terminé, le bouton efface seulement l’entrée dans Celay.

Pour envoyer, Celay utilise Codex installé et connecté sur ta machine. Tu peux indiquer le chemin absolu de l’exécutable avec la variable `CELAY_CODEX` si la détection automatique échoue. L’envoi reprend la discussion existante et utilise les réglages de Codex. Celay décline les demandes d’autorisation interactives. Pour programmer un message dans une archive, désarchive-la d’abord dans Codex. [Documentation Codex App Server](https://learn.chatgpt.com/docs/app-server)

La croix masque la fenêtre quand il reste des envois. Pour la rouvrir, utilise l’icône de Celay dans la zone de notification ou relance le programme. Pour respecter l’heure choisie, garde l’ordinateur éveillé. Après un redémarrage ou après avoir choisi **Quitter**, relance Celay pour reprendre les envois en attente, y compris ceux dont l’heure est passée.

Les messages d’une même discussion passent l’un après l’autre. Si un autre processus Codex garde la discussion occupée, Celay attend qu’elle soit libérée avant d’envoyer le message.

## Télécharger une discussion

Clique sur **Télécharger**. Celay enregistre le fichier `.md` dans **Téléchargements** et ajoute un numéro si le nom existe déjà. Les discussions archivées restent téléchargeables et les images jointes sont signalées dans le texte.

Celay lit l’historique dans le dossier `.codex` du compte courant ou celui indiqué par `CODEX_HOME`. Il enregistre la file d’envoi séparément dans son dossier de configuration local.

## Compiler

Avec Python 3.10+, Go 1.26 et les outils de compilation Fyne, exécute `python build.py` pour créer l’archive de ta machine dans `versions`. Le workflow **Desktop builds** compile les six éditions. [Prérequis Fyne](https://docs.fyne.io/started/quick/)

## Licence

Le code est sous [licence MIT](LICENSE). Les archives incluent les avis de licence des dépendances.
