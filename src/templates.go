package main

import (
	"crypto/rand"
	"encoding/hex"
	"html/template"
	"net/http"
	"net/url"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	maxTitleLength       = 200
	maxMessageLength     = 10000
	maxDescriptionLength = 2000
	maxUsernameLength    = 50
)

func renderPage(w http.ResponseWriter, pageFile string, data any) {
	tmpl, err := template.ParseFiles("web/head.html", "web/upbar.html", pageFile)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := tmpl.ExecuteTemplate(w, filepath.Base(pageFile), data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func generateCSRFToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return hex.EncodeToString(b)
}

func getSession(r *http.Request) *Session {
	cookie, err := r.Cookie(config.SessionTokenName)
	if err != nil {
		return nil
	}
	mu.Lock()
	session, ok := sessions[cookie.Value]
	mu.Unlock()
	if !ok {
		return nil
	}
	if config.SessionExpireMinutes > 0 && time.Now().After(session.ExpiresAt) {
		mu.Lock()
		delete(sessions, cookie.Value)
		mu.Unlock()
		return nil
	}
	return &session
}

func getLoggedUser(r *http.Request) string {
	s := getSession(r)
	if s == nil {
		return ""
	}
	return s.Username
}

func getCSRFToken(r *http.Request) string {
	s := getSession(r)
	if s == nil {
		return ""
	}
	return s.CSRFToken
}

func validateCSRF(r *http.Request) bool {
	token := r.FormValue("csrf_token")
	return token != "" && token == getCSRFToken(r)
}

func getTheme(r *http.Request) string {
	cookie, err := r.Cookie("theme")
	if err != nil {
		return ""
	}
	if cookie.Value == "dark" {
		return "dark"
	}
	return ""
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

func buildCommentTree(all []Comment, parentID, postID int, loggedUser, csrfToken string) []*CommentNode {
	var nodes []*CommentNode
	for _, c := range all {
		if c.ParentID == parentID {
			nodes = append(nodes, &CommentNode{
				Comment:    c,
				PostID:     postID,
				LoggedUser: loggedUser,
				CSRFToken:  csrfToken,
				Children:   buildCommentTree(all, c.ID, postID, loggedUser, csrfToken),
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
