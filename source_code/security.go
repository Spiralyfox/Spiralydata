package main

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"net"
	"sync"
	"time"
)

// ============================================================================
// 7.1 AUTHENTIFICATION RENFORCÉE
// ============================================================================

// AuthConfig configuration d'authentification
type AuthConfig struct {
	PasswordEnabled     bool          `json:"password_enabled"`
	PasswordHash        string        `json:"password_hash"`
	MaxLoginAttempts    int           `json:"max_login_attempts"`
	LockoutDuration     time.Duration `json:"lockout_duration"`
	SessionTimeout      time.Duration `json:"session_timeout"`
	IPWhitelistEnabled  bool          `json:"ip_whitelist_enabled"`
	IPWhitelist         []string      `json:"ip_whitelist"`
	RequireEncryption   bool          `json:"require_encryption"`
}

// NewAuthConfig crée une configuration par défaut
func NewAuthConfig() *AuthConfig {
	return &AuthConfig{
		PasswordEnabled:    false,
		MaxLoginAttempts:   5,
		LockoutDuration:    15 * time.Minute,
		SessionTimeout:     24 * time.Hour,
		IPWhitelistEnabled: false,
		IPWhitelist:        []string{},
		RequireEncryption:  false,
	}
}

// SetPassword définit le mot de passe
func (ac *AuthConfig) SetPassword(password string) {
	if password == "" {
		ac.PasswordEnabled = false
		ac.PasswordHash = ""
		return
	}
	
	ac.PasswordEnabled = true
	ac.PasswordHash = HashPassword(password)
}

// VerifyPassword vérifie le mot de passe
func (ac *AuthConfig) VerifyPassword(password string) bool {
	if !ac.PasswordEnabled {
		return true
	}
	
	hash := HashPassword(password)
	return subtle.ConstantTimeCompare([]byte(ac.PasswordHash), []byte(hash)) == 1
}

// HashPassword hash un mot de passe avec SHA256 + salt
func HashPassword(password string) string {
	// Dans une vraie application, utiliser bcrypt ou argon2
	h := sha256.New()
	h.Write([]byte("spiralydata_salt_"))
	h.Write([]byte(password))
	return hex.EncodeToString(h.Sum(nil))
}

// ============================================================================
// SESSION MANAGEMENT
// ============================================================================

// Session représente une session utilisateur
type Session struct {
	ID        string
	ClientIP  string
	CreatedAt time.Time
	ExpiresAt time.Time
	LastSeen  time.Time
	UserRole  UserRole
	IsValid   bool
}

// SessionManager gère les sessions
type SessionManager struct {
	sessions map[string]*Session
	mu       sync.RWMutex
	timeout  time.Duration
}

// NewSessionManager crée un gestionnaire de sessions
func NewSessionManager(timeout time.Duration) *SessionManager {
	sm := &SessionManager{
		sessions: make(map[string]*Session),
		timeout:  timeout,
	}
	
	// Nettoyage périodique
	go sm.cleanupLoop()
	
	return sm
}

// CreateSession crée une nouvelle session
func (sm *SessionManager) CreateSession(clientIP string, role UserRole) *Session {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	
	sessionID := GenerateSecureToken(32)
	now := time.Now()
	
	session := &Session{
		ID:        sessionID,
		ClientIP:  clientIP,
		CreatedAt: now,
		ExpiresAt: now.Add(sm.timeout),
		LastSeen:  now,
		UserRole:  role,
		IsValid:   true,
	}
	
	sm.sessions[sessionID] = session
	return session
}

// GetSession récupère une session
func (sm *SessionManager) GetSession(sessionID string) (*Session, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	
	session, ok := sm.sessions[sessionID]
	if !ok || !session.IsValid {
		return nil, false
	}
	
	if time.Now().After(session.ExpiresAt) {
		return nil, false
	}
	
	return session, true
}

// RefreshSession rafraîchit une session
func (sm *SessionManager) RefreshSession(sessionID string) bool {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	
	session, ok := sm.sessions[sessionID]
	if !ok || !session.IsValid {
		return false
	}
	
	session.LastSeen = time.Now()
	session.ExpiresAt = time.Now().Add(sm.timeout)
	return true
}

// InvalidateSession invalide une session
func (sm *SessionManager) InvalidateSession(sessionID string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	
	if session, ok := sm.sessions[sessionID]; ok {
		session.IsValid = false
	}
}

// InvalidateAllSessions invalide toutes les sessions
func (sm *SessionManager) InvalidateAllSessions() {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	
	for _, session := range sm.sessions {
		session.IsValid = false
	}
}

