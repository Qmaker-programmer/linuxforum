// Copyright (C) 2026 Qmaker <andresavalosgallegos@gmail.com>
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

// inlineScriptCSPHash pins the Content-Security-Policy to the one
// first-party inline <script> in web/head.html (scroll-position restore —
// see the comment there). This is the hash of the script as html/template
// actually renders it, not of the source file: Go's JS-context escaper
// strips "//" comments from <script> blocks, so the two differ. If that
// script's contents ever change, this hash must be recomputed (e.g. by
// fetching any page and hashing the sha256 of the rendered <script> body),
// or the script will silently stop running.
const inlineScriptCSPHash = "sha256-4g4bkcBIeR3Mew6StkUoVBB57XONZZnAl2nEGi3ygw0="

func loadConfig() {
	config = Config{
		Port:                8080,
		DBPath:              "forum.db",
		CertFile:            "cert.pem",
		KeyFile:             "key.pem",
		SessionTokenName:    "session_token",
		RateLimit:           100,
		ResetMinutes:        1,
		BackupIntervalHours: defaultBackupIntervalHours,
	}

	f, err := os.Open("config.json")
	if err != nil {
		slog.Warn("No se pudo cargar config.json, usando valores por defecto", "err", err)
		return
	}
	defer f.Close()
	if err := json.NewDecoder(f).Decode(&config); err != nil {
		slog.Warn("No se pudo decodificar config.json, usando valores por defecto", "err", err)
		return
	}
	if config.RateLimit <= 0 {
		config.RateLimit = 100
	}
	if config.ResetMinutes <= 0 {
		config.ResetMinutes = 1
	}
	if config.Port <= 0 {
		config.Port = 8080
	}
	if config.DBPath == "" {
		config.DBPath = "forum.db"
	}
	if config.CertFile == "" {
		config.CertFile = "cert.pem"
	}
	if config.KeyFile == "" {
		config.KeyFile = "key.pem"
	}
	if config.SessionTokenName == "" {
		config.SessionTokenName = "session_token"
	}
	if config.BackupIntervalHours <= 0 {
		config.BackupIntervalHours = defaultBackupIntervalHours
	}
}

// clientIP returns the address to key rate limiting on. X-Forwarded-For
// and X-Real-IP are only trusted when trust_proxy_headers is enabled in
// config.json — otherwise any direct client could spoof them to dodge
// the limit or frame another IP. Only enable it when the server is
// actually reachable exclusively through a reverse proxy that sets
// these headers itself (see the "Integración" section of the README).
func clientIP(r *http.Request) string {
	if config.TrustProxyHeaders {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			if idx := strings.Index(xff, ","); idx != -1 {
				xff = xff[:idx]
			}
			return strings.TrimSpace(xff)
		}
		if xrip := strings.TrimSpace(r.Header.Get("X-Real-IP")); xrip != "" {
			return xrip
		}
	}

	ip := r.RemoteAddr
	if idx := strings.LastIndex(ip, ":"); idx != -1 {
		ip = ip[:idx]
	}
	return ip
}

func rateLimitMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := clientIP(r)

		mu.Lock()
		count := requestCounts[ip]
		if count >= config.RateLimit {
			mu.Unlock()
			http.Error(w, "Demasiadas solicitudes. Intenta de nuevo en un minuto.", http.StatusTooManyRequests)
			return
		}
		requestCounts[ip] = count + 1
		mu.Unlock()

		next.ServeHTTP(w, r)
	})
}

// securityHeadersMiddleware adds defense-in-depth headers. There's no JS
// in this app except the one inline script pinned by hash below (see
// web/head.html), so script-src is about as strict as CSP gets.
func securityHeadersMiddleware(next http.Handler) http.Handler {
	csp := "default-src 'self'; img-src 'self'; style-src 'self' 'unsafe-inline'; " +
		"script-src 'self' " + inlineScriptCSPHash + "; base-uri 'self'; " +
		"form-action 'self'; frame-ancestors 'none'"

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("Content-Security-Policy", csp)
		next.ServeHTTP(w, r)
	})
}

// recordLoginFailure counts a failed login attempt for username and locks
// it out once it hits maxLoginAttempts. Tracked per-username rather than
// per-IP: the account being brute-forced is what needs protecting,
// regardless of how many IPs the attacker spreads attempts across.
func recordLoginFailure(username string) {
	loginMu.Lock()
	defer loginMu.Unlock()
	loginAttempts[username]++
	if loginAttempts[username] >= maxLoginAttempts {
		loginLockedUntil[username] = time.Now().Add(loginLockoutMinutes * time.Minute)
	}
}

func clearLoginFailures(username string) {
	loginMu.Lock()
	defer loginMu.Unlock()
	delete(loginAttempts, username)
	delete(loginLockedUntil, username)
}

// isLoginLocked reports whether username is currently locked out, and how
// many minutes remain. It also lazily clears expired lockouts.
func isLoginLocked(username string) (bool, int) {
	loginMu.Lock()
	defer loginMu.Unlock()
	until, ok := loginLockedUntil[username]
	if !ok {
		return false, 0
	}
	remaining := time.Until(until)
	if remaining <= 0 {
		delete(loginLockedUntil, username)
		delete(loginAttempts, username)
		return false, 0
	}
	return true, int(remaining.Minutes()) + 1
}

