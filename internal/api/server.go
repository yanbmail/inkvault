package api

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	qrcode "github.com/skip2/go-qrcode"

	"inkvault/internal/store"
)

const Version = "1.0.0"

// Config is persisted in data/config.json.
type Config struct {
	Port          int      `json:"port"`
	PIN           string   `json:"pin"`
	Token         string   `json:"token"`
	RequireAuth   bool     `json:"requireAuth"`
	PublicURL     string   `json:"publicUrl"`
	TrustProxy    bool     `json:"trustProxy"`
	ForceHTTPS    bool     `json:"forceHttps"`
	SessionDays   int      `json:"sessionDays"`
	AutoBackup    bool     `json:"autoBackup"`
	BackupKeep    int      `json:"backupKeep"`
	BackupEvery   int      `json:"backupEveryHours"`
	VaultName     string   `json:"vaultName"`
	MaxUploadMB   int      `json:"maxUploadMb"`
	AllowedEmails []string `json:"-"`
}

type Server struct {
	mu       sync.RWMutex
	db       *store.DB
	cfg      *Config
	dataDir  string
	webFS    fs.FS
	startAt  time.Time
	attempts map[string]*loginAttempt
}

type loginAttempt struct {
	Count int
	Until time.Time
}

func New(db *store.DB, cfg *Config, dataDir string, webFS fs.FS) *Server {
	s := &Server{db: db, cfg: cfg, dataDir: dataDir, webFS: webFS, startAt: time.Now(), attempts: map[string]*loginAttempt{}}
	go s.autoBackupLoop()
	return s
}

func (s *Server) DB() *store.DB {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.db
}

func (s *Server) Config() *Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	cp := *s.cfg
	return &cp
}

func (s *Server) ReloadDB(db *store.DB) {
	s.mu.Lock()
	old := s.db
	s.db = db
	s.mu.Unlock()
	if old != nil {
		_ = old.Close()
	}
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func randomPIN() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	n := (int(b[0])<<8 | int(b[1])) % 900000
	return fmt.Sprintf("%06d", n+100000)
}

func LoadConfig(dataDir string, defaultPort int) (*Config, error) {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, err
	}
	path := filepath.Join(dataDir, "config.json")
	cfg := &Config{Port: defaultPort, RequireAuth: true, SessionDays: 180, AutoBackup: true,
		BackupKeep: 14, BackupEvery: 24, VaultName: "我的知识库", MaxUploadMB: 64, TrustProxy: true}
	if raw, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(raw, cfg)
	}
	changed := false
	if cfg.Port == 0 {
		cfg.Port = defaultPort
		changed = true
	}
	if cfg.PIN == "" {
		cfg.PIN = randomPIN()
		changed = true
	}
	if cfg.Token == "" {
		cfg.Token = randomHex(24)
		changed = true
	}
	if cfg.SessionDays <= 0 {
		cfg.SessionDays = 180
	}
	if cfg.BackupKeep <= 0 {
		cfg.BackupKeep = 14
	}
	if cfg.BackupEvery <= 0 {
		cfg.BackupEvery = 24
	}
	if cfg.MaxUploadMB <= 0 {
		cfg.MaxUploadMB = 64
	}
	if changed {
		if err := SaveConfig(dataDir, cfg); err != nil {
			return nil, err
		}
	}
	return cfg, nil
}

func SaveConfig(dataDir string, cfg *Config) error {
	raw, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dataDir, "config.json"), raw, 0o600)
}

/* ------------------------------- middleware ------------------------------ */

func (s *Server) isAuthed(r *http.Request) bool {
	cfg := s.Config()
	if !cfg.RequireAuth {
		return true
	}
	// API token (used by the mobile app and automation)
	if tok := r.Header.Get("X-Ink-Token"); tok != "" && subtle.ConstantTimeCompare([]byte(tok), []byte(cfg.Token)) == 1 {
		return true
	}
	if c, err := r.Cookie("inkv_session"); err == nil {
		return verifySession(c.Value, cfg.Token)
	}
	return false
}

func signSession(exp int64, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	fmt.Fprintf(mac, "inkv.%d", exp)
	return fmt.Sprintf("%d.%s", exp, hex.EncodeToString(mac.Sum(nil)))
}