// GetActiveSessions retourne les sessions actives
func (sm *SessionManager) GetActiveSessions() []*Session {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	
	var active []*Session
	now := time.Now()
	
	for _, session := range sm.sessions {
		if session.IsValid && now.Before(session.ExpiresAt) {
			active = append(active, session)
		}
	}
	
	return active
}

func (sm *SessionManager) cleanupLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	
	for range ticker.C {
		sm.mu.Lock()
		now := time.Now()
		for id, session := range sm.sessions {
			if !session.IsValid || now.After(session.ExpiresAt) {
				delete(sm.sessions, id)
			}
		}
		sm.mu.Unlock()
	}
}

// ============================================================================
// LOGIN ATTEMPTS LIMITING
// ============================================================================

// LoginAttempt représente une tentative de connexion
type LoginAttempt struct {
	IP        string
	Attempts  int
	FirstTry  time.Time
	LockedUntil time.Time
}

// LoginLimiter limite les tentatives de connexion
type LoginLimiter struct {
	attempts    map[string]*LoginAttempt
	mu          sync.RWMutex
	maxAttempts int
	lockoutTime time.Duration
	windowTime  time.Duration
}

// NewLoginLimiter crée un nouveau limiteur
func NewLoginLimiter(maxAttempts int, lockoutTime, windowTime time.Duration) *LoginLimiter {
	ll := &LoginLimiter{
		attempts:    make(map[string]*LoginAttempt),
		maxAttempts: maxAttempts,
		lockoutTime: lockoutTime,
		windowTime:  windowTime,
	}
	
	go ll.cleanupLoop()
	
	return ll
}

// CanAttempt vérifie si une IP peut tenter une connexion
func (ll *LoginLimiter) CanAttempt(ip string) (bool, time.Duration) {
	ll.mu.RLock()
	defer ll.mu.RUnlock()
	
	attempt, ok := ll.attempts[ip]
	if !ok {
		return true, 0
	}
	
	now := time.Now()
	
	// Vérifier si verrouillé
	if now.Before(attempt.LockedUntil) {
		return false, attempt.LockedUntil.Sub(now)
	}
	
	// Vérifier si la fenêtre est expirée
	if now.Sub(attempt.FirstTry) > ll.windowTime {
		return true, 0
	}
	
	return attempt.Attempts < ll.maxAttempts, 0
}

// RecordAttempt enregistre une tentative
func (ll *LoginLimiter) RecordAttempt(ip string, success bool) {
	ll.mu.Lock()
	defer ll.mu.Unlock()
	
	now := time.Now()
	
	attempt, ok := ll.attempts[ip]
	if !ok {
		attempt = &LoginAttempt{
			IP:       ip,
			FirstTry: now,
		}
		ll.attempts[ip] = attempt
	}
	
	// Réinitialiser si fenêtre expirée
	if now.Sub(attempt.FirstTry) > ll.windowTime {
		attempt.Attempts = 0
		attempt.FirstTry = now
	}
	
	if success {
		// Réinitialiser après succès
		attempt.Attempts = 0
		attempt.FirstTry = now
		attempt.LockedUntil = time.Time{}
	} else {
		attempt.Attempts++
		
		// Verrouiller si trop de tentatives
		if attempt.Attempts >= ll.maxAttempts {
			attempt.LockedUntil = now.Add(ll.lockoutTime)
			addLog(fmt.Sprintf("🔒 IP %s verrouillée pour %v", ip, ll.lockoutTime))
		}
	}
}

// GetLockedIPs retourne les IPs verrouillées
func (ll *LoginLimiter) GetLockedIPs() []string {
	ll.mu.RLock()
	defer ll.mu.RUnlock()
	
	var locked []string
	now := time.Now()
	
	for ip, attempt := range ll.attempts {
		if now.Before(attempt.LockedUntil) {
			locked = append(locked, ip)
		}
	}
	
	return locked
}

// UnlockIP déverrouille une IP
func (ll *LoginLimiter) UnlockIP(ip string) {
	ll.mu.Lock()
	defer ll.mu.Unlock()
	
	if attempt, ok := ll.attempts[ip]; ok {
		attempt.LockedUntil = time.Time{}
		attempt.Attempts = 0
	}
}

func (ll *LoginLimiter) cleanupLoop() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()
	
	for range ticker.C {
		ll.mu.Lock()
		now := time.Now()
		for ip, attempt := range ll.attempts {
			if now.Sub(attempt.FirstTry) > ll.windowTime*2 && now.After(attempt.LockedUntil) {
				delete(ll.attempts, ip)
			}
		}
		ll.mu.Unlock()
	}
}

