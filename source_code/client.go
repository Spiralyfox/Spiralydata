package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"fyne.io/fyne/v2/widget"
	"github.com/fsnotify/fsnotify"
	"github.com/gorilla/websocket"
)

type Client struct {
	ws               *websocket.Conn
	localDir         string
	mu               sync.Mutex
	skipNext         map[string]time.Time
	knownFiles       map[string]time.Time
	knownDirs        map[string]time.Time
	lastState        map[string]time.Time
	lastDirs         map[string]time.Time
	scanRunning      bool
	shouldExit       bool
	autoSync         bool
	localChanges     []FileChange
	connectionTime   time.Time
	isProcessing     bool
	watcherActive    bool
	pendingChanges   []FileChange
	pendingMu        sync.Mutex
}

func StartClientGUI(serverAddr, hostID string, stopAnimation, connectionSuccess *bool, loadingLabel, statusLabel, infoLabel *widget.Label, client **Client) {
	addLog("🔌 Connexion au serveur " + serverAddr)
	ws, _, err := websocket.DefaultDialer.Dial("ws://"+serverAddr+"/ws", nil)
	if err != nil {
		addLog(fmt.Sprintf("❌ Impossible de se connecter: %v", err))
		*stopAnimation = true
		loadingLabel.SetText("✗ Connexion échouée")
		loadingLabel.Refresh()
		statusLabel.SetText("Statut: Échec de connexion")
		statusLabel.Refresh()
		infoLabel.SetText(fmt.Sprintf(
			"ÉCHEC DE CONNEXION\n\n"+
				"Serveur: %s\n"+
				"ID: %s\n\n"+
				"Impossible de se connecter au serveur.\n"+
				"Vérifiez l'adresse IP et le port.",
			serverAddr, hostID,
		))
		infoLabel.Refresh()
		return
	}
	addLog("✅ Connexion WebSocket établie")

	authReq := AuthRequest{
		Type:   "auth_request",
		HostID: hostID,
	}

	addLog("🔐 Authentification en cours...")
	if err := ws.WriteJSON(authReq); err != nil {
		addLog(fmt.Sprintf("❌ Erreur d'authentification: %v", err))
		ws.Close()
		*stopAnimation = true
		loadingLabel.SetText("✗ Erreur d'authentification")
		loadingLabel.Refresh()
		statusLabel.SetText("Statut: Erreur d'authentification")
		statusLabel.Refresh()
		return
	}

	addLog("⏳ Attente de la réponse...")
	var authResp AuthResponse
	if err := ws.ReadJSON(&authResp); err != nil {
		addLog(fmt.Sprintf("❌ Pas de réponse du serveur: %v", err))
		ws.Close()
		*stopAnimation = true
		loadingLabel.SetText("✗ Pas de réponse")
		loadingLabel.Refresh()
		statusLabel.SetText("Statut: Pas de réponse")
		statusLabel.Refresh()
		return
	}

	if authResp.Type == "auth_failed" {
		addLog(fmt.Sprintf("🚫 Authentification refusée: %s", authResp.Message))
		ws.Close()
		*stopAnimation = true
		loadingLabel.SetText("✗ ID incorrect")
		loadingLabel.Refresh()
		statusLabel.SetText("Statut: ID incorrect")
		statusLabel.Refresh()
		infoLabel.SetText(fmt.Sprintf(
			"AUTHENTIFICATION REFUSÉE\n\n"+
				"Serveur: %s\n"+
				"ID: %s\n\n"+
				"L'ID du host est incorrect.\n"+
				"Vérifiez l'ID et réessayez.",
			serverAddr, hostID,
		))
		infoLabel.Refresh()
		return
	}

	*stopAnimation = true
	*connectionSuccess = true
	addLog(fmt.Sprintf("🎉 Connecté au serveur %s", serverAddr))
	addLog(fmt.Sprintf("🔑 ID validé: %s", hostID))
	
	loadingLabel.SetText("✓ Connecté")
	loadingLabel.Refresh()
	statusLabel.SetText("Statut: Connecté (Mode Manuel)")
	statusLabel.Refresh()
	
	infoLabel.SetText(fmt.Sprintf(
		"CONNECTÉ\n\n"+
			"Serveur: %s\n"+
			"ID: %s\n\n"+
			"Mode: Manuel\n"+
			"Activez la sync pour synchroniser automatiquement",
		serverAddr, hostID,
	))
	infoLabel.Refresh()

	localDir := filepath.Join(getExecutableDir(), "Spiralydata")
	if err := os.MkdirAll(localDir, 0755); err != nil {
		addLog(fmt.Sprintf("❌ Impossible de créer le dossier: %v", err))
	} else {
		addLog(fmt.Sprintf("📁 Dossier: %s", localDir))
	}

	*client = &Client{
		ws:             ws,
		localDir:       localDir,
		skipNext:       make(map[string]time.Time),
		knownFiles:     make(map[string]time.Time),
		knownDirs:      make(map[string]time.Time),
		lastState:      make(map[string]time.Time),
		lastDirs:       make(map[string]time.Time),
		shouldExit:     false,
		autoSync:       false,
		localChanges:   []FileChange{},
		connectionTime: time.Now(),
		isProcessing:   false,
		watcherActive:  false,
		pendingChanges: []FileChange{},
	}

	addLog("🔍 Scan initial du dossier local...")
	(*client).scanInitial()
	addLog("✅ Client prêt - Mode Manuel")
	addLog("👀 En attente de commandes...")

	go (*client).watchRecursive()

	for {
		var rawMsg json.RawMessage
		if err := ws.ReadJSON(&rawMsg); err != nil {
			if !(*client).shouldExit {
				addLog("💔 Connexion perdue")
				*connectionSuccess = false
				loadingLabel.SetText("✗ Connexion perdue")
				loadingLabel.Refresh()
				statusLabel.SetText("Statut: Déconnecté")
				statusLabel.Refresh()
			}
			ws.Close()
			break
		}

		var msg FileChange
		if err := json.Unmarshal(rawMsg, &msg); err == nil {
			if msg.Origin != "client" {
				if (*client).autoSync {
					logPrefix := "📥 "
					if msg.IsDir {
						if msg.Op == "mkdir" {
							addLog(logPrefix + "Dossier créé: " + msg.FileName)
						} else if msg.Op == "remove" {
							addLog(logPrefix + "Dossier supprimé: " + msg.FileName)
						}
					} else {
						if msg.Op == "create" {
							addLog(logPrefix + "Nouveau: " + msg.FileName)
						} else if msg.Op == "write" {
							addLog(logPrefix + "Modifié: " + msg.FileName)
						} else if msg.Op == "remove" {
							addLog(logPrefix + "Supprimé: " + msg.FileName)
						}
					}
					(*client).applyChange(msg)
				} else {
					if (*client).isProcessing {
						logPrefix := "📥 "
						if msg.IsDir {
							if msg.Op == "mkdir" {
								addLog(logPrefix + "Dossier reçu: " + msg.FileName)
							}
						} else {
							if msg.Op == "create" || msg.Op == "write" {
								addLog(logPrefix + "Fichier reçu: " + msg.FileName)
							}
						}
						(*client).applyChange(msg)
					} else {
						(*client).pendingMu.Lock()
						(*client).pendingChanges = append((*client).pendingChanges, msg)
						pendingCount := len((*client).pendingChanges)
						(*client).pendingMu.Unlock()
						
						addLog(fmt.Sprintf("📋 En attente (%d): %s", pendingCount, msg.FileName))
					}
				}
			}
		}
	}
}

