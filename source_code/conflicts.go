package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// FileVersion représente une version d'un fichier
type FileVersion struct {
	Path       string
	Hash       string
	Size       int64
	ModTime    time.Time
	Content    []byte
	IsLocal    bool
	IsRemote   bool
}

// Conflict représente un conflit de synchronisation
type Conflict struct {
	ID           string
	Path         string
	LocalVersion *FileVersion
	RemoteVersion *FileVersion
	DetectedAt   time.Time
	Resolved     bool
	Resolution   ConflictResolution
	ResolvedAt   time.Time
}

// ConflictResolution contient les détails de la résolution
type ConflictResolution struct {
	Strategy    ConflictStrategy
	KeptVersion string // "local", "remote", "both", "merged"
	NewPath     string // Pour "both" - nouveau chemin du fichier renommé
	MergedHash  string // Pour "merged" - hash du fichier fusionné
}

// ConflictManager gère les conflits
type ConflictManager struct {
	conflicts     map[string]*Conflict
	mu            sync.RWMutex
	history       []*Conflict
	maxHistory    int
	autoResolve   bool
	strategy      ConflictStrategy
	onConflict    func(*Conflict)
}

// NewConflictManager crée un nouveau gestionnaire de conflits
func NewConflictManager() *ConflictManager {
	return &ConflictManager{
		conflicts:   make(map[string]*Conflict),
		history:     make([]*Conflict, 0),
		maxHistory:  100,
		autoResolve: false,
		strategy:    ConflictAskUser,
	}
}

// SetAutoResolve active/désactive la résolution automatique
func (cm *ConflictManager) SetAutoResolve(enabled bool, strategy ConflictStrategy) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.autoResolve = enabled
	cm.strategy = strategy
}

// SetOnConflictCallback définit le callback appelé lors d'un conflit
func (cm *ConflictManager) SetOnConflictCallback(callback func(*Conflict)) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.onConflict = callback
}

// DetectConflict détecte si un fichier est en conflit
func (cm *ConflictManager) DetectConflict(localPath string, remoteHash string, remoteModTime time.Time, remoteSize int64) (*Conflict, bool) {
	// Lire le fichier local
	localInfo, err := os.Stat(localPath)
	if err != nil {
		// Fichier local n'existe pas, pas de conflit
		return nil, false
	}
	
	// Calculer le hash local
	localContent, err := os.ReadFile(localPath)
	if err != nil {
		return nil, false
	}
	localHash := HashData(localContent)
	
	// Si les hash sont identiques, pas de conflit
	if localHash == remoteHash {
		return nil, false
	}
	
	// Si les dates de modification sont différentes des deux côtés, c'est un conflit
	// (le fichier a été modifié localement ET à distance)
	
	cm.mu.Lock()
	defer cm.mu.Unlock()
	
	// Vérifier si un conflit existe déjà pour ce fichier
	if existing, ok := cm.conflicts[localPath]; ok && !existing.Resolved {
		return existing, true
	}
	
	// Créer un nouveau conflit
	conflict := &Conflict{
		ID:   generateConflictID(),
		Path: localPath,
		LocalVersion: &FileVersion{
			Path:    localPath,
			Hash:    localHash,
			Size:    localInfo.Size(),
			ModTime: localInfo.ModTime(),
			Content: localContent,
			IsLocal: true,
		},
		RemoteVersion: &FileVersion{
			Path:     localPath,
			Hash:     remoteHash,
			Size:     remoteSize,
			ModTime:  remoteModTime,
			IsRemote: true,
		},
		DetectedAt: time.Now(),
		Resolved:   false,
	}
	
	cm.conflicts[localPath] = conflict
	
	addLog(fmt.Sprintf("⚠️ Conflit détecté: %s", filepath.Base(localPath)))
	
	// Appeler le callback si défini
	if cm.onConflict != nil {
		go cm.onConflict(conflict)
	}
	
	// Auto-résoudre si activé
	if cm.autoResolve {
		cm.ResolveConflict(conflict.ID, cm.strategy, nil)
	}
	
	return conflict, true
}

