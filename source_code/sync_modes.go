package main

import (
	"compress/gzip"
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"sync"
	"time"
)

// SyncMode représente le mode de synchronisation
type SyncMode int

const (
	SyncModeBidirectional SyncMode = iota // Sync dans les deux sens
	SyncModeHostToUser                    // Serveur vers clients uniquement
	SyncModeUserToHost                    // Clients vers serveur uniquement
	SyncModeMirror                        // Client = copie exacte du serveur
	SyncModeMerge                         // Fusionner sans jamais supprimer
	SyncModeOnDemand                      // Synchroniser uniquement sur demande
)

// SyncConfig contient la configuration de synchronisation
type SyncConfig struct {
	Mode               SyncMode      `json:"mode"`
	CompressionEnabled bool          `json:"compression_enabled"`
	CompressionLevel   int           `json:"compression_level"` // 1-9
	BandwidthLimit     int64         `json:"bandwidth_limit"`   // bytes/sec, 0 = illimité
	RetryCount         int           `json:"retry_count"`
	RetryDelay         time.Duration `json:"retry_delay"`
	ScheduleEnabled    bool          `json:"schedule_enabled"`
	ScheduleInterval   time.Duration `json:"schedule_interval"`
	ScheduleTimes      []string      `json:"schedule_times"` // Format "HH:MM"
	PriorityExtensions []string      `json:"priority_extensions"`
	ConflictStrategy   ConflictStrategy `json:"conflict_strategy"`
}

// ConflictStrategy définit la stratégie de résolution de conflits
type ConflictStrategy int

const (
	ConflictAskUser ConflictStrategy = iota // Demander à l'utilisateur
	ConflictKeepNewest                      // Garder le plus récent
	ConflictKeepLocal                       // Garder la version locale
	ConflictKeepRemote                      // Garder la version distante
	ConflictKeepBoth                        // Créer deux copies
	ConflictAutoMerge                       // Tenter un merge automatique
)

// TransferItem représente un élément dans la file de transfert
type TransferItem struct {
	Path       string
	Priority   int // Plus bas = plus prioritaire
	Size       int64
	IsDir      bool
	Operation  string // "create", "write", "remove", "mkdir"
	Content    string // Base64 encoded
	Compressed bool
	Retries    int
	AddedAt    time.Time
}

// TransferQueue gère la file d'attente des transferts
type TransferQueue struct {
	items    []*TransferItem
	mu       sync.Mutex
	maxSize  int
	paused   bool
	throttle int64 // bytes/sec
}

// NewTransferQueue crée une nouvelle file de transfert
func NewTransferQueue(maxSize int) *TransferQueue {
	return &TransferQueue{
		items:   make([]*TransferItem, 0),
		maxSize: maxSize,
	}
}

// Add ajoute un élément à la file
func (tq *TransferQueue) Add(item *TransferItem) bool {
	tq.mu.Lock()
	defer tq.mu.Unlock()
	
	if len(tq.items) >= tq.maxSize {
		return false
	}
	
	item.AddedAt = time.Now()
	
	// Insérer selon la priorité
	inserted := false
	for i, existing := range tq.items {
		if item.Priority < existing.Priority {
			tq.items = append(tq.items[:i], append([]*TransferItem{item}, tq.items[i:]...)...)
			inserted = true
			break
		}
	}
	
	if !inserted {
		tq.items = append(tq.items, item)
	}
	
	return true
}

// Pop retire et retourne le premier élément
func (tq *TransferQueue) Pop() *TransferItem {
	tq.mu.Lock()
	defer tq.mu.Unlock()
	
	if len(tq.items) == 0 || tq.paused {
		return nil
	}
	
	item := tq.items[0]
	tq.items = tq.items[1:]
	return item
}

// Peek retourne le premier élément sans le retirer
func (tq *TransferQueue) Peek() *TransferItem {
	tq.mu.Lock()
	defer tq.mu.Unlock()
	
	if len(tq.items) == 0 {
		return nil
	}
	return tq.items[0]
}

// Size retourne le nombre d'éléments
func (tq *TransferQueue) Size() int {
	tq.mu.Lock()
	defer tq.mu.Unlock()
	return len(tq.items)
}

// Clear vide la file
func (tq *TransferQueue) Clear() {
	tq.mu.Lock()
	defer tq.mu.Unlock()
	tq.items = make([]*TransferItem, 0)
}

// Pause met la file en pause
func (tq *TransferQueue) Pause() {
	tq.mu.Lock()
	defer tq.mu.Unlock()
	tq.paused = true
}

// Resume reprend la file
func (tq *TransferQueue) Resume() {
	tq.mu.Lock()
	defer tq.mu.Unlock()
	tq.paused = false
}

