package api

import (
	"encoding/json"
	"net/http"
	"time"
	"fmt"
	"github.com/gorilla/mux"
	"pr-reviewer-service/internal/models"
	"pr-reviewer-service/internal/storage"
)

type Handler struct {
	store storage.Store
}

func NewHandler(s storage.Store) *Handler {
	return &Handler{store: s}
}

func (h *Handler) RegisterRoutes(r *mux.Router) {
	// Teams
	r.HandleFunc("/team/add", h.createTeam).Methods("POST")
	r.HandleFunc("/team/get", h.getTeam).Methods("GET")
	
	// Users
	r.HandleFunc("/users/setIsActive", h.setUserActive).Methods("POST")
	
	// Pull Requests
	r.HandleFunc("/pullRequest/create", h.createPR).Methods("POST")
	r.HandleFunc("/pullRequest/merge", h.mergePR).Methods("POST")
	r.HandleFunc("/pullRequest/reassign", h.reassignReviewer).Methods("POST")
	r.HandleFunc("/users/getReview", h.listPRsAssignedTo).Methods("GET")
	
	// Health check
	r.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}).Methods("GET")
}

// Helpers
func decode(r *http.Request, v interface{}) error {
	return json.NewDecoder(r.Body).Decode(v)
}

func respondJSON(w http.ResponseWriter, code int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func respondError(w http.ResponseWriter, code, errorCode, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(getHTTPStatusCode(code))
	json.NewEncoder(w).Encode(map[string]interface{}{
		"error": map[string]string{
			"code":    errorCode,
			"message": message,
		},
	})
}

func getHTTPStatusCode(errorCode string) int {
	switch errorCode {
	case "TEAM_EXISTS", "PR_EXISTS":
		return http.StatusConflict
	case "NOT_FOUND":
		return http.StatusNotFound
	case "PR_MERGED", "NOT_ASSIGNED", "NO_CANDIDATE":
		return http.StatusConflict
	default:
		return http.StatusBadRequest
	}
}

// Handlers

func (h *Handler) createTeam(w http.ResponseWriter, r *http.Request) {
	var in struct {
		TeamName string          `json:"team_name"`
		Members  []models.User   `json:"members"`
	}
	if err := decode(r, &in); err != nil || in.TeamName == "" {
		respondError(w, "400", "BAD_REQUEST", "Invalid request body")
		return
	}

	if err := h.store.CreateTeam(in.TeamName, in.Members); err != nil {
		if err.Error() == "TEAM_EXISTS" {
			respondError(w, "409", "TEAM_EXISTS", "team_name already exists")
		} else {
			respondError(w, "400", "BAD_REQUEST", err.Error())
		}
		return
	}

	team := models.Team{
		Name:    in.TeamName,
		Members: in.Members,
	}
	respondJSON(w, 201, map[string]interface{}{"team": team})
}

func (h *Handler) getTeam(w http.ResponseWriter, r *http.Request) {
	teamName := r.URL.Query().Get("team_name")
	if teamName == "" {
		respondError(w, "400", "BAD_REQUEST", "team_name is required")
		return
	}

	team, err := h.store.GetTeam(teamName)
	if err != nil {
		respondError(w, "404", "NOT_FOUND", "team not found")
		return
	}

	respondJSON(w, 200, team)
}

func (h *Handler) setUserActive(w http.ResponseWriter, r *http.Request) {
	var in struct {
		UserID   string `json:"user_id"`
		IsActive bool   `json:"is_active"`
	}
	if err := decode(r, &in); err != nil || in.UserID == "" {
		respondError(w, "400", "BAD_REQUEST", "Invalid request body")
		return
	}

	user, err := h.store.SetUserActive(in.UserID, in.IsActive)
	if err != nil {
		respondError(w, "404", "NOT_FOUND", "user not found")
		return
	}

	respondJSON(w, 200, map[string]interface{}{"user": user})
}

func (h *Handler) createPR(w http.ResponseWriter, r *http.Request) {
    var in struct {
        PullRequestID   string `json:"pull_request_id"`
        PullRequestName string `json:"pull_request_name"`
        AuthorID        string `json:"author_id"`
    }
    
    fmt.Printf("DEBUG: Received PR creation request\n")
    
    if err := decode(r, &in); err != nil {
        fmt.Printf("DEBUG: JSON decode error: %v\n", err)
        respondError(w, "400", "BAD_REQUEST", "Invalid request body")
        return
    }
    
    fmt.Printf("DEBUG: Parsed data - PR ID: %s, Name: %s, Author: %s\n", in.PullRequestID, in.PullRequestName, in.AuthorID)

    if in.PullRequestID == "" || in.PullRequestName == "" || in.AuthorID == "" {
        fmt.Printf("DEBUG: Missing required fields\n")
        respondError(w, "400", "BAD_REQUEST", "Missing required fields")
        return
    }

    pr := models.PullRequest{
        ID:       in.PullRequestID,
        Title:    in.PullRequestName,
        AuthorID: in.AuthorID,
        Status:   models.OPEN,
        CreatedAt: func() *time.Time { t := time.Now(); return &t }(),
    }

    fmt.Printf("DEBUG: Creating PR in database...\n")
    if err := h.store.CreatePR(pr); err != nil {
        fmt.Printf("DEBUG: Store error: %v\n", err)
        if err.Error() == "PR_EXISTS" {
            respondError(w, "409", "PR_EXISTS", "PR id already exists")
        } else {
            respondError(w, "404", "NOT_FOUND", err.Error())
        }
        return
    }

    // Get the created PR with reviewers assigned
    createdPR, err := h.store.GetPR(pr.ID)
    if err != nil {
        respondError(w, "500", "INTERNAL_ERROR", "Failed to get created PR")
        return
    }

    fmt.Printf("DEBUG: PR created successfully\n")
    respondJSON(w, 201, map[string]interface{}{"pr": createdPR})
}

func (h *Handler) mergePR(w http.ResponseWriter, r *http.Request) {
	var in struct {
		PullRequestID string `json:"pull_request_id"`
	}
	if err := decode(r, &in); err != nil || in.PullRequestID == "" {
		respondError(w, "400", "BAD_REQUEST", "Invalid request body")
		return
	}

	pr, err := h.store.MergePR(in.PullRequestID)
	if err != nil {
		respondError(w, "404", "NOT_FOUND", "PR not found")
		return
	}

	respondJSON(w, 200, map[string]interface{}{"pr": pr})
}

func (h *Handler) reassignReviewer(w http.ResponseWriter, r *http.Request) {
	var in struct {
		PullRequestID string `json:"pull_request_id"`
		OldUserID     string `json:"old_user_id"`
	}
	if err := decode(r, &in); err != nil {
		respondError(w, "400", "BAD_REQUEST", "Invalid request body")
		return
	}

	if in.PullRequestID == "" || in.OldUserID == "" {
		respondError(w, "400", "BAD_REQUEST", "Missing required fields")
		return
	}

	pr, newReviewerID, err := h.store.ReassignReviewer(in.PullRequestID, in.OldUserID)
	if err != nil {
		switch err.Error() {
		case "NOT_FOUND":
			respondError(w, "404", "NOT_FOUND", "PR or user not found")
		case "PR_MERGED":
			respondError(w, "409", "PR_MERGED", "cannot reassign on merged PR")
		case "NOT_ASSIGNED":
			respondError(w, "409", "NOT_ASSIGNED", "reviewer is not assigned to this PR")
		case "NO_CANDIDATE":
			respondError(w, "409", "NO_CANDIDATE", "no active replacement candidate in team")
		default:
			respondError(w, "409", "CONFLICT", err.Error())
		}
		return
	}

	respondJSON(w, 200, map[string]interface{}{
		"pr":          pr,
		"replaced_by": newReviewerID,
	})
}

func (h *Handler) listPRsAssignedTo(w http.ResponseWriter, r *http.Request) {
	userID := r.URL.Query().Get("user_id")
	if userID == "" {
		respondError(w, "400", "BAD_REQUEST", "user_id is required")
		return
	}

	prs, err := h.store.ListPRsAssignedTo(userID)
	if err != nil {
		respondError(w, "500", "INTERNAL_ERROR", "Failed to get PRs")
		return
	}

	// Convert to short format
	var shortPRs []models.PullRequestShort
	for _, pr := range prs {
		shortPRs = append(shortPRs, models.PullRequestShort{
			ID:       pr.ID,
			Title:    pr.Title,
			AuthorID: pr.AuthorID,
			Status:   pr.Status,
		})
	}

	respondJSON(w, 200, map[string]interface{}{
		"user_id":       userID,
		"pull_requests": shortPRs,
	})
}