package storage

import (
	"database/sql"
	"errors"
	"time"

	"pr-reviewer-service/internal/models"

	"github.com/jmoiron/sqlx"
)

type Store interface {
	CreateTeam(name string, members []models.User) error
	GetTeam(name string) (models.Team, error)
	SetUserActive(userID string, active bool) (models.User, error)
	CreatePR(pr models.PullRequest) error
	GetPR(id string) (models.PullRequest, error)
	MergePR(id string) (models.PullRequest, error)
	ReassignReviewer(prID, oldReviewerID string) (models.PullRequest, string, error)
	ListPRsAssignedTo(userID string) ([]models.PullRequest, error)
}

type SQLStore struct {
	db *sqlx.DB
}

func NewSQLStore(db *sql.DB) Store {
	return &SQLStore{db: sqlx.NewDb(db, "postgres")}
}

// Team
func (s *SQLStore) CreateTeam(name string, members []models.User) error {
	tx, err := s.db.Beginx()
	if err != nil {
		return err
	}

	// Check if team exists
	var existingTeam string
	err = tx.Get(&existingTeam, "SELECT name FROM teams WHERE name = $1", name)
	if err == nil {
		tx.Rollback()
		return errors.New("TEAM_EXISTS")
	}

	// Create team
	_, err = tx.Exec("INSERT INTO teams (name) VALUES ($1)", name)
	if err != nil {
		tx.Rollback()
		return errors.New("TEAM_EXISTS")
	}

	// Create users and add to team
	for _, m := range members {
		// Insert or update user
		_, err := tx.Exec(
			"INSERT INTO users (user_id, username, is_active) VALUES ($1, $2, $3) ON CONFLICT (user_id) DO UPDATE SET username = EXCLUDED.username, is_active = EXCLUDED.is_active",
			m.UserID, m.Username, m.IsActive,
		)
		if err != nil {
			tx.Rollback()
			return err
		}

		// Add to team
		_, err = tx.Exec(
			"INSERT INTO team_members (team_name, user_id) VALUES ($1, $2) ON CONFLICT DO NOTHING",
			name, m.UserID,
		)
		if err != nil {
			tx.Rollback()
			return err
		}
	}

	return tx.Commit()
}

func (s *SQLStore) GetTeam(name string) (models.Team, error) {
	var team models.Team
	team.Name = name
	
	var members []models.User
	err := s.db.Select(&members, `
		SELECT u.user_id, u.username, u.is_active 
		FROM users u 
		JOIN team_members tm ON tm.user_id = u.user_id 
		WHERE tm.team_name = $1`, name)
	if err != nil {
		return team, err
	}
	
	team.Members = members
	return team, nil
}

// User
func (s *SQLStore) SetUserActive(userID string, active bool) (models.User, error) {
	_, err := s.db.Exec("UPDATE users SET is_active = $1 WHERE user_id = $2", active, userID)
	if err != nil {
		return models.User{}, err
	}
	
	var u models.User
	err = s.db.Get(&u, "SELECT user_id, username, is_active FROM users WHERE user_id = $1", userID)
	if err != nil {
		return models.User{}, err
	}
	
	// Get team name
	var teamName string
	err = s.db.Get(&teamName, "SELECT team_name FROM team_members WHERE user_id = $1 LIMIT 1", userID)
	if err == nil {
		u.TeamName = teamName
	}
	
	return u, nil
}

// PRs
func (s *SQLStore) CreatePR(pr models.PullRequest) error {
	// Check if PR already exists
	var existingPR string
	err := s.db.Get(&existingPR, "SELECT pull_request_id FROM prs WHERE pull_request_id = $1", pr.ID)
	if err == nil {
		return errors.New("PR_EXISTS")
	}

	// Create PR
	_, err = s.db.Exec(
		"INSERT INTO prs (pull_request_id, pull_request_name, author_id, status, created_at) VALUES ($1, $2, $3, $4, $5)",
		pr.ID, pr.Title, pr.AuthorID, pr.Status, pr.CreatedAt,
	)
	if err != nil {
		return err
	}

	// Get author's team and assign reviewers
	var teamName string
	err = s.db.Get(&teamName, "SELECT team_name FROM team_members WHERE user_id = $1", pr.AuthorID)
	if err != nil {
		return errors.New("author team not found")
	}

	// Find active reviewers from the same team (excluding author)
	var reviewers []string
	err = s.db.Select(&reviewers, `
		SELECT u.user_id 
		FROM users u 
		JOIN team_members tm ON tm.user_id = u.user_id 
		WHERE tm.team_name = $1 
		AND u.is_active = true 
		AND u.user_id != $2 
		LIMIT 2`,
		teamName, pr.AuthorID)
	if err != nil {
		return err
	}

	// Assign reviewers
	for _, reviewerID := range reviewers {
		_, err = s.db.Exec(
			"INSERT INTO pr_reviewers (pull_request_id, user_id) VALUES ($1, $2)",
			pr.ID, reviewerID,
		)
		if err != nil {
			return err
		}
	}

	return nil
}

