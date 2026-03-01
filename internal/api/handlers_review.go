package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/jordanhubbard/loom/internal/auth"
)

// handleReview handles review operations
// GET /api/v1/review - List reviews
// POST /api/v1/review - Create review
// GET /api/v1/review/{id} - Get specific review
// PUT /api/v1/review/{id} - Update review
// DELETE /api/v1/review/{id} - Delete review
func (s *Server) handleReview(w http.ResponseWriter, r *http.Request) {
	reviewID := s.extractID(r.URL.Path, "/api/v1/review")

	switch r.Method {
	case http.MethodGet:
		if reviewID == "" {
			s.handleListReviews(w, r)
		} else {
			s.handleGetReview(w, r, reviewID)
		}
	case http.MethodPost:
		s.handleCreateReview(w, r)
	case http.MethodPut:
		s.handleUpdateReview(w, r, reviewID)
	case http.MethodDelete:
		s.handleDeleteReview(w, r, reviewID)
	default:
		s.respondError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

// handleListReviews lists all reviews
func (s *Server) handleListReviews(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.respondError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	// Parse query parameters
	projectID := r.URL.Query().Get("project_id")
	beadID := r.URL.Query().Get("bead_id")
	agentID := r.URL.Query().Get("agent_id")
	status := r.URL.Query().Get("status")

	type ReviewLister interface {
		ListReviews(projectID, beadID, agentID, status string) ([]interface{}, error)
	}

	reviewManager := s.app.GetReviewManager()
	if reviewManager == nil {
		s.respondError(w, http.StatusServiceUnavailable, "Review manager not available")
		return
	}

	lister, ok := reviewManager.(ReviewLister)
	if !ok {
		s.respondError(w, http.StatusInternalServerError, "Invalid review manager")
		return
	}

	reviews, err := lister.ListReviews(projectID, beadID, agentID, status)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to list reviews: %v", err))
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]interface{}{
		"reviews": reviews,
		"count":   len(reviews),
	})
}

// handleGetReview retrieves a specific review
func (s *Server) handleGetReview(w http.ResponseWriter, r *http.Request, reviewID string) {
	if r.Method != http.MethodGet {
		s.respondError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	type ReviewGetter interface {
		GetReview(reviewID string) (interface{}, error)
	}

	reviewManager := s.app.GetReviewManager()
	if reviewManager == nil {
		s.respondError(w, http.StatusServiceUnavailable, "Review manager not available")
		return
	}

	getter, ok := reviewManager.(ReviewGetter)
	if !ok {
		s.respondError(w, http.StatusInternalServerError, "Invalid review manager")
		return
	}

	review, err := getter.GetReview(reviewID)
	if err != nil {
		s.respondError(w, http.StatusNotFound, fmt.Sprintf("Review not found: %v", err))
		return
	}

	s.respondJSON(w, http.StatusOK, review)
}

// handleCreateReview creates a new review
func (s *Server) handleCreateReview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.respondError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	user := s.getUserFromContext(r)
	if user == nil {
		s.respondError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	var req struct {
		ProjectID   string                 `json:"project_id"`
		BeadID      string                 `json:"bead_id,omitempty"`
		AgentID     string                 `json:"agent_id,omitempty"`
		ReviewType  string                 `json:"review_type"` // "code", "design", "performance", "security"
		Title       string                 `json:"title"`
		Description string                 `json:"description"`
		Findings    []string               `json:"findings,omitempty"`
		Metadata    map[string]interface{} `json:"metadata,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondError(w, http.StatusBadRequest, fmt.Sprintf("Invalid request body: %v", err))
		return
	}

	if req.ProjectID == "" {
		s.respondError(w, http.StatusBadRequest, "project_id is required")
		return
	}

	if req.ReviewType == "" {
		s.respondError(w, http.StatusBadRequest, "review_type is required")
		return
	}

	if req.Title == "" {
		s.respondError(w, http.StatusBadRequest, "title is required")
		return
	}

	type ReviewCreator interface {
		CreateReview(projectID, beadID, agentID, reviewType, title, description, authorID string, findings []string, metadata map[string]interface{}) (interface{}, error)
	}

	reviewManager := s.app.GetReviewManager()
	if reviewManager == nil {
		s.respondError(w, http.StatusServiceUnavailable, "Review manager not available")
		return
	}

	creator, ok := reviewManager.(ReviewCreator)
	if !ok {
		s.respondError(w, http.StatusInternalServerError, "Invalid review manager")
		return
	}

	review, err := creator.CreateReview(req.ProjectID, req.BeadID, req.AgentID, req.ReviewType, req.Title, req.Description, user.ID, req.Findings, req.Metadata)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, fmt.Sprintf("Failed to create review: %v", err))
		return
	}

	s.respondJSON(w, http.StatusCreated, review)
}

// handleUpdateReview updates a review
func (s *Server) handleUpdateReview(w http.ResponseWriter, r *http.Request, reviewID string) {
	if r.Method != http.MethodPut {
		s.respondError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	if reviewID == "" {
		s.respondError(w, http.StatusBadRequest, "Review ID is required")
		return
	}

	user := s.getUserFromContext(r)
	if user == nil {
		s.respondError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	var req struct {
		Title       string                 `json:"title,omitempty"`
		Description string                 `json:"description,omitempty"`
		Findings    []string               `json:"findings,omitempty"`
		Status      string                 `json:"status,omitempty"` // "draft", "in_progress", "completed"
		Metadata    map[string]interface{} `json:"metadata,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondError(w, http.StatusBadRequest, fmt.Sprintf("Invalid request body: %v", err))
		return
	}

	type ReviewUpdater interface {
		UpdateReview(reviewID, title, description, status, authorID string, findings []string, metadata map[string]interface{}) error
	}

	reviewManager := s.app.GetReviewManager()
	if reviewManager == nil {
		s.respondError(w, http.StatusServiceUnavailable, "Review manager not available")
		return
	}

	updater, ok := reviewManager.(ReviewUpdater)
	if !ok {
		s.respondError(w, http.StatusInternalServerError, "Invalid review manager")
		return
	}

	if err := updater.UpdateReview(reviewID, req.Title, req.Description, req.Status, user.ID, req.Findings, req.Metadata); err != nil {
		if strings.Contains(err.Error(), "unauthorized") {
			s.respondError(w, http.StatusForbidden, err.Error())
			return
		}
		if strings.Contains(err.Error(), "not found") {
			s.respondError(w, http.StatusNotFound, err.Error())
			return
		}
		s.respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to update review: %v", err))
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]interface{}{
		"message": "Review updated successfully",
	})
}

// handleDeleteReview deletes a review
func (s *Server) handleDeleteReview(w http.ResponseWriter, r *http.Request, reviewID string) {
	if r.Method != http.MethodDelete {
		s.respondError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	if reviewID == "" {
		s.respondError(w, http.StatusBadRequest, "Review ID is required")
		return
	}

	user := s.getUserFromContext(r)
	if user == nil {
		s.respondError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	type ReviewDeleter interface {
		DeleteReview(reviewID, authorID string) error
	}

	reviewManager := s.app.GetReviewManager()
	if reviewManager == nil {
		s.respondError(w, http.StatusServiceUnavailable, "Review manager not available")
		return
	}

	deleter, ok := reviewManager.(ReviewDeleter)
	if !ok {
		s.respondError(w, http.StatusInternalServerError, "Invalid review manager")
		return
	}

	if err := deleter.DeleteReview(reviewID, user.ID); err != nil {
		if strings.Contains(err.Error(), "unauthorized") {
			s.respondError(w, http.StatusForbidden, err.Error())
			return
		}
		if strings.Contains(err.Error(), "not found") {
			s.respondError(w, http.StatusNotFound, err.Error())
			return
		}
		s.respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to delete review: %v", err))
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
