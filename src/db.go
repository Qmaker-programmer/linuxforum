package main

import (
	"database/sql"
	"fmt"
	"strings"

	_ "github.com/mattn/go-sqlite3"
)

func initDB() {
	var err error
	db, err = sql.Open("sqlite3", config.DBPath)
	if err != nil {
		fmt.Println("Error al abrir base de datos:", err)
		panic(err)
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
		panic(err)
	}
}

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
