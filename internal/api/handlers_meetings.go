package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/jordanhubbard/loom/internal/meetings"
)

// handleMeetings handles GET and POST requests for meetings
func (s *Server) handleMeetings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleListMeetings(w, r)
	case http.MethodPost:
		s.handleCreateMeeting(w, r)
	default:
		s.respondError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

// handleListMeetings handles GET /api/v1/meetings
func (s *Server) handleListMeetings(w http.ResponseWriter, r *http.Request) {
	meetingsMgr := s.app.GetMeetingsManager()
	if meetingsMgr == nil {
		s.respondError(w, http.StatusServiceUnavailable, "Meetings manager not available")
		return
	}

	limit := 50
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	meeting, err := meetingsMgr.ListMeetings(limit)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to list meetings: %v", err))
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]interface{}{
		"meetings": meeting,
		"count":    len(meeting),
		"limit":    limit,
	})
}

// handleCreateMeeting handles POST /api/v1/meetings
func (s *Server) handleCreateMeeting(w http.ResponseWriter, r *http.Request) {
	meetingsMgr := s.app.GetMeetingsManager()
	if meetingsMgr == nil {
		s.respondError(w, http.StatusServiceUnavailable, "Meetings manager not available")
		return
	}

	var req meetings.CreateMeetingRequest
	if err := s.parseJSON(r, &req); err != nil {
		s.respondError(w, http.StatusBadRequest, fmt.Sprintf("Invalid request: %v", err))
		return
	}

	meeting, err := meetingsMgr.CreateMeeting(&req)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, fmt.Sprintf("Failed to create meeting: %v", err))
		return
	}

	s.respondJSON(w, http.StatusCreated, meeting)
}

// handleMeeting handles requests for a specific meeting
func (s *Server) handleMeeting(w http.ResponseWriter, r *http.Request) {
	id := s.extractID(r.URL.Path, "/api/v1/meetings")
	if id == "" {
		s.respondError(w, http.StatusBadRequest, "Meeting ID is required")
		return
	}

	if strings.Contains(r.URL.Path, "/transcript") {
		s.handleMeetingTranscript(w, r, id)
		return
	}
	if strings.Contains(r.URL.Path, "/action-items") {
		s.handleMeetingActionItems(w, r, id)
		return
	}
	if strings.Contains(r.URL.Path, "/start") {
		s.handleStartMeeting(w, r, id)
		return
	}
	if strings.Contains(r.URL.Path, "/complete") {
		s.handleCompleteMeeting(w, r, id)
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.handleGetMeeting(w, r, id)
	case http.MethodPut:
		s.handleUpdateMeeting(w, r, id)
	case http.MethodDelete:
		s.handleDeleteMeeting(w, r, id)
	default:
		s.respondError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

// handleGetMeeting handles GET /api/v1/meetings/{id}
func (s *Server) handleGetMeeting(w http.ResponseWriter, r *http.Request, id string) {
	meetingsMgr := s.app.GetMeetingsManager()
	if meetingsMgr == nil {
		s.respondError(w, http.StatusServiceUnavailable, "Meetings manager not available")
		return
	}

	meeting, err := meetingsMgr.GetMeeting(id)
	if err != nil {
		s.respondError(w, http.StatusNotFound, fmt.Sprintf("Meeting not found: %v", err))
		return
	}

	s.respondJSON(w, http.StatusOK, meeting)
}

// handleUpdateMeeting handles PUT /api/v1/meetings/{id}
func (s *Server) handleUpdateMeeting(w http.ResponseWriter, r *http.Request, id string) {
	meetingsMgr := s.app.GetMeetingsManager()
	if meetingsMgr == nil {
		s.respondError(w, http.StatusServiceUnavailable, "Meetings manager not available")
		return
	}

	var req meetings.UpdateMeetingRequest
	if err := s.parseJSON(r, &req); err != nil {
		s.respondError(w, http.StatusBadRequest, fmt.Sprintf("Invalid request: %v", err))
		return
	}

	meeting, err := meetingsMgr.UpdateMeeting(id, &req)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, fmt.Sprintf("Failed to update meeting: %v", err))
		return
	}

	s.respondJSON(w, http.StatusOK, meeting)
}

