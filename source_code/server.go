package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/gorilla/websocket"
)

type Server struct {
	HostID       string
	Clients      map[*websocket.Conn]string
	Upgrader     websocket.Upgrader
	WatchDir     string
	mu           sync.Mutex
	skipNext     map[string]time.Time
	knownFiles   map[string]time.Time
	knownDirs    map[string]time.Time
	clientNum    int
	shouldExit   bool
	httpServer   *http.Server
	pendingMoves map[string]time.Time
}

func NewServer(hostID string) *Server {
	return &Server{
		HostID:  hostID,
		Clients: make(map[*websocket.Conn]string),
		Upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
		skipNext:     make(map[string]time.Time),
		knownFiles:   make(map[string]time.Time),
		knownDirs:    make(map[string]time.Time),
		pendingMoves: make(map[string]time.Time),
		clientNum:    0,
		shouldExit:   false,
	}
}

func (s *Server) Start(port string) {
	s.WatchDir = filepath.Join(getExecutableDir(), "Spiralydata")
	os.MkdirAll(s.WatchDir, 0755)

	addLog("🚀 Serveur démarré")
	addLog(fmt.Sprintf("🔑 ID: %s", s.HostID))
	addLog(fmt.Sprintf("📁 Dossier: %s", s.WatchDir))
	addLog("👂 En attente de connexions...")

	s.updateKnownFilesAndDirs()
	go s.watchRecursive()
	go s.periodicCheck()
	go s.cleanPendingMoves()

	http.HandleFunc("/ws", s.handleWS)
	s.httpServer = &http.Server{Addr: ":" + port}
	addLog(fmt.Sprintf("🌐 Port: %s", port))
	
	if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		addLog(fmt.Sprintf("❌ Erreur serveur: %v", err))
	}
}

func (s *Server) Stop() {
	addLog("🛑 Arrêt du serveur...")
	s.shouldExit = true
	
	s.mu.Lock()
	for client := range s.Clients {
		client.Close()
	}
	s.mu.Unlock()
	
	if s.httpServer != nil {
		s.httpServer.Close()
	}
	
	addLog("✅ Serveur arrêté")
}

func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	addLog(fmt.Sprintf("🔌 Connexion depuis %s", r.RemoteAddr))
	
	ws, err := s.Upgrader.Upgrade(w, r, nil)
	if err != nil {
		addLog(fmt.Sprintf("❌ Erreur WebSocket: %v", err))
		return
	}

	var rawMsg json.RawMessage
	if err := ws.ReadJSON(&rawMsg); err != nil {
		addLog(fmt.Sprintf("❌ Erreur lecture: %v", err))
		ws.Close()
		return
	}

	var authReq AuthRequest
	if err := json.Unmarshal(rawMsg, &authReq); err != nil {
		addLog(fmt.Sprintf("❌ Erreur parsing: %v", err))
		ws.Close()
		return
	}

	if authReq.Type == "auth_request" {
		if authReq.HostID == s.HostID {
			s.mu.Lock()
			s.clientNum++
			clientName := fmt.Sprintf("Client_%d", s.clientNum)
			s.Clients[ws] = clientName
			totalClients := len(s.Clients)
			s.mu.Unlock()

			addLog(fmt.Sprintf("✅ %s connecté", clientName))
			addLog(fmt.Sprintf("👥 Clients: %d", totalClients))

			ws.WriteJSON(AuthResponse{
				Type:    "auth_success",
				Message: "Connexion établie",
			})

			addLog(fmt.Sprintf("📤 Envoi structure à %s...", clientName))
			s.sendAllFilesAndDirs(ws)
			addLog(fmt.Sprintf("✅ Structure envoyée à %s", clientName))
			
			s.handleClientMessages(ws, clientName)

		} else {
			addLog(fmt.Sprintf("🚫 Connexion refusée (ID: %s)", authReq.HostID))
			ws.WriteJSON(AuthResponse{
				Type:    "auth_failed",
				Message: "Identifiant incorrect",
			})
			ws.Close()
			return
		}
	}
}

