package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/jordanhubbard/loom/pkg/models"
)

// handleDepartment handles department operations
// GET /api/v1/departments - List departments
// POST /api/v1/departments - Create department
// GET /api/v1/departments/{id} - Get specific department
// PUT /api/v1/departments/{id} - Update department
// DELETE /api/v1/departments/{id} - Delete department
func (s *Server) handleDepartment(w http.ResponseWriter, r *http.Request) {
	departmentID := s.extractID(r.URL.Path, "/api/v1/departments")

	switch r.Method {
	case http.MethodGet:
		if departmentID == "" {
			s.handleListDepartments(w, r)
		} else {
			s.handleGetDepartment(w, r, departmentID)
		}
	case http.MethodPost:
		s.handleCreateDepartment(w, r)
	case http.MethodPut:
		s.handleUpdateDepartment(w, r, departmentID)
	case http.MethodDelete:
		s.handleDeleteDepartment(w, r, departmentID)
	default:
		s.respondError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

// handleListDepartments lists all departments
func (s *Server) handleListDepartments(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.respondError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	// Parse query parameters
	projectID := r.URL.Query().Get("project_id")
	parentID := r.URL.Query().Get("parent_id")

	type DepartmentLister interface {
		ListDepartments(projectID, parentID string) ([]interface{}, error)
	}

	departmentManager := s.app.GetDepartmentManager()
	if departmentManager == nil {
		s.respondError(w, http.StatusServiceUnavailable, "Department manager not available")
		return
	}

	lister, ok := departmentManager.(DepartmentLister)
	if !ok {
		s.respondError(w, http.StatusInternalServerError, "Invalid department manager")
		return
	}

	departments, err := lister.ListDepartments(projectID, parentID)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to list departments: %v", err))
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]interface{}{
		"departments": departments,
		"count":       len(departments),
	})
}

// handleGetDepartment retrieves a specific department
func (s *Server) handleGetDepartment(w http.ResponseWriter, r *http.Request, departmentID string) {
	if r.Method != http.MethodGet {
		s.respondError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	type DepartmentGetter interface {
		GetDepartment(departmentID string) (interface{}, error)
	}

	departmentManager := s.app.GetDepartmentManager()
	if departmentManager == nil {
		s.respondError(w, http.StatusServiceUnavailable, "Department manager not available")
		return
	}

	getter, ok := departmentManager.(DepartmentGetter)
	if !ok {
		s.respondError(w, http.StatusInternalServerError, "Invalid department manager")
		return
	}

	department, err := getter.GetDepartment(departmentID)
	if err != nil {
		s.respondError(w, http.StatusNotFound, fmt.Sprintf("Department not found: %v", err))
		return
	}

	s.respondJSON(w, http.StatusOK, department)
}

// handleCreateDepartment creates a new department
func (s *Server) handleCreateDepartment(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.respondError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var req struct {
		ProjectID   string                 `json:"project_id"`
		Name        string                 `json:"name"`
		Description string                 `json:"description,omitempty"`
		ParentID    string                 `json:"parent_id,omitempty"`
		ManagerID   string                 `json:"manager_id,omitempty"`
		Budget      float64                `json:"budget,omitempty"`
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

	if req.Name == "" {
		s.respondError(w, http.StatusBadRequest, "name is required")
		return
	}

	type DepartmentCreator interface {
		CreateDepartment(projectID, name, description, parentID, managerID string, budget float64, metadata map[string]interface{}) (interface{}, error)
	}

	departmentManager := s.app.GetDepartmentManager()
	if departmentManager == nil {
		s.respondError(w, http.StatusServiceUnavailable, "Department manager not available")
		return
	}

	creator, ok := departmentManager.(DepartmentCreator)
	if !ok {
		s.respondError(w, http.StatusInternalServerError, "Invalid department manager")
		return
	}

	department, err := creator.CreateDepartment(req.ProjectID, req.Name, req.Description, req.ParentID, req.ManagerID, req.Budget, req.Metadata)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, fmt.Sprintf("Failed to create department: %v", err))
		return
	}

	s.respondJSON(w, http.StatusCreated, department)
}

// handleUpdateDepartment updates a department
func (s *Server) handleUpdateDepartment(w http.ResponseWriter, r *http.Request, departmentID string) {
	if r.Method != http.MethodPut {
		s.respondError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	if departmentID == "" {
		s.respondError(w, http.StatusBadRequest, "Department ID is required")
		return
	}

	var req struct {
		Name        string                 `json:"name,omitempty"`
		Description string                 `json:"description,omitempty"`
		ManagerID   string                 `json:"manager_id,omitempty"`
		Budget      float64                `json:"budget,omitempty"`
		Metadata    map[string]interface{} `json:"metadata,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondError(w, http.StatusBadRequest, fmt.Sprintf("Invalid request body: %v", err))
		return
	}

	type DepartmentUpdater interface {
		UpdateDepartment(departmentID, name, description, managerID string, budget float64, metadata map[string]interface{}) error
	}

	departmentManager := s.app.GetDepartmentManager()
	if departmentManager == nil {
		s.respondError(w, http.StatusServiceUnavailable, "Department manager not available")
		return
	}

	updater, ok := departmentManager.(DepartmentUpdater)
	if !ok {
		s.respondError(w, http.StatusInternalServerError, "Invalid department manager")
		return
	}

	if err := updater.UpdateDepartment(departmentID, req.Name, req.Description, req.ManagerID, req.Budget, req.Metadata); err != nil {
		if strings.Contains(err.Error(), "not found") {
			s.respondError(w, http.StatusNotFound, err.Error())
			return
		}
		s.respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to update department: %v", err))
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]interface{}{
		"message": "Department updated successfully",
	})
}

// handleDeleteDepartment deletes a department
func (s *Server) handleDeleteDepartment(w http.ResponseWriter, r *http.Request, departmentID string) {
	if r.Method != http.MethodDelete {
		s.respondError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	if departmentID == "" {
		s.respondError(w, http.StatusBadRequest, "Department ID is required")
		return
	}

	type DepartmentDeleter interface {
		DeleteDepartment(departmentID string) error
	}

	departmentManager := s.app.GetDepartmentManager()
	if departmentManager == nil {
		s.respondError(w, http.StatusServiceUnavailable, "Department manager not available")
		return
	}

	deleter, ok := departmentManager.(DepartmentDeleter)
	if !ok {
		s.respondError(w, http.StatusInternalServerError, "Invalid department manager")
		return
	}

	if err := deleter.DeleteDepartment(departmentID); err != nil {
		if strings.Contains(err.Error(), "not found") {
			s.respondError(w, http.StatusNotFound, err.Error())
			return
		}
		s.respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to delete department: %v", err))
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
