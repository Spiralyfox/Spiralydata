package main

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/gorilla/websocket"
)

type Server struct {
	HostID     string
	Clients    map[*websocket.Conn]string
	Upgrader   websocket.Upgrader
	WatchDir   string
	mu         sync.Mutex
	skipNext   map[string]time.Time
	knownFiles map[string]time.Time
	knownDirs  map[string]time.Time
	clientNum  int
	shouldExit bool
	httpServer *http.Server
	pendingMoves map[string]time.Time
}

func NewServer(customID string) *Server {
	hostID := customID
	if hostID == "" {
		// Si pas d'ID personnalisé, demander à l'utilisateur
		fmt.Print("🔑 Entrez un ID à 6 chiffres pour ce serveur: ")
		reader := bufio.NewReader(os.Stdin)
		input, _ := reader.ReadString('\n')
		hostID = strings.TrimSpace(input)
		
		// Validation basique
		if len(hostID) != 6 {
			fmt.Println("⚠️  L'ID doit contenir exactement 6 caractères")
			fmt.Print("🔑 Entrez un ID à 6 chiffres: ")
			input, _ = reader.ReadString('\n')
			hostID = strings.TrimSpace(input)
		}
	}
	
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
	s.WatchDir = "./Spiralydata"
	os.MkdirAll(s.WatchDir, 0755)

	fmt.Println("🟢 Serveur démarré")
	fmt.Println("🆔 ID:", s.HostID)
	fmt.Println("📁 Dossier:", s.WatchDir)
	fmt.Println("⏳ En attente de connexions...")
	fmt.Println("\n💡 Tapez 'x' puis Entrée pour arrêter le serveur")

	s.updateKnownFilesAndDirs()
	go s.watchRecursive()
	go s.periodicCheck()
	go s.cleanPendingMoves()

	go func() {
		reader := bufio.NewReader(os.Stdin)
		for {
			input, _ := reader.ReadString('\n')
			if strings.TrimSpace(strings.ToLower(input)) == "x" {
				fmt.Println("🛑 Arrêt du serveur...")
				s.shouldExit = true
				
				s.mu.Lock()
				for client := range s.Clients {
					client.Close()
				}
				s.mu.Unlock()
				
				os.Exit(0)
			}
		}
	}()

	http.HandleFunc("/ws", s.handleWS)
	s.httpServer = &http.Server{Addr: ":" + port}
	s.httpServer.ListenAndServe()
}

func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	ws, err := s.Upgrader.Upgrade(w, r, nil)
	if err != nil {
		fmt.Println("❌ Erreur websocket:", err)
		return
	}

	var rawMsg json.RawMessage
	if err := ws.ReadJSON(&rawMsg); err != nil {
		ws.Close()
		return
	}

	var authReq AuthRequest
	if err := json.Unmarshal(rawMsg, &authReq); err != nil {
		ws.Close()
		return
	}

	if authReq.Type == "auth_request" {
		if authReq.HostID == s.HostID {
			s.mu.Lock()
			s.clientNum++
			clientName := fmt.Sprintf("Client_%d", s.clientNum)
			s.Clients[ws] = clientName
			s.mu.Unlock()

			fmt.Printf("✅ %s connecté (ID vérifié)\n", clientName)
			fmt.Printf("👥 Clients connectés: %d\n", len(s.Clients))

			ws.WriteJSON(AuthResponse{
				Type:    "auth_success",
				Message: "Connexion établie - Synchronisation en cours...",
			})

			s.sendAllFilesAndDirs(ws)
			s.handleClientMessages(ws, clientName)

		} else {
			fmt.Printf("⛔ Tentative de connexion refusée (ID incorrect: %s)\n", authReq.HostID)
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
		fmt.Printf("❌ %s déconnecté\n", clientName)
		fmt.Printf("👥 Clients restants: %d\n", remaining)
	}()

	for {
		var msg FileChange
		if err := ws.ReadJSON(&msg); err != nil {
			break
		}
		
		if msg.Origin != "server" {
			if msg.IsDir {
				fmt.Printf("📥 %s: %s dossier %s\n", clientName, msg.Op, msg.FileName)
			} else {
				fmt.Printf("📥 %s: %s fichier %s\n", clientName, msg.Op, msg.FileName)
			}
			
			s.applyChange(msg)
			s.broadcastExcept(msg, ws)
		}
	}
}

