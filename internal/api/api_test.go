package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"pr-reviewer-service/internal/models"
	"pr-reviewer-service/internal/storage"

	"github.com/gorilla/mux"
)

// MockStore for testing
type MockStore struct {
	teams map[string]models.Team
	users map[string]models.User
	prs   map[string]models.PullRequest
}

func NewMockStore() *MockStore {
	return &MockStore{
		teams: make(map[string]models.Team),
		users: make(map[string]models.User),
		prs:   make(map[string]models.PullRequest),
	}
}

func (m *MockStore) CreateTeam(name string, members []models.User) error {
	if _, exists := m.teams[name]; exists {
		return storage.ErrTeamExists
	}
	m.teams[name] = models.Team{Name: name, Members: members}
	for _, user := range members {
		m.users[user.UserID] = user
	}
	return nil
}

func (m *MockStore) GetTeam(name string) (models.Team, error) {
	team, exists := m.teams[name]
	if !exists {
		return models.Team{}, storage.ErrNotFound
	}
	return team, nil
}

func (m *MockStore) SetUserActive(userID string, active bool) (models.User, error) {
	user, exists := m.users[userID]
	if !exists {
		return models.User{}, storage.ErrNotFound
	}
	user.IsActive = active
	m.users[userID] = user
	return user, nil
}

func (m *MockStore) CreatePR(pr models.PullRequest) error {
	if _, exists := m.prs[pr.ID]; exists {
		return storage.ErrPRExists
	}
	
	// Simple auto-assignment logic for testing
	authorTeam := m.findUserTeam(pr.AuthorID)
	var reviewers []models.User
	for _, member := range authorTeam.Members {
		if member.UserID != pr.AuthorID && member.IsActive && len(reviewers) < 2 {
			reviewers = append(reviewers, member)
		}
	}
	
	pr.Reviewers = reviewers
	m.prs[pr.ID] = pr
	return nil
}

func (m *MockStore) GetPR(id string) (models.PullRequest, error) {
	pr, exists := m.prs[id]
	if !exists {
		return models.PullRequest{}, storage.ErrNotFound
	}
	return pr, nil
}

func (m *MockStore) MergePR(id string) (models.PullRequest, error) {
	pr, exists := m.prs[id]
	if !exists {
		return models.PullRequest{}, storage.ErrNotFound
	}
	pr.Status = models.MERGED
	now := time.Now()
	pr.MergedAt = &now
	m.prs[id] = pr
	return pr, nil
}

func (m *MockStore) ReassignReviewer(prID, oldReviewerID string) (models.PullRequest, string, error) {
	pr, exists := m.prs[prID]
	if !exists {
		return models.PullRequest{}, "", storage.ErrNotFound
	}
	
	if pr.Status == models.MERGED {
		return models.PullRequest{}, "", storage.ErrPRMerged
	}
	
	for i, reviewer := range pr.Reviewers {
		if reviewer.UserID == oldReviewerID {
			team := m.findUserTeam(oldReviewerID)
			for _, member := range team.Members {
				if member.UserID != oldReviewerID && member.IsActive {
					pr.Reviewers[i] = member
					return pr, member.UserID, nil
				}
			}
			return models.PullRequest{}, "", storage.ErrNoCandidate
		}
	}
	
	return models.PullRequest{}, "", storage.ErrNotAssigned
}

func (m *MockStore) ListPRsAssignedTo(userID string) ([]models.PullRequest, error) {
	var result []models.PullRequest
	for _, pr := range m.prs {
		for _, reviewer := range pr.Reviewers {
			if reviewer.UserID == userID {
				result = append(result, pr)
				break
			}
		}
	}
	return result, nil
}

func (m *MockStore) GetStats() (map[string]interface{}, error) {
	stats := map[string]interface{}{
		"total_teams": len(m.teams),
		"total_users": len(m.users),
		"total_prs":   len(m.prs),
		"user_assignments": []map[string]interface{}{
			{"user_id": "u1", "username": "Alice", "assignment_count": 2},
			{"user_id": "u2", "username": "Bob", "assignment_count": 1},
		},
	}
	return stats, nil
}

func (m *MockStore) MassDeactivate(teamName string, excludeUsers []string) (map[string]interface{}, error) {
	team, exists := m.teams[teamName]
	if !exists {
		return nil, storage.ErrNotFound
	}
	
	deactivated := 0
	for i, member := range team.Members {
		shouldExclude := false
		for _, exclude := range excludeUsers {
			if member.UserID == exclude {
				shouldExclude = true
				break
			}
		}
		
		if !shouldExclude {
			member.IsActive = false
			team.Members[i] = member
			m.users[member.UserID] = member
			deactivated++
		}
	}
	
	m.teams[teamName] = team
	
	return map[string]interface{}{
		"deactivated_users": deactivated,
		"team_name": teamName,
		"reassigned_prs": []string{},
		"reassigned_count": 0,
	}, nil
}

func (m *MockStore) findUserTeam(userID string) models.Team {
	for _, team := range m.teams {
		for _, member := range team.Members {
			if member.UserID == userID {
				return team
			}
		}
	}
	return models.Team{}
}

// Tests
func TestCreateTeam(t *testing.T) {
	store := NewMockStore()
	handler := NewHandler(store)
	router := mux.NewRouter()
	handler.RegisterRoutes(router)

	teamData := map[string]interface{}{
		"team_name": "backend",
		"members": []map[string]interface{}{
			{"user_id": "u1", "username": "Alice", "is_active": true},
		},
	}
	
	body, _ := json.Marshal(teamData)
	req := httptest.NewRequest("POST", "/team/add", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	
	if rr.Code != http.StatusCreated {
		t.Errorf("Expected status 201, got %d", rr.Code)
	}
}

func TestCreatePR(t *testing.T) {
	store := NewMockStore()
	
	// Create team first
	store.CreateTeam("backend", []models.User{
		{UserID: "u1", Username: "Alice", IsActive: true},
		{UserID: "u2", Username: "Bob", IsActive: true},
	})
	
	handler := NewHandler(store)
	router := mux.NewRouter()
	handler.RegisterRoutes(router)

	prData := map[string]interface{}{
		"pull_request_id":   "pr-1",
		"pull_request_name": "Test PR",
		"author_id":         "u1",
	}
	
	body, _ := json.Marshal(prData)
	req := httptest.NewRequest("POST", "/pullRequest/create", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	
	if rr.Code != http.StatusCreated {
		t.Errorf("Expected status 201, got %d", rr.Code)
	}
}

func TestGetStats(t *testing.T) {
	store := NewMockStore()
	handler := NewHandler(store)
	router := mux.NewRouter()
	handler.RegisterRoutes(router)

	req := httptest.NewRequest("GET", "/stats/assignments", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	
	if rr.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rr.Code)
	}
}

func TestMassDeactivate(t *testing.T) {
	store := NewMockStore()
	
	// Create team first
	store.CreateTeam("backend", []models.User{
		{UserID: "u1", Username: "Alice", IsActive: true},
		{UserID: "u2", Username: "Bob", IsActive: true},
	})
	
	handler := NewHandler(store)
	router := mux.NewRouter()
	handler.RegisterRoutes(router)

	deactivateData := map[string]interface{}{
		"exclude_users": []string{"u1"},
	}
	
	body, _ := json.Marshal(deactivateData)
	req := httptest.NewRequest("POST", "/team/backend/deactivate", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	
	if rr.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rr.Code)
	}
}