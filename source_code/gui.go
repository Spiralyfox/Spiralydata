package main

import (
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
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

var (
	logWidget       *widget.Label
	logScroll       *container.Scroll
	logs            []string
	maxLogs         = 150
	myApp           fyne.App
	logMutex        sync.Mutex
	logChannel      chan string
	logTicker       *time.Ticker
	logNeedsUpdate  bool
)

func main() {
	StartGUI()
}

type darkTheme struct{}

func (d darkTheme) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	switch name {
	case theme.ColorNameBackground:
		return color.RGBA{R: 30, G: 33, B: 41, A: 255}
	case theme.ColorNameButton:
		return color.RGBA{R: 52, G: 58, B: 70, A: 255}
	case theme.ColorNameForeground:
		return color.RGBA{R: 255, G: 255, B: 255, A: 255}
	case theme.ColorNameSuccess:
		return color.RGBA{R: 46, G: 204, B: 113, A: 255}
	case theme.ColorNameError:
		return color.RGBA{R: 231, G: 76, B: 60, A: 255}
	default:
		return theme.DefaultTheme().Color(name, variant)
	}
}

func (d darkTheme) Icon(name fyne.ThemeIconName) fyne.Resource {
	return theme.DefaultTheme().Icon(name)
}

func (d darkTheme) Font(style fyne.TextStyle) fyne.Resource {
	return theme.DefaultTheme().Font(style)
}

func (d darkTheme) Size(name fyne.ThemeSizeName) float32 {
	return theme.DefaultTheme().Size(name)
}

func addLog(message string) {
	timestamp := time.Now().Format("15:04:05")
	logEntry := fmt.Sprintf("[%s] %s", timestamp, message)
	
	logMutex.Lock()
	logs = append(logs, logEntry)
	if len(logs) > maxLogs {
		logs = logs[1:]
	}
	logNeedsUpdate = true
	logMutex.Unlock()
}

func startLogUpdater() {
	logTicker = time.NewTicker(200 * time.Millisecond)
	
	go func() {
		for range logTicker.C {
			logMutex.Lock()
			needsUpdate := logNeedsUpdate
			logNeedsUpdate = false
			logMutex.Unlock()
			
			if needsUpdate && logWidget != nil {
				logMutex.Lock()
				text := strings.Join(logs, "\n")
				logMutex.Unlock()
				
				logWidget.SetText(text)
				if logScroll != nil {
					logScroll.ScrollToBottom()
				}
			}
		}
	}()
}

