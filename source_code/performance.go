package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

// ============================================================================
// 6.1 OPTIMISATION TRANSFERTS
// ============================================================================

// ParallelTransferManager gère les transferts parallèles
type ParallelTransferManager struct {
	maxWorkers    int
	activeWorkers int32
	queue         chan *TransferTask
	wg            sync.WaitGroup
	stopped       bool
	mu            sync.Mutex
	stats         TransferStats
}

// TransferTask représente une tâche de transfert
type TransferTask struct {
	ID        string
	Path      string
	Data      []byte
	Size      int64
	IsUpload  bool
	Priority  int
	Callback  func(*TransferResult)
}

// TransferResult représente le résultat d'un transfert
type TransferResult struct {
	TaskID    string
	Path      string
	Success   bool
	Error     error
	BytesSent int64
	Duration  time.Duration
}

// TransferStats statistiques des transferts
type TransferStats struct {
	Completed int64
	Failed    int64
	Bytes     int64
	StartTime time.Time
}

// NewParallelTransferManager crée un nouveau gestionnaire
func NewParallelTransferManager(maxWorkers int) *ParallelTransferManager {
	if maxWorkers <= 0 {
		maxWorkers = runtime.NumCPU()
	}
	return &ParallelTransferManager{
		maxWorkers: maxWorkers,
		queue:      make(chan *TransferTask, 1000),
		stats:      TransferStats{StartTime: time.Now()},
	}
}

// Start démarre les workers
func (ptm *ParallelTransferManager) Start() {
	ptm.mu.Lock()
	defer ptm.mu.Unlock()
	
	if ptm.stopped {
		return
	}
	
	for i := 0; i < ptm.maxWorkers; i++ {
		ptm.wg.Add(1)
		go ptm.worker()
	}
}

// Stop arrête les workers
func (ptm *ParallelTransferManager) Stop() {
	ptm.mu.Lock()
	ptm.stopped = true
	ptm.mu.Unlock()
	close(ptm.queue)
	ptm.wg.Wait()
}

// Submit soumet une tâche
func (ptm *ParallelTransferManager) Submit(task *TransferTask) bool {
	ptm.mu.Lock()
	if ptm.stopped {
		ptm.mu.Unlock()
		return false
	}
	ptm.mu.Unlock()
	
	select {
	case ptm.queue <- task:
		return true
	default:
		return false
	}
}

func (ptm *ParallelTransferManager) worker() {
	defer ptm.wg.Done()
	
	for task := range ptm.queue {
		atomic.AddInt32(&ptm.activeWorkers, 1)
		start := time.Now()
		
		result := &TransferResult{
			TaskID:  task.ID,
			Path:    task.Path,
			Success: true,
		}
		
		if task.Data != nil {
			result.BytesSent = int64(len(task.Data))
			atomic.AddInt64(&ptm.stats.Bytes, result.BytesSent)
		}
		
		result.Duration = time.Since(start)
		atomic.AddInt64(&ptm.stats.Completed, 1)
		
		if task.Callback != nil {
			task.Callback(result)
		}
		
		atomic.AddInt32(&ptm.activeWorkers, -1)
	}
}

// GetActiveWorkers retourne le nombre de workers actifs
func (ptm *ParallelTransferManager) GetActiveWorkers() int {
	return int(atomic.LoadInt32(&ptm.activeWorkers))
}

// GetStats retourne les statistiques
func (ptm *ParallelTransferManager) GetStats() TransferStats {
	return TransferStats{
		Completed: atomic.LoadInt64(&ptm.stats.Completed),
		Failed:    atomic.LoadInt64(&ptm.stats.Failed),
		Bytes:     atomic.LoadInt64(&ptm.stats.Bytes),
		StartTime: ptm.stats.StartTime,
	}
}

// ============================================================================
// BUFFER POOL - Réutilisation des buffers mémoire
// ============================================================================

// BufferPool gère un pool de buffers réutilisables
type BufferPool struct {
	pools map[int]*sync.Pool
	stats BufferPoolStats
}

// BufferPoolStats statistiques du pool
type BufferPoolStats struct {
	Gets    int64
	Puts    int64
	Misses  int64
	Created int64
}

