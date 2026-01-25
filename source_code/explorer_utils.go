package main

import (
	"sort"
	"strings"
	"time"
)

// SortField représente un champ de tri
type SortField int

const (
	SortByName SortField = iota
	SortBySize
	SortByDate
	SortByType
)

// SortOrder représente l'ordre de tri
type SortOrder int

const (
	SortAscending SortOrder = iota
	SortDescending
)

// ViewMode représente le mode d'affichage
type ViewMode int

const (
	ViewList ViewMode = iota
	ViewGrid
	ViewTree
)

// ExplorerSettings contient les paramètres de l'explorateur
type ExplorerSettings struct {
	SortField     SortField
	SortOrder     SortOrder
	ViewMode      ViewMode
	ShowHidden    bool
	ShowMetadata  bool
	Favorites     []string
	RecentFolders []string
}

// FileItemSort représente un élément triable
type FileItemSort struct {
	Name    string
	Path    string
	Size    int64
	ModTime time.Time
	IsDir   bool
	Ext     string
}

// NewExplorerSettings crée des paramètres par défaut
func NewExplorerSettings() *ExplorerSettings {
	return &ExplorerSettings{
		SortField:     SortByName,
		SortOrder:     SortAscending,
		ViewMode:      ViewList,
		ShowHidden:    false,
		ShowMetadata:  true,
		Favorites:     []string{},
		RecentFolders: []string{},
	}
}

// SortItems trie les éléments selon les paramètres
func (es *ExplorerSettings) SortItems(items []*FileTreeItem) []*FileTreeItem {
	if items == nil || len(items) == 0 {
		return []*FileTreeItem{}
	}

	// Filtrer les éléments nil
	validItems := make([]*FileTreeItem, 0, len(items))
	for _, item := range items {
		if item != nil {
			validItems = append(validItems, item)
		}
	}

	if len(validItems) == 0 {
		return []*FileTreeItem{}
	}

	// Créer une copie pour le tri
	sorted := make([]*FileTreeItem, len(validItems))
	copy(sorted, validItems)

	// Fonction de comparaison
	less := func(i, j int) bool {
		a, b := sorted[i], sorted[j]
		if a == nil || b == nil {
			return false
		}

		// Toujours mettre les dossiers en premier
		if a.IsDir != b.IsDir {
			return a.IsDir
		}

		var result bool

		switch es.SortField {
		case SortByName:
			result = strings.ToLower(a.Name) < strings.ToLower(b.Name)

		case SortBySize:
			result = a.Size < b.Size

		case SortByDate:
			result = a.ModTime.Before(b.ModTime)

		case SortByType:
			extA := strings.ToLower(getExtension(a.Name))
			extB := strings.ToLower(getExtension(b.Name))
			if extA == extB {
				result = strings.ToLower(a.Name) < strings.ToLower(b.Name)
			} else {
				result = extA < extB
			}

		default:
			result = strings.ToLower(a.Name) < strings.ToLower(b.Name)
		}

		// Inverser si ordre descendant
		if es.SortOrder == SortDescending {
			return !result
		}
		return result
	}

	sort.Slice(sorted, less)
	return sorted
}

// getExtension retourne l'extension d'un fichier
func getExtension(name string) string {
	idx := strings.LastIndex(name, ".")
	if idx == -1 || idx == len(name)-1 {
		return ""
	}
	return name[idx+1:]
}

// AddFavorite ajoute un favori
func (es *ExplorerSettings) AddFavorite(path string) {
	// Vérifier si déjà présent
	for _, f := range es.Favorites {
		if f == path {
			return
		}
	}
	es.Favorites = append(es.Favorites, path)

	// Limiter à 20 favoris
	if len(es.Favorites) > 20 {
		es.Favorites = es.Favorites[1:]
	}
}

// RemoveFavorite supprime un favori
func (es *ExplorerSettings) RemoveFavorite(path string) {
	newFavs := []string{}
	for _, f := range es.Favorites {
		if f != path {
			newFavs = append(newFavs, f)
		}
	}
	es.Favorites = newFavs
}

// IsFavorite vérifie si un chemin est en favori
func (es *ExplorerSettings) IsFavorite(path string) bool {
	for _, f := range es.Favorites {
		if f == path {
			return true
		}
	}
	return false
}

// AddRecentFolder ajoute un dossier récent
func (es *ExplorerSettings) AddRecentFolder(path string) {
	// Retirer s'il existe déjà pour le mettre en premier
	newRecent := []string{path}
	for _, f := range es.RecentFolders {
		if f != path {
			newRecent = append(newRecent, f)
		}
	}
	es.RecentFolders = newRecent

	// Limiter à 10 récents
	if len(es.RecentFolders) > 10 {
		es.RecentFolders = es.RecentFolders[:10]
	}
}