// handleDeleteMeeting handles DELETE /api/v1/meetings/{id}
// handleDeleteMeeting handles DELETE /api/v1/meetings/{id}
func (s *Server) handleDeleteMeeting(w http.ResponseWriter, r *http.Request, id string) {
	meetingsMgr := s.app.GetMeetingsManager()
	if meetingsMgr == nil {
		s.respondError(w, http.StatusServiceUnavailable, "Meetings manager not available")
		return
	}

	// For now, we just return NoContent since the manager doesn't have a delete method
	// In a real implementation, this would delete from the database
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleMeetingTranscript(w http.ResponseWriter, r *http.Request, id string) {
	meetingsMgr := s.app.GetMeetingsManager()
	if meetingsMgr == nil {
		s.respondError(w, http.StatusServiceUnavailable, "Meetings manager not available")
		return
	}

	switch r.Method {
	case http.MethodGet:
		meeting, err := meetingsMgr.GetMeeting(id)
		if err != nil {
			s.respondError(w, http.StatusNotFound, fmt.Sprintf("Meeting not found: %v", err))
			return
		}
		s.respondJSON(w, http.StatusOK, map[string]interface{}{
			"meeting_id": id,
			"transcript": meeting.Transcript,
			"count":      len(meeting.Transcript),
		})

	case http.MethodPost:
		var req meetings.AddTranscriptEntryRequest
		if err := s.parseJSON(r, &req); err != nil {
			s.respondError(w, http.StatusBadRequest, fmt.Sprintf("Invalid request: %v", err))
			return
		}

		entry, err := meetingsMgr.AddTranscriptEntry(id, &req)
		if err != nil {
			s.respondError(w, http.StatusBadRequest, fmt.Sprintf("Failed to add transcript entry: %v", err))
			return
		}

		s.respondJSON(w, http.StatusCreated, entry)

	default:
		s.respondError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

// handleMeetingActionItems handles action item operations
func (s *Server) handleMeetingActionItems(w http.ResponseWriter, r *http.Request, id string) {
	meetingsMgr := s.app.GetMeetingsManager()
	if meetingsMgr == nil {
		s.respondError(w, http.StatusServiceUnavailable, "Meetings manager not available")
		return
	}

	switch r.Method {
	case http.MethodGet:
		meeting, err := meetingsMgr.GetMeeting(id)
		if err != nil {
			s.respondError(w, http.StatusNotFound, fmt.Sprintf("Meeting not found: %v", err))
			return
		}
		s.respondJSON(w, http.StatusOK, map[string]interface{}{
			"meeting_id":   id,
			"action_items": meeting.ActionItems,
			"count":        len(meeting.ActionItems),
		})

	case http.MethodPost:
		var req meetings.AddActionItemRequest
		if err := s.parseJSON(r, &req); err != nil {
			s.respondError(w, http.StatusBadRequest, fmt.Sprintf("Invalid request: %v", err))
			return
		}

		item, err := meetingsMgr.AddActionItem(id, &req)
		if err != nil {
			s.respondError(w, http.StatusBadRequest, fmt.Sprintf("Failed to add action item: %v", err))
			return
		}

		s.respondJSON(w, http.StatusCreated, item)

	default:
		s.respondError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

// handleActiveMeetings handles GET /api/v1/meetings/active
func (s *Server) handleActiveMeetings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.respondError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	meetingsMgr := s.app.GetMeetingsManager()
	if meetingsMgr == nil {
		s.respondError(w, http.StatusServiceUnavailable, "Meetings manager not available")
		return
	}

	active, err := meetingsMgr.ListActiveMeetings()
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to list active meetings: %v", err))
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]interface{}{
		"meetings": active,
		"count":    len(active),
	})
}

// handleStartMeeting handles POST /api/v1/meetings/{id}/start
func (s *Server) handleStartMeeting(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost {
		s.respondError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	meetingsMgr := s.app.GetMeetingsManager()
	if meetingsMgr == nil {
		s.respondError(w, http.StatusServiceUnavailable, "Meetings manager not available")
		return
	}

	meeting, err := meetingsMgr.StartMeeting(id)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, fmt.Sprintf("Failed to start meeting: %v", err))
		return
	}

	s.respondJSON(w, http.StatusOK, meeting)
}

// handleCompleteMeeting handles POST /api/v1/meetings/{id}/complete
func (s *Server) handleCompleteMeeting(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost {
		s.respondError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	meetingsMgr := s.app.GetMeetingsManager()
	if meetingsMgr == nil {
		s.respondError(w, http.StatusServiceUnavailable, "Meetings manager not available")
		return
	}

	var req struct {
		Summary string `json:"summary"`
	}
	if err := s.parseJSON(r, &req); err != nil {
		s.respondError(w, http.StatusBadRequest, fmt.Sprintf("Invalid request: %v", err))
		return
	}

	meeting, err := meetingsMgr.CompleteMeeting(id, req.Summary)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, fmt.Sprintf("Failed to complete meeting: %v", err))
		return
	}

	s.respondJSON(w, http.StatusOK, meeting)
}
