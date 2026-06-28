package drn

import (
	"scmp/internal/str"
	"slices"
	"testing"
)

func TestPathToNamespace(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "Two namespace segments",
			input:    "_global/host/host1",
			expected: []string{"host", "host1"},
		},
		{
			name:     "Single namespace segments",
			input:    "_global/host1",
			expected: []string{"host1"},
		},
		{
			name:     "Namespace segment containing top level duplicate name",
			input:    "_global/hosts/_global/host1",
			expected: []string{"hosts", "_global", "host1"},
		},
		{
			name:     "Missing top directory",
			input:    "host1",
			expected: []string{},
		},
		{
			name:     "Only top directory",
			input:    "_global",
			expected: []string{},
		},
		{
			name:     "Only top directory trailing path",
			input:    "_global/",
			expected: []string{""},
		},
		{
			name:     "Only path separator",
			input:    "/",
			expected: []string{},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := PathToNamespace(test.input)
			if !slices.Equal(test.expected, got) {
				t.Errorf("got %v, expected %v", got, test.expected)
			}
		})
	}
}

func TestNamespaceToPath(t *testing.T) {
	tests := []struct {
		name      string
		repoRoot  string
		namespace []string
		expected  string
	}{
		{
			name:      "Basic single repo root",
			repoRoot:  "/repo1",
			namespace: []string{"hosts", "host1"},
			expected:  "/repo1/_global/hosts/host1",
		},
		{
			name:      "Basic single repo single namespace",
			repoRoot:  "/repo1",
			namespace: []string{"host1"},
			expected:  "/repo1/_global/host1",
		},
		{
			name:      "Root fs repo",
			repoRoot:  "/",
			namespace: []string{"hosts", "host1"},
			expected:  "/_global/hosts/host1",
		},
		{
			name:      "Missing namespace (empty)",
			repoRoot:  "/repo1",
			namespace: []string{""},
			expected:  "/repo1/_global",
		},
		{
			name:      "Missing namespace (len0)",
			repoRoot:  "/repo1",
			namespace: []string{},
			expected:  "/repo1/_global",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := NamespaceToPath(test.repoRoot, test.namespace)
			if test.expected != got {
				t.Errorf("got '%s', expected '%s'", got, test.expected)
			}
		})
	}
}

