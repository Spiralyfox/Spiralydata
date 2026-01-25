package main

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Types de fichiers supportés pour la prévisualisation
type PreviewType int

const (
	PreviewTypeUnknown PreviewType = iota
	PreviewTypeImage
	PreviewTypeText
	PreviewTypeCode
	PreviewTypeMarkdown
	PreviewTypeArchive
	PreviewTypeAudio
	PreviewTypeVideo
	PreviewTypePDF
)

// Extensions par type
var imageExtensions = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true, ".gif": true,
	".bmp": true, ".webp": true, ".ico": true, ".tiff": true,
}

var textExtensions = map[string]bool{
	".txt": true, ".log": true, ".ini": true, ".cfg": true,
	".conf": true, ".properties": true, ".env": true,
}

var codeExtensions = map[string]bool{
	".go": true, ".py": true, ".js": true, ".ts": true,
	".java": true, ".c": true, ".cpp": true, ".h": true,
	".hpp": true, ".cs": true, ".rb": true, ".php": true,
	".swift": true, ".kt": true, ".rs": true, ".lua": true,
	".sh": true, ".bash": true, ".zsh": true, ".fish": true,
	".ps1": true, ".bat": true, ".cmd": true,
	".html": true, ".htm": true, ".css": true, ".scss": true,
	".less": true, ".sass": true, ".xml": true, ".xsl": true,
	".json": true, ".yaml": true, ".yml": true, ".toml": true,
	".sql": true, ".graphql": true, ".proto": true,
	".dockerfile": true, ".makefile": true, ".cmake": true,
	".gradle": true, ".maven": true, ".mod": true, ".sum": true,
}

var markdownExtensions = map[string]bool{
	".md": true, ".markdown": true, ".mdown": true, ".mkd": true,
}

var archiveExtensions = map[string]bool{
	".zip": true, ".tar": true, ".gz": true, ".tgz": true,
	".tar.gz": true, ".rar": true, ".7z": true, ".bz2": true,
}

var audioExtensions = map[string]bool{
	".mp3": true, ".wav": true, ".flac": true, ".ogg": true,
	".aac": true, ".m4a": true, ".wma": true, ".aiff": true,
}

var videoExtensions = map[string]bool{
	".mp4": true, ".avi": true, ".mkv": true, ".mov": true,
	".webm": true, ".wmv": true, ".flv": true, ".m4v": true,
}

var pdfExtensions = map[string]bool{
	".pdf": true,
}

// FileMetadata contient les métadonnées d'un fichier
type FileMetadata struct {
	Name         string
	Path         string
	Size         int64
	SizeHuman    string
	ModTime      time.Time
	ModTimeStr   string
	IsDir        bool
	Extension    string
	PreviewType  PreviewType
	Permissions  string
	MimeType     string
	LineCount    int
	WordCount    int
	CharCount    int
	ImageWidth   int
	ImageHeight  int
	Duration     string
	Bitrate      string
	SampleRate   string
	ArchiveFiles int
}

// ArchiveEntry représente un élément dans une archive
type ArchiveEntry struct {
	Name       string
	Path       string
	Size       int64
	SizeHuman  string
	IsDir      bool
	ModTime    time.Time
	Compressed int64
}

// GetPreviewType détermine le type de prévisualisation pour un fichier
func GetPreviewType(filename string) PreviewType {
	ext := strings.ToLower(filepath.Ext(filename))
	
	// Cas spécial pour .tar.gz
	if strings.HasSuffix(strings.ToLower(filename), ".tar.gz") {
		return PreviewTypeArchive
	}
	
	if imageExtensions[ext] {
		return PreviewTypeImage
	}
	if textExtensions[ext] {
		return PreviewTypeText
	}
	if codeExtensions[ext] {
		return PreviewTypeCode
	}
	if markdownExtensions[ext] {
		return PreviewTypeMarkdown
	}
	if archiveExtensions[ext] {
		return PreviewTypeArchive
	}
	if audioExtensions[ext] {
		return PreviewTypeAudio
	}
	if videoExtensions[ext] {
		return PreviewTypeVideo
	}
	if pdfExtensions[ext] {
		return PreviewTypePDF
	}
	
	return PreviewTypeUnknown
}

// GetPreviewTypeName retourne le nom du type de prévisualisation
func GetPreviewTypeName(pt PreviewType) string {
	switch pt {
	case PreviewTypeImage:
		return "Image"
	case PreviewTypeText:
		return "Texte"
	case PreviewTypeCode:
		return "Code source"
	case PreviewTypeMarkdown:
		return "Markdown"
	case PreviewTypeArchive:
		return "Archive"
	case PreviewTypeAudio:
		return "Audio"
	case PreviewTypeVideo:
		return "Vidéo"
	case PreviewTypePDF:
		return "PDF"
	default:
		return "Inconnu"
	}
}

