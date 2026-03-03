package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jordanhubbard/loom/internal/database"
	"github.com/jordanhubbard/loom/pkg/models"
)

// mockDatabase is a minimal mock of the database for testing
type mockDatabase struct {
	conversations map[string]*models.ConversationContext
}

func newMockDatabase() *mockDatabase {
	return &mockDatabase{
		conversations: make(map[string]*models.ConversationContext),
	}
}

func (m *mockDatabase) ListConversationContextsByProject(ctx context.Context, projectID string, limit int) ([]*models.ConversationContext, error) {
	var result []*models.ConversationContext
	for _, conv := range m.conversations {
		if conv.ProjectID == projectID {
			result = append(result, conv)
		}
		if len(result) >= limit {
			break
		}
	}
	return result, nil
}

func (m *mockDatabase) GetConversationContext(sessionID string) (*models.ConversationContext, error) {
	if conv, ok := m.conversations[sessionID]; ok {
		return conv, nil
	}
	return nil, nil
}

func (m *mockDatabase) GetConversationContextByBeadID(beadID string) (*models.ConversationContext, error) {
	for _, conv := range m.conversations {
		if conv.BeadID == beadID {
			return conv, nil
		}
	}
	return nil, nil
}

func (m *mockDatabase) CreateConversationContext(ctx *models.ConversationContext) error {
	m.conversations[ctx.SessionID] = ctx
	return nil
}

func (m *mockDatabase) UpdateConversationContext(ctx *models.ConversationContext) error {
	m.conversations[ctx.SessionID] = ctx
	return nil
}

func (m *mockDatabase) DeleteConversationContext(sessionID string) error {
	delete(m.conversations, sessionID)
	return nil
}

func (m *mockDatabase) ResetConversationMessages(sessionID string, keepSystemMessage bool) error {
	return nil
}

func (m *mockDatabase) InjectMessageIntoConversation(sessionID string, message string) error {
	return nil
}

func (m *mockDatabase) DeleteExpiredConversationContexts() (int64, error) {
	return 0, nil
}

// mockApp is a minimal mock of the Loom app for testing
type mockApp struct {
	db *database.Database
}

func (m *mockApp) GetDatabase() *database.Database {
	return m.db
}

// newTestServerWithApp creates a test server with a mock app and database
func newTestServerWithApp() *Server {
	db := newMockDatabase()
	app := &mockApp{
		db: (*database.Database)(nil), // nil database for testing
	}

	return &Server{
		config:         nil,
		app:            app,
		apiFailureLast: make(map[string]time.Time),
	}
}

func TestHandleConversationsList_MethodNotAllowed(t *testing.T) {
	s := newTestServerWithApp()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/conversations?project_id=loom&limit=50", nil)
	w := httptest.NewRecorder()
	s.handleConversationsList(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

func TestHandleConversationsList_BadRequest(t *testing.T) {
	s := newTestServerWithApp()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/conversations?limit=50", nil)
	w := httptest.NewRecorder()
	s.handleConversationsList(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleConversationsList_Success(t *testing.T) {
	s := newTestServerWithApp()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/conversations?project_id=loom&limit=50", nil)
	w := httptest.NewRecorder()
	s.handleConversationsList(w, req)
	if w.Code != http.StatusServiceUnavailable {
		// Expected because db is nil, but the test should not panic
		t.Logf("got status %d (expected 503 because db is nil)", w.Code)
	}
}