func (s *Server) handleClientMessages(ws *websocket.Conn, clientName string) {
	defer func() {
		s.mu.Lock()
		delete(s.Clients, ws)
		remaining := len(s.Clients)
		s.mu.Unlock()
		ws.Close()
		addLog(fmt.Sprintf("❌ %s déconnecté", clientName))
		addLog(fmt.Sprintf("👥 Clients restants: %d", remaining))
	}()

	for {
		var rawMsg json.RawMessage
		if err := ws.ReadJSON(&rawMsg); err != nil {
			break
		}
		
		var reqMap map[string]string
		if err := json.Unmarshal(rawMsg, &reqMap); err == nil {
			if reqType, ok := reqMap["type"]; ok {
				if reqType == "request_all_files" {
					addLog(fmt.Sprintf("📥 %s: Demande structure complète", clientName))
					s.sendAllFilesAndDirs(ws)
					addLog(fmt.Sprintf("📤 Structure envoyée à %s", clientName))
					continue
				}
			}
		}
		
		var msg FileChange
		if err := json.Unmarshal(rawMsg, &msg); err == nil {
			if msg.Origin != "server" {
				if msg.IsDir {
					if msg.Op == "mkdir" {
						addLog(fmt.Sprintf("📥 %s: Dossier créé → %s", clientName, msg.FileName))
					} else if msg.Op == "remove" {
						addLog(fmt.Sprintf("📥 %s: Dossier supprimé → %s", clientName, msg.FileName))
					}
				} else {
					if msg.Op == "create" {
						addLog(fmt.Sprintf("📥 %s: Nouveau → %s", clientName, msg.FileName))
					} else if msg.Op == "write" {
						addLog(fmt.Sprintf("📥 %s: Modifié → %s", clientName, msg.FileName))
					} else if msg.Op == "remove" {
						addLog(fmt.Sprintf("📥 %s: Supprimé → %s", clientName, msg.FileName))
					}
				}
				
				s.applyChange(msg)
				s.broadcastExcept(msg, ws)
			}
		}
	}
}

func (s *Server) sendAllFilesAndDirs(ws *websocket.Conn) {
	s.sendDirRecursive(ws, s.WatchDir, "")
}

func (s *Server) sendDirRecursive(ws *websocket.Conn, basePath, relPath string) {
	fullPath := filepath.Join(basePath, relPath)
	entries, err := os.ReadDir(fullPath)
	if err != nil {
		return
	}

	for _, entry := range entries {
		itemRelPath := filepath.ToSlash(filepath.Join(relPath, entry.Name()))
		
		if entry.IsDir() {
			ws.WriteJSON(FileChange{
				FileName: itemRelPath,
				Op:       "mkdir",
				IsDir:    true,
				Origin:   "server",
			})
			time.Sleep(100 * time.Millisecond)
			s.sendDirRecursive(ws, basePath, filepath.Join(relPath, entry.Name()))
		} else {
			fullFilePath := filepath.Join(basePath, relPath, entry.Name())
			data, err := readFileWithRetry(fullFilePath)
			if err != nil {
				continue
			}
			ws.WriteJSON(FileChange{
				FileName: itemRelPath,
				Op:       "create",
				Content:  base64.StdEncoding.EncodeToString(data),
				IsDir:    false,
				Origin:   "server",
			})
			time.Sleep(100 * time.Millisecond)
		}
	}
}

func (s *Server) watchRecursive() {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		addLog(fmt.Sprintf("❌ Erreur watcher: %v", err))
		return
	}
	defer watcher.Close()

	s.addDirToWatcher(watcher, s.WatchDir)

	for {
		if s.shouldExit {
			return
		}
		select {
		case event := <-watcher.Events:
			s.handleEvent(event)
			
			if event.Op&fsnotify.Create != 0 {
				if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
					watcher.Add(event.Name)
				}
			}
			
		case err := <-watcher.Errors:
			if !s.shouldExit {
				addLog(fmt.Sprintf("⚠️ Erreur watcher: %v", err))
			}
		}
	}
}

func (s *Server) addDirToWatcher(watcher *fsnotify.Watcher, dir string) {
	watcher.Add(dir)
	
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	
	for _, entry := range entries {
		if entry.IsDir() {
			subDir := filepath.Join(dir, entry.Name())
			s.addDirToWatcher(watcher, subDir)
		}
	}
}

