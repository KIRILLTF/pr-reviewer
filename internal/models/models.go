package models

import (
	"time"
)

type User struct {
	UserID   string `db:"user_id" json:"user_id"`
	Username string `db:"username" json:"username"`
	IsActive bool   `db:"is_active" json:"is_active"`
	TeamName string `db:"team_name" json:"team_name,omitempty"`
}

type Team struct {
	Name    string `db:"name" json:"team_name"`
	Members []User `json:"members"`
}

type PRStatus string

const (
	OPEN   PRStatus = "OPEN"
	MERGED PRStatus = "MERGED"
)

type PullRequest struct {
	ID               string    `db:"pull_request_id" json:"pull_request_id"`
	Title            string    `db:"pull_request_name" json:"pull_request_name"`
	AuthorID         string    `db:"author_id" json:"author_id"`
	Status           PRStatus  `db:"status" json:"status"`
	Reviewers        []User    `json:"assigned_reviewers"`
	CreatedAt        *time.Time `db:"created_at" json:"createdAt,omitempty"`
	MergedAt         *time.Time `db:"merged_at" json:"mergedAt,omitempty"`
}

type PullRequestShort struct {
	ID       string   `json:"pull_request_id"`
	Title    string   `json:"pull_request_name"`
	AuthorID string   `json:"author_id"`
	Status   PRStatus `json:"status"`
}