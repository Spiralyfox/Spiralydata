# 📚 Documentation Technique - Spiralydata

---

## 🇫🇷 Français

### 🏗️ Architecture

#### Vue d'ensemble

Spiralydata utilise une architecture client-serveur basée sur WebSocket pour la synchronisation en temps réel.

```
┌─────────────────┐         WebSocket            ┌─────────────────┐
│                 │ ◄──────────────────────────► │                 │
│   HÔTE          │         JSON/Base64          │   CLIENT 1      │
│   (Serveur)     │ ◄──────────────────────────► │                 │
│                 │                              └─────────────────┘
│   Port: 1212    │ ◄──────────────────────────► ┌─────────────────┐
│                 │                              │   CLIENT 2      │
└─────────────────┘                              └─────────────────┘
```

#### Composants principaux

| Fichier | Rôle |
|---------|------|
| `main.go` | Point d'entrée, initialisation |
| `gui.go` | Interface graphique principale |
| `gui_user.go` | Interface client |
| `server.go` | Serveur WebSocket et gestion des connexions |
| `server_handlers.go` | Gestionnaires de messages serveur |
| `client.go` | Client WebSocket et réception des messages |
| `client_operations.go` | Opérations client (envoi, réception, backup) |
| `client_connect.go` | Interface de connexion client |
| `file_explorer.go` | Explorateur de fichiers distant |
| `types.go` | Structures de données partagées |
| `config.go` | Gestion de la configuration |
| `themes.go` | Thèmes de l'interface |
| `logging.go` | Système de logs |
| `utils.go` | Fonctions utilitaires |

### 📡 Protocole de communication

#### Format des messages

Tous les messages sont au format JSON via WebSocket.

**Structure FileChange :**
```json
{
  "filename": "chemin/vers/fichier.txt",
  "op": "create|write|remove|mkdir",
  "content": "base64_encoded_content",
  "origin": "client|server",
  "is_dir": false
}
```

**Opérations disponibles :**
| Opération | Description |
|-----------|-------------|
| `create` | Création d'un nouveau fichier |
| `write` | Modification d'un fichier existant |
| `remove` | Suppression d'un fichier ou dossier |
| `mkdir` | Création d'un dossier |

#### Types de requêtes

| Type | Direction | Description |
|------|-----------|-------------|
| `auth_request` | Client → Serveur | Authentification avec l'identifiant hôte |
| `auth_success` | Serveur → Client | Confirmation de connexion |
| `auth_failed` | Serveur → Client | Échec d'authentification |
| `request_all_files` | Client → Serveur | Demande de tous les fichiers |
| `request_file_tree` | Client → Serveur | Demande de l'arborescence |
| `file_tree_item` | Serveur → Client | Élément de l'arborescence |
| `file_tree_complete` | Serveur → Client | Fin de l'arborescence |
| `download_request` | Client → Serveur | Demande de téléchargement |

### 🔄 Flux de synchronisation

#### Connexion initiale
```
1. Client se connecte au WebSocket
2. Client envoie auth_request avec host_id
3. Serveur vérifie l'identifiant
4. Si OK: auth_success + envoi de tous les fichiers
5. Si KO: auth_failed + fermeture connexion
```

#### Synchronisation temps réel
```
1. Modification détectée par fsnotify (watcher)
2. Lecture du fichier modifié
3. Encodage en Base64
4. Envoi du FileChange via WebSocket
5. Réception par les autres parties
6. Décodage et écriture du fichier
```

#### Processus de Backup
```
1. Scan du serveur (request_file_tree)
2. Comptage des éléments attendus
3. Demande de tous les fichiers (request_all_files)
4. Attente de la réception (monitoring du dossier local)
5. Copie du dossier local vers Backup_Spiralydata_DATE
```

### 📂 Gestion des fichiers

#### Watcher (fsnotify)

Le système surveille récursivement le dossier synchronisé :
- Détection des créations, modifications, suppressions
- Filtrage des événements en double
- Délai anti-rebond pour éviter les envois multiples

#### Encodage des fichiers

- Les fichiers sont lus en binaire
- Encodés en Base64 pour le transport JSON
- Décodés à la réception avant écriture

#### Gestion des conflits

- Timestamps comparés pour déterminer la version la plus récente
- Fichiers `.conflict` créés en cas de conflit non résolu

