package main

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/gorilla/websocket"
)

type Client struct {
	ws          *websocket.Conn
	localDir    string
	mu          sync.Mutex
	skipNext    map[string]time.Time
	knownFiles  map[string]time.Time
	knownDirs   map[string]time.Time
	lastState   map[string]time.Time
	lastDirs    map[string]time.Time
	scanRunning bool
	shouldExit  bool
	receivedChanges map[string]int64 // NOUVEAU: Track des changements reçus
}

func StartClient(addr string) {
	reader := bufio.NewReader(os.Stdin)
	
	var serverAddr, hostID string
	var noConfig bool
	var shouldSaveConfig bool

	if ConfigExists() {
		serverAddr, hostID, noConfig = ShowConfigMenu()
		
		if noConfig {
			fmt.Print("\n📡 Adresse serveur (IP:PORT): ")
			addrInput, _ := reader.ReadString('\n')
			serverAddr = strings.TrimSpace(addrInput)
			
			fmt.Print("🔑 Entrez l'ID du host: ")
			id, _ := reader.ReadString('\n')
			hostID = strings.TrimSpace(id)
		}
	} else {
		fmt.Println("\n╔═══════════════════════════════════════╗")
		fmt.Println("║     PREMIÈRE CONNEXION - SPIRALY       ║")
		fmt.Println("╚═══════════════════════════════════════╝")
		fmt.Print("\n💾 Voulez-vous sauvegarder cette configuration? (y/n): ")
		
		var saveChoice string
		fmt.Scanln(&saveChoice)
		shouldSaveConfig = (saveChoice == "y" || saveChoice == "Y")
		
		fmt.Print("\n📡 Adresse serveur (IP:PORT): ")
		addrInput, _ := reader.ReadString('\n')
		serverAddr = strings.TrimSpace(addrInput)
		
		fmt.Print("🔑 Entrez l'ID du host: ")
		id, _ := reader.ReadString('\n')
		hostID = strings.TrimSpace(id)
	}
	
	if serverAddr == "" || hostID == "" {
		fmt.Println("❌ Adresse serveur ou ID du host manquant!")
		return
	}

	for {
		fmt.Println("\n🔌 Tentative de connexion à ", serverAddr, "...")
		ws, _, err := websocket.DefaultDialer.Dial("ws://"+serverAddr+"/ws", nil)
		if err != nil {
			fmt.Println("❌ Connexion impossible:", err)
			fmt.Print("\n💡 Tapez 'r' pour réessayer ou 'b' pour retourner au menu: ")
			choice, _ := reader.ReadString('\n')
			choice = strings.TrimSpace(strings.ToLower(choice))
			
			if choice == "b" {
				return
			}
			continue
		}
		
		if shouldSaveConfig && !ConfigExists() {
			if err := SaveConfig(serverAddr, hostID); err != nil {
				fmt.Println("⚠️  Erreur de sauvegarde de la configuration:", err)
			} else {
				fmt.Println("✅ Configuration sauvegardée!")
				shouldSaveConfig = false
			}
		}

		authReq := AuthRequest{
			Type:   "auth_request",
			HostID: hostID,
		}
		
		if err := ws.WriteJSON(authReq); err != nil {
			fmt.Println("❌ Erreur d'envoi:", err)
			ws.Close()
			continue
		}

		var authResp AuthResponse
		if err := ws.ReadJSON(&authResp); err != nil {
			fmt.Println("❌ Erreur de réception:", err)
			ws.Close()
			continue
		}

		if authResp.Type == "auth_failed" {
			fmt.Println("\n⛔", authResp.Message)
			ws.Close()
			
			fmt.Print("💡 Tapez 'r' pour réessayer l'ID ou 'b' pour retourner au menu: ")
			choice, _ := reader.ReadString('\n')
			choice = strings.TrimSpace(strings.ToLower(choice))
			
			if choice == "b" {
				return
			}
			continue
		}

		fmt.Println("\n✅", authResp.Message)
		fmt.Println("🟢 Connecté au host", hostID)
		fmt.Println("📡 Serveur:", serverAddr)
		fmt.Println("\n💡 Tapez 'x' puis Entrée pour vous déconnecter")

		localDir := "./Spiralydata"
		os.MkdirAll(localDir, 0755)

		client := &Client{
			ws:         ws,
			localDir:   localDir,
			skipNext:   make(map[string]time.Time),
			knownFiles: make(map[string]time.Time),
			knownDirs:  make(map[string]time.Time),
			lastState:  make(map[string]time.Time),
			lastDirs:   make(map[string]time.Time),
			shouldExit: false,
			receivedChanges: make(map[string]int64),
		}

		// MODIFIÉ: Ne scanner qu'après avoir reçu le changelog
		// client.scanInitial() - déplacé après réception des messages initiaux

		go func() {
			for {
				input, _ := reader.ReadString('\n')
				if strings.TrimSpace(strings.ToLower(input)) == "x" {
					fmt.Println("🛑 Déconnexion...")
					client.shouldExit = true
					ws.Close()
					return
				}
			}
		}()

		// NOUVEAU: D'abord recevoir tous les changements du serveur
		initialSyncDone := false
		go func() {
			time.Sleep(2 * time.Second) // Attendre que le changelog soit appliqué
			if !initialSyncDone {
				client.scanInitial()
				client.syncLocalInitToHost()
				go client.watchRecursive()
				go client.periodicScanner()
				initialSyncDone = true
			}
		}()

		for {
			var rawMsg json.RawMessage
			if err := ws.ReadJSON(&rawMsg); err != nil {
				if !client.shouldExit {
					fmt.Println("\n❌ Connexion perdue")
				}
				ws.Close()
				break
			}

			var msg FileChange
			if err := json.Unmarshal(rawMsg, &msg); err == nil {
				if msg.Origin != "client" {
					client.applyChange(msg)
					
					// Lancer la sync après les premiers messages
					if !initialSyncDone && time.Now().Unix()-msg.Timestamp > 5 {
						client.scanInitial()
						client.syncLocalInitToHost()
						go client.watchRecursive()
						go client.periodicScanner()
						initialSyncDone = true
					}
				}
			}
		}

		if client.shouldExit {
			return
		}

		fmt.Print("\n💡 Tapez 'r' pour se reconnecter ou 'q' pour quitter: ")
		choice, _ := reader.ReadString('\n')
		choice = strings.TrimSpace(strings.ToLower(choice))
		
		if choice != "r" {
			return
		}
	}
}

