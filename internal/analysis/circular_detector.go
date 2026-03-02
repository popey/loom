package analysis

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// CircularDependency represents a detected circular dependency
type CircularDependency struct {
	Type  string   // "import" or "function"
	Cycle []string // The cycle path
}

// CircularDetector detects circular dependencies in Go code
type CircularDetector struct {
	packages    map[string]*PackageInfo
	functions   map[string]*FunctionInfo
	importGraph map[string][]string
	callGraph   map[string][]string
	cycles      []CircularDependency
	visited     map[string]bool
	recStack    map[string]bool
	path        []string
}

// PackageInfo holds information about a package
type PackageInfo struct {
	Name    string
	Path    string
	Imports []string
}

// FunctionInfo holds information about a function
type FunctionInfo struct {
	Name    string
	Package string
	Calls   []string // Fully qualified function names
}

// NewCircularDetector creates a new detector
func NewCircularDetector() *CircularDetector {
	return &CircularDetector{
		packages:    make(map[string]*PackageInfo),
		functions:   make(map[string]*FunctionInfo),
		importGraph: make(map[string][]string),
		callGraph:   make(map[string][]string),
		cycles:      []CircularDependency{},
		visited:     make(map[string]bool),
		recStack:    make(map[string]bool),
		path:        []string{},
	}
}

// AnalyzeDirectory analyzes all Go files in a directory
func (cd *CircularDetector) AnalyzeDirectory(dir string) error {
	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip non-Go files and test files
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		// Skip vendor and hidden directories
		if strings.Contains(path, "/vendor/") || strings.Contains(path, "/.git/") {
			return nil
		}

		return cd.analyzeFile(path)
	})
}

// analyzeFile parses a single Go file and extracts imports and function calls
func (cd *CircularDetector) analyzeFile(filePath string) error {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filePath, nil, parser.AllErrors)
	if err != nil {
		return fmt.Errorf("failed to parse %s: %w", filePath, err)
	}

	pkgName := file.Name.Name

	// Extract imports
	for _, imp := range file.Imports {
		impPath := strings.Trim(imp.Path.Value, `"`)
		cd.importGraph[pkgName] = append(cd.importGraph[pkgName], impPath)
	}

	// Extract function calls using AST visitor
	visitor := &callVisitor{
		detector:    cd,
		currentPkg:  pkgName,
		currentFile: filePath,
	}
	ast.Walk(visitor, file)

	return nil
}

// callVisitor walks the AST to find function calls
type callVisitor struct {
	detector    *CircularDetector
	currentPkg  string
	currentFile string
	currentFunc string
}

func (v *callVisitor) Visit(node ast.Node) ast.Visitor {
	if node == nil {
		return v
	}

	// Track current function
	switch n := node.(type) {
	case *ast.FuncDecl:
		v.currentFunc = n.Name.Name
	case *ast.CallExpr:
		// Extract called function name
		if ident, ok := n.Fun.(*ast.Ident); ok {
			calledFunc := ident.Name
			if v.currentFunc != "" {
				fullName := fmt.Sprintf("%s.%s", v.currentPkg, v.currentFunc)
				calledFullName := fmt.Sprintf("%s.%s", v.currentPkg, calledFunc)
				v.detector.callGraph[fullName] = append(v.detector.callGraph[fullName], calledFullName)
			}
		}
	}

	return v
}

// DetectImportCycles detects circular import dependencies
func (cd *CircularDetector) DetectImportCycles() []CircularDependency {
	cd.visited = make(map[string]bool)
	cd.recStack = make(map[string]bool)
	cd.path = []string{}

	for pkg := range cd.importGraph {
		if !cd.visited[pkg] {
			cd.dfsImport(pkg)
		}
	}

	return cd.cycles
}

// dfsImport performs DFS to detect import cycles
func (cd *CircularDetector) dfsImport(pkg string) {
	cd.visited[pkg] = true
	cd.recStack[pkg] = true
	cd.path = append(cd.path, pkg)

	for _, neighbor := range cd.importGraph[pkg] {
		if !cd.visited[neighbor] {
			cd.dfsImport(neighbor)
		} else if cd.recStack[neighbor] {
			// Found a cycle
			cycle := cd.extractCycle(neighbor)
			cd.cycles = append(cd.cycles, CircularDependency{
				Type:  "import",
				Cycle: cycle,
			})
		}
	}

	cd.path = cd.path[:len(cd.path)-1]
	cd.recStack[pkg] = false
}

