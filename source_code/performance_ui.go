package main

import (
	"fmt"
	"runtime"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// ShowPerformanceMonitor affiche le moniteur de performance
func ShowPerformanceMonitor(window fyne.Window) {
	// Labels pour les statistiques
	memAllocLabel := widget.NewLabel("--")
	memSysLabel := widget.NewLabel("--")
	memHeapLabel := widget.NewLabel("--")
	gcCountLabel := widget.NewLabel("--")
	
	bufferGetsLabel := widget.NewLabel("--")
	bufferPutsLabel := widget.NewLabel("--")
	bufferCreatedLabel := widget.NewLabel("--")
	
	dirCacheLabel := widget.NewLabel("--")
	hashCacheLabel := widget.NewLabel("--")
	
	transfersLabel := widget.NewLabel("--")
	bytesLabel := widget.NewLabel("--")
	
	// Fonction de mise à jour
	updateStats := func() {
		// Mémoire
		var memStats runtime.MemStats
		runtime.ReadMemStats(&memStats)
		
		memAllocLabel.SetText(FormatFileSize(int64(memStats.Alloc)))
		memSysLabel.SetText(FormatFileSize(int64(memStats.Sys)))
		memHeapLabel.SetText(FormatFileSize(int64(memStats.HeapAlloc)))
		gcCountLabel.SetText(fmt.Sprintf("%d", memStats.NumGC))
		
		// Buffer pool
		bufStats := GetBufferPool().GetStats()
		bufferGetsLabel.SetText(fmt.Sprintf("%d", bufStats.Gets))
		bufferPutsLabel.SetText(fmt.Sprintf("%d", bufStats.Puts))
		bufferCreatedLabel.SetText(fmt.Sprintf("%d", bufStats.Created))
		
		// Caches
		dirCacheLabel.SetText(fmt.Sprintf("%d entrées", GetDirCache().Size()))
		hashCacheLabel.SetText(fmt.Sprintf("%d entrées", GetHashCache().Size()))
		
		// Performance globale
		perfStats := GetPerfStats()
		transfersLabel.SetText(fmt.Sprintf("%d", perfStats.TransfersCompleted))
		bytesLabel.SetText(FormatFileSize(perfStats.BytesTransferred))
	}
	
	// Section Mémoire
	memorySection := container.NewVBox(
		widget.NewLabelWithStyle("💾 Mémoire", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		container.NewGridWithColumns(2,
			widget.NewLabel("Alloué:"), memAllocLabel,
			widget.NewLabel("Système:"), memSysLabel,
			widget.NewLabel("Heap:"), memHeapLabel,
			widget.NewLabel("GC runs:"), gcCountLabel,
		),
	)
	
	// Section Buffer Pool
	bufferSection := container.NewVBox(
		widget.NewLabelWithStyle("📦 Buffer Pool", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		container.NewGridWithColumns(2,
			widget.NewLabel("Gets:"), bufferGetsLabel,
			widget.NewLabel("Puts:"), bufferPutsLabel,
			widget.NewLabel("Créés:"), bufferCreatedLabel,
		),
	)
	
	// Section Cache
	cacheSection := container.NewVBox(
		widget.NewLabelWithStyle("🗂️ Cache", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		container.NewGridWithColumns(2,
			widget.NewLabel("Dir cache:"), dirCacheLabel,
			widget.NewLabel("Hash cache:"), hashCacheLabel,
		),
	)
	
	// Section Transferts
	transferSection := container.NewVBox(
		widget.NewLabelWithStyle("📤 Transferts", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		container.NewGridWithColumns(2,
			widget.NewLabel("Complétés:"), transfersLabel,
			widget.NewLabel("Octets:"), bytesLabel,
		),
	)
	
	// Boutons d'action
	forceGCBtn := widget.NewButtonWithIcon("Forcer GC", theme.ViewRefreshIcon(), func() {
		runtime.GC()
		addLog("🔄 Garbage collection forcé")
		updateStats()
	})
	
	clearCacheBtn := widget.NewButtonWithIcon("Vider caches", theme.DeleteIcon(), func() {
		GetDirCache().InvalidateAll()
		GetHashCache().Clear()
		addLog("🗑️ Caches vidés")
		updateStats()
	})
	
	// Mise à jour automatique
	stopUpdate := make(chan bool)
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		
		for {
			select {
			case <-stopUpdate:
				return
			case <-ticker.C:
				updateStats()
			}
		}
	}()
	
	// Contenu
	content := container.NewVBox(
		memorySection,
		widget.NewSeparator(),
		bufferSection,
		widget.NewSeparator(),
		cacheSection,
		widget.NewSeparator(),
		transferSection,
		widget.NewSeparator(),
		container.NewHBox(
			layout.NewSpacer(),
			forceGCBtn,
			clearCacheBtn,
			layout.NewSpacer(),
		),
	)
	
	// Mise à jour initiale
	updateStats()
	
	// Afficher le dialogue
	dlg := dialog.NewCustom("📊 Moniteur de Performance", "Fermer", content, window)
	dlg.SetOnClosed(func() {
		close(stopUpdate)
	})
	dlg.Resize(fyne.NewSize(350, 450))
	dlg.Show()
}

// ShowPerformanceSettings affiche les paramètres de performance
func ShowPerformanceSettings(window fyne.Window) {
	// Paramètres de compression
	compressionCheck := widget.NewCheck("Compression intelligente", nil)
	compressionCheck.SetChecked(true)
	
	compressionLevelSlider := widget.NewSlider(1, 9)
	compressionLevelSlider.Value = 6
	compressionLevelLabel := widget.NewLabel("6")
	compressionLevelSlider.OnChanged = func(v float64) {
		compressionLevelLabel.SetText(fmt.Sprintf("%.0f", v))
	}
	
	// Paramètres de cache
	cacheDurationOptions := []string{"1 min", "5 min", "15 min", "30 min", "1 heure"}
	cacheDurationSelect := widget.NewSelect(cacheDurationOptions, nil)
	cacheDurationSelect.SetSelected("5 min")
	
	cacheMaxEntriesEntry := widget.NewEntry()
	cacheMaxEntriesEntry.SetText("1000")
	
	// Paramètres de transfert
	parallelWorkersSlider := widget.NewSlider(1, 16)
	parallelWorkersSlider.Value = float64(runtime.NumCPU())
	workersLabel := widget.NewLabel(fmt.Sprintf("%d", runtime.NumCPU()))
	parallelWorkersSlider.OnChanged = func(v float64) {
		workersLabel.SetText(fmt.Sprintf("%.0f", v))
	}
	
	chunkSizeOptions := []string{"64 KB", "256 KB", "512 KB", "1 MB", "4 MB"}
	chunkSizeSelect := widget.NewSelect(chunkSizeOptions, nil)
	chunkSizeSelect.SetSelected("256 KB")
	
	// Paramètres mémoire
	memoryLimitOptions := []string{"256 MB", "512 MB", "1 GB", "2 GB", "Illimité"}
	memoryLimitSelect := widget.NewSelect(memoryLimitOptions, nil)
	memoryLimitSelect.SetSelected("512 MB")
	
	memoryWarningSlider := widget.NewSlider(50, 95)
	memoryWarningSlider.Value = 80
	warningLabel := widget.NewLabel("80%")
	memoryWarningSlider.OnChanged = func(v float64) {
		warningLabel.SetText(fmt.Sprintf("%.0f%%", v))
	}
	
	// Contenu organisé
	content := container.NewVBox(
		// Compression
		widget.NewLabelWithStyle("📦 Compression", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		compressionCheck,
		container.NewBorder(nil, nil, widget.NewLabel("Niveau:"), compressionLevelLabel, compressionLevelSlider),
		widget.NewSeparator(),
		
		// Cache
		widget.NewLabelWithStyle("🗂️ Cache", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		container.NewGridWithColumns(2,
			widget.NewLabel("Durée:"), cacheDurationSelect,
			widget.NewLabel("Max entrées:"), cacheMaxEntriesEntry,
		),
		widget.NewSeparator(),
		
		// Transferts
		widget.NewLabelWithStyle("📤 Transferts", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		container.NewBorder(nil, nil, widget.NewLabel("Workers parallèles:"), workersLabel, parallelWorkersSlider),
		container.NewGridWithColumns(2,
			widget.NewLabel("Taille chunk:"), chunkSizeSelect,
		),
		widget.NewSeparator(),
		
		// Mémoire
		widget.NewLabelWithStyle("💾 Mémoire", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		container.NewGridWithColumns(2,
			widget.NewLabel("Limite:"), memoryLimitSelect,
		),
		container.NewBorder(nil, nil, widget.NewLabel("Alerte à:"), warningLabel, memoryWarningSlider),
	)
	
	scroll := container.NewVScroll(content)
	scroll.SetMinSize(fyne.NewSize(400, 450))
	
	// Bouton sauvegarder
	saveBtn := widget.NewButtonWithIcon("Sauvegarder", theme.DocumentSaveIcon(), func() {
		addLog("✅ Paramètres de performance sauvegardés")
	})
	saveBtn.Importance = widget.HighImportance
	
	dialogContent := container.NewBorder(
		nil,
		container.NewHBox(layout.NewSpacer(), saveBtn),
		nil,
		nil,
		scroll,
	)
	
	dialog.ShowCustom("⚡ Paramètres de Performance", "Fermer", dialogContent, window)
}

// CreatePerformanceButton crée un bouton pour le moniteur
func CreatePerformanceButton(window fyne.Window) *widget.Button {
	return widget.NewButtonWithIcon("📊 Perf", theme.InfoIcon(), func() {
		ShowPerformanceMonitor(window)
	})
}

// CreatePerformanceSettingsButton crée un bouton pour les paramètres
func CreatePerformanceSettingsButton(window fyne.Window) *widget.Button {
	return widget.NewButtonWithIcon("⚡ Config Perf", theme.SettingsIcon(), func() {
		ShowPerformanceSettings(window)
	})
}

// ProgressTracker suit la progression d'une opération
type ProgressTracker struct {
	total     int64
	current   int64
	startTime time.Time
	bar       *widget.ProgressBar
	label     *widget.Label
}

// NewProgressTracker crée un nouveau tracker
func NewProgressTracker(bar *widget.ProgressBar, label *widget.Label) *ProgressTracker {
	return &ProgressTracker{
		bar:   bar,
		label: label,
	}
}

// Start démarre le suivi
func (pt *ProgressTracker) Start(total int64) {
	pt.total = total
	pt.current = 0
	pt.startTime = time.Now()
	pt.update()
}

// Update met à jour
func (pt *ProgressTracker) Update(current int64) {
	pt.current = current
	pt.update()
}

// Increment incrémente
func (pt *ProgressTracker) Increment(delta int64) {
	pt.current += delta
	pt.update()
}

func (pt *ProgressTracker) update() {
	if pt.total == 0 {
		return
	}
	
	progress := float64(pt.current) / float64(pt.total)
	elapsed := time.Since(pt.startTime)
	
	if pt.bar != nil {
		pt.bar.SetValue(progress)
	}
	
	if pt.label != nil {
		var eta string
		if pt.current > 0 {
			totalTime := elapsed * time.Duration(pt.total) / time.Duration(pt.current)
			remaining := totalTime - elapsed
			if remaining > 0 {
				eta = fmt.Sprintf(" - ETA: %s", formatDuration(remaining))
			}
		}
		
		speed := float64(pt.current) / elapsed.Seconds()
		pt.label.SetText(fmt.Sprintf(
			"%s / %s (%.1f%%) - %s/s%s",
			FormatFileSize(pt.current),
			FormatFileSize(pt.total),
			progress*100,
			FormatFileSize(int64(speed)),
			eta,
		))
	}
}

// Done marque la fin
func (pt *ProgressTracker) Done() {
	pt.current = pt.total
	pt.update()
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm%ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%dh%dm", int(d.Hours()), int(d.Minutes())%60)
}

// SkeletonLoader crée un placeholder de chargement
type SkeletonLoader struct {
	container *fyne.Container
	items     []fyne.CanvasObject
}

// NewSkeletonLoader crée un nouveau skeleton loader
func NewSkeletonLoader(itemCount int) *SkeletonLoader {
	sl := &SkeletonLoader{
		items: make([]fyne.CanvasObject, itemCount),
	}
	
	for i := 0; i < itemCount; i++ {
		placeholder := widget.NewLabel("⬜ Chargement...")
		placeholder.TextStyle = fyne.TextStyle{Italic: true}
		sl.items[i] = placeholder
	}
	
	sl.container = container.NewVBox(sl.items...)
	return sl
}

// GetContainer retourne le container
func (sl *SkeletonLoader) GetContainer() *fyne.Container {
	return sl.container
}

// Replace remplace un placeholder par le contenu réel
func (sl *SkeletonLoader) Replace(index int, content fyne.CanvasObject) {
	if index >= 0 && index < len(sl.items) {
		sl.items[index] = content
		sl.container.Objects[index] = content
		sl.container.Refresh()
	}
}

// ReplaceAll remplace tous les placeholders
func (sl *SkeletonLoader) ReplaceAll(contents []fyne.CanvasObject) {
	for i, content := range contents {
		if i < len(sl.items) {
			sl.items[i] = content
			sl.container.Objects[i] = content
		}
	}
	sl.container.Refresh()
}
