package main

import (
	"fmt"
	"image/color"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
)

// ExtensionTag représente un tag d'extension avec bouton de suppression
type ExtensionTag struct {
	widget.BaseWidget
	extension string
	onRemove  func(string)
	container *fyne.Container
}

// NewExtensionTag crée un nouveau tag d'extension
func NewExtensionTag(ext string, onRemove func(string)) *ExtensionTag {
	tag := &ExtensionTag{
		extension: ext,
		onRemove:  onRemove,
	}
	tag.ExtendBaseWidget(tag)
	return tag
}

// CreateRenderer crée le renderer pour le tag
func (t *ExtensionTag) CreateRenderer() fyne.WidgetRenderer {
	bg := canvas.NewRectangle(color.RGBA{R: 60, G: 120, B: 200, A: 255})
	bg.CornerRadius = 4

	label := widget.NewLabel(t.extension)
	label.TextStyle = fyne.TextStyle{Monospace: true}

	removeBtn := widget.NewButton("×", func() {
		if t.onRemove != nil {
			t.onRemove(t.extension)
		}
	})
	removeBtn.Importance = widget.LowImportance

	t.container = container.NewHBox(label, removeBtn)
	content := container.NewStack(bg, container.NewPadded(t.container))

	return widget.NewSimpleRenderer(content)
}

// FilterConfigUI gère l'interface des filtres
type FilterConfigUI struct {
	config       *FilterConfig
	win          fyne.Window
	tagsContainer *fyne.Container
	modeToggle   *widget.Button
	enableCheck  *widget.Check
	summaryLabel *widget.Label
	onUpdate     func()
}

// NewFilterConfigUI crée une nouvelle interface de filtres
func NewFilterConfigUI(config *FilterConfig, win fyne.Window, onUpdate func()) *FilterConfigUI {
	return &FilterConfigUI{
		config:   config,
		win:      win,
		onUpdate: onUpdate,
	}
}

// CreateExtensionFilterPanel crée le panneau de filtrage par extension
func (ui *FilterConfigUI) CreateExtensionFilterPanel() *fyne.Container {
	// Titre
	titleLabel := widget.NewLabelWithStyle("📁 Filtrage par Extension", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})

	// Checkbox pour activer/désactiver
	ui.enableCheck = widget.NewCheck("Activer le filtrage", func(enabled bool) {
		ui.config.Filters.Extension.SetEnabled(enabled)
		ui.updateUI()
		if ui.onUpdate != nil {
			ui.onUpdate()
		}
	})
	ui.enableCheck.SetChecked(ui.config.Filters.Extension.IsEnabled())

	// Label du mode
	modeLabel := widget.NewLabel("Extensions à exclure:")

	// Bouton toggle mode
	ui.modeToggle = widget.NewButton("🔄 Inverser (Whitelist)", func() {
		currentMode := ui.config.Filters.Extension.GetMode()
		if currentMode == FilterModeBlacklist {
			ui.config.Filters.Extension.SetMode(FilterModeWhitelist)
			ui.modeToggle.SetText("🔄 Inverser (Blacklist)")
			modeLabel.SetText("Extensions autorisées:")
		} else {
			ui.config.Filters.Extension.SetMode(FilterModeBlacklist)
			ui.modeToggle.SetText("🔄 Inverser (Whitelist)")
			modeLabel.SetText("Extensions à exclure:")
		}
		if ui.onUpdate != nil {
			ui.onUpdate()
		}
	})
	ui.modeToggle.Importance = widget.LowImportance

	// Initialiser le texte du mode
	if ui.config.Filters.Extension.GetMode() == FilterModeWhitelist {
		ui.modeToggle.SetText("🔄 Inverser (Blacklist)")
		modeLabel.SetText("Extensions autorisées:")
	}

	// Conteneur pour les tags
	ui.tagsContainer = container.NewGridWrap(fyne.NewSize(80, 35))
	ui.refreshTags()

	tagsScroll := container.NewHScroll(ui.tagsContainer)
	tagsScroll.SetMinSize(fyne.NewSize(400, 45))

	// Bouton ajouter
	addBtn := widget.NewButton("➕ Ajouter", func() {
		ui.showAddExtensionDialog()
	})
	addBtn.Importance = widget.HighImportance

	// Bouton suggestions
	suggestBtn := widget.NewButton("📋 Suggestions", func() {
		ui.showSuggestionsDialog()
	})
	suggestBtn.Importance = widget.MediumImportance

	// Bouton tout effacer
	clearBtn := widget.NewButton("🗑️ Tout effacer", func() {
		dialog.ShowConfirm("Confirmer", "Supprimer toutes les extensions?", func(ok bool) {
			if ok {
				ui.config.Filters.Extension.Clear()
				ui.refreshTags()
				if ui.onUpdate != nil {
					ui.onUpdate()
				}
			}
		}, ui.win)
	})
	clearBtn.Importance = widget.DangerImportance

	// Résumé
	ui.summaryLabel = widget.NewLabel("")
	ui.updateSummary()

	// Layout
	header := container.NewHBox(
		titleLabel,
		layout.NewSpacer(),
		ui.enableCheck,
	)

	modeRow := container.NewHBox(
		modeLabel,
		layout.NewSpacer(),
		ui.modeToggle,
	)

	buttonsRow := container.NewHBox(
		addBtn,
		suggestBtn,
		layout.NewSpacer(),
		clearBtn,
	)

	return container.NewVBox(
		header,
		widget.NewSeparator(),
		modeRow,
		tagsScroll,
		buttonsRow,
		widget.NewSeparator(),
		ui.summaryLabel,
	)
}

