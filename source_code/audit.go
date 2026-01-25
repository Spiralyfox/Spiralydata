package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// ============================================================================
// 7.5 AUDIT & COMPLIANCE
// ============================================================================

// AuditEventType type d'événement d'audit
type AuditEventType string

const (
	AuditLogin          AuditEventType = "LOGIN"
	AuditLogout         AuditEventType = "LOGOUT"
	AuditLoginFailed    AuditEventType = "LOGIN_FAILED"
	AuditFileRead       AuditEventType = "FILE_READ"
	AuditFileWrite      AuditEventType = "FILE_WRITE"
	AuditFileDelete     AuditEventType = "FILE_DELETE"
	AuditFileCreate     AuditEventType = "FILE_CREATE"
	AuditDirCreate      AuditEventType = "DIR_CREATE"
	AuditDirDelete      AuditEventType = "DIR_DELETE"
	AuditConfigChange   AuditEventType = "CONFIG_CHANGE"
	AuditPermChange     AuditEventType = "PERM_CHANGE"
	AuditUserCreate     AuditEventType = "USER_CREATE"
	AuditUserDelete     AuditEventType = "USER_DELETE"
	AuditUserModify     AuditEventType = "USER_MODIFY"
	AuditSessionStart   AuditEventType = "SESSION_START"
	AuditSessionEnd     AuditEventType = "SESSION_END"
	AuditSync           AuditEventType = "SYNC"
	AuditError          AuditEventType = "ERROR"
	AuditSecurityAlert  AuditEventType = "SECURITY_ALERT"
	AuditAccessDenied   AuditEventType = "ACCESS_DENIED"
	AuditRateLimited    AuditEventType = "RATE_LIMITED"
	AuditIPBlocked      AuditEventType = "IP_BLOCKED"
)

// AuditSeverity niveau de sévérité
type AuditSeverity string

const (
	SeverityInfo     AuditSeverity = "INFO"
	SeverityWarning  AuditSeverity = "WARNING"
	SeverityError    AuditSeverity = "ERROR"
	SeverityCritical AuditSeverity = "CRITICAL"
)

// AuditEvent représente un événement d'audit
type AuditEvent struct {
	ID        string            `json:"id"`
	Timestamp time.Time         `json:"timestamp"`
	Type      AuditEventType    `json:"type"`
	Severity  AuditSeverity     `json:"severity"`
	UserID    string            `json:"user_id,omitempty"`
	SessionID string            `json:"session_id,omitempty"`
	ClientIP  string            `json:"client_ip,omitempty"`
	Resource  string            `json:"resource,omitempty"`
	Action    string            `json:"action,omitempty"`
	Details   map[string]string `json:"details,omitempty"`
	Success   bool              `json:"success"`
	ErrorMsg  string            `json:"error_msg,omitempty"`
}

// AuditLogger gère les logs d'audit
type AuditLogger struct {
	events       []*AuditEvent
	mu           sync.RWMutex
	maxEvents    int
	logFile      string
	writeToFile  bool
	alertFunc    func(*AuditEvent)
	eventCounter int64
}

// NewAuditLogger crée un nouveau logger d'audit
func NewAuditLogger(maxEvents int) *AuditLogger {
	return &AuditLogger{
		events:    make([]*AuditEvent, 0),
		maxEvents: maxEvents,
	}
}

// SetLogFile définit le fichier de log
func (al *AuditLogger) SetLogFile(path string) error {
	al.mu.Lock()
	defer al.mu.Unlock()
	
	// Créer le répertoire si nécessaire
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	
	al.logFile = path
	al.writeToFile = true
	return nil
}

// SetAlertCallback définit la fonction d'alerte
func (al *AuditLogger) SetAlertCallback(fn func(*AuditEvent)) {
	al.mu.Lock()
	defer al.mu.Unlock()
	al.alertFunc = fn
}

// Log enregistre un événement
func (al *AuditLogger) Log(event *AuditEvent) {
	al.mu.Lock()
	defer al.mu.Unlock()
	
	al.eventCounter++
	event.ID = fmt.Sprintf("EVT-%d-%d", time.Now().Unix(), al.eventCounter)
	event.Timestamp = time.Now()
	
	// Ajouter à la liste en mémoire
	al.events = append(al.events, event)
	
	// Éviction si nécessaire
	if len(al.events) > al.maxEvents {
		al.events = al.events[1:]
	}
	
	// Écrire dans le fichier
	if al.writeToFile && al.logFile != "" {
		al.writeEventToFile(event)
	}
	
	// Déclencher une alerte si nécessaire
	if al.alertFunc != nil && (event.Severity == SeverityCritical || event.Type == AuditSecurityAlert) {
		go al.alertFunc(event)
	}
}