func verifySession(v, secret string) bool {
	parts := strings.Split(v, ".")
	if len(parts) != 2 {
		return false
	}
	exp, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || time.Now().Unix() > exp {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	fmt.Fprintf(mac, "inkv.%d", exp)
	want := hex.EncodeToString(mac.Sum(nil))
	return subtle.ConstantTimeCompare([]byte(parts[1]), []byte(want)) == 1
}

func clientIP(r *http.Request) string {
	if ip, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return ip
	}
	return r.RemoteAddr
}

func (s *Server) publicBase(r *http.Request) string {
	cfg := s.Config()
	if cfg.PublicURL != "" {
		return strings.TrimRight(cfg.PublicURL, "/")
	}
	proto := "http"
	if r.TLS != nil {
		proto = "https"
	}
	if cfg.TrustProxy {
		if p := r.Header.Get("X-Forwarded-Proto"); p != "" {
			proto = strings.Split(p, ",")[0]
		}
	}
	host := r.Host
	if cfg.TrustProxy {
		if h := r.Header.Get("X-Forwarded-Host"); h != "" {
			host = strings.Split(h, ",")[0]
		}
	}
	return proto + "://" + strings.TrimSpace(host)
}

func (s *Server) rateLimited(ip string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	a := s.attempts[ip]
	if a == nil {
		return false
	}
	if time.Now().Before(a.Until) {
		return true
	}
	if a.Until.Before(time.Now().Add(-10 * time.Minute)) {
		delete(s.attempts, ip)
	}
	return false
}

func (s *Server) recordFailure(ip string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	a := s.attempts[ip]
	if a == nil {
		a = &loginAttempt{}
		s.attempts[ip] = a
	}
	a.Count++
	if a.Count >= 8 {
		a.Until = time.Now().Add(10 * time.Minute)
		a.Count = 0
	}
}

/* --------------------------------- routes -------------------------------- */

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/ping", s.handlePing)
	mux.HandleFunc("POST /api/login", s.handleLogin)
	mux.HandleFunc("POST /api/logout", s.handleLogout)

	api := func(pattern string, h http.HandlerFunc) {
		mux.Handle(pattern, s.requireAuth(h))
	}

	api("GET /api/bootstrap", s.handleBootstrap)
	api("GET /api/settings", s.handleGetSettings)
	api("PUT /api/settings", s.handleSaveSettings)
	api("GET /api/audit", s.handleAudit)

	api("GET /api/notes", s.handleListNotes)
	api("POST /api/notes", s.handleCreateNote)
	api("GET /api/notes/{uid}", s.handleGetNote)
	api("PUT /api/notes/{uid}", s.handleUpdateNote)
	api("PATCH /api/notes/{uid}", s.handlePatchNote)
	api("DELETE /api/notes/{uid}", s.handleDeleteNote)
	api("POST /api/notes/{uid}/restore", s.handleRestoreNote)
	api("DELETE /api/notes/{uid}/purge", s.handlePurgeNote)
	api("POST /api/notes/{uid}/duplicate", s.handleDuplicateNote)
	api("GET /api/notes/{uid}/versions", s.handleVersions)
	api("POST /api/notes/{uid}/versions/{id}/restore", s.handleRestoreVersion)
	api("POST /api/notes/{uid}/fields", s.handleSetFields)
	api("POST /api/notes/{uid}/attachments", s.handleUploadAttachment)
	api("GET /api/notes/{uid}/export", s.handleExportNote)
	api("POST /api/notes/{uid}/index", s.handleMakeIndexNote)

	api("GET /api/files/{uid}", s.handleFile)
	api("DELETE /api/attachments/{uid}", s.handleDeleteAttachment)

	api("GET /api/notebooks", s.handleListNotebooks)
	api("POST /api/notebooks", s.handleCreateNotebook)
	api("PUT /api/notebooks/{uid}", s.handleUpdateNotebook)
	api("DELETE /api/notebooks/{uid}", s.handleDeleteNotebook)

	api("GET /api/tags", s.handleListTags)
	api("POST /api/tags", s.handleCreateTag)
	api("PUT /api/tags/{uid}", s.handleUpdateTag)
	api("DELETE /api/tags/{uid}", s.handleDeleteTag)

	api("GET /api/fields", s.handleListFields)
	api("POST /api/fields", s.handleSaveField)
	api("DELETE /api/fields/{uid}", s.handleDeleteField)

	api("GET /api/templates", s.handleListTemplates)
	api("POST /api/templates", s.handleSaveTemplate)
	api("DELETE /api/templates/{uid}", s.handleDeleteTemplate)

	api("GET /api/searches", s.handleListSearches)
	api("POST /api/searches", s.handleSaveSearch)
	api("DELETE /api/searches/{uid}", s.handleDeleteSearch)

	api("POST /api/trash/empty", s.handleEmptyTrash)
	api("GET /api/stats", s.handleStats)
	api("POST /api/stats/note", s.handleStatsNote)

	api("GET /api/export/toc", s.handleExportTOC)
	api("POST /api/export/notes", s.handleExportNotes)
	api("GET /api/export/stats", s.handleExportStats)
	api("GET /api/export/vault", s.handleExportVault)
	api("GET /api/toc/preview", s.handleTOCPreview)
	api("POST /api/toc/preview", s.handleTOCPreview)

	api("POST /api/import", s.handleImport)
	api("POST /api/import/preview", s.handleImportPreview)

	api("GET /api/backups", s.handleListBackups)
	api("POST /api/backups", s.handleCreateBackup)
	api("GET /api/backups/download/{name}", s.handleDownloadBackup)
	api("POST /api/backups/restore", s.handleRestoreBackup)
	api("DELETE /api/backups/{name}", s.handleDeleteBackup)

	api("GET /api/sync/pull", s.handleSyncPull)
	api("POST /api/sync/push", s.handleSyncPush)

	api("POST /api/tools/reindex", s.handleReindex)

	// static
	mux.HandleFunc("/", s.handleStatic)
	return withCORS(withLogging(withRecovery(mux)))
}

