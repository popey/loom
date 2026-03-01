package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/jordanhubbard/loom/pkg/models"
)

// handleStatus handles status operations
// GET /api/v1/status - List all statuses
// POST /api/v1/status - Create new status
// GET /api/v1/status/{id} - Get specific status
// PUT /api/v1/status/{id} - Update status
// DELETE /api/v1/status/{id} - Delete status
func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	statusID := s.extractID(r.URL.Path, "/api/v1/status")

	switch r.Method {
	case http.MethodGet:
		if statusID == "" {
			s.handleListStatuses(w, r)
		} else {
			s.handleGetStatus(w, r, statusID)
		}
	case http.MethodPost:
		s.handleCreateStatus(w, r)
	case http.MethodPut:
		s.handleUpdateStatus(w, r, statusID)
	case http.MethodDelete:
		s.handleDeleteStatus(w, r, statusID)
	default:
		s.respondError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

// handleListStatuses lists all statuses
func (s *Server) handleListStatuses(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.respondError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	// Parse query parameters
	projectID := r.URL.Query().Get("project_id")
	beadID := r.URL.Query().Get("bead_id")
	agentID := r.URL.Query().Get("agent_id")

	type StatusLister interface {
		ListStatuses(projectID, beadID, agentID string) ([]interface{}, error)
	}

	statusManager := s.app.GetStatusManager()
	if statusManager == nil {
		s.respondError(w, http.StatusServiceUnavailable, "Status manager not available")
		return
	}

	lister, ok := statusManager.(StatusLister)
	if !ok {
		s.respondError(w, http.StatusInternalServerError, "Invalid status manager")
		return
	}

	statuses, err := lister.ListStatuses(projectID, beadID, agentID)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to list statuses: %v", err))
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]interface{}{
		"statuses": statuses,
		"count":    len(statuses),
	})
}

// handleGetStatus retrieves a specific status
func (s *Server) handleGetStatus(w http.ResponseWriter, r *http.Request, statusID string) {
	if r.Method != http.MethodGet {
		s.respondError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	type StatusGetter interface {
		GetStatus(statusID string) (interface{}, error)
	}

	statusManager := s.app.GetStatusManager()
	if statusManager == nil {
		s.respondError(w, http.StatusServiceUnavailable, "Status manager not available")
		return
	}

	getter, ok := statusManager.(StatusGetter)
	if !ok {
		s.respondError(w, http.StatusInternalServerError, "Invalid status manager")
		return
	}

	status, err := getter.GetStatus(statusID)
	if err != nil {
		s.respondError(w, http.StatusNotFound, fmt.Sprintf("Status not found: %v", err))
		return
	}

	s.respondJSON(w, http.StatusOK, status)
}

// handleCreateStatus creates a new status
func (s *Server) handleCreateStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.respondError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var req struct {
		ProjectID   string                 `json:"project_id"`
		BeadID      string                 `json:"bead_id,omitempty"`
		AgentID     string                 `json:"agent_id,omitempty"`
		StatusType  string                 `json:"status_type"` // "progress", "health", "milestone"
		Value       string                 `json:"value"`
		Description string                 `json:"description,omitempty"`
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

	if req.StatusType == "" {
		s.respondError(w, http.StatusBadRequest, "status_type is required")
		return
	}

	if req.Value == "" {
		s.respondError(w, http.StatusBadRequest, "value is required")
		return
	}

	type StatusCreator interface {
		CreateStatus(projectID, beadID, agentID, statusType, value, description string, metadata map[string]interface{}) (interface{}, error)
	}

	statusManager := s.app.GetStatusManager()
	if statusManager == nil {
		s.respondError(w, http.StatusServiceUnavailable, "Status manager not available")
		return
	}

	creator, ok := statusManager.(StatusCreator)
	if !ok {
		s.respondError(w, http.StatusInternalServerError, "Invalid status manager")
		return
	}

	status, err := creator.CreateStatus(req.ProjectID, req.BeadID, req.AgentID, req.StatusType, req.Value, req.Description, req.Metadata)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, fmt.Sprintf("Failed to create status: %v", err))
		return
	}

	s.respondJSON(w, http.StatusCreated, status)
}

// handleUpdateStatus updates a status
func (s *Server) handleUpdateStatus(w http.ResponseWriter, r *http.Request, statusID string) {
	if r.Method != http.MethodPut {
		s.respondError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	if statusID == "" {
		s.respondError(w, http.StatusBadRequest, "Status ID is required")
		return
	}

	var req struct {
		Value       string                 `json:"value,omitempty"`
		Description string                 `json:"description,omitempty"`
		Metadata    map[string]interface{} `json:"metadata,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondError(w, http.StatusBadRequest, fmt.Sprintf("Invalid request body: %v", err))
		return
	}

	type StatusUpdater interface {
		UpdateStatus(statusID, value, description string, metadata map[string]interface{}) error
	}

	statusManager := s.app.GetStatusManager()
	if statusManager == nil {
		s.respondError(w, http.StatusServiceUnavailable, "Status manager not available")
		return
	}

	updater, ok := statusManager.(StatusUpdater)
	if !ok {
		s.respondError(w, http.StatusInternalServerError, "Invalid status manager")
		return
	}

	if err := updater.UpdateStatus(statusID, req.Value, req.Description, req.Metadata); err != nil {
		s.respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to update status: %v", err))
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]interface{}{
		"message": "Status updated successfully",
	})
}

// handleDeleteStatus deletes a status
func (s *Server) handleDeleteStatus(w http.ResponseWriter, r *http.Request, statusID string) {
	if r.Method != http.MethodDelete {
		s.respondError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	if statusID == "" {
		s.respondError(w, http.StatusBadRequest, "Status ID is required")
		return
	}

	type StatusDeleter interface {
		DeleteStatus(statusID string) error
	}

	statusManager := s.app.GetStatusManager()
	if statusManager == nil {
		s.respondError(w, http.StatusServiceUnavailable, "Status manager not available")
		return
	}

	deleter, ok := statusManager.(StatusDeleter)
	if !ok {
		s.respondError(w, http.StatusInternalServerError, "Invalid status manager")
		return
	}

	if err := deleter.DeleteStatus(statusID); err != nil {
		s.respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to delete status: %v", err))
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