// ResolveConflict résout un conflit
func (cm *ConflictManager) ResolveConflict(conflictID string, strategy ConflictStrategy, remoteContent []byte) error {
	cm.mu.Lock()
	
	// Trouver le conflit
	var conflict *Conflict
	for _, c := range cm.conflicts {
		if c.ID == conflictID {
			conflict = c
			break
		}
	}
	
	if conflict == nil {
		cm.mu.Unlock()
		return fmt.Errorf("conflit non trouvé: %s", conflictID)
	}
	
	if conflict.Resolved {
		cm.mu.Unlock()
		return fmt.Errorf("conflit déjà résolu: %s", conflictID)
	}
	
	cm.mu.Unlock()
	
	var resolution ConflictResolution
	resolution.Strategy = strategy
	
	switch strategy {
	case ConflictKeepLocal:
		resolution.KeptVersion = "local"
		addLog(fmt.Sprintf("✅ Conflit résolu (local gardé): %s", filepath.Base(conflict.Path)))
		
	case ConflictKeepRemote:
		resolution.KeptVersion = "remote"
		if remoteContent != nil {
			if err := os.WriteFile(conflict.Path, remoteContent, 0644); err != nil {
				return fmt.Errorf("erreur écriture: %v", err)
			}
		}
		addLog(fmt.Sprintf("✅ Conflit résolu (distant gardé): %s", filepath.Base(conflict.Path)))
		
	case ConflictKeepNewest:
		if conflict.LocalVersion.ModTime.After(conflict.RemoteVersion.ModTime) {
			resolution.KeptVersion = "local"
			addLog(fmt.Sprintf("✅ Conflit résolu (local plus récent): %s", filepath.Base(conflict.Path)))
		} else {
			resolution.KeptVersion = "remote"
			if remoteContent != nil {
				if err := os.WriteFile(conflict.Path, remoteContent, 0644); err != nil {
					return fmt.Errorf("erreur écriture: %v", err)
				}
			}
			addLog(fmt.Sprintf("✅ Conflit résolu (distant plus récent): %s", filepath.Base(conflict.Path)))
		}
		
	case ConflictKeepBoth:
		resolution.KeptVersion = "both"
		// Renommer le fichier local avec un suffixe
		ext := filepath.Ext(conflict.Path)
		base := strings.TrimSuffix(conflict.Path, ext)
		timestamp := time.Now().Format("20060102_150405")
		newPath := fmt.Sprintf("%s_local_%s%s", base, timestamp, ext)
		
		// Copier le fichier local vers le nouveau chemin
		if err := os.WriteFile(newPath, conflict.LocalVersion.Content, 0644); err != nil {
			return fmt.Errorf("erreur copie locale: %v", err)
		}
		
		// Écrire la version distante
		if remoteContent != nil {
			if err := os.WriteFile(conflict.Path, remoteContent, 0644); err != nil {
				return fmt.Errorf("erreur écriture distante: %v", err)
			}
		}
		
		resolution.NewPath = newPath
		addLog(fmt.Sprintf("✅ Conflit résolu (les deux gardés): %s", filepath.Base(conflict.Path)))
		
	case ConflictAutoMerge:
		// Pour les fichiers texte, tenter une fusion simple
		if IsTextFile(conflict.Path) && remoteContent != nil {
			merged, err := SimpleMerge(conflict.LocalVersion.Content, remoteContent)
			if err != nil {
				// Si la fusion échoue, garder les deux
				return cm.ResolveConflict(conflictID, ConflictKeepBoth, remoteContent)
			}
			
			if err := os.WriteFile(conflict.Path, merged, 0644); err != nil {
				return fmt.Errorf("erreur écriture fusionnée: %v", err)
			}
			
			resolution.KeptVersion = "merged"
			resolution.MergedHash = HashData(merged)
			addLog(fmt.Sprintf("✅ Conflit résolu (fusion auto): %s", filepath.Base(conflict.Path)))
		} else {
			// Pour les fichiers binaires, garder le plus récent
			return cm.ResolveConflict(conflictID, ConflictKeepNewest, remoteContent)
		}
		
	default:
		return fmt.Errorf("stratégie non implémentée: %v", strategy)
	}
	
	cm.mu.Lock()
	conflict.Resolved = true
	conflict.Resolution = resolution
	conflict.ResolvedAt = time.Now()
	
	// Ajouter à l'historique
	cm.history = append(cm.history, conflict)
	if len(cm.history) > cm.maxHistory {
		cm.history = cm.history[1:]
	}
	
	// Supprimer des conflits actifs
	delete(cm.conflicts, conflict.Path)
	cm.mu.Unlock()
	
	return nil
}