// GetFileMetadata récupère les métadonnées d'un fichier
func GetFileMetadata(filePath string) (*FileMetadata, error) {
	info, err := os.Stat(filePath)
	if err != nil {
		return nil, err
	}
	
	meta := &FileMetadata{
		Name:        info.Name(),
		Path:        filePath,
		Size:        info.Size(),
		SizeHuman:   FormatFileSize(info.Size()),
		ModTime:     info.ModTime(),
		ModTimeStr:  info.ModTime().Format("02/01/2006 15:04:05"),
		IsDir:       info.IsDir(),
		Extension:   strings.ToLower(filepath.Ext(info.Name())),
		PreviewType: GetPreviewType(info.Name()),
		Permissions: info.Mode().String(),
	}
	
	// Déterminer le type MIME basique
	meta.MimeType = GetBasicMimeType(meta.Extension)
	
	// Pour les fichiers texte/code, compter les lignes
	if meta.PreviewType == PreviewTypeText || meta.PreviewType == PreviewTypeCode || meta.PreviewType == PreviewTypeMarkdown {
		lines, words, chars := CountTextStats(filePath)
		meta.LineCount = lines
		meta.WordCount = words
		meta.CharCount = chars
	}
	
	// Pour les archives, compter les fichiers
	if meta.PreviewType == PreviewTypeArchive {
		entries, _ := ListArchiveContents(filePath)
		meta.ArchiveFiles = len(entries)
	}
	
	return meta, nil
}

// GetBasicMimeType retourne un type MIME basique
func GetBasicMimeType(ext string) string {
	mimeTypes := map[string]string{
		".jpg":  "image/jpeg",
		".jpeg": "image/jpeg",
		".png":  "image/png",
		".gif":  "image/gif",
		".bmp":  "image/bmp",
		".webp": "image/webp",
		".svg":  "image/svg+xml",
		".ico":  "image/x-icon",
		".txt":  "text/plain",
		".html": "text/html",
		".css":  "text/css",
		".js":   "application/javascript",
		".json": "application/json",
		".xml":  "application/xml",
		".pdf":  "application/pdf",
		".zip":  "application/zip",
		".tar":  "application/x-tar",
		".gz":   "application/gzip",
		".mp3":  "audio/mpeg",
		".wav":  "audio/wav",
		".ogg":  "audio/ogg",
		".mp4":  "video/mp4",
		".webm": "video/webm",
		".avi":  "video/x-msvideo",
	}
	
	if mime, ok := mimeTypes[ext]; ok {
		return mime
	}
	return "application/octet-stream"
}

// CountTextStats compte les lignes, mots et caractères d'un fichier texte
func CountTextStats(filePath string) (lines, words, chars int) {
	file, err := os.Open(filePath)
	if err != nil {
		return 0, 0, 0
	}
	defer file.Close()
	
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines++
		line := scanner.Text()
		chars += len(line) + 1 // +1 pour le newline
		words += len(strings.Fields(line))
	}
	
	return lines, words, chars
}

// ReadTextFile lit un fichier texte et retourne son contenu
func ReadTextFile(filePath string, maxLines int) ([]string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	
	var lines []string
	scanner := bufio.NewScanner(file)
	
	// Augmenter le buffer pour les longues lignes
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)
	
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		if maxLines > 0 && lineNum > maxLines {
			lines = append(lines, fmt.Sprintf("... (%d lignes supplémentaires)", lineNum-maxLines))
			break
		}
		lines = append(lines, scanner.Text())
	}
	
	return lines, scanner.Err()
}

// ListArchiveContents liste le contenu d'une archive
func ListArchiveContents(filePath string) ([]ArchiveEntry, error) {
	ext := strings.ToLower(filepath.Ext(filePath))
	
	// Cas spécial pour .tar.gz
	if strings.HasSuffix(strings.ToLower(filePath), ".tar.gz") || ext == ".tgz" {
		return listTarGzContents(filePath)
	}
	
	switch ext {
	case ".zip":
		return listZipContents(filePath)
	case ".tar":
		return listTarContents(filePath)
	case ".gz":
		return listGzContents(filePath)
	default:
		return nil, fmt.Errorf("format d'archive non supporté: %s", ext)
	}
}

func listZipContents(filePath string) ([]ArchiveEntry, error) {
	reader, err := zip.OpenReader(filePath)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	
	var entries []ArchiveEntry
	for _, file := range reader.File {
		entry := ArchiveEntry{
			Name:       filepath.Base(file.Name),
			Path:       file.Name,
			Size:       int64(file.UncompressedSize64),
			SizeHuman:  FormatFileSize(int64(file.UncompressedSize64)),
			IsDir:      file.FileInfo().IsDir(),
			ModTime:    file.Modified,
			Compressed: int64(file.CompressedSize64),
		}
		entries = append(entries, entry)
	}
	
	// Trier par chemin
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Path < entries[j].Path
	})
	
	return entries, nil
}