### 🎨 Interface graphique

#### Framework utilisé
- **Fyne v2** : Toolkit Go multiplateforme

#### Thèmes disponibles
| Thème | Description |
|-------|-------------|
| Clair | Fond blanc, texte sombre |
| Sombre | Fond sombre, texte clair |
| Personnalisé | Couleurs configurables |

#### Composants UI
- `StatusBar` : Barre de statut avec indicateur de connexion
- `LogPanel` : Panneau de logs scrollable
- `FileExplorer` : Explorateur de fichiers avec navigation
- `ControlButtons` : Boutons d'action (Envoyer, Recevoir, etc.)

### 🔐 Sécurité

#### Authentification
- Identifiant hôte généré aléatoirement (6 chiffres)
- Validation obligatoire à la connexion
- Connexion refusée si identifiant incorrect

#### Limitations
- Pas de chiffrement des données en transit (WebSocket non-TLS)
- Recommandé pour usage en réseau local uniquement

### ⚡ Performance

#### Optimisations
- Délais entre les envois pour éviter la surcharge
- Buffers WebSocket augmentés (10MB)
- Traitement asynchrone des fichiers
- Compression implicite via Base64

#### Limites recommandées
| Paramètre | Valeur recommandée |
|-----------|-------------------|
| Taille max fichier | 50 MB |
| Nombre de fichiers | < 1000 |
| Clients simultanés | < 10 |

### 🛠️ Compilation

#### Dépendances
```go
require (
    fyne.io/fyne/v2 v2.7.2
    github.com/gorilla/websocket v1.5.3
    github.com/fsnotify/fsnotify v1.7.0
)
```

#### Commandes de build
```bash
# Windows (sans console)
go build -ldflags "-H=windowsgui" -o spiralydata.exe .

# Linux
go build -o spiralydata .

# Avec debug
go build -o spiralydata_debug.exe .
```

### 📊 Structures de données

#### Client
```go
type Client struct {
    ws              *websocket.Conn  // Connexion WebSocket
    localDir        string           // Dossier local
    isProcessing    bool             // Opération en cours
    autoSync        bool             // Mode auto activé
    downloadActive  bool             // Téléchargement en cours
    downloadChan    chan FileChange  // Canal de téléchargement
    explorerActive  bool             // Explorateur actif
    treeItemsChan   chan FileTreeItemMessage
}
```

#### Server
```go
type Server struct {
    HostID    string                       // Identifiant hôte
    WatchDir  string                       // Dossier surveillé
    Clients   map[*websocket.Conn]string   // Clients connectés
    Upgrader  websocket.Upgrader           // Upgrader HTTP→WS
}
```

---

## 🇬🇧 English

### 🏗️ Architecture

#### Overview

Spiralydata uses a client-server architecture based on WebSocket for real-time synchronization.

```
┌─────────────────┐         WebSocket            ┌─────────────────┐
│                 │ ◄──────────────────────────► │                 │
│   HOST          │         JSON/Base64          │   CLIENT 1      │
│   (Server)      │ ◄──────────────────────────► │                 │
│                 │                              └─────────────────┘
│   Port: 1212    │ ◄──────────────────────────► ┌─────────────────┐
│                 │                              │   CLIENT 2      │
└─────────────────┘                              └─────────────────┘
```

#### Main Components

| File | Role |
|------|------|
| `main.go` | Entry point, initialization |
| `gui.go` | Main graphical interface |
| `gui_user.go` | Client interface |
| `server.go` | WebSocket server and connection management |
| `server_handlers.go` | Server message handlers |
| `client.go` | WebSocket client and message reception |
| `client_operations.go` | Client operations (send, receive, backup) |
| `client_connect.go` | Client connection interface |
| `file_explorer.go` | Remote file explorer |
| `types.go` | Shared data structures |
| `config.go` | Configuration management |
| `themes.go` | Interface themes |
| `logging.go` | Logging system |
| `utils.go` | Utility functions |

### 📡 Communication Protocol

#### Message Format

All messages are in JSON format via WebSocket.

**FileChange Structure:**
```json
{
  "filename": "path/to/file.txt",
  "op": "create|write|remove|mkdir",
  "content": "base64_encoded_content",
  "origin": "client|server",
  "is_dir": false
}
```