func (c *Client) scanInitial() {
	c.scanDirRecursive(c.localDir, "")
}

func (c *Client) scanDirRecursive(basePath, relPath string) {
	fullPath := filepath.Join(basePath, relPath)
	entries, _ := ioutil.ReadDir(fullPath)
	
	for _, entry := range entries {
		itemRelPath := filepath.ToSlash(filepath.Join(relPath, entry.Name()))
		
		if entry.IsDir() {
			c.lastDirs[itemRelPath] = entry.ModTime()
			c.knownDirs[itemRelPath] = entry.ModTime()
			c.scanDirRecursive(basePath, filepath.Join(relPath, entry.Name()))
		} else {
			c.lastState[itemRelPath] = entry.ModTime()
			c.knownFiles[itemRelPath] = entry.ModTime()
		}
	}
}

func (c *Client) syncLocalInitToHost() {
	// MODIFIÉ: Ne plus envoyer les fichiers qui ont déjà été traités par le changelog
	c.sendDirRecursiveSelective(c.localDir, "")
}

func (c *Client) sendDirRecursiveSelective(basePath, relPath string) {
	fullPath := filepath.Join(basePath, relPath)
	entries, _ := ioutil.ReadDir(fullPath)
	
	for _, entry := range entries {
		itemRelPath := filepath.ToSlash(filepath.Join(relPath, entry.Name()))
		
		// NOUVEAU: Ne pas renvoyer si déjà dans le changelog reçu
		if _, received := c.receivedChanges[itemRelPath]; received {
			continue
		}
		
		if entry.IsDir() {
			c.ws.WriteJSON(FileChange{
				FileName: itemRelPath,
				Op:       "mkdir",
				IsDir:    true,
				Origin:   "client",
				Timestamp: time.Now().Unix(),
			})
			c.sendDirRecursiveSelective(basePath, filepath.Join(relPath, entry.Name()))
		} else {
			c.sendFileNow(itemRelPath)
		}
	}
}

func (c *Client) sendDirRecursive(basePath, relPath string) {
	fullPath := filepath.Join(basePath, relPath)
	entries, _ := ioutil.ReadDir(fullPath)
	
	for _, entry := range entries {
		itemRelPath := filepath.ToSlash(filepath.Join(relPath, entry.Name()))
		
		if entry.IsDir() {
			c.ws.WriteJSON(FileChange{
				FileName: itemRelPath,
				Op:       "mkdir",
				IsDir:    true,
				Origin:   "client",
				Timestamp: time.Now().Unix(),
			})
			c.sendDirRecursive(basePath, filepath.Join(relPath, entry.Name()))
		} else {
			c.sendFileNow(itemRelPath)
		}
	}
}

func (c *Client) watchRecursive() {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		fmt.Println("❌ Erreur watcher:", err)
		return
	}
	defer watcher.Close()

	c.addDirToWatcher(watcher, c.localDir)

	for {
		select {
		case event := <-watcher.Events:
			if event.Op&fsnotify.Create != 0 {
				if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
					watcher.Add(event.Name)
				}
			}
		case err := <-watcher.Errors:
			if !c.shouldExit {
				fmt.Println("⚠️ Watcher error:", err)
			}
		}
	}
}

