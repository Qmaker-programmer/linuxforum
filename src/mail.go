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
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/smtp"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

type MailConfig struct {
	Mail     string `json:"mail"`
	Password string `json:"password"`
	SMTPHost string `json:"smtp_host"`
	SMTPPort int    `json:"smtp_port"`
}

var mailCfg MailConfig

func loadMailConfig() {
	mailCfg = MailConfig{
		SMTPPort: 587,
	}

	f, err := os.Open("noUpload/mail.json")
	if err != nil {
		slog.Info("Mail no configurado (noUpload/mail.json no encontrado)", "err", err)
		return
	}
	defer f.Close()
	if err := json.NewDecoder(f).Decode(&mailCfg); err != nil {
		slog.Warn("No se pudo decodificar noUpload/mail.json", "err", err)
		return
	}
	if mailCfg.SMTPHost == "" {
		mailCfg.SMTPHost = guessSMTPHost(mailCfg.Mail)
	}
	if mailCfg.SMTPPort <= 0 {
		mailCfg.SMTPPort = 587
	}
}

func guessSMTPHost(email string) string {
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return "smtp.gmail.com"
	}
	switch strings.ToLower(parts[1]) {
	case "gmail.com":
		return "smtp.gmail.com"
	case "outlook.com", "hotmail.com":
		return "smtp-mail.outlook.com"
	case "yahoo.com":
		return "smtp.mail.yahoo.com"
	default:
		return "smtp." + strings.ToLower(parts[1])
	}
}

func generateSessionToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return hex.EncodeToString(b)
}

func generateResetToken() (string, string) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", ""
	}
	token := hex.EncodeToString(b)
	return token, hashToken(token)
}

// hashToken hashes a random token before it's stored or looked up in the
// database, so that read access to the DB alone (a leaked backup, etc.)
// never yields a usable token.
func hashToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

func sendResetEmail(to, token, baseURL string) error {
	auth := smtp.PlainAuth("", mailCfg.Mail, mailCfg.Password, mailCfg.SMTPHost)

	subject := "Restablece tu contraseña en LinuxForum"
	resetLink := baseURL + "/reset?token=" + token
	body := fmt.Sprintf(`Has solicitado restablecer tu contraseña en LinuxForum.

Haz clic en el siguiente enlace para restablecer tu contraseña:
%s

Si no solicitaste esto, ignora este mensaje.

Este enlace expirará en 1 hora.`, resetLink)

	msg := []byte("To: " + to + "\r\n" +
		"Subject: " + subject + "\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/plain; charset=UTF-8\r\n" +
		"\r\n" +
		body + "\r\n")

	addr := fmt.Sprintf("%s:%d", mailCfg.SMTPHost, mailCfg.SMTPPort)
	return smtp.SendMail(addr, auth, mailCfg.Mail, []string{to}, msg)
}

func sendDeletionEmail(to, token, baseURL string) error {
	auth := smtp.PlainAuth("", mailCfg.Mail, mailCfg.Password, mailCfg.SMTPHost)

	subject := "Eliminación de cuenta en LinuxForum"
	deleteLink := baseURL + "/confirm-deletion?token=" + token
	body := fmt.Sprintf(`Has solicitado eliminar tu cuenta de LinuxForum.

Esta acción es irreversible. Todos tus posts serán eliminados y tus comentarios marcados como eliminados.

Haz clic en el siguiente enlace para confirmar la eliminación:
%s

Si no solicitaste esto, ignora este mensaje.

Este enlace expirará en 1 hora.`, deleteLink)

	msg := []byte("To: " + to + "\r\n" +
		"Subject: " + subject + "\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/plain; charset=UTF-8\r\n" +
		"\r\n" +
		body + "\r\n")

	addr := fmt.Sprintf("%s:%d", mailCfg.SMTPHost, mailCfg.SMTPPort)
	return smtp.SendMail(addr, auth, mailCfg.Mail, []string{to}, msg)
}

func handleRequestDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	loggedUser := getLoggedUser(r)
	if loggedUser == "" {
		http.Redirect(w, r, "/web/login.html", http.StatusSeeOther)
		return
	}

	if !validateCSRF(r) {
		http.Error(w, "CSRF token inválido", http.StatusForbidden)
		return
	}

	user := getUser(loggedUser)
	if user == nil || user.Email == "" {
		http.Redirect(w, r, "/user?u="+url.QueryEscape(loggedUser)+"&error="+url.QueryEscape("No tienes un correo asociado. Agrega uno para eliminar la cuenta."), http.StatusSeeOther)
		return
	}

	token, tokenHash := generateResetToken()
	if err := savePendingDeletion(loggedUser, tokenHash); err != nil {
		http.Redirect(w, r, "/user?u="+url.QueryEscape(loggedUser)+"&error="+url.QueryEscape("Error al procesar la solicitud."), http.StatusSeeOther)
		return
	}

	baseURL := getBaseURL(r)
	if err := sendDeletionEmail(user.Email, token, baseURL); err != nil {
		deletePendingDeletion(loggedUser)
		slog.Error("No se pudo enviar el correo de eliminación de cuenta", "err", err)
		http.Redirect(w, r, "/user?u="+url.QueryEscape(loggedUser)+"&error="+url.QueryEscape("Error al enviar el correo."), http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, "/user?u="+url.QueryEscape(loggedUser)+"&success="+url.QueryEscape("Revisa tu correo para confirmar la eliminación de la cuenta."), http.StatusSeeOther)
}

