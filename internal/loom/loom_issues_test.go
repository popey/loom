package loom

import (
	"os"
	"testing"
	"time"

	"github.com/jordanhubbard/loom/internal/motivation"
)

// ---------------------------------------------------------------------------
// Issue #37: GetProjectIdle
// ---------------------------------------------------------------------------

func TestGetProjectIdle_EmptyProjectID(t *testing.T) {
	l, tmpDir := testLoom(t)
	defer os.RemoveAll(tmpDir)

	_, err := l.GetProjectIdle("", time.Minute)
	if err == nil {
		t.Error("expected error for empty project ID")
	}
}

func TestGetProjectIdle_NoActivityManager(t *testing.T) {
	l, tmpDir := testLoom(t)
	defer os.RemoveAll(tmpDir)

	// Without a database there is no activity manager; should return false, nil.
	if l.activityManager != nil {
		t.Skip("activity manager present, skipping no-activity-manager path")
	}
	idle, err := l.GetProjectIdle("proj-1", time.Minute)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if idle {
		t.Error("expected false when activity manager is unavailable")
	}
}

func TestGetProjectIdle_NoActivity(t *testing.T) {
	l, tmpDir := testLoom(t)
	defer os.RemoveAll(tmpDir)
	requireDatabase(t, l)

	// A project that has never generated activity should be considered idle.
	idle, err := l.GetProjectIdle("nonexistent-project-id", time.Hour)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if !idle {
		t.Error("expected project with no activity to be idle")
	}
}

// ---------------------------------------------------------------------------
// Issue #35: GetPendingDecisions
// ---------------------------------------------------------------------------

func TestGetPendingDecisions_ReturnsSlice(t *testing.T) {
	l, tmpDir := testLoom(t)
	defer os.RemoveAll(tmpDir)

	ids, err := l.GetPendingDecisions()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if ids == nil {
		t.Error("expected non-nil slice")
	}
}