// refreshTags rafraîchit l'affichage des tags
func (ui *FilterConfigUI) refreshTags() {
	ui.tagsContainer.Objects = nil

	extensions := ui.config.Filters.Extension.GetExtensions()
	for _, ext := range extensions {
		extCopy := ext
		tag := ui.createTagWidget(extCopy)
		ui.tagsContainer.Add(tag)
	}

	ui.tagsContainer.Refresh()
	ui.updateSummary()
}

// createTagWidget crée un widget tag pour une extension
func (ui *FilterConfigUI) createTagWidget(ext string) fyne.CanvasObject {
	// Fond coloré
	var bgColor color.Color
	if ui.config.Filters.Extension.GetMode() == FilterModeBlacklist {
		bgColor = color.RGBA{R: 180, G: 60, B: 60, A: 255} // Rouge pour exclusion
	} else {
		bgColor = color.RGBA{R: 60, G: 140, B: 60, A: 255} // Vert pour inclusion
	}

	bg := canvas.NewRectangle(bgColor)
	bg.CornerRadius = 4

	label := canvas.NewText(ext, color.White)
	label.TextSize = 12
	label.TextStyle = fyne.TextStyle{Monospace: true}

	removeBtn := widget.NewButton("×", func() {
		ui.config.Filters.Extension.RemoveExtension(ext)
		ui.refreshTags()
		if ui.onUpdate != nil {
			ui.onUpdate()
		}
		addLog(fmt.Sprintf("🏷️ Extension supprimée: %s", ext))
	})
	removeBtn.Importance = widget.LowImportance

	content := container.NewHBox(
		container.NewPadded(label),
		removeBtn,
	)

	return container.NewStack(bg, content)
}

// showAddExtensionDialog affiche le dialogue d'ajout d'extension
func (ui *FilterConfigUI) showAddExtensionDialog() {
	entry := widget.NewEntry()
	entry.SetPlaceHolder("ex: .jpg ou jpg")

	errorLabel := widget.NewLabel("")
	errorLabel.TextStyle = fyne.TextStyle{Italic: true}

	content := container.NewVBox(
		widget.NewLabel("Entrez l'extension à ajouter:"),
		entry,
		errorLabel,
	)

	dialog.ShowCustomConfirm("Ajouter une extension", "Ajouter", "Annuler", content, func(ok bool) {
		if ok {
			ext := entry.Text
			if ext == "" {
				return
			}

			err := ui.config.Filters.Extension.AddExtension(ext)
			if err != nil {
				dialog.ShowError(err, ui.win)
				return
			}

			ui.refreshTags()
			if ui.onUpdate != nil {
				ui.onUpdate()
			}
			addLog(fmt.Sprintf("🏷️ Extension ajoutée: %s", ext))
		}
	}, ui.win)

	// Validation en temps réel
	entry.OnChanged = func(s string) {
		if s == "" {
			errorLabel.SetText("")
			return
		}
		if !ValidateExtension(s) {
			errorLabel.SetText("❌ Format invalide")
		} else {
			normalized, _ := NormalizeExtension(s)
			errorLabel.SetText(fmt.Sprintf("✓ Sera ajouté comme: %s", normalized))
		}
	}
}

