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
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/crypto/bcrypt"
)

func TestMain(m *testing.M) {
	// Change to project root so web/ templates are found
	os.Chdir("..")
	os.Exit(m.Run())
}

func setupTest(t *testing.T) *sql.DB {
	t.Helper()

	mu.Lock()
	requestCounts = make(map[string]int)
	config = Config{
		DBPath:           t.TempDir() + "/test.db",
		SessionTokenName: "session_token",
		RateLimit:        1000,
		ResetMinutes:     1,
	}
	mu.Unlock()

	var err error
	db, err = sql.Open("sqlite3", config.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	db.Exec("PRAGMA journal_mode=WAL")
	runMigrations()

	// Create test user with known CSRF token
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatal(err)
	}
	db.Exec("INSERT INTO users (username, password, description, email) VALUES (?, ?, ?, ?)", "testuser", string(hashedPassword), "Test description", "test@example.com")

	return db
}

func loginUser(t *testing.T, r *http.Request) {
	t.Helper()
	token := "test-session-token"
	saveSession(token, Session{Username: "testuser", CSRFToken: "test-csrf-token"})
	r.AddCookie(&http.Cookie{Name: config.SessionTokenName, Value: token})
}

func TestHomePage(t *testing.T) {
	setupTest(t)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	handleHome(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Discusiones Recientes") {
		t.Error("missing expected text")
	}
}

