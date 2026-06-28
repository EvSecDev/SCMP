package drn

import (
	"scmp/internal/str"
	"scmp/internal/tests/utils"
	"slices"
	"strings"
	"testing"
)

func TestValidateDRN(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expect      DRC
		expectError string
	}{
		{
			name:  "Basic Internal Valid",
			input: QuickFormat([]string{InternalNamespacePrefix}, "Field1", "field2"),
			expect: DRC{
				Original:  str.DRNRaw(QuickFormat([]string{InternalNamespacePrefix}, "Field1", "field2")),
				Namespace: []string{InternalNamespacePrefix},
				Fields:    []string{"Field1", "field2"},
			},
		},
		{
			name:  "Basic External Valid",
			input: QuickFormat([]string{"Repo"}, "field"),
			expect: DRC{
				Original:  str.DRNRaw(QuickFormat([]string{"Repo"}, "field")),
				Namespace: []string{"Repo"},
				Fields:    []string{"field"},
			},
		},
		{
			name:  "Nested Namespace Valid",
			input: QuickFormat([]string{"repo", "subdir", "main"}, "field"),
			expect: DRC{
				Original:  str.DRNRaw(QuickFormat([]string{"repo", "subdir", "main"}, "field")),
				Namespace: []string{"repo", "subdir", "main"},
				Fields:    []string{"field"},
			},
		},
		{
			name: "Max Namespace Depth",
			input: QuickFormat([]string{
				"a",
				"b",
				"c",
				"d",
				"e",
				"f",
				"g",
				"h",
				"i",
				"j",
			}, "field"),
			expect: DRC{
				Original: str.DRNRaw(QuickFormat([]string{
					"a",
					"b",
					"c",
					"d",
					"e",
					"f",
					"g",
					"h",
					"i",
					"j",
				}, "field")),
				Namespace: []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j"},
				Fields:    []string{"field"},
			},
		},
		{
			name: "Max Field Depth",
			input: QuickFormat([]string{"repo"},
				"a",
				"b",
				"c",
				"d",
				"e",
				"f",
				"g",
				"h",
				"i",
				"j",
			),
			expect: DRC{
				Original: str.DRNRaw(QuickFormat([]string{"repo"},
					"a",
					"b",
					"c",
					"d",
					"e",
					"f",
					"g",
					"h",
					"i",
					"j",
				)),
				Namespace: []string{"repo"},
				Fields:    []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j"},
			},
		},
		{
			name:  "Valid Macro In Namespace",
			input: QuickFormat([]string{"repo{{var}}"}, "field"),
			expect: DRC{
				Original:  str.DRNRaw(QuickFormat([]string{"repo{{var}}"}, "field")),
				Namespace: []string{"repo{{var}}"},
				Fields:    []string{"field"},
			},
		},
		{
			name:  "Valid Macro In Field",
			input: QuickFormat([]string{"repo"}, "field{{var}}"),
			expect: DRC{
				Original:  str.DRNRaw(QuickFormat([]string{"repo"}, "field{{var}}")),
				Namespace: []string{"repo"},
				Fields:    []string{"field{{var}}"},
			},
		},
		{
			name:  "Multiple Valid Macros",
			input: QuickFormat([]string{"repo"}, "field{{a}}{{b}}"),
			expect: DRC{
				Original:  str.DRNRaw(QuickFormat([]string{"repo"}, "field{{a}}{{b}}")),
				Namespace: []string{"repo"},
				Fields:    []string{"field{{a}}{{b}}"},
			},
		},
		{
			name:        "Missing Open Delimiter",
			input:       Prefix + "repo" + PrimarySeparator + "field",
			expectError: ErrNotDRN.Error(),
		},
		{
			name:        "Missing Prefix",
			input:       "repo" + PrimarySeparator + "field",
			expectError: ErrNotDRN.Error(),
		},
		{
			name:        "Wrong Prefix Case",
			input:       OpenDelimiter + "SCMP://repo" + PrimarySeparator + "field",
			expectError: ErrNotDRN.Error(),
		},
		{
			name:        "Below Minimum Length",
			input:       OpenDelimiter + Prefix + CloseDelimiter,
			expectError: "below the minimum length",
		},
		{
			name:        "Above Maximum Length",
			input:       OpenDelimiter + Prefix + strings.Repeat("a", MaxTotalLength-len(OpenDelimiter)-len(Prefix)) + CloseDelimiter,
			expectError: "exceeds maximum length",
		},
		{
			name:        "Space In Namespace",
			input:       OpenDelimiter + Prefix + "my repo" + PrimarySeparator + "field" + CloseDelimiter,
			expectError: "cannot contain spaces",
		},
		{
			name:        "Space In Field",
			input:       OpenDelimiter + Prefix + "repo" + PrimarySeparator + "my field" + CloseDelimiter,
			expectError: "cannot contain spaces",
		},
		{
			name:        "Missing Primary Separator",
			input:       OpenDelimiter + Prefix + "repofield" + CloseDelimiter,
			expectError: "missing primary separator character",
		},
		{
			name:        "Multiple Primary Separators",
			input:       OpenDelimiter + Prefix + "repo" + PrimarySeparator + "field" + PrimarySeparator + "extra" + CloseDelimiter,
			expectError: "too many primary separator characters",
		},
		{
			name:        "Single Open Brace",
			input:       OpenDelimiter + Prefix + "repo" + PrimarySeparator + "field{" + CloseDelimiter,
			expectError: "single opening brace",
		},
		{
			name:        "Single Close Brace",
			input:       OpenDelimiter + Prefix + "repo" + PrimarySeparator + "field}" + CloseDelimiter,
			expectError: "single closing brace",
		},
		{
			name:        "Unmatched Close Macro",
			input:       OpenDelimiter + Prefix + "repo" + PrimarySeparator + "field}}" + CloseDelimiter,
			expectError: "unmatched macro close",
		},
		{
			name:        "Unclosed Macro",
			input:       OpenDelimiter + Prefix + "repo" + PrimarySeparator + "field{{abc" + CloseDelimiter,
			expectError: "unclosed macro",
		},
		{
			name:        "Nested Macro",
			input:       OpenDelimiter + Prefix + "repo" + PrimarySeparator + "field{{outer{{inner}}}}" + CloseDelimiter,
			expectError: "nested macros are not permitted",
		},
		{
			name:        "Empty Macro",
			input:       OpenDelimiter + Prefix + "repo" + PrimarySeparator + "field{{}}" + CloseDelimiter,
			expectError: "empty macros are not permitted",
		},
		{
			name:        "Triple Opening Brace",
			input:       OpenDelimiter + Prefix + "repo" + PrimarySeparator + "field{{{abc}}" + CloseDelimiter,
			expectError: "single opening brace is not permitted",
		},
		{
			name:        "Triple Closing Brace",
			input:       OpenDelimiter + Prefix + "repo" + PrimarySeparator + "field{{abc}}}" + CloseDelimiter,
			expectError: "single closing brace",
		},
		{
			name:        "Empty Namespace",
			input:       OpenDelimiter + Prefix + NamespaceSeparator + "ns" + PrimarySeparator + "field" + CloseDelimiter,
			expectError: "namespace segment 1 is empty",
		},
		{
			name: "Namespace Too Deep",
			input: QuickFormat([]string{
				"a",
				"b",
				"c",
				"d",
				"e",
				"f",
				"g",
				"h",
				"i",
				"j",
				"k",
			}, "field"),
			expectError: "exceeds maximum component count",
		},
		{
			name:        "Empty Namespace Segment",
			input:       OpenDelimiter + Prefix + "repo" + NamespaceSeparator + NamespaceSeparator + "child" + PrimarySeparator + "field" + CloseDelimiter,
			expectError: "namespace segment 2 is empty",
		},
		{
			name:        "Invalid Namespace Character",
			input:       QuickFormat([]string{"repo!"}, "field"),
			expectError: "contains unsupported characters",
		},
		{
			name:        "Empty Field List",
			input:       OpenDelimiter + Prefix + "repo" + PrimarySeparator + FieldSeparator + "field" + CloseDelimiter,
			expectError: "field 1 is empty",
		},
		{
			name: "Field Too Deep",
			input: QuickFormat([]string{"repo"},
				"a",
				"b",
				"c",
				"d",
				"e",
				"f",
				"g",
				"h",
				"i",
				"j",
				"k",
			),
			expectError: "exceeds maximum field count",
		},
		{
			name:        "Empty Field Segment",
			input:       OpenDelimiter + Prefix + "repo" + PrimarySeparator + "field" + FieldSeparator + FieldSeparator + "name" + CloseDelimiter,
			expectError: "field 2 is empty",
		},
		{
			name:        "Invalid Field Character",
			input:       QuickFormat([]string{"repo"}, "field!"),
			expectError: "contains unsupported characters",
		},
		{
			name:        "Unicode Namespace",
			input:       QuickFormat([]string{"répo"}, "field"),
			expectError: "contains unsupported characters",
		},
		{
			name:        "Unicode Field",
			input:       QuickFormat([]string{"repo"}, "fíeld"),
			expectError: "contains unsupported characters",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := Validate(test.input)
			matches, err := utils.MatchErrorString(err, test.expectError)
			if err != nil {
				t.Fatalf("%v", err)
			} else if matches {
				return
			}

			if result.Original != test.expect.Original {
				t.Errorf("DRN mismatch:\nExpected: %s\nGot:      %s", test.expect.Original, result.Original)
			}
			if !slices.Equal(result.Namespace, test.expect.Namespace) {
				t.Errorf("DRN Namespace mismatch:\nExpected: %#v\nGot:      %#v", test.expect.Namespace, result.Namespace)
			}
			if !slices.Equal(result.Fields, test.expect.Fields) {
				t.Errorf("DRN Fields mismatch:\nExpected: %#v\nGot:      %#v", test.expect.Fields, result.Fields)
			}
		})
	}
}