// LogSimple enregistre un événement simple
func (al *AuditLogger) LogSimple(eventType AuditEventType, severity AuditSeverity, userID, resource, action string, success bool) {
	al.Log(&AuditEvent{
		Type:     eventType,
		Severity: severity,
		UserID:   userID,
		Resource: resource,
		Action:   action,
		Success:  success,
	})
}

func (al *AuditLogger) writeEventToFile(event *AuditEvent) {
	file, err := os.OpenFile(al.logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return
	}
	defer file.Close()
	
	data, err := json.Marshal(event)
	if err != nil {
		return
	}
	
	file.WriteString(string(data) + "\n")
}

// GetEvents retourne les événements récents
func (al *AuditLogger) GetEvents(limit int) []*AuditEvent {
	al.mu.RLock()
	defer al.mu.RUnlock()
	
	if limit <= 0 || limit > len(al.events) {
		limit = len(al.events)
	}
	
	// Retourner les plus récents
	start := len(al.events) - limit
	result := make([]*AuditEvent, limit)
	copy(result, al.events[start:])
	
	return result
}

// GetEventsByType retourne les événements d'un type spécifique
func (al *AuditLogger) GetEventsByType(eventType AuditEventType, limit int) []*AuditEvent {
	al.mu.RLock()
	defer al.mu.RUnlock()
	
	var result []*AuditEvent
	
	for i := len(al.events) - 1; i >= 0 && len(result) < limit; i-- {
		if al.events[i].Type == eventType {
			result = append(result, al.events[i])
		}
	}
	
	return result
}

// GetEventsByUser retourne les événements d'un utilisateur
func (al *AuditLogger) GetEventsByUser(userID string, limit int) []*AuditEvent {
	al.mu.RLock()
	defer al.mu.RUnlock()
	
	var result []*AuditEvent
	
	for i := len(al.events) - 1; i >= 0 && len(result) < limit; i-- {
		if al.events[i].UserID == userID {
			result = append(result, al.events[i])
		}
	}
	
	return result
}

// GetSecurityAlerts retourne les alertes de sécurité
func (al *AuditLogger) GetSecurityAlerts(limit int) []*AuditEvent {
	al.mu.RLock()
	defer al.mu.RUnlock()
	
	var result []*AuditEvent
	
	for i := len(al.events) - 1; i >= 0 && len(result) < limit; i-- {
		event := al.events[i]
		if event.Type == AuditSecurityAlert || 
		   event.Type == AuditAccessDenied ||
		   event.Type == AuditIPBlocked ||
		   event.Type == AuditLoginFailed ||
		   event.Severity == SeverityCritical {
			result = append(result, event)
		}
	}
	
	return result
}

// ExportToJSON exporte les logs en JSON
func (al *AuditLogger) ExportToJSON(path string) error {
	al.mu.RLock()
	defer al.mu.RUnlock()
	
	data, err := json.MarshalIndent(al.events, "", "  ")
	if err != nil {
		return err
	}
	
	return os.WriteFile(path, data, 0600)
}

// ExportToCSV exporte les logs en CSV
func (al *AuditLogger) ExportToCSV(path string) error {
	al.mu.RLock()
	defer al.mu.RUnlock()
	
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	
	// En-tête
	file.WriteString("ID,Timestamp,Type,Severity,UserID,ClientIP,Resource,Action,Success,Error\n")
	
	for _, event := range al.events {
		line := fmt.Sprintf("%s,%s,%s,%s,%s,%s,%s,%s,%v,%s\n",
			event.ID,
			event.Timestamp.Format(time.RFC3339),
			event.Type,
			event.Severity,
			event.UserID,
			event.ClientIP,
			event.Resource,
			event.Action,
			event.Success,
			event.ErrorMsg,
		)
		file.WriteString(line)
	}
	
	return nil
}

// Clear efface tous les événements (après export)
func (al *AuditLogger) Clear() {
	al.mu.Lock()
	defer al.mu.Unlock()
	al.events = make([]*AuditEvent, 0)
}

// GetStatistics retourne des statistiques sur les événements
func (al *AuditLogger) GetStatistics() map[string]int {
	al.mu.RLock()
	defer al.mu.RUnlock()
	
	stats := make(map[string]int)
	
	for _, event := range al.events {
		stats[string(event.Type)]++
		stats["severity_"+string(event.Severity)]++
		if event.Success {
			stats["success"]++
		} else {
			stats["failure"]++
		}
	}
	
	stats["total"] = len(al.events)
	return stats
}

// ============================================================================
// ACTIVITY MONITOR - Surveillance en temps réel
// ============================================================================

// ActivityMonitor surveille l'activité en temps réel
type ActivityMonitor struct {
	auditLogger     *AuditLogger
	suspiciousIPs   map[string]*SuspiciousActivity
	mu              sync.RWMutex
	thresholds      ActivityThresholds
	alertCallbacks  []func(AlertInfo)
}