func (c *Client) PullAllFromServer() {
	if c.isProcessing {
		addLog("⏳ Opération en cours, veuillez patienter...")
		return
	}
	
	c.isProcessing = true
	
	c.pendingMu.Lock()
	pendingCount := len(c.pendingChanges)
	if pendingCount > 0 {
		addLog(fmt.Sprintf("📦 Application de %d changements en attente...", pendingCount))
		for i, change := range c.pendingChanges {
			if i%10 == 0 || i == pendingCount-1 {
				logPrefix := "📥 "
				if change.IsDir {
					if change.Op == "mkdir" {
						addLog(logPrefix + "Dossier: " + change.FileName)
					} else if change.Op == "remove" {
						addLog(logPrefix + "Suppression dossier: " + change.FileName)
					}
				} else {
					if change.Op == "create" {
						addLog(logPrefix + "Nouveau: " + change.FileName)
					} else if change.Op == "write" {
						addLog(logPrefix + "Modifié: " + change.FileName)
					} else if change.Op == "remove" {
						addLog(logPrefix + "Supprimé: " + change.FileName)
					}
				}
			}
			c.applyChange(change)
			time.Sleep(100 * time.Millisecond)
		}
		c.pendingChanges = []FileChange{}
		addLog("✅ Changements appliqués")
	}
	c.pendingMu.Unlock()
	
	addLog("🔄 Demande de tous les fichiers du serveur...")
	
	reqMsg := map[string]string{
		"type":   "request_all_files",
		"origin": "client",
	}
	
	if err := c.ws.WriteJSON(reqMsg); err != nil {
		addLog(fmt.Sprintf("❌ Erreur lors de l'envoi: %v", err))
		c.isProcessing = false
		return
	}
	
	addLog("📡 Requête envoyée, réception en cours...")
	
	go func() {
		time.Sleep(3 * time.Second)
		c.isProcessing = false
		addLog("✅ Réception terminée")
	}()
}