var bufferSizes = []int{1024, 4096, 16384, 65536, 262144, 1048576}

// NewBufferPool crée un nouveau pool
func NewBufferPool() *BufferPool {
	bp := &BufferPool{pools: make(map[int]*sync.Pool)}
	
	for _, size := range bufferSizes {
		s := size
		bp.pools[size] = &sync.Pool{
			New: func() interface{} {
				atomic.AddInt64(&bp.stats.Created, 1)
				return make([]byte, s)
			},
		}
	}
	return bp
}

// Get obtient un buffer
func (bp *BufferPool) Get(size int) []byte {
	atomic.AddInt64(&bp.stats.Gets, 1)
	
	for _, poolSize := range bufferSizes {
		if poolSize >= size {
			if pool, ok := bp.pools[poolSize]; ok {
				return pool.Get().([]byte)
			}
		}
	}
	
	atomic.AddInt64(&bp.stats.Misses, 1)
	return make([]byte, size)
}

// Put remet un buffer
func (bp *BufferPool) Put(buf []byte) {
	atomic.AddInt64(&bp.stats.Puts, 1)
	
	size := cap(buf)
	for _, poolSize := range bufferSizes {
		if poolSize == size {
			if pool, ok := bp.pools[poolSize]; ok {
				pool.Put(buf[:poolSize])
				return
			}
		}
	}
}

// GetStats retourne les statistiques
func (bp *BufferPool) GetStats() BufferPoolStats {
	return BufferPoolStats{
		Gets:    atomic.LoadInt64(&bp.stats.Gets),
		Puts:    atomic.LoadInt64(&bp.stats.Puts),
		Misses:  atomic.LoadInt64(&bp.stats.Misses),
		Created: atomic.LoadInt64(&bp.stats.Created),
	}
}

// ============================================================================
// COMPRESSION INTELLIGENTE
// ============================================================================

var incompressibleExts = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true, ".gif": true, ".webp": true,
	".mp3": true, ".mp4": true, ".avi": true, ".mkv": true, ".webm": true,
	".zip": true, ".gz": true, ".tar": true, ".rar": true, ".7z": true,
	".pdf": true, ".docx": true, ".xlsx": true, ".pptx": true,
}

// ShouldCompress détermine si un fichier devrait être compressé
func ShouldCompress(path string, size int64) bool {
	if size < 1024 || size > 50*1024*1024 {
		return false
	}
	return !incompressibleExts[filepath.Ext(path)]
}

// SmartCompress compresse intelligemment
func SmartCompress(data []byte, path string) ([]byte, bool) {
	if !ShouldCompress(path, int64(len(data))) {
		return data, false
	}
	
	compressed, err := CompressData(data, 6)
	if err != nil || len(compressed) > int(float64(len(data))*0.9) {
		return data, false
	}
	
	return compressed, true
}

// ============================================================================
// CHUNK MANAGER - Transfert par morceaux
// ============================================================================

// ChunkManager gère les transferts par morceaux
type ChunkManager struct {
	defaultSize int
	minSize     int
	maxSize     int
	netSpeed    int64
	mu          sync.Mutex
}

// NewChunkManager crée un gestionnaire de chunks
func NewChunkManager() *ChunkManager {
	return &ChunkManager{
		defaultSize: 256 * 1024,
		minSize:     64 * 1024,
		maxSize:     4 * 1024 * 1024,
		netSpeed:    1024 * 1024,
	}
}

// GetOptimalSize calcule la taille optimale
func (cm *ChunkManager) GetOptimalSize(fileSize int64) int {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	
	switch {
	case fileSize < 1024*1024:
		return cm.minSize
	case fileSize < 10*1024*1024:
		return 256 * 1024
	case fileSize < 100*1024*1024:
		return 512 * 1024
	default:
		return cm.maxSize
	}
}

// UpdateNetSpeed met à jour la vitesse réseau
func (cm *ChunkManager) UpdateNetSpeed(bytes int64, duration time.Duration) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	
	if duration > 0 {
		newSpeed := int64(float64(bytes) / duration.Seconds())
		cm.netSpeed = (cm.netSpeed + newSpeed) / 2
	}
}