func (s *Server) handleEvent(event fsnotify.Event) {
	relPath, err := filepath.Rel(s.WatchDir, event.Name)
	if err != nil || relPath == "." {
		return
	}

	relPath = filepath.ToSlash(filepath.Clean(relPath))
	
	s.mu.Lock()
	
	if until, exists := s.skipNext[relPath]; exists && time.Now().Before(until) {
		s.mu.Unlock()
		return
	}
	
	if until, exists := s.pendingMoves[relPath]; exists && time.Now().Before(until) {
		s.mu.Unlock()
		return
	}
	
	s.mu.Unlock()

	info, err := os.Stat(event.Name)
	isDir := err == nil && info.IsDir()

	if event.Op&fsnotify.Create != 0 {
		s.mu.Lock()
		s.pendingMoves[relPath] = time.Now().Add(300 * time.Millisecond)
		s.mu.Unlock()
		
		time.Sleep(150 * time.Millisecond)
		
		if _, err := os.Stat(event.Name); err != nil {
			return
		}
		
		if isDir {
			s.mu.Lock()
			if _, known := s.knownDirs[relPath]; known {
				s.mu.Unlock()
				return
			}
			s.knownDirs[relPath] = time.Now()
			s.mu.Unlock()
			
			msg := FileChange{
				FileName: relPath,
				Op:       "mkdir",
				IsDir:    true,
				Origin:   "server",
			}
			s.broadcast(msg)
			addLog(fmt.Sprintf("📤 Dossier créé: %s", relPath))
			time.Sleep(100 * time.Millisecond)
		} else {
			s.mu.Lock()
			if _, known := s.knownFiles[relPath]; known {
				s.mu.Unlock()
				return
			}
			s.mu.Unlock()
			
			data, err := readFileWithRetry(event.Name)
			if err != nil {
				return
			}
			
			s.mu.Lock()
			s.knownFiles[relPath] = time.Now()
			s.mu.Unlock()
			
			msg := FileChange{
				FileName: relPath,
				Op:       "create",
				Content:  base64.StdEncoding.EncodeToString(data),
				IsDir:    false,
				Origin:   "server",
			}
			s.broadcast(msg)
			addLog(fmt.Sprintf("📤 Nouveau: %s", relPath))
			time.Sleep(100 * time.Millisecond)
		}
	}

	if event.Op&fsnotify.Write != 0 && !isDir {
		data, err := readFileWithRetry(event.Name)
		if err != nil {
			return
		}
		
		s.mu.Lock()
		s.knownFiles[relPath] = time.Now()
		s.mu.Unlock()
		
		msg := FileChange{
			FileName: relPath,
			Op:       "write",
			Content:  base64.StdEncoding.EncodeToString(data),
			IsDir:    false,
			Origin:   "server",
		}
		s.broadcast(msg)
		addLog(fmt.Sprintf("📤 Modifié: %s", relPath))
		time.Sleep(100 * time.Millisecond)
	}

	if event.Op&fsnotify.Remove != 0 {
		s.mu.Lock()
		wasDir := s.knownDirs[relPath] != time.Time{}
		
		if wasDir {
			delete(s.knownDirs, relPath)
		} else {
			delete(s.knownFiles, relPath)
		}
		s.mu.Unlock()
		
		msg := FileChange{
			FileName: relPath,
			Op:       "remove",
			IsDir:    wasDir,
			Origin:   "server",
		}
		s.broadcast(msg)
		
		if wasDir {
			addLog(fmt.Sprintf("📤 Dossier supprimé: %s", relPath))
		} else {
			addLog(fmt.Sprintf("📤 Supprimé: %s", relPath))
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func (s *Server) cleanPendingMoves() {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	
	for range ticker.C {
		if s.shouldExit {
			return
		}
		
		s.mu.Lock()
		now := time.Now()
		for path, until := range s.pendingMoves {
			if now.After(until) {
				delete(s.pendingMoves, path)
			}
		}
		s.mu.Unlock()
	}
}

func (s *Server) applyChange(msg FileChange) {
	normalizedPath := filepath.FromSlash(msg.FileName)
	path := filepath.Join(s.WatchDir, normalizedPath)

	s.mu.Lock()
	s.skipNext[msg.FileName] = time.Now().Add(3 * time.Second)
	s.mu.Unlock()

	switch msg.Op {
	case "mkdir":
		os.MkdirAll(path, 0755)
		s.mu.Lock()
		s.knownDirs[msg.FileName] = time.Now()
		s.mu.Unlock()
		
	case "create", "write":
		dir := filepath.Dir(path)
		os.MkdirAll(dir, 0755)
		
		data, err := base64.StdEncoding.DecodeString(msg.Content)
		if err != nil {
			return
		}
		os.WriteFile(path, data, 0644)
		s.mu.Lock()
		s.knownFiles[msg.FileName] = time.Now()
		s.mu.Unlock()
		
	case "remove":
		if msg.IsDir {
			os.RemoveAll(path)
			s.mu.Lock()
			delete(s.knownDirs, msg.FileName)
			s.mu.Unlock()
		} else {
			os.Remove(path)
			s.mu.Lock()
			delete(s.knownFiles, msg.FileName)
			s.mu.Unlock()
		}
	}
}

func (s *Server) broadcast(msg FileChange) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for client := range s.Clients {
		client.WriteJSON(msg)
	}
}

func (s *Server) broadcastExcept(msg FileChange, skip *websocket.Conn) {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	msg.Origin = "server"
	
	for client := range s.Clients {
		if client != skip {
			client.WriteJSON(msg)
		}
	}
}

func (s *Server) updateKnownFilesAndDirs() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.scanDirRecursive(s.WatchDir, "")
}

func (s *Server) scanDirRecursive(basePath, relPath string) {
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
				s.knownDirs[itemRelPath] = info.ModTime()
			}
			s.scanDirRecursive(basePath, filepath.Join(relPath, entry.Name()))
		} else {
			if info != nil {
				s.knownFiles[itemRelPath] = info.ModTime()
			}
		}
	}
}

