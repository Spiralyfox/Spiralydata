#!/bin/bash

# Créer le dossier de logs
mkdir -p setup_crash_log
LOGFILE="setup_crash_log/setup_$(date +%Y%m%d_%H%M%S).log"

log() {
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] $1" | tee -a "$LOGFILE"
}

log "Démarrage du script setup_linux.sh"
log "Système: $(uname -a)"

echo ""
echo "========================================="
echo "  Installation Automatique Spiraly"
echo "========================================="
echo ""
echo "[INFO] Logs sauvegardés dans: $LOGFILE"
echo ""

# Détecter la distribution Linux
if [ -f /etc/os-release ]; then
    . /etc/os-release
    DISTRO=$ID
    log "Distribution détectée: $DISTRO ($PRETTY_NAME)"
else
    DISTRO="unknown"
    log "ATTENTION: Distribution inconnue"
fi

echo "Distribution detectee: $DISTRO"
echo ""

# Fonction pour vérifier si une commande existe
command_exists() {
    command -v "$1" >/dev/null 2>&1
}

# Vérification des droits sudo
log "Vérification des droits sudo..."
if [ "$EUID" -eq 0 ]; then 
    SUDO=""
    log "Script exécuté en tant que root"
else 
    SUDO="sudo"
    if ! command_exists sudo; then
        log "ERREUR: sudo n'est pas disponible"
        echo "ERREUR: Ce script necessite sudo pour installer les dependances."
        echo "Connectez-vous en tant que root ou installez sudo."
        echo ""
        echo "Consultez les logs: $LOGFILE"
        exit 1
    fi
    log "sudo est disponible"
fi

echo "[1/5] Verification et installation de Go..."
log "[1/5] Vérification de Go..."
if command_exists go; then
    echo "Go est deja installe."
    go version
    log "Go déjà installé: $(go version)"
else
    echo "Installation de Go..."
    log "Go non trouvé, tentative d'installation..."
    
    case $DISTRO in
        ubuntu|debian|linuxmint|pop|elementary)
            log "Installation via apt-get..."
            $SUDO apt-get update >> "$LOGFILE" 2>&1
            $SUDO apt-get install -y golang-go >> "$LOGFILE" 2>&1
            ;;
        fedora|rhel|centos|rocky|almalinux)
            log "Installation via dnf..."
            $SUDO dnf install -y golang >> "$LOGFILE" 2>&1
            ;;
        arch|manjaro|endeavouros)
            log "Installation via pacman..."
            $SUDO pacman -S --noconfirm go >> "$LOGFILE" 2>&1
            ;;
        opensuse*|sles)
            log "Installation via zypper..."
            $SUDO zypper install -y go >> "$LOGFILE" 2>&1
            ;;
        *)
            log "ERREUR: Distribution non supportée pour installation automatique"
            echo "Distribution non supportee pour installation automatique."
            echo "Installez Go manuellement: https://golang.org/download/"
            echo ""
            echo "Consultez les logs: $LOGFILE"
            exit 1
            ;;
    esac
    
    if [ $? -ne 0 ]; then
        log "ERREUR: Échec de l'installation de Go"
        echo "ERREUR: Impossible d'installer Go"
        echo "Installez Go manuellement: https://golang.org/download/"
        echo ""
        echo "Consultez les logs: $LOGFILE"
        exit 1
    fi
    
    log "Go installé avec succès"
    echo "Go installe avec succes!"
fi

echo ""
echo "[2/5] Verification et installation de GCC..."
log "[2/5] Vérification de GCC..."
if command_exists gcc; then
    echo "GCC est deja installe."
    gcc --version | head -n1
    log "GCC déjà installé: $(gcc --version | head -n1)"
else
    echo "Installation de GCC..."
    log "GCC non trouvé, tentative d'installation..."
    
    case $DISTRO in
        ubuntu|debian|linuxmint|pop|elementary)
            log "Installation de GCC via apt-get..."
            $SUDO apt-get install -y gcc >> "$LOGFILE" 2>&1
            ;;
        fedora|rhel|centos|rocky|almalinux)
            log "Installation de GCC via dnf..."
            $SUDO dnf install -y gcc >> "$LOGFILE" 2>&1
            ;;
        arch|manjaro|endeavouros)
            log "Installation de GCC via pacman..."
            $SUDO pacman -S --noconfirm gcc >> "$LOGFILE" 2>&1
            ;;
        opensuse*|sles)
            log "Installation de GCC via zypper..."
            $SUDO zypper install -y gcc >> "$LOGFILE" 2>&1
            ;;
        *)
            log "ERREUR: Distribution non supportée"
            echo "Distribution non supportee."
            echo "Consultez les logs: $LOGFILE"
            exit 1
            ;;
    esac
    
    if [ $? -ne 0 ]; then
        log "ERREUR: Échec de l'installation de GCC"
        echo "ERREUR: Impossible d'installer GCC"
        echo "Consultez les logs: $LOGFILE"
        exit 1
    fi
    
    log "GCC installé avec succès"
    echo "GCC installe avec succes!"
fi

echo ""
echo "[3/5] Installation de pkg-config..."
log "[3/5] Vérification de pkg-config..."
if command_exists pkg-config; then
    echo "pkg-config est deja installe."
    log "pkg-config déjà installé"
else
    echo "Installation de pkg-config..."
    log "Installation de pkg-config..."
    
    case $DISTRO in
        ubuntu|debian|linuxmint|pop|elementary)
            $SUDO apt-get install -y pkg-config >> "$LOGFILE" 2>&1
            ;;
        fedora|rhel|centos|rocky|almalinux)
            $SUDO dnf install -y pkgconfig >> "$LOGFILE" 2>&1
            ;;
        arch|manjaro|endeavouros)
            $SUDO pacman -S --noconfirm pkgconf >> "$LOGFILE" 2>&1
            ;;
        opensuse*|sles)
            $SUDO zypper install -y pkg-config >> "$LOGFILE" 2>&1
            ;;
    esac
    
    log "pkg-config installé"
    echo "pkg-config installe!"
