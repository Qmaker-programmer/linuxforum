package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/crypto/bcrypt"
)

type Post struct {
	ID      int
	Title   string
	User    string
	Message string
	Time    string
}

type Comment struct {
	ID       int
	PostID   int
	ParentID int
	User     string
	Message  string
	Time     string
	Deleted  bool
}

type CommentNode struct {
	Comment    Comment
	Children   []*CommentNode
	PostID     int
	LoggedUser string
}

type User struct {
	Username     string
	Password     string
	Description  string
	SavedPostIDs []int
}

type Session struct {
	Username  string
	ExpiresAt time.Time
}

var db *sql.DB
var sessions = make(map[string]Session)

const searchQueryCookie = "search_query"
const commentSearchCookie = "comment_search_query"

type Config struct {
	RateLimit            int    `json:"rate_limit"`
	ResetMinutes         int    `json:"reset_minutes"`
	Port                 int    `json:"port"`
	DBPath               string `json:"db_path"`
	HTTPS                bool   `json:"https"`
	CertFile             string `json:"cert_file"`
	KeyFile              string `json:"key_file"`
	SessionTokenName     string `json:"session_token_name"`
	SessionExpireMinutes int    `json:"session_expire_minutes"`
}

var config Config

var requestCounts = make(map[string]int)
var mu sync.Mutex

func loadConfig() {
	config = Config{
		Port:                 8080,
		DBPath:               "forum.db",
		CertFile:             "cert.pem",
		KeyFile:              "key.pem",
		SessionTokenName:     "session_token",
		RateLimit:            100,
		ResetMinutes:         1,
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

func initDB() {
	var err error
	db, err = sql.Open("sqlite3", config.DBPath)
	if err != nil {
		fmt.Println("Error al abrir base de datos:", err)
		os.Exit(1)
	}
	db.Exec("PRAGMA journal_mode=WAL")

	schema := `
	CREATE TABLE IF NOT EXISTS posts (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		title TEXT NOT NULL,
		user TEXT NOT NULL,
		message TEXT NOT NULL,
		created_at TEXT NOT NULL
	);
	CREATE TABLE IF NOT EXISTS comments (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		post_id INTEGER NOT NULL,
		parent_id INTEGER NOT NULL DEFAULT 0,
		user TEXT NOT NULL,
		message TEXT NOT NULL,
		created_at TEXT NOT NULL,
		deleted INTEGER NOT NULL DEFAULT 0
	);
	CREATE TABLE IF NOT EXISTS users (
		username TEXT PRIMARY KEY,
		password TEXT NOT NULL,
		description TEXT NOT NULL DEFAULT ''
	);
	CREATE TABLE IF NOT EXISTS saved_posts (
		username TEXT NOT NULL,
		post_id INTEGER NOT NULL,
		PRIMARY KEY (username, post_id)
	);`

	if _, err := db.Exec(schema); err != nil {
		fmt.Println("Error al crear tablas:", err)
		os.Exit(1)
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
		now := time.Now()
		mu.Lock()
		for token, session := range sessions {
			if now.After(session.ExpiresAt) {
				delete(sessions, token)
			}
		}
		mu.Unlock()
	}
}

func getLoggedUser(r *http.Request) string {
	cookie, err := r.Cookie(config.SessionTokenName)
	if err != nil {
		return ""
	}
	session, ok := sessions[cookie.Value]
	if !ok {
		return ""
	}
	if config.SessionExpireMinutes > 0 && time.Now().After(session.ExpiresAt) {
		delete(sessions, cookie.Value)
		return ""
	}
	return session.Username
}

func getSearchQueryFromCookie(r *http.Request) string {
	cookie, err := r.Cookie(searchQueryCookie)
	if err != nil {
		return ""
	}
	return cookie.Value
}

func setSearchQueryCookie(w http.ResponseWriter, query string) {
	if query == "" {
		http.SetCookie(w, &http.Cookie{
			Name:   searchQueryCookie,
			Value:  "",
			Path:   "/",
			MaxAge: -1,
		})
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:  searchQueryCookie,
		Value: query,
		Path:  "/",
	})
}

func getCommentSearchFromCookie(r *http.Request) string {
	cookie, err := r.Cookie(commentSearchCookie)
	if err != nil {
		return ""
	}
	return cookie.Value
}

func setCommentSearchCookie(w http.ResponseWriter, query string) {
	if query == "" {
		http.SetCookie(w, &http.Cookie{
			Name:   commentSearchCookie,
			Value:  "",
			Path:   "/",
			MaxAge: -1,
		})
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:  commentSearchCookie,
		Value: query,
		Path:  "/",
	})
}

