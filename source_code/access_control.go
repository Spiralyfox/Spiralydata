package main

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ============================================================================
// 7.3 CONTRÔLE D'ACCÈS
// ============================================================================

// UserRole représente un rôle utilisateur
type UserRole int

const (
	RoleNone     UserRole = iota
	RoleReadOnly          // Peut seulement lire/télécharger
	RoleReadWrite         // Peut lire et écrire
	RoleAdmin             // Tous les droits
)

// String retourne le nom du rôle
func (r UserRole) String() string {
	switch r {
	case RoleReadOnly:
		return "Lecture seule"
	case RoleReadWrite:
		return "Lecture/Écriture"
	case RoleAdmin:
		return "Administrateur"
	default:
		return "Aucun"
	}
}

// CanRead vérifie si le rôle peut lire
func (r UserRole) CanRead() bool {
	return r >= RoleReadOnly
}

// CanWrite vérifie si le rôle peut écrire
func (r UserRole) CanWrite() bool {
	return r >= RoleReadWrite
}

// CanDelete vérifie si le rôle peut supprimer
func (r UserRole) CanDelete() bool {
	return r >= RoleReadWrite
}

// CanAdmin vérifie si le rôle est admin
func (r UserRole) CanAdmin() bool {
	return r >= RoleAdmin
}

// ============================================================================
// USER MANAGEMENT
// ============================================================================

// User représente un utilisateur
type User struct {
	ID           string
	Name         string
	Role         UserRole
	PasswordHash string
	CreatedAt    time.Time
	LastLogin    time.Time
	IsActive     bool
	Permissions  *UserPermissions
	Quota        *UserQuota
}

// UserPermissions permissions granulaires
type UserPermissions struct {
	AllowedPaths    []string          // Chemins autorisés (patterns)
	DeniedPaths     []string          // Chemins interdits (patterns)
	AllowedActions  map[string]bool   // Actions autorisées
	FolderPerms     map[string]Permission // Permissions par dossier
}

// Permission pour un dossier
type Permission struct {
	Read   bool
	Write  bool
	Delete bool
	Share  bool
}

// UserQuota quotas utilisateur
type UserQuota struct {
	MaxStorage      int64 // Bytes max
	UsedStorage     int64 // Bytes utilisés
	MaxDownload     int64 // Bytes/jour download
	UsedDownload    int64
	MaxUpload       int64 // Bytes/jour upload
	UsedUpload      int64
	MaxFiles        int64 // Nombre max de fichiers
	UsedFiles       int64
	LastReset       time.Time
}

// NewUser crée un nouvel utilisateur
func NewUser(id, name string, role UserRole) *User {
	return &User{
		ID:        id,
		Name:      name,
		Role:      role,
		CreatedAt: time.Now(),
		IsActive:  true,
		Permissions: &UserPermissions{
			AllowedPaths:   []string{"*"},
			DeniedPaths:    []string{},
			AllowedActions: make(map[string]bool),
			FolderPerms:    make(map[string]Permission),
		},
		Quota: &UserQuota{
			MaxStorage:  10 * 1024 * 1024 * 1024, // 10 GB
			MaxDownload: 1024 * 1024 * 1024,       // 1 GB/jour
			MaxUpload:   1024 * 1024 * 1024,       // 1 GB/jour
			MaxFiles:    10000,
			LastReset:   time.Now(),
		},
	}
}

// ============================================================================
// USER MANAGER
// ============================================================================

// UserManager gère les utilisateurs
type UserManager struct {
	users map[string]*User
	mu    sync.RWMutex
}

// NewUserManager crée un gestionnaire d'utilisateurs
func NewUserManager() *UserManager {
	return &UserManager{
		users: make(map[string]*User),
	}
}

// AddUser ajoute un utilisateur
func (um *UserManager) AddUser(user *User) {
	um.mu.Lock()
	defer um.mu.Unlock()
	um.users[user.ID] = user
}

// GetUser récupère un utilisateur
func (um *UserManager) GetUser(id string) (*User, bool) {
	um.mu.RLock()
	defer um.mu.RUnlock()
	user, ok := um.users[id]
	return user, ok
}