// ChunkData découpe des données
func (cm *ChunkManager) ChunkData(data []byte) [][]byte {
	chunkSize := cm.GetOptimalSize(int64(len(data)))
	var chunks [][]byte
	
	for i := 0; i < len(data); i += chunkSize {
		end := i + chunkSize
		if end > len(data) {
			end = len(data)
		}
		chunks = append(chunks, data[i:end])
	}
	
	return chunks
}

// ============================================================================
// 6.2 OPTIMISATION FILESYSTEM
// ============================================================================

// DirectoryCache cache les répertoires
type DirectoryCache struct {
	entries    map[string]*DirCacheEntry
	mu         sync.RWMutex
	maxAge     time.Duration
	maxEntries int
}

// DirCacheEntry entrée de cache
type DirCacheEntry struct {
	Path     string
	Files    []CachedFileInfo
	ModTime  time.Time
	CachedAt time.Time
}

// CachedFileInfo info fichier cachée
type CachedFileInfo struct {
	Name    string
	Size    int64
	ModTime time.Time
	IsDir   bool
}

// NewDirectoryCache crée un cache de répertoires
func NewDirectoryCache(maxAge time.Duration, maxEntries int) *DirectoryCache {
	dc := &DirectoryCache{
		entries:    make(map[string]*DirCacheEntry),
		maxAge:     maxAge,
		maxEntries: maxEntries,
	}
	go dc.cleanupLoop()
	return dc
}

// Get récupère une entrée
func (dc *DirectoryCache) Get(path string) (*DirCacheEntry, bool) {
	dc.mu.RLock()
	defer dc.mu.RUnlock()
	
	entry, ok := dc.entries[path]
	if !ok || time.Since(entry.CachedAt) > dc.maxAge {
		return nil, false
	}
	return entry, true
}

// Set ajoute une entrée
func (dc *DirectoryCache) Set(path string, files []CachedFileInfo, modTime time.Time) {
	dc.mu.Lock()
	defer dc.mu.Unlock()
	
	if len(dc.entries) >= dc.maxEntries {
		dc.evictOldest()
	}
	
	dc.entries[path] = &DirCacheEntry{
		Path:     path,
		Files:    files,
		ModTime:  modTime,
		CachedAt: time.Now(),
	}
}

// Invalidate invalide une entrée
func (dc *DirectoryCache) Invalidate(path string) {
	dc.mu.Lock()
	defer dc.mu.Unlock()
	delete(dc.entries, path)
}

// InvalidateAll invalide tout
func (dc *DirectoryCache) InvalidateAll() {
	dc.mu.Lock()
	defer dc.mu.Unlock()
	dc.entries = make(map[string]*DirCacheEntry)
}

// Size retourne la taille du cache
func (dc *DirectoryCache) Size() int {
	dc.mu.RLock()
	defer dc.mu.RUnlock()
	return len(dc.entries)
}

func (dc *DirectoryCache) evictOldest() {
	var oldestPath string
	var oldestTime time.Time
	
	for path, entry := range dc.entries {
		if oldestPath == "" || entry.CachedAt.Before(oldestTime) {
			oldestPath = path
			oldestTime = entry.CachedAt
		}
	}
	
	if oldestPath != "" {
		delete(dc.entries, oldestPath)
	}
}

func (dc *DirectoryCache) cleanupLoop() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	
	for range ticker.C {
		dc.mu.Lock()
		now := time.Now()
		for path, entry := range dc.entries {
			if now.Sub(entry.CachedAt) > dc.maxAge {
				delete(dc.entries, path)
			}
		}
		dc.mu.Unlock()
	}
}

// ============================================================================
// FILE HASH CACHE
// ============================================================================

// FileHashCache cache les hash de fichiers
type FileHashCache struct {
	hashes map[string]*HashEntry
	mu     sync.RWMutex
}

// HashEntry entrée de hash
type HashEntry struct {
	Hash    string
	ModTime time.Time
	Size    int64
}

