package main

type FileChange struct {
	FileName string `json:"filename"`
	Op       string `json:"op"`
	Content  string `json:"content,omitempty"`
	Origin   string `json:"origin"`
	IsDir    bool   `json:"is_dir"`
}

type AuthRequest struct {
	Type   string `json:"type"`
	HostID string `json:"host_id"`
}

type AuthResponse struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

type FileTreeItemMessage struct {
	Type  string `json:"type"`
	Path  string `json:"path"`
	Name  string `json:"name"`
	IsDir bool   `json:"is_dir"`
}

type DownloadRequest struct {
	Type  string   `json:"type"`
	Items []string `json:"items"`
} 