// RemoveUser supprime un utilisateur
func (um *UserManager) RemoveUser(id string) {
	um.mu.Lock()
	defer um.mu.Unlock()
	delete(um.users, id)
}

// GetUsers retourne tous les utilisateurs
func (um *UserManager) GetUsers() []*User {
	um.mu.RLock()
	defer um.mu.RUnlock()
	
	users := make([]*User, 0, len(um.users))
	for _, user := range um.users {
		users = append(users, user)
	}
	return users
}

// ============================================================================
// ACCESS CONTROL
// ============================================================================

// AccessController contrôle les accès
type AccessController struct {
	userManager *UserManager
	mu          sync.RWMutex
}

// NewAccessController crée un contrôleur d'accès
func NewAccessController(um *UserManager) *AccessController {
	return &AccessController{
		userManager: um,
	}
}

// CanAccess vérifie si un utilisateur peut accéder à un chemin
func (ac *AccessController) CanAccess(userID, path string, action string) (bool, string) {
	user, ok := ac.userManager.GetUser(userID)
	if !ok {
		return false, "Utilisateur non trouvé"
	}
	
	if !user.IsActive {
		return false, "Compte désactivé"
	}
	
	// Vérifier le rôle
	switch action {
	case "read", "download", "list":
		if !user.Role.CanRead() {
			return false, "Permission de lecture refusée"
		}
	case "write", "upload", "create", "modify":
		if !user.Role.CanWrite() {
			return false, "Permission d'écriture refusée"
		}
	case "delete", "remove":
		if !user.Role.CanDelete() {
			return false, "Permission de suppression refusée"
		}
	case "admin", "config", "users":
		if !user.Role.CanAdmin() {
			return false, "Permission d'administration refusée"
		}
	}
	
	// Vérifier les chemins interdits
	for _, denied := range user.Permissions.DeniedPaths {
		if matchPath(path, denied) {
			return false, fmt.Sprintf("Accès interdit: %s", path)
		}
	}
	
	// Vérifier les chemins autorisés
	allowed := false
	for _, allowedPath := range user.Permissions.AllowedPaths {
		if matchPath(path, allowedPath) {
			allowed = true
			break
		}
	}
	
	if !allowed {
		return false, fmt.Sprintf("Chemin non autorisé: %s", path)
	}
	
	// Vérifier les permissions par dossier
	for folder, perm := range user.Permissions.FolderPerms {
		if strings.HasPrefix(path, folder) {
			switch action {
			case "read", "download", "list":
				if !perm.Read {
					return false, "Lecture interdite pour ce dossier"
				}
			case "write", "upload", "create", "modify":
				if !perm.Write {
					return false, "Écriture interdite pour ce dossier"
				}
			case "delete", "remove":
				if !perm.Delete {
					return false, "Suppression interdite pour ce dossier"
				}
			}
		}
	}
	
	return true, ""
}

// CheckQuota vérifie si l'utilisateur a assez de quota
func (ac *AccessController) CheckQuota(userID string, size int64, action string) (bool, string) {
	user, ok := ac.userManager.GetUser(userID)
	if !ok {
		return false, "Utilisateur non trouvé"
	}
	
	quota := user.Quota
	
	// Réinitialiser les quotas journaliers si nécessaire
	if time.Since(quota.LastReset) > 24*time.Hour {
		quota.UsedDownload = 0
		quota.UsedUpload = 0
		quota.LastReset = time.Now()
	}
	
	switch action {
	case "upload", "write":
		if quota.UsedStorage+size > quota.MaxStorage {
			return false, fmt.Sprintf("Quota de stockage dépassé (%s/%s)",
				FormatFileSize(quota.UsedStorage),
				FormatFileSize(quota.MaxStorage))
		}
		if quota.UsedUpload+size > quota.MaxUpload {
			return false, fmt.Sprintf("Quota d'upload journalier dépassé (%s/%s)",
				FormatFileSize(quota.UsedUpload),
				FormatFileSize(quota.MaxUpload))
		}
	case "download", "read":
		if quota.UsedDownload+size > quota.MaxDownload {
			return false, fmt.Sprintf("Quota de download journalier dépassé (%s/%s)",
				FormatFileSize(quota.UsedDownload),
				FormatFileSize(quota.MaxDownload))
		}
	}
	
	return true, ""
}

