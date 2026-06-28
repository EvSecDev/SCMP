package drn

import (
	"fmt"
	"scmp/internal/str"
	"strings"
)

func (drc *DRC) SerializeNamespace() (namespace string) {
	namespace = strings.Join(drc.Namespace, NamespaceSeparator)
	return
}

// Converts normalized fields into DRN string and revalidates
func (drc *DRC) SerializeExpanded() (err error) {
	var builder strings.Builder
	builder.WriteString(OpenDelimiter)
	builder.WriteString(Prefix)
	builder.WriteString(strings.Join(drc.Namespace, NamespaceSeparator))
	builder.WriteString(PrimarySeparator)
	builder.WriteString(strings.Join(drc.Fields, FieldSeparator))
	builder.WriteString(CloseDelimiter)
	_, err = Validate(builder.String())
	if err != nil {
		err = fmt.Errorf("invalid built DRN '%s': %w", builder.String(), err)
		return
	}
	drc.Expanded = str.DRN(builder.String())
	return
}

// Checks if the normalized namespace (post-expansion) contains the sentinel prefix for an internal DRN
func (drc *DRC) IsInternalDRN() (isInternal bool) {
	if len(drc.Namespace) == 0 {
		return
	}
	if drc.Namespace[0] != InternalNamespacePrefix {
		return
	}
	isInternal = true
	return
}

// Checks if any parents of the current DRN config have the same post-macro-expansion DRN string
func (drc *DRC) HasCycle() (selfReference bool) {
	for parent := drc.Parent; parent != nil; parent = parent.Parent {
		if parent.Expanded == drc.Expanded {
			selfReference = true
			return
		}
	}
	return
}
