package main

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

func handleLogin(w http.ResponseWriter, r *http.Request) {
	query, loggedUser := pageContext(r)
	if loggedUser != "" {
		http.Redirect(w, r, "/user?u="+url.QueryEscape(loggedUser), http.StatusSeeOther)
		return
	}
	renderPage(w, "web/login.html", struct {
		Query             string
		LoggedUser        string
		Theme             string
		LoginUsername     string
		RegisterUsername  string
		LoginUserError    string
		LoginPassError    string
		RegisterUserError string
		RegisterPassError string
	}{
		Query:             query,
		LoggedUser:        loggedUser,
		Theme:             getTheme(r),
		LoginUsername:     r.URL.Query().Get("login_user"),
		RegisterUsername:  r.URL.Query().Get("register_user"),
		LoginUserError:    r.URL.Query().Get("login_user_error"),
		LoginPassError:    r.URL.Query().Get("login_pass_error"),
		RegisterUserError: r.URL.Query().Get("register_user_error"),
		RegisterPassError: r.URL.Query().Get("register_pass_error"),
	})
}

func handlePublic(w http.ResponseWriter, r *http.Request) {
	query, loggedUser := pageContext(r)
	if loggedUser == "" {
		http.Redirect(w, r, "/web/login.html", http.StatusSeeOther)
		return
	}
	renderPage(w, "web/public.html", struct {
		Query      string
		LoggedUser string
		Theme      string
	}{Query: query, LoggedUser: loggedUser, Theme: getTheme(r)})
}

func handleHome(w http.ResponseWriter, r *http.Request) {
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
		Theme      string
	}{
		Posts:      allPosts,
		Query:      query,
		LoggedUser: loggedUser,
		Theme:      getTheme(r),
	})
}

func handleFiltered(w http.ResponseWriter, r *http.Request) {
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
		Theme      string
		SortBy     string
		Order      string
	}{
		Posts:      sorted,
		Query:      query,
		LoggedUser: loggedUser,
		Theme:      getTheme(r),
		SortBy:     sortBy,
		Order:      order,
	})
}

func handleAuth(w http.ResponseWriter, r *http.Request) {
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
}

func handleLogout(w http.ResponseWriter, r *http.Request) {
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
}

func handlePost(w http.ResponseWriter, r *http.Request) {
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
}

func handleComment(w http.ResponseWriter, r *http.Request) {
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
}

func handleDeleteComment(w http.ResponseWriter, r *http.Request) {
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
}

func handleSearch(w http.ResponseWriter, r *http.Request) {
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
	theme := getTheme(r)
	renderPage(w, "web/search.html", struct {
		Query      string
		Posts      []Post
		Users      []string
		UserQuery  string
		LoggedUser string
		Theme      string
		SortBy     string
		Order      string
	}{
		Query:      searchQuery,
		Posts:      matchedPosts,
		Users:      matchedUsers,
		UserQuery:  userQuery,
		LoggedUser: loggedUser,
		Theme:      theme,
		SortBy:     sortBy,
		Order:      order,
	})
}

func handleView(w http.ResponseWriter, r *http.Request) {
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
		Post            *Post
		From            string
		SearchQuery     string
		Query           string
		LoggedUser      string
		Theme           string
		IsSaved         bool
		IsAuthor        bool
		CommentQuery    string
		CommentTree     []*CommentNode
		MatchedComments []Comment
	}{
		Post:            foundPost,
		From:            fromPage,
		SearchQuery:     searchQuery,
		Query:           query,
		LoggedUser:      loggedUser,
		Theme:           getTheme(r),
		IsSaved:         isPostSaved(loggedUser, id),
		IsAuthor:        loggedUser == foundPost.User,
		CommentQuery:    commentQuery,
		CommentTree:     buildCommentTree(postComments, 0, id, loggedUser),
		MatchedComments: filterComments(postComments, commentQuery),
	})
}

func handleConfirm(w http.ResponseWriter, r *http.Request) {
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
		Theme      string
		Post       *Post
		Error      string
	}{
		Query:      query,
		LoggedUser: loggedUser,
		Theme:      getTheme(r),
		Post:       post,
		Error:      r.URL.Query().Get("error"),
	})
}

func handleUser(w http.ResponseWriter, r *http.Request) {
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
		Theme        string
		ProfileUser  User
		Posts        []Post
		SavedPosts   []Post
		IsOwnProfile bool
		Error        string
	}{
		Query:        query,
		LoggedUser:   loggedUser,
		Theme:        getTheme(r),
		ProfileUser:  *user,
		Posts:        getUserPosts(username),
		IsOwnProfile: isOwn,
		Error:        r.URL.Query().Get("error"),
	}
	if isOwn {
		data.SavedPosts = getSavedPosts(username)
	}

	renderPage(w, "web/user.html", data)
}

func handleProfile(w http.ResponseWriter, r *http.Request) {
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
}

func handleSave(w http.ResponseWriter, r *http.Request) {
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
}

func handleUnsave(w http.ResponseWriter, r *http.Request) {
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
}

func handleTheme(w http.ResponseWriter, r *http.Request) {
	mode := r.URL.Query().Get("mode")
	if mode == "dark" || mode == "light" {
		http.SetCookie(w, &http.Cookie{
			Name:  "theme",
			Value: mode,
			Path:  "/",
		})
	}
	returnURL := r.Header.Get("Referer")
	if returnURL == "" {
		returnURL = "/"
	}
	http.Redirect(w, r, returnURL, http.StatusSeeOther)
}