// NewFileHashCache crée un cache de hash
func NewFileHashCache() *FileHashCache {
	return &FileHashCache{
		hashes: make(map[string]*HashEntry),
	}
}

// GetHash récupère ou calcule un hash
func (fhc *FileHashCache) GetHash(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	
	fhc.mu.RLock()
	entry, ok := fhc.hashes[path]
	fhc.mu.RUnlock()
	
	if ok && entry.ModTime.Equal(info.ModTime()) && entry.Size == info.Size() {
		return entry.Hash, nil
	}
	
	hash, err := StreamHash(path)
	if err != nil {
		return "", err
	}
	
	fhc.mu.Lock()
	fhc.hashes[path] = &HashEntry{
		Hash:    hash,
		ModTime: info.ModTime(),
		Size:    info.Size(),
	}
	fhc.mu.Unlock()
	
	return hash, nil
}

// Invalidate invalide une entrée
func (fhc *FileHashCache) Invalidate(path string) {
	fhc.mu.Lock()
	defer fhc.mu.Unlock()
	delete(fhc.hashes, path)
}

// Size retourne la taille
func (fhc *FileHashCache) Size() int {
	fhc.mu.RLock()
	defer fhc.mu.RUnlock()
	return len(fhc.hashes)
}

// Clear vide le cache
func (fhc *FileHashCache) Clear() {
	fhc.mu.Lock()
	defer fhc.mu.Unlock()
	fhc.hashes = make(map[string]*HashEntry)
}

// ============================================================================
// PARALLEL SCANNER
// ============================================================================

// ParallelScanner scanne en parallèle
type ParallelScanner struct {
	workers int
	wg      sync.WaitGroup
}

// ScanResultItem résultat de scan
type ScanResultItem struct {
	Path  string
	Files []CachedFileInfo
	Error error
}

// NewParallelScanner crée un scanner parallèle
func NewParallelScanner(workers int) *ParallelScanner {
	if workers <= 0 {
		workers = runtime.NumCPU()
	}
	return &ParallelScanner{workers: workers}
}

// ScanDirectory scanne un répertoire
func (ps *ParallelScanner) ScanDirectory(root string) ([]*ScanResultItem, error) {
	dirs := make(chan string, 1000)
	results := make([]*ScanResultItem, 0)
	var resultMu sync.Mutex
	
	for i := 0; i < ps.workers; i++ {
		ps.wg.Add(1)
		go func() {
			defer ps.wg.Done()
			for dir := range dirs {
				result := &ScanResultItem{Path: dir}
				entries, err := os.ReadDir(dir)
				if err != nil {
					result.Error = err
				} else {
					for _, e := range entries {
						info, _ := e.Info()
						if info != nil {
							result.Files = append(result.Files, CachedFileInfo{
								Name:    e.Name(),
								Size:    info.Size(),
								ModTime: info.ModTime(),
								IsDir:   e.IsDir(),
							})
						}
					}
				}
				resultMu.Lock()
				results = append(results, result)
				resultMu.Unlock()
			}
		}()
	}
	
	go func() {
		filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err == nil && info.IsDir() {
				dirs <- path
			}
			return nil
		})
		close(dirs)
	}()
	
	ps.wg.Wait()
	return results, nil
}

// ============================================================================
// 6.3 OPTIMISATION MÉMOIRE
// ============================================================================

// MemoryMonitor surveille la mémoire
type MemoryMonitor struct {
	limitMB   uint64
	warnPct   float64
	running   bool
	stopChan  chan bool
	callbacks []func(MemoryStatus)
	mu        sync.Mutex
}

// MemoryStatus statut mémoire
type MemoryStatus struct {
	Alloc      uint64
	Sys        uint64
	HeapAlloc  uint64
	NumGC      uint32
	Percentage float64
	Warning    bool
}

// NewMemoryMonitor crée un moniteur
func NewMemoryMonitor(limitMB uint64, warnPct float64) *MemoryMonitor {
	return &MemoryMonitor{
		limitMB:  limitMB,
		warnPct:  warnPct,
		stopChan: make(chan bool),
	}
}