// DetectFunctionCycles detects circular function call dependencies
func (cd *CircularDetector) DetectFunctionCycles() []CircularDependency {
	cd.visited = make(map[string]bool)
	cd.recStack = make(map[string]bool)
	cd.path = []string{}

	for fn := range cd.callGraph {
		if !cd.visited[fn] {
			cd.dfsFunction(fn)
		}
	}

	return cd.cycles
}

// dfsFunction performs DFS to detect function call cycles
func (cd *CircularDetector) dfsFunction(fn string) {
	cd.visited[fn] = true
	cd.recStack[fn] = true
	cd.path = append(cd.path, fn)

	for _, neighbor := range cd.callGraph[fn] {
		if !cd.visited[neighbor] {
			cd.dfsFunction(neighbor)
		} else if cd.recStack[neighbor] {
			// Found a cycle
			cycle := cd.extractCycle(neighbor)
			cd.cycles = append(cd.cycles, CircularDependency{
				Type:  "function",
				Cycle: cycle,
			})
		}
	}

	cd.path = cd.path[:len(cd.path)-1]
	cd.recStack[fn] = false
}

// extractCycle extracts the cycle from the current path
func (cd *CircularDetector) extractCycle(start string) []string {
	var cycle []string
	found := false

	for _, node := range cd.path {
		if node == start {
			found = true
		}
		if found {
			cycle = append(cycle, node)
		}
	}

	if found {
		cycle = append(cycle, start) // Close the cycle
	}

	return cycle
}

// GetCycles returns all detected cycles
func (cd *CircularDetector) GetCycles() []CircularDependency {
	return cd.cycles
}

// GetImportGraph returns the import dependency graph
func (cd *CircularDetector) GetImportGraph() map[string][]string {
	return cd.importGraph
}

// GetCallGraph returns the function call graph
func (cd *CircularDetector) GetCallGraph() map[string][]string {
	return cd.callGraph
}

// PrintReport prints a human-readable report of detected cycles
func (cd *CircularDetector) PrintReport() string {
	var report strings.Builder

	report.WriteString("=== Circular Dependency Detection Report ===\n\n")

	if len(cd.cycles) == 0 {
		report.WriteString("✓ No circular dependencies detected.\n")
		return report.String()
	}

	// Group cycles by type
	importCycles := []CircularDependency{}
	functionCycles := []CircularDependency{}

	for _, cycle := range cd.cycles {
		if cycle.Type == "import" {
			importCycles = append(importCycles, cycle)
		} else {
			functionCycles = append(functionCycles, cycle)
		}
	}

	// Print import cycles
	if len(importCycles) > 0 {
		report.WriteString(fmt.Sprintf("Found %d import cycle(s):\n\n", len(importCycles)))
		for i, cycle := range importCycles {
			report.WriteString(fmt.Sprintf("  Cycle %d: %s\n", i+1, strings.Join(cycle.Cycle, " → ")))
		}
		report.WriteString("\n")
	}

	// Print function cycles
	if len(functionCycles) > 0 {
		report.WriteString(fmt.Sprintf("Found %d function call cycle(s):\n\n", len(functionCycles)))
		for i, cycle := range functionCycles {
			report.WriteString(fmt.Sprintf("  Cycle %d: %s\n", i+1, strings.Join(cycle.Cycle, " → ")))
		}
		report.WriteString("\n")
	}

	return report.String()
}

// GetSortedCycles returns cycles sorted by type and length
func (cd *CircularDetector) GetSortedCycles() []CircularDependency {
	sorted := make([]CircularDependency, len(cd.cycles))
	copy(sorted, cd.cycles)

	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Type != sorted[j].Type {
			return sorted[i].Type < sorted[j].Type
		}
		return len(sorted[i].Cycle) < len(sorted[j].Cycle)
	})

	return sorted
}