func (s *Server) requireAuth(next http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.isAuthed(r) {
			writeErr(w, http.StatusUnauthorized, "需要登录")
			return
		}
		if r.Method != http.MethodGet {
			s.db.Audit(r.Method+" "+r.URL.Path, "", "", clientIP(r))
		}
		next(w, r)
	})
}

// withCORS lets the offline mobile build (file://) reach the sync endpoints.
// Only token-authenticated endpoints are exposed; cookies are never allowed
// cross-origin, so a hostile page cannot ride an existing session.
func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		allowed := p == "/api/ping" || p == "/api/login" || strings.HasPrefix(p, "/api/sync")
		if allowed {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Ink-Token")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Max-Age", "600")
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func withRecovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("panic: %v", rec)
				writeErr(w, 500, "服务器内部错误")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func withLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		if time.Since(start) > 600*time.Millisecond {
			log.Printf("slow %s %s %s", r.Method, r.URL.Path, time.Since(start))
		}
	})
}

/* --------------------------------- helpers ------------------------------- */

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]any{"error": msg})
}

func readJSON(r *http.Request, v any) error {
	defer io.Copy(io.Discard, r.Body)
	dec := json.NewDecoder(io.LimitReader(r.Body, 32<<20))
	return dec.Decode(v)
}

func ok(w http.ResponseWriter, v any) { writeJSON(w, 200, v) }

/* --------------------------------- auth ---------------------------------- */