func (s *Server) sendAllFilesAndDirs(ws *websocket.Conn) {
	s.sendDirRecursive(ws, s.WatchDir, "")
	fmt.Println("📤 Structure complète envoyée")
}

func (s *Server) sendDirRecursive(ws *websocket.Conn, basePath, relPath string) {
	fullPath := filepath.Join(basePath, relPath)
	entries, err := ioutil.ReadDir(fullPath)
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
		}
	}
}

func (s *Server) watchRecursive() {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		fmt.Println("❌ Erreur watcher:", err)
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
				fmt.Println("⚠️ Erreur watcher:", err)
			}
		}
	}
}

func (s *Server) addDirToWatcher(watcher *fsnotify.Watcher, dir string) {
	watcher.Add(dir)
	
	entries, err := ioutil.ReadDir(dir)
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

	// Normaliser avec des slashes pour tous les OS
	relPath = filepath.ToSlash(filepath.Clean(relPath))
	
	s.mu.Lock()
	
	// Vérifier si on doit ignorer cet événement
	if until, exists := s.skipNext[relPath]; exists && time.Now().Before(until) {
		s.mu.Unlock()
		return
	}
	
	// Vérifier si c'est un déplacement récent
	if until, exists := s.pendingMoves[relPath]; exists && time.Now().Before(until) {
		s.mu.Unlock()
		return
	}
	
	s.mu.Unlock()

	info, err := os.Stat(event.Name)
	isDir := err == nil && info.IsDir()

	// CREATE
	if event.Op&fsnotify.Create != 0 {
		// Marquer comme potentiel déplacement
		s.mu.Lock()
		s.pendingMoves[relPath] = time.Now().Add(300 * time.Millisecond)
		s.mu.Unlock()
		
		// Attendre pour détecter si c'est un move
		time.Sleep(150 * time.Millisecond)
		
		// Revérifier que le fichier existe toujours
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
			fmt.Println("📤 mkdir", relPath)
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
			fmt.Println("📤 create", relPath)
		}
	}

	// WRITE
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
		fmt.Println("📤 write", relPath)
	}

	// REMOVE
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
		fmt.Println("📤 remove", relPath)
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
	// Normaliser le chemin reçu en chemin système
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
	entries, err := ioutil.ReadDir(fullPath)
	if err != nil {
		return
	}

	for _, entry := range entries {
		itemRelPath := filepath.ToSlash(filepath.Join(relPath, entry.Name()))
		
		if entry.IsDir() {
			s.knownDirs[itemRelPath] = entry.ModTime()
			s.scanDirRecursive(basePath, filepath.Join(relPath, entry.Name()))
		} else {
			s.knownFiles[itemRelPath] = entry.ModTime()
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
		
		// Vérifier les suppressions de dossiers
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
				fmt.Println("🗑️ Suppression dossier:", oldDir)
			}
		}
		
		// Vérifier les suppressions de fichiers
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
				fmt.Println("🗑️ Suppression fichier:", oldFile)
			}
		}

		// Nouveaux dossiers
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
				fmt.Println("📤 Nouveau dossier:", newDir)
			}
		}

		// Nouveaux fichiers ou modifiés
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
					fmt.Println("📤 Fichier modifié:", name)
				}
			}
		}
		s.mu.Unlock()
	}
}

func (s *Server) scanCurrentState(basePath, relPath string, files map[string]time.Time, dirs map[string]time.Time) {
	fullPath := filepath.Join(basePath, relPath)
	entries, err := ioutil.ReadDir(fullPath)
	if err != nil {
		return
	}

	for _, entry := range entries {
		itemRelPath := filepath.ToSlash(filepath.Join(relPath, entry.Name()))
		
		if entry.IsDir() {
			dirs[itemRelPath] = entry.ModTime()
			s.scanCurrentState(basePath, filepath.Join(relPath, entry.Name()), files, dirs)
		} else {
			files[itemRelPath] = entry.ModTime()
		}
	}
}