func pageContext(r *http.Request) (query, loggedUser string) {
	return getSearchQueryFromCookie(r), getLoggedUser(r)
}

func redirectToLogin(w http.ResponseWriter, r *http.Request, params url.Values) {
	http.Redirect(w, r, "/web/login.html?"+params.Encode(), http.StatusSeeOther)
}

// --- Database access functions ---

func getAllPosts() []Post {
	rows, err := db.Query("SELECT id, title, user, message, created_at FROM posts ORDER BY created_at DESC")
	if err != nil {
		return nil
	}
	defer rows.Close()
	var result []Post
	for rows.Next() {
		var p Post
		if err := rows.Scan(&p.ID, &p.Title, &p.User, &p.Message, &p.Time); err != nil {
			continue
		}
		result = append(result, p)
	}
	return result
}

func getPostByID(id int) *Post {
	row := db.QueryRow("SELECT id, title, user, message, created_at FROM posts WHERE id = ?", id)
	var p Post
	if err := row.Scan(&p.ID, &p.Title, &p.User, &p.Message, &p.Time); err != nil {
		return nil
	}
	return &p
}

func getCommentByID(id int) *Comment {
	row := db.QueryRow("SELECT id, post_id, parent_id, user, message, created_at, deleted FROM comments WHERE id = ?", id)
	var c Comment
	var deleted int
	if err := row.Scan(&c.ID, &c.PostID, &c.ParentID, &c.User, &c.Message, &c.Time, &deleted); err != nil {
		return nil
	}
	c.Deleted = deleted == 1
	return &c
}

func getCommentsForPost(postID int) []Comment {
	rows, err := db.Query("SELECT id, post_id, parent_id, user, message, created_at, deleted FROM comments WHERE post_id = ? ORDER BY created_at ASC", postID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var result []Comment
	for rows.Next() {
		var c Comment
		var deleted int
		if err := rows.Scan(&c.ID, &c.PostID, &c.ParentID, &c.User, &c.Message, &c.Time, &deleted); err != nil {
			continue
		}
		c.Deleted = deleted == 1
		result = append(result, c)
	}
	return result
}

func getUserPosts(username string) []Post {
	rows, err := db.Query("SELECT id, title, user, message, created_at FROM posts WHERE user = ? ORDER BY created_at DESC", username)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var result []Post
	for rows.Next() {
		var p Post
		if err := rows.Scan(&p.ID, &p.Title, &p.User, &p.Message, &p.Time); err != nil {
			continue
		}
		result = append(result, p)
	}
	return result
}

func getSavedPosts(username string) []Post {
	rows, err := db.Query("SELECT p.id, p.title, p.user, p.message, p.created_at FROM posts p JOIN saved_posts s ON p.id = s.post_id WHERE s.username = ? ORDER BY p.created_at DESC", username)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var result []Post
	for rows.Next() {
		var p Post
		if err := rows.Scan(&p.ID, &p.Title, &p.User, &p.Message, &p.Time); err != nil {
			continue
		}
		result = append(result, p)
	}
	return result
}

func isPostSaved(username string, postID int) bool {
	var count int
	db.QueryRow("SELECT COUNT(*) FROM saved_posts WHERE username = ? AND post_id = ?", username, postID).Scan(&count)
	return count > 0
}

func existsUser(username string) bool {
	var count int
	db.QueryRow("SELECT COUNT(*) FROM users WHERE username = ?", username).Scan(&count)
	return count > 0
}

func getUser(username string) *User {
	row := db.QueryRow("SELECT username, password, description FROM users WHERE username = ?", username)
	var u User
	if err := row.Scan(&u.Username, &u.Password, &u.Description); err != nil {
		return nil
	}
	rows, err := db.Query("SELECT post_id FROM saved_posts WHERE username = ?", username)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var id int
			rows.Scan(&id)
			u.SavedPostIDs = append(u.SavedPostIDs, id)
		}
	}
	return &u
}