**Available Operations:**
| Operation | Description |
|-----------|-------------|
| `create` | Create a new file |
| `write` | Modify an existing file |
| `remove` | Delete a file or folder |
| `mkdir` | Create a folder |

#### Request Types

| Type | Direction | Description |
|------|-----------|-------------|
| `auth_request` | Client → Server | Authentication with host identifier |
| `auth_success` | Server → Client | Connection confirmation |
| `auth_failed` | Server → Client | Authentication failure |
| `request_all_files` | Client → Server | Request all files |
| `request_file_tree` | Client → Server | Request file tree |
| `file_tree_item` | Server → Client | File tree element |
| `file_tree_complete` | Server → Client | End of file tree |
| `download_request` | Client → Server | Download request |

### 🔄 Synchronization Flow

#### Initial Connection
```
1. Client connects to WebSocket
2. Client sends auth_request with host_id
3. Server verifies identifier
4. If OK: auth_success + send all files
5. If KO: auth_failed + close connection
```

#### Real-time Synchronization
```
1. Change detected by fsnotify (watcher)
2. Read modified file
3. Encode to Base64
4. Send FileChange via WebSocket
5. Reception by other parties
6. Decode and write file
```

#### Backup Process
```
1. Server scan (request_file_tree)
2. Count expected elements
3. Request all files (request_all_files)
4. Wait for reception (local folder monitoring)
5. Copy local folder to Backup_Spiralydata_DATE
```

### 📂 File Management

#### Watcher (fsnotify)

The system recursively monitors the synchronized folder:
- Detection of creations, modifications, deletions
- Filtering of duplicate events
- Debounce delay to avoid multiple sends

#### File Encoding

- Files are read in binary
- Encoded in Base64 for JSON transport
- Decoded on reception before writing

#### Conflict Management

- Timestamps compared to determine most recent version
- `.conflict` files created for unresolved conflicts

### 🎨 Graphical Interface

#### Framework Used
- **Fyne v2**: Cross-platform Go toolkit

#### Available Themes
| Theme | Description |
|-------|-------------|
| Light | White background, dark text |
| Dark | Dark background, light text |
| Custom | Configurable colors |

#### UI Components
- `StatusBar`: Status bar with connection indicator
- `LogPanel`: Scrollable log panel
- `FileExplorer`: File explorer with navigation
- `ControlButtons`: Action buttons (Send, Receive, etc.)

### 🔐 Security

#### Authentication
- Randomly generated host identifier (6 digits)
- Mandatory validation on connection
- Connection refused if identifier incorrect

#### Limitations
- No data encryption in transit (non-TLS WebSocket)
- Recommended for local network use only

### ⚡ Performance

#### Optimizations
- Delays between sends to avoid overload
- Increased WebSocket buffers (10MB)
- Asynchronous file processing
- Implicit compression via Base64

#### Recommended Limits
| Parameter | Recommended Value |
|-----------|-------------------|
| Max file size | 50 MB |
| Number of files | < 1000 |
| Simultaneous clients | < 10 |

### 🛠️ Compilation

#### Dependencies
```go
require (
    fyne.io/fyne/v2 v2.7.2
    github.com/gorilla/websocket v1.5.3
    github.com/fsnotify/fsnotify v1.7.0
)
```

#### Build Commands
```bash
# Windows (no console)
go build -ldflags "-H=windowsgui" -o spiralydata.exe .

# Linux
go build -o spiralydata .

# With debug
go build -o spiralydata_debug.exe .
```

### 📊 Data Structures

#### Client
```go
type Client struct {
    ws              *websocket.Conn  // WebSocket connection
    localDir        string           // Local folder
    isProcessing    bool             // Operation in progress
    autoSync        bool             // Auto mode enabled
    downloadActive  bool             // Download in progress
    downloadChan    chan FileChange  // Download channel
    explorerActive  bool             // Explorer active
    treeItemsChan   chan FileTreeItemMessage
}
```

#### Server
```go
type Server struct {
    HostID    string                       // Host identifier
    WatchDir  string                       // Watched folder
    Clients   map[*websocket.Conn]string   // Connected clients
    Upgrader  websocket.Upgrader           // HTTP→WS Upgrader
}
```