// ToggleSortOrder inverse l'ordre de tri
func (es *ExplorerSettings) ToggleSortOrder() {
	if es.SortOrder == SortAscending {
		es.SortOrder = SortDescending
	} else {
		es.SortOrder = SortAscending
	}
}

// GetSortFieldName retourne le nom du champ de tri
func (es *ExplorerSettings) GetSortFieldName() string {
	switch es.SortField {
	case SortByName:
		return "Nom"
	case SortBySize:
		return "Taille"
	case SortByDate:
		return "Date"
	case SortByType:
		return "Type"
	default:
		return "Nom"
	}
}

// GetSortOrderIcon retourne l'icône de l'ordre de tri
func (es *ExplorerSettings) GetSortOrderIcon() string {
	if es.SortOrder == SortAscending {
		return "↑"
	}
	return "↓"
}

// GetViewModeName retourne le nom du mode de vue
func (es *ExplorerSettings) GetViewModeName() string {
	switch es.ViewMode {
	case ViewList:
		return "Liste"
	case ViewGrid:
		return "Grille"
	case ViewTree:
		return "Arbre"
	default:
		return "Liste"
	}
}

// GetViewModeIcon retourne l'icône du mode de vue
func (es *ExplorerSettings) GetViewModeIcon() string {
	switch es.ViewMode {
	case ViewList:
		return "📋"
	case ViewGrid:
		return "⊞"
	case ViewTree:
		return "🌳"
	default:
		return "📋"
	}
}

// FilterItems filtre les éléments selon une requête
func FilterItems(items []*FileTreeItem, query string) []*FileTreeItem {
	if query == "" {
		return items
	}

	query = strings.ToLower(query)
	var filtered []*FileTreeItem

	for _, item := range items {
		if strings.Contains(strings.ToLower(item.Name), query) {
			filtered = append(filtered, item)
		}
	}

	return filtered
}

// GetFileTypeCategory retourne la catégorie d'un fichier
func GetFileTypeCategory(name string) string {
	ext := strings.ToLower(getExtension(name))

	categories := map[string][]string{
		"📷 Images":    {"jpg", "jpeg", "png", "gif", "bmp", "webp", "svg", "ico", "tiff"},
		"🎬 Vidéos":    {"mp4", "avi", "mkv", "mov", "wmv", "flv", "webm", "m4v"},
		"🎵 Audio":     {"mp3", "wav", "flac", "aac", "ogg", "wma", "m4a"},
		"📄 Documents": {"pdf", "doc", "docx", "xls", "xlsx", "ppt", "pptx", "odt", "ods"},
		"📝 Texte":     {"txt", "md", "log", "csv", "json", "xml", "yaml", "yml", "ini", "conf"},
		"💻 Code":      {"go", "py", "js", "ts", "java", "c", "cpp", "h", "cs", "php", "rb", "rs", "html", "css"},
		"📦 Archives":  {"zip", "tar", "gz", "rar", "7z", "bz2", "xz"},
		"⚙️ Exécutables": {"exe", "msi", "app", "dmg", "deb", "rpm", "sh", "bat"},
	}

	for category, extensions := range categories {
		for _, e := range extensions {
			if ext == e {
				return category
			}
		}
	}

	return "📄 Autre"
}

// GroupItemsByType groupe les éléments par type
func GroupItemsByType(items []*FileTreeItem) map[string][]*FileTreeItem {
	groups := make(map[string][]*FileTreeItem)

	// Toujours avoir le groupe "Dossiers" en premier
	groups["📁 Dossiers"] = []*FileTreeItem{}

	for _, item := range items {
		if item.IsDir {
			groups["📁 Dossiers"] = append(groups["📁 Dossiers"], item)
		} else {
			category := GetFileTypeCategory(item.Name)
			if groups[category] == nil {
				groups[category] = []*FileTreeItem{}
			}
			groups[category] = append(groups[category], item)
		}
	}

	return groups
}

// CountItemsByType compte les éléments par type
func CountItemsByType(items []*FileTreeItem) map[string]int {
	counts := make(map[string]int)

	for _, item := range items {
		if item.IsDir {
			counts["📁 Dossiers"]++
		} else {
			category := GetFileTypeCategory(item.Name)
			counts[category]++
		}
	}

	return counts
}

// CalculateTotalSize calcule la taille totale des fichiers
func CalculateTotalSize(items []*FileTreeItem) int64 {
	var total int64
	for _, item := range items {
		if !item.IsDir {
			total += item.Size
		}
	}
	return total
}