func TestGetPendingDecisions_IncludesCreatedDecision(t *testing.T) {
	l, tmpDir := testLoom(t)
	defer os.RemoveAll(tmpDir)

	d, err := l.decisionManager.CreateDecision("should we refactor?", "", "user-1", nil, "", 0, "proj-1")
	if err != nil {
		t.Fatalf("failed to create test decision: %v", err)
	}

	ids, err := l.GetPendingDecisions()
	if err != nil {
		t.Fatalf("GetPendingDecisions error: %v", err)
	}

	found := false
	for _, id := range ids {
		if id == d.ID {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected decision %s in pending list, got %v", d.ID, ids)
	}
}

func TestGetPendingDecisions_ExcludesResolvedDecision(t *testing.T) {
	l, tmpDir := testLoom(t)
	defer os.RemoveAll(tmpDir)

	d, err := l.decisionManager.CreateDecision("ship it?", "", "user-1", nil, "", 0, "proj-2")
	if err != nil {
		t.Fatalf("failed to create test decision: %v", err)
	}
	_ = l.decisionManager.MakeDecision(d.ID, "user-1", "approve", "looks good")

	ids, err := l.GetPendingDecisions()
	if err != nil {
		t.Fatalf("GetPendingDecisions error: %v", err)
	}

	for _, id := range ids {
		if id == d.ID {
			t.Errorf("resolved decision %s should not appear in pending list", d.ID)
		}
	}
}

// ---------------------------------------------------------------------------
// Issue #34: GetAgentsByRole / WakeAgent
// ---------------------------------------------------------------------------

func TestGetAgentsByRole_Empty(t *testing.T) {
	l, tmpDir := testLoom(t)
	defer os.RemoveAll(tmpDir)

	ids, err := l.GetAgentsByRole("nonexistent-role")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if len(ids) != 0 {
		t.Errorf("expected empty list, got %v", ids)
	}
}

func TestGetAgentsByRole_MatchesRole(t *testing.T) {
	l, tmpDir := testLoom(t)
	defer os.RemoveAll(tmpDir)

	ag, err := l.agentManager.CreateAgent(t.Context(), "Test Agent", "test-persona", "proj-1", "engineer", nil)
	if err != nil {
		t.Fatalf("failed to create agent: %v", err)
	}

	ids, err := l.GetAgentsByRole("engineer")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	found := false
	for _, id := range ids {
		if id == ag.ID {
			found = true
		}
	}
	if !found {
		t.Errorf("expected agent %s in role list, got %v", ag.ID, ids)
	}
}

func TestWakeAgent_NotFound(t *testing.T) {
	l, tmpDir := testLoom(t)
	defer os.RemoveAll(tmpDir)

	err := l.WakeAgent("nonexistent-agent", nil)
	if err == nil {
		t.Error("expected error for nonexistent agent")
	}
}

func TestWakeAgent_IdleAgentStaysIdle(t *testing.T) {
	l, tmpDir := testLoom(t)
	defer os.RemoveAll(tmpDir)

	// CreateAgent starts the agent in "paused" state; promote to "idle".
	ag, err := l.agentManager.CreateAgent(t.Context(), "Idle Agent", "test-persona", "proj-1", "", nil)
	if err != nil {
		t.Fatalf("failed to create agent: %v", err)
	}
	_ = l.agentManager.UpdateAgentStatus(ag.ID, "idle")

	if err := l.WakeAgent(ag.ID, nil); err != nil {
		t.Errorf("unexpected error waking idle agent: %v", err)
	}

	refreshed, _ := l.agentManager.GetAgent(ag.ID)
	if refreshed.Status != "idle" {
		t.Errorf("idle agent status should remain idle, got %s", refreshed.Status)
	}
}

func TestWakeAgent_PausedBecomesIdle(t *testing.T) {
	l, tmpDir := testLoom(t)
	defer os.RemoveAll(tmpDir)

	// CreateAgent starts the agent in "paused" state already.
	ag, err := l.agentManager.CreateAgent(t.Context(), "Paused Agent", "test-persona", "proj-1", "", nil)
	if err != nil {
		t.Fatalf("failed to create agent: %v", err)
	}

	var m *motivation.Motivation
	if err := l.WakeAgent(ag.ID, m); err != nil {
		t.Errorf("unexpected error waking paused agent: %v", err)
	}

	refreshed, _ := l.agentManager.GetAgent(ag.ID)
	if refreshed.Status != "idle" {
		t.Errorf("expected idle after wake, got %s", refreshed.Status)
	}
}

// ---------------------------------------------------------------------------
// Issue #36: StartWorkflow
// ---------------------------------------------------------------------------

func TestStartWorkflow_NoEngine(t *testing.T) {
	l, tmpDir := testLoom(t)
	defer os.RemoveAll(tmpDir)

	// Remove the workflow engine to test the guard.
	saved := l.workflowEngine
	l.workflowEngine = nil
	defer func() { l.workflowEngine = saved }()

	_, err := l.StartWorkflow("bug", nil)
	if err == nil {
		t.Error("expected error when workflow engine is unavailable")
	}
}

func TestStartWorkflow_EmptyType(t *testing.T) {
	l, tmpDir := testLoom(t)
	defer os.RemoveAll(tmpDir)

	if l.workflowEngine == nil {
		t.Skip("workflow engine not available")
	}

	_, err := l.StartWorkflow("", nil)
	if err == nil {
		t.Error("expected error for empty workflow type")
	}
}

func TestStartWorkflow_UnknownType(t *testing.T) {
	l, tmpDir := testLoom(t)
	defer os.RemoveAll(tmpDir)

	if l.workflowEngine == nil {
		t.Skip("workflow engine not available")
	}
	requireDatabase(t, l)

	_, err := l.StartWorkflow("nonexistent-workflow-type-xyz", nil)
	if err == nil {
		t.Error("expected error for unknown workflow type")
	}
}

func TestStartWorkflow_InvalidProject(t *testing.T) {
	l, tmpDir := testLoom(t)
	defer os.RemoveAll(tmpDir)

	if l.workflowEngine == nil {
		t.Skip("workflow engine not available")
	}

	_, err := l.StartWorkflow("bug", map[string]interface{}{
		"project_id": "nonexistent-project",
	})
	if err == nil {
		t.Error("expected error for nonexistent project")
	}
}