// ============================================================================
// IP WHITELISTING
// ============================================================================

// IPWhitelist gère la liste blanche d'IPs
type IPWhitelist struct {
	ips     map[string]bool
	subnets []*net.IPNet
	enabled bool
	mu      sync.RWMutex
}

// NewIPWhitelist crée une nouvelle liste blanche
func NewIPWhitelist() *IPWhitelist {
	return &IPWhitelist{
		ips:     make(map[string]bool),
		subnets: make([]*net.IPNet, 0),
		enabled: false,
	}
}

// Enable active la liste blanche
func (wl *IPWhitelist) Enable() {
	wl.mu.Lock()
	defer wl.mu.Unlock()
	wl.enabled = true
}

// Disable désactive la liste blanche
func (wl *IPWhitelist) Disable() {
	wl.mu.Lock()
	defer wl.mu.Unlock()
	wl.enabled = false
}

// IsEnabled vérifie si la liste blanche est active
func (wl *IPWhitelist) IsEnabled() bool {
	wl.mu.RLock()
	defer wl.mu.RUnlock()
	return wl.enabled
}

// AddIP ajoute une IP ou un subnet
func (wl *IPWhitelist) AddIP(ipStr string) error {
	wl.mu.Lock()
	defer wl.mu.Unlock()
	
	// Vérifier si c'est un subnet
	if _, subnet, err := net.ParseCIDR(ipStr); err == nil {
		wl.subnets = append(wl.subnets, subnet)
		return nil
	}
	
	// Vérifier si c'est une IP valide
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return fmt.Errorf("IP invalide: %s", ipStr)
	}
	
	wl.ips[ipStr] = true
	return nil
}

// RemoveIP supprime une IP
func (wl *IPWhitelist) RemoveIP(ipStr string) {
	wl.mu.Lock()
	defer wl.mu.Unlock()
	delete(wl.ips, ipStr)
}

// IsAllowed vérifie si une IP est autorisée
func (wl *IPWhitelist) IsAllowed(ipStr string) bool {
	wl.mu.RLock()
	defer wl.mu.RUnlock()
	
	if !wl.enabled {
		return true
	}
	
	// Vérifier IP directe
	if wl.ips[ipStr] {
		return true
	}
	
	// Vérifier subnets
	ip := net.ParseIP(ipStr)
	if ip != nil {
		for _, subnet := range wl.subnets {
			if subnet.Contains(ip) {
				return true
			}
		}
	}
	
	// Toujours autoriser localhost
	if ipStr == "127.0.0.1" || ipStr == "::1" || ipStr == "localhost" {
		return true
	}
	
	return false
}

// GetIPs retourne toutes les IPs autorisées
func (wl *IPWhitelist) GetIPs() []string {
	wl.mu.RLock()
	defer wl.mu.RUnlock()
	
	var ips []string
	for ip := range wl.ips {
		ips = append(ips, ip)
	}
	
	for _, subnet := range wl.subnets {
		ips = append(ips, subnet.String())
	}
	
	return ips
}

// ============================================================================
// TOKEN MANAGEMENT
// ============================================================================

// Token représente un token d'authentification
type Token struct {
	ID        string
	Secret    string
	ClientID  string
	CreatedAt time.Time
	ExpiresAt time.Time
	Scope     []string
	IsValid   bool
}

// TokenManager gère les tokens
type TokenManager struct {
	tokens map[string]*Token
	mu     sync.RWMutex
}

// NewTokenManager crée un gestionnaire de tokens
func NewTokenManager() *TokenManager {
	return &TokenManager{
		tokens: make(map[string]*Token),
	}
}

// CreateToken crée un nouveau token
func (tm *TokenManager) CreateToken(clientID string, duration time.Duration, scope []string) *Token {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	
	tokenID := GenerateSecureToken(16)
	secret := GenerateSecureToken(32)
	now := time.Now()
	
	token := &Token{
		ID:        tokenID,
		Secret:    secret,
		ClientID:  clientID,
		CreatedAt: now,
		ExpiresAt: now.Add(duration),
		Scope:     scope,
		IsValid:   true,
	}
	
	tm.tokens[tokenID] = token
	return token
}

// ValidateToken valide un token
func (tm *TokenManager) ValidateToken(tokenID, secret string) (*Token, bool) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	
	token, ok := tm.tokens[tokenID]
	if !ok || !token.IsValid {
		return nil, false
	}
	
	if time.Now().After(token.ExpiresAt) {
		return nil, false
	}
	
	if subtle.ConstantTimeCompare([]byte(token.Secret), []byte(secret)) != 1 {
		return nil, false
	}
	
	return token, true
}

