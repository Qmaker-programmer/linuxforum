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
	"html/template"
	"sync"
	"time"
)

type Post struct {
	ID       int
	Title    string
	User     string
	Message  string
	Markdown template.HTML
	Time     string
}

type Comment struct {
	ID       int
	PostID   int
	ParentID int
	User     string
	Message  string
	Markdown template.HTML
	Time     string
	Deleted  bool
}

type Draft struct {
	ID        int
	Username  string
	Title     string
	Message   string
	CreatedAt string
	UpdatedAt string
}

type CommentDraft struct {
	ID        int
	Username  string
	PostID    int
	ParentID  int
	Message   string
	PostTitle string
	CreatedAt string
	UpdatedAt string
}

type CommentNode struct {
	Comment    Comment
	Children   []*CommentNode
	PostID     int
	LoggedUser string
	CSRFToken  string
}

type User struct {
	Username     string
	Password     string
	Description  string
	Email        string
	SavedPostIDs []int
}

type PendingPostDeletion struct {
	PostID    int
	TokenHash string
	CreatedAt string
}

type PendingDeletion struct {
	Username  string
	TokenHash string
	CreatedAt string
}

type PendingActivation struct {
	Username  string
	Password  string
	Email     string
	TokenHash string
	CreatedAt string
}

type ResetToken struct {
	Username  string
	TokenHash string
	ExpiresAt string
	Used      bool
}

type Session struct {
	Username  string
	ExpiresAt time.Time
	CSRFToken string
}

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
	TrustProxyHeaders    bool   `json:"trust_proxy_headers"`
	BackupIntervalHours  int    `json:"backup_interval_hours"`
	MaxBackups           int    `json:"max_backups"`
}

var db *sql.DB
var config Config
var requestCounts = make(map[string]int)
var mu sync.Mutex

var loginAttempts = make(map[string]int)
var loginLockedUntil = make(map[string]time.Time)
var loginMu sync.Mutex

const searchQueryCookie = "search_query"
const commentSearchCookie = "comment_search_query"

const (
	maxLoginAttempts    = 5
	loginLockoutMinutes = 15
)