func listTarContents(filePath string) ([]ArchiveEntry, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	
	return readTarEntries(tar.NewReader(file))
}

func listTarGzContents(filePath string) ([]ArchiveEntry, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	
	gzReader, err := gzip.NewReader(file)
	if err != nil {
		return nil, err
	}
	defer gzReader.Close()
	
	return readTarEntries(tar.NewReader(gzReader))
}

func listGzContents(filePath string) ([]ArchiveEntry, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	
	gzReader, err := gzip.NewReader(file)
	if err != nil {
		return nil, err
	}
	defer gzReader.Close()
	
	// Pour un fichier .gz simple, on retourne juste le nom du fichier décompressé
	name := gzReader.Name
	if name == "" {
		name = strings.TrimSuffix(filepath.Base(filePath), ".gz")
	}
	
	// Lire la taille décompressée
	data, err := io.ReadAll(gzReader)
	if err != nil {
		return nil, err
	}
	
	return []ArchiveEntry{
		{
			Name:      name,
			Path:      name,
			Size:      int64(len(data)),
			SizeHuman: FormatFileSize(int64(len(data))),
			IsDir:     false,
		},
	}, nil
}

func readTarEntries(tarReader *tar.Reader) ([]ArchiveEntry, error) {
	var entries []ArchiveEntry
	
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		
		entry := ArchiveEntry{
			Name:      filepath.Base(header.Name),
			Path:      header.Name,
			Size:      header.Size,
			SizeHuman: FormatFileSize(header.Size),
			IsDir:     header.Typeflag == tar.TypeDir,
			ModTime:   header.ModTime,
		}
		entries = append(entries, entry)
	}
	
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Path < entries[j].Path
	})
	
	return entries, nil
}

// GetLanguageFromExtension retourne le langage pour la coloration syntaxique
func GetLanguageFromExtension(ext string) string {
	languages := map[string]string{
		".go":         "Go",
		".py":         "Python",
		".js":         "JavaScript",
		".ts":         "TypeScript",
		".java":       "Java",
		".c":          "C",
		".cpp":        "C++",
		".h":          "C/C++ Header",
		".hpp":        "C++ Header",
		".cs":         "C#",
		".rb":         "Ruby",
		".php":        "PHP",
		".swift":      "Swift",
		".kt":         "Kotlin",
		".rs":         "Rust",
		".lua":        "Lua",
		".sh":         "Shell",
		".bash":       "Bash",
		".ps1":        "PowerShell",
		".bat":        "Batch",
		".html":       "HTML",
		".htm":        "HTML",
		".css":        "CSS",
		".scss":       "SCSS",
		".less":       "Less",
		".xml":        "XML",
		".json":       "JSON",
		".yaml":       "YAML",
		".yml":        "YAML",
		".toml":       "TOML",
		".sql":        "SQL",
		".md":         "Markdown",
		".dockerfile": "Dockerfile",
		".makefile":   "Makefile",
	}
	
	if lang, ok := languages[strings.ToLower(ext)]; ok {
		return lang
	}
	return "Texte"
}

// CanPreview vérifie si un fichier peut être prévisualisé
func CanPreview(filename string) bool {
	pt := GetPreviewType(filename)
	// On peut prévisualiser tous les types sauf Unknown
	return pt != PreviewTypeUnknown
}

// CanPreviewLocally vérifie si un fichier peut être prévisualisé localement (sans téléchargement)
func CanPreviewLocally(filename string) bool {
	pt := GetPreviewType(filename)
	// Seuls les images, texte, code, markdown et archives peuvent être prévisualisés
	switch pt {
	case PreviewTypeImage, PreviewTypeText, PreviewTypeCode, PreviewTypeMarkdown, PreviewTypeArchive:
		return true
	default:
		return false
	}
}

// IsMediaFile vérifie si un fichier est un média (audio/vidéo)
func IsMediaFile(filename string) bool {
	pt := GetPreviewType(filename)
	return pt == PreviewTypeAudio || pt == PreviewTypeVideo
}

// IsBinaryFile vérifie si un fichier est probablement binaire
func IsBinaryFile(filename string) bool {
	pt := GetPreviewType(filename)
	switch pt {
	case PreviewTypeImage, PreviewTypeAudio, PreviewTypeVideo, PreviewTypePDF, PreviewTypeArchive:
		return true
	default:
		return false
	}
}
