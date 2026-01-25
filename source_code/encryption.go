package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// ============================================================================
// 7.2 CHIFFREMENT
// ============================================================================

// EncryptionConfig configuration du chiffrement
type EncryptionConfig struct {
	Enabled          bool   `json:"enabled"`
	Algorithm        string `json:"algorithm"` // "aes-256-gcm"
	KeyDerivation    string `json:"key_derivation"` // "sha256"
	EncryptMetadata  bool   `json:"encrypt_metadata"`
	EncryptFilenames bool   `json:"encrypt_filenames"`
	PerFileKeys      bool   `json:"per_file_keys"`
}

// NewEncryptionConfig crée une configuration par défaut
func NewEncryptionConfig() *EncryptionConfig {
	return &EncryptionConfig{
		Enabled:          false,
		Algorithm:        "aes-256-gcm",
		KeyDerivation:    "sha256",
		EncryptMetadata:  false,
		EncryptFilenames: false,
		PerFileKeys:      false,
	}
}

// ============================================================================
// KEY MANAGEMENT
// ============================================================================

// EncryptionKey représente une clé de chiffrement
type EncryptionKey struct {
	ID        string
	Key       []byte
	CreatedAt time.Time
	ExpiresAt time.Time
	IsActive  bool
	UsageCount int64
}

// KeyManager gère les clés de chiffrement
type KeyManager struct {
	keys       map[string]*EncryptionKey
	activeKey  string
	masterKey  []byte
	mu         sync.RWMutex
}

// NewKeyManager crée un gestionnaire de clés
func NewKeyManager() *KeyManager {
	return &KeyManager{
		keys: make(map[string]*EncryptionKey),
	}
}

// SetMasterKey définit la clé maître (dérivée du mot de passe)
func (km *KeyManager) SetMasterKey(password string) {
	km.mu.Lock()
	defer km.mu.Unlock()
	
	// Dériver la clé avec SHA256 (pour production, utiliser PBKDF2 ou Argon2)
	hash := sha256.Sum256([]byte("spiralydata_master_" + password))
	km.masterKey = hash[:]
}

// HasMasterKey vérifie si une clé maître est définie
func (km *KeyManager) HasMasterKey() bool {
	km.mu.RLock()
	defer km.mu.RUnlock()
	return len(km.masterKey) > 0
}

// ClearKeys efface toutes les clés de la mémoire
func (km *KeyManager) ClearKeys() {
	km.mu.Lock()
	defer km.mu.Unlock()
	
	// Écraser avec des zéros avant de supprimer
	for i := range km.masterKey {
		km.masterKey[i] = 0
	}
	km.masterKey = nil
	
	for keyID, key := range km.keys {
		for i := range key.Key {
			key.Key[i] = 0
		}
		delete(km.keys, keyID)
	}
	
	km.activeKey = ""
}

// GenerateKey génère une nouvelle clé de chiffrement
func (km *KeyManager) GenerateKey(duration time.Duration) (*EncryptionKey, error) {
	km.mu.Lock()
	defer km.mu.Unlock()
	
	keyBytes := make([]byte, 32) // AES-256
	if _, err := rand.Read(keyBytes); err != nil {
		return nil, err
	}
	
	keyID := GenerateSecureToken(8)
	now := time.Now()
	
	key := &EncryptionKey{
		ID:        keyID,
		Key:       keyBytes,
		CreatedAt: now,
		ExpiresAt: now.Add(duration),
		IsActive:  true,
	}
	
	km.keys[keyID] = key
	km.activeKey = keyID
	
	return key, nil
}

// GetActiveKey retourne la clé active
func (km *KeyManager) GetActiveKey() (*EncryptionKey, error) {
	km.mu.RLock()
	defer km.mu.RUnlock()
	
	if km.activeKey == "" {
		return nil, errors.New("aucune clé active")
	}
	
	key, ok := km.keys[km.activeKey]
	if !ok || !key.IsActive {
		return nil, errors.New("clé active invalide")
	}
	
	return key, nil
}