func TestFiltered(t *testing.T) {
	setupTest(t)

	req := httptest.NewRequest(http.MethodGet, "/filtered?sort_by=title&order=asc", nil)
	w := httptest.NewRecorder()
	handleFiltered(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestRegisterThenLogin(t *testing.T) {
	setupTest(t)

	form := url.Values{"action": {"register"}, "username": {"newuser"}, "password": {"password123"}}
	req := httptest.NewRequest(http.MethodPost, "/auth", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	handleAuth(w, req)

	if w.Code != http.StatusSeeOther {
		t.Errorf("expected 303 on register, got %d", w.Code)
	}

	// Login
	cookies := w.Result().Cookies()
	form = url.Values{"action": {"login"}, "username": {"newuser"}, "password": {"password123"}}
	req = httptest.NewRequest(http.MethodPost, "/auth", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for _, c := range cookies {
		req.AddCookie(c)
	}
	w = httptest.NewRecorder()
	handleAuth(w, req)

	if w.Code != http.StatusSeeOther {
		t.Errorf("expected 303 on login, got %d", w.Code)
	}
}

func TestLoginWrongPassword(t *testing.T) {
	setupTest(t)

	form := url.Values{"action": {"login"}, "username": {"testuser"}, "password": {"wrongpass"}}
	req := httptest.NewRequest(http.MethodPost, "/auth", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	handleAuth(w, req)

	if w.Code != http.StatusSeeOther {
		t.Errorf("expected 303, got %d", w.Code)
	}
	loc := w.Result().Header.Get("Location")
	if !strings.Contains(loc, "login_pass_error") {
		t.Error("expected password error on redirect")
	}
}

func TestLoginNonexistentUser(t *testing.T) {
	setupTest(t)

	form := url.Values{"action": {"login"}, "username": {"nobody"}, "password": {"x"}}
	req := httptest.NewRequest(http.MethodPost, "/auth", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	handleAuth(w, req)

	if w.Code != http.StatusSeeOther {
		t.Errorf("expected 303, got %d", w.Code)
	}
	loc := w.Result().Header.Get("Location")
	if !strings.Contains(loc, "login_user_error") {
		t.Error("expected user error on redirect")
	}
}

func TestRegisterDuplicateUser(t *testing.T) {
	setupTest(t)

	form := url.Values{"action": {"register"}, "username": {"testuser"}, "password": {"password123"}}
	req := httptest.NewRequest(http.MethodPost, "/auth", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	handleAuth(w, req)

	if w.Code != http.StatusSeeOther {
		t.Errorf("expected 303, got %d", w.Code)
	}
	loc := w.Result().Header.Get("Location")
	if !strings.Contains(loc, "register_user_error") {
		t.Error("expected user error on duplicate register")
	}
}

func TestCreatePost(t *testing.T) {
	setupTest(t)

	form := url.Values{"title": {"Test Post Title"}, "message": {"Test message content"}, "csrf_token": {"test-csrf-token"}}
	req := httptest.NewRequest(http.MethodPost, "/post", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	loginUser(t, req)
	w := httptest.NewRecorder()
	handlePost(w, req)

	if w.Code != http.StatusSeeOther {
		t.Errorf("expected 303, got %d", w.Code)
	}

	// Verify post appears on home
	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	w2 := httptest.NewRecorder()
	handleHome(w2, req2)

	if !strings.Contains(w2.Body.String(), "Test Post Title") {
		t.Error("post not found on home page")
	}
}

func TestCreatePostUnauthenticated(t *testing.T) {
	setupTest(t)

	form := url.Values{"title": {"Test"}, "message": {"Content"}}
	req := httptest.NewRequest(http.MethodPost, "/post", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	handlePost(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for unauthenticated post, got %d", w.Code)
	}
}

func TestCreatePostInvalidCSRF(t *testing.T) {
	setupTest(t)

	form := url.Values{"title": {"Test"}, "message": {"Content"}, "csrf_token": {"wrong-token"}}
	req := httptest.NewRequest(http.MethodPost, "/post", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	loginUser(t, req)
	w := httptest.NewRecorder()
	handlePost(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403 for invalid CSRF, got %d", w.Code)
	}
}

func TestViewPost(t *testing.T) {
	setupTest(t)

	db.Exec("INSERT INTO posts (title, user, message, created_at) VALUES (?, ?, ?, ?)", "Viewable Post", "testuser", "Content here", "2025-01-01 12:00")

	req := httptest.NewRequest(http.MethodGet, "/view?id=1", nil)
	w := httptest.NewRecorder()
	handleView(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Viewable Post") {
		t.Error("post title not found on view page")
	}
}

func TestViewPostNotFound(t *testing.T) {
	setupTest(t)

	req := httptest.NewRequest(http.MethodGet, "/view?id=999", nil)
	w := httptest.NewRecorder()
	handleView(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestCommentOnPost(t *testing.T) {
	setupTest(t)

	db.Exec("INSERT INTO posts (title, user, message, created_at) VALUES (?, ?, ?, ?)", "Commentable", "testuser", "Body", "2025-01-01 12:00")

	form := url.Values{"post_id": {"1"}, "parent_id": {"0"}, "message": {"Nice post!"}, "csrf_token": {"test-csrf-token"}}
	req := httptest.NewRequest(http.MethodPost, "/comment", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	loginUser(t, req)
	w := httptest.NewRecorder()
	handleComment(w, req)

	if w.Code != http.StatusSeeOther {
		t.Errorf("expected 303, got %d", w.Code)
	}

	// Verify comment appears
	req2 := httptest.NewRequest(http.MethodGet, "/view?id=1", nil)
	w2 := httptest.NewRecorder()
	handleView(w2, req2)

	if !strings.Contains(w2.Body.String(), "Nice post!") {
		t.Error("comment not found on post view")
	}
}

func TestDeleteComment(t *testing.T) {
	setupTest(t)

	db.Exec("INSERT INTO posts (title, user, message, created_at) VALUES (?, ?, ?, ?)", "Post", "testuser", "Body", "2025-01-01 12:00")
	db.Exec("INSERT INTO comments (post_id, parent_id, user, message, created_at) VALUES (?, ?, ?, ?, ?)", 1, 0, "testuser", "Comment to delete", "2025-01-01 12:30")

	form := url.Values{"comment_id": {"1"}, "csrf_token": {"test-csrf-token"}}
	req := httptest.NewRequest(http.MethodPost, "/delete-comment", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	loginUser(t, req)
	w := httptest.NewRecorder()
	handleDeleteComment(w, req)

	if w.Code != http.StatusSeeOther {
		t.Errorf("expected 303, got %d", w.Code)
	}

	req2 := httptest.NewRequest(http.MethodGet, "/view?id=1", nil)
	w2 := httptest.NewRecorder()
	handleView(w2, req2)

	if strings.Contains(w2.Body.String(), "Comment to delete") {
		t.Error("deleted comment should no longer appear")
	}
}

func TestDeleteCommentUnauthorized(t *testing.T) {
	setupTest(t)

	hashed, _ := bcrypt.GenerateFromPassword([]byte("pass1234"), bcrypt.DefaultCost)
	db.Exec("INSERT INTO users (username, password) VALUES (?, ?)", "otheruser", string(hashed))
	db.Exec("INSERT INTO posts (title, user, message, created_at) VALUES (?, ?, ?, ?)", "Post", "otheruser", "Body", "2025-01-01 12:00")
	db.Exec("INSERT INTO comments (post_id, parent_id, user, message, created_at) VALUES (?, ?, ?, ?, ?)", 1, 0, "otheruser", "Other's comment", "2025-01-01 12:30")

	form := url.Values{"comment_id": {"1"}, "csrf_token": {"test-csrf-token"}}
	req := httptest.NewRequest(http.MethodPost, "/delete-comment", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	loginUser(t, req)
	w := httptest.NewRecorder()
	handleDeleteComment(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestSearchPosts(t *testing.T) {
	setupTest(t)

	db.Exec("INSERT INTO posts (title, user, message, created_at) VALUES (?, ?, ?, ?)", "Linux Tips and Tricks", "testuser", "Some content about Linux", "2025-01-01 12:00")
	db.Exec("INSERT INTO posts (title, user, message, created_at) VALUES (?, ?, ?, ?)", "Windows Discussion", "testuser", "Windows content", "2025-01-01 13:00")

	req := httptest.NewRequest(http.MethodGet, "/search?query=linux", nil)
	w := httptest.NewRecorder()
	handleSearch(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Linux Tips") {
		t.Error("expected Linux post in search results")
	}
	if strings.Contains(w.Body.String(), "Windows") {
		t.Error("Windows post should not appear in Linux search")
	}
}

func TestSearchUsers(t *testing.T) {
	setupTest(t)

	db.Exec("INSERT INTO users (username, password) VALUES (?, ?)", "john_doe", "hash")
	db.Exec("INSERT INTO users (username, password) VALUES (?, ?)", "jane_doe", "hash")

	req := httptest.NewRequest(http.MethodGet, "/search?user=john", nil)
	w := httptest.NewRecorder()
	handleSearch(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "john_doe") {
		t.Error("expected john_doe in user search results")
	}
}

func TestProfilePage(t *testing.T) {
	setupTest(t)

	req := httptest.NewRequest(http.MethodGet, "/user?u=testuser", nil)
	w := httptest.NewRecorder()
	handleUser(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Test description") {
		t.Error("expected description on profile page")
	}
}

func TestProfilePageNotFound(t *testing.T) {
	setupTest(t)

	req := httptest.NewRequest(http.MethodGet, "/user?u=nonexistent", nil)
	w := httptest.NewRecorder()
	handleUser(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestUpdateProfile(t *testing.T) {
	setupTest(t)

	form := url.Values{
		"username":    {"testuser"},
		"description": {"Updated description"},
		"email":       {"updated@example.com"},
		"csrf_token":  {"test-csrf-token"},
	}
	req := httptest.NewRequest(http.MethodPost, "/profile", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	loginUser(t, req)
	w := httptest.NewRecorder()
	handleProfile(w, req)

	if w.Code != http.StatusSeeOther {
		t.Errorf("expected 303, got %d", w.Code)
	}

	req2 := httptest.NewRequest(http.MethodGet, "/user?u=testuser", nil)
	w2 := httptest.NewRecorder()
	handleUser(w2, req2)

	if !strings.Contains(w2.Body.String(), "Updated description") {
		t.Error("updated description not found")
	}
}

func TestSaveAndUnsavePost(t *testing.T) {
	setupTest(t)

	db.Exec("INSERT INTO posts (title, user, message, created_at) VALUES (?, ?, ?, ?)", "Savable", "testuser", "Content", "2025-01-01 12:00")

	// Save
	form := url.Values{"post_id": {"1"}, "csrf_token": {"test-csrf-token"}}
	req := httptest.NewRequest(http.MethodPost, "/save", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	loginUser(t, req)
	w := httptest.NewRecorder()
	handleSave(w, req)

	if w.Code != http.StatusSeeOther {
		t.Errorf("expected 303 on save, got %d", w.Code)
	}

	// Verify saved appears on profile
	req2 := httptest.NewRequest(http.MethodGet, "/user?u=testuser", nil)
	loginUser(t, req2)
	w2 := httptest.NewRecorder()
	handleUser(w2, req2)

	if !strings.Contains(w2.Body.String(), "Savable") {
		t.Error("saved post not found on profile")
	}

	// Unsave
	form = url.Values{"post_id": {"1"}, "csrf_token": {"test-csrf-token"}}
	req3 := httptest.NewRequest(http.MethodPost, "/unsave", strings.NewReader(form.Encode()))
	req3.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	loginUser(t, req3)
	w3 := httptest.NewRecorder()
	handleUnsave(w3, req3)

	if w3.Code != http.StatusSeeOther {
		t.Errorf("expected 303 on unsave, got %d", w3.Code)
	}
}

func TestThemeToggle(t *testing.T) {
	setupTest(t)

	req := httptest.NewRequest(http.MethodGet, "/theme?mode=dark", nil)
	w := httptest.NewRecorder()
	handleTheme(w, req)

	if w.Code != http.StatusSeeOther {
		t.Errorf("expected 303, got %d", w.Code)
	}

	cookies := w.Result().Cookies()
	var themeCookie string
	for _, c := range cookies {
		if c.Name == "theme" {
			themeCookie = c.Value
			break
		}
	}
	if themeCookie != "dark" {
		t.Errorf("expected theme=dark, got %s", themeCookie)
	}
}

func TestConfirmPage(t *testing.T) {
	setupTest(t)

	db.Exec("INSERT INTO posts (title, user, message, created_at) VALUES (?, ?, ?, ?)", "Deletable Post", "testuser", "Content", "2025-01-01 12:00")

	req := httptest.NewRequest(http.MethodGet, "/confirm?id=1", nil)
	w := httptest.NewRecorder()
	handleConfirm(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Deletable Post") {
		t.Error("expected post title on confirm page")
	}
}

func TestDeletePost(t *testing.T) {
	setupTest(t)

	db.Exec("INSERT INTO posts (title, user, message, created_at) VALUES (?, ?, ?, ?)", "Delete Me", "testuser", "Content", "2025-01-01 12:00")

	form := url.Values{"post_id": {"1"}, "title": {"Delete Me"}, "csrf_token": {"test-csrf-token"}}
	req := httptest.NewRequest(http.MethodPost, "/confirm", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	loginUser(t, req)
	w := httptest.NewRecorder()
	handleConfirm(w, req)

	if w.Code != http.StatusSeeOther {
		t.Errorf("expected 303, got %d", w.Code)
	}

	// Verify post is gone
	req2 := httptest.NewRequest(http.MethodGet, "/view?id=1", nil)
	w2 := httptest.NewRecorder()
	handleView(w2, req2)

	if w2.Code != http.StatusNotFound {
		t.Errorf("expected 404 after deletion, got %d", w2.Code)
	}
}

func TestDeletePostWrongTitle(t *testing.T) {
	setupTest(t)

	db.Exec("INSERT INTO posts (title, user, message, created_at) VALUES (?, ?, ?, ?)", "Original Title", "testuser", "Content", "2025-01-01 12:00")

	form := url.Values{"post_id": {"1"}, "title": {"Wrong Title"}, "csrf_token": {"test-csrf-token"}}
	req := httptest.NewRequest(http.MethodPost, "/confirm", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	loginUser(t, req)
	w := httptest.NewRecorder()
	handleConfirm(w, req)

	if w.Code != http.StatusSeeOther {
		t.Errorf("expected 303, got %d", w.Code)
	}
	loc := w.Result().Header.Get("Location")
	if !strings.Contains(loc, "error=") {
		t.Error("expected error param when title doesn't match")
	}
}

func TestDeletePostUnauthorized(t *testing.T) {
	setupTest(t)

	hashed, _ := bcrypt.GenerateFromPassword([]byte("pass1234"), bcrypt.DefaultCost)
	db.Exec("INSERT INTO users (username, password) VALUES (?, ?)", "otheruser", string(hashed))
	db.Exec("INSERT INTO posts (title, user, message, created_at) VALUES (?, ?, ?, ?)", "Their Post", "otheruser", "Content", "2025-01-01 12:00")

	form := url.Values{"post_id": {"1"}, "title": {"Their Post"}, "csrf_token": {"test-csrf-token"}}
	req := httptest.NewRequest(http.MethodPost, "/confirm", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	loginUser(t, req)
	w := httptest.NewRecorder()
	handleConfirm(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestCSRFProtectionOnPost(t *testing.T) {
	setupTest(t)

	db.Exec("INSERT INTO posts (title, user, message, created_at) VALUES (?, ?, ?, ?)", "Post", "testuser", "Content", "2025-01-01 12:00")

	// Try to delete without CSRF
	form := url.Values{"post_id": {"1"}, "title": {"Post"}}
	req := httptest.NewRequest(http.MethodPost, "/confirm", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	loginUser(t, req)
	w := httptest.NewRecorder()
	handleConfirm(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403 without CSRF, got %d", w.Code)
	}
}

func TestLogout(t *testing.T) {
	setupTest(t)

	req := httptest.NewRequest(http.MethodGet, "/logout", nil)
	w := httptest.NewRecorder()
	handleLogout(w, req)

	if w.Code != http.StatusSeeOther {
		t.Errorf("expected 303, got %d", w.Code)
	}

	cookies := w.Result().Cookies()
	for _, c := range cookies {
		if c.Name == config.SessionTokenName {
			if c.MaxAge != -1 {
				t.Error("expected session cookie to be cleared")
			}
		}
	}
}