func (s *Server) periodicCheck() {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	
	for range ticker.C {
		if s.shouldExit {
			return
		}

		currentFiles := make(map[string]time.Time)
		currentDirs := make(map[string]time.Time)
		s.scanCurrentState(s.WatchDir, "", currentFiles, currentDirs)

		s.mu.Lock()
		
		for oldDir := range s.knownDirs {
			if until, exists := s.skipNext[oldDir]; exists && time.Now().Before(until) {
				continue
			}
			if _, exists := currentDirs[oldDir]; !exists {
				msg := FileChange{
					FileName: oldDir,
					Op:       "remove",
					IsDir:    true,
					Origin:   "server",
				}
				delete(s.knownDirs, oldDir)
				for c := range s.Clients {
					c.WriteJSON(msg)
				}
				addLog(fmt.Sprintf("🗑️ Dossier supprimé: %s", oldDir))
				time.Sleep(100 * time.Millisecond)
			}
		}
		
		for oldFile := range s.knownFiles {
			if until, exists := s.skipNext[oldFile]; exists && time.Now().Before(until) {
				continue
			}
			if _, exists := currentFiles[oldFile]; !exists {
				msg := FileChange{
					FileName: oldFile,
					Op:       "remove",
					IsDir:    false,
					Origin:   "server",
				}
				delete(s.knownFiles, oldFile)
				for c := range s.Clients {
					c.WriteJSON(msg)
				}
				addLog(fmt.Sprintf("🗑️ Supprimé: %s", oldFile))
				time.Sleep(100 * time.Millisecond)
			}
		}

		for newDir, modTime := range currentDirs {
			if _, exists := s.knownDirs[newDir]; !exists {
				if until, exists := s.skipNext[newDir]; exists && time.Now().Before(until) {
					continue
				}
				msg := FileChange{
					FileName: newDir,
					Op:       "mkdir",
					IsDir:    true,
					Origin:   "server",
				}
				s.knownDirs[newDir] = modTime
				for c := range s.Clients {
					c.WriteJSON(msg)
				}
				addLog(fmt.Sprintf("📤 Dossier créé: %s", newDir))
				time.Sleep(100 * time.Millisecond)
			}
		}

		for name, modTime := range currentFiles {
			if t, exists := s.knownFiles[name]; !exists || modTime.After(t) {
				if until, exists := s.skipNext[name]; exists && time.Now().Before(until) {
					continue
				}

				data, err := readFileWithRetry(filepath.Join(s.WatchDir, name))
				if err == nil {
					msg := FileChange{
						FileName: name,
						Op:       "write",
						Content:  base64.StdEncoding.EncodeToString(data),
						IsDir:    false,
						Origin:   "server",
					}
					for c := range s.Clients {
						c.WriteJSON(msg)
					}
					s.knownFiles[name] = modTime
					addLog(fmt.Sprintf("📤 Modifié: %s", name))
					time.Sleep(100 * time.Millisecond)
				}
			}
		}
		s.mu.Unlock()
	}
}

func (s *Server) scanCurrentState(basePath, relPath string, files map[string]time.Time, dirs map[string]time.Time) {
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
			s.scanCurrentState(basePath, filepath.Join(relPath, entry.Name()), files, dirs)
		} else {
			if info != nil {
				files[itemRelPath] = info.ModTime()
			}
		}
	}
}