// IsPaused vérifie si la file est en pause
func (tq *TransferQueue) IsPaused() bool {
	tq.mu.Lock()
	defer tq.mu.Unlock()
	return tq.paused
}

// SetThrottle définit la limite de bande passante
func (tq *TransferQueue) SetThrottle(bytesPerSec int64) {
	tq.mu.Lock()
	defer tq.mu.Unlock()
	tq.throttle = bytesPerSec
}

// GetThrottle retourne la limite de bande passante
func (tq *TransferQueue) GetThrottle() int64 {
	tq.mu.Lock()
	defer tq.mu.Unlock()
	return tq.throttle
}

// GetItems retourne une copie des éléments
func (tq *TransferQueue) GetItems() []*TransferItem {
	tq.mu.Lock()
	defer tq.mu.Unlock()
	
	copy := make([]*TransferItem, len(tq.items))
	for i, item := range tq.items {
		copy[i] = item
	}
	return copy
}

// RemoveByPath retire un élément par son chemin
func (tq *TransferQueue) RemoveByPath(path string) bool {
	tq.mu.Lock()
	defer tq.mu.Unlock()
	
	for i, item := range tq.items {
		if item.Path == path {
			tq.items = append(tq.items[:i], tq.items[i+1:]...)
			return true
		}
	}
	return false
}

// NewSyncConfig crée une configuration par défaut
func NewSyncConfig() *SyncConfig {
	return &SyncConfig{
		Mode:               SyncModeBidirectional,
		CompressionEnabled: true,
		CompressionLevel:   6,
		BandwidthLimit:     0, // Illimité
		RetryCount:         3,
		RetryDelay:         time.Second * 2,
		ScheduleEnabled:    false,
		ScheduleInterval:   time.Minute * 5,
		ScheduleTimes:      []string{},
		PriorityExtensions: []string{".txt", ".md", ".json", ".go", ".py"},
		ConflictStrategy:   ConflictAskUser,
	}
}

// GetModeName retourne le nom du mode
func (sc *SyncConfig) GetModeName() string {
	switch sc.Mode {
	case SyncModeBidirectional:
		return "Bidirectionnel"
	case SyncModeHostToUser:
		return "Host → User"
	case SyncModeUserToHost:
		return "User → Host"
	case SyncModeMirror:
		return "Miroir"
	case SyncModeMerge:
		return "Fusion"
	case SyncModeOnDemand:
		return "Sur demande"
	default:
		return "Inconnu"
	}
}

// GetConflictStrategyName retourne le nom de la stratégie
func (sc *SyncConfig) GetConflictStrategyName() string {
	switch sc.ConflictStrategy {
	case ConflictAskUser:
		return "Demander"
	case ConflictKeepNewest:
		return "Plus récent"
	case ConflictKeepLocal:
		return "Version locale"
	case ConflictKeepRemote:
		return "Version distante"
	case ConflictKeepBoth:
		return "Garder les deux"
	case ConflictAutoMerge:
		return "Fusion auto"
	default:
		return "Inconnu"
	}
}

// ShouldSendToUser vérifie si on doit envoyer au client (User)
func (sc *SyncConfig) ShouldSendToUser() bool {
	return sc.Mode == SyncModeBidirectional ||
		sc.Mode == SyncModeHostToUser ||
		sc.Mode == SyncModeMirror
}

// ShouldReceiveFromUser vérifie si on doit recevoir du client (User)
func (sc *SyncConfig) ShouldReceiveFromUser() bool {
	return sc.Mode == SyncModeBidirectional ||
		sc.Mode == SyncModeUserToHost ||
		sc.Mode == SyncModeMerge
}

// ShouldDeleteExtra vérifie si on doit supprimer les fichiers en trop
func (sc *SyncConfig) ShouldDeleteExtra() bool {
	return sc.Mode == SyncModeMirror
}

// ShouldNeverDelete vérifie si on ne doit jamais supprimer
func (sc *SyncConfig) ShouldNeverDelete() bool {
	return sc.Mode == SyncModeMerge
}

// CompressData compresse des données avec gzip
func CompressData(data []byte, level int) ([]byte, error) {
	if level < 1 || level > 9 {
		level = 6
	}
	
	var buf bytes.Buffer
	writer, err := gzip.NewWriterLevel(&buf, level)
	if err != nil {
		return nil, err
	}
	
	_, err = writer.Write(data)
	if err != nil {
		writer.Close()
		return nil, err
	}
	
	err = writer.Close()
	if err != nil {
		return nil, err
	}
	
	return buf.Bytes(), nil
}

// DecompressData décompresse des données gzip
func DecompressData(data []byte) ([]byte, error) {
	reader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	
	return io.ReadAll(reader)
}

// CompressAndEncode compresse et encode en base64
func CompressAndEncode(data []byte, level int) (string, error) {
	compressed, err := CompressData(data, level)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(compressed), nil
}

