===============================
      Spiralydata
===============================

Spiralydata est un logiciel de synchronisation de fichiers en temps réel 
entre plusieurs machines via une connexion réseau locale ou Internet. 
Il permet à un ou plusieurs utilisateurs de se connecter à un serveur 
(hôte) et de partager automatiquement des fichiers dans un dossier spécifique.

--------------------------------------------------------------------------------
Fonctionnalités principales :

1. Synchronisation bidirectionnelle :
   - Les fichiers ajoutés, modifiés ou supprimés sont automatiquement 
     synchronisés entre le client et le serveur.
   - Détection des renommages de fichiers pour éviter les doublons.

2. Multi-utilisateur :
   - Plusieurs clients peuvent se connecter au même serveur.
   - Chaque client est isolé mais reçoit les mises à jour en temps réel.

3. Authentification par ID :
   - Chaque serveur génère un ID unique (6 chiffres).
   - Les clients doivent fournir cet ID pour se connecter.
   - Tentative échouée si l’ID est incorrect, avec possibilité de réessayer.

4. Robustesse :
   - Scanner périodique pour rattraper les fichiers manqués.
   - Gestion des suppressions et modifications simultanées.
   - Protection contre les boucles d’envoi (évite les doubles synchronisations).

5. Cross-platform :
   - Fonctionne sur Windows et Linux.
   - Développé en Go pour exécutions rapides et portables.

--------------------------------------------------------------------------------
Installation et setup :

Prérequis :
- Go installé (https://golang.org/doc/install)
- Windows 10/11 ou Linux Ubuntu 20.04+
- Accès réseau entre clients et serveur

1. Télécharger le projet depuis GitHub :

   git clone https://github.com/Spiralyfox/Spiralydata.git
   cd Spiralydata

2. Lancez le setup (Windows ou Linux)

3. Lancer le logiciel :

   Un .exe sera compilé (et une version Linux)

   ⚠️ Important : Si vous voulez que des clients sur Internet 
   se connectent, configurez la **redirection de port** sur votre box 
   pour que le port choisi pointe vers votre machine hôte.

4. Arrêter le programme :
   - Tapez `x` puis Entrée dans la console pour arrêter le serveur ou le client.

--------------------------------------------------------------------------------
Structure du projet :

- client.go          : code client
- server.go          : code serveur
- main.go            : gestion du mode host/client
- types.go           : structures de données (FileChange etc.)
- utils.go           : fonctions utilitaires
- config.go          : configurations globales
- setup_linux.sh     : script d’installation Linux
- setup_windows.bat  : script d’installation Windows
- Spiralydata        : dossier synchronisé par défaut

--------------------------------------------------------------------------------
Notes importantes :

- Le dossier par défaut pour la synchronisation est `./Spiralydata`.
- Les clients doivent avoir une connexion stable au serveur pour une synchronisation efficace.
- En cas de modification simultanée d’un même fichier, la dernière modification détectée sera appliquée.
- L’authentification via ID garantit que seuls les clients autorisés peuvent se connecter.
- Pour un usage sur Internet, assurez-vous que le port choisi est ouvert et redirigé correctement sur votre box/routeur.

--------------------------------------------------------------------------------
Support :

Pour toute question ou problème :
- Pseudo : Spiralyfox
- Email : dauriacmatteo@gmail.com
- GitHub : https://github.com/Spiralyfox/Spiralydata

--------------------------------------------------------------------------------
Merci d’utiliser Spiralydata !
