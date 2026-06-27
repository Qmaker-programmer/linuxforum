package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

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

var posts []Post
var comments []Comment
var nextID = 1
var nextCommentID = 1
var users = make(map[string]User)
var sessions = make(map[string]string)

const searchQueryCookie = "search_query"
const commentSearchCookie = "comment_search_query"

var requestCounts = make(map[string]int)
var mu sync.Mutex

const rateLimitPerMin = 100

func rateLimitMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := r.RemoteAddr
		if idx := strings.LastIndex(ip, ":"); idx != -1 {
			ip = ip[:idx]
		}

		mu.Lock()
		count := requestCounts[ip]
		if count >= rateLimitPerMin {
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
		time.Sleep(1 * time.Minute)
		mu.Lock()
		requestCounts = make(map[string]int)
		mu.Unlock()
	}
}

const dataFile = "forum.json"

type persistedData struct {
	Posts         []Post          `json:"posts"`
	Comments      []Comment       `json:"comments"`
	NextID        int             `json:"next_id"`
	NextCommentID int             `json:"next_comment_id"`
	Users         map[string]User `json:"users"`
}

func saveData() {
	data := persistedData{
		Posts:         posts,
		Comments:      comments,
		NextID:        nextID,
		NextCommentID: nextCommentID,
		Users:         users,
	}

	tmpFile := dataFile + ".tmp"
	f, err := os.Create(tmpFile)
	if err != nil {
		fmt.Println("Error al guardar datos:", err)
		return
	}
	defer f.Close()

	if err := json.NewEncoder(f).Encode(data); err != nil {
		fmt.Println("Error al codificar datos:", err)
		return
	}
	if err := f.Sync(); err != nil {
		fmt.Println("Error al sincronizar datos:", err)
		return
	}
	if err := os.Rename(tmpFile, dataFile); err != nil {
		fmt.Println("Error al renombrar archivo temporal:", err)
	}
}

func loadData() {
	f, err := os.Open(dataFile)
	if err != nil {
		if os.IsNotExist(err) {
			return
		}
		fmt.Println("Error al cargar datos:", err)
		return
	}
	defer f.Close()

	var data persistedData
	if err := json.NewDecoder(f).Decode(&data); err != nil {
		fmt.Println("Error al decodificar datos:", err)
		return
	}

	posts = data.Posts
	comments = data.Comments
	nextID = data.NextID
	nextCommentID = data.NextCommentID
	users = data.Users
}

