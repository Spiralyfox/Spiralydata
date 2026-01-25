package main

import (
	"context"
	"fmt"
	"image/color"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
)

var (
	logWidget      *widget.Entry
	logScroll      *container.Scroll
	logs           []string
	maxLogs        = 100
	myApp          fyne.App
	myWindow       fyne.Window
	logMutex       sync.Mutex
	logTicker      *time.Ticker
	logNeedsUpdate bool
	logBuffer      []string
	logBufferMu    sync.Mutex
	statusBar      *StatusBar
	shortcutHandler *ShortcutHandler
)

// Constantes pour les dimensions de fenêtre
const (
	MinWindowWidth  float32 = 800
	MinWindowHeight float32 = 500
	DefaultWidth    float32 = 1100
	DefaultHeight   float32 = 650
)

func main() {
	StartGUI()
}

// addLog avec buffering pour éviter les freezes
func addLog(message string) {
	timestamp := time.Now().Format("15:04:05")
	logEntry := fmt.Sprintf("[%s] %s", timestamp, message)

	logBufferMu.Lock()
	logBuffer = append(logBuffer, logEntry)
	logBufferMu.Unlock()
}

// flushLogBuffer transfère le buffer vers les logs principaux
func flushLogBuffer() {
	logBufferMu.Lock()
	if len(logBuffer) == 0 {
		logBufferMu.Unlock()
		return
	}
	buffer := logBuffer
	logBuffer = nil
	logBufferMu.Unlock()

	logMutex.Lock()
	logs = append(logs, buffer...)
	// Limiter le nombre de logs
	if len(logs) > maxLogs {
		logs = logs[len(logs)-maxLogs:]
	}
	logNeedsUpdate = true
	logMutex.Unlock()
}

func startLogUpdater() {
	logTicker = time.NewTicker(150 * time.Millisecond)

	go func() {
		for range logTicker.C {
			// Flush le buffer d'abord
			flushLogBuffer()

			logMutex.Lock()
			needsUpdate := logNeedsUpdate
			logNeedsUpdate = false
			logMutex.Unlock()

			if needsUpdate && logWidget != nil {
				logMutex.Lock()
				text := strings.Join(logs, "\n")
				logMutex.Unlock()

				// Mise à jour UI dans le thread principal
				if myWindow != nil {
					myWindow.Canvas().Refresh(logWidget)
				}
				logWidget.SetText(text)
				if logScroll != nil {
					logScroll.ScrollToBottom()
				}
			}
		}
	}()
}

// getPublicIP avec timeout pour éviter les blocages
func getPublicIP() string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", "https://api.ipify.org?format=text", nil)
	if err != nil {
		return "Unknown"
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "Unknown"
	}
	defer resp.Body.Close()

	ip, err := io.ReadAll(resp.Body)
	if err != nil {
		return "Unknown"
	}

	return string(ip)
}

func getLocalIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "Unknown"
	}

	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				return ipnet.IP.String()
			}
		}
	}
	return "127.0.0.1"
}

// saveWindowConfig sauvegarde la configuration de la fenêtre
func saveWindowConfig() {
	if myWindow == nil {
		return
	}
	config, _ := LoadConfig()
	if config == nil {
		config = &AppConfig{}
	}
	size := myWindow.Canvas().Size()
	config.WindowWidth = size.Width
	config.WindowHeight = size.Height
	config.DarkTheme = (GetCurrentTheme() == ThemeDark)
	SaveConfig(config)
}

// loadWindowSize charge les dimensions sauvegardées
func loadWindowSize() (float32, float32) {
	config, err := LoadConfig()
	if err != nil || config.WindowWidth < MinWindowWidth || config.WindowHeight < MinWindowHeight {
		return DefaultWidth, DefaultHeight
	}
	return config.WindowWidth, config.WindowHeight
}

// loadTheme charge le thème sauvegardé
func loadTheme() ThemeType {
	config, err := LoadConfig()
	if err != nil {
		return ThemeDark
	}
	if config.DarkTheme {
		return ThemeDark
	}
	return ThemeLight
}

