// Copyright (C) 2026 Qmaker <andresavalosgallegos@gmail.com>
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

func loadConfig() {
	config = Config{
		Port:             8080,
		DBPath:           "forum.db",
		CertFile:         "cert.pem",
		KeyFile:          "key.pem",
		SessionTokenName: "session_token",
		RateLimit:        100,
		ResetMinutes:     1,
	}

	f, err := os.Open("config.json")
	if err != nil {
		fmt.Println("Error al cargar config.json, usando valores por defecto:", err)
		return
	}
	defer f.Close()
	if err := json.NewDecoder(f).Decode(&config); err != nil {
		fmt.Println("Error al decodificar config.json, usando valores por defecto:", err)
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
}

func rateLimitMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := r.RemoteAddr
		if idx := strings.LastIndex(ip, ":"); idx != -1 {
			ip = ip[:idx]
		}

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
	loadConfig()
	loadMailConfig()
	initDB()

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
	http.HandleFunc("/delete-comment", handleDeleteComment)
	http.HandleFunc("/search", handleSearch)
	http.HandleFunc("/view", handleView)
	http.HandleFunc("/confirm", handleConfirm)
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

	handler := rateLimitMiddleware(http.DefaultServeMux)

	addr := fmt.Sprintf(":%d", config.Port)
	if config.HTTPS {
		if _, err := os.Stat(config.CertFile); os.IsNotExist(err) {
			fmt.Println("Error: No se encuentra", config.CertFile, ". Genera un certificado SSL.")
			os.Exit(1)
		}
		if _, err := os.Stat(config.KeyFile); os.IsNotExist(err) {
			fmt.Println("Error: No se encuentra", config.KeyFile, ". Genera un certificado SSL.")
			os.Exit(1)
		}
		fmt.Println("Servidor corriendo con HTTPS en https://localhost" + addr)
		http.ListenAndServeTLS(addr, config.CertFile, config.KeyFile, handler)
	} else {
		fmt.Println("Servidor corriendo en http://localhost" + addr)
		http.ListenAndServe(addr, handler)
	}
}