// SuspiciousActivity activité suspecte d'une IP
type SuspiciousActivity struct {
	IP              string
	FailedLogins    int
	RateLimitHits   int
	AccessDenied    int
	LastActivity    time.Time
	Blocked         bool
	BlockedUntil    time.Time
}

// ActivityThresholds seuils pour détection d'activité suspecte
type ActivityThresholds struct {
	MaxFailedLogins   int
	MaxRateLimitHits  int
	MaxAccessDenied   int
	WindowDuration    time.Duration
	BlockDuration     time.Duration
}

// AlertInfo information d'alerte
type AlertInfo struct {
	Type      string
	Message   string
	IP        string
	Timestamp time.Time
	Severity  AuditSeverity
}

// NewActivityMonitor crée un nouveau moniteur
func NewActivityMonitor(logger *AuditLogger) *ActivityMonitor {
	return &ActivityMonitor{
		auditLogger:   logger,
		suspiciousIPs: make(map[string]*SuspiciousActivity),
		thresholds: ActivityThresholds{
			MaxFailedLogins:  5,
			MaxRateLimitHits: 20,
			MaxAccessDenied:  10,
			WindowDuration:   5 * time.Minute,
			BlockDuration:    15 * time.Minute,
		},
	}
}

// RecordActivity enregistre une activité et vérifie si elle est suspecte
func (am *ActivityMonitor) RecordActivity(ip string, activityType AuditEventType) {
	am.mu.Lock()
	defer am.mu.Unlock()
	
	activity, exists := am.suspiciousIPs[ip]
	if !exists {
		activity = &SuspiciousActivity{
			IP:           ip,
			LastActivity: time.Now(),
		}
		am.suspiciousIPs[ip] = activity
	}
	
	// Réinitialiser si la fenêtre est expirée
	if time.Since(activity.LastActivity) > am.thresholds.WindowDuration {
		activity.FailedLogins = 0
		activity.RateLimitHits = 0
		activity.AccessDenied = 0
	}
	
	activity.LastActivity = time.Now()
	
	switch activityType {
	case AuditLoginFailed:
		activity.FailedLogins++
		if activity.FailedLogins >= am.thresholds.MaxFailedLogins {
			am.blockIP(activity, "Trop de tentatives de connexion échouées")
		}
	case AuditRateLimited:
		activity.RateLimitHits++
		if activity.RateLimitHits >= am.thresholds.MaxRateLimitHits {
			am.blockIP(activity, "Rate limit excessif")
		}
	case AuditAccessDenied:
		activity.AccessDenied++
		if activity.AccessDenied >= am.thresholds.MaxAccessDenied {
			am.blockIP(activity, "Trop d'accès refusés")
		}
	}
}

func (am *ActivityMonitor) blockIP(activity *SuspiciousActivity, reason string) {
	activity.Blocked = true
	activity.BlockedUntil = time.Now().Add(am.thresholds.BlockDuration)
	
	// Log l'événement
	am.auditLogger.Log(&AuditEvent{
		Type:     AuditIPBlocked,
		Severity: SeverityCritical,
		ClientIP: activity.IP,
		Action:   "BLOCK",
		Details:  map[string]string{"reason": reason},
		Success:  true,
	})
	
	// Déclencher les alertes
	alert := AlertInfo{
		Type:      "IP_BLOCKED",
		Message:   fmt.Sprintf("IP %s bloquée: %s", activity.IP, reason),
		IP:        activity.IP,
		Timestamp: time.Now(),
		Severity:  SeverityCritical,
	}
	
	for _, callback := range am.alertCallbacks {
		go callback(alert)
	}
	
	addLog(fmt.Sprintf("🚨 IP bloquée: %s - %s", activity.IP, reason))
}

// IsIPBlocked vérifie si une IP est bloquée
func (am *ActivityMonitor) IsIPBlocked(ip string) bool {
	am.mu.RLock()
	defer am.mu.RUnlock()
	
	activity, exists := am.suspiciousIPs[ip]
	if !exists {
		return false
	}
	
	if !activity.Blocked {
		return false
	}
	
	if time.Now().After(activity.BlockedUntil) {
		activity.Blocked = false
		return false
	}
	
	return true
}

// UnblockIP débloque une IP
func (am *ActivityMonitor) UnblockIP(ip string) {
	am.mu.Lock()
	defer am.mu.Unlock()
	
	if activity, exists := am.suspiciousIPs[ip]; exists {
		activity.Blocked = false
		activity.FailedLogins = 0
		activity.RateLimitHits = 0
		activity.AccessDenied = 0
	}
}

// GetBlockedIPs retourne les IPs bloquées
func (am *ActivityMonitor) GetBlockedIPs() []string {
	am.mu.RLock()
	defer am.mu.RUnlock()
	
	var blocked []string
	now := time.Now()
	
	for ip, activity := range am.suspiciousIPs {
		if activity.Blocked && now.Before(activity.BlockedUntil) {
			blocked = append(blocked, ip)
		}
	}
	
	return blocked
}