// showSuggestionsDialog affiche le dialogue de suggestions
func (ui *FilterConfigUI) showSuggestionsDialog() {
	var checkboxes []*widget.Check
	categoryBoxes := make(map[string]*widget.Check)

	content := container.NewVBox()

	for category, extensions := range SuggestedExtensions {
		catCopy := category
		extList := strings.Join(extensions, ", ")

		check := widget.NewCheck(fmt.Sprintf("%s (%s)", category, extList), func(checked bool) {})
		categoryBoxes[catCopy] = check
		checkboxes = append(checkboxes, check)
		content.Add(check)
	}

	// Ajouter une option pour les dossiers communs
	content.Add(widget.NewSeparator())
	content.Add(widget.NewLabel("Dossiers à exclure:"))

	folderChecks := make(map[string]*widget.Check)
	for _, folder := range CommonExcludedFolders[:7] { // Limiter à 7 dossiers
		folderCopy := folder
		check := widget.NewCheck(folder, func(checked bool) {})
		folderChecks[folderCopy] = check
		content.Add(check)
	}

	scroll := container.NewVScroll(content)
	scroll.SetMinSize(fyne.NewSize(350, 300))

	dialog.ShowCustomConfirm("Suggestions", "Appliquer", "Annuler", scroll, func(ok bool) {
		if ok {
			// Ajouter les extensions cochées
			for category, check := range categoryBoxes {
				if check.Checked {
					ui.config.Filters.Extension.AddSuggestedCategory(category)
					addLog(fmt.Sprintf("🏷️ Catégorie ajoutée: %s", category))
				}
			}

			// Ajouter les dossiers cochés
			for folder, check := range folderChecks {
				if check.Checked {
					ui.config.Filters.Path.AddExcludedFolder(folder)
					addLog(fmt.Sprintf("📁 Dossier exclu: %s", folder))
				}
			}

			ui.refreshTags()
			if ui.onUpdate != nil {
				ui.onUpdate()
			}
		}
	}, ui.win)
}

// updateSummary met à jour le résumé
func (ui *FilterConfigUI) updateSummary() {
	if ui.summaryLabel == nil {
		return
	}

	extCount := len(ui.config.Filters.Extension.GetExtensions())
	ignoredCount := ui.config.Filters.Extension.GetIgnoredCount()

	mode := "exclusion"
	if ui.config.Filters.Extension.GetMode() == FilterModeWhitelist {
		mode = "inclusion"
	}

	status := "désactivé"
	if ui.config.Filters.Extension.IsEnabled() {
		status = "activé"
	}

	summary := fmt.Sprintf("📊 %d extensions en %s | Statut: %s | Fichiers ignorés: %d",
		extCount, mode, status, ignoredCount)
	ui.summaryLabel.SetText(summary)
}

// updateUI met à jour l'interface
func (ui *FilterConfigUI) updateUI() {
	ui.refreshTags()
}