// GetConflicts retourne tous les conflits non résolus
func (cm *ConflictManager) GetConflicts() []*Conflict {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	
	result := make([]*Conflict, 0, len(cm.conflicts))
	for _, c := range cm.conflicts {
		if !c.Resolved {
			result = append(result, c)
		}
	}
	return result
}

// GetConflictByID retourne un conflit par son ID
func (cm *ConflictManager) GetConflictByID(id string) *Conflict {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	
	for _, c := range cm.conflicts {
		if c.ID == id {
			return c
		}
	}
	return nil
}

// GetConflictByPath retourne un conflit par son chemin
func (cm *ConflictManager) GetConflictByPath(path string) *Conflict {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	
	return cm.conflicts[path]
}

// GetHistory retourne l'historique des conflits résolus
func (cm *ConflictManager) GetHistory() []*Conflict {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	
	result := make([]*Conflict, len(cm.history))
	copy(result, cm.history)
	return result
}

// HasConflicts vérifie s'il y a des conflits non résolus
func (cm *ConflictManager) HasConflicts() bool {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return len(cm.conflicts) > 0
}

// ConflictCount retourne le nombre de conflits non résolus
func (cm *ConflictManager) ConflictCount() int {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return len(cm.conflicts)
}

// ClearHistory efface l'historique
func (cm *ConflictManager) ClearHistory() {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.history = make([]*Conflict, 0)
}

// ResolveAll résout tous les conflits avec la stratégie donnée
func (cm *ConflictManager) ResolveAll(strategy ConflictStrategy) int {
	conflicts := cm.GetConflicts()
	resolved := 0
	
	for _, c := range conflicts {
		if err := cm.ResolveConflict(c.ID, strategy, nil); err == nil {
			resolved++
		}
	}
	
	return resolved
}

// HashData calcule le hash SHA256 de données
func HashData(data []byte) string {
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

// HashFile calcule le hash SHA256 d'un fichier
func HashFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return HashData(data), nil
}

// IsTextFile vérifie si un fichier est un fichier texte
func IsTextFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	textExtensions := map[string]bool{
		".txt": true, ".md": true, ".log": true,
		".json": true, ".xml": true, ".yaml": true, ".yml": true,
		".html": true, ".css": true, ".js": true, ".ts": true,
		".go": true, ".py": true, ".java": true, ".c": true, ".cpp": true,
		".h": true, ".hpp": true, ".cs": true, ".rb": true, ".php": true,
		".sh": true, ".bash": true, ".bat": true, ".ps1": true,
		".ini": true, ".cfg": true, ".conf": true, ".env": true,
		".sql": true, ".csv": true, ".tsv": true,
	}
	return textExtensions[ext]
}

// SimpleMerge tente une fusion simple de deux fichiers texte
func SimpleMerge(local, remote []byte) ([]byte, error) {
	localLines := strings.Split(string(local), "\n")
	remoteLines := strings.Split(string(remote), "\n")
	
	// Si les fichiers sont identiques
	if string(local) == string(remote) {
		return local, nil
	}
	
	// Fusion simple: prendre les lignes uniques des deux côtés
	// C'est une approche basique, pas un vrai merge 3-way
	
	seen := make(map[string]bool)
	var merged []string
	
	// Ajouter les lignes locales
	for _, line := range localLines {
		if !seen[line] {
			merged = append(merged, line)
			seen[line] = true
		}
	}
	
	// Ajouter les lignes distantes qui ne sont pas déjà présentes
	for _, line := range remoteLines {
		if !seen[line] {
			merged = append(merged, line)
			seen[line] = true
		}
	}
	
	// Si la fusion a plus de lignes que le max des deux, ajouter un marqueur
	if len(merged) > len(localLines) && len(merged) > len(remoteLines) {
		header := "<<<< MERGED AUTOMATICALLY >>>>\n"
		return []byte(header + strings.Join(merged, "\n")), nil
	}
	
	return []byte(strings.Join(merged, "\n")), nil
}

func generateConflictID() string {
	return fmt.Sprintf("conflict_%d", time.Now().UnixNano())
}

// Global conflict manager
var globalConflictManager = NewConflictManager()

// GetConflictManager retourne le gestionnaire de conflits global
func GetConflictManager() *ConflictManager {
	return globalConflictManager
}
