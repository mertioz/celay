# Confidentialité de Celay

Dernière mise à jour le 11 novembre 2026.

Celay est une application de bureau indépendante, publiée par GreatInvention. Elle permet d’exporter les discussions Codex présentes sur l’ordinateur et de programmer des messages à leur intention.

## Données utilisées sur votre ordinateur

Celay lit les titres et l’historique des discussions dans le dossier local de Codex, ou dans le dossier défini par `CODEX_HOME`. Un export Markdown est enregistré dans le dossier Téléchargements de l’utilisateur.

Pour les envois programmés, Celay conserve localement le texte du message, la discussion destinataire, la date prévue, les choix de modèle et de raisonnement, ainsi que le statut de l’envoi. La file est stockée dans le dossier de configuration Celay du compte Windows. Vous pouvez supprimer les envois en attente et les entrées terminées depuis l’application.

Ces fichiers peuvent contenir des informations personnelles que vous avez incluses dans vos discussions ou vos messages.

## Envoi par Codex

À l’heure prévue, Celay utilise l’installation locale de Codex et sa connexion existante pour soumettre le message à la discussion choisie. Codex et le fournisseur de modèle configuré, notamment OpenAI, traitent alors le contenu selon leurs propres conditions et politiques de confidentialité. Les fonctions réseau et de diagnostic de Codex suivent sa configuration.

L’utilisation des fonctions d’envoi nécessite une installation et un compte Codex compatibles. Les conditions et tarifs du fournisseur de modèle s’appliquent.

## Services de Celay

Celay utilise le stockage local pour son fonctionnement. L’application ne met en place aucun serveur de collecte de discussions, de messages programmés ou d’identifiants de compte pour l’éditeur. Elle ne contient pas de publicité ni de mécanisme d’analyse d’audience ajouté par Celay.

Microsoft peut traiter les données relatives à l’installation et à l’utilisation du Microsoft Store selon sa propre politique. GitHub traite les informations que vous choisissez d’y publier, par exemple dans une demande d’assistance.

## Conservation et suppression

Les exports restent dans le dossier que vous avez utilisé. Vous pouvez les supprimer avec les outils de Windows. La suppression d’une entrée dans Celay agit sur son historique local ou sur sa file d’attente. Les messages déjà soumis à Codex suivent les mécanismes de conservation et de suppression de Codex et de son fournisseur.

Les données locales peuvent rester sur l’ordinateur après la désinstallation afin de permettre une réinstallation ou l’utilisation de la version portable. La configuration Windows habituelle est enregistrée dans `%APPDATA%\Celay`. Le dossier peut être différent si vous avez défini `CELAY_DATA_DIR`.

## Assistance

Vous pouvez contacter l’éditeur depuis les [issues du dépôt Celay](https://github.com/mertioz/celay/issues). Évitez d’y publier des conversations privées, des identifiants ou des informations sensibles.
