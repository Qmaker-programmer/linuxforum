package main

import (
	"database/sql"
	"sync"
	"time"
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
}

var db *sql.DB
var sessions = make(map[string]Session)
var config Config
var requestCounts = make(map[string]int)
var mu sync.Mutex

const searchQueryCookie = "search_query"
const commentSearchCookie = "comment_search_query"
