package main

import (
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// PreviewPanel gère le panneau de prévisualisation
type PreviewPanel struct {
	container    *fyne.Container
	window       fyne.Window
	currentFile  string
	backCallback func()
	zoomLevel    float64
}

// NewPreviewPanel crée un nouveau panneau de prévisualisation
func NewPreviewPanel(window fyne.Window, backCallback func()) *PreviewPanel {
	return &PreviewPanel{
		window:       window,
		backCallback: backCallback,
		zoomLevel:    1.0,
	}
}

// ShowPreview affiche la prévisualisation d'un fichier
func (pp *PreviewPanel) ShowPreview(filePath string) *fyne.Container {
	pp.currentFile = filePath
	pp.zoomLevel = 1.0
	
	// Récupérer les métadonnées
	meta, err := GetFileMetadata(filePath)
	if err != nil {
		return pp.showError(fmt.Sprintf("Erreur: %v", err))
	}
	
	// Header avec bouton retour et infos
	backBtn := widget.NewButtonWithIcon("Retour aux logs", theme.NavigateBackIcon(), func() {
		if pp.backCallback != nil {
			pp.backCallback()
		}
	})
	
	titleLabel := widget.NewLabelWithStyle(
		fmt.Sprintf("📄 %s", meta.Name),
		fyne.TextAlignLeading,
		fyne.TextStyle{Bold: true},
	)
	
	typeLabel := widget.NewLabel(fmt.Sprintf("Type: %s | %s", GetPreviewTypeName(meta.PreviewType), meta.SizeHuman))
	typeLabel.TextStyle = fyne.TextStyle{Italic: true}
	
	header := container.NewVBox(
		container.NewBorder(nil, nil, backBtn, nil, titleLabel),
		typeLabel,
		widget.NewSeparator(),
	)
	
	// Contenu de la prévisualisation selon le type
	var content fyne.CanvasObject
	
	switch meta.PreviewType {
	case PreviewTypeImage:
		content = pp.createImagePreview(filePath, meta)
	case PreviewTypeText, PreviewTypeCode, PreviewTypeMarkdown:
		content = pp.createTextPreview(filePath, meta)
	case PreviewTypeArchive:
		content = pp.createArchivePreview(filePath, meta)
	case PreviewTypeAudio:
		content = pp.createAudioPreview(filePath, meta)
	case PreviewTypeVideo:
		content = pp.createVideoPreview(filePath, meta)
	case PreviewTypePDF:
		content = pp.createPDFPreview(filePath, meta)
	default:
		content = pp.createGenericPreview(filePath, meta)
	}
	
	// Panel de métadonnées (collapsible sur le côté)
	metaPanel := pp.createMetadataPanel(meta)
	
	// Layout principal
	mainContent := container.NewBorder(
		header,
		nil,
		nil,
		metaPanel,
		content,
	)
	
	pp.container = mainContent
	return mainContent
}

// createImagePreview crée une prévisualisation d'image
func (pp *PreviewPanel) createImagePreview(filePath string, meta *FileMetadata) fyne.CanvasObject {
	file, err := os.Open(filePath)
	if err != nil {
		return widget.NewLabel(fmt.Sprintf("Erreur: %v", err))
	}
	defer file.Close()
	
	img, format, err := image.Decode(file)
	if err != nil {
		return widget.NewLabel(fmt.Sprintf("Erreur décodage: %v", err))
	}
	
	// Mettre à jour les métadonnées avec les dimensions
	bounds := img.Bounds()
	meta.ImageWidth = bounds.Dx()
	meta.ImageHeight = bounds.Dy()
	
	canvasImg := canvas.NewImageFromImage(img)
	canvasImg.FillMode = canvas.ImageFillContain
	canvasImg.SetMinSize(fyne.NewSize(400, 300))
	
	// Contrôles de zoom
	zoomInBtn := widget.NewButtonWithIcon("", theme.ZoomInIcon(), func() {
		pp.zoomLevel *= 1.2
		size := canvasImg.MinSize()
		canvasImg.SetMinSize(fyne.NewSize(size.Width*1.2, size.Height*1.2))
		canvasImg.Refresh()
	})
	
	zoomOutBtn := widget.NewButtonWithIcon("", theme.ZoomOutIcon(), func() {
		pp.zoomLevel *= 0.8
		size := canvasImg.MinSize()
		canvasImg.SetMinSize(fyne.NewSize(size.Width*0.8, size.Height*0.8))
		canvasImg.Refresh()
	})
	
	zoomResetBtn := widget.NewButton("100%", func() {
		pp.zoomLevel = 1.0
		canvasImg.SetMinSize(fyne.NewSize(400, 300))
		canvasImg.Refresh()
	})
	
	// Informations image
	infoLabel := widget.NewLabel(fmt.Sprintf("📐 %dx%d pixels | Format: %s", 
		meta.ImageWidth, meta.ImageHeight, strings.ToUpper(format)))
	
	controls := container.NewHBox(
		zoomOutBtn,
		zoomResetBtn,
		zoomInBtn,
		layout.NewSpacer(),
		infoLabel,
	)
	
	scrollContainer := container.NewScroll(container.NewCenter(canvasImg))
	
	return container.NewBorder(
		nil,
		controls,
		nil,
		nil,
		scrollContainer,
	)
}

// createTextPreview crée une prévisualisation de fichier texte
func (pp *PreviewPanel) createTextPreview(filePath string, meta *FileMetadata) fyne.CanvasObject {
	lines, err := ReadTextFile(filePath, 1000)
	if err != nil {
		return widget.NewLabel(fmt.Sprintf("Erreur lecture: %v", err))
	}
	
	// Créer le contenu avec numéros de ligne
	var content strings.Builder
	for i, line := range lines {
		content.WriteString(fmt.Sprintf("%4d │ %s\n", i+1, line))
	}
	
	textWidget := widget.NewRichTextWithText(content.String())
	textWidget.Wrapping = fyne.TextWrapOff
	
	// Utiliser une police monospace via un label standard
	textLabel := widget.NewLabel(content.String())
	textLabel.Wrapping = fyne.TextWrapOff
	textLabel.TextStyle = fyne.TextStyle{Monospace: true}
	
	// Infos sur le fichier
	lang := GetLanguageFromExtension(meta.Extension)
	infoLabel := widget.NewLabel(fmt.Sprintf("📝 %s | %d lignes | %d mots | %d caractères",
		lang, meta.LineCount, meta.WordCount, meta.CharCount))
	
	// Bouton copier
	copyBtn := widget.NewButtonWithIcon("Copier", theme.ContentCopyIcon(), func() {
		fullContent, _ := ReadTextFile(filePath, 0)
		pp.window.Clipboard().SetContent(strings.Join(fullContent, "\n"))
		dialog.ShowInformation("Copié", "Contenu copié dans le presse-papiers", pp.window)
	})
	
	// Recherche dans le texte
	searchEntry := widget.NewEntry()
	searchEntry.SetPlaceHolder("Rechercher...")
	
	controls := container.NewBorder(
		nil, nil,
		infoLabel,
		container.NewHBox(searchEntry, copyBtn),
		nil,
	)
	
	scrollContainer := container.NewScroll(textLabel)
	
	return container.NewBorder(
		controls,
		nil,
		nil,
		nil,
		scrollContainer,
	)
}

// createArchivePreview crée une prévisualisation d'archive
func (pp *PreviewPanel) createArchivePreview(filePath string, meta *FileMetadata) fyne.CanvasObject {
	entries, err := ListArchiveContents(filePath)
	if err != nil {
		return widget.NewLabel(fmt.Sprintf("Erreur lecture archive: %v", err))
	}
	
	// Créer la liste des fichiers
	list := widget.NewList(
		func() int {
			return len(entries)
		},
		func() fyne.CanvasObject {
			return container.NewHBox(
				widget.NewIcon(theme.FileIcon()),
				widget.NewLabel("Nom du fichier"),
				layout.NewSpacer(),
				widget.NewLabel("Taille"),
			)
		},
		func(id widget.ListItemID, item fyne.CanvasObject) {
			entry := entries[id]
			box := item.(*fyne.Container)
			
			icon := box.Objects[0].(*widget.Icon)
			if entry.IsDir {
				icon.SetResource(theme.FolderIcon())
			} else {
				icon.SetResource(GetFileIconResource(entry.Name))
			}
			
			nameLabel := box.Objects[1].(*widget.Label)
			nameLabel.SetText(entry.Path)
			
			sizeLabel := box.Objects[3].(*widget.Label)
			if entry.IsDir {
				sizeLabel.SetText("-")
			} else {
				sizeLabel.SetText(entry.SizeHuman)
			}
		},
	)
	
	// Calculer la taille totale
	var totalSize int64
	fileCount := 0
	dirCount := 0
	for _, entry := range entries {
		if entry.IsDir {
			dirCount++
		} else {
			fileCount++
			totalSize += entry.Size
		}
	}
	
	infoLabel := widget.NewLabel(fmt.Sprintf("📦 %d fichiers, %d dossiers | Taille décompressée: %s",
		fileCount, dirCount, FormatFileSize(totalSize)))
	
	return container.NewBorder(
		infoLabel,
		nil,
		nil,
		nil,
		list,
	)
}

// createAudioPreview crée une prévisualisation audio (basique sans lecture)
func (pp *PreviewPanel) createAudioPreview(filePath string, meta *FileMetadata) fyne.CanvasObject {
	// Icône grande
	icon := canvas.NewImageFromResource(theme.MediaMusicIcon())
	icon.SetMinSize(fyne.NewSize(128, 128))
	icon.FillMode = canvas.ImageFillContain
	
	nameLabel := widget.NewLabelWithStyle(meta.Name, fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	sizeLabel := widget.NewLabel(fmt.Sprintf("Taille: %s", meta.SizeHuman))
	
	// Message explicatif
	infoText := widget.NewRichTextFromMarkdown(`
### Lecture audio non disponible

La lecture audio n'est pas supportée dans cette version.

**Informations:**
- Format: ` + strings.ToUpper(strings.TrimPrefix(meta.Extension, ".")) + `
- Taille: ` + meta.SizeHuman + `

Utilisez le bouton **Ouvrir** pour lire ce fichier avec votre lecteur audio par défaut.
`)
	
	openBtn := widget.NewButtonWithIcon("Ouvrir avec le lecteur par défaut", theme.MediaPlayIcon(), func() {
		OpenWithDefaultApp(filePath)
	})
	openBtn.Importance = widget.HighImportance
	
	return container.NewVBox(
		layout.NewSpacer(),
		container.NewCenter(icon),
		container.NewCenter(nameLabel),
		container.NewCenter(sizeLabel),
		widget.NewSeparator(),
		infoText,
		container.NewCenter(openBtn),
		layout.NewSpacer(),
	)
}

// createVideoPreview crée une prévisualisation vidéo (basique sans lecture)
func (pp *PreviewPanel) createVideoPreview(filePath string, meta *FileMetadata) fyne.CanvasObject {
	// Icône grande
	icon := canvas.NewImageFromResource(theme.MediaVideoIcon())
	icon.SetMinSize(fyne.NewSize(128, 128))
	icon.FillMode = canvas.ImageFillContain
	
	nameLabel := widget.NewLabelWithStyle(meta.Name, fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	
	// Message explicatif
	infoText := widget.NewRichTextFromMarkdown(`
### Lecture vidéo non disponible

La lecture vidéo n'est pas supportée dans cette version.

**Informations:**
- Format: ` + strings.ToUpper(strings.TrimPrefix(meta.Extension, ".")) + `
- Taille: ` + meta.SizeHuman + `

Utilisez le bouton **Ouvrir** pour regarder cette vidéo avec votre lecteur par défaut.
`)
	
	openBtn := widget.NewButtonWithIcon("Ouvrir avec le lecteur par défaut", theme.MediaPlayIcon(), func() {
		OpenWithDefaultApp(filePath)
	})
	openBtn.Importance = widget.HighImportance
	
	return container.NewVBox(
		layout.NewSpacer(),
		container.NewCenter(icon),
		container.NewCenter(nameLabel),
		widget.NewSeparator(),
		infoText,
		container.NewCenter(openBtn),
		layout.NewSpacer(),
	)
}

// createPDFPreview crée une prévisualisation PDF (basique)
func (pp *PreviewPanel) createPDFPreview(filePath string, meta *FileMetadata) fyne.CanvasObject {
	// Icône grande
	icon := canvas.NewImageFromResource(theme.DocumentIcon())
	icon.SetMinSize(fyne.NewSize(128, 128))
	icon.FillMode = canvas.ImageFillContain
	
	nameLabel := widget.NewLabelWithStyle(meta.Name, fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	
	// Message explicatif
	infoText := widget.NewRichTextFromMarkdown(`
### Aperçu PDF non disponible

L'aperçu des fichiers PDF n'est pas supporté dans cette version.

**Informations:**
- Taille: ` + meta.SizeHuman + `

Utilisez le bouton **Ouvrir** pour visualiser ce PDF avec votre lecteur par défaut.
`)
	
	openBtn := widget.NewButtonWithIcon("Ouvrir avec le lecteur PDF", theme.DocumentIcon(), func() {
		OpenWithDefaultApp(filePath)
	})
	openBtn.Importance = widget.HighImportance
	
	return container.NewVBox(
		layout.NewSpacer(),
		container.NewCenter(icon),
		container.NewCenter(nameLabel),
		widget.NewSeparator(),
		infoText,
		container.NewCenter(openBtn),
		layout.NewSpacer(),
	)
}

// createGenericPreview crée une prévisualisation générique
func (pp *PreviewPanel) createGenericPreview(filePath string, meta *FileMetadata) fyne.CanvasObject {
	icon := canvas.NewImageFromResource(GetFileIconResource(meta.Name))
	icon.SetMinSize(fyne.NewSize(96, 96))
	icon.FillMode = canvas.ImageFillContain
	
	nameLabel := widget.NewLabelWithStyle(meta.Name, fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	
	infoText := widget.NewLabel(fmt.Sprintf(`
Type de fichier non reconnu pour la prévisualisation.

Extension: %s
Taille: %s
Modifié: %s

Utilisez le bouton "Ouvrir" pour ouvrir ce fichier avec l'application par défaut.
`, meta.Extension, meta.SizeHuman, meta.ModTimeStr))
	
	openBtn := widget.NewButtonWithIcon("Ouvrir avec l'application par défaut", theme.ComputerIcon(), func() {
		OpenWithDefaultApp(filePath)
	})
	
	return container.NewVBox(
		layout.NewSpacer(),
		container.NewCenter(icon),
		container.NewCenter(nameLabel),
		widget.NewSeparator(),
		infoText,
		container.NewCenter(openBtn),
		layout.NewSpacer(),
	)
}

// createMetadataPanel crée le panneau de métadonnées
func (pp *PreviewPanel) createMetadataPanel(meta *FileMetadata) fyne.CanvasObject {
	title := widget.NewLabelWithStyle("📋 Métadonnées", fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	
	items := []fyne.CanvasObject{
		title,
		widget.NewSeparator(),
		pp.createMetaRow("Nom", meta.Name),
		pp.createMetaRow("Taille", meta.SizeHuman),
		pp.createMetaRow("Type", GetPreviewTypeName(meta.PreviewType)),
		pp.createMetaRow("MIME", meta.MimeType),
		pp.createMetaRow("Modifié", meta.ModTimeStr),
		pp.createMetaRow("Permissions", meta.Permissions),
	}
	
	// Ajouter les infos spécifiques au type
	if meta.PreviewType == PreviewTypeImage && meta.ImageWidth > 0 {
		items = append(items, pp.createMetaRow("Dimensions", fmt.Sprintf("%dx%d", meta.ImageWidth, meta.ImageHeight)))
	}
	
	if meta.LineCount > 0 {
		items = append(items, pp.createMetaRow("Lignes", fmt.Sprintf("%d", meta.LineCount)))
		items = append(items, pp.createMetaRow("Mots", fmt.Sprintf("%d", meta.WordCount)))
	}
	
	if meta.ArchiveFiles > 0 {
		items = append(items, pp.createMetaRow("Fichiers", fmt.Sprintf("%d", meta.ArchiveFiles)))
	}
	
	panel := container.NewVBox(items...)
	
	// Wrapper avec largeur fixe
	wrapper := container.NewVBox(panel)
	wrapper.Resize(fyne.NewSize(200, 0))
	
	return container.NewPadded(wrapper)
}

func (pp *PreviewPanel) createMetaRow(label, value string) fyne.CanvasObject {
	labelWidget := widget.NewLabelWithStyle(label+":", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	valueWidget := widget.NewLabel(value)
	valueWidget.Wrapping = fyne.TextWrapWord
	
	return container.NewVBox(labelWidget, valueWidget)
}

func (pp *PreviewPanel) showError(message string) *fyne.Container {
	errorLabel := widget.NewLabelWithStyle(message, fyne.TextAlignCenter, fyne.TextStyle{})
	
	backBtn := widget.NewButtonWithIcon("Retour", theme.NavigateBackIcon(), func() {
		if pp.backCallback != nil {
			pp.backCallback()
		}
	})
	
	return container.NewBorder(
		container.NewHBox(backBtn),
		nil,
		nil,
		nil,
		container.NewCenter(errorLabel),
	)
}

// OpenWithDefaultApp ouvre un fichier avec l'application par défaut du système
func OpenWithDefaultApp(filePath string) error {
	var cmd *exec.Cmd
	
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", "", filePath)
	case "darwin":
		cmd = exec.Command("open", filePath)
	default: // Linux et autres Unix
		cmd = exec.Command("xdg-open", filePath)
	}
	
	return cmd.Start()
}

// GetFileIconResource retourne l'icône appropriée pour un fichier
func GetFileIconResource(filename string) fyne.Resource {
	ext := strings.ToLower(filepath.Ext(filename))
	
	if imageExtensions[ext] {
		return theme.MediaPhotoIcon()
	}
	if audioExtensions[ext] {
		return theme.MediaMusicIcon()
	}
	if videoExtensions[ext] {
		return theme.MediaVideoIcon()
	}
	if archiveExtensions[ext] {
		return theme.FolderIcon() // Utiliser folder pour les archives
	}
	if codeExtensions[ext] || textExtensions[ext] || markdownExtensions[ext] {
		return theme.DocumentIcon()
	}
	if pdfExtensions[ext] {
		return theme.DocumentIcon()
	}
	
	return theme.FileIcon()
}