func renameUser(oldName, newName string) error {
	if oldName == newName {
		return nil
	}
	if existsUser(newName) {
		return fmt.Errorf("El nombre ya está en uso.")
	}

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("Error al iniciar transacción")
	}
	defer tx.Rollback()

	tx.Exec("INSERT INTO users (username, password, description) SELECT ?, password, description FROM users WHERE username = ?", newName, oldName)
	tx.Exec("DELETE FROM users WHERE username = ?", oldName)
	tx.Exec("UPDATE posts SET user = ? WHERE user = ?", newName, oldName)
	tx.Exec("UPDATE comments SET user = ? WHERE user = ?", newName, oldName)
	tx.Exec("UPDATE saved_posts SET username = ? WHERE username = ?", newName, oldName)

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("Error al renombrar usuario")
	}

	for token, session := range sessions {
		if session.Username == oldName {
			session.Username = newName
			sessions[token] = session
		}
	}
	return nil
}

// --- Comment pruning ---

func subtreeIsDead(id int) bool {
	var deleted int
	db.QueryRow("SELECT deleted FROM comments WHERE id = ?", id).Scan(&deleted)
	if deleted == 0 {
		return false
	}
	rows, err := db.Query("SELECT id FROM comments WHERE parent_id = ?", id)
	if err != nil {
		return true
	}
	defer rows.Close()
	for rows.Next() {
		var childID int
		rows.Scan(&childID)
		if !subtreeIsDead(childID) {
			return false
		}
	}
	return true
}

func removeDeadSubtree(id int) {
	rows, err := db.Query("SELECT id FROM comments WHERE parent_id = ?", id)
	if err != nil {
		return
	}
	defer rows.Close()
	var childIDs []int
	for rows.Next() {
		var cid int
		rows.Scan(&cid)
		childIDs = append(childIDs, cid)
	}
	for _, childID := range childIDs {
		removeDeadSubtree(childID)
	}
	db.Exec("DELETE FROM comments WHERE id = ?", id)
}

func checkAndPruneUpward(id int) {
	var deleted int
	var parentID int
	db.QueryRow("SELECT deleted, parent_id FROM comments WHERE id = ?", id).Scan(&deleted, &parentID)
	if deleted == 0 {
		return
	}
	if !subtreeIsDead(id) {
		return
	}
	removeDeadSubtree(id)
	if parentID != 0 {
		checkAndPruneUpward(parentID)
	}
}

func deleteCommentAndPrune(commentID int) {
	db.Exec("UPDATE comments SET deleted = 1, message = '[eliminado]' WHERE id = ?", commentID)
	checkAndPruneUpward(commentID)
}

// --- Template helpers ---

func buildCommentTree(all []Comment, parentID, postID int, loggedUser string) []*CommentNode {
	var nodes []*CommentNode
	for _, c := range all {
		if c.ParentID == parentID {
			nodes = append(nodes, &CommentNode{
				Comment:    c,
				PostID:     postID,
				LoggedUser: loggedUser,
				Children:   buildCommentTree(all, c.ID, postID, loggedUser),
			})
		}
	}
	return nodes
}

