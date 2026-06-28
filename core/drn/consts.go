package drn

import (
	"errors"
)

const (
	// Structure
	OpenDelimiter     string = "<"
	Prefix             string = "scmp://" // Case sensitive
	NamespaceSeparator string = "/"       // Path-like namespace
	PrimarySeparator   string = "@"       // Separates namespace from dotted fields
	FieldSeparator     string = "."       // Separates field names
	CloseDelimiter       string = ">"

	// Validation
	MinTotalLength    int = len(OpenDelimiter) + len(Prefix) + 1 + len(PrimarySeparator) + 1 + len(CloseDelimiter) // Example of min len DRN: <scmp://a@e>
	MaxTotalLength    int = 255                                                                                   // Includes prefix and separators
	minNameSpaceDepth int = 1
	maxNameSpaceDepth int = 10
	minFieldDepth     int = 1
	maxFieldDepth     int = 10

	MaxNesting int = 20 // Maximum drn's within drn's

	// Special Markers
	InternalNamespacePrefix   string = "_local"  // Namespace prefix that will be treated as internal-lookup-only, i.e. <scmp://_local@repo.file.name>
	ExternalVariableDirectory string = "_global" // From root of repository
	InternalMacroOpen         string = "{{"
	InternalMacroClose        string = "}}"
)

// Sentinel Errors
var (
	ErrNotDRN = errors.New("string is not a DRN string")
)
