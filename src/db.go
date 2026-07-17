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
	"fmt"
	"log/slog"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type migration struct {
	Version int
	Apply   func()
}

func runMigrations() {
	db.Exec("CREATE TABLE IF NOT EXISTS migrations (version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL)")

	migrations := []migration{
		{
			Version: 1,
			Apply: func() {
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
					description TEXT NOT NULL DEFAULT '',
					email TEXT NOT NULL DEFAULT ''
				);
				CREATE TABLE IF NOT EXISTS saved_posts (
					username TEXT NOT NULL,
					post_id INTEGER NOT NULL,
					PRIMARY KEY (username, post_id)
				);
				CREATE TABLE IF NOT EXISTS password_resets (
					username TEXT PRIMARY KEY,
					token_hash TEXT NOT NULL,
					expires_at TEXT NOT NULL,
					used INTEGER NOT NULL DEFAULT 0
				);
				CREATE TABLE IF NOT EXISTS pending_deletions (
					username TEXT PRIMARY KEY,
					token_hash TEXT NOT NULL,
					created_at TEXT NOT NULL
				);
				CREATE TABLE IF NOT EXISTS pending_post_deletions (
					post_id INTEGER PRIMARY KEY,
					token_hash TEXT NOT NULL,
					created_at TEXT NOT NULL
				);
				CREATE TABLE IF NOT EXISTS pending_activations (
					username TEXT PRIMARY KEY,
					password TEXT NOT NULL,
					email TEXT NOT NULL,
					token_hash TEXT NOT NULL,
					created_at TEXT NOT NULL
				);`
				if _, err := db.Exec(schema); err != nil {
					panic(fmt.Sprintf("Migración %d fallida: %v", 1, err))
				}
			},
		},
		{
			Version: 2,
			Apply: func() {
				var count int
				db.QueryRow("SELECT COUNT(*) FROM pragma_table_info('users') WHERE name='email'").Scan(&count)
				if count == 0 {
					if _, err := db.Exec("ALTER TABLE users ADD COLUMN email TEXT NOT NULL DEFAULT ''"); err != nil {
						panic(fmt.Sprintf("Migración %d fallida: %v", 2, err))
					}
				}
			},
		},
		{
			Version: 3,
			Apply: func() {
				var count int
				db.QueryRow("SELECT COUNT(*) FROM pragma_table_info('posts') WHERE name='markdown'").Scan(&count)
				if count == 0 {
					if _, err := db.Exec("ALTER TABLE posts ADD COLUMN markdown TEXT NOT NULL DEFAULT ''"); err != nil {
						panic(fmt.Sprintf("Migración %d fallida: %v", 3, err))
					}
				}
				db.QueryRow("SELECT COUNT(*) FROM pragma_table_info('comments') WHERE name='markdown'").Scan(&count)
				if count == 0 {
					if _, err := db.Exec("ALTER TABLE comments ADD COLUMN markdown TEXT NOT NULL DEFAULT ''"); err != nil {
						panic(fmt.Sprintf("Migración %d fallida: %v", 3, err))
					}
				}
			},
		},
		{
			Version: 4,
			Apply: func() {
				if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS sessions (
					token TEXT PRIMARY KEY,
					username TEXT NOT NULL,
					expires_at TEXT NOT NULL DEFAULT '',
					csrf_token TEXT NOT NULL
				)`); err != nil {
					panic(fmt.Sprintf("Migración %d fallida: %v", 4, err))
				}
			},
		},
		{
			Version: 5,
			Apply: func() {
				if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS post_drafts (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					username TEXT NOT NULL,
					title TEXT NOT NULL DEFAULT '',
					message TEXT NOT NULL DEFAULT '',
					created_at TEXT NOT NULL,
					updated_at TEXT NOT NULL
				)`); err != nil {
					panic(fmt.Sprintf("Migración %d fallida: %v", 5, err))
				}
			},
		},
		{
			Version: 6,
			Apply: func() {
				if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS comment_drafts (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					username TEXT NOT NULL,
					post_id INTEGER NOT NULL,
					parent_id INTEGER NOT NULL DEFAULT 0,
					message TEXT NOT NULL DEFAULT '',
					created_at TEXT NOT NULL,
					updated_at TEXT NOT NULL
				)`); err != nil {
					panic(fmt.Sprintf("Migración %d fallida: %v", 6, err))
				}
			},
		},
		{
			// Sessions used to store the raw session token as-is. That
			// means read access to the DB alone (a leaked backup, etc.)
			// handed over ready-to-use session cookies. Renaming the
			// column to token_hash invalidates every existing session
			// (their old values aren't hashes of anything), forcing a
			// harmless re-login, and all sessions from here on are
			// looked up by hash instead.
			Version: 7,
			Apply: func() {
				if _, err := db.Exec("ALTER TABLE sessions RENAME COLUMN token TO token_hash"); err != nil {
					panic(fmt.Sprintf("Migración %d fallida: %v", 7, err))
				}
			},
		},
		{
			Version: 8,
			Apply: func() {
				var count int
				db.QueryRow("SELECT COUNT(*) FROM pragma_table_info('posts') WHERE name='updated_at'").Scan(&count)
				if count == 0 {
					if _, err := db.Exec("ALTER TABLE posts ADD COLUMN updated_at TEXT NOT NULL DEFAULT ''"); err != nil {
						panic(fmt.Sprintf("Migración %d fallida: %v", 8, err))
					}
				}
			},
		},
		{
			// Each row is a snapshot of a post's content right before it
			// changed (edit or revert), so history + revert share one
			// mechanism: see updatePostWithRevision in this file.
			Version: 9,
			Apply: func() {
				if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS post_revisions (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					post_id INTEGER NOT NULL,
					title TEXT NOT NULL,
					message TEXT NOT NULL,
					markdown TEXT NOT NULL,
					edited_at TEXT NOT NULL
				)`); err != nil {
					panic(fmt.Sprintf("Migración %d fallida: %v", 9, err))
				}
			},
		},
		{
			// 0 = draft of a brand-new post (as before); non-zero = draft
			// of an edit to that existing post.
			Version: 10,
			Apply: func() {
				var count int
				db.QueryRow("SELECT COUNT(*) FROM pragma_table_info('post_drafts') WHERE name='editing_post_id'").Scan(&count)
				if count == 0 {
					if _, err := db.Exec("ALTER TABLE post_drafts ADD COLUMN editing_post_id INTEGER NOT NULL DEFAULT 0"); err != nil {
						panic(fmt.Sprintf("Migración %d fallida: %v", 10, err))
					}
				}
			},
		},
	}

	for _, m := range migrations {
		var count int
		db.QueryRow("SELECT COUNT(*) FROM migrations WHERE version = ?", m.Version).Scan(&count)
		if count > 0 {
			continue
		}

		m.Apply()

		db.Exec("INSERT INTO migrations (version, applied_at) VALUES (?, datetime('now'))", m.Version)
		slog.Info("Migración aplicada", "version", m.Version)
	}
}

func initDB() {
	var err error
	db, err = sql.Open("sqlite3", config.DBPath)
	if err != nil {
		slog.Error("No se pudo abrir la base de datos", "err", err)
		panic(err)
	}
	db.Exec("PRAGMA journal_mode=WAL")

	runMigrations()
}

func getAllPosts() []Post {
	rows, err := db.Query("SELECT id, title, user, message, markdown, created_at, updated_at FROM posts ORDER BY created_at DESC")
	if err != nil {
		return nil
	}
	defer rows.Close()
	var result []Post
	for rows.Next() {
		var p Post
		if err := rows.Scan(&p.ID, &p.Title, &p.User, &p.Message, &p.Markdown, &p.Time, &p.UpdatedAt); err != nil {
			continue
		}
		result = append(result, p)
	}
	return result
}

func getPostByID(id int) *Post {
	row := db.QueryRow("SELECT id, title, user, message, markdown, created_at, updated_at FROM posts WHERE id = ?", id)
	var p Post
	if err := row.Scan(&p.ID, &p.Title, &p.User, &p.Message, &p.Markdown, &p.Time, &p.UpdatedAt); err != nil {
		return nil
	}
	return &p
}

// updatePostWithRevision snapshots the post's current content into
// post_revisions before overwriting it. Both editing a post and
// reverting to an old revision go through this same function — a
// revert just passes an old revision's content as the "new" content,
// which leaves the pre-revert state as a fresh revision too.
func updatePostWithRevision(postID int, newTitle, newMessage, newMarkdown string) error {
	current := getPostByID(postID)
	if current == nil {
		return fmt.Errorf("Post no encontrado.")
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	now := time.Now().Format("2006-01-02 15:04")
	if _, err := tx.Exec("INSERT INTO post_revisions (post_id, title, message, markdown, edited_at) VALUES (?, ?, ?, ?, ?)",
		postID, current.Title, current.Message, string(current.Markdown), now); err != nil {
		return err
	}
	if _, err := tx.Exec("UPDATE posts SET title = ?, message = ?, markdown = ?, updated_at = ? WHERE id = ?",
		newTitle, newMessage, newMarkdown, now, postID); err != nil {
		return err
	}
	return tx.Commit()
}

func getPostRevisions(postID int) []PostRevision {
	rows, err := db.Query("SELECT id, post_id, title, message, markdown, edited_at FROM post_revisions WHERE post_id = ? ORDER BY edited_at DESC, id DESC", postID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var result []PostRevision
	for rows.Next() {
		var rv PostRevision
		if err := rows.Scan(&rv.ID, &rv.PostID, &rv.Title, &rv.Message, &rv.Markdown, &rv.EditedAt); err != nil {
			continue
		}
		result = append(result, rv)
	}
	return result
}

func getPostRevisionByID(id int) *PostRevision {
	row := db.QueryRow("SELECT id, post_id, title, message, markdown, edited_at FROM post_revisions WHERE id = ?", id)
	var rv PostRevision
	if err := row.Scan(&rv.ID, &rv.PostID, &rv.Title, &rv.Message, &rv.Markdown, &rv.EditedAt); err != nil {
		return nil
	}
	return &rv
}

func getCommentByID(id int) *Comment {
	row := db.QueryRow("SELECT id, post_id, parent_id, user, message, markdown, created_at, deleted FROM comments WHERE id = ?", id)
	var c Comment
	var deleted int
	if err := row.Scan(&c.ID, &c.PostID, &c.ParentID, &c.User, &c.Message, &c.Markdown, &c.Time, &deleted); err != nil {
		return nil
	}
	c.Deleted = deleted == 1
	return &c
}

func getCommentsForPost(postID int) []Comment {
	rows, err := db.Query("SELECT id, post_id, parent_id, user, message, markdown, created_at, deleted FROM comments WHERE post_id = ? ORDER BY created_at ASC", postID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var result []Comment
	for rows.Next() {
		var c Comment
		var deleted int
		if err := rows.Scan(&c.ID, &c.PostID, &c.ParentID, &c.User, &c.Message, &c.Markdown, &c.Time, &deleted); err != nil {
			continue
		}
		c.Deleted = deleted == 1
		result = append(result, c)
	}
	return result
}

func collectMessageImages(query string, args ...any) []string {
	var paths []string
	rows, err := db.Query(query, args...)
	if err != nil {
		return paths
	}
	defer rows.Close()
	for rows.Next() {
		var message string
		if rows.Scan(&message) == nil {
			paths = append(paths, extractUploadedImagePaths(message)...)
		}
	}
	return paths
}

// deletePostCascade removes a post along with everything that references
// it (comments, saved-post bookmarks, pending comment drafts) and cleans
// up any uploaded images from all of their messages.
func deletePostCascade(postID int, postMessage string) {
	var imagePaths []string
	imagePaths = append(imagePaths, extractUploadedImagePaths(postMessage)...)
	imagePaths = append(imagePaths, collectMessageImages("SELECT message FROM comments WHERE post_id = ?", postID)...)
	imagePaths = append(imagePaths, collectMessageImages("SELECT message FROM comment_drafts WHERE post_id = ?", postID)...)
	imagePaths = append(imagePaths, collectMessageImages("SELECT message FROM post_revisions WHERE post_id = ?", postID)...)
	imagePaths = append(imagePaths, collectMessageImages("SELECT message FROM post_drafts WHERE editing_post_id = ?", postID)...)

	db.Exec("DELETE FROM comments WHERE post_id = ?", postID)
	db.Exec("DELETE FROM saved_posts WHERE post_id = ?", postID)
	db.Exec("DELETE FROM comment_drafts WHERE post_id = ?", postID)
	db.Exec("DELETE FROM post_revisions WHERE post_id = ?", postID)
	db.Exec("DELETE FROM post_drafts WHERE editing_post_id = ?", postID)
	db.Exec("DELETE FROM posts WHERE id = ?", postID)

	deleteUploadedImages(imagePaths)
}

func getUserPosts(username string) []Post {
	rows, err := db.Query("SELECT id, title, user, message, markdown, created_at, updated_at FROM posts WHERE user = ? ORDER BY created_at DESC", username)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var result []Post
	for rows.Next() {
		var p Post
		if err := rows.Scan(&p.ID, &p.Title, &p.User, &p.Message, &p.Markdown, &p.Time, &p.UpdatedAt); err != nil {
			continue
		}
		result = append(result, p)
	}
	return result
}

func getSavedPosts(username string) []Post {
	rows, err := db.Query("SELECT p.id, p.title, p.user, p.message, p.markdown, p.created_at, p.updated_at FROM posts p JOIN saved_posts s ON p.id = s.post_id WHERE s.username = ? ORDER BY p.created_at DESC", username)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var result []Post
	for rows.Next() {
		var p Post
		if err := rows.Scan(&p.ID, &p.Title, &p.User, &p.Message, &p.Markdown, &p.Time, &p.UpdatedAt); err != nil {
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

func getDraftByID(id int) *Draft {
	row := db.QueryRow("SELECT id, username, title, message, editing_post_id, created_at, updated_at FROM post_drafts WHERE id = ?", id)
	var d Draft
	if err := row.Scan(&d.ID, &d.Username, &d.Title, &d.Message, &d.EditingPostID, &d.CreatedAt, &d.UpdatedAt); err != nil {
		return nil
	}
	return &d
}

func getUserDrafts(username string) []Draft {
	rows, err := db.Query(`SELECT pd.id, pd.username, pd.title, pd.message, pd.editing_post_id, pd.created_at, pd.updated_at, COALESCE(p.title, '')
		FROM post_drafts pd LEFT JOIN posts p ON pd.editing_post_id = p.id
		WHERE pd.username = ? ORDER BY pd.updated_at DESC`, username)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var result []Draft
	for rows.Next() {
		var d Draft
		if err := rows.Scan(&d.ID, &d.Username, &d.Title, &d.Message, &d.EditingPostID, &d.CreatedAt, &d.UpdatedAt, &d.EditingPostTitle); err != nil {
			continue
		}
		result = append(result, d)
	}
	return result
}

func saveDraft(draftID int, username, title, message string, editingPostID int) (int, error) {
	now := time.Now().Format("2006-01-02 15:04")

	if draftID != 0 {
		existing := getDraftByID(draftID)
		if existing != nil && existing.Username == username {
			if _, err := db.Exec("UPDATE post_drafts SET title = ?, message = ?, editing_post_id = ?, updated_at = ? WHERE id = ?", title, message, editingPostID, now, draftID); err != nil {
				return 0, err
			}
			deleteUploadedImages(diffRemovedImages(existing.Message, message))
			return draftID, nil
		}
	}

	res, err := db.Exec("INSERT INTO post_drafts (username, title, message, editing_post_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)", username, title, message, editingPostID, now, now)
	if err != nil {
		return 0, err
	}
	newID, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	return int(newID), nil
}

func deleteDraft(draftID int, username string) {
	db.Exec("DELETE FROM post_drafts WHERE id = ? AND username = ?", draftID, username)
}

func getCommentDraftByID(id int) *CommentDraft {
	row := db.QueryRow("SELECT id, username, post_id, parent_id, message, created_at, updated_at FROM comment_drafts WHERE id = ?", id)
	var d CommentDraft
	if err := row.Scan(&d.ID, &d.Username, &d.PostID, &d.ParentID, &d.Message, &d.CreatedAt, &d.UpdatedAt); err != nil {
		return nil
	}
	return &d
}

func getUserCommentDrafts(username string) []CommentDraft {
	rows, err := db.Query(`SELECT cd.id, cd.username, cd.post_id, cd.parent_id, cd.message, cd.created_at, cd.updated_at, COALESCE(p.title, '')
		FROM comment_drafts cd LEFT JOIN posts p ON cd.post_id = p.id
		WHERE cd.username = ? ORDER BY cd.updated_at DESC`, username)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var result []CommentDraft
	for rows.Next() {
		var d CommentDraft
		if err := rows.Scan(&d.ID, &d.Username, &d.PostID, &d.ParentID, &d.Message, &d.CreatedAt, &d.UpdatedAt, &d.PostTitle); err != nil {
			continue
		}
		result = append(result, d)
	}
	return result
}

func saveCommentDraft(draftID int, username string, postID, parentID int, message string) (int, error) {
	now := time.Now().Format("2006-01-02 15:04")

	if draftID != 0 {
		existing := getCommentDraftByID(draftID)
		if existing != nil && existing.Username == username {
			if _, err := db.Exec("UPDATE comment_drafts SET post_id = ?, parent_id = ?, message = ?, updated_at = ? WHERE id = ?", postID, parentID, message, now, draftID); err != nil {
				return 0, err
			}
			deleteUploadedImages(diffRemovedImages(existing.Message, message))
			return draftID, nil
		}
	}

	res, err := db.Exec("INSERT INTO comment_drafts (username, post_id, parent_id, message, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)", username, postID, parentID, message, now, now)
	if err != nil {
		return 0, err
	}
	newID, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	return int(newID), nil
}

func deleteCommentDraft(draftID int, username string) {
	db.Exec("DELETE FROM comment_drafts WHERE id = ? AND username = ?", draftID, username)
}

func existsUser(username string) bool {
	var count int
	db.QueryRow("SELECT COUNT(*) FROM users WHERE username = ?", username).Scan(&count)
	return count > 0
}

func getUser(username string) *User {
	row := db.QueryRow("SELECT username, password, description, email FROM users WHERE username = ?", username)
	var u User
	if err := row.Scan(&u.Username, &u.Password, &u.Description, &u.Email); err != nil {
		return nil
	}
	rows, err := db.Query("SELECT post_id FROM saved_posts WHERE username = ?", username)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var id int
			if err := rows.Scan(&id); err == nil {
				u.SavedPostIDs = append(u.SavedPostIDs, id)
			}
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

	tx.Exec("INSERT INTO users (username, password, description, email) SELECT ?, password, description, email FROM users WHERE username = ?", newName, oldName)
	tx.Exec("DELETE FROM users WHERE username = ?", oldName)
	tx.Exec("UPDATE posts SET user = ? WHERE user = ?", newName, oldName)
	tx.Exec("UPDATE comments SET user = ? WHERE user = ?", newName, oldName)
	tx.Exec("UPDATE saved_posts SET username = ? WHERE username = ?", newName, oldName)

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("Error al renombrar usuario")
	}

	db.Exec("UPDATE sessions SET username = ? WHERE username = ?", newName, oldName)
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

func getUserByEmail(email string) *User {
	row := db.QueryRow("SELECT username, password, description, email FROM users WHERE email = ?", email)
	var u User
	if err := row.Scan(&u.Username, &u.Password, &u.Description, &u.Email); err != nil {
		return nil
	}
	return &u
}

func updateUserEmail(username, email string) error {
	_, err := db.Exec("UPDATE users SET email = ? WHERE username = ?", email, username)
	return err
}

func setUserPassword(username, hashedPassword string) error {
	_, err := db.Exec("UPDATE users SET password = ? WHERE username = ?", hashedPassword, username)
	return err
}

func saveResetToken(username, tokenHash string, expiresAt time.Time) error {
	_, err := db.Exec("INSERT OR REPLACE INTO password_resets (username, token_hash, expires_at, used) VALUES (?, ?, ?, 0)",
		username, tokenHash, expiresAt.Format(time.RFC3339))
	return err
}

func getResetTokenByHash(tokenHash string) *ResetToken {
	row := db.QueryRow("SELECT username, token_hash, expires_at, used FROM password_resets WHERE token_hash = ?", tokenHash)
	var rt ResetToken
	var used int
	if err := row.Scan(&rt.Username, &rt.TokenHash, &rt.ExpiresAt, &used); err != nil {
		return nil
	}
	rt.Used = used == 1
	return &rt
}

func markResetTokenUsed(username string) error {
	_, err := db.Exec("DELETE FROM password_resets WHERE username = ?", username)
	return err
}

func cleanupExpiredResetTokens() {
	db.Exec("DELETE FROM password_resets WHERE expires_at < datetime('now')")
}

func savePendingActivation(username, password, email, tokenHash string) error {
	_, err := db.Exec("INSERT OR REPLACE INTO pending_activations (username, password, email, token_hash, created_at) VALUES (?, ?, ?, ?, datetime('now'))",
		username, password, email, tokenHash)
	return err
}

func getPendingActivationByHash(tokenHash string) *PendingActivation {
	row := db.QueryRow("SELECT username, password, email, token_hash, created_at FROM pending_activations WHERE token_hash = ?", tokenHash)
	var pa PendingActivation
	if err := row.Scan(&pa.Username, &pa.Password, &pa.Email, &pa.TokenHash, &pa.CreatedAt); err != nil {
		return nil
	}
	return &pa
}

func deletePendingActivation(username string) error {
	_, err := db.Exec("DELETE FROM pending_activations WHERE username = ?", username)
	return err
}

func cleanupExpiredPendingActivations() {
	db.Exec("DELETE FROM pending_activations WHERE created_at < datetime('now', '-1 day')")
}

func existsPendingUsername(username string) bool {
	var count int
	db.QueryRow("SELECT COUNT(*) FROM pending_activations WHERE username = ?", username).Scan(&count)
	return count > 0
}

func existsPendingEmail(email string) bool {
	var count int
	db.QueryRow("SELECT COUNT(*) FROM pending_activations WHERE email = ?", email).Scan(&count)
	return count > 0
}

func savePendingDeletion(username, tokenHash string) error {
	_, err := db.Exec("INSERT OR REPLACE INTO pending_deletions (username, token_hash, created_at) VALUES (?, ?, datetime('now'))",
		username, tokenHash)
	return err
}

func getPendingDeletionByHash(tokenHash string) *PendingDeletion {
	row := db.QueryRow("SELECT username, token_hash, created_at FROM pending_deletions WHERE token_hash = ?", tokenHash)
	var pd PendingDeletion
	if err := row.Scan(&pd.Username, &pd.TokenHash, &pd.CreatedAt); err != nil {
		return nil
	}
	return &pd
}

func deletePendingDeletion(username string) error {
	_, err := db.Exec("DELETE FROM pending_deletions WHERE username = ?", username)
	return err
}

func cleanupExpiredPendingDeletions() {
	db.Exec("DELETE FROM pending_deletions WHERE created_at < datetime('now', '-1 hour')")
}

func savePendingPostDeletion(postID int, tokenHash string) error {
	_, err := db.Exec("INSERT OR REPLACE INTO pending_post_deletions (post_id, token_hash, created_at) VALUES (?, ?, datetime('now'))",
		postID, tokenHash)
	return err
}

func getPendingPostDeletionByHash(tokenHash string) *PendingPostDeletion {
	row := db.QueryRow("SELECT post_id, token_hash, created_at FROM pending_post_deletions WHERE token_hash = ?", tokenHash)
	var ppd PendingPostDeletion
	if err := row.Scan(&ppd.PostID, &ppd.TokenHash, &ppd.CreatedAt); err != nil {
		return nil
	}
	return &ppd
}

func deletePendingPostDeletion(postID int) error {
	_, err := db.Exec("DELETE FROM pending_post_deletions WHERE post_id = ?", postID)
	return err
}

func cleanupExpiredPendingPostDeletions() {
	db.Exec("DELETE FROM pending_post_deletions WHERE created_at < datetime('now', '-1 hour')")
}

func deleteUserAccount(username string) error {
	var imagePaths []string
	imagePaths = append(imagePaths, collectMessageImages("SELECT message FROM posts WHERE user = ?", username)...)
	imagePaths = append(imagePaths, collectMessageImages("SELECT message FROM comments WHERE user = ? AND deleted = 0", username)...)
	imagePaths = append(imagePaths, collectMessageImages("SELECT message FROM post_drafts WHERE username = ?", username)...)
	imagePaths = append(imagePaths, collectMessageImages("SELECT message FROM comment_drafts WHERE username = ?", username)...)

	var postIDs []int
	if rows, err := db.Query("SELECT id FROM posts WHERE user = ?", username); err == nil {
		for rows.Next() {
			var id int
			if rows.Scan(&id) == nil {
				postIDs = append(postIDs, id)
			}
		}
		rows.Close()
	}
	for _, id := range postIDs {
		imagePaths = append(imagePaths, collectMessageImages("SELECT message FROM comment_drafts WHERE post_id = ?", id)...)
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	tx.Exec("DELETE FROM saved_posts WHERE username = ?", username)
	tx.Exec("UPDATE comments SET deleted = 1, message = '[eliminado]' WHERE user = ?", username)
	for _, id := range postIDs {
		tx.Exec("DELETE FROM comment_drafts WHERE post_id = ?", id)
	}
	tx.Exec("DELETE FROM posts WHERE user = ?", username)
	tx.Exec("DELETE FROM password_resets WHERE username = ?", username)
	tx.Exec("DELETE FROM pending_activations WHERE username = ?", username)
	tx.Exec("DELETE FROM pending_deletions WHERE username = ?", username)
	tx.Exec("DELETE FROM post_drafts WHERE username = ?", username)
	tx.Exec("DELETE FROM comment_drafts WHERE username = ?", username)
	tx.Exec("DELETE FROM sessions WHERE username = ?", username)
	tx.Exec("DELETE FROM users WHERE username = ?", username)

	if err := tx.Commit(); err != nil {
		return err
	}

	deleteUploadedImages(imagePaths)
	return nil
}

func saveSession(token string, session Session) error {
	expiresAt := ""
	if !session.ExpiresAt.IsZero() {
		expiresAt = session.ExpiresAt.Format(time.RFC3339)
	}
	_, err := db.Exec("INSERT OR REPLACE INTO sessions (token_hash, username, expires_at, csrf_token) VALUES (?, ?, ?, ?)",
		hashToken(token), session.Username, expiresAt, session.CSRFToken)
	return err
}

func getSessionByToken(token string) *Session {
	row := db.QueryRow("SELECT username, expires_at, csrf_token FROM sessions WHERE token_hash = ?", hashToken(token))
	var username, expiresAtStr, csrfToken string
	if err := row.Scan(&username, &expiresAtStr, &csrfToken); err != nil {
		return nil
	}
	var expiresAt time.Time
	if expiresAtStr != "" {
		expiresAt, _ = time.Parse(time.RFC3339, expiresAtStr)
	}
	return &Session{Username: username, ExpiresAt: expiresAt, CSRFToken: csrfToken}
}

func deleteSession(token string) error {
	_, err := db.Exec("DELETE FROM sessions WHERE token_hash = ?", hashToken(token))
	return err
}

func deleteUserSessions(username string) error {
	_, err := db.Exec("DELETE FROM sessions WHERE username = ?", username)
	return err
}