// RevokeToken révoque un token
func (tm *TokenManager) RevokeToken(tokenID string) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	
	if token, ok := tm.tokens[tokenID]; ok {
		token.IsValid = false
	}
}

// RevokeAllTokens révoque tous les tokens d'un client
func (tm *TokenManager) RevokeAllTokens(clientID string) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	
	for _, token := range tm.tokens {
		if token.ClientID == clientID {
			token.IsValid = false
		}
	}
}

// ============================================================================
// RATE LIMITING
// ============================================================================

// RateLimiter limite les requêtes par IP
type RateLimiter struct {
	requests    map[string]*RateLimit
	mu          sync.RWMutex
	maxRequests int
	windowTime  time.Duration
}

// RateLimit tracking par IP
type RateLimit struct {
	Count     int
	FirstReq  time.Time
	LastReq   time.Time
}

// NewRateLimiter crée un nouveau rate limiter
func NewRateLimiter(maxRequests int, windowTime time.Duration) *RateLimiter {
	rl := &RateLimiter{
		requests:    make(map[string]*RateLimit),
		maxRequests: maxRequests,
		windowTime:  windowTime,
	}
	
	go rl.cleanupLoop()
	
	return rl
}

// Allow vérifie si une requête est autorisée
func (rl *RateLimiter) Allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	
	now := time.Now()
	
	limit, ok := rl.requests[ip]
	if !ok {
		rl.requests[ip] = &RateLimit{
			Count:    1,
			FirstReq: now,
			LastReq:  now,
		}
		return true
	}
	
	// Réinitialiser si fenêtre expirée
	if now.Sub(limit.FirstReq) > rl.windowTime {
		limit.Count = 1
		limit.FirstReq = now
		limit.LastReq = now
		return true
	}
	
	limit.LastReq = now
	limit.Count++
	
	return limit.Count <= rl.maxRequests
}

// GetRequestCount retourne le nombre de requêtes pour une IP
func (rl *RateLimiter) GetRequestCount(ip string) int {
	rl.mu.RLock()
	defer rl.mu.RUnlock()
	
	if limit, ok := rl.requests[ip]; ok {
		return limit.Count
	}
	return 0
}

func (rl *RateLimiter) cleanupLoop() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	
	for range ticker.C {
		rl.mu.Lock()
		now := time.Now()
		for ip, limit := range rl.requests {
			if now.Sub(limit.LastReq) > rl.windowTime*2 {
				delete(rl.requests, ip)
			}
		}
		rl.mu.Unlock()
	}
}

// ============================================================================
// UTILITIES
// ============================================================================

// GenerateSecureToken génère un token sécurisé
func GenerateSecureToken(length int) string {
	bytes := make([]byte, length)
	_, err := rand.Read(bytes)
	if err != nil {
		// Fallback moins sécurisé
		for i := range bytes {
			bytes[i] = byte(time.Now().UnixNano() % 256)
		}
	}
	return hex.EncodeToString(bytes)
}

// GenerateHostID génère un ID host sécurisé
func GenerateHostID() string {
	return GenerateSecureToken(6)[:6] // 6 caractères hex
}

// ============================================================================
// GLOBAL INSTANCES
// ============================================================================

var (
	globalAuthConfig    = NewAuthConfig()
	globalSessionMgr    = NewSessionManager(24 * time.Hour)
	globalLoginLimiter  = NewLoginLimiter(5, 15*time.Minute, 5*time.Minute)
	globalIPWhitelist   = NewIPWhitelist()
	globalTokenMgr      = NewTokenManager()
	globalRateLimiter   = NewRateLimiter(100, time.Minute)
)

// GetAuthConfig retourne la config d'auth globale
func GetAuthConfig() *AuthConfig { return globalAuthConfig }

// GetSessionManager retourne le gestionnaire de sessions global
func GetSessionManager() *SessionManager { return globalSessionMgr }

// GetLoginLimiter retourne le limiteur de connexion global
func GetLoginLimiter() *LoginLimiter { return globalLoginLimiter }

// GetIPWhitelist retourne la liste blanche globale
func GetIPWhitelist() *IPWhitelist { return globalIPWhitelist }

// GetTokenManager retourne le gestionnaire de tokens global
func GetTokenManager() *TokenManager { return globalTokenMgr }

// GetRateLimiter retourne le rate limiter global
func GetRateLimiter() *RateLimiter { return globalRateLimiter }