// createLogPanel crée le panneau de logs avec barre de recherche
func createLogPanel() *fyne.Container {
	// Titre et boutons d'action
	titleLabel := widget.NewLabelWithStyle("📋 Logs", fyne.TextAlignCenter, fyne.TextStyle{Bold: true})

	// Barre de recherche pour filtrer les logs
	searchEntry := widget.NewEntry()
	searchEntry.SetPlaceHolder("🔍 Filtrer les logs...")
	searchEntry.OnChanged = func(query string) {
		filterLogs(query)
	}

	// Bouton pour vider les logs
	clearBtn := widget.NewButton("🗑️", func() {
		logMutex.Lock()
		logs = []string{}
		logNeedsUpdate = true
		logMutex.Unlock()
		addLog("📋 Logs effacés")
	})
	clearBtn.Importance = widget.LowImportance

	// Bouton pour copier les logs
	copyBtn := widget.NewButton("📋", func() {
		logMutex.Lock()
		text := strings.Join(logs, "\n")
		logMutex.Unlock()
		myWindow.Clipboard().SetContent(text)
		addLog("📋 Logs copiés dans le presse-papiers")
	})
	copyBtn.Importance = widget.LowImportance

	header := container.NewBorder(
		nil, nil,
		titleLabel,
		container.NewHBox(clearBtn, copyBtn),
		searchEntry,
	)

	return container.NewBorder(
		header,
		nil, nil, nil,
		logScroll,
	)
}

// filterLogs filtre les logs affichés
func filterLogs(query string) {
	if logWidget == nil {
		return
	}

	logMutex.Lock()
	defer logMutex.Unlock()

	if query == "" {
		logWidget.SetText(strings.Join(logs, "\n"))
		return
	}

	query = strings.ToLower(query)
	var filtered []string
	for _, log := range logs {
		if strings.Contains(strings.ToLower(log), query) {
			filtered = append(filtered, log)
		}
	}
	logWidget.SetText(strings.Join(filtered, "\n"))
}

func StartGUI() {
	myApp = app.NewWithID("com.spiraly.sync")

	// Charger et appliquer le thème sauvegardé
	savedTheme := loadTheme()
	currentTheme = savedTheme
	myApp.Settings().SetTheme(NewAppTheme(savedTheme))

	// Initialiser et charger les filtres
	InitFilterConfig()
	LoadFiltersFromConfig(GetFilterConfig())

	// Charger la configuration de synchronisation
	SetSyncConfig(LoadSyncConfigFromFile())

	startLogUpdater()

	myWindow = myApp.NewWindow("Spiralydata")

	// Charger les dimensions sauvegardées
	width, height := loadWindowSize()
	myWindow.Resize(fyne.NewSize(width, height))

	// Définir la taille minimale
	myWindow.SetFixedSize(false)

	// Créer la barre de statut
	statusBar = NewStatusBar()

	// Configurer les raccourcis clavier
	shortcutHandler = NewShortcutHandler()
	shortcutHandler.Register("clearlogs", func() {
		logMutex.Lock()
		logs = []string{}
		logNeedsUpdate = true
		logMutex.Unlock()
		addLog("📋 Logs effacés (Ctrl+L)")
	})
	shortcutHandler.SetupWindowShortcuts(myWindow)

	// Sauvegarder la configuration à la fermeture
	myWindow.SetOnClosed(func() {
		saveWindowConfig()
	})

	logWidget = widget.NewEntry()
	logWidget.SetText("🚀 Bienvenue dans Spiralydata\n💡 Raccourcis: Ctrl+T (thème), Ctrl+L (vider logs), F11 (plein écran)\n")
	logWidget.MultiLine = true
	logWidget.Wrapping = fyne.TextWrapWord
	logWidget.Disable()
	logWidget.TextStyle = fyne.TextStyle{Monospace: true}

	// Réduire la taille minimale des logs pour permettre un meilleur redimensionnement
	logScroll = container.NewVScroll(logWidget)
	logScroll.SetMinSize(fyne.NewSize(250, 150))

	logContainer := createLogPanel()

	if !tryAutoConnect(myWindow) {
		content := createMainMenu(myWindow)

		split := container.NewHSplit(
			content,
			logContainer,
		)
		split.Offset = 0.55

		// Ajouter la barre de statut en bas
		mainContent := container.NewBorder(
			nil,
			statusBar.GetContainer(),
			nil, nil,
			split,
		)

		myWindow.SetContent(mainContent)
	} else {
		addLog("🔄 Connexion automatique en cours...")
	}

	myWindow.ShowAndRun()
}

