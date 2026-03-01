package main

import (
	"fmt"
	"io/ioutil"
	"strings"
)

func main() {
	content, err := ioutil.ReadFile("internal/actions/router.go")
	if err != nil {
		fmt.Printf("Error reading file: %v\n", err)
		return
	}

	text := string(content)

	// Add persona import after files import
	text = strings.Replace(text,
		`"github.com/jordanhubbard/loom/internal/files"`,
		`"github.com/jordanhubbard/loom/internal/files"
	"github.com/jordanhubbard/loom/internal/persona"`,
		1)

	// Add PersonaManager field after Voter field
	text = strings.Replace(text,
		`	Voter         VoteCaster
	BeadType      string`,
		`	Voter         VoteCaster
	PersonaManager *persona.Manager
	BeadType      string`,
		1)

	err = ioutil.WriteFile("internal/actions/router.go", []byte(text), 0644)
	if err != nil {
		fmt.Printf("Error writing file: %v\n", err)
		return
	}

	fmt.Println("Successfully updated router.go")
}