fi

echo ""
echo "[4/5] Installation des dependances systeme pour Fyne..."
log "[4/5] Installation des dépendances système pour Fyne..."

case $DISTRO in
    ubuntu|debian|linuxmint|pop|elementary)
        echo "Installation des dependances pour Ubuntu/Debian..."
        log "Installation des dépendances Fyne (Ubuntu/Debian)..."
        $SUDO apt-get update >> "$LOGFILE" 2>&1
        $SUDO apt-get install -y libgl1-mesa-dev libxcursor-dev libxrandr-dev libxinerama-dev libxi-dev libxxf86vm-dev libglfw3-dev >> "$LOGFILE" 2>&1
        ;;
    fedora|rhel|centos|rocky|almalinux)
        echo "Installation des dependances pour Fedora/RHEL..."
        log "Installation des dépendances Fyne (Fedora/RHEL)..."
        $SUDO dnf install -y mesa-libGL-devel libXcursor-devel libXrandr-devel libXinerama-devel libXi-devel libXxf86vm-devel glfw-devel >> "$LOGFILE" 2>&1
        ;;
    arch|manjaro|endeavouros)
        echo "Installation des dependances pour Arch..."
        log "Installation des dépendances Fyne (Arch)..."
        $SUDO pacman -S --noconfirm libgl libxcursor libxrandr libxinerama libxi libxxf86vm glfw-x11 >> "$LOGFILE" 2>&1
        ;;
    opensuse*|sles)
        echo "Installation des dependances pour openSUSE..."
        log "Installation des dépendances Fyne (openSUSE)..."
        $SUDO zypper install -y Mesa-libGL-devel libXcursor-devel libXrandr-devel libXinerama-devel libXi-devel glfw-devel >> "$LOGFILE" 2>&1
        ;;
    *)
        log "ATTENTION: Distribution non reconnue pour les dépendances"
        echo "ATTENTION: Distribution non reconnue."
        echo "Installez manuellement les dependances OpenGL et X11."
        ;;
esac

if [ $? -ne 0 ]; then
    log "AVERTISSEMENT: Problème lors de l'installation des dépendances système"
    echo "ERREUR lors de l'installation des dependances systeme"
    echo "Certaines dependances peuvent manquer."
    echo "La compilation peut echouer."
fi

echo ""
echo "[5/5] Installation des dependances Go et compilation..."
log "[5/5] Installation des dépendances Go et compilation..."
echo ""

# Nettoyer les anciens fichiers
log "Nettoyage des anciens fichiers go.mod et go.sum..."
rm -f go.mod go.sum

echo "Initialisation du module..."
log "Exécution: go mod init spiralydata"
go mod init spiralydata >> "$LOGFILE" 2>&1
if [ $? -ne 0 ]; then
    log "ERREUR: go mod init a échoué"
    echo "ERREUR: Impossible d'initialiser le module Go"
    echo "Consultez les logs: $LOGFILE"
    exit 1
fi

echo "Telechargement des dependances Fyne..."
log "Téléchargement des dépendances Fyne..."
go get fyne.io/fyne/v2@latest >> "$LOGFILE" 2>&1
go get fyne.io/fyne/v2/app@latest >> "$LOGFILE" 2>&1
go get fyne.io/fyne/v2/canvas@latest >> "$LOGFILE" 2>&1
go get fyne.io/fyne/v2/container@latest >> "$LOGFILE" 2>&1
go get fyne.io/fyne/v2/layout@latest >> "$LOGFILE" 2>&1
go get fyne.io/fyne/v2/theme@latest >> "$LOGFILE" 2>&1
go get fyne.io/fyne/v2/widget@latest >> "$LOGFILE" 2>&1

echo "Telechargement des autres dependances..."
log "Téléchargement des autres dépendances..."
go get github.com/fsnotify/fsnotify@latest >> "$LOGFILE" 2>&1
go get github.com/gorilla/websocket@latest >> "$LOGFILE" 2>&1

echo "Nettoyage des dependances..."
log "Exécution: go mod tidy"
go mod tidy >> "$LOGFILE" 2>&1

echo ""
echo "Compilation de Spiralydata pour Linux..."
log "Compilation: go build -buildvcs=false -o spiralydata"
go build -buildvcs=false -o spiralydata >> "$LOGFILE" 2>&1

if [ $? -ne 0 ]; then
    log "ERREUR: La compilation a échoué"
    echo ""
    echo "================================================"
    echo "         ERREUR DE COMPILATION"
    echo "================================================"
    echo ""
    echo "Verifiez que toutes les dependances sont installees."
    echo "Consultez les logs: $LOGFILE"
    echo ""
    echo "Dernieres lignes du log:"
    tail -n 20 "$LOGFILE"
    echo ""
    exit 1
fi

chmod +x spiralydata
log "Fichier spiralydata créé avec succès"

echo ""
echo "================================================"
echo "     INSTALLATION TERMINEE AVEC SUCCES!"
echo "================================================"
echo ""
echo "Fichier cree:"
echo "  - spiralydata (Linux - Interface Graphique)"
echo ""
echo "Pour lancer l'application:"
echo "  ./spiralydata              (interface graphique)"
echo "  ./spiralydata --console    (mode console)"
echo ""
echo "Logs sauvegardes dans: $LOGFILE"
echo ""
log "Installation terminée avec succès"