// GetKey retourne une clé par son ID
func (km *KeyManager) GetKey(keyID string) (*EncryptionKey, error) {
	km.mu.RLock()
	defer km.mu.RUnlock()
	
	key, ok := km.keys[keyID]
	if !ok {
		return nil, fmt.Errorf("clé non trouvée: %s", keyID)
	}
	
	return key, nil
}

// RotateKey effectue une rotation de clé
func (km *KeyManager) RotateKey(duration time.Duration) (*EncryptionKey, error) {
	// Désactiver l'ancienne clé (mais la garder pour déchiffrement)
	km.mu.Lock()
	if km.activeKey != "" {
		if oldKey, ok := km.keys[km.activeKey]; ok {
			oldKey.IsActive = false
		}
	}
	km.mu.Unlock()
	
	// Générer une nouvelle clé
	return km.GenerateKey(duration)
}

// ExportKey exporte une clé chiffrée avec la clé maître
func (km *KeyManager) ExportKey(keyID string) (string, error) {
	km.mu.RLock()
	defer km.mu.RUnlock()
	
	if len(km.masterKey) == 0 {
		return "", errors.New("clé maître non définie")
	}
	
	key, ok := km.keys[keyID]
	if !ok {
		return "", fmt.Errorf("clé non trouvée: %s", keyID)
	}
	
	// Chiffrer la clé avec la clé maître
	encrypted, err := EncryptAESGCM(key.Key, km.masterKey)
	if err != nil {
		return "", err
	}
	
	return base64.StdEncoding.EncodeToString(encrypted), nil
}

// ImportKey importe une clé chiffrée
func (km *KeyManager) ImportKey(keyID string, encryptedKey string) error {
	km.mu.Lock()
	defer km.mu.Unlock()
	
	if len(km.masterKey) == 0 {
		return errors.New("clé maître non définie")
	}
	
	encrypted, err := base64.StdEncoding.DecodeString(encryptedKey)
	if err != nil {
		return err
	}
	
	keyBytes, err := DecryptAESGCM(encrypted, km.masterKey)
	if err != nil {
		return err
	}
	
	key := &EncryptionKey{
		ID:        keyID,
		Key:       keyBytes,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(365 * 24 * time.Hour),
		IsActive:  true,
	}
	
	km.keys[keyID] = key
	return nil
}

// ============================================================================
// AES-256-GCM ENCRYPTION
// ============================================================================

// EncryptAESGCM chiffre des données avec AES-256-GCM
func EncryptAESGCM(plaintext, key []byte) ([]byte, error) {
	if len(key) != 32 {
		return nil, errors.New("clé doit être de 32 bytes (AES-256)")
	}
	
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	
	// Générer un nonce aléatoire
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	
	// Chiffrer et ajouter le nonce au début
	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return ciphertext, nil
}

// DecryptAESGCM déchiffre des données AES-256-GCM
func DecryptAESGCM(ciphertext, key []byte) ([]byte, error) {
	if len(key) != 32 {
		return nil, errors.New("clé doit être de 32 bytes (AES-256)")
	}
	
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	
	if len(ciphertext) < gcm.NonceSize() {
		return nil, errors.New("ciphertext trop court")
	}
	
	// Extraire le nonce
	nonce := ciphertext[:gcm.NonceSize()]
	ciphertext = ciphertext[gcm.NonceSize():]
	
	// Déchiffrer
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, err
	}
	
	return plaintext, nil
}

// ============================================================================
// FILE ENCRYPTION
// ============================================================================

// FileEncryptor chiffre/déchiffre des fichiers
type FileEncryptor struct {
	keyManager *KeyManager
	config     *EncryptionConfig
}

// NewFileEncryptor crée un nouveau chiffreur de fichiers
func NewFileEncryptor(km *KeyManager, config *EncryptionConfig) *FileEncryptor {
	return &FileEncryptor{
		keyManager: km,
		config:     config,
	}
}