func (s *Server) handlePing(w http.ResponseWriter, r *http.Request) {
	cfg := s.Config()
	ok(w, map[string]any{
		"app": "InkVault", "version": Version, "name": cfg.VaultName,
		"auth": cfg.RequireAuth, "publicUrl": cfg.PublicURL, "time": store.Now(),
		"notes": s.db.NoteCounts()["all"],
	})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	ip := clientIP(r)
	if s.rateLimited(ip) {
		writeErr(w, http.StatusTooManyRequests, "尝试次数过多，请 10 分钟后再试")
		return
	}
	var body struct {
		PIN string `json:"pin"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, 400, "请求格式错误")
		return
	}
	cfg := s.Config()
	if subtle.ConstantTimeCompare([]byte(strings.TrimSpace(body.PIN)), []byte(cfg.PIN)) != 1 {
		s.recordFailure(ip)
		s.db.Audit("login.failed", "", "wrong pin", ip)
		writeErr(w, http.StatusUnauthorized, "访问码不正确")
		return
	}
	exp := time.Now().Add(time.Duration(cfg.SessionDays) * 24 * time.Hour).Unix()
	cookie := &http.Cookie{
		Name: "inkv_session", Value: signSession(exp, cfg.Token), Path: "/",
		HttpOnly: true, SameSite: http.SameSiteLaxMode, Expires: time.Unix(exp, 0),
		Secure: cfg.ForceHTTPS || r.TLS != nil,
	}
	http.SetCookie(w, cookie)
	s.db.Audit("login.ok", "", "", ip)
	ok(w, map[string]any{"ok": true, "token": cfg.Token, "expires": exp})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: "inkv_session", Value: "", Path: "/", MaxAge: -1})
	ok(w, map[string]any{"ok": true})
}

/* -------------------------------- static --------------------------------- */

func (s *Server) handleStatic(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") {
		writeErr(w, 404, "接口不存在")
		return
	}
	p := strings.TrimPrefix(r.URL.Path, "/")
	if p == "" {
		p = "index.html"
	}
	clean := filepath.Clean("/" + p)
	clean = strings.TrimPrefix(clean, "/")
	data, err := fs.ReadFile(s.webFS, clean)
	if err != nil {
		data, err = fs.ReadFile(s.webFS, "index.html")
		if err != nil {
			http.NotFound(w, r)
			return
		}
		clean = "index.html"
	}
	switch {
	case strings.HasSuffix(clean, ".html"):
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
	case strings.HasSuffix(clean, ".js"):
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	case strings.HasSuffix(clean, ".css"):
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
	case strings.HasSuffix(clean, ".json"), strings.HasSuffix(clean, ".webmanifest"):
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
	case strings.HasSuffix(clean, ".svg"):
		w.Header().Set("Content-Type", "image/svg+xml")
	case strings.HasSuffix(clean, ".png"):
		w.Header().Set("Content-Type", "image/png")
	case strings.HasSuffix(clean, ".woff2"):
		w.Header().Set("Content-Type", "font/woff2")
	}
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(data)
}

/* ------------------------------ auto backup ------------------------------ */

func (s *Server) autoBackupLoop() {
	time.Sleep(2 * time.Minute)
	for {
		cfg := s.Config()
		if cfg.AutoBackup && s.DB().LatestBackupAge() > time.Duration(cfg.BackupEvery)*time.Hour {
			if _, err := s.DB().CreateBackup("auto"); err != nil {
				log.Printf("自动备份失败: %v", err)
			} else {
				s.DB().PruneBackups(cfg.BackupKeep)
			}
		}
		time.Sleep(30 * time.Minute)
	}
}

/* --------------------------------- misc ---------------------------------- */

func (s *Server) handleBootstrap(w http.ResponseWriter, r *http.Request) {
	db := s.DB()
	cfg := s.Config()
	nbs, err := db.Notebooks()
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	stats, _ := db.Stats()
	ok(w, map[string]any{
		"app":           "InkVault",
		"version":       Version,
		"vaultName":     cfg.VaultName,
		"publicUrl":     cfg.PublicURL,
		"baseUrl":       s.publicBase(r),
		"notebooks":     nbs,
		"notebookTree":  db.NotebookOptions(),
		"tags":          db.TagCloud(),
		"fields":        db.Fields(),
		"templates":     db.Templates(),
		"searches":      db.SavedSearches(),
		"counts":        db.NoteCounts(),
		"stats":         stats,
		"settings":      db.AllSettings(),
		"auth":          cfg.RequireAuth,
		"backupKeep":    cfg.BackupKeep,
		"autoBackup":    cfg.AutoBackup,
		"maxUploadMb":   cfg.MaxUploadMB,
		"usesProxy":     cfg.TrustProxy,
		"now":           store.Now(),
		"lanUrls":       lanURLs(cfg.Port),
		"pinSet":        cfg.PIN != "",
		"fts":           db.FTS,
	})
}

func lanURLs(port int) []string {
	out := []string{}
	ifaces, err := net.Interfaces()
	if err != nil {
		return out
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			if ipnet, ok := a.(*net.IPNet); ok {
				ip := ipnet.IP.To4()
				if ip != nil && !ip.IsLoopback() {
					out = append(out, fmt.Sprintf("http://%s:%d", ip.String(), port))
				}
			}
		}
	}
	return out
}

func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	cfg := s.Config()
	db := s.DB()
	publicURL := cfg.PublicURL
	qr := ""
	if publicURL != "" {
		if png, err := qrcode.Encode(publicURL, qrcode.Medium, 512); err == nil {
			qr = "data:image/png;base64," + base64Std(png)
		}
	}
	ok(w, map[string]any{
		"vaultName": cfg.VaultName, "publicUrl": publicURL, "port": cfg.Port,
		"pin": cfg.PIN, "token": cfg.Token, "requireAuth": cfg.RequireAuth,
		"trustProxy": cfg.TrustProxy, "forceHttps": cfg.ForceHTTPS, "sessionDays": cfg.SessionDays,
		"autoBackup": cfg.AutoBackup, "backupKeep": cfg.BackupKeep, "backupEveryHours": cfg.BackupEvery,
		"maxUploadMb": cfg.MaxUploadMB, "lanUrls": lanURLs(cfg.Port), "qr": qr,
		"dataDir": db.DataDir, "dbPath": db.DBPath, "filesDir": db.FilesDir, "version": Version,
		"fts": db.FTS, "settings": db.AllSettings(), "baseUrl": s.publicBase(r),
		"counts": db.NoteCounts(),
	})
}

func (s *Server) handleSaveSettings(w http.ResponseWriter, r *http.Request) {
	var body struct {
		VaultName       *string `json:"vaultName"`
		PublicURL       *string `json:"publicUrl"`
		PIN             *string `json:"pin"`
		Token           *string `json:"token"`
		RequireAuth     *bool   `json:"requireAuth"`
		TrustProxy      *bool   `json:"trustProxy"`
		ForceHTTPS      *bool   `json:"forceHttps"`
		SessionDays     *int    `json:"sessionDays"`
		AutoBackup      *bool   `json:"autoBackup"`
		BackupKeep      *int    `json:"backupKeep"`
		BackupEvery     *int    `json:"backupEveryHours"`
		MaxUploadMB     *int    `json:"maxUploadMb"`
		Theme           *string `json:"theme"`
		EditorFont      *string `json:"editorFont"`
		EditorWidth     *int    `json:"editorWidth"`
		CustomCSS       *string `json:"customCss"`
		DefaultNotebook *string `json:"defaultNotebook"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, 400, "请求格式错误")
		return
	}
	s.mu.Lock()
	cfg := s.cfg
	if body.VaultName != nil && strings.TrimSpace(*body.VaultName) != "" {
		cfg.VaultName = strings.TrimSpace(*body.VaultName)
	}
	if body.PublicURL != nil {
		cfg.PublicURL = strings.TrimRight(strings.TrimSpace(*body.PublicURL), "/")
	}
	if body.PIN != nil && len(strings.TrimSpace(*body.PIN)) >= 4 {
		cfg.PIN = strings.TrimSpace(*body.PIN)
	}
	if body.Token != nil && len(strings.TrimSpace(*body.Token)) >= 8 {
		cfg.Token = strings.TrimSpace(*body.Token)
	}
	if body.RequireAuth != nil {
		cfg.RequireAuth = *body.RequireAuth
	}
	if body.TrustProxy != nil {
		cfg.TrustProxy = *body.TrustProxy
	}
	if body.ForceHTTPS != nil {
		cfg.ForceHTTPS = *body.ForceHTTPS
	}
	if body.SessionDays != nil && *body.SessionDays > 0 {
		cfg.SessionDays = *body.SessionDays
	}
	if body.AutoBackup != nil {
		cfg.AutoBackup = *body.AutoBackup
	}
	if body.BackupKeep != nil && *body.BackupKeep >= 1 {
		cfg.BackupKeep = *body.BackupKeep
	}
	if body.BackupEvery != nil && *body.BackupEvery >= 1 {
		cfg.BackupEvery = *body.BackupEvery
	}
	if body.MaxUploadMB != nil && *body.MaxUploadMB >= 1 {
		cfg.MaxUploadMB = *body.MaxUploadMB
	}
	cp := *cfg
	s.mu.Unlock()
	if err := SaveConfig(s.dataDir, &cp); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	db := s.DB()
	pairs := map[string]*string{
		"theme": body.Theme, "editor_font": body.EditorFont, "custom_css": body.CustomCSS,
		"default_notebook": body.DefaultNotebook,
	}
	for k, v := range pairs {
		if v != nil {
			_ = db.SetSetting(k, *v)
		}
	}
	if body.EditorWidth != nil {
		_ = db.SetSetting("editor_width", strconv.Itoa(*body.EditorWidth))
	}
	_ = db.SetSetting("vault_name", cp.VaultName)
	db.Audit("settings.save", "", "", clientIP(r))
	ok(w, map[string]any{"ok": true, "pin": cp.PIN, "token": cp.Token})
}