func handleConfirmDeletion(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		if !validateCSRF(r) {
			http.Error(w, "CSRF token inválido", http.StatusForbidden)
			return
		}

		token := strings.TrimSpace(r.FormValue("token"))
		if token == "" {
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}

		tokenHash := hashToken(token)
		pd := getPendingDeletionByHash(tokenHash)
		if pd == nil {
			http.Redirect(w, r, "/web/login.html?login_user_error="+url.QueryEscape("El enlace de eliminación no es válido o ya expiró."), http.StatusSeeOther)
			return
		}

		if err := deleteUserAccount(pd.Username); err != nil {
			slog.Error("No se pudo eliminar la cuenta", "username", pd.Username, "err", err)
			http.Redirect(w, r, "/web/login.html?login_user_error="+url.QueryEscape("Error al eliminar la cuenta."), http.StatusSeeOther)
			return
		}

		deleteUserSessions(pd.Username)

		http.SetCookie(w, &http.Cookie{
			Name:   config.SessionTokenName,
			Value:  "",
			Path:   "/",
			MaxAge: -1,
		})

		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	query, loggedUser := pageContext(r)
	token := strings.TrimSpace(r.URL.Query().Get("token"))
	if token == "" {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	tokenHash := hashToken(token)
	pd := getPendingDeletionByHash(tokenHash)
	if pd == nil {
		http.Redirect(w, r, "/web/login.html?login_user_error="+url.QueryEscape("El enlace de eliminación no es válido o ya expiró."), http.StatusSeeOther)
		return
	}

	renderPage(w, r, "web/confirm-deletion.html", struct {
		Query      string
		LoggedUser string
		Theme      string
		Token      string
		Username   string
		CSRFToken  string
	}{
		Query:      query,
		LoggedUser: loggedUser,
		Theme:      getTheme(r),
		Token:      token,
		Username:   pd.Username,
		CSRFToken:  getCSRFToken(r),
	})
}

func sendPostDeletionEmail(to, postTitle, token, baseURL string) error {
	auth := smtp.PlainAuth("", mailCfg.Mail, mailCfg.Password, mailCfg.SMTPHost)

	subject := "Eliminar post en LinuxForum"
	deleteLink := baseURL + "/confirm-post-deletion?token=" + token
	body := fmt.Sprintf(`Has solicitado eliminar el post "%s" de LinuxForum.

Haz clic en el siguiente enlace para confirmar la eliminación:
%s

Si no solicitaste esto, ignora este mensaje.

Este enlace expirará en 1 hora.`, postTitle, deleteLink)

	msg := []byte("To: " + to + "\r\n" +
		"Subject: " + subject + "\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/plain; charset=UTF-8\r\n" +
		"\r\n" +
		body + "\r\n")

	addr := fmt.Sprintf("%s:%d", mailCfg.SMTPHost, mailCfg.SMTPPort)
	return smtp.SendMail(addr, auth, mailCfg.Mail, []string{to}, msg)
}

func handleConfirmPostDeletion(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		if !validateCSRF(r) {
			http.Error(w, "CSRF token inválido", http.StatusForbidden)
			return
		}

		token := strings.TrimSpace(r.FormValue("token"))
		if token == "" {
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}

		tokenHash := hashToken(token)
		ppd := getPendingPostDeletionByHash(tokenHash)
		if ppd == nil {
			http.Redirect(w, r, "/?error="+url.QueryEscape("El enlace de eliminación no es válido o ya expiró."), http.StatusSeeOther)
			return
		}

		postMessage := ""
		if post := getPostByID(ppd.PostID); post != nil {
			postMessage = post.Message
		}
		deletePostCascade(ppd.PostID, postMessage)
		deletePendingPostDeletion(ppd.PostID)

		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	token := strings.TrimSpace(r.URL.Query().Get("token"))
	if token == "" {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	tokenHash := hashToken(token)
	ppd := getPendingPostDeletionByHash(tokenHash)
	if ppd == nil {
		http.Redirect(w, r, "/?error="+url.QueryEscape("El enlace de eliminación no es válido o ya expiró."), http.StatusSeeOther)
		return
	}

	post := getPostByID(ppd.PostID)
	if post == nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	query, loggedUser := pageContext(r)
	renderPage(w, r, "web/confirm-post-deletion.html", struct {
		Query      string
		LoggedUser string
		Theme      string
		Token      string
		PostTitle  string
		CSRFToken  string
	}{
		Query:      query,
		LoggedUser: loggedUser,
		Theme:      getTheme(r),
		Token:      token,
		PostTitle:  post.Title,
		CSRFToken:  getCSRFToken(r),
	})
}

func handleForgot(w http.ResponseWriter, r *http.Request) {
	query, loggedUser := pageContext(r)
	theme := getTheme(r)

	if r.Method != http.MethodPost {
		renderPage(w, r, "web/forgot.html", struct {
			Query      string
			LoggedUser string
			Theme      string
			Success    string
			Error      string
		}{
			Query:      query,
			LoggedUser: loggedUser,
			Theme:      theme,
			Success:    r.URL.Query().Get("success"),
			Error:      r.URL.Query().Get("error"),
		})
		return
	}

	email := strings.TrimSpace(r.FormValue("email"))
	if email == "" || !strings.Contains(email, "@") {
		http.Redirect(w, r, "/forgot?error="+url.QueryEscape("Indica un correo electrónico válido."), http.StatusSeeOther)
		return
	}

	user := getUserByEmail(email)
	if user == nil || mailCfg.Mail == "" {
		http.Redirect(w, r, "/forgot?success="+url.QueryEscape("Si el correo está registrado, recibirás un enlace para restablecer tu contraseña."), http.StatusSeeOther)
		return
	}

	token, tokenHash := generateResetToken()
	expiresAt := time.Now().Add(1 * time.Hour)

	if err := saveResetToken(user.Username, tokenHash, expiresAt); err != nil {
		http.Redirect(w, r, "/forgot?error="+url.QueryEscape("Error al procesar la solicitud. Intenta de nuevo."), http.StatusSeeOther)
		return
	}

	baseURL := getBaseURL(r)
	if err := sendResetEmail(email, token, baseURL); err != nil {
		slog.Error("No se pudo enviar el correo de reset de contraseña", "err", err)
		http.Redirect(w, r, "/forgot?error="+url.QueryEscape("Error al enviar el correo. Verifica la configuración de mail."), http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, "/forgot?success="+url.QueryEscape("Si el correo está registrado, recibirás un enlace para restablecer tu contraseña."), http.StatusSeeOther)
}

func sendVerificationEmail(to, token, baseURL string) error {
	auth := smtp.PlainAuth("", mailCfg.Mail, mailCfg.Password, mailCfg.SMTPHost)

	subject := "Activa tu cuenta en LinuxForum"
	activationLink := baseURL + "/activate?token=" + token
	body := fmt.Sprintf(`Gracias por registrarte en LinuxForum.

Haz clic en el siguiente enlace para activar tu cuenta:
%s

Si no solicitaste esto, ignora este mensaje.

Este enlace expirará en 24 horas.`, activationLink)

	msg := []byte("To: " + to + "\r\n" +
		"Subject: " + subject + "\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/plain; charset=UTF-8\r\n" +
		"\r\n" +
		body + "\r\n")

	addr := fmt.Sprintf("%s:%d", mailCfg.SMTPHost, mailCfg.SMTPPort)
	return smtp.SendMail(addr, auth, mailCfg.Mail, []string{to}, msg)
}

func handleActivate(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSpace(r.URL.Query().Get("token"))
	if token == "" {
		http.Redirect(w, r, "/web/login.html", http.StatusSeeOther)
		return
	}

	tokenHash := hashToken(token)
	pa := getPendingActivationByHash(tokenHash)
	if pa == nil {
		http.Redirect(w, r, "/web/login.html?login_user_error="+url.QueryEscape("El enlace de activación no es válido o ya expiró."), http.StatusSeeOther)
		return
	}

	if existsUser(pa.Username) {
		deletePendingActivation(pa.Username)
		http.Redirect(w, r, "/web/login.html?login_user_error="+url.QueryEscape("Esa cuenta ya está activada. Inicia sesión."), http.StatusSeeOther)
		return
	}

	db.Exec("INSERT INTO users (username, password, description, email) VALUES (?, ?, ?, ?)", pa.Username, pa.Password, "", pa.Email)

	deletePendingActivation(pa.Username)

	sessionToken := generateSessionToken()
	saveSession(sessionToken, Session{
		Username:  pa.Username,
		ExpiresAt: sessionExpiry(),
		CSRFToken: generateCSRFToken(),
	})

	http.SetCookie(w, &http.Cookie{
		Name:     config.SessionTokenName,
		Value:    sessionToken,
		Path:     "/",
		HttpOnly: true,
		Secure:   config.HTTPS,
		SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func handleReset(w http.ResponseWriter, r *http.Request) {
	query, loggedUser := pageContext(r)
	theme := getTheme(r)

	if r.Method != http.MethodPost {
		token := strings.TrimSpace(r.URL.Query().Get("token"))
		if token == "" {
			http.Redirect(w, r, "/forgot", http.StatusSeeOther)
			return
		}

		tokenHash := hashToken(token)
		rt := getResetTokenByHash(tokenHash)

		if rt == nil || rt.Used {
			http.Redirect(w, r, "/forgot?error="+url.QueryEscape("El enlace no es válido o ya fue usado."), http.StatusSeeOther)
			return
		}

		expiry, err := time.Parse(time.RFC3339, rt.ExpiresAt)
		if err != nil || time.Now().After(expiry) {
			markResetTokenUsed(rt.Username)
			http.Redirect(w, r, "/forgot?error="+url.QueryEscape("El enlace ha expirado. Solicita uno nuevo."), http.StatusSeeOther)
			return
		}

		renderPage(w, r, "web/reset.html", struct {
			Query      string
			LoggedUser string
			Theme      string
			Token      string
			Error      string
		}{
			Query:      query,
			LoggedUser: loggedUser,
			Theme:      theme,
			Token:      token,
			Error:      r.URL.Query().Get("error"),
		})
		return
	}

	token := strings.TrimSpace(r.FormValue("token"))
	password := r.FormValue("password")

	if token == "" {
		http.Redirect(w, r, "/forgot", http.StatusSeeOther)
		return
	}

	tokenHash := hashToken(token)
	rt := getResetTokenByHash(tokenHash)

	if rt == nil || rt.Used {
		http.Redirect(w, r, "/forgot?error="+url.QueryEscape("El enlace no es válido o ya fue usado."), http.StatusSeeOther)
		return
	}

	expiry, err := time.Parse(time.RFC3339, rt.ExpiresAt)
	if err != nil || time.Now().After(expiry) {
		markResetTokenUsed(rt.Username)
		http.Redirect(w, r, "/forgot?error="+url.QueryEscape("El enlace ha expirado. Solicita uno nuevo."), http.StatusSeeOther)
		return
	}

	if password == "" || len(password) < 8 {
		http.Redirect(w, r, "/reset?token="+token+"&error="+url.QueryEscape("La contraseña debe tener al menos 8 caracteres."), http.StatusSeeOther)
		return
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		http.Redirect(w, r, "/reset?token="+token+"&error="+url.QueryEscape("Error al cambiar la contraseña. Intenta de nuevo."), http.StatusSeeOther)
		return
	}

	if err := setUserPassword(rt.Username, string(hashed)); err != nil {
		http.Redirect(w, r, "/reset?token="+token+"&error="+url.QueryEscape("Error al cambiar la contraseña. Intenta de nuevo."), http.StatusSeeOther)
		return
	}

	markResetTokenUsed(rt.Username)
	http.Redirect(w, r, "/web/login.html?login_user="+url.QueryEscape(rt.Username), http.StatusSeeOther)
}