// EncryptedFileHeader en-tête d'un fichier chiffré
type EncryptedFileHeader struct {
	Magic     string `json:"magic"`     // "SPENC"
	Version   int    `json:"version"`   // 1
	KeyID     string `json:"key_id"`
	Algorithm string `json:"algorithm"`
	OrigSize  int64  `json:"orig_size"`
	OrigName  string `json:"orig_name,omitempty"`
	OrigHash  string `json:"orig_hash,omitempty"`
}

// EncryptFile chiffre un fichier
func (fe *FileEncryptor) EncryptFile(srcPath, dstPath string) error {
	if !fe.config.Enabled {
		// Simplement copier si chiffrement désactivé
		return copyFile(srcPath, dstPath)
	}
	
	// Lire le fichier source
	plaintext, err := os.ReadFile(srcPath)
	if err != nil {
		return err
	}
	
	// Obtenir la clé active
	key, err := fe.keyManager.GetActiveKey()
	if err != nil {
		return err
	}
	
	// Chiffrer
	ciphertext, err := EncryptAESGCM(plaintext, key.Key)
	if err != nil {
		return err
	}
	
	// Créer le répertoire de destination
	if err := os.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
		return err
	}
	
	// Écrire le fichier chiffré avec en-tête
	// Format: [4 bytes: header size][header JSON][ciphertext]
	header := fmt.Sprintf(`{"magic":"SPENC","version":1,"key_id":"%s","algorithm":"aes-256-gcm","orig_size":%d}`,
		key.ID, len(plaintext))
	
	headerBytes := []byte(header)
	headerSize := uint32(len(headerBytes))
	
	file, err := os.Create(dstPath)
	if err != nil {
		return err
	}
	defer file.Close()
	
	// Écrire la taille de l'en-tête (4 bytes, big-endian)
	file.Write([]byte{
		byte(headerSize >> 24),
		byte(headerSize >> 16),
		byte(headerSize >> 8),
		byte(headerSize),
	})
	
	// Écrire l'en-tête
	file.Write(headerBytes)
	
	// Écrire le ciphertext
	file.Write(ciphertext)
	
	key.UsageCount++
	
	return nil
}

// DecryptFile déchiffre un fichier
func (fe *FileEncryptor) DecryptFile(srcPath, dstPath string) error {
	file, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer file.Close()
	
	// Lire la taille de l'en-tête
	headerSizeBytes := make([]byte, 4)
	if _, err := io.ReadFull(file, headerSizeBytes); err != nil {
		// Fichier non chiffré, simplement copier
		return copyFile(srcPath, dstPath)
	}
	
	headerSize := uint32(headerSizeBytes[0])<<24 |
		uint32(headerSizeBytes[1])<<16 |
		uint32(headerSizeBytes[2])<<8 |
		uint32(headerSizeBytes[3])
	
	// Lire l'en-tête
	headerBytes := make([]byte, headerSize)
	if _, err := io.ReadFull(file, headerBytes); err != nil {
		return err
	}
	
	// Parser l'en-tête (simple extraction du key_id)
	// En production, utiliser encoding/json
	headerStr := string(headerBytes)
	if len(headerStr) < 10 || headerStr[:16] != `{"magic":"SPENC"` {
		// Pas un fichier chiffré
		return copyFile(srcPath, dstPath)
	}
	
	// Extraire key_id (simplification)
	keyIDStart := indexOf(headerStr, `"key_id":"`) + 10
	keyIDEnd := indexOf(headerStr[keyIDStart:], `"`) + keyIDStart
	keyID := headerStr[keyIDStart:keyIDEnd]
	
	// Obtenir la clé
	key, err := fe.keyManager.GetKey(keyID)
	if err != nil {
		return fmt.Errorf("clé de déchiffrement non trouvée: %s", keyID)
	}
	
	// Lire le ciphertext
	ciphertext, err := io.ReadAll(file)
	if err != nil {
		return err
	}
	
	// Déchiffrer
	plaintext, err := DecryptAESGCM(ciphertext, key.Key)
	if err != nil {
		return err
	}
	
	// Écrire le fichier déchiffré
	if err := os.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
		return err
	}
	
	return os.WriteFile(dstPath, plaintext, 0644)
}