func (c *Client) PushLocalChanges() {
	if c.isProcessing {
		addLog("⏳ Opération en cours, veuillez patienter...")
		return
	}
	
	c.isProcessing = true
	addLog("🔄 Synchronisation avec le serveur...")
	
	allFiles := make(map[string]time.Time)
	allDirs := make(map[string]time.Time)
	c.scanCurrentState(c.localDir, "", allFiles, allDirs)
	
	sent := 0
	
	c.mu.Lock()
	for knownDir := range c.knownDirs {
		if _, exists := allDirs[knownDir]; !exists {
			change := FileChange{
				FileName: knownDir,
				Op:       "remove",
				IsDir:    true,
				Origin:   "client",
			}
			c.ws.WriteJSON(change)
			delete(c.knownDirs, knownDir)
			sent++
			if sent%10 == 0 {
				addLog(fmt.Sprintf("📤 Suppression dossier: %s", knownDir))
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
	
	for knownFile := range c.knownFiles {
		if _, exists := allFiles[knownFile]; !exists {
			change := FileChange{
				FileName: knownFile,
				Op:       "remove",
				IsDir:    false,
				Origin:   "client",
			}
			c.ws.WriteJSON(change)
			delete(c.knownFiles, knownFile)
			sent++
			if sent%10 == 0 {
				addLog(fmt.Sprintf("📤 Suppression: %s", knownFile))
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
	c.mu.Unlock()
	
	for dirPath := range allDirs {
		c.mu.Lock()
		_, known := c.knownDirs[dirPath]
		c.mu.Unlock()
		
		if !known {
			change := FileChange{
				FileName: dirPath,
				Op:       "mkdir",
				IsDir:    true,
				Origin:   "client",
			}
			c.ws.WriteJSON(change)
			c.mu.Lock()
			c.knownDirs[dirPath] = time.Now()
			c.mu.Unlock()
			sent++
			if sent%10 == 0 {
				addLog(fmt.Sprintf("📤 Dossier: %s", dirPath))
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
	
	for filePath, modTime := range allFiles {
		c.mu.Lock()
		lastMod, known := c.knownFiles[filePath]
		shouldSend := !known || modTime.After(lastMod)
		c.mu.Unlock()
		
		if shouldSend {
			fullPath := filepath.Join(c.localDir, filepath.FromSlash(filePath))
			data, err := os.ReadFile(fullPath)
			if err != nil {
				continue
			}
			
			change := FileChange{
				FileName: filePath,
				Op:       "write",
				Content:  base64.StdEncoding.EncodeToString(data),
				IsDir:    false,
				Origin:   "client",
			}
			c.ws.WriteJSON(change)
			c.mu.Lock()
			c.knownFiles[filePath] = modTime
			c.mu.Unlock()
			sent++
			if sent%10 == 0 {
				addLog(fmt.Sprintf("📤 Fichier: %s", filePath))
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
	
	c.isProcessing = false
	addLog(fmt.Sprintf("✅ Synchronisation terminée: %d opérations", sent))
}

func (c *Client) ClearLocalFiles() {
	if c.isProcessing {
		addLog("⏳ Opération en cours, veuillez patienter...")
		return
	}
	
	if c.autoSync {
		addLog("⚠️ Désactivez d'abord la synchronisation automatique")
		return
	}
	
	c.isProcessing = true
	defer func() { c.isProcessing = false }()
	
	addLog("🗑️ Suppression de tous les fichiers locaux...")
	
	entries, err := os.ReadDir(c.localDir)
	if err != nil {
		addLog(fmt.Sprintf("❌ Impossible de lire le dossier: %v", err))
		return
	}
	
	count := 0
	for _, entry := range entries {
		path := filepath.Join(c.localDir, entry.Name())
		if err := os.RemoveAll(path); err == nil {
			count++
			addLog(fmt.Sprintf("🗑️ Supprimé: %s", entry.Name()))
		}
		time.Sleep(100 * time.Millisecond)
	}
	
	c.mu.Lock()
	c.knownFiles = make(map[string]time.Time)
	c.knownDirs = make(map[string]time.Time)
	c.lastState = make(map[string]time.Time)
	c.lastDirs = make(map[string]time.Time)
	c.mu.Unlock()
	
	addLog(fmt.Sprintf("✅ Suppression terminée: %d éléments", count))
}

func (c *Client) ToggleAutoSync() {
	c.mu.Lock()
	c.autoSync = !c.autoSync
	status := c.autoSync
	c.mu.Unlock()
	
	if status {
		addLog("🟢 Synchronisation automatique ACTIVÉE")
		
		c.pendingMu.Lock()
		pendingCount := len(c.pendingChanges)
		if pendingCount > 0 {
			addLog(fmt.Sprintf("📦 Application de %d changements...", pendingCount))
			for _, change := range c.pendingChanges {
				c.applyChange(change)
			}
			c.pendingChanges = []FileChange{}
		}
		c.pendingMu.Unlock()
		
		go c.periodicScanner()
	} else {
		addLog("🔴 Synchronisation automatique DÉSACTIVÉE")
		
		c.pendingMu.Lock()
		pendingCount := len(c.pendingChanges)
		c.pendingMu.Unlock()
		
		if pendingCount > 0 {
			addLog(fmt.Sprintf("📋 %d changements en attente", pendingCount))
		}
	}
}

func (c *Client) scanInitial() {
	c.scanDirRecursive(c.localDir, "")
}

func (c *Client) scanDirRecursive(basePath, relPath string) {
	fullPath := filepath.Join(basePath, relPath)
	entries, err := os.ReadDir(fullPath)
	if err != nil {
		return
	}
	
	for _, entry := range entries {
		itemRelPath := filepath.ToSlash(filepath.Join(relPath, entry.Name()))
		info, _ := entry.Info()
		
		if entry.IsDir() {
			if info != nil {
				c.lastDirs[itemRelPath] = info.ModTime()
				c.knownDirs[itemRelPath] = info.ModTime()
			}
			c.scanDirRecursive(basePath, filepath.Join(relPath, entry.Name()))
		} else {
			if info != nil {
				c.lastState[itemRelPath] = info.ModTime()
				c.knownFiles[itemRelPath] = info.ModTime()
			}
		}
	}
}

func (c *Client) watchRecursive() {
	if c.watcherActive {
		return
	}
	c.watcherActive = true
	
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		addLog(fmt.Sprintf("❌ Erreur watcher: %v", err))
		return
	}
	defer watcher.Close()

	c.addDirToWatcher(watcher, c.localDir)
	addLog("👀 Surveillance activée")

	for {
		if c.shouldExit {
			return
		}
		
		select {
		case event := <-watcher.Events:
			if c.autoSync {
				c.handleLocalEvent(event)
			}
			
			if event.Op&fsnotify.Create != 0 {
				if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
					watcher.Add(event.Name)
				}
			}
		case err := <-watcher.Errors:
			if !c.shouldExit {
				addLog(fmt.Sprintf("⚠️ Erreur watcher: %v", err))
			}
		}
	}
}

func (c *Client) handleLocalEvent(event fsnotify.Event) {
	relPath, err := filepath.Rel(c.localDir, event.Name)
	if err != nil {
		return
	}
	relPath = filepath.ToSlash(relPath)
	
	c.mu.Lock()
	if until, exists := c.skipNext[relPath]; exists && time.Now().Before(until) {
		c.mu.Unlock()
		return
	}
	c.mu.Unlock()
	
	if event.Op&fsnotify.Create != 0 || event.Op&fsnotify.Write != 0 {
		info, err := os.Stat(event.Name)
		if err != nil {
			return
		}
		
		if info.IsDir() {
			change := FileChange{
				FileName: relPath,
				Op:       "mkdir",
				IsDir:    true,
				Origin:   "client",
			}
			c.ws.WriteJSON(change)
			addLog(fmt.Sprintf("📤 Dossier créé: %s", relPath))
		} else {
			data, err := os.ReadFile(event.Name)
			if err != nil {
				return
			}
			change := FileChange{
				FileName: relPath,
				Op:       "write",
				Content:  base64.StdEncoding.EncodeToString(data),
				IsDir:    false,
				Origin:   "client",
			}
			c.ws.WriteJSON(change)
			addLog(fmt.Sprintf("📤 Modifié: %s", relPath))
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func (c *Client) addDirToWatcher(watcher *fsnotify.Watcher, dir string) {
	watcher.Add(dir)
	
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	
	for _, entry := range entries {
		if entry.IsDir() {
			subDir := filepath.Join(dir, entry.Name())
			c.addDirToWatcher(watcher, subDir)
		}
	}
}

func (c *Client) periodicScanner() {
	if c.scanRunning || !c.autoSync {
		return
	}
	c.scanRunning = true
	defer func() { c.scanRunning = false }()

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		if c.shouldExit || !c.autoSync {
			return
		}

		currentFiles := make(map[string]time.Time)
		currentDirs := make(map[string]time.Time)
		c.scanCurrentState(c.localDir, "", currentFiles, currentDirs)

		c.mu.Lock()
		
		for oldDir := range c.lastDirs {
			if until, exists := c.skipNext[oldDir]; exists && time.Now().Before(until) {
				continue
			}
			if _, exists := currentDirs[oldDir]; !exists {
				change := FileChange{
					FileName: oldDir,
					Op:       "remove",
					IsDir:    true,
					Origin:   "client",
				}
				c.ws.WriteJSON(change)
				addLog(fmt.Sprintf("📤 Dossier supprimé: %s", oldDir))
				delete(c.knownDirs, oldDir)
				time.Sleep(100 * time.Millisecond)
			}
		}

		for newDir, modTime := range currentDirs {
			if _, known := c.lastDirs[newDir]; !known {
				if until, exists := c.skipNext[newDir]; exists && time.Now().Before(until) {
					continue
				}
				change := FileChange{
					FileName: newDir,
					Op:       "mkdir",
					IsDir:    true,
					Origin:   "client",
				}
				c.ws.WriteJSON(change)
				addLog(fmt.Sprintf("📤 Dossier créé: %s", newDir))
				c.knownDirs[newDir] = modTime
				time.Sleep(100 * time.Millisecond)
			}
		}

		for name, modTime := range currentFiles {
			if until, exists := c.skipNext[name]; exists && time.Now().Before(until) {
				continue
			}

			lastMod, known := c.lastState[name]
			if !known || modTime.After(lastMod) {
				c.sendFileNow(name)
			}
		}

		for oldFile := range c.lastState {
			if _, still := currentFiles[oldFile]; !still {
				if until, exists := c.skipNext[oldFile]; exists && time.Now().Before(until) {
					delete(c.skipNext, oldFile)
					continue
				}
				
				change := FileChange{
					FileName: oldFile,
					Op:       "remove",
					IsDir:    false,
					Origin:   "client",
				}
				c.ws.WriteJSON(change)
				addLog(fmt.Sprintf("📤 Supprimé: %s", oldFile))
				delete(c.knownFiles, oldFile)
				time.Sleep(100 * time.Millisecond)
			}
		}

		c.lastState = currentFiles
		c.lastDirs = currentDirs
		c.mu.Unlock()
	}
}

func (c *Client) scanCurrentState(basePath, relPath string, files map[string]time.Time, dirs map[string]time.Time) {
	fullPath := filepath.Join(basePath, relPath)
	entries, err := os.ReadDir(fullPath)
	if err != nil {
		return
	}

	for _, entry := range entries {
		itemRelPath := filepath.ToSlash(filepath.Join(relPath, entry.Name()))
		info, _ := entry.Info()
		
		if entry.IsDir() {
			if info != nil {
				dirs[itemRelPath] = info.ModTime()
			}
			c.scanCurrentState(basePath, filepath.Join(relPath, entry.Name()), files, dirs)
		} else {
			if info != nil {
				files[itemRelPath] = info.ModTime()
			}
		}
	}
}

func (c *Client) sendFileNow(relPath string) {
	fullPath := filepath.Join(c.localDir, filepath.FromSlash(relPath))
	data, err := os.ReadFile(fullPath)
	if err != nil {
		addLog(fmt.Sprintf("❌ Erreur lecture: %s", relPath))
		return
	}
	
	_, wasKnown := c.knownFiles[relPath]
	if wasKnown {
		addLog(fmt.Sprintf("📤 Modifié: %s", relPath))
	} else {
		addLog(fmt.Sprintf("📤 Nouveau: %s", relPath))
	}
	
	change := FileChange{
		FileName: relPath,
		Op:       "write",
		Content:  base64.StdEncoding.EncodeToString(data),
		IsDir:    false,
		Origin:   "client",
	}
	
	c.ws.WriteJSON(change)
	c.knownFiles[relPath] = time.Now()
	time.Sleep(100 * time.Millisecond)
}

func (c *Client) applyChange(msg FileChange) {
	normalizedPath := filepath.FromSlash(msg.FileName)
	target := filepath.Join(c.localDir, normalizedPath)

	c.mu.Lock()
	c.skipNext[msg.FileName] = time.Now().Add(3 * time.Second)
	c.mu.Unlock()

	switch msg.Op {
	case "mkdir":
		os.MkdirAll(target, 0755)
		c.mu.Lock()
		c.knownDirs[msg.FileName] = time.Now()
		c.lastDirs[msg.FileName] = time.Now()
		c.mu.Unlock()
		
	case "remove":
		if msg.IsDir {
			os.RemoveAll(target)
			c.mu.Lock()
			delete(c.knownDirs, msg.FileName)
			delete(c.lastDirs, msg.FileName)
			c.mu.Unlock()
		} else {
			os.Remove(target)
			c.mu.Lock()
			delete(c.knownFiles, msg.FileName)
			delete(c.lastState, msg.FileName)
			c.mu.Unlock()
		}
		
	case "create", "write":
		dir := filepath.Dir(target)
		os.MkdirAll(dir, 0755)
		
		data, _ := base64.StdEncoding.DecodeString(msg.Content)
		os.WriteFile(target, data, 0644)
		info, _ := os.Stat(target)
		c.mu.Lock()
		if info != nil {
			c.knownFiles[msg.FileName] = info.ModTime()
			c.lastState[msg.FileName] = info.ModTime()
		}
		c.mu.Unlock()
	}
}