// AddAlertCallback ajoute un callback d'alerte
func (am *ActivityMonitor) AddAlertCallback(callback func(AlertInfo)) {
	am.mu.Lock()
	defer am.mu.Unlock()
	am.alertCallbacks = append(am.alertCallbacks, callback)
}

// ============================================================================
// DATA RETENTION - Politiques de rétention
// ============================================================================

// RetentionPolicy politique de rétention des données
type RetentionPolicy struct {
	Name            string
	MaxAge          time.Duration
	MaxSize         int64 // bytes
	ApplyToLogs     bool
	ApplyToFiles    bool
	SecureDelete    bool
	ArchiveFirst    bool
}

// RetentionManager gère les politiques de rétention
type RetentionManager struct {
	policies map[string]*RetentionPolicy
	mu       sync.RWMutex
}

// NewRetentionManager crée un gestionnaire de rétention
func NewRetentionManager() *RetentionManager {
	rm := &RetentionManager{
		policies: make(map[string]*RetentionPolicy),
	}
	
	// Politique par défaut pour les logs
	rm.AddPolicy(&RetentionPolicy{
		Name:         "default_logs",
		MaxAge:       90 * 24 * time.Hour, // 90 jours
		ApplyToLogs:  true,
		SecureDelete: false,
	})
	
	return rm
}

// AddPolicy ajoute une politique
func (rm *RetentionManager) AddPolicy(policy *RetentionPolicy) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.policies[policy.Name] = policy
}

// GetPolicy récupère une politique
func (rm *RetentionManager) GetPolicy(name string) (*RetentionPolicy, bool) {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	policy, ok := rm.policies[name]
	return policy, ok
}

// ============================================================================
// GLOBAL INSTANCES
// ============================================================================

var (
	globalAuditLogger     = NewAuditLogger(10000)
	globalActivityMonitor *ActivityMonitor
	globalRetentionMgr    = NewRetentionManager()
)

func init() {
	globalActivityMonitor = NewActivityMonitor(globalAuditLogger)
	
	// Définir le fichier de log d'audit
	execDir := getExecutableDir()
	logPath := filepath.Join(execDir, "audit.log")
	globalAuditLogger.SetLogFile(logPath)
}

// GetAuditLogger retourne le logger d'audit global
func GetAuditLogger() *AuditLogger {
	return globalAuditLogger
}

// GetActivityMonitor retourne le moniteur d'activité global
func GetActivityMonitor() *ActivityMonitor {
	return globalActivityMonitor
}

// GetRetentionManager retourne le gestionnaire de rétention global
func GetRetentionManager() *RetentionManager {
	return globalRetentionMgr
}

// ============================================================================
// AUDIT HELPERS - Fonctions helper pour l'audit
// ============================================================================

// LogLogin enregistre une connexion
func LogLogin(userID, clientIP string, success bool, errorMsg string) {
	eventType := AuditLogin
	severity := SeverityInfo
	
	if !success {
		eventType = AuditLoginFailed
		severity = SeverityWarning
		GetActivityMonitor().RecordActivity(clientIP, AuditLoginFailed)
	}
	
	GetAuditLogger().Log(&AuditEvent{
		Type:     eventType,
		Severity: severity,
		UserID:   userID,
		ClientIP: clientIP,
		Action:   "LOGIN",
		Success:  success,
		ErrorMsg: errorMsg,
	})
}

// AuditFileOperation enregistre une opération sur fichier
func AuditFileOperation(eventType AuditEventType, userID, path string, success bool) {
	GetAuditLogger().Log(&AuditEvent{
		Type:     eventType,
		Severity: SeverityInfo,
		UserID:   userID,
		Resource: path,
		Action:   string(eventType),
		Success:  success,
	})
}

// AuditSecurityEvent enregistre un événement de sécurité
func AuditSecurityEvent(message, clientIP string, severity AuditSeverity) {
	GetAuditLogger().Log(&AuditEvent{
		Type:     AuditSecurityAlert,
		Severity: severity,
		ClientIP: clientIP,
		Action:   message,
		Success:  false,
	})
}

// AuditAccessDeniedEvent enregistre un accès refusé
func AuditAccessDeniedEvent(userID, clientIP, resource, reason string) {
	GetActivityMonitor().RecordActivity(clientIP, AuditAccessDenied)
	
	GetAuditLogger().Log(&AuditEvent{
		Type:     AuditAccessDenied,
		Severity: SeverityWarning,
		UserID:   userID,
		ClientIP: clientIP,
		Resource: resource,
		Action:   "ACCESS_DENIED",
		ErrorMsg: reason,
		Success:  false,
	})
}