// Start démarre le monitoring
func (mm *MemoryMonitor) Start(interval time.Duration) {
	mm.mu.Lock()
	if mm.running {
		mm.mu.Unlock()
		return
	}
	mm.running = true
	mm.mu.Unlock()
	
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		
		for {
			select {
			case <-mm.stopChan:
				return
			case <-ticker.C:
				status := mm.GetStatus()
				if status.Warning {
					mm.mu.Lock()
					for _, cb := range mm.callbacks {
						go cb(status)
					}
					mm.mu.Unlock()
				}
			}
		}
	}()
}

// Stop arrête le monitoring
func (mm *MemoryMonitor) Stop() {
	mm.mu.Lock()
	mm.running = false
	mm.mu.Unlock()
	mm.stopChan <- true
}

// GetStatus retourne le statut
func (mm *MemoryMonitor) GetStatus() MemoryStatus {
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	
	limitBytes := mm.limitMB * 1024 * 1024
	pct := float64(stats.HeapAlloc) / float64(limitBytes) * 100
	
	return MemoryStatus{
		Alloc:      stats.Alloc,
		Sys:        stats.Sys,
		HeapAlloc:  stats.HeapAlloc,
		NumGC:      stats.NumGC,
		Percentage: pct,
		Warning:    pct >= mm.warnPct,
	}
}

// OnWarning ajoute un callback
func (mm *MemoryMonitor) OnWarning(cb func(MemoryStatus)) {
	mm.mu.Lock()
	defer mm.mu.Unlock()
	mm.callbacks = append(mm.callbacks, cb)
}

// ForceGC force un GC
func (mm *MemoryMonitor) ForceGC() {
	runtime.GC()
}

// ============================================================================
// 6.4 OPTIMISATION INTERFACE
// ============================================================================

// Debouncer regroupe les appels rapides
type Debouncer struct {
	delay time.Duration
	timer *time.Timer
	mu    sync.Mutex
}

// NewDebouncer crée un debouncer
func NewDebouncer(delay time.Duration) *Debouncer {
	return &Debouncer{delay: delay}
}

// Call appelle avec debounce
func (d *Debouncer) Call(fn func()) {
	d.mu.Lock()
	defer d.mu.Unlock()
	
	if d.timer != nil {
		d.timer.Stop()
	}
	
	d.timer = time.AfterFunc(d.delay, fn)
}

// Throttler limite la fréquence
type Throttler struct {
	interval time.Duration
	lastCall time.Time
	mu       sync.Mutex
}

// NewThrottler crée un throttler
func NewThrottler(interval time.Duration) *Throttler {
	return &Throttler{interval: interval}
}

// Call appelle avec throttle
func (t *Throttler) Call(fn func()) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	
	if time.Since(t.lastCall) < t.interval {
		return false
	}
	
	t.lastCall = time.Now()
	fn()
	return true
}

// ============================================================================
// BATCH PROCESSOR
// ============================================================================

// BatchProcessor traite par lots
type BatchProcessor struct {
	batchSize int
	timeout   time.Duration
	pending   []interface{}
	mu        sync.Mutex
	processor func([]interface{})
	timer     *time.Timer
}

// NewBatchProcessor crée un processeur
func NewBatchProcessor(batchSize int, timeout time.Duration, processor func([]interface{})) *BatchProcessor {
	return &BatchProcessor{
		batchSize: batchSize,
		timeout:   timeout,
		pending:   make([]interface{}, 0),
		processor: processor,
	}
}

// Add ajoute un élément
func (bp *BatchProcessor) Add(item interface{}) {
	bp.mu.Lock()
	defer bp.mu.Unlock()
	
	bp.pending = append(bp.pending, item)
	
	if len(bp.pending) >= bp.batchSize {
		bp.flush()
		return
	}
	
	if bp.timer == nil {
		bp.timer = time.AfterFunc(bp.timeout, func() {
			bp.mu.Lock()
			bp.flush()
			bp.mu.Unlock()
		})
	}
}

// Flush traite le lot
func (bp *BatchProcessor) Flush() {
	bp.mu.Lock()
	defer bp.mu.Unlock()
	bp.flush()
}