// DecodeAndDecompress décode et décompresse
func DecodeAndDecompress(encoded string) ([]byte, error) {
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, err
	}
	return DecompressData(data)
}

// GetPriority retourne la priorité d'un fichier (plus bas = plus prioritaire)
func (sc *SyncConfig) GetPriority(path string, size int64) int {
	// Vérifier les extensions prioritaires
	for i, ext := range sc.PriorityExtensions {
		if len(path) > len(ext) && path[len(path)-len(ext):] == ext {
			return i
		}
	}
	
	// Priorité basée sur la taille (petits fichiers d'abord)
	if size < 1024 { // < 1 KB
		return 100
	} else if size < 1024*1024 { // < 1 MB
		return 200
	} else if size < 10*1024*1024 { // < 10 MB
		return 300
	}
	
	return 1000
}

// Scheduler gère la synchronisation planifiée
type Scheduler struct {
	config    *SyncConfig
	running   bool
	stopChan  chan bool
	callback  func()
	mu        sync.Mutex
}

// NewScheduler crée un nouveau scheduler
func NewScheduler(config *SyncConfig, callback func()) *Scheduler {
	return &Scheduler{
		config:   config,
		stopChan: make(chan bool),
		callback: callback,
	}
}

// Start démarre le scheduler
func (s *Scheduler) Start() {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.running = true
	s.mu.Unlock()
	
	go s.run()
}

// Stop arrête le scheduler
func (s *Scheduler) Stop() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	s.running = false
	s.mu.Unlock()
	
	s.stopChan <- true
}

// IsRunning vérifie si le scheduler est actif
func (s *Scheduler) IsRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}

func (s *Scheduler) run() {
	ticker := time.NewTicker(s.config.ScheduleInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-s.stopChan:
			return
		case t := <-ticker.C:
			if s.shouldSync(t) {
				addLog("⏰ Synchronisation planifiée déclenchée")
				s.callback()
			}
		}
	}
}

func (s *Scheduler) shouldSync(t time.Time) bool {
	// Si pas de temps spécifiques, sync à chaque intervalle
	if len(s.config.ScheduleTimes) == 0 {
		return true
	}
	
	// Vérifier si l'heure actuelle correspond
	currentTime := t.Format("15:04")
	for _, schedTime := range s.config.ScheduleTimes {
		if currentTime == schedTime {
			return true
		}
	}
	
	return false
}

// RetryWithBackoff exécute une fonction avec retry et backoff exponentiel
func RetryWithBackoff(fn func() error, maxRetries int, initialDelay time.Duration) error {
	var lastErr error
	delay := initialDelay
	
	for i := 0; i <= maxRetries; i++ {
		lastErr = fn()
		if lastErr == nil {
			return nil
		}
		
		if i < maxRetries {
			addLog(fmt.Sprintf("⚠️ Tentative %d/%d échouée, retry dans %v", i+1, maxRetries, delay))
			time.Sleep(delay)
			delay *= 2 // Backoff exponentiel
		}
	}
	
	return fmt.Errorf("échec après %d tentatives: %v", maxRetries, lastErr)
}

// ThrottledWriter limite la vitesse d'écriture
type ThrottledWriter struct {
	writer    io.Writer
	rateLimit int64 // bytes/sec
	written   int64
	startTime time.Time
}

// NewThrottledWriter crée un writer avec limite de débit
func NewThrottledWriter(w io.Writer, rateLimit int64) *ThrottledWriter {
	return &ThrottledWriter{
		writer:    w,
		rateLimit: rateLimit,
		startTime: time.Now(),
	}
}

func (tw *ThrottledWriter) Write(p []byte) (n int, err error) {
	if tw.rateLimit <= 0 {
		return tw.writer.Write(p)
	}
	
	// Calculer le temps attendu pour les données écrites
	tw.written += int64(len(p))
	expectedDuration := time.Duration(tw.written * int64(time.Second) / tw.rateLimit)
	actualDuration := time.Since(tw.startTime)
	
	// Si on va trop vite, attendre
	if expectedDuration > actualDuration {
		time.Sleep(expectedDuration - actualDuration)
	}
	
	return tw.writer.Write(p)
}

// Global sync config
var globalSyncConfig = NewSyncConfig()
var globalTransferQueue = NewTransferQueue(1000)
var globalScheduler *Scheduler

// GetSyncConfig retourne la configuration globale
func GetSyncConfig() *SyncConfig {
	return globalSyncConfig
}

// SetSyncConfig définit la configuration globale
func SetSyncConfig(config *SyncConfig) {
	globalSyncConfig = config
}

// GetTransferQueue retourne la file de transfert globale
func GetTransferQueue() *TransferQueue {
	return globalTransferQueue
}
