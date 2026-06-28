package drn

import (
	"scmp/internal/str"
	"scmp/internal/tests/utils"
	"testing"
)

func TestHasCycle(t *testing.T) {
	tests := []struct {
		name     string
		setup    func() *DRC
		expected bool
	}{
		{
			name: "No parent - no cycle",
			setup: func() *DRC {
				return &DRC{Expanded: str.DRN(QuickFormat([]string{"repo"}, "field"))}
			},
			expected: false,
		},
		{
			name: "Parent with different expanded - no cycle",
			setup: func() *DRC {
				parent := &DRC{Expanded: str.DRN(QuickFormat([]string{"repo"}, "other"))}
				return &DRC{Expanded: str.DRN(QuickFormat([]string{"repo"}, "field")), Parent: parent}
			},
			expected: false,
		},
		{
			name: "Parent with same expanded - cycle detected",
			setup: func() *DRC {
				parent := &DRC{Expanded: str.DRN(QuickFormat([]string{"repo"}, "field"))}
				return &DRC{Expanded: str.DRN(QuickFormat([]string{"repo"}, "field")), Parent: parent}
			},
			expected: true,
		},
		{
			name: "Grandparent with same expanded - cycle detected",
			setup: func() *DRC {
				grandparent := &DRC{Expanded: str.DRN(QuickFormat([]string{"repo"}, "field"))}
				parent := &DRC{Expanded: str.DRN(QuickFormat([]string{"repo"}, "intermediate")), Parent: grandparent}
				return &DRC{Expanded: str.DRN(QuickFormat([]string{"repo"}, "field")), Parent: parent}
			},
			expected: true,
		},
		{
			name: "Deep chain no cycle",
			setup: func() *DRC {
				a := &DRC{Expanded: str.DRN(QuickFormat([]string{"a"}, "f"))}
				b := &DRC{Expanded: str.DRN(QuickFormat([]string{"b"}, "f")), Parent: a}
				c := &DRC{Expanded: str.DRN(QuickFormat([]string{"c"}, "f")), Parent: b}
				d := &DRC{Expanded: str.DRN(QuickFormat([]string{"d"}, "f")), Parent: c}
				return d
			},
			expected: false,
		},
		{
			name: "Deep chain cycle at root",
			setup: func() *DRC {
				root := &DRC{Expanded: str.DRN(QuickFormat([]string{"root"}, "f"))}
				b := &DRC{Expanded: str.DRN(QuickFormat([]string{"b"}, "f")), Parent: root}
				c := &DRC{Expanded: str.DRN(QuickFormat([]string{"c"}, "f")), Parent: b}
				current := &DRC{Expanded: str.DRN(QuickFormat([]string{"root"}, "f")), Parent: c}
				return current
			},
			expected: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			current := test.setup()
			got := current.HasCycle()
			if got != test.expected {
				t.Errorf("hasCycle() = %v, expected %v", got, test.expected)
			}
		})
	}
}

func TestSerializeExpanded(t *testing.T) {
	tests := []struct {
		name        string
		drc         DRC
		expectError string
		expectValid bool
	}{
		{
			name: "Simple single field",
			drc: DRC{
				Namespace: []string{"repo"},
				Fields:    []string{"field"},
			},
			expectValid: true,
		},
		{
			name: "Deep fields",
			drc: DRC{
				Namespace: []string{"repo"},
				Fields:    []string{"a", "b", "c"},
			},
			expectValid: true,
		},
		{
			name: "Nested namespace",
			drc: DRC{
				Namespace: []string{"a", "b", "c"},
				Fields:    []string{"field"},
			},
			expectValid: true,
		},
		{
			name: "Namespace with dots (from macro expansion)",
			drc: DRC{
				Namespace: []string{"file.conf"},
				Fields:    []string{"field"},
			},
			expectValid: true,
		},
		{
			name: "Namespace too deep fails",
			drc: DRC{
				Namespace: []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k"},
				Fields:    []string{"field"},
			},
			expectError: "exceeds maximum component count",
		},
		{
			name: "Field too deep fails",
			drc: DRC{
				Namespace: []string{"repo"},
				Fields:    []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k"},
			},
			expectError: "exceeds maximum field count",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.drc.SerializeExpanded()
			matches, err := utils.MatchErrorString(err, test.expectError)
			if err != nil {
				t.Fatalf("%v", err)
			} else if matches {
				return
			}

			// Reconstruct expected string
			expected := QuickFormat(test.drc.Namespace, test.drc.Fields...)
			if expected != string(test.drc.Expanded) {
				t.Errorf("expanded = %q, expected %q", test.drc.Expanded, expected)
			}
		})
	}
}

func TestIsInternalDRN(t *testing.T) {
	tests := []struct {
		name     string
		drc      DRC
		expected bool
	}{
		{
			name:     "Empty namespace returns false",
			drc:      DRC{Namespace: []string{}},
			expected: false,
		},
		{
			name:     "Nil namespace returns false",
			drc:      DRC{Namespace: nil},
			expected: false,
		},
		{
			name:     "Internal prefix _local returns true",
			drc:      DRC{Namespace: []string{InternalNamespacePrefix}},
			expected: true,
		},
		{
			name: "Internal prefix with deep fields returns true",
			drc: DRC{
				Namespace: []string{InternalNamespacePrefix},
				Fields:    []string{"repo", "file", "name"},
			},
			expected: true,
		},
		{
			name:     "External namespace returns false",
			drc:      DRC{Namespace: []string{"repo"}},
			expected: false,
		},
		{
			name:     "External namespace with subdirs returns false",
			drc:      DRC{Namespace: []string{"repo", "config", "main"}},
			expected: false,
		},
		{
			name:     "Namespace starts with non-internal prefix returns false",
			drc:      DRC{Namespace: []string{ExternalVariableDirectory}},
			expected: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := test.drc.IsInternalDRN()
			if got != test.expected {
				t.Errorf("test %v = %v, expected %v", test.drc, got, test.expected)
			}
		})
	}
}