// UpdateQuotaUsage met à jour l'utilisation des quotas
func (ac *AccessController) UpdateQuotaUsage(userID string, size int64, action string) {
	user, ok := ac.userManager.GetUser(userID)
	if !ok {
		return
	}
	
	quota := user.Quota
	
	switch action {
	case "upload", "write":
		quota.UsedStorage += size
		quota.UsedUpload += size
		quota.UsedFiles++
	case "download", "read":
		quota.UsedDownload += size
	case "delete", "remove":
		quota.UsedStorage -= size
		if quota.UsedStorage < 0 {
			quota.UsedStorage = 0
		}
		quota.UsedFiles--
		if quota.UsedFiles < 0 {
			quota.UsedFiles = 0
		}
	}
}

// ============================================================================
// TIME-BASED ACCESS
// ============================================================================

// TimeBasedAccess accès temporaire
type TimeBasedAccess struct {
	ID        string
	UserID    string
	Path      string
	Actions   []string
	StartTime time.Time
	EndTime   time.Time
	IsActive  bool
}

// TimeAccessManager gère les accès temporaires
type TimeAccessManager struct {
	accesses map[string]*TimeBasedAccess
	mu       sync.RWMutex
}

// NewTimeAccessManager crée un gestionnaire d'accès temporaires
func NewTimeAccessManager() *TimeAccessManager {
	tam := &TimeAccessManager{
		accesses: make(map[string]*TimeBasedAccess),
	}
	
	go tam.cleanupLoop()
	
	return tam
}

// CreateAccess crée un accès temporaire
func (tam *TimeAccessManager) CreateAccess(userID, path string, actions []string, duration time.Duration) *TimeBasedAccess {
	tam.mu.Lock()
	defer tam.mu.Unlock()
	
	now := time.Now()
	access := &TimeBasedAccess{
		ID:        GenerateSecureToken(8),
		UserID:    userID,
		Path:      path,
		Actions:   actions,
		StartTime: now,
		EndTime:   now.Add(duration),
		IsActive:  true,
	}
	
	tam.accesses[access.ID] = access
	return access
}

// CheckAccess vérifie un accès temporaire
func (tam *TimeAccessManager) CheckAccess(userID, path, action string) bool {
	tam.mu.RLock()
	defer tam.mu.RUnlock()
	
	now := time.Now()
	
	for _, access := range tam.accesses {
		if access.UserID != userID || !access.IsActive {
			continue
		}
		
		if now.Before(access.StartTime) || now.After(access.EndTime) {
			continue
		}
		
		if !matchPath(path, access.Path) {
			continue
		}
		
		for _, a := range access.Actions {
			if a == action || a == "*" {
				return true
			}
		}
	}
	
	return false
}

// RevokeAccess révoque un accès temporaire
func (tam *TimeAccessManager) RevokeAccess(accessID string) {
	tam.mu.Lock()
	defer tam.mu.Unlock()
	
	if access, ok := tam.accesses[accessID]; ok {
		access.IsActive = false
	}
}

// GetActiveAccesses retourne les accès actifs
func (tam *TimeAccessManager) GetActiveAccesses() []*TimeBasedAccess {
	tam.mu.RLock()
	defer tam.mu.RUnlock()
	
	now := time.Now()
	var active []*TimeBasedAccess
	
	for _, access := range tam.accesses {
		if access.IsActive && now.After(access.StartTime) && now.Before(access.EndTime) {
			active = append(active, access)
		}
	}
	
	return active
}

func (tam *TimeAccessManager) cleanupLoop() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	
	for range ticker.C {
		tam.mu.Lock()
		now := time.Now()
		for id, access := range tam.accesses {
			if now.After(access.EndTime) {
				delete(tam.accesses, id)
			}
		}
		tam.mu.Unlock()
	}
}