func (c *Client) addDirToWatcher(watcher *fsnotify.Watcher, dir string) {
	watcher.Add(dir)
	
	entries, err := ioutil.ReadDir(dir)
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
	if c.scanRunning {
		return
	}
	c.scanRunning = true

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		if c.shouldExit {
			return
		}

		currentFiles := make(map[string]time.Time)
		currentDirs := make(map[string]time.Time)
		c.scanCurrentState(c.localDir, "", currentFiles, currentDirs)

		c.mu.Lock()
		
		// Détecter suppressions de dossiers
		for oldDir := range c.lastDirs {
			if until, exists := c.skipNext[oldDir]; exists && time.Now().Before(until) {
				continue
			}
			if _, exists := currentDirs[oldDir]; !exists {
				fmt.Println("📤 remove dir", oldDir)
				c.ws.WriteJSON(FileChange{
					FileName: oldDir,
					Op:       "remove",
					IsDir:    true,
					Origin:   "client",
					Timestamp: time.Now().Unix(),
				})
				delete(c.knownDirs, oldDir)
			}
		}

		// Détecter nouveaux dossiers
		for newDir, modTime := range currentDirs {
			if _, known := c.lastDirs[newDir]; !known {
				if until, exists := c.skipNext[newDir]; exists && time.Now().Before(until) {
					continue
				}
				fmt.Println("📤 mkdir", newDir)
				c.ws.WriteJSON(FileChange{
					FileName: newDir,
					Op:       "mkdir",
					IsDir:    true,
					Origin:   "client",
					Timestamp: time.Now().Unix(),
				})
				c.knownDirs[newDir] = modTime
			}
		}

		// Détecter additions & modifications de fichiers
		for name, modTime := range currentFiles {
			if until, exists := c.skipNext[name]; exists && time.Now().Before(until) {
				continue
			}

			lastMod, known := c.lastState[name]
			if !known {
				c.sendFileNow(name)
			} else if modTime.After(lastMod) {
				c.sendFileNow(name)
			}
		}

		// Détecter suppressions de fichiers
		for oldFile := range c.lastState {
			if _, still := currentFiles[oldFile]; !still {
				if until, exists := c.skipNext[oldFile]; exists && time.Now().Before(until) {
					delete(c.skipNext, oldFile)
					continue
				}
				
				fmt.Println("📤 remove file", oldFile)
				c.ws.WriteJSON(FileChange{
					FileName: oldFile,
					Op:       "remove",
					IsDir:    false,
					Origin:   "client",
					Timestamp: time.Now().Unix(),
				})
				delete(c.knownFiles, oldFile)
			}
		}

		c.lastState = currentFiles
		c.lastDirs = currentDirs
		c.mu.Unlock()
	}
}

func (c *Client) scanCurrentState(basePath, relPath string, files map[string]time.Time, dirs map[string]time.Time) {
	fullPath := filepath.Join(basePath, relPath)
	entries, err := ioutil.ReadDir(fullPath)
	if err != nil {
		return
	}

	for _, entry := range entries {
		itemRelPath := filepath.ToSlash(filepath.Join(relPath, entry.Name()))
		
		if entry.IsDir() {
			dirs[itemRelPath] = entry.ModTime()
			c.scanCurrentState(basePath, filepath.Join(relPath, entry.Name()), files, dirs)
		} else {
			files[itemRelPath] = entry.ModTime()
		}
	}
}

func (c *Client) sendFileNow(relPath string) {
	fullPath := filepath.Join(c.localDir, filepath.FromSlash(relPath))
	data, err := ioutil.ReadFile(fullPath)
	if err != nil {
		return
	}
	fmt.Println("📤 send", relPath)
	c.ws.WriteJSON(FileChange{
		FileName: relPath,
		Op:       "write",
		Content:  base64.StdEncoding.EncodeToString(data),
		IsDir:    false,
		Origin:   "client",
		Timestamp: time.Now().Unix(),
	})
	c.knownFiles[relPath] = time.Now()
}

func (c *Client) applyChange(msg FileChange) {
	normalizedPath := filepath.FromSlash(msg.FileName)
	target := filepath.Join(c.localDir, normalizedPath)

	c.mu.Lock()
	c.skipNext[msg.FileName] = time.Now().Add(3 * time.Second)
	c.receivedChanges[msg.FileName] = msg.Timestamp // NOUVEAU: Tracker
	c.mu.Unlock()

	switch msg.Op {
	case "mkdir":
		os.MkdirAll(target, 0755)
		c.mu.Lock()
		c.knownDirs[msg.FileName] = time.Now()
		c.lastDirs[msg.FileName] = time.Now()
		c.mu.Unlock()
		fmt.Println("📥 mkdir", msg.FileName)
		
	case "remove":
		if msg.IsDir {
			// NOUVEAU: Suppression robuste
			forceRemoveAll(target)
			c.mu.Lock()
			delete(c.knownDirs, msg.FileName)
			delete(c.lastDirs, msg.FileName)
			c.mu.Unlock()
			fmt.Println("📥 remove dir", msg.FileName)
		} else {
			os.Remove(target)
			c.mu.Lock()
			delete(c.knownFiles, msg.FileName)
			delete(c.lastState, msg.FileName)
			c.mu.Unlock()
			fmt.Println("📥 remove file", msg.FileName)
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
		fmt.Println("📥 receive", msg.FileName)
	}
}