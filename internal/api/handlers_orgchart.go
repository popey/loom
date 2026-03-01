package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/jordanhubbard/loom/pkg/models"
)

// handleOrgChart handles org chart operations
func (s *Server) handleOrgChart(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/org-charts/")
	parts := strings.Split(path, "/")

	if len(parts) == 0 || parts[0] == "" {
		s.respondError(w, http.StatusBadRequest, "Org chart ID is required")
		return
	}

	orgChartID := parts[0]

	if len(parts) > 1 && parts[1] == "positions" {
		if len(parts) > 2 {
			positionID := parts[2]
			if len(parts) > 3 && parts[3] == "assign" {
				s.handleAssignAgentToPosition(w, r, orgChartID, positionID)
			} else if len(parts) > 3 && parts[3] == "unassign" {
				s.handleUnassignAgentFromPosition(w, r, orgChartID, positionID)
			} else {
				s.handleDeletePosition(w, r, orgChartID, positionID)
			}
		} else {
			s.handleAddPosition(w, r, orgChartID)
		}
	} else {
		switch r.Method {
		case http.MethodGet:
			s.handleGetOrgChart(w, r, orgChartID)
		case http.MethodPut:
			s.handleUpdateOrgChart(w, r, orgChartID)
		default:
			s.respondError(w, http.StatusMethodNotAllowed, "Method not allowed")
		}
	}
}

func (s *Server) handleGetOrgChart(w http.ResponseWriter, r *http.Request, orgChartID string) {
	if r.Method != http.MethodGet {
		s.respondError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	type OrgChartGetter interface {
		GetOrgChart(orgChartID string) (interface{}, error)
	}

	orgChartManager := s.app.GetOrgChartManager()
	if orgChartManager == nil {
		s.respondError(w, http.StatusServiceUnavailable, "Org chart manager not available")
		return
	}

	getter, ok := orgChartManager.(OrgChartGetter)
	if !ok {
		s.respondError(w, http.StatusInternalServerError, "Invalid org chart manager")
		return
	}

	orgChart, err := getter.GetOrgChart(orgChartID)
	if err != nil {
		s.respondError(w, http.StatusNotFound, fmt.Sprintf("Org chart not found: %v", err))
		return
	}

	s.respondJSON(w, http.StatusOK, orgChart)
}

func (s *Server) handleUpdateOrgChart(w http.ResponseWriter, r *http.Request, orgChartID string) {
	if r.Method != http.MethodPut {
		s.respondError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var req struct {
		Name       string `json:"name,omitempty"`
		IsTemplate bool   `json:"is_template,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondError(w, http.StatusBadRequest, fmt.Sprintf("Invalid request body: %v", err))
		return
	}

	type OrgChartUpdater interface {
		UpdateOrgChart(orgChartID, name string, isTemplate bool) error
	}

	orgChartManager := s.app.GetOrgChartManager()
	if orgChartManager == nil {
		s.respondError(w, http.StatusServiceUnavailable, "Org chart manager not available")
		return
	}

	updater, ok := orgChartManager.(OrgChartUpdater)
	if !ok {
		s.respondError(w, http.StatusInternalServerError, "Invalid org chart manager")
		return
	}

	if err := updater.UpdateOrgChart(orgChartID, req.Name, req.IsTemplate); err != nil {
		s.respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to update org chart: %v", err))
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]interface{}{"message": "Org chart updated successfully"})
}

func (s *Server) handleAddPosition(w http.ResponseWriter, r *http.Request, orgChartID string) {
	if r.Method != http.MethodPost {
		s.respondError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var req struct {
		RoleName     string `json:"role_name"`
		PersonaPath  string `json:"persona_path"`
		Required     bool   `json:"required"`
		MaxInstances int    `json:"max_instances"`
		ReportsTo    string `json:"reports_to,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondError(w, http.StatusBadRequest, fmt.Sprintf("Invalid request body: %v", err))
		return
	}

	if req.RoleName == "" {
		s.respondError(w, http.StatusBadRequest, "role_name is required")
		return
	}

	if req.PersonaPath == "" {
		s.respondError(w, http.StatusBadRequest, "persona_path is required")
		return
	}

	type PositionAdder interface {
		AddPosition(orgChartID, roleName, personaPath, reportsTo string, required bool, maxInstances int) (interface{}, error)
	}

	orgChartManager := s.app.GetOrgChartManager()
	if orgChartManager == nil {
		s.respondError(w, http.StatusServiceUnavailable, "Org chart manager not available")
		return
	}

	adder, ok := orgChartManager.(PositionAdder)
	if !ok {
		s.respondError(w, http.StatusInternalServerError, "Invalid org chart manager")
		return
	}

	position, err := adder.AddPosition(orgChartID, req.RoleName, req.PersonaPath, req.ReportsTo, req.Required, req.MaxInstances)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, fmt.Sprintf("Failed to add position: %v", err))
		return
	}

	s.respondJSON(w, http.StatusCreated, position)
}