func getPublicIP() string {
	resp, err := http.Get("https://api.ipify.org?format=text")
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

func StartGUI() {
	myApp = app.NewWithID("com.spiraly.sync")
	myApp.Settings().SetTheme(&darkTheme{})
	
	startLogUpdater()
	
	myWindow := myApp.NewWindow("Spiraly Sync")
	myWindow.Resize(fyne.NewSize(1200, 700))
	
	logWidget = widget.NewLabel("🚀 Bienvenue dans Spiraly Sync\n")
	logWidget.Wrapping = fyne.TextWrapWord
	logScroll = container.NewVScroll(logWidget)
	
	logContainer := container.NewBorder(
		widget.NewLabelWithStyle("📋 Logs", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		nil, nil, nil,
		logScroll,
	)
	
	content := createMainMenu(myWindow)
	
	split := container.NewHSplit(
		content,
		logContainer,
	)
	split.Offset = 0.5
	
	myWindow.SetContent(split)
	myWindow.ShowAndRun()
}

func createMainMenu(win fyne.Window) fyne.CanvasObject {
	title := canvas.NewText("SPIRALY SYNC", color.White)
	title.TextSize = 28
	title.Alignment = fyne.TextAlignCenter
	title.TextStyle = fyne.TextStyle{Bold: true}
	
	subtitle := widget.NewLabel("🔄 Synchronisation de fichiers intelligente")
	subtitle.Alignment = fyne.TextAlignCenter
	
	hostBtn := widget.NewButton("🖥️ Mode Hôte (Host)", func() {
		showHostSetup(win)
	})
	hostBtn.Importance = widget.HighImportance
	
	userBtn := widget.NewButton("👤 Mode Utilisateur (User)", func() {
		showUserSetup(win)
	})
	userBtn.Importance = widget.HighImportance
	
	quitBtn := widget.NewButton("🚪 Quitter", func() {
		myApp.Quit()
	})
	
	buttonsContainer := container.NewVBox(
		hostBtn,
		userBtn,
		layout.NewSpacer(),
		quitBtn,
	)
	
	return container.NewVBox(
		layout.NewSpacer(),
		container.NewCenter(title),
		container.NewCenter(subtitle),
		layout.NewSpacer(),
		container.NewCenter(container.NewPadded(
			container.NewVBox(
				buttonsContainer,
			),
		)),
		layout.NewSpacer(),
	)
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
	
	formContent := container.NewVBox(
		portLabel,
		portEntry,
		widget.NewSeparator(),
		idLabel,
		idEntry,
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
		win.SetContent(container.NewHSplit(
			createMainMenu(win),
			container.NewBorder(
				widget.NewLabelWithStyle("📋 Logs", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
				nil, nil, nil,
				container.NewVScroll(logWidget),
			),
		))
	})
	
	buttonsContainer := container.NewVBox(
		startBtn,
		backBtn,
	)
	
	content := container.NewVBox(
		widget.NewLabelWithStyle("⚙️ Configuration du Serveur", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		layout.NewSpacer(),
		formContent,
		layout.NewSpacer(),
		container.NewCenter(container.NewPadded(buttonsContainer)),
		layout.NewSpacer(),
	)
	
	split := container.NewHSplit(
		content,
		container.NewBorder(
			widget.NewLabelWithStyle("📋 Logs", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
			nil, nil, nil,
			container.NewVScroll(logWidget),
		),
	)
	split.Offset = 0.5
	
	win.SetContent(split)
}

func showHostRunning(win fyne.Window, port, hostID string) {
	addLog("🚀 Démarrage du serveur...")
	
	localIP := getLocalIP()
	publicIP := "⏳ Chargement..."
	
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
	}()
	
	stopBtn := widget.NewButton("🛑 Arrêter le serveur", func() {
		addLog("🛑 Arrêt du serveur...")
		stopLoading = true
		
		if currentServer != nil {
			currentServer.Stop()
		}
		
		win.SetContent(container.NewHSplit(
			createMainMenu(win),
			container.NewBorder(
				widget.NewLabelWithStyle("📋 Logs", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
				nil, nil, nil,
				container.NewVScroll(logWidget),
			),
		))
	})
	stopBtn.Importance = widget.DangerImportance
	
	content := container.NewVBox(
		widget.NewLabelWithStyle("ℹ️ Informations du Serveur", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		widget.NewSeparator(),
		info,
		widget.NewSeparator(),
		loadingLabel,
		layout.NewSpacer(),
		container.NewCenter(container.NewPadded(stopBtn)),
	)
	
	split := container.NewHSplit(
		content,
		container.NewBorder(
			widget.NewLabelWithStyle("📋 Logs", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
			nil, nil, nil,
			container.NewVScroll(logWidget),
		),
	)
	split.Offset = 0.5
	
	win.SetContent(split)
	
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
		
		currentServer = NewServer(hostID)
		addLog(fmt.Sprintf("🌐 Port: %s", port))
		addLog(fmt.Sprintf("🔑 ID: %s", hostID))
		addLog(fmt.Sprintf("🏠 IP Locale: %s", localIP))
		addLog(fmt.Sprintf("🌍 IP Publique: %s", pubIP))
		currentServer.Start(port)
	}()
}

func showUserSetup(win fyne.Window) {
	prefs := myApp.Preferences()
	
	serverLabel := widget.NewLabel("🌐 IP du serveur")
	serverLabel.Alignment = fyne.TextAlignLeading
	serverEntry := widget.NewEntry()
	serverEntry.SetPlaceHolder("ex: 192.168.1.100")
	if savedIP := prefs.String("server_ip"); savedIP != "" {
		serverEntry.SetText(savedIP)
	}
	
	portLabel := widget.NewLabel("🔌 Port")
	portLabel.Alignment = fyne.TextAlignLeading
	portEntry := widget.NewEntry()
	portEntry.SetPlaceHolder("ex: 1234")
	if savedPort := prefs.String("server_port"); savedPort != "" {
		portEntry.SetText(savedPort)
	}
	
	idLabel := widget.NewLabel("🔑 ID du host")
	idLabel.Alignment = fyne.TextAlignLeading
	idEntry := widget.NewEntry()
	idEntry.SetPlaceHolder("ex: 123456")
	if savedID := prefs.String("host_id"); savedID != "" {
		idEntry.SetText(savedID)
	}
	
	saveCheck := widget.NewCheck("💾 Sauvegarder la configuration", nil)
	saveCheck.SetChecked(prefs.Bool("save_config"))
	
	formContent := container.NewVBox(
		serverLabel,
		serverEntry,
		widget.NewSeparator(),
		portLabel,
		portEntry,
		widget.NewSeparator(),
		idLabel,
		idEntry,
		widget.NewSeparator(),
		saveCheck,
	)
	
	connectBtn := widget.NewButton("🔌 Se connecter", func() {
		serverIP := serverEntry.Text
		port := portEntry.Text
		hostID := idEntry.Text
		
		if serverIP == "" || port == "" || hostID == "" {
			addLog("❌ IP, port et ID requis")
			return
		}
		
		serverAddr := serverIP + ":" + port
		
		if saveCheck.Checked {
			prefs.SetString("server_ip", serverIP)
			prefs.SetString("server_port", port)
			prefs.SetString("host_id", hostID)
			prefs.SetBool("save_config", true)
			addLog("💾 Configuration sauvegardée")
		}
		
		showUserConnecting(win, serverAddr, hostID)
	})
	connectBtn.Importance = widget.HighImportance
	
	backBtn := widget.NewButton("⬅️ Retour", func() {
		win.SetContent(container.NewHSplit(
			createMainMenu(win),
			container.NewBorder(
				widget.NewLabelWithStyle("📋 Logs", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
				nil, nil, nil,
				container.NewVScroll(logWidget),
			),
		))
	})
	
	buttonsContainer := container.NewVBox(
		connectBtn,
		backBtn,
	)
	
	content := container.NewVBox(
		widget.NewLabelWithStyle("🔌 Connexion au Serveur", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		layout.NewSpacer(),
		formContent,
		layout.NewSpacer(),
		container.NewCenter(container.NewPadded(buttonsContainer)),
		layout.NewSpacer(),
	)
	
	split := container.NewHSplit(
		content,
		container.NewBorder(
			widget.NewLabelWithStyle("📋 Logs", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
			nil, nil, nil,
			container.NewVScroll(logWidget),
		),
	)
	split.Offset = 0.5
	
	win.SetContent(split)
}

func showUserConnecting(win fyne.Window, serverAddr, hostID string) {
	addLog(fmt.Sprintf("🔌 Connexion à %s...", serverAddr))
	
	infoText := fmt.Sprintf(
		"⏳ CONNEXION EN COURS\n\n"+
			"🌐 Serveur: %s\n"+
			"🔑 ID: %s\n\n"+
			"📡 Statut: Connexion...",
		serverAddr, hostID,
	)
	
	info := widget.NewLabel(infoText)
	info.Wrapping = fyne.TextWrapWord
	
	loadingChars := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	loadingIndex := 0
	loadingLabel := widget.NewLabel("⠋ Connexion en cours...")
	statusLabel := widget.NewLabel("📡 Statut: Connexion en cours...")
	
	stopAnimation := false
	connectionSuccess := false
	var client *Client
	
	syncBtn := widget.NewButton("🔄 SYNC AUTO", nil)
	syncBtn.Importance = widget.DangerImportance
	syncBtn.Hide()
	
	pullBtn := widget.NewButton("📥 RECEVOIR", nil)
	pullBtn.Importance = widget.MediumImportance
	pullBtn.Disable()
	pullBtn.Hide()
	
	pushBtn := widget.NewButton("📤 ENVOYER", nil)
	pushBtn.Importance = widget.MediumImportance
	pushBtn.Disable()
	pushBtn.Hide()
	
	clearBtn := widget.NewButton("🗑️ VIDER LOCAL", nil)
	clearBtn.Importance = widget.MediumImportance
	clearBtn.Disable()
	clearBtn.Hide()
	
	syncBtn.OnTapped = func() {
		if client != nil {
			client.ToggleAutoSync()
			if client.autoSync {
				syncBtn.SetText("🟢 SYNC AUTO ACTIVE")
				syncBtn.Importance = widget.SuccessImportance
				statusLabel.SetText("📡 Statut: Synchronisation Automatique Active")
				
				pullBtn.Disable()
				pushBtn.Disable()
				clearBtn.Disable()
			} else {
				syncBtn.SetText("🔄 SYNC AUTO")
				syncBtn.Importance = widget.DangerImportance
				statusLabel.SetText("📡 Statut: Mode Manuel")
				
				pullBtn.Enable()
				pushBtn.Enable()
				clearBtn.Enable()
			}
			syncBtn.Refresh()
			statusLabel.Refresh()
		}
	}
	
	pullBtn.OnTapped = func() {
		if client != nil && !client.autoSync {
			pullBtn.Disable()
			pullBtn.SetText("⏳ Reception...")
			go func() {
				client.PullAllFromServer()
				time.Sleep(100 * time.Millisecond)
				pullBtn.SetText("📥 RECEVOIR")
				pullBtn.Enable()
				pullBtn.Refresh()
			}()
		}
	}
	
	pushBtn.OnTapped = func() {
		if client != nil && !client.autoSync {
			pushBtn.Disable()
			pushBtn.SetText("⏳ Envoi...")
			go func() {
				client.PushLocalChanges()
				time.Sleep(100 * time.Millisecond)
				pushBtn.SetText("📤 ENVOYER")
				pushBtn.Enable()
				pushBtn.Refresh()
			}()
		}
	}
	
	clearBtn.OnTapped = func() {
		if client != nil && !client.autoSync {
			clearBtn.Disable()
			clearBtn.SetText("⏳ Suppression...")
			go func() {
				client.ClearLocalFiles()
				time.Sleep(100 * time.Millisecond)
				clearBtn.SetText("🗑️ VIDER LOCAL")
				clearBtn.Enable()
				clearBtn.Refresh()
			}()
		}
	}
	
	syncContainer := container.NewVBox(
		widget.NewSeparator(),
		widget.NewLabelWithStyle("⚙️ Mode de Synchronisation", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		container.NewCenter(
			container.NewMax(
				container.NewPadded(syncBtn),
			),
		),
	)
	syncContainer.Hide()
	
	manualControlsContainer := container.NewVBox(
		widget.NewSeparator(),
		widget.NewLabelWithStyle("🎮 Contrôles Manuels", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		container.NewCenter(
			container.NewMax(
				container.NewPadded(pullBtn),
			),
		),
		container.NewCenter(
			container.NewMax(
				container.NewPadded(pushBtn),
			),
		),
		widget.NewSeparator(),
		widget.NewLabelWithStyle("⚡ Actions Avancées", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		container.NewCenter(
			container.NewMax(
				container.NewPadded(clearBtn),
			),
		),
	)
	manualControlsContainer.Hide()
	
	disconnectBtn := widget.NewButton("🔌 DÉCONNECTER", func() {
		addLog("👋 Déconnexion...")
		stopAnimation = true
		if client != nil {
			client.shouldExit = true
			if client.ws != nil {
				client.ws.Close()
			}
		}
		win.SetContent(container.NewHSplit(
			createMainMenu(win),
			container.NewBorder(
				widget.NewLabelWithStyle("📋 Logs", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
				nil, nil, nil,
				container.NewVScroll(logWidget),
			),
		))
	})
	disconnectBtn.Importance = widget.DangerImportance
	
	content := container.NewVBox(
		widget.NewLabelWithStyle("ℹ️ Informations de Connexion", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		widget.NewSeparator(),
		info,
		widget.NewSeparator(),
		loadingLabel,
		statusLabel,
		syncContainer,
		manualControlsContainer,
		layout.NewSpacer(),
		container.NewCenter(container.NewPadded(disconnectBtn)),
	)
	
	split := container.NewHSplit(
		content,
		container.NewBorder(
			widget.NewLabelWithStyle("📋 Logs", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
			nil, nil, nil,
			container.NewVScroll(logWidget),
		),
	)
	split.Offset = 0.5
	
	win.SetContent(split)
	
	go func() {
		for !stopAnimation && !connectionSuccess {
			time.Sleep(100 * time.Millisecond)
			if !stopAnimation && !connectionSuccess {
				char := loadingChars[loadingIndex%len(loadingChars)]
				loadingLabel.SetText(fmt.Sprintf("%s Connexion en cours...", char))
				loadingLabel.Refresh()
				loadingIndex++
			}
		}
	}()
	
	go func() {
		addLog(fmt.Sprintf("🔌 Connexion au serveur %s avec l'ID %s", serverAddr, hostID))
		
		go StartClientGUI(serverAddr, hostID, &stopAnimation, &connectionSuccess, loadingLabel, statusLabel, info, &client)
		
		time.Sleep(2 * time.Second)
		if connectionSuccess {
			syncBtn.Show()
			pullBtn.Show()
			pushBtn.Show()
			clearBtn.Show()
			
			pullBtn.Enable()
			pushBtn.Enable()
			clearBtn.Enable()
			
			syncContainer.Show()
			manualControlsContainer.Show()
			content.Refresh()
			
			addLog("🎮 Interface de contrôle prête")
		}
	}()
}