func (s *Server) handleAudit(w http.ResponseWriter, r *http.Request) {
	rows, err := s.DB().SQL.Query(`SELECT action,target,detail,ip,created_at FROM audit_log ORDER BY id DESC LIMIT 200`)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	defer rows.Close()
	out := []map[string]string{}
	for rows.Next() {
		var a, t, d, ip, c string
		if rows.Scan(&a, &t, &d, &ip, &c) == nil {
			out = append(out, map[string]string{"action": a, "target": t, "detail": d, "ip": ip, "createdAt": c})
		}
	}
	ok(w, out)
}

func (s *Server) handleReindex(w http.ResponseWriter, r *http.Request) {
	if err := s.DB().RebuildFTS(); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	ok(w, map[string]any{"ok": true})
}

func (s *Server) handleListBackups(w http.ResponseWriter, r *http.Request) {
	ok(w, map[string]any{"backups": s.DB().ListBackups(), "dir": s.DB().BackupsDir()})
}

func (s *Server) handleCreateBackup(w http.ResponseWriter, r *http.Request) {
	info, err := s.DB().CreateBackup("manual")
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.DB().PruneBackups(s.Config().BackupKeep)
	ok(w, info)
}

func (s *Server) handleDownloadBackup(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if strings.Contains(name, "/") || strings.Contains(name, "..") {
		writeErr(w, 400, "非法文件名")
		return
	}
	path := filepath.Join(s.DB().BackupsDir(), name)
	if _, err := os.Stat(path); err != nil {
		writeErr(w, 404, "备份不存在")
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", "attachment; filename=\""+name+"\"")
	http.ServeFile(w, r, path)
}

