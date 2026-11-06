# Codex Markdown Extractor

Un clic sur le titre d’une discussion Codex télécharge son texte en Markdown. Les discussions archivées apparaissent dans la liste et les images jointes sont signalées dans l’export.

## Télécharger et lancer

1. Téléchargez l’archive de votre système ci-dessous ou prenez l’[archive complète](https://github.com/mertioz/codex-md-extractor/releases/latest/download/Codex-Markdown-Windows-macOS-Linux.zip).
2. Extrayez l’archive. Si vous avez pris l’archive complète, ouvrez le dossier `Windows` ou extrayez aussi l’archive macOS ou Linux de votre machine.
3. Lancez le programme indiqué, puis cliquez sur le titre d’une discussion.

| Système | Télécharger | Lancer après extraction |
| --- | --- | --- |
| Windows 10 ou 11 | [ZIP Windows](https://github.com/mertioz/codex-md-extractor/releases/latest/download/Codex-Markdown-Windows.zip) | Double-cliquez sur `Codex-Markdown.exe` |
| macOS 12 ou ultérieur, Mac Apple Silicon | [ZIP Mac Apple Silicon](https://github.com/mertioz/codex-md-extractor/releases/latest/download/macOS-Apple-Silicon.zip) | Ouvrez `Codex Markdown.app` |
| macOS 12 ou ultérieur, Mac Intel | [ZIP Mac Intel](https://github.com/mertioz/codex-md-extractor/releases/latest/download/macOS-Intel.zip) | Ouvrez `Codex Markdown.app` |
| Linux x86-64, PC Intel ou AMD | [Archive Linux x86-64](https://github.com/mertioz/codex-md-extractor/releases/latest/download/Linux-x86_64.tar.gz) | Lancez `./codex-markdown` dans le dossier extrait |
| Linux ARM 64 bits | [Archive Linux ARM64](https://github.com/mertioz/codex-md-extractor/releases/latest/download/Linux-ARM64.tar.gz) | Lancez `./codex-markdown` dans le dossier extrait |

Sous Windows, le fichier arrive dans **Téléchargements**. Sous macOS et Linux, l’interface s’ouvre dans le navigateur par défaut, qui choisit le dossier de téléchargement. Sous Linux, si le navigateur ne s’ouvre pas automatiquement, copiez dans le navigateur l’adresse affichée dans le terminal.

L’application macOS est distribuée sans notarisation Apple. Au premier lancement, macOS peut demander de l’autoriser dans **Réglages Système → Confidentialité et sécurité**. [Aide Apple](https://support.apple.com/fr-fr/102445)

Le programme lit l’historique Codex présent sur l’ordinateur, dans le dossier `.codex` du compte courant ou celui indiqué par `CODEX_HOME`. Il travaille en local et en lecture seule.

## Compiler le projet

Le dossier `src/windows` contient l’édition native Windows. Le dossier `src/portable` contient l’édition macOS et Linux.

Depuis Windows, avec Python 3.10+ et Go 1.26 disponibles, exécutez `python build.py`. Le script compile les éditions et prépare les archives dans `versions`. On peut aussi lancer `go build` depuis `src/portable` sous macOS ou Linux.

## Licence

Le code du projet est sous [licence MIT](LICENSE). Les archives macOS et Linux incluent les avis de licence des dépendances dans `Licences.txt`.
