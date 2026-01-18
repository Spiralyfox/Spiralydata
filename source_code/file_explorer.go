package main

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
)

type FileTreeItem struct {
	Path     string
	Name     string
	IsDir    bool
	Children []*FileTreeItem
	Parent   *FileTreeItem
}

type FileExplorer struct {
	client           *Client
	win              fyne.Window
	allItems         map[string]*FileTreeItem
	currentDir       *FileTreeItem
	rootDir          *FileTreeItem
	itemWidgets      map[string]*widget.Check
	mu               sync.Mutex
	loadingLabel     *widget.Label
	contentContainer *fyne.Container
	backCallback     func()
	treeLoaded       bool
}

func NewFileExplorer(client *Client, win fyne.Window, backCallback func()) *FileExplorer {
	return &FileExplorer{
		client:       client,
		win:          win,
		allItems:     make(map[string]*FileTreeItem),
		itemWidgets:  make(map[string]*widget.Check),
		backCallback: backCallback,
		treeLoaded:   false,
	}
}

func (fe *FileExplorer) Show() {
	fe.loadingLabel = widget.NewLabel("📄 Chargement de la structure des fichiers...")
	fe.loadingLabel.Alignment = fyne.TextAlignCenter
	
	loadingChars := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	loadingIndex := 0
	stopLoading := false
	
	progressLabel := widget.NewLabel("Fichiers scannés : 0")
	progressLabel.Alignment = fyne.TextAlignCenter
	
	cancelBtn := widget.NewButton("Annuler", func() {
		stopLoading = true
		fe.client.explorerActive = false
		fe.backCallback()
	})
	
	loadingContent := container.NewVBox(
		layout.NewSpacer(),
		fe.loadingLabel,
		progressLabel,
		layout.NewSpacer(),
		container.NewCenter(cancelBtn),
		layout.NewSpacer(),
	)
	
	fe.contentContainer = container.NewVBox(loadingContent)
	
	split := container.NewHSplit(
		fe.contentContainer,
		container.NewBorder(
			widget.NewLabelWithStyle("📋 Logs", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
			nil, nil, nil,
			container.NewVScroll(logWidget),
		),
	)
	split.Offset = 0.5
	
	fe.win.SetContent(split)
	
	go func() {
		for !stopLoading {
			time.Sleep(100 * time.Millisecond)
			if !stopLoading {
				char := loadingChars[loadingIndex%len(loadingChars)]
				fe.loadingLabel.SetText(fmt.Sprintf("%s Chargement de la structure des fichiers...", char))
				fe.loadingLabel.Refresh()
				loadingIndex++
			}
		}
	}()
	
	go fe.requestAndBuildTree(&stopLoading, progressLabel)
}

func (fe *FileExplorer) requestAndBuildTree(stopLoading *bool, progressLabel *widget.Label) {
	addLog("📂 Demande de la structure des fichiers au serveur...")
	
	// Activer le mode explorateur sur le client
	fe.client.explorerActive = true
	fe.client.treeItemsChan = make(chan FileTreeItemMessage, 100)
	
	reqMsg := map[string]string{
		"type":   "request_file_tree",
		"origin": "client",
	}
	
	fe.client.mu.Lock()
	err := fe.client.ws.WriteJSON(reqMsg)
	fe.client.mu.Unlock()
	
	if err != nil {
		addLog("❌ Erreur lors de la demande")
		*stopLoading = true
		fe.client.explorerActive = false
		fe.backCallback()
		return
	}
	
	filesReceived := 0
	timeout := time.After(60 * time.Second)
	complete := false
	
	for !complete && !*stopLoading {
		select {
		case <-timeout:
			addLog("⏱️ Timeout lors de la réception")
			*stopLoading = true
			fe.client.explorerActive = false
			fe.backCallback()
			return
			
		case treeItem := <-fe.client.treeItemsChan:
			if treeItem.Type == "file_tree_item" {
				item := &FileTreeItem{
					Path:     treeItem.Path,
					Name:     treeItem.Name,
					IsDir:    treeItem.IsDir,
					Children: []*FileTreeItem{},
				}
				fe.allItems[item.Path] = item
				filesReceived++
				
				if filesReceived%10 == 0 || filesReceived < 10 {
					progressLabel.SetText(fmt.Sprintf("Fichiers scannés : %d", filesReceived))
					progressLabel.Refresh()
				}
			} else if treeItem.Type == "file_tree_complete" {
				*stopLoading = true
				complete = true
				fe.client.explorerActive = false
				addLog(fmt.Sprintf("✅ %d fichiers/dossiers reçus", filesReceived))
				fe.buildTreeStructure()
				fe.showDirectoryUI()
				return
			}
		}
	}
}

func (fe *FileExplorer) buildTreeStructure() {
	addLog("🔨 Construction de l'arborescence...")
	
	// Créer la racine virtuelle
	fe.rootDir = &FileTreeItem{
		Path:     "",
		Name:     "Spiralydata",
		IsDir:    true,
		Children: []*FileTreeItem{},
	}
	fe.allItems[""] = fe.rootDir
	
	// Construire les relations parent-enfant
	for path, item := range fe.allItems {
		if path == "" {
			continue
		}
		
		parentPath := filepath.Dir(path)
		parentPath = filepath.ToSlash(parentPath)
		
		if parentPath == "." {
			parentPath = ""
		}
		
		if parent, exists := fe.allItems[parentPath]; exists {
			item.Parent = parent
			parent.Children = append(parent.Children, item)
		}
	}
	
	// Trier les enfants
	for _, item := range fe.allItems {
		if item.IsDir && len(item.Children) > 0 {
			sort.Slice(item.Children, func(i, j int) bool {
				if item.Children[i].IsDir != item.Children[j].IsDir {
					return item.Children[i].IsDir
				}
				return item.Children[i].Name < item.Children[j].Name
			})
		}
	}
	
	fe.currentDir = fe.rootDir
	fe.treeLoaded = true
}

func (fe *FileExplorer) showDirectoryUI() {
	fe.itemWidgets = make(map[string]*widget.Check)
	
	currentPath := fe.currentDir.Path
	if currentPath == "" {
		currentPath = "/"
	}
	
	title := widget.NewLabelWithStyle(fmt.Sprintf("📁 %s", currentPath), fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	
	selectAllBtn := widget.NewButton("Tout sélectionner", func() {
		fe.selectAll(true)
	})
	
	deselectAllBtn := widget.NewButton("Tout désélectionner", func() {
		fe.selectAll(false)
	})
	
	parentBtn := widget.NewButton("⬆️ Dossier parent", func() {
		if fe.currentDir.Parent != nil {
			fe.currentDir = fe.currentDir.Parent
			fe.showDirectoryUI()
		}
	})
	
	if fe.currentDir.Parent == nil {
		parentBtn.Disable()
	}
	
	selectionBar := container.NewHBox(
		parentBtn,
		layout.NewSpacer(),
		selectAllBtn,
		deselectAllBtn,
	)
	
	treeContent := container.NewVBox()
	
	// Afficher uniquement le contenu du dossier actuel
	for _, item := range fe.currentDir.Children {
		fe.addItemToList(treeContent, item)
	}
	
	if len(fe.currentDir.Children) == 0 {
		emptyLabel := widget.NewLabel("Ce dossier est vide")
		emptyLabel.Alignment = fyne.TextAlignCenter
		treeContent.Add(emptyLabel)
	}
	
	treeScroll := container.NewVScroll(treeContent)
	treeScroll.SetMinSize(fyne.NewSize(500, 400))
	
	downloadBtn := widget.NewButton("⬇️ Télécharger la sélection", func() {
		fe.showDownloadOptions()
	})
	downloadBtn.Importance = widget.HighImportance
	
	backBtn := widget.NewButton("Retour", func() {
		fe.backCallback()
	})
	
	mainContent := container.NewBorder(
		container.NewVBox(
			title,
			widget.NewSeparator(),
			selectionBar,
			widget.NewSeparator(),
		),
		container.NewVBox(
			widget.NewSeparator(),
			container.NewHBox(
				downloadBtn,
				layout.NewSpacer(),
				backBtn,
			),
		),
		nil,
		nil,
		treeScroll,
	)
	
	split := container.NewHSplit(
		mainContent,
		container.NewBorder(
			widget.NewLabelWithStyle("📋 Logs", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
			nil, nil, nil,
			container.NewVScroll(logWidget),
		),
	)
	split.Offset = 0.5
	
	fe.contentContainer.Objects = []fyne.CanvasObject{mainContent}
	fe.contentContainer.Refresh()
	
	fe.win.SetContent(split)
	
	addLog(fmt.Sprintf("✅ Explorateur: %s", currentPath))
}

func (fe *FileExplorer) addItemToList(parent *fyne.Container, item *FileTreeItem) {
	icon := "📄"
	if item.IsDir {
		icon = "📁"
	}
	
	displayName := icon + " " + item.Name
	
	itemContainer := container.NewHBox()
	
	if item.IsDir {
		openBtn := widget.NewButton("Ouvrir", func() {
			fe.currentDir = item
			fe.showDirectoryUI()
		})
		openBtn.Importance = widget.LowImportance
		
		check := widget.NewCheck(displayName, nil)
		fe.itemWidgets[item.Path] = check
		
		itemContainer.Add(check)
		itemContainer.Add(layout.NewSpacer())
		itemContainer.Add(openBtn)
	} else {
		check := widget.NewCheck(displayName, nil)
		fe.itemWidgets[item.Path] = check
		itemContainer.Add(check)
	}
	
	parent.Add(itemContainer)
}

func (fe *FileExplorer) selectAll(selected bool) {
	fe.mu.Lock()
	defer fe.mu.Unlock()
	
	for _, check := range fe.itemWidgets {
		check.SetChecked(selected)
		check.Refresh()
	}
}

func (fe *FileExplorer) showDownloadOptions() {
	selected := fe.getSelectedItems()
	
	if len(selected) == 0 {
		addLog("⚠️ Aucun fichier sélectionné")
		dialog.ShowInformation("Aucune sélection", "Veuillez sélectionner au moins un fichier ou dossier", fe.win)
		return
	}
	
	addLog(fmt.Sprintf("📦 %d éléments sélectionnés", len(selected)))
	
	content := container.NewVBox(
		widget.NewLabel(fmt.Sprintf("Télécharger %d élément(s) sélectionné(s)", len(selected))),
		widget.NewSeparator(),
		widget.NewLabel("Choisissez la destination :"),
	)
	
	dlg := dialog.NewCustom("Destination du téléchargement", "Annuler", content, fe.win)
	
	syncDirBtn := widget.NewButton("📂 Dans le dossier de synchronisation", func() {
		dlg.Hide()
		fe.downloadToSyncDir(selected)
	})
	syncDirBtn.Importance = widget.HighImportance
	
	customDirBtn := widget.NewButton("📁 Autre emplacement...", func() {
		dlg.Hide()
		fe.downloadToCustomDir(selected)
	})
	
	content.Add(syncDirBtn)
	content.Add(customDirBtn)
	
	dlg.Show()
}

func (fe *FileExplorer) getSelectedItems() []string {
	fe.mu.Lock()
	defer fe.mu.Unlock()
	
	var selected []string
	
	for path, check := range fe.itemWidgets {
		if check.Checked {
			selected = append(selected, path)
		}
	}
	
	return selected
}

func (fe *FileExplorer) downloadToSyncDir(items []string) {
	addLog(fmt.Sprintf("⬇️ Téléchargement de %d éléments vers le dossier de sync...", len(items)))
	go fe.performDownload(items, fe.client.localDir)
	fe.backCallback()
}

func (fe *FileExplorer) downloadToCustomDir(items []string) {
	dialog.ShowFolderOpen(func(uri fyne.ListableURI, err error) {
		if err != nil || uri == nil {
			return
		}
		
		targetDir := uri.Path()
		addLog(fmt.Sprintf("⬇️ Téléchargement de %d éléments vers %s...", len(items), targetDir))
		go fe.performDownload(items, targetDir)
		fe.backCallback()
	}, fe.win)
}

func (fe *FileExplorer) performDownload(items []string, targetDir string) {
	addLog("📥 Début du téléchargement...")
	
	expandedItems := fe.expandDirectories(items)
	
	addLog(fmt.Sprintf("📦 %d fichiers/dossiers à télécharger", len(expandedItems)))
	
	// Activer le mode téléchargement
	fe.client.downloadActive = true
	fe.client.downloadChan = make(chan FileChange, 100)
	
	reqMsg := DownloadRequest{
		Type:  "download_request",
		Items: expandedItems,
	}
	
	fe.client.mu.Lock()
	err := fe.client.ws.WriteJSON(reqMsg)
	fe.client.mu.Unlock()
	
	if err != nil {
		addLog(fmt.Sprintf("❌ Erreur envoi requête: %v", err))
		fe.client.downloadActive = false
		return
	}
	
	downloaded := 0
	timeout := time.After(120 * time.Second)
	lastUpdate := time.Now()
	
	for downloaded < len(expandedItems) {
		select {
		case <-timeout:
			addLog(fmt.Sprintf("⏱️ Timeout - %d/%d fichiers téléchargés", downloaded, len(expandedItems)))
			fe.client.downloadActive = false
			return
			
		case msg := <-fe.client.downloadChan:
			if msg.Origin == "server" {
				fe.saveDownloadedFile(msg, targetDir)
				downloaded++
				
				if time.Since(lastUpdate) > 1*time.Second || downloaded == len(expandedItems) {
					addLog(fmt.Sprintf("📥 Téléchargés: %d/%d", downloaded, len(expandedItems)))
					lastUpdate = time.Now()
				}
			}
		}
	}
	
	fe.client.downloadActive = false
	addLog(fmt.Sprintf("✅ Téléchargement terminé: %d fichiers", downloaded))
}

func (fe *FileExplorer) expandDirectories(items []string) []string {
	expanded := make(map[string]bool)
	
	for _, path := range items {
		fe.expandDirectoryRecursive(path, expanded)
	}
	
	var result []string
	for path := range expanded {
		result = append(result, path)
	}
	
	sort.Strings(result)
	
	return result
}

func (fe *FileExplorer) expandDirectoryRecursive(path string, expanded map[string]bool) {
	expanded[path] = true
	
	if item, exists := fe.allItems[path]; exists && item.IsDir {
		for _, child := range item.Children {
			fe.expandDirectoryRecursive(child.Path, expanded)
		}
	}
}

func (fe *FileExplorer) saveDownloadedFile(msg FileChange, targetDir string) {
	normalizedPath := filepath.FromSlash(msg.FileName)
	targetPath := filepath.Join(targetDir, normalizedPath)
	
	switch msg.Op {
	case "mkdir":
		if err := os.MkdirAll(targetPath, 0755); err != nil {
			addLog(fmt.Sprintf("❌ Erreur création dossier %s: %v", msg.FileName, err))
		}
		
	case "create", "write":
		dir := filepath.Dir(targetPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			addLog(fmt.Sprintf("❌ Erreur création répertoire: %v", err))
			return
		}
		
		data, err := base64.StdEncoding.DecodeString(msg.Content)
		if err != nil {
			addLog(fmt.Sprintf("❌ Erreur décodage %s: %v", msg.FileName, err))
			return
		}
		
		if err := os.WriteFile(targetPath, data, 0644); err != nil {
			addLog(fmt.Sprintf("❌ Erreur écriture %s: %v", msg.FileName, err))
		}
	}
}