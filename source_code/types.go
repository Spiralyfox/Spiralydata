package main

type FileChange struct {
	FileName string `json:"filename"`
	Op       string `json:"op"`                // "create", "write", "remove", "mkdir", "rmdir"
	Content  string `json:"content,omitempty"` // base64
	Origin   string `json:"origin"`            // "server" ou "client"
	IsDir    bool   `json:"is_dir"`            // true si c'est un dossier
}

type AuthRequest struct {
	Type   string `json:"type"` // "auth_request"
	HostID string `json:"host_id"`
}

type AuthResponse struct {
	Type    string `json:"type"`    // "auth_success" ou "auth_failed"
	Message string `json:"message"`
}