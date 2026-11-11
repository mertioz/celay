# Publication Microsoft Store

La fiche Celay est associée à l’identifiant `9PN2HQ4P50KT`. Les valeurs publiques du paquet sont dans `identity.json`.

Construire les exécutables Windows x64 et ARM64 avec `build.py`, puis préparer le bundle avec le Windows SDK.

```powershell
python store/package.py --makeappx "C:/Program Files (x86)/Windows Kits/10/bin/10.0.28000.0/x64/MakeAppx.exe" --binaries .build --output versions/store
```

Le paquet cible Windows Desktop 10.0.19041 ou ultérieur. Il utilise les autorisations de bureau `runFullTrust` et `unvirtualizedResources` pour lancer Codex avec le compte local de l’utilisateur et partager la file Celay avec la version portable. Sous Windows 11, l’exception de virtualisation est limitée au dossier de configuration Celay.

Microsoft fournit la signature des paquets MSIX publiés via le Store. Le bundle destiné à la soumission est préparé avec MakeAppx et ne contient pas de certificat privé. [Documentation Microsoft](https://learn.microsoft.com/en-us/windows/apps/publish/get-started)