func (bp *BatchProcessor) flush() {
	if len(bp.pending) == 0 {
		return
	}
	
	if bp.timer != nil {
		bp.timer.Stop()
		bp.timer = nil
	}
	
	items := bp.pending
	bp.pending = make([]interface{}, 0)
	
	go bp.processor(items)
}

// ============================================================================
// GLOBAL INSTANCES
// ============================================================================

var (
	globalBufferPool    = NewBufferPool()
	globalDirCache      = NewDirectoryCache(5*time.Minute, 1000)
	globalHashCache     = NewFileHashCache()
	globalChunkManager  = NewChunkManager()
	globalMemoryMonitor = NewMemoryMonitor(512, 80)
	globalDebouncer     = NewDebouncer(100 * time.Millisecond)
	globalThrottler     = NewThrottler(50 * time.Millisecond)
)

// GetBufferPool retourne le pool global
func GetBufferPool() *BufferPool { return globalBufferPool }

// GetDirCache retourne le cache global
func GetDirCache() *DirectoryCache { return globalDirCache }

// GetHashCache retourne le cache de hash global
func GetHashCache() *FileHashCache { return globalHashCache }

// GetChunkManager retourne le gestionnaire de chunks global
func GetChunkManager() *ChunkManager { return globalChunkManager }

// GetMemoryMonitor retourne le moniteur mémoire global
func GetMemoryMonitor() *MemoryMonitor { return globalMemoryMonitor }

// ============================================================================
// UTILITAIRES
// ============================================================================

// StreamHash calcule le hash en streaming
func StreamHash(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	
	h := sha256.New()
	buf := globalBufferPool.Get(65536)
	defer globalBufferPool.Put(buf)
	
	for {
		n, err := file.Read(buf)
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
		h.Write(buf[:n])
	}
	
	return hex.EncodeToString(h.Sum(nil)), nil
}

// CompareFiles compare deux fichiers efficacement
func CompareFiles(path1, path2 string) (bool, error) {
	info1, err := os.Stat(path1)
	if err != nil {
		return false, err
	}
	
	info2, err := os.Stat(path2)
	if err != nil {
		return false, err
	}
	
	if info1.Size() != info2.Size() {
		return false, nil
	}
	
	file1, _ := os.Open(path1)
	file2, _ := os.Open(path2)
	defer file1.Close()
	defer file2.Close()
	
	buf1 := globalBufferPool.Get(65536)
	buf2 := globalBufferPool.Get(65536)
	defer globalBufferPool.Put(buf1)
	defer globalBufferPool.Put(buf2)
	
	for {
		n1, err1 := file1.Read(buf1)
		n2, err2 := file2.Read(buf2)
		
		if n1 != n2 || !bytes.Equal(buf1[:n1], buf2[:n2]) {
			return false, nil
		}
		
		if err1 == io.EOF && err2 == io.EOF {
			return true, nil
		}
		
		if err1 != nil || err2 != nil {
			return false, fmt.Errorf("read error")
		}
	}
}

// PerformanceStats statistiques globales
type PerformanceStats struct {
	TransfersCompleted int64
	BytesTransferred   int64
	CacheHits          int64
	CacheMisses        int64
	MemoryUsage        uint64
	GCRuns             uint32
}

var perfStats PerformanceStats
var perfMu sync.Mutex

// GetPerfStats retourne les statistiques
func GetPerfStats() PerformanceStats {
	perfMu.Lock()
	defer perfMu.Unlock()
	
	status := GetMemoryMonitor().GetStatus()
	perfStats.MemoryUsage = status.HeapAlloc
	perfStats.GCRuns = status.NumGC
	
	return perfStats
}

// UpdatePerfStats met à jour les stats
func UpdatePerfStats(field string, value int64) {
	perfMu.Lock()
	defer perfMu.Unlock()
	
	switch field {
	case "transfers":
		perfStats.TransfersCompleted += value
	case "bytes":
		perfStats.BytesTransferred += value
	case "cache_hit":
		perfStats.CacheHits += value
	case "cache_miss":
		perfStats.CacheMisses += value
	}
}