// EncryptData chiffre des données en mémoire
func (fe *FileEncryptor) EncryptData(data []byte) ([]byte, string, error) {
	if !fe.config.Enabled {
		return data, "", nil
	}
	
	key, err := fe.keyManager.GetActiveKey()
	if err != nil {
		return nil, "", err
	}
	
	encrypted, err := EncryptAESGCM(data, key.Key)
	if err != nil {
		return nil, "", err
	}
	
	key.UsageCount++
	return encrypted, key.ID, nil
}

// DecryptData déchiffre des données en mémoire
func (fe *FileEncryptor) DecryptData(data []byte, keyID string) ([]byte, error) {
	if keyID == "" {
		return data, nil
	}
	
	key, err := fe.keyManager.GetKey(keyID)
	if err != nil {
		return nil, err
	}
	
	return DecryptAESGCM(data, key.Key)
}

// ============================================================================
// SECURE DELETE
// ============================================================================

// SecureDelete supprime un fichier de manière sécurisée (overwrite)
func SecureDelete(path string, passes int) error {
	if passes < 1 {
		passes = 3
	}
	
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	
	size := info.Size()
	
	file, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	
	buf := make([]byte, 4096)
	
	for pass := 0; pass < passes; pass++ {
		file.Seek(0, 0)
		
		remaining := size
		for remaining > 0 {
			toWrite := int64(len(buf))
			if toWrite > remaining {
				toWrite = remaining
			}
			
			// Remplir avec des données aléatoires
			rand.Read(buf[:toWrite])
			
			if _, err := file.Write(buf[:toWrite]); err != nil {
				file.Close()
				return err
			}
			
			remaining -= toWrite
		}
		
		file.Sync()
	}
	
	file.Close()
	
	// Supprimer le fichier
	return os.Remove(path)
}

// ============================================================================
// UTILITIES
// ============================================================================

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	
	return os.WriteFile(dst, data, 0644)
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

// DeriveKey dérive une clé à partir d'un mot de passe
func DeriveKey(password, salt string) []byte {
	// Simple dérivation (pour production, utiliser PBKDF2 ou Argon2)
	h := sha256.New()
	h.Write([]byte(salt))
	h.Write([]byte(password))
	return h.Sum(nil)
}

// EncryptString chiffre une chaîne avec un mot de passe
func EncryptString(plaintext, password string) (string, error) {
	key := DeriveKey(password, "spiralydata_string_")
	
	encrypted, err := EncryptAESGCM([]byte(plaintext), key)
	if err != nil {
		return "", err
	}
	
	return base64.StdEncoding.EncodeToString(encrypted), nil
}

// DecryptString déchiffre une chaîne avec un mot de passe
func DecryptString(ciphertext, password string) (string, error) {
	key := DeriveKey(password, "spiralydata_string_")
	
	encrypted, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", err
	}
	
	decrypted, err := DecryptAESGCM(encrypted, key)
	if err != nil {
		return "", err
	}
	
	return string(decrypted), nil
}

// ============================================================================
// GLOBAL INSTANCES
// ============================================================================

var (
	globalKeyManager     = NewKeyManager()
	globalEncryptConfig  = NewEncryptionConfig()
	globalFileEncryptor  *FileEncryptor
)

func init() {
	globalFileEncryptor = NewFileEncryptor(globalKeyManager, globalEncryptConfig)
}

// GetKeyManager retourne le gestionnaire de clés global
func GetKeyManager() *KeyManager { return globalKeyManager }

// GetEncryptionConfig retourne la config de chiffrement globale
func GetEncryptionConfig() *EncryptionConfig { return globalEncryptConfig }

// GetFileEncryptor retourne le chiffreur de fichiers global
func GetFileEncryptor() *FileEncryptor { return globalFileEncryptor }