func resetRequestCounts() {
	for {
		time.Sleep(time.Duration(config.ResetMinutes) * time.Minute)
		mu.Lock()
		requestCounts = make(map[string]int)
		mu.Unlock()
	}
}

func sessionExpiry() time.Time {
	if config.SessionExpireMinutes <= 0 {
		return time.Time{}
	}
	return time.Now().Add(time.Duration(config.SessionExpireMinutes) * time.Minute)
}

func cleanupExpiredSessions() {
	for {
		time.Sleep(10 * time.Minute)
		if config.SessionExpireMinutes <= 0 {
			continue
		}
		db.Exec("DELETE FROM sessions WHERE expires_at != '' AND expires_at < datetime('now')")
	}
}

func main() {
	initLogger()
	loadConfig()
	applyLogLevel(config.LogLevel)
	loadMailConfig()
	initDB()
	if err := ensureUploadsDir(); err != nil {
		slog.Error("No se pudo crear el directorio de subidas", "err", err)
		os.Exit(1)
	}
	if err := ensureBackupsDir(); err != nil {
		slog.Error("No se pudo crear el directorio de backups", "err", err)
		os.Exit(1)
	}

	http.HandleFunc("/web/login.html", handleLogin)
	http.HandleFunc("/web/public.html", handlePublic)

	fs := http.FileServer(http.Dir("./web"))
	http.Handle("/web/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/web/" || strings.HasSuffix(r.URL.Path, ".html") {
			http.NotFound(w, r)
			return
		}
		http.StripPrefix("/web/", fs).ServeHTTP(w, r)
	}))

	http.HandleFunc("/", handleHome)
	http.HandleFunc("/filtered", handleFiltered)
	http.HandleFunc("/auth", handleAuth)
	http.HandleFunc("/logout", handleLogout)
	http.HandleFunc("/post", handlePost)
	http.HandleFunc("/post-form", handlePostForm)
	http.HandleFunc("/draft", handleDraftSave)
	http.HandleFunc("/drafts", handleDrafts)
	http.HandleFunc("/draft-delete", handleDraftDelete)
	http.HandleFunc("/comment", handleComment)
	http.HandleFunc("/comment-form", handleCommentForm)
	http.HandleFunc("/comment-draft", handleCommentDraftSave)
	http.HandleFunc("/comment-draft-delete", handleCommentDraftDelete)
	http.HandleFunc("/delete-comment", handleDeleteComment)
	http.HandleFunc("/search", handleSearch)
	http.HandleFunc("/view", handleView)
	http.HandleFunc("/confirm", handleConfirm)
	http.HandleFunc("/post-history", handlePostHistory)
	http.HandleFunc("/post-revert", handlePostRevert)
	http.HandleFunc("/user", handleUser)
	http.HandleFunc("/profile", handleProfile)
	http.HandleFunc("/save", handleSave)
	http.HandleFunc("/unsave", handleUnsave)
	http.HandleFunc("/theme", handleTheme)
	http.HandleFunc("/forgot", handleForgot)
	http.HandleFunc("/reset", handleReset)
	http.HandleFunc("/activate", handleActivate)
	http.HandleFunc("/request-delete", handleRequestDelete)
	http.HandleFunc("/confirm-deletion", handleConfirmDeletion)
	http.HandleFunc("/confirm-post-deletion", handleConfirmPostDeletion)

	go resetRequestCounts()
	go cleanupExpiredSessions()
	go runPeriodicBackups()
	go func() {
		for {
			time.Sleep(30 * time.Minute)
			cleanupExpiredResetTokens()
		}
	}()
	go func() {
		for {
			time.Sleep(30 * time.Minute)
			cleanupExpiredPendingActivations()
		}
	}()
	go func() {
		for {
			time.Sleep(30 * time.Minute)
			cleanupExpiredPendingDeletions()
		}
	}()
	go func() {
		for {
			time.Sleep(30 * time.Minute)
			cleanupExpiredPendingPostDeletions()
		}
	}()

	handler := securityHeadersMiddleware(rateLimitMiddleware(http.DefaultServeMux))

	addr := fmt.Sprintf(":%d", config.Port)
	server := &http.Server{Addr: addr, Handler: handler}

	serverErr := make(chan error, 1)
	if config.HTTPS {
		if _, err := os.Stat(config.CertFile); os.IsNotExist(err) {
			slog.Error("No se encuentra el certificado SSL. Genera uno.", "cert_file", config.CertFile)
			os.Exit(1)
		}
		if _, err := os.Stat(config.KeyFile); os.IsNotExist(err) {
			slog.Error("No se encuentra la llave SSL. Genera una.", "key_file", config.KeyFile)
			os.Exit(1)
		}
		slog.Info("Servidor corriendo con HTTPS", "addr", "https://localhost"+addr)
		go func() { serverErr <- server.ListenAndServeTLS(config.CertFile, config.KeyFile) }()
	} else {
		slog.Info("Servidor corriendo", "addr", "http://localhost"+addr)
		go func() { serverErr <- server.ListenAndServe() }()
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-serverErr:
		if err != nil && err != http.ErrServerClosed {
			slog.Error("Error del servidor", "err", err)
		}
	case <-stop:
		slog.Info("Apagando servidor...")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			slog.Error("Error al apagar el servidor", "err", err)
		}
	}

	db.Close()
}
