package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/jordanhubbard/loom/internal/analysis"
)

func main() {
	var (
		dir       = flag.String("dir", ".", "Directory to analyze")
		imports   = flag.Bool("imports", true, "Detect import cycles")
		functions = flag.Bool("functions", true, "Detect function call cycles")
		json      = flag.Bool("json", false, "Output as JSON")
		verbose   = flag.Bool("v", false, "Verbose output")
	)
	flag.Parse()

	if *dir == "" {
		fmt.Fprintf(os.Stderr, "Error: directory cannot be empty\n")
		os.Exit(1)
	}

	// Create detector
	detector := analysis.NewCircularDetector()

	// Analyze directory
	if *verbose {
		fmt.Printf("Analyzing directory: %s\n", *dir)
	}

	err := detector.AnalyzeDirectory(*dir)
	if err != nil {
		log.Fatalf("Failed to analyze directory: %v", err)
	}

	// Detect cycles
	if *imports {
		if *verbose {
			fmt.Println("Detecting import cycles...")
		}
		detector.DetectImportCycles()
	}

	if *functions {
		if *verbose {
			fmt.Println("Detecting function call cycles...")
		}
		detector.DetectFunctionCycles()
	}

	// Output results
	if *json {
		outputJSON(detector)
	} else {
		fmt.Print(detector.PrintReport())
	}

	// Exit with error code if cycles found
	if len(detector.GetCycles()) > 0 {
		os.Exit(1)
	}
}

func outputJSON(detector *analysis.CircularDetector) {
	cycles := detector.GetSortedCycles()
	fmt.Printf("{ \"cycles\": [\n")
	for i, cycle := range cycles {
		fmt.Printf("  { \"type\": \"%s\", \"path\": [", cycle.Type)
		for j, node := range cycle.Cycle {
			if j > 0 {
				fmt.Printf(", ")
			}
			fmt.Printf("\"%s\"", node)
		}
		fmt.Printf("] }")
		if i < len(cycles)-1 {
			fmt.Printf(",")
		}
		fmt.Printf("\n")
	}
	fmt.Printf("] }\n")
}