func (s *Server) handleDeletePosition(w http.ResponseWriter, r *http.Request, orgChartID, positionID string) {
	if r.Method != http.MethodDelete {
		s.respondError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	type PositionRemover interface {
		RemovePosition(orgChartID, positionID string) error
	}

	orgChartManager := s.app.GetOrgChartManager()
	if orgChartManager == nil {
		s.respondError(w, http.StatusServiceUnavailable, "Org chart manager not available")
		return
	}

	remover, ok := orgChartManager.(PositionRemover)
	if !ok {
		s.respondError(w, http.StatusInternalServerError, "Invalid org chart manager")
		return
	}

	if err := remover.RemovePosition(orgChartID, positionID); err != nil {
		s.respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to remove position: %v", err))
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleAssignAgentToPosition(w http.ResponseWriter, r *http.Request, orgChartID, positionID string) {
	if r.Method != http.MethodPost {
		s.respondError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var req struct {
		AgentID string `json:"agent_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondError(w, http.StatusBadRequest, fmt.Sprintf("Invalid request body: %v", err))
		return
	}

	if req.AgentID == "" {
		s.respondError(w, http.StatusBadRequest, "agent_id is required")
		return
	}

	type AgentAssigner interface {
		AssignAgentToPosition(orgChartID, positionID, agentID string) error
	}

	orgChartManager := s.app.GetOrgChartManager()
	if orgChartManager == nil {
		s.respondError(w, http.StatusServiceUnavailable, "Org chart manager not available")
		return
	}

	assigner, ok := orgChartManager.(AgentAssigner)
	if !ok {
		s.respondError(w, http.StatusInternalServerError, "Invalid org chart manager")
		return
	}

	if err := assigner.AssignAgentToPosition(orgChartID, positionID, req.AgentID); err != nil {
		s.respondError(w, http.StatusBadRequest, fmt.Sprintf("Failed to assign agent: %v", err))
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]interface{}{"message": "Agent assigned successfully"})
}

func (s *Server) handleUnassignAgentFromPosition(w http.ResponseWriter, r *http.Request, orgChartID, positionID string) {
	if r.Method != http.MethodPost {
		s.respondError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var req struct {
		AgentID string `json:"agent_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondError(w, http.StatusBadRequest, fmt.Sprintf("Invalid request body: %v", err))
		return
	}

	if req.AgentID == "" {
		s.respondError(w, http.StatusBadRequest, "agent_id is required")
		return
	}

	type AgentUnassigner interface {
		UnassignAgentFromPosition(orgChartID, positionID, agentID string) error
	}

	orgChartManager := s.app.GetOrgChartManager()
	if orgChartManager == nil {
		s.respondError(w, http.StatusServiceUnavailable, "Org chart manager not available")
		return
	}

	unassigner, ok := orgChartManager.(AgentUnassigner)
	if !ok {
		s.respondError(w, http.StatusInternalServerError, "Invalid org chart manager")
		return
	}

	if err := unassigner.UnassignAgentFromPosition(orgChartID, positionID, req.AgentID); err != nil {
		s.respondError(w, http.StatusBadRequest, fmt.Sprintf("Failed to unassign agent: %v", err))
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]interface{}{"message": "Agent unassigned successfully"})
}