func (s *Server) handleRestoreBackup(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}
	if err := readJSON(r, &body); err != nil || body.Name == "" {
		writeErr(w, 400, "请指定备份文件")
		return
	}
	path := filepath.Join(s.DB().BackupsDir(), filepath.Base(body.Name))
	if _, err := os.Stat(path); err != nil {
		writeErr(w, 404, "备份不存在")
		return
	}
	// safety snapshot before touching the live vault
	if _, err := s.DB().CreateBackup("before-restore"); err != nil {
		writeErr(w, 500, "恢复前备份失败："+err.Error())
		return
	}
	stage, err := store.StageBackup(path, s.dataDir)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	old := s.DB()
	_ = old.Close()
	if err := store.SwapVault(stage, s.dataDir); err != nil {
		writeErr(w, 500, "替换数据失败："+err.Error())
		return
	}
	db, err := store.Open(s.dataDir)
	if err != nil {
		writeErr(w, 500, "重新打开数据库失败："+err.Error())
		return
	}
	s.ReloadDB(db)
	ok(w, map[string]any{"ok": true, "message": "已从备份恢复"})
}

func (s *Server) handleDeleteBackup(w http.ResponseWriter, r *http.Request) {
	if err := s.DB().DeleteBackup(r.PathValue("name")); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	ok(w, map[string]any{"ok": true})
}