func createMainMenu(win fyne.Window) fyne.CanvasObject {
	// Logo/Titre avec style
	title := canvas.NewText("SPIRALYDATA", color.White)
	title.TextSize = 32
	title.Alignment = fyne.TextAlignCenter
	title.TextStyle = fyne.TextStyle{Bold: true}

	subtitle := widget.NewLabel("📁 Synchronisation de fichiers intelligente")
	subtitle.Alignment = fyne.TextAlignCenter

	// Version
	versionLabel := widget.NewLabel("v2.0.0")
	versionLabel.Alignment = fyne.TextAlignCenter
	versionLabel.TextStyle = fyne.TextStyle{Italic: true}

	// Boutons avec tooltips
	hostBtn := widget.NewButton("🖥️ Mode Hôte (Host)", func() {
		showHostSetup(win)
	})
	hostBtn.Importance = widget.HighImportance

	userBtn := widget.NewButton("👤 Mode Utilisateur (User)", func() {
		showUserSetup(win)
	})
	userBtn.Importance = widget.HighImportance

	// Bouton paramètres
	settingsBtn := widget.NewButton("⚙️ Paramètres", func() {
		showSettings(win)
	})
	settingsBtn.Importance = widget.MediumImportance

	quitBtn := widget.NewButton("❌ Quitter", func() {
		saveWindowConfig()
		myApp.Quit()
	})
	quitBtn.Importance = widget.LowImportance

	// Info sur les raccourcis
	shortcutsInfo := widget.NewLabel("💡 Ctrl+T: Changer thème | F11: Plein écran")
	shortcutsInfo.Alignment = fyne.TextAlignCenter
	shortcutsInfo.TextStyle = fyne.TextStyle{Italic: true}

	buttonsContainer := container.NewVBox(
		hostBtn,
		userBtn,
		widget.NewSeparator(),
		settingsBtn,
		layout.NewSpacer(),
		quitBtn,
	)

	return container.NewVBox(
		layout.NewSpacer(),
		container.NewCenter(title),
		container.NewCenter(subtitle),
		container.NewCenter(versionLabel),
		layout.NewSpacer(),
		container.NewCenter(container.NewPadded(
			container.NewVBox(
				buttonsContainer,
			),
		)),
		layout.NewSpacer(),
		container.NewCenter(shortcutsInfo),
		layout.NewSpacer(),
	)
}