// ============================================================================
// SHARING CONTROLS
// ============================================================================

// ShareLink lien de partage
type ShareLink struct {
	ID         string
	Path       string
	CreatedBy  string
	CreatedAt  time.Time
	ExpiresAt  time.Time
	Password   string // Hash
	MaxUses    int
	UsageCount int
	IsActive   bool
	Permissions Permission
}

// ShareManager gère les liens de partage
type ShareManager struct {
	shares map[string]*ShareLink
	mu     sync.RWMutex
}

// NewShareManager crée un gestionnaire de partage
func NewShareManager() *ShareManager {
	return &ShareManager{
		shares: make(map[string]*ShareLink),
	}
}

// CreateShare crée un lien de partage
func (sm *ShareManager) CreateShare(path, createdBy string, duration time.Duration, maxUses int, perms Permission) *ShareLink {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	
	now := time.Now()
	share := &ShareLink{
		ID:         GenerateSecureToken(16),
		Path:       path,
		CreatedBy:  createdBy,
		CreatedAt:  now,
		ExpiresAt:  now.Add(duration),
		MaxUses:    maxUses,
		IsActive:   true,
		Permissions: perms,
	}
	
	sm.shares[share.ID] = share
	return share
}

// GetShare récupère un lien de partage
func (sm *ShareManager) GetShare(shareID string) (*ShareLink, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	
	share, ok := sm.shares[shareID]
	if !ok || !share.IsActive {
		return nil, false
	}
	
	now := time.Now()
	if now.After(share.ExpiresAt) {
		return nil, false
	}
	
	if share.MaxUses > 0 && share.UsageCount >= share.MaxUses {
		return nil, false
	}
	
	return share, true
}

// UseShare utilise un lien de partage
func (sm *ShareManager) UseShare(shareID string) bool {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	
	share, ok := sm.shares[shareID]
	if !ok || !share.IsActive {
		return false
	}
	
	share.UsageCount++
	return true
}

// RevokeShare révoque un lien de partage
func (sm *ShareManager) RevokeShare(shareID string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	
	if share, ok := sm.shares[shareID]; ok {
		share.IsActive = false
	}
}

// ============================================================================
// UTILITIES
// ============================================================================

// matchPath vérifie si un chemin correspond à un pattern
func matchPath(path, pattern string) bool {
	// Pattern * = tout
	if pattern == "*" {
		return true
	}
	
	// Pattern exact
	if path == pattern {
		return true
	}
	
	// Pattern avec wildcard à la fin
	if strings.HasSuffix(pattern, "*") {
		prefix := strings.TrimSuffix(pattern, "*")
		return strings.HasPrefix(path, prefix)
	}
	
	// Utiliser filepath.Match pour les patterns plus complexes
	matched, err := filepath.Match(pattern, path)
	if err == nil && matched {
		return true
	}
	
	return false
}

// ============================================================================
// GLOBAL INSTANCES
// ============================================================================

var (
	globalUserManager     = NewUserManager()
	globalAccessController *AccessController
	globalTimeAccessMgr   = NewTimeAccessManager()
	globalShareManager    = NewShareManager()
)

func init() {
	globalAccessController = NewAccessController(globalUserManager)
	
	// Créer un utilisateur admin par défaut
	adminUser := NewUser("admin", "Administrateur", RoleAdmin)
	globalUserManager.AddUser(adminUser)
}

// GetUserManager retourne le gestionnaire d'utilisateurs global
func GetUserManager() *UserManager { return globalUserManager }

// GetAccessController retourne le contrôleur d'accès global
func GetAccessController() *AccessController { return globalAccessController }

// GetTimeAccessManager retourne le gestionnaire d'accès temporaires global
func GetTimeAccessManager() *TimeAccessManager { return globalTimeAccessMgr }

// GetShareManager retourne le gestionnaire de partage global
func GetShareManager() *ShareManager { return globalShareManager }
