package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/jordanhubbard/loom/internal/auth"
)

// handleFeedback handles feedback operations
// GET /api/v1/feedback - List feedback
// POST /api/v1/feedback - Create feedback
// PATCH /api/v1/feedback/{id} - Update feedback
// DELETE /api/v1/feedback/{id} - Delete feedback
func (s *Server) handleFeedback(w http.ResponseWriter, r *http.Request) {
	feedbackMgr := s.app.GetFeedbackManager()
	if feedbackMgr == nil {
		s.respondError(w, http.StatusServiceUnavailable, "Feedback manager not available")
		return
	}

	// Get user from context
	user := s.getUserFromContext(r)
	if user == nil {
		s.respondError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	// Extract feedback ID from path if present
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/feedback")
	path = strings.TrimPrefix(path, "/")

	switch r.Method {
	case http.MethodGet:
		if path == "" {
			s.handleListFeedback(w, r, feedbackMgr)
		} else {
			s.handleGetFeedback(w, r, path, feedbackMgr)
		}
	case http.MethodPost:
		s.handleCreateFeedback(w, r, user, feedbackMgr)
	case http.MethodPatch:
		s.handleUpdateFeedback(w, r, path, user, feedbackMgr)
	case http.MethodDelete:
		s.handleDeleteFeedback(w, r, path, user, feedbackMgr)
	default:
		s.respondError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

// handleListFeedback lists feedback with optional filters
func (s *Server) handleListFeedback(w http.ResponseWriter, r *http.Request, feedbackMgr interface{}) {
	// Parse query parameters
	beadID := r.URL.Query().Get("bead_id")
	agentID := r.URL.Query().Get("agent_id")

	type FeedbackLister interface {
		GetFeedbackByBead(beadID string) (interface{}, error)
		GetFeedbackByAgent(agentID string) (interface{}, error)
	}

	lister, ok := feedbackMgr.(FeedbackLister)
	if !ok {
		s.respondError(w, http.StatusInternalServerError, "Invalid feedback manager")
		return
	}

	var feedbacks interface{}
	var err error

	if beadID != "" {
		feedbacks, err = lister.GetFeedbackByBead(beadID)
	} else if agentID != "" {
		feedbacks, err = lister.GetFeedbackByAgent(agentID)
	} else {
		s.respondError(w, http.StatusBadRequest, "Either bead_id or agent_id is required")
		return
	}

	if err != nil {
		s.respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to get feedback: %v", err))
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]interface{}{
		"feedback": feedbacks,
	})
}

// handleGetFeedback retrieves a specific feedback item
func (s *Server) handleGetFeedback(w http.ResponseWriter, r *http.Request, feedbackID string, feedbackMgr interface{}) {
	type FeedbackGetter interface {
		GetFeedback(feedbackID string) (interface{}, error)
	}

	getter, ok := feedbackMgr.(FeedbackGetter)
	if !ok {
		s.respondError(w, http.StatusInternalServerError, "Invalid feedback manager")
		return
	}

	feedback, err := getter.GetFeedback(feedbackID)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to get feedback: %v", err))
		return
	}

	s.respondJSON(w, http.StatusOK, feedback)
}

// handleCreateFeedback creates new feedback
func (s *Server) handleCreateFeedback(w http.ResponseWriter, r *http.Request, user *auth.User, feedbackMgr interface{}) {
	// Parse request body
	var req struct {
		BeadID   string                 `json:"bead_id,omitempty"`
		AgentID  string                 `json:"agent_id,omitempty"`
		Rating   int                    `json:"rating"`
		Category string                 `json:"category"`
		Content  string                 `json:"content"`
		Metadata map[string]interface{} `json:"metadata,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondError(w, http.StatusBadRequest, fmt.Sprintf("Invalid request body: %v", err))
		return
	}

	// Validate required fields
	if req.Rating < 1 || req.Rating > 5 {
		s.respondError(w, http.StatusBadRequest, "Rating must be between 1 and 5")
		return
	}

	if req.Category == "" {
		s.respondError(w, http.StatusBadRequest, "Category is required")
		return
	}

	if req.BeadID == "" && req.AgentID == "" {
		s.respondError(w, http.StatusBadRequest, "Either bead_id or agent_id is required")
		return
	}

	type FeedbackCreator interface {
		CreateFeedback(beadID, agentID, authorID, author, category, content string, rating int, metadata map[string]interface{}) (interface{}, error)
	}

	creator, ok := feedbackMgr.(FeedbackCreator)
	if !ok {
		s.respondError(w, http.StatusInternalServerError, "Invalid feedback manager")
		return
	}

	feedback, err := creator.CreateFeedback(req.BeadID, req.AgentID, user.ID, user.Username, req.Category, req.Content, req.Rating, req.Metadata)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to create feedback: %v", err))
		return
	}

	s.respondJSON(w, http.StatusCreated, feedback)
}

// handleUpdateFeedback updates feedback
func (s *Server) handleUpdateFeedback(w http.ResponseWriter, r *http.Request, feedbackID string, user *auth.User, feedbackMgr interface{}) {
	// Parse request body
	var req struct {
		Rating   int    `json:"rating"`
		Category string `json:"category"`
		Content  string `json:"content"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondError(w, http.StatusBadRequest, fmt.Sprintf("Invalid request body: %v", err))
		return
	}

	if req.Rating < 1 || req.Rating > 5 {
		s.respondError(w, http.StatusBadRequest, "Rating must be between 1 and 5")
		return
	}

	type FeedbackUpdater interface {
		UpdateFeedback(feedbackID, authorID, category, content string, rating int) error
	}

	updater, ok := feedbackMgr.(FeedbackUpdater)
	if !ok {
		s.respondError(w, http.StatusInternalServerError, "Invalid feedback manager")
		return
	}

	if err := updater.UpdateFeedback(feedbackID, user.ID, req.Category, req.Content, req.Rating); err != nil {
		if strings.Contains(err.Error(), "unauthorized") {
			s.respondError(w, http.StatusForbidden, err.Error())
			return
		}
		if strings.Contains(err.Error(), "not found") {
			s.respondError(w, http.StatusNotFound, err.Error())
			return
		}
		s.respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to update feedback: %v", err))
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]interface{}{
		"message": "Feedback updated successfully",
	})
}

// handleDeleteFeedback deletes feedback
func (s *Server) handleDeleteFeedback(w http.ResponseWriter, r *http.Request, feedbackID string, user *auth.User, feedbackMgr interface{}) {
	type FeedbackDeleter interface {
		DeleteFeedback(feedbackID, authorID string) error
	}

	deleter, ok := feedbackMgr.(FeedbackDeleter)
	if !ok {
		s.respondError(w, http.StatusInternalServerError, "Invalid feedback manager")
		return
	}

	if err := deleter.DeleteFeedback(feedbackID, user.ID); err != nil {
		if strings.Contains(err.Error(), "unauthorized") {
			s.respondError(w, http.StatusForbidden, err.Error())
			return
		}
		if strings.Contains(err.Error(), "not found") {
			s.respondError(w, http.StatusNotFound, err.Error())
			return
		}
		s.respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to delete feedback: %v", err))
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]interface{}{
		"message": "Feedback deleted successfully",
	})
}
