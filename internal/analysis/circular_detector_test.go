package analysis

import (
	"testing"
)

func TestNewCircularDetector(t *testing.T) {
	detector := NewCircularDetector()
	if detector == nil {
		t.Fatal("NewCircularDetector returned nil")
	}
	if len(detector.importGraph) != 0 {
		t.Errorf("expected empty import graph, got %d entries", len(detector.importGraph))
	}
	if len(detector.callGraph) != 0 {
		t.Errorf("expected empty call graph, got %d entries", len(detector.callGraph))
	}
}

func TestDetectImportCycles(t *testing.T) {
	detector := NewCircularDetector()

	// Manually add a simple cycle: a -> b -> a
	detector.importGraph["a"] = []string{"b"}
	detector.importGraph["b"] = []string{"a"}

	cycles := detector.DetectImportCycles()
	if len(cycles) == 0 {
		t.Error("expected to detect import cycle, but found none")
	}

	for _, cycle := range cycles {
		if cycle.Type != "import" {
			t.Errorf("expected cycle type 'import', got '%s'", cycle.Type)
		}
	}
}

func TestDetectFunctionCycles(t *testing.T) {
	detector := NewCircularDetector()

	// Manually add a simple cycle: pkg.funcA -> pkg.funcB -> pkg.funcA
	detector.callGraph["pkg.funcA"] = []string{"pkg.funcB"}
	detector.callGraph["pkg.funcB"] = []string{"pkg.funcA"}

	cycles := detector.DetectFunctionCycles()
	if len(cycles) == 0 {
		t.Error("expected to detect function cycle, but found none")
	}

	for _, cycle := range cycles {
		if cycle.Type != "function" {
			t.Errorf("expected cycle type 'function', got '%s'", cycle.Type)
		}
	}
}

func TestNoCycles(t *testing.T) {
	detector := NewCircularDetector()

	// Add a linear dependency: a -> b -> c (no cycle)
	detector.importGraph["a"] = []string{"b"}
	detector.importGraph["b"] = []string{"c"}

	cycles := detector.DetectImportCycles()
	if len(cycles) != 0 {
		t.Errorf("expected no cycles, but found %d", len(cycles))
	}
}

func TestPrintReport(t *testing.T) {
	detector := NewCircularDetector()

	// Add a cycle
	detector.importGraph["a"] = []string{"b"}
	detector.importGraph["b"] = []string{"a"}

	detector.DetectImportCycles()
	report := detector.PrintReport()

	if len(report) == 0 {
		t.Error("PrintReport returned empty string")
	}

	if len(detector.cycles) > 0 && !contains(report, "Cycle") {
		t.Error("report should contain 'Cycle' when cycles are detected")
	}
}

func TestGetSortedCycles(t *testing.T) {
	detector := NewCircularDetector()

	// Add multiple cycles
	detector.importGraph["a"] = []string{"b"}
	detector.importGraph["b"] = []string{"a"}
	detector.callGraph["pkg.f1"] = []string{"pkg.f2"}
	detector.callGraph["pkg.f2"] = []string{"pkg.f1"}

	detector.DetectImportCycles()
	detector.DetectFunctionCycles()

	sorted := detector.GetSortedCycles()
	if len(sorted) == 0 {
		t.Error("expected sorted cycles, got none")
	}
}

func contains(s, substr string) bool {
	for i := 0; i < len(s)-len(substr)+1; i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