// showSettings affiche la page des paramètres
func showSettings(win fyne.Window) {
	config, _ := LoadConfig()
	if config == nil {
		config = &AppConfig{DarkTheme: true, ShowStatusBar: true, LogsMaxCount: 100}
	}

	// Thème
	themeLabel := widget.NewLabel("🎨 Thème")
	themeSelect := widget.NewSelect([]string{"Sombre", "Clair"}, func(selected string) {
		if selected == "Sombre" {
			SetTheme(ThemeDark)
		} else {
			SetTheme(ThemeLight)
		}
	})
	if GetCurrentTheme() == ThemeDark {
		themeSelect.SetSelected("Sombre")
	} else {
		themeSelect.SetSelected("Clair")
	}

	// Nombre max de logs
	logsLabel := widget.NewLabel("📋 Nombre max de logs")
	logsEntry := widget.NewEntry()
	logsEntry.SetText(fmt.Sprintf("%d", maxLogs))
	logsEntry.OnChanged = func(s string) {
		var n int
		if _, err := fmt.Sscanf(s, "%d", &n); err == nil && n > 0 && n <= 1000 {
			maxLogs = n
		}
	}

	// Réinitialiser la configuration
	resetBtn := widget.NewButton("🔄 Réinitialiser la configuration", func() {
		SaveConfig(&AppConfig{DarkTheme: true, ShowStatusBar: true, LogsMaxCount: 100})
		addLog("⚙️ Configuration réinitialisée")
	})
	resetBtn.Importance = widget.DangerImportance

	formContent := container.NewVBox(
		themeLabel,
		themeSelect,
		widget.NewSeparator(),
		logsLabel,
		logsEntry,
		widget.NewSeparator(),
		resetBtn,
	)

	backBtn := widget.NewButton("⬅️ Retour", func() {
		win.SetContent(container.NewBorder(
			nil,
			statusBar.GetContainer(),
			nil, nil,
			container.NewHSplit(
				createMainMenu(win),
				createLogPanel(),
			),
		))
	})

	content := container.NewVBox(
		widget.NewLabelWithStyle("⚙️ Paramètres", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		layout.NewSpacer(),
		formContent,
		layout.NewSpacer(),
		container.NewCenter(container.NewPadded(backBtn)),
		layout.NewSpacer(),
	)

	split := container.NewHSplit(
		content,
		createLogPanel(),
	)
	split.Offset = 0.5

	win.SetContent(container.NewBorder(
		nil,
		statusBar.GetContainer(),
		nil, nil,
		split,
	))
}

func showHostSetup(win fyne.Window) {
	portLabel := widget.NewLabel("🌐 Port")
	portLabel.Alignment = fyne.TextAlignLeading
	portEntry := widget.NewEntry()
	portEntry.SetPlaceHolder("ex: 1234")

	idLabel := widget.NewLabel("🔑 ID du serveur (6 chiffres)")
	idLabel.Alignment = fyne.TextAlignLeading
	idEntry := widget.NewEntry()
	idEntry.SetPlaceHolder("ex: 123456")

	// Validation en temps réel
	idEntry.OnChanged = func(s string) {
		if len(s) > 6 {
			idEntry.SetText(s[:6])
		}
	}

	// Section filtres
	filterConfig := GetFilterConfig()
	filterSummary := widget.NewLabel(filterConfig.GetSummary())
	filterSummary.TextStyle = fyne.TextStyle{Italic: true}
	filterSummary.Wrapping = fyne.TextWrapWord

	filterBtn := widget.NewButton("🔍 Configurer les filtres", func() {
		ShowFilterDialog(filterConfig, win, func() {
			filterSummary.SetText(filterConfig.GetSummary())
		})
	})
	filterBtn.Importance = widget.MediumImportance

	filterSection := container.NewVBox(
		widget.NewSeparator(),
		widget.NewLabelWithStyle("📁 Filtrage des fichiers", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		filterSummary,
		filterBtn,
	)

	formContent := container.NewVBox(
		portLabel,
		portEntry,
		widget.NewSeparator(),
		idLabel,
		idEntry,
		filterSection,
	)

	startBtn := widget.NewButton("🚀 Démarrer le serveur", func() {
		port := portEntry.Text
		hostID := idEntry.Text

		if len(hostID) != 6 {
			addLog("❌ L'ID doit contenir 6 caractères")
			return
		}

		if port == "" {
			addLog("❌ Le port est requis")
			return
		}

		showHostRunning(win, port, hostID)
	})
	startBtn.Importance = widget.HighImportance

	backBtn := widget.NewButton("⬅️ Retour", func() {
		win.SetContent(container.NewBorder(
			nil,
			statusBar.GetContainer(),
			nil, nil,
			container.NewHSplit(
				createMainMenu(win),
				createLogPanel(),
			),
		))
	})

	buttonsContainer := container.NewVBox(
		startBtn,
		backBtn,
	)

	content := container.NewVBox(
		widget.NewLabelWithStyle("🖥️ Configuration du Serveur", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		layout.NewSpacer(),
		formContent,
		layout.NewSpacer(),
		container.NewCenter(container.NewPadded(buttonsContainer)),
		layout.NewSpacer(),
	)

	split := container.NewHSplit(
		content,
		createLogPanel(),
	)
	split.Offset = 0.5

	win.SetContent(container.NewBorder(
		nil,
		statusBar.GetContainer(),
		nil, nil,
		split,
	))
}

func showHostRunning(win fyne.Window, port, hostID string) {
	addLog("🚀 Démarrage du serveur...")

	localIP := getLocalIP()
	publicIP := "⏳ Chargement..."

	// Carte d'information du serveur
	serverInfoCard := NewStatCard("Serveur", "🖥️", "Démarrage...")

	infoText := fmt.Sprintf(
		"🖥️ SERVEUR EN DÉMARRAGE\n\n"+
			"🔑 ID: %s\n"+
			"🌐 Port: %s\n"+
			"🏠 IP Locale: %s\n"+
			"🌍 IP Publique: %s\n"+
			"📍 Adresse: %s:%s\n\n"+
			"👥 Clients connectés: 0",
		hostID, port, localIP, publicIP, publicIP, port,
	)

	info := widget.NewLabel(infoText)
	info.Wrapping = fyne.TextWrapWord

	loadingChars := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	loadingIndex := 0
	stopLoading := false
	loadingLabel := widget.NewLabel("⠋ Démarrage du serveur...")

	var currentServer *Server

	go func() {
		for !stopLoading {
			time.Sleep(100 * time.Millisecond)
			if !stopLoading {
				char := loadingChars[loadingIndex%len(loadingChars)]
				loadingLabel.SetText(fmt.Sprintf("%s Démarrage du serveur...", char))
				loadingLabel.Refresh()
				loadingIndex++
			}
		}
		loadingLabel.SetText("✅ Serveur actif")
		loadingLabel.Refresh()
		serverInfoCard.SetValue("Actif ✅")
	}()

	stopBtn := widget.NewButton("🛑 Arrêter le serveur", func() {
		addLog("🛑 Arrêt du serveur...")
		stopLoading = true

		if currentServer != nil {
			currentServer.Stop()
		}

		statusBar.SetConnected(false, "")

		win.SetContent(container.NewBorder(
			nil,
			statusBar.GetContainer(),
			nil, nil,
			container.NewHSplit(
				createMainMenu(win),
				createLogPanel(),
			),
		))
	})
	stopBtn.Importance = widget.DangerImportance

	// Bouton copier l'adresse
	copyAddrBtn := widget.NewButton("📋 Copier l'adresse", func() {
		addr := fmt.Sprintf("%s:%s", localIP, port)
		win.Clipboard().SetContent(addr)
		addLog("📋 Adresse copiée: " + addr)
	})
	copyAddrBtn.Importance = widget.LowImportance

	content := container.NewVBox(
		widget.NewLabelWithStyle("ℹ️ Informations du Serveur", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		widget.NewSeparator(),
		serverInfoCard.GetContainer(),
		widget.NewSeparator(),
		info,
		widget.NewSeparator(),
		loadingLabel,
		layout.NewSpacer(),
		container.NewHBox(
			copyAddrBtn,
			layout.NewSpacer(),
			stopBtn,
		),
	)

	split := container.NewHSplit(
		container.NewPadded(content),
		createLogPanel(),
	)
	split.Offset = 0.5

	win.SetContent(container.NewBorder(
		nil,
		statusBar.GetContainer(),
		nil, nil,
		split,
	))

	go func() {
		pubIP := getPublicIP()
		time.Sleep(1 * time.Second)
		stopLoading = true

		updatedInfo := fmt.Sprintf(
			"✅ SERVEUR ACTIF\n\n"+
				"🔑 ID: %s\n"+
				"🌐 Port: %s\n"+
				"🏠 IP Locale: %s\n"+
				"🌍 IP Publique: %s\n"+
				"📍 Adresse: %s:%s\n\n"+
				"💡 Partagez l'IP publique et l'ID\n"+
				"avec les utilisateurs.",
			hostID, port, localIP, pubIP, pubIP, port,
		)
		info.SetText(updatedInfo)
		info.Refresh()

		statusBar.SetConnected(true, localIP+":"+port)

		currentServer = NewServer(hostID)
		addLog(fmt.Sprintf("🌐 Port: %s", port))
		addLog(fmt.Sprintf("🔑 ID: %s", hostID))
		addLog(fmt.Sprintf("🏠 IP Locale: %s", localIP))
		addLog(fmt.Sprintf("🌍 IP Publique: %s", pubIP))
		currentServer.Start(port)
	}()
} 