func TestDiffSet(t *testing.T) {
	testCases := []struct {
		name   string
		before map[str.DRN]str.DRNVal
		after  map[str.DRN]str.DRNVal
		expect []str.DRN
	}{
		{
			name: "Value changed",
			before: map[str.DRN]str.DRNVal{
				str.DRN(QuickFormat([]string{"a"}, "b")): "val1",
				str.DRN(QuickFormat([]string{"c"}, "d")): "val2",
			},
			after: map[str.DRN]str.DRNVal{
				str.DRN(QuickFormat([]string{"a"}, "b")): "val1",
				str.DRN(QuickFormat([]string{"c"}, "d")): "val4",
			},
			expect: []str.DRN{str.DRN(QuickFormat([]string{"c"}, "d"))},
		},
		{
			name: "Key removed",
			before: map[str.DRN]str.DRNVal{
				str.DRN(QuickFormat([]string{"a"}, "b")): "val1",
				str.DRN(QuickFormat([]string{"c"}, "d")): "val2",
			},
			after: map[str.DRN]str.DRNVal{
				str.DRN(QuickFormat([]string{"a"}, "b")): "val1",
			},
			expect: []str.DRN{str.DRN(QuickFormat([]string{"c"}, "d"))},
		},
		{
			name: "Key added",
			before: map[str.DRN]str.DRNVal{
				str.DRN(QuickFormat([]string{"a"}, "b")): "val1",
			},
			after: map[str.DRN]str.DRNVal{
				str.DRN(QuickFormat([]string{"a"}, "b")): "val1",
				str.DRN(QuickFormat([]string{"c"}, "d")): "val2",
			},
			expect: []str.DRN{str.DRN(QuickFormat([]string{"c"}, "d"))},
		},
		{
			name:   "Both empty maps",
			before: map[str.DRN]str.DRNVal{},
			after:  map[str.DRN]str.DRNVal{},
			expect: []str.DRN{},
		},
		{
			name:   "Before empty, after has entries",
			before: map[str.DRN]str.DRNVal{},
			after: map[str.DRN]str.DRNVal{
				str.DRN(QuickFormat([]string{"a"}, "b")): "val1",
				str.DRN(QuickFormat([]string{"c"}, "d")): "val2",
			},
			expect: []str.DRN{str.DRN(QuickFormat([]string{"a"}, "b")),
				str.DRN(QuickFormat([]string{"c"}, "d"))},
		},
		{
			name: "After empty, before has entries",
			before: map[str.DRN]str.DRNVal{
				str.DRN(QuickFormat([]string{"a"}, "b")): "val1",
				str.DRN(QuickFormat([]string{"c"}, "d")): "val2",
			},
			after: map[str.DRN]str.DRNVal{},
			expect: []str.DRN{str.DRN(QuickFormat([]string{"a"}, "b")),
				str.DRN(QuickFormat([]string{"c"}, "d"))},
		},
		{
			name: "No changes",
			before: map[str.DRN]str.DRNVal{
				str.DRN(QuickFormat([]string{"a"}, "b")): "val1",
				str.DRN(QuickFormat([]string{"c"}, "d")): "val2",
			},
			after: map[str.DRN]str.DRNVal{
				str.DRN(QuickFormat([]string{"a"}, "b")): "val1",
				str.DRN(QuickFormat([]string{"c"}, "d")): "val2",
			},
			expect: []str.DRN{},
		},
		{
			name: "Multiple changes of different types",
			before: map[str.DRN]str.DRNVal{
				str.DRN(QuickFormat([]string{"a"}, "b")): "val1",
				str.DRN(QuickFormat([]string{"c"}, "d")): "val2",
				str.DRN(QuickFormat([]string{"e"}, "f")): "val3",
				str.DRN(QuickFormat([]string{"g"}, "h")): "val4",
			},
			after: map[str.DRN]str.DRNVal{
				str.DRN(QuickFormat([]string{"a"}, "b")): "val1",
				str.DRN(QuickFormat([]string{"c"}, "d")): "val20",
				str.DRN(QuickFormat([]string{"e"}, "f")): "val3",
				str.DRN(QuickFormat([]string{"i"}, "j")): "val5",
			},
			expect: []str.DRN{
				str.DRN(QuickFormat([]string{"c"}, "d")),
				str.DRN(QuickFormat([]string{"g"}, "h")),
				str.DRN(QuickFormat([]string{"i"}, "j")),
			},
		},
		{
			name: "Single entry changed",
			before: map[str.DRN]str.DRNVal{
				str.DRN(QuickFormat([]string{"a"}, "b")): "val1",
			},
			after: map[str.DRN]str.DRNVal{
				str.DRN(QuickFormat([]string{"a"}, "b")): "val2",
			},
			expect: []str.DRN{str.DRN(QuickFormat([]string{"a"}, "b"))},
		},
		{
			name: "Single entry removed",
			before: map[str.DRN]str.DRNVal{
				str.DRN(QuickFormat([]string{"a"}, "b")): "val1",
			},
			after:  map[str.DRN]str.DRNVal{},
			expect: []str.DRN{str.DRN(QuickFormat([]string{"a"}, "b"))},
		},
		{
			name:   "Single entry added",
			before: map[str.DRN]str.DRNVal{},
			after: map[str.DRN]str.DRNVal{
				str.DRN(QuickFormat([]string{"a"}, "b")): "val1",
			},
			expect: []str.DRN{str.DRN(QuickFormat([]string{"a"}, "b"))},
		},
		{
			name: "Removed and added with same value but different keys",
			before: map[str.DRN]str.DRNVal{
				str.DRN(QuickFormat([]string{"old"}, "key")): "val1",
			},
			after: map[str.DRN]str.DRNVal{
				str.DRN(QuickFormat([]string{"new"}, "key")): "val1",
			},
			expect: []str.DRN{str.DRN(QuickFormat([]string{"old"}, "key")),
				str.DRN(QuickFormat([]string{"new"}, "key"))},
		},
		{
			name: "Large map with mixed changes",
			before: map[str.DRN]str.DRNVal{
				str.DRN(QuickFormat([]string{"1"}, "k")): "v1",
				str.DRN(QuickFormat([]string{"2"}, "k")): "v2",
				str.DRN(QuickFormat([]string{"3"}, "k")): "v3",
				str.DRN(QuickFormat([]string{"4"}, "k")): "v4",
				str.DRN(QuickFormat([]string{"5"}, "k")): "v5",
				str.DRN(QuickFormat([]string{"6"}, "k")): "v6",
				str.DRN(QuickFormat([]string{"7"}, "k")): "v7",
				str.DRN(QuickFormat([]string{"8"}, "k")): "v8",
			},
			after: map[str.DRN]str.DRNVal{
				str.DRN(QuickFormat([]string{"1"}, "k")):  "v1",
				str.DRN(QuickFormat([]string{"2"}, "k")):  "v2_new",
				str.DRN(QuickFormat([]string{"3"}, "k")):  "v3",
				str.DRN(QuickFormat([]string{"5"}, "k")):  "v5",
				str.DRN(QuickFormat([]string{"6"}, "k")):  "v6",
				str.DRN(QuickFormat([]string{"7"}, "k")):  "v7",
				str.DRN(QuickFormat([]string{"9"}, "k")):  "v9",
				str.DRN(QuickFormat([]string{"10"}, "k")): "v10",
			},
			expect: []str.DRN{
				str.DRN(QuickFormat([]string{"2"}, "k")),
				str.DRN(QuickFormat([]string{"4"}, "k")),
				str.DRN(QuickFormat([]string{"8"}, "k")),
				str.DRN(QuickFormat([]string{"9"}, "k")),
				str.DRN(QuickFormat([]string{"10"}, "k")),
			},
		},
	}

	// Loop over all test cases
	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			got := DiffSet(test.before, test.after)

			// Reorder (since map will make slices random)
			slices.Sort(got)
			slices.Sort(test.expect)

			if !slices.Equal(test.expect, got) {
				t.Fatalf("Mismatch diff:\nexpected: %#v\ngot:      %#v", test.expect, got)
			}
		})
	}
}

func TestLikelyDRN(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		expectedIsDRN bool
	}{
		{
			name:          "Valid DRN",
			input:         QuickFormat([]string{"cfg"}, "field"),
			expectedIsDRN: true,
		},
		{
			name:          "DRN with misc prefix is invalid",
			input:         "prefix" + QuickFormat([]string{"cfg"}, "field"),
			expectedIsDRN: false,
		},
		{
			name:          "Valid DRN with misc suffix",
			input:         QuickFormat([]string{"cfg"}, "field") + "suffix",
			expectedIsDRN: true,
		},
		{
			name:          "DRN missing open delimiter",
			input:         Prefix + "t" + PrimarySeparator + "f" + CloseDelimiter,
			expectedIsDRN: false,
		},
		{
			name:          "Not DRN",
			input:         "data data data",
			expectedIsDRN: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := LikelyDRN(test.input)
			if test.expectedIsDRN != got {
				t.Errorf("isDRN? got '%v', expected '%v'", got, test.expectedIsDRN)
			}
		})
	}
}
