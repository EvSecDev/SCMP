package drn

import (
	"fmt"
	"scmp/internal/str"
	"strings"
)

// Checks if string input is a valid dynamic reference name returns clean type
// Returns ErrNotDRN if the input does not meet the basic criteria for DRN strings
// Returns other errors if it looks like a DRN but is malformed or violates requirements
func Validate(input string) (drnConfig DRC, err error) {
	if !strings.HasPrefix(input, OpenDelimiter) {
		err = fmt.Errorf("%w: missing open delimiter '%s'", ErrNotDRN, OpenDelimiter)
		return
	}
	if !strings.HasPrefix(input, OpenDelimiter+Prefix) {
		err = fmt.Errorf("%w: missing prefix '%s'", ErrNotDRN, Prefix)
		return
	}
	if !strings.HasSuffix(input, CloseDelimiter) {
		err = fmt.Errorf("%w: missing close delimiter '%s'", ErrNotDRN, CloseDelimiter)
		return
	}

	// Validate size bounds
	if len(input) < MinTotalLength {
		err = fmt.Errorf("%d characters is below the minimum length of %d",
			len(input), MinTotalLength)
		return
	}
	if len(input) > MaxTotalLength {
		err = fmt.Errorf("%d characters exceeds maximum length of %d",
			len(input), MaxTotalLength)
		return
	}

	// Spaces are not permitted anywhere
	if strings.Contains(input, " ") {
		err = fmt.Errorf("cannot contain spaces")
		return
	}

	// Validate macros by ensuring correct open/close braces
	insideMacro := false
	macroContentLen := 0
	for i := 0; i < len(input); i++ {
		switch input[i] {
		case InternalMacroOpen[0]:
			if i+1 >= len(input) || input[i+1] != InternalMacroOpen[0] {
				err = fmt.Errorf("single opening brace is not permitted; use '%s'", InternalMacroOpen)
				return
			}
			if insideMacro {
				err = fmt.Errorf("nested macros are not permitted")
				return
			}
			insideMacro = true
			i++ // consume second open
		case InternalMacroClose[0]:
			if i+1 >= len(input) || input[i+1] != InternalMacroClose[0] {
				err = fmt.Errorf("single closing brace is not permitted; use '%s'", InternalMacroClose)
				return
			}
			if !insideMacro {
				err = fmt.Errorf("unmatched macro close")
				return
			}
			if macroContentLen == 0 {
				err = fmt.Errorf("empty macros are not permitted")
				return
			}
			insideMacro = false
			macroContentLen = 0
			i++ // consume second close
		default:
			if insideMacro {
				macroContentLen++
			}
		}
	}
	if insideMacro {
		err = fmt.Errorf("unclosed macro")
		return
	}

	// Separate into segments
	trimmed := input[len(OpenDelimiter)+len(Prefix):]
	sepIndex := strings.IndexByte(trimmed, byte(PrimarySeparator[0]))
	if sepIndex < 0 {
		err = fmt.Errorf("missing primary separator character '%s'", PrimarySeparator)
		return
	}
	if strings.IndexByte(trimmed[sepIndex+1:], byte(PrimarySeparator[0])) >= 0 {
		err = fmt.Errorf("too many primary separator characters '%s'", PrimarySeparator)
		return
	}

	namespace := trimmed[:sepIndex]
	dottedFields := trimmed[sepIndex+1:]
	dottedFields = strings.TrimSuffix(dottedFields, CloseDelimiter)

	// Enforce namespace requirements
	nsComponents := strings.Split(namespace, NamespaceSeparator)
	if len(nsComponents) < minNameSpaceDepth {
		err = fmt.Errorf("%d namespace components is below minimum component count of %d",
			len(nsComponents), minNameSpaceDepth)
		return
	}
	if len(nsComponents) > maxNameSpaceDepth {
		err = fmt.Errorf("%d namespace components exceeds maximum component count of %d",
			len(nsComponents), maxNameSpaceDepth)
		return
	}

	for index, nsComponent := range nsComponents {
		// Only an error if extra separator is present
		if nsComponent == "" && strings.Contains(namespace, NamespaceSeparator) {
			err = fmt.Errorf("namespace segment %d is empty", index+1)
			return
		}

		if index == 0 && nsComponent == InternalNamespacePrefix {
			// Internal prefix bypasses validation when first segment
			continue
		}

		if !isValidNamespaceCharacters(nsComponent) {
			err = fmt.Errorf("namespace segment %d ('%s') contains unsupported characters: must only contain [a-zA-Z0-9_-{}]",
				index+1, nsComponent)
			return
		}
	}

	// Enforce field requirements
	fields := strings.Split(dottedFields, FieldSeparator)
	if len(fields) < minFieldDepth {
		err = fmt.Errorf("%d field names is below minimum field count of %d",
			len(fields), minFieldDepth)
		return
	}
	if len(fields) > maxFieldDepth {
		err = fmt.Errorf("%d field names exceeds maximum field count of %d",
			len(fields), maxFieldDepth)
		return
	}

	for index, field := range fields {
		if field == "" && strings.Contains(dottedFields, FieldSeparator) {
			// Only an error if extra separator is present
			err = fmt.Errorf("field %d is empty", index+1)
			return
		}

		if !isValidFieldCharacters(field) {
			err = fmt.Errorf("field %d ('%s') contains unsupported characters: must only contain [a-zA-Z0-9_-{}]",
				index+1, field)
			return
		}
	}

	drnConfig.Original = str.DRNRaw(input)
	drnConfig.Namespace = nsComponents
	drnConfig.Fields = fields
	return
}

// Ensures fields only contains the allowed characters
func isValidFieldCharacters(input string) (valid bool) {
	for i := 0; i < len(input); i++ {
		char := input[i]
		if (char < 'a' || char > 'z') &&
			(char < 'A' || char > 'Z') &&
			(char < '0' || char > '9') &&
			char != '_' && char != '-' &&
			char != '}' && char != '{' {
			return
		}
	}
	valid = true
	return
}

// Ensures namespace only contains the allowed characters
func isValidNamespaceCharacters(input string) (valid bool) {
	for i := 0; i < len(input); i++ {
		char := input[i]
		if (char < 'a' || char > 'z') &&
			(char < 'A' || char > 'Z') &&
			(char < '0' || char > '9') &&
			char != '_' && char != '-' && char != '.' &&
			char != '}' && char != '{' {
			return
		}
	}
	valid = true
	return
}