func filterComments(all []Comment, query string) []Comment {
	if query == "" {
		return nil
	}
	queryLower := strings.ToLower(query)
	var result []Comment
	for _, c := range all {
		if strings.Contains(strings.ToLower(c.Message), queryLower) {
			result = append(result, c)
		}
	}
	return result
}

func sortPosts(posts []Post, sortBy, order string) []Post {
	result := make([]Post, len(posts))
	copy(result, posts)

	sort.SliceStable(result, func(i, j int) bool {
		var less bool
		if sortBy == "title" {
			less = strings.ToLower(result[i].Title) < strings.ToLower(result[j].Title)
		} else {
			less = result[i].Time < result[j].Time
		}
		if order == "desc" {
			return !less
		}
		return less
	})
	return result
}

// --- Helper for searching users ---

func searchUsers(query string) []string {
	var result []string
	rows, err := db.Query("SELECT username FROM users WHERE LOWER(username) LIKE ?", "%"+strings.ToLower(query)+"%")
	if err != nil {
		return nil
	}
	defer rows.Close()
	for rows.Next() {
		var u string
		rows.Scan(&u)
		result = append(result, u)
	}
	return result
}

// --- Main ---

func main() {
	loadConfig()
	initDB()

	var userCount int
	db.QueryRow("SELECT COUNT(*) FROM users").Scan(&userCount)
	if userCount == 0 {
		hashedPassword, _ := bcrypt.GenerateFromPassword([]byte("1234"), bcrypt.DefaultCost)
		db.Exec("INSERT INTO users (username, password, description) VALUES (?, ?, ?)", "admin", string(hashedPassword), "Administrador del foro.")
		now := time.Now().Format("2006-01-02 15:04")
		db.Exec("INSERT INTO posts (title, user, message, created_at) VALUES (?, ?, ?, ?)", "¡Bienvenidos al nuevo foro!", "admin", "Este es el contenido completo del primer post de prueba.", now)
	}

	renderPage := func(w http.ResponseWriter, pageFile string, data any) {
		tmpl, err := template.ParseFiles("web/head.html", "web/upbar.html", pageFile)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := tmpl.ExecuteTemplate(w, filepath.Base(pageFile), data); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}

	http.HandleFunc("/web/login.html", func(w http.ResponseWriter, r *http.Request) {
		query, loggedUser := pageContext(r)
		if loggedUser != "" {
			http.Redirect(w, r, "/user?u="+url.QueryEscape(loggedUser), http.StatusSeeOther)
			return
		}
		renderPage(w, "web/login.html", struct {
			Query             string
			LoggedUser        string
			LoginUsername     string
			RegisterUsername  string
			LoginUserError    string
			LoginPassError    string
			RegisterUserError string
			RegisterPassError string
		}{
			Query:             query,
			LoggedUser:        loggedUser,
			LoginUsername:     r.URL.Query().Get("login_user"),
			RegisterUsername:  r.URL.Query().Get("register_user"),
			LoginUserError:    r.URL.Query().Get("login_user_error"),
			LoginPassError:    r.URL.Query().Get("login_pass_error"),
			RegisterUserError: r.URL.Query().Get("register_user_error"),
			RegisterPassError: r.URL.Query().Get("register_pass_error"),
		})
	})

	http.HandleFunc("/web/public.html", func(w http.ResponseWriter, r *http.Request) {
		query, loggedUser := pageContext(r)
		if loggedUser == "" {
			http.Redirect(w, r, "/web/login.html", http.StatusSeeOther)
			return
		}
		renderPage(w, "web/public.html", struct {
			Query      string
			LoggedUser string
		}{Query: query, LoggedUser: loggedUser})
	})

	fs := http.FileServer(http.Dir("./web"))
	http.Handle("/web/", http.StripPrefix("/web/", fs))

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		query, loggedUser := pageContext(r)
		allPosts := getAllPosts()
		renderPage(w, "web/index.html", struct {
			Posts      []Post
			Query      string
			LoggedUser string
		}{
			Posts:      allPosts,
			Query:      query,
			LoggedUser: loggedUser,
		})
	})

	http.HandleFunc("/filtered", func(w http.ResponseWriter, r *http.Request) {
		sortBy := r.URL.Query().Get("sort_by")
		if sortBy == "" {
			sortBy = "date"
		}
		order := r.URL.Query().Get("order")
		if order == "" {
			order = "asc"
		}

		query, loggedUser := pageContext(r)
		allPosts := getAllPosts()
		sorted := sortPosts(allPosts, sortBy, order)
		renderPage(w, "web/filtered.html", struct {
			Posts      []Post
			Query      string
			LoggedUser string
			SortBy     string
			Order      string
		}{
			Posts:      sorted,
			Query:      query,
			LoggedUser: loggedUser,
			SortBy:     sortBy,
			Order:      order,
		})
	})

	http.HandleFunc("/auth", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Redirect(w, r, "/web/login.html", http.StatusSeeOther)
			return
		}

		action := r.FormValue("action")
		username := strings.TrimSpace(r.FormValue("username"))
		password := r.FormValue("password")

		if action == "register" {
			params := url.Values{}
			params.Set("register_user", username)

			if username == "" {
				params.Set("register_user_error", "El nombre no puede estar vacío.")
				redirectToLogin(w, r, params)
				return
			}
			if password == "" {
				params.Set("register_pass_error", "La contraseña no puede estar vacía.")
				redirectToLogin(w, r, params)
				return
			}
			if existsUser(username) {
				params.Set("register_user_error", "Ese nombre ya está en uso.")
				redirectToLogin(w, r, params)
				return
			}
			hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
			if err != nil {
				params.Set("register_user_error", "Error al crear la cuenta. Inténtalo de nuevo.")
				redirectToLogin(w, r, params)
				return
			}
			db.Exec("INSERT INTO users (username, password, description) VALUES (?, ?, ?)", username, string(hashed), "")

			token := strconv.FormatInt(time.Now().UnixNano(), 10)
			sessions[token] = Session{
				Username:  username,
				ExpiresAt: sessionExpiry(),
			}

			http.SetCookie(w, &http.Cookie{
				Name:  config.SessionTokenName,
				Value: token,
				Path:  "/",
			})
			fmt.Println("Usuario registrado con éxito de forma encriptada.")
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}

		if action == "login" {
			params := url.Values{}
			params.Set("login_user", username)

			if username == "" {
				params.Set("login_user_error", "Indica tu nombre de usuario.")
				redirectToLogin(w, r, params)
				return
			}
			if password == "" {
				params.Set("login_pass_error", "Indica tu contraseña.")
				redirectToLogin(w, r, params)
				return
			}

			user := getUser(username)
			if user == nil {
				params.Set("login_user_error", "Ese usuario no existe.")
				redirectToLogin(w, r, params)
				return
			}

			err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password))
			if err != nil {
				params.Set("login_pass_error", "Contraseña incorrecta.")
				redirectToLogin(w, r, params)
				return
			}

			token := strconv.FormatInt(time.Now().UnixNano(), 10)
			sessions[token] = Session{
				Username:  username,
				ExpiresAt: sessionExpiry(),
			}

			http.SetCookie(w, &http.Cookie{
				Name:  config.SessionTokenName,
				Value: token,
				Path:  "/",
			})
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}

		http.Redirect(w, r, "/web/login.html", http.StatusSeeOther)
	})

	http.HandleFunc("/logout", func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(config.SessionTokenName)
		if err == nil {
			delete(sessions, cookie.Value)
		}
		http.SetCookie(w, &http.Cookie{
			Name:   config.SessionTokenName,
			Value:  "",
			Path:   "/",
			MaxAge: -1,
		})
		http.Redirect(w, r, "/", http.StatusSeeOther)
	})

	http.HandleFunc("/post", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			username := getLoggedUser(r)
			if username == "" {
				http.Error(w, "Debes iniciar sesión para publicar", http.StatusUnauthorized)
				return
			}

			title := r.FormValue("title")
			message := r.FormValue("message")

			if title != "" && message != "" {
				now := time.Now().Format("2006-01-02 15:04")
				db.Exec("INSERT INTO posts (title, user, message, created_at) VALUES (?, ?, ?, ?)", title, username, message, now)
			}
		}
		http.Redirect(w, r, "/", http.StatusSeeOther)
	})

	http.HandleFunc("/comment", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}

		username := getLoggedUser(r)
		if username == "" {
			http.Redirect(w, r, "/web/login.html", http.StatusSeeOther)
			return
		}

		postID, err := strconv.Atoi(r.FormValue("post_id"))
		if err != nil || getPostByID(postID) == nil {
			http.Error(w, "Post no válido", http.StatusBadRequest)
			return
		}

		parentID, _ := strconv.Atoi(r.FormValue("parent_id"))
		if parentID != 0 {
			parent := getCommentByID(parentID)
			if parent == nil || parent.PostID != postID {
				http.Error(w, "Comentario padre no válido", http.StatusBadRequest)
				return
			}
		}

		message := strings.TrimSpace(r.FormValue("message"))
		if message == "" {
			http.Redirect(w, r, "/view?id="+strconv.Itoa(postID), http.StatusSeeOther)
			return
		}

		now := time.Now().Format("2006-01-02 15:04")
		db.Exec("INSERT INTO comments (post_id, parent_id, user, message, created_at) VALUES (?, ?, ?, ?, ?)", postID, parentID, username, message, now)

		http.Redirect(w, r, "/view?id="+strconv.Itoa(postID), http.StatusSeeOther)
	})

	http.HandleFunc("/delete-comment", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}

		username := getLoggedUser(r)
		if username == "" {
			http.Redirect(w, r, "/web/login.html", http.StatusSeeOther)
			return
		}

		commentID, err := strconv.Atoi(r.FormValue("comment_id"))
		if err != nil {
			http.Error(w, "Comentario no válido", http.StatusBadRequest)
			return
		}

		comment := getCommentByID(commentID)
		if comment == nil {
			http.Error(w, "Comentario no encontrado", http.StatusNotFound)
			return
		}

		if comment.User != username {
			http.Error(w, "No eres el autor de este comentario", http.StatusForbidden)
			return
		}

		postID := comment.PostID
		deleteCommentAndPrune(commentID)
		http.Redirect(w, r, "/view?id="+strconv.Itoa(postID), http.StatusSeeOther)
	})

	http.HandleFunc("/search", func(w http.ResponseWriter, r *http.Request) {
		searchQuery := r.URL.Query().Get("query")
		if searchQuery != "" {
			setSearchQueryCookie(w, searchQuery)
		} else {
			searchQuery = getSearchQueryFromCookie(r)
		}

		var matchedPosts []Post
		if searchQuery != "" {
			rows, err := db.Query("SELECT id, title, user, message, created_at FROM posts WHERE LOWER(title) LIKE ? ORDER BY created_at DESC", "%"+strings.ToLower(searchQuery)+"%")
			if err == nil {
				defer rows.Close()
				for rows.Next() {
					var p Post
					rows.Scan(&p.ID, &p.Title, &p.User, &p.Message, &p.Time)
					matchedPosts = append(matchedPosts, p)
				}
			}
		}

		userQuery := r.URL.Query().Get("user")
		var matchedUsers []string
		if userQuery != "" {
			matchedUsers = searchUsers(userQuery)
		}

		sortBy := r.URL.Query().Get("sort_by")
		if sortBy == "" {
			sortBy = "date"
		}
		order := r.URL.Query().Get("order")
		if order == "" {
			order = "asc"
		}

		matchedPosts = sortPosts(matchedPosts, sortBy, order)

		_, loggedUser := pageContext(r)
		renderPage(w, "web/search.html", struct {
			Query      string
			Posts      []Post
			Users      []string
			UserQuery  string
			LoggedUser string
			SortBy     string
			Order      string
		}{
			Query:      searchQuery,
			Posts:      matchedPosts,
			Users:      matchedUsers,
			UserQuery:  userQuery,
			LoggedUser: loggedUser,
			SortBy:     sortBy,
			Order:      order,
		})
	})

	http.HandleFunc("/view", func(w http.ResponseWriter, r *http.Request) {
		idStr := r.URL.Query().Get("id")
		id, err := strconv.Atoi(idStr)
		if err != nil {
			http.Error(w, "ID inválido", http.StatusBadRequest)
			return
		}

		if r.URL.Query().Get("clear_cquery") == "1" {
			setCommentSearchCookie(w, "")
		}

		fromPage := r.URL.Query().Get("from")
		searchQuery := r.URL.Query().Get("query")
		if searchQuery == "" {
			searchQuery = getSearchQueryFromCookie(r)
		}

		commentQuery := r.URL.Query().Get("cquery")
		if commentQuery != "" {
			setCommentSearchCookie(w, commentQuery)
		} else if r.URL.Query().Get("clear_cquery") != "1" {
			commentQuery = getCommentSearchFromCookie(r)
		}

		foundPost := getPostByID(id)
		if foundPost == nil {
			http.Error(w, "Post no encontrado", http.StatusNotFound)
			return
		}

		postComments := getCommentsForPost(id)
		query, loggedUser := pageContext(r)

		renderPage(w, "web/post.html", struct {
			Post             *Post
			From             string
			SearchQuery      string
			Query            string
			LoggedUser       string
			IsSaved          bool
			IsAuthor         bool
			CommentQuery     string
			CommentTree      []*CommentNode
			MatchedComments  []Comment
		}{
			Post:            foundPost,
			From:            fromPage,
			SearchQuery:     searchQuery,
			Query:           query,
			LoggedUser:      loggedUser,
			IsSaved:         isPostSaved(loggedUser, id),
			IsAuthor:        loggedUser == foundPost.User,
			CommentQuery:    commentQuery,
			CommentTree:     buildCommentTree(postComments, 0, id, loggedUser),
			MatchedComments: filterComments(postComments, commentQuery),
		})
	})

	http.HandleFunc("/confirm", func(w http.ResponseWriter, r *http.Request) {
		query, loggedUser := pageContext(r)

		idStr := r.URL.Query().Get("id")
		if r.Method == http.MethodPost {
			idStr = r.FormValue("post_id")
		}

		id, err := strconv.Atoi(idStr)
		if err != nil {
			http.Error(w, "ID inválido", http.StatusBadRequest)
			return
		}

		post := getPostByID(id)
		if post == nil {
			http.Error(w, "Post no encontrado", http.StatusNotFound)
			return
		}

		if r.Method == http.MethodPost {
			if loggedUser == "" {
				http.Redirect(w, r, "/web/login.html", http.StatusSeeOther)
				return
			}
			if post.User != loggedUser {
				http.Error(w, "No eres el autor de este post", http.StatusForbidden)
				return
			}

			title := r.FormValue("title")
			if title != post.Title {
				http.Redirect(w, r, "/confirm?id="+idStr+"&error="+url.QueryEscape("El título no coincide."), http.StatusSeeOther)
				return
			}

			db.Exec("DELETE FROM comments WHERE post_id = ?", post.ID)
			db.Exec("DELETE FROM saved_posts WHERE post_id = ?", post.ID)
			db.Exec("DELETE FROM posts WHERE id = ?", post.ID)

			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}

		renderPage(w, "web/confirm.html", struct {
			Query      string
			LoggedUser string
			Post       *Post
			Error      string
		}{
			Query:      query,
			LoggedUser: loggedUser,
			Post:       post,
			Error:      r.URL.Query().Get("error"),
		})
	})

	http.HandleFunc("/user", func(w http.ResponseWriter, r *http.Request) {
		username := strings.TrimSpace(r.URL.Query().Get("u"))
		if username == "" {
			http.Error(w, "Usuario no especificado", http.StatusBadRequest)
			return
		}

		user := getUser(username)
		if user == nil {
			http.Error(w, "Usuario no encontrado", http.StatusNotFound)
			return
		}

		query, loggedUser := pageContext(r)
		isOwn := loggedUser == username

		data := struct {
			Query        string
			LoggedUser   string
			ProfileUser  User
			Posts        []Post
			SavedPosts   []Post
			IsOwnProfile bool
			Error        string
		}{
			Query:        query,
			LoggedUser:   loggedUser,
			ProfileUser:  *user,
			Posts:        getUserPosts(username),
			IsOwnProfile: isOwn,
			Error:        r.URL.Query().Get("error"),
		}
		if isOwn {
			data.SavedPosts = getSavedPosts(username)
		}

		renderPage(w, "web/user.html", data)
	})

	http.HandleFunc("/profile", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}

		loggedUser := getLoggedUser(r)
		if loggedUser == "" {
			http.Error(w, "Debes iniciar sesión", http.StatusUnauthorized)
			return
		}

		newUsername := strings.TrimSpace(r.FormValue("username"))
		description := r.FormValue("description")

		if newUsername == "" {
			http.Redirect(w, r, "/user?u="+url.QueryEscape(loggedUser)+"&error="+url.QueryEscape("El nombre no puede estar vacío."), http.StatusSeeOther)
			return
		}

		if newUsername != loggedUser {
			if err := renameUser(loggedUser, newUsername); err != nil {
				http.Redirect(w, r, "/user?u="+url.QueryEscape(loggedUser)+"&error="+url.QueryEscape(err.Error()), http.StatusSeeOther)
				return
			}
			loggedUser = newUsername
		}

		db.Exec("UPDATE users SET description = ? WHERE username = ?", description, loggedUser)
		http.Redirect(w, r, "/user?u="+url.QueryEscape(loggedUser), http.StatusSeeOther)
	})

	http.HandleFunc("/save", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}

		loggedUser := getLoggedUser(r)
		if loggedUser == "" {
			http.Error(w, "Debes iniciar sesión", http.StatusUnauthorized)
			return
		}

		postID, err := strconv.Atoi(r.FormValue("post_id"))
		if err != nil || getPostByID(postID) == nil {
			http.Error(w, "Post no válido", http.StatusBadRequest)
			return
		}

		if !isPostSaved(loggedUser, postID) {
			db.Exec("INSERT OR IGNORE INTO saved_posts (username, post_id) VALUES (?, ?)", loggedUser, postID)
		}

		returnURL := r.FormValue("return")
		if returnURL == "" {
			returnURL = "/"
		}
		http.Redirect(w, r, returnURL, http.StatusSeeOther)
	})

	http.HandleFunc("/unsave", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}

		loggedUser := getLoggedUser(r)
		if loggedUser == "" {
			http.Error(w, "Debes iniciar sesión", http.StatusUnauthorized)
			return
		}

		postID, err := strconv.Atoi(r.FormValue("post_id"))
		if err != nil {
			http.Error(w, "Post no válido", http.StatusBadRequest)
			return
		}

		db.Exec("DELETE FROM saved_posts WHERE username = ? AND post_id = ?", loggedUser, postID)

		returnURL := r.FormValue("return")
		if returnURL == "" {
			returnURL = "/user?u=" + url.QueryEscape(loggedUser)
		}
		http.Redirect(w, r, returnURL, http.StatusSeeOther)
	})

	go resetRequestCounts()
	go cleanupExpiredSessions()

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
