// Package for handling Dynamic Reference Names and their resolution and replacement pre-deployment
package drn

import (
	"scmp/internal/str"
	"sync"
)

// Parsed Dynamic Reference Config in host+file context
type DRC struct {
	Original str.DRNRaw // Original text found in source
	Expanded str.DRN    // Expanded macros, string is contextualized to host+file

	// contextual derivation
	Parent *DRC
	Depth  int // Depth of current DRN config

	// Parsed fields
	Namespace []string
	Fields    []string

	// Final value (contains no macros or DRN)
	Resolved str.DRNVal
}

type macroRegistry struct {
	byName   map[string]macroDef
	byDRN    map[str.DRN]macroDef
	host     map[string]struct{}
	file     map[string]struct{}
	allNames []string
	once     sync.Once
}

type ContextField uint8

type macroDef struct {
	name         string
	drn          string
	category     string
	contextField ContextField // which value to resolve to
}
