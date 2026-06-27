package main

import (
	"html/template"
	"net/http"
	"net/url"
	"path/filepath"
	"sort"
	"strings"
	"time"
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