func getLoggedUser(r *http.Request) string {
	cookie, err := r.Cookie("session_token")
	if err != nil {
		return ""
	}
	return sessions[cookie.Value]
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

func getUserPosts(username string) []Post {
	var result []Post
	for _, p := range posts {
		if p.User == username {
			result = append(result, p)
		}
	}
	return result
}

func getPostByID(id int) *Post {
	for _, p := range posts {
		if p.ID == id {
			copy := p
			return &copy
		}
	}
	return nil
}

func getCommentByID(id int) *Comment {
	for _, c := range comments {
		if c.ID == id {
			copy := c
			return &copy
		}
	}
	return nil
}

func getCommentsForPost(postID int) []Comment {
	var result []Comment
	for _, c := range comments {
		if c.PostID == postID {
			result = append(result, c)
		}
	}
	return result
}

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

func getSavedPosts(username string) []Post {
	user, ok := users[username]
	if !ok {
		return nil
	}
	var result []Post
	for _, id := range user.SavedPostIDs {
		if p := getPostByID(id); p != nil {
			result = append(result, *p)
		}
	}
	return result
}

func isPostSaved(username string, postID int) bool {
	user, ok := users[username]
	if !ok {
		return false
	}
	for _, id := range user.SavedPostIDs {
		if id == postID {
			return true
		}
	}
	return false
}

func renameUser(oldName, newName string) error {
	if _, exists := users[newName]; exists {
		return fmt.Errorf("El nombre ya está en uso.")
	}
	user, ok := users[oldName]
	if !ok {
		return fmt.Errorf("Usuario no encontrado")
	}
	user.Username = newName
	users[newName] = user
	delete(users, oldName)

	for i := range posts {
		if posts[i].User == oldName {
			posts[i].User = newName
		}
	}
	for i := range comments {
		if comments[i].User == oldName {
			comments[i].User = newName
		}
	}
	for token, uname := range sessions {
		if uname == oldName {
			sessions[token] = newName
		}
	}
	return nil
}

func subtreeIsDead(id int) bool {
	// Checks if a comment and ALL its descendants are deleted or removed
	for _, c := range comments {
		if c.ID == id {
			if !c.Deleted {
				return false
			}
			break
		}
	}
	for _, c := range comments {
		if c.ParentID == id {
			if !subtreeIsDead(c.ID) {
				return false
			}
		}
	}
	return true
}

func removeDeadSubtree(id int) {
	var childIDs []int
	for _, c := range comments {
		if c.ParentID == id {
			childIDs = append(childIDs, c.ID)
		}
	}
	for _, childID := range childIDs {
		removeDeadSubtree(childID)
	}
	for i, c := range comments {
		if c.ID == id {
			comments = append(comments[:i], comments[i+1:]...)
			break
		}
	}
}

func checkAndPruneUpward(id int) {
	isDeleted := false
	parentID := 0
	for _, c := range comments {
		if c.ID == id {
			isDeleted = c.Deleted
			parentID = c.ParentID
			break
		}
	}
	if !isDeleted {
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
	for i, c := range comments {
		if c.ID == commentID {
			comments[i].Deleted = true
			comments[i].Message = "[eliminado]"
			break
		}
	}
	checkAndPruneUpward(commentID)
}

func main() {
	loadData()

	if len(users) == 0 {
		hashedPassword, _ := bcrypt.GenerateFromPassword([]byte("1234"), bcrypt.DefaultCost)
		users["admin"] = User{
			Username:     "admin",
			Password:     string(hashedPassword),
			Description:  "Administrador del foro.",
			SavedPostIDs: []int{},
		}

		posts = append(posts, Post{
			ID:      nextID,
			Title:   "¡Bienvenidos al nuevo foro!",
			User:    "admin",
			Message: "Este es el contenido completo del primer post de prueba.",
			Time:    time.Now().Format("15:04"),
		})
		nextID++
		saveData()
	}

	renderPage := func(w http.ResponseWriter, pageFile string, data any) {
		// Añadimos "web/head.html" a los archivos que se parsean
		tmpl, err := template.ParseFiles("web/head.html", "web/upbar.html", pageFile)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		
		// Se ejecuta la plantilla principal (pageFile) pasándole los datos
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
		renderPage(w, "web/index.html", struct {
			Posts      []Post
			Query      string
			LoggedUser string
		}{
			Posts:      posts,
			Query:      query,
			LoggedUser: loggedUser,
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
			if _, exists := users[username]; exists {
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
			users[username] = User{
				Username:     username,
				Password:     string(hashed),
				SavedPostIDs: []int{},
			}

			token := strconv.FormatInt(time.Now().UnixNano(), 10)
			sessions[token] = username

			http.SetCookie(w, &http.Cookie{
				Name:  "session_token",
				Value: token,
				Path:  "/",
			})
			fmt.Println("Usuario registrado con éxito de forma encriptada.")
			saveData()
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

			user, exists := users[username]
			if !exists {
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
			sessions[token] = username

			http.SetCookie(w, &http.Cookie{
				Name:  "session_token",
				Value: token,
				Path:  "/",
			})
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}

		http.Redirect(w, r, "/web/login.html", http.StatusSeeOther)
	})

	http.HandleFunc("/logout", func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("session_token")
		if err == nil {
			delete(sessions, cookie.Value)
		}
		http.SetCookie(w, &http.Cookie{
			Name:   "session_token",
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
				posts = append(posts, Post{
					ID:      nextID,
					Title:   title,
					User:    username,
					Message: message,
					Time:    time.Now().Format("15:04"),
				})
				nextID++
				saveData()
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

		comments = append(comments, Comment{
			ID:       nextCommentID,
			PostID:   postID,
			ParentID: parentID,
			User:     username,
			Message:  message,
			Time:     time.Now().Format("15:04"),
		})
		nextCommentID++
		saveData()

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
		saveData()
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
		searchQueryLower := strings.ToLower(searchQuery)

		for _, p := range posts {
			if strings.Contains(strings.ToLower(p.Title), searchQueryLower) {
				matchedPosts = append(matchedPosts, p)
			}
		}

		userQuery := r.URL.Query().Get("user")
		var matchedUsers []string
		if userQuery != "" {
			userQueryLower := strings.ToLower(userQuery)
			for u := range users {
				if strings.Contains(strings.ToLower(u), userQueryLower) {
					matchedUsers = append(matchedUsers, u)
				}
			}
		}

		_, loggedUser := pageContext(r)
		renderPage(w, "web/search.html", struct {
			Query      string
			Posts      []Post
			Users      []string
			UserQuery  string
			LoggedUser string
		}{
			Query:      searchQuery,
			Posts:      matchedPosts,
			Users:      matchedUsers,
			UserQuery:  userQuery,
			LoggedUser: loggedUser,
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

			var kept []Post
			for _, p := range posts {
				if p.ID != post.ID {
					kept = append(kept, p)
				}
			}
			posts = kept

			var keptComments []Comment
			for _, c := range comments {
				if c.PostID != post.ID {
					keptComments = append(keptComments, c)
				}
			}
			comments = keptComments
			saveData()

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

		user, exists := users[username]
		if !exists {
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
			ProfileUser:  user,
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

		user := users[loggedUser]
		user.Description = description
		users[loggedUser] = user
		saveData()

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

		user := users[loggedUser]
		if !isPostSaved(loggedUser, postID) {
			user.SavedPostIDs = append(user.SavedPostIDs, postID)
			users[loggedUser] = user
			saveData()
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

		user := users[loggedUser]
		var kept []int
		for _, id := range user.SavedPostIDs {
			if id != postID {
				kept = append(kept, id)
			}
		}
		user.SavedPostIDs = kept
		users[loggedUser] = user
		saveData()

		returnURL := r.FormValue("return")
		if returnURL == "" {
			returnURL = "/user?u=" + url.QueryEscape(loggedUser)
		}
		http.Redirect(w, r, returnURL, http.StatusSeeOther)
	})

	go resetRequestCounts()

	wh := flag.Bool("wh", false, "habilitar HTTPS")
	flag.Parse()

	handler := rateLimitMiddleware(http.DefaultServeMux)

	if *wh {
		if _, err := os.Stat("cert.pem"); os.IsNotExist(err) {
			fmt.Println("Error: No se encuentra cert.pem. Genera un certificado SSL.")
			os.Exit(1)
		}
		if _, err := os.Stat("key.pem"); os.IsNotExist(err) {
			fmt.Println("Error: No se encuentra key.pem. Genera un certificado SSL.")
			os.Exit(1)
		}
		fmt.Println("Servidor corriendo con HTTPS en https://localhost:8080")
		http.ListenAndServeTLS(":8080", "cert.pem", "key.pem", handler)
	} else {
		fmt.Println("Servidor corriendo en http://localhost:8080")
		http.ListenAndServe(":8080", handler)
	}
}
