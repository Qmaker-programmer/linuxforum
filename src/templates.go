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
	postsPerPage         = 20
)

func renderPage(w http.ResponseWriter, r *http.Request, pageFile string, data any) {
	backLabel, backURL := lastListBackLink(r)
	funcMap := template.FuncMap{
		"backLabel": func() string { return backLabel },
		"backURL":   func() string { return backURL },
		"add":       func(a, b int) int { return a + b },
		"sub":       func(a, b int) int { return a - b },
	}
	tmpl, err := template.New(filepath.Base(pageFile)).Funcs(funcMap).ParseFiles("web/head.html", "web/upbar.html", pageFile)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := tmpl.ExecuteTemplate(w, filepath.Base(pageFile), data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func currentURL(r *http.Request) string {
	target := r.URL.Path
	if r.URL.RawQuery != "" {
		target += "?" + r.URL.RawQuery
	}
	return target
}

const lastListCookie = "last_list"

// setLastListCookie remembers the last "listing" page (home, filtered,
// search, drafts) the user visited, so other pages can offer a
// "<- Volver a X" link back to it, preserving whatever filters/sort/search
// terms were active.
func setLastListCookie(w http.ResponseWriter, label, target string) {
	value := url.QueryEscape(label) + "|" + url.QueryEscape(target)
	http.SetCookie(w, &http.Cookie{
		Name:  lastListCookie,
		Value: value,
		Path:  "/",
	})
}

// lastListBackLink returns the label/URL to show in the upbar's back-link,
// or ("", "") if there's nothing to show: no listing visited yet, we're
// already on that listing page, or the current page already renders its
// own tailored back-link (post view).
func lastListBackLink(r *http.Request) (label, target string) {
	if r.URL.Path == "/view" {
		return "", ""
	}
	cookie, err := r.Cookie(lastListCookie)
	if err != nil {
		return "", ""
	}
	parts := strings.SplitN(cookie.Value, "|", 2)
	if len(parts) != 2 {
		return "", ""
	}
	l, errL := url.QueryUnescape(parts[0])
	t, errT := url.QueryUnescape(parts[1])
	if errL != nil || errT != nil || t == "" {
		return "", ""
	}
	if currentURL(r) == t {
		return "", ""
	}
	return l, t
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
	session := getSessionByToken(cookie.Value)
	if session == nil {
		return nil
	}
	if config.SessionExpireMinutes > 0 && time.Now().After(session.ExpiresAt) {
		deleteSession(cookie.Value)
		return nil
	}
	return session
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
		return "system"
	}
	switch cookie.Value {
	case "dark":
		return "dark"
	case "light":
		return "light"
	default:
		return "system"
	}
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

func sortDrafts(drafts []Draft, sortBy, order string) []Draft {
	result := make([]Draft, len(drafts))
	copy(result, drafts)

	sort.SliceStable(result, func(i, j int) bool {
		var less bool
		if sortBy == "title" {
			less = strings.ToLower(result[i].Title) < strings.ToLower(result[j].Title)
		} else {
			less = result[i].UpdatedAt < result[j].UpdatedAt
		}
		if order == "desc" {
			return !less
		}
		return less
	})
	return result
}

func sortCommentDrafts(drafts []CommentDraft, sortBy, order string) []CommentDraft {
	result := make([]CommentDraft, len(drafts))
	copy(result, drafts)

	sort.SliceStable(result, func(i, j int) bool {
		var less bool
		if sortBy == "title" {
			less = strings.ToLower(result[i].PostTitle) < strings.ToLower(result[j].PostTitle)
		} else {
			less = result[i].UpdatedAt < result[j].UpdatedAt
		}
		if order == "desc" {
			return !less
		}
		return less
	})
	return result
}

func normalizeSortParams(sortBy, order string) (string, string) {
	switch sortBy {
	case "date", "title":
	default:
		sortBy = "date"
	}
	switch order {
	case "asc", "desc":
	default:
		order = "desc"
	}
	return sortBy, order
}

func parsePage(r *http.Request) int {
	page, err := strconv.Atoi(r.URL.Query().Get("page"))
	if err != nil || page < 1 {
		return 1
	}
	return page
}

// paginatePosts slices posts into the requested page, clamping page into
// range instead of returning an empty page when it's out of bounds.
func paginatePosts(posts []Post, page int) (pageItems []Post, clampedPage, totalPages int) {
	total := len(posts)
	totalPages = (total + postsPerPage - 1) / postsPerPage
	if totalPages < 1 {
		totalPages = 1
	}
	clampedPage = page
	if clampedPage < 1 {
		clampedPage = 1
	}
	if clampedPage > totalPages {
		clampedPage = totalPages
	}

	start := (clampedPage - 1) * postsPerPage
	if start > total {
		start = total
	}
	end := start + postsPerPage
	if end > total {
		end = total
	}
	return posts[start:end], clampedPage, totalPages
}