// EncryptForTransfer chiffre des données pour le transfert
func EncryptForTransfer(data []byte) ([]byte, string, error) {
	return globalFileEncryptor.EncryptData(data)
}

// DecryptFromTransfer déchiffre des données reçues
func DecryptFromTransfer(data []byte, keyID string) ([]byte, error) {
	return globalFileEncryptor.DecryptData(data, keyID)
}

// CalculateChecksum calcule un checksum pour la détection d'intégrité
func CalculateChecksum(data []byte) string {
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:8])
}

// VerifyChecksum vérifie un checksum
func VerifyChecksum(data []byte, checksum string) bool {
	return CalculateChecksum(data) == checksum
}

// SetEncryptionPassword définit le mot de passe de chiffrement
func SetEncryptionPassword(password string) {
	globalKeyManager.SetMasterKey(password)
	globalEncryptConfig.Enabled = true
	addLog("🔐 Chiffrement activé")
}

// DisableEncryption désactive le chiffrement
func DisableEncryption() {
	globalKeyManager.ClearKeys()
	globalEncryptConfig.Enabled = false
	addLog("🔓 Chiffrement désactivé")
}

// IsEncryptionEnabled vérifie si le chiffrement est activé
func IsEncryptionEnabled() bool {
	return globalEncryptConfig.Enabled && globalKeyManager.HasMasterKey()
}

// GetIntegrityChecker retourne le vérificateur d'intégrité global
func GetIntegrityChecker() *IntegrityChecker {
	return globalIntegrityChecker
}

// IntegrityChecker vérifie l'intégrité des fichiers
type IntegrityChecker struct {
	baseline map[string]*FileIntegrity
	mu       sync.RWMutex
}

// FileIntegrity représente l'intégrité d'un fichier
type FileIntegrity struct {
	Path      string
	Hash      string
	Size      int64
	ModTime   time.Time
	CheckedAt time.Time
}

// NewIntegrityChecker crée un nouveau vérificateur
func NewIntegrityChecker() *IntegrityChecker {
	return &IntegrityChecker{
		baseline: make(map[string]*FileIntegrity),
	}
}

// AddToBaseline ajoute un fichier à la baseline
func (ic *IntegrityChecker) AddToBaseline(path string) error {
	ic.mu.Lock()
	defer ic.mu.Unlock()
	
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	
	hash, err := StreamHash(path)
	if err != nil {
		return err
	}
	
	ic.baseline[path] = &FileIntegrity{
		Path:      path,
		Hash:      hash,
		Size:      info.Size(),
		ModTime:   info.ModTime(),
		CheckedAt: time.Now(),
	}
	
	return nil
}

// CheckIntegrity vérifie l'intégrité d'un fichier
func (ic *IntegrityChecker) CheckIntegrity(path string) (bool, string, error) {
	ic.mu.RLock()
	baseline, exists := ic.baseline[path]
	ic.mu.RUnlock()
	
	if !exists {
		return false, "not in baseline", nil
	}
	
	info, err := os.Stat(path)
	if err != nil {
		return false, "file not found", err
	}
	
	if info.Size() != baseline.Size {
		return false, "size changed", nil
	}
	
	hash, err := StreamHash(path)
	if err != nil {
		return false, "hash error", err
	}
	
	if hash != baseline.Hash {
		return false, "content changed", nil
	}
	
	return true, "ok", nil
}

// CheckAllIntegrity vérifie tous les fichiers
func (ic *IntegrityChecker) CheckAllIntegrity() map[string]string {
	ic.mu.RLock()
	paths := make([]string, 0, len(ic.baseline))
	for path := range ic.baseline {
		paths = append(paths, path)
	}
	ic.mu.RUnlock()
	
	results := make(map[string]string)
	for _, path := range paths {
		ok, reason, _ := ic.CheckIntegrity(path)
		if ok {
			results[path] = "OK"
		} else {
			results[path] = "TAMPERED: " + reason
		}
	}
	
	return results
}

var globalIntegrityChecker = NewIntegrityChecker()