func (s *SQLStore) GetPR(id string) (models.PullRequest, error) {
	var pr models.PullRequest
	err := s.db.Get(&pr, `
		SELECT pull_request_id, pull_request_name, author_id, status, created_at, merged_at 
		FROM prs 
		WHERE pull_request_id = $1`, id)
	if err != nil {
		return pr, err
	}

	var reviewerIDs []string
	err = s.db.Select(&reviewerIDs, "SELECT user_id FROM pr_reviewers WHERE pull_request_id = $1", id)
	if err != nil {
		return pr, err
	}

	var reviewers []models.User
	for _, reviewerID := range reviewerIDs {
		var u models.User
		err = s.db.Get(&u, "SELECT user_id, username, is_active FROM users WHERE user_id = $1", reviewerID)
		if err == nil {
			reviewers = append(reviewers, u)
		}
	}
	pr.Reviewers = reviewers

	return pr, nil
}

func (s *SQLStore) MergePR(id string) (models.PullRequest, error) {
	// Check if PR exists and is not already merged
	var currentStatus string
	err := s.db.Get(&currentStatus, "SELECT status FROM prs WHERE pull_request_id = $1", id)
	if err != nil {
		return models.PullRequest{}, errors.New("PR not found")
	}

	if currentStatus != "MERGED" {
		_, err = s.db.Exec(
			"UPDATE prs SET status = 'MERGED', merged_at = $1 WHERE pull_request_id = $2",
			time.Now(), id,
		)
		if err != nil {
			return models.PullRequest{}, err
		}
	}

	return s.GetPR(id)
}

func (s *SQLStore) ReassignReviewer(prID, oldReviewerID string) (models.PullRequest, string, error) {
	// Check if PR is merged
	var status string
	err := s.db.Get(&status, "SELECT status FROM prs WHERE pull_request_id = $1", prID)
	if err != nil {
		return models.PullRequest{}, "", errors.New("NOT_FOUND")
	}
	if status == "MERGED" {
		return models.PullRequest{}, "", errors.New("PR_MERGED")
	}

	// Check if old reviewer is assigned
	var isAssigned bool
	err = s.db.Get(&isAssigned, "SELECT EXISTS(SELECT 1 FROM pr_reviewers WHERE pull_request_id = $1 AND user_id = $2)", prID, oldReviewerID)
	if err != nil || !isAssigned {
		return models.PullRequest{}, "", errors.New("NOT_ASSIGNED")
	}

	// Get team of old reviewer
	var teamName string
	err = s.db.Get(&teamName, "SELECT team_name FROM team_members WHERE user_id = $1", oldReviewerID)
	if err != nil {
		return models.PullRequest{}, "", errors.New("NOT_FOUND")
	}

	// Find replacement (active user from same team, not already assigned, not the old reviewer)
	var newReviewerID string
	err = s.db.Get(&newReviewerID, `
		SELECT u.user_id 
		FROM users u 
		JOIN team_members tm ON tm.user_id = u.user_id 
		WHERE tm.team_name = $1 
		AND u.is_active = true 
		AND u.user_id != $2
		AND u.user_id NOT IN (
			SELECT user_id FROM pr_reviewers WHERE pull_request_id = $3
		)
		LIMIT 1`,
		teamName, oldReviewerID, prID)
	if err != nil {
		return models.PullRequest{}, "", errors.New("NO_CANDIDATE")
	}

	// Perform reassignment
	_, err = s.db.Exec(
		"UPDATE pr_reviewers SET user_id = $1 WHERE pull_request_id = $2 AND user_id = $3",
		newReviewerID, prID, oldReviewerID,
	)
	if err != nil {
		return models.PullRequest{}, "", err
	}

	pr, _ := s.GetPR(prID)
	return pr, newReviewerID, nil
}

func (s *SQLStore) ListPRsAssignedTo(userID string) ([]models.PullRequest, error) {
	var prs []models.PullRequest
	err := s.db.Select(&prs, `
		SELECT p.pull_request_id, p.pull_request_name, p.author_id, p.status, p.created_at, p.merged_at 
		FROM prs p 
		JOIN pr_reviewers r ON r.pull_request_id = p.pull_request_id 
		WHERE r.user_id = $1`, userID)
	if err != nil {
		return nil, err
	}

	for i := range prs {
		pr, _ := s.GetPR(prs[i].ID)
		prs[i].Reviewers = pr.Reviewers
	}

	return prs, nil
}