// CreateSizeFilterPanel crée le panneau de filtrage par taille
func (ui *FilterConfigUI) CreateSizeFilterPanel() *fyne.Container {
	titleLabel := widget.NewLabelWithStyle("📏 Filtrage par Taille", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})

	enableCheck := widget.NewCheck("Activer", func(enabled bool) {
		ui.config.Filters.Size.Enabled = enabled
		if ui.onUpdate != nil {
			ui.onUpdate()
		}
	})
	enableCheck.SetChecked(ui.config.Filters.Size.Enabled)

	// Taille minimale
	minLabel := widget.NewLabel("Taille minimale (Ko):")
	minEntry := widget.NewEntry()
	minEntry.SetPlaceHolder("0")
	if ui.config.Filters.Size.MinSize > 0 {
		minEntry.SetText(fmt.Sprintf("%d", ui.config.Filters.Size.MinSize/1024))
	}
	minEntry.OnChanged = func(s string) {
		if val, err := strconv.ParseInt(s, 10, 64); err == nil {
			ui.config.Filters.Size.MinSize = val * 1024
			if ui.onUpdate != nil {
				ui.onUpdate()
			}
		}
	}

	// Taille maximale
	maxLabel := widget.NewLabel("Taille maximale (Mo):")
	maxEntry := widget.NewEntry()
	maxEntry.SetPlaceHolder("0 (illimité)")
	if ui.config.Filters.Size.MaxSize > 0 {
		maxEntry.SetText(fmt.Sprintf("%d", ui.config.Filters.Size.MaxSize/(1024*1024)))
	}
	maxEntry.OnChanged = func(s string) {
		if val, err := strconv.ParseInt(s, 10, 64); err == nil {
			ui.config.Filters.Size.MaxSize = val * 1024 * 1024
			if ui.onUpdate != nil {
				ui.onUpdate()
			}
		}
	}

	// Presets rapides
	presetsLabel := widget.NewLabel("Presets:")
	preset10MB := widget.NewButton("< 10 Mo", func() {
		maxEntry.SetText("10")
		ui.config.Filters.Size.MaxSize = 10 * 1024 * 1024
		if ui.onUpdate != nil {
			ui.onUpdate()
		}
	})
	preset10MB.Importance = widget.LowImportance

	preset100MB := widget.NewButton("< 100 Mo", func() {
		maxEntry.SetText("100")
		ui.config.Filters.Size.MaxSize = 100 * 1024 * 1024
		if ui.onUpdate != nil {
			ui.onUpdate()
		}
	})
	preset100MB.Importance = widget.LowImportance

	preset1GB := widget.NewButton("< 1 Go", func() {
		maxEntry.SetText("1024")
		ui.config.Filters.Size.MaxSize = 1024 * 1024 * 1024
		if ui.onUpdate != nil {
			ui.onUpdate()
		}
	})
	preset1GB.Importance = widget.LowImportance

	header := container.NewHBox(titleLabel, layout.NewSpacer(), enableCheck)

	minRow := container.NewBorder(nil, nil, minLabel, nil, minEntry)
	maxRow := container.NewBorder(nil, nil, maxLabel, nil, maxEntry)
	presetsRow := container.NewHBox(presetsLabel, preset10MB, preset100MB, preset1GB)

	return container.NewVBox(
		header,
		widget.NewSeparator(),
		minRow,
		maxRow,
		presetsRow,
	)
}

// CreatePathFilterPanel crée le panneau de filtrage par chemin
func (ui *FilterConfigUI) CreatePathFilterPanel() *fyne.Container {
	titleLabel := widget.NewLabelWithStyle("📂 Filtrage par Chemin", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})

	enableCheck := widget.NewCheck("Activer", func(enabled bool) {
		ui.config.Filters.Path.Enabled = enabled
		if ui.onUpdate != nil {
			ui.onUpdate()
		}
	})
	enableCheck.SetChecked(ui.config.Filters.Path.Enabled)

	// Options
	hiddenCheck := widget.NewCheck("Exclure fichiers cachés", func(checked bool) {
		ui.config.Filters.Path.ExcludeHidden = checked
		if ui.onUpdate != nil {
			ui.onUpdate()
		}
	})
	hiddenCheck.SetChecked(ui.config.Filters.Path.ExcludeHidden)

	symlinkCheck := widget.NewCheck("Exclure liens symboliques", func(checked bool) {
		ui.config.Filters.Path.ExcludeSymlinks = checked
		if ui.onUpdate != nil {
			ui.onUpdate()
		}
	})
	symlinkCheck.SetChecked(ui.config.Filters.Path.ExcludeSymlinks)

	// Liste des dossiers exclus
	foldersLabel := widget.NewLabel("Dossiers exclus:")
	foldersList := widget.NewLabel(strings.Join(ui.config.Filters.Path.ExcludedFolders, ", "))
	foldersList.Wrapping = fyne.TextWrapWord

	// Bouton ajouter dossier
	addFolderBtn := widget.NewButton("➕ Ajouter dossier", func() {
		entry := widget.NewEntry()
		entry.SetPlaceHolder("ex: node_modules")

		dialog.ShowCustomConfirm("Ajouter un dossier à exclure", "Ajouter", "Annuler",
			container.NewVBox(widget.NewLabel("Nom du dossier:"), entry),
			func(ok bool) {
				if ok && entry.Text != "" {
					ui.config.Filters.Path.AddExcludedFolder(entry.Text)
					foldersList.SetText(strings.Join(ui.config.Filters.Path.ExcludedFolders, ", "))
					if ui.onUpdate != nil {
						ui.onUpdate()
					}
					addLog(fmt.Sprintf("📁 Dossier exclu ajouté: %s", entry.Text))
				}
			}, ui.win)
	})
	addFolderBtn.Importance = widget.MediumImportance

	header := container.NewHBox(titleLabel, layout.NewSpacer(), enableCheck)

	return container.NewVBox(
		header,
		widget.NewSeparator(),
		hiddenCheck,
		symlinkCheck,
		widget.NewSeparator(),
		foldersLabel,
		foldersList,
		addFolderBtn,
	)
}

// CreateFullFilterPanel crée le panneau complet des filtres
func (ui *FilterConfigUI) CreateFullFilterPanel() *fyne.Container {
	extPanel := ui.CreateExtensionFilterPanel()
	sizePanel := ui.CreateSizeFilterPanel()
	pathPanel := ui.CreatePathFilterPanel()

	// Boutons d'action globaux
	saveBtn := widget.NewButton("💾 Sauvegarder", func() {
		if err := SaveFiltersToConfig(ui.config); err != nil {
			dialog.ShowError(err, ui.win)
		} else {
			addLog("✅ Filtres sauvegardés dans la configuration")
			dialog.ShowInformation("Succès", "Les filtres ont été sauvegardés.", ui.win)
		}
	})
	saveBtn.Importance = widget.HighImportance

	loadBtn := widget.NewButton("📂 Charger", func() {
		LoadFiltersFromConfig(ui.config)
		ui.refreshTags()
		if ui.onUpdate != nil {
			ui.onUpdate()
		}
		addLog("✅ Filtres chargés depuis la configuration")
	})
	loadBtn.Importance = widget.MediumImportance

	resetBtn := widget.NewButton("🔄 Réinitialiser", func() {
		dialog.ShowConfirm("Confirmer", "Réinitialiser tous les filtres?", func(ok bool) {
			if ok {
				ui.config.Filters = NewFilterConfig().Filters
				ui.refreshTags()
				if ui.onUpdate != nil {
					ui.onUpdate()
				}
				addLog("🔄 Filtres réinitialisés")
			}
		}, ui.win)
	})
	resetBtn.Importance = widget.DangerImportance

	actionsRow := container.NewHBox(
		saveBtn,
		loadBtn,
		layout.NewSpacer(),
		resetBtn,
	)

	return container.NewVBox(
		container.NewPadded(extPanel),
		container.NewPadded(sizePanel),
		container.NewPadded(pathPanel),
		widget.NewSeparator(),
		container.NewPadded(actionsRow),
	)
}

// ShowFilterDialog affiche le dialogue de configuration des filtres
func ShowFilterDialog(config *FilterConfig, win fyne.Window, onUpdate func()) {
	ui := NewFilterConfigUI(config, win, onUpdate)
	content := ui.CreateFullFilterPanel()

	scroll := container.NewVScroll(content)
	scroll.SetMinSize(fyne.NewSize(500, 450))

	dialog.ShowCustom("⚙️ Configuration des Filtres", "Fermer", scroll, win)
}

// CreateFilterIndicator crée un indicateur de filtres actifs pour l'UI
func CreateFilterIndicator(config *FilterConfig) *fyne.Container {
	icon := widget.NewLabel("🔍")
	
	summary := config.GetSummary()
	label := widget.NewLabel(summary)
	label.TextStyle = fyne.TextStyle{Italic: true}

	return container.NewHBox(icon, label)
}
