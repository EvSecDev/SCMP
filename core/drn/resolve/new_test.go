package resolve

import (
	"scmp/core/drn"
	"scmp/internal/tests/utils"
	"testing"
)

func TestReplacerExtractedCount(t *testing.T) {
	tests := []struct {
		name  string
		count int
		kvs   map[originKey][]string
	}{
		{
			name:  "Empty",
			count: 0,
			kvs:   map[originKey][]string{},
		},
		{
			name:  "Single DRN",
			count: 1,
			kvs: map[originKey][]string{
				{globalID: "h1", file: "f1"}: {drn.QuickFormat([]string{"ns"}, "f")},
			},
		},
		{
			name:  "Multiple DRNs same key",
			count: 3,
			kvs: map[originKey][]string{
				{globalID: "h1", file: "f1"}: {"d1", "d2", "d3"},
			},
		},
		{
			name:  "Multiple keys",
			count: 4,
			kvs: map[originKey][]string{
				{globalID: "h1", file: "f1"}: {"d1", "d2"},
				{globalID: "h2", file: "f2"}: {"d3", "d4"},
			},
		},
		{
			name:  "Include headerField key",
			count: 3,
			kvs: map[originKey][]string{
				{globalID: "h1", file: "f1"}:                           {"d1"},
				{globalID: "h1", file: "f1", headerField: "PreDeploy"}: {"d2", "d3"},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			replacer := NewReplacer("/tmp", nil)
			replacer.unvalidatedDRNs = test.kvs

			got := replacer.ExtractedCount()
			if got != test.count {
				t.Errorf("got      %d, expected %d", got, test.count)
			}
		})
	}
}

func TestReplacerInitDRConfigs(t *testing.T) {
	tests := []struct {
		name        string
		pending     map[originKey][]string
		expectCount int
		expectError string
	}{
		{
			name:        "Empty pending returns immediately",
			pending:     map[originKey][]string{},
			expectCount: 0,
			expectError: "",
		},
		{
			name: "Valid DRN creates config",
			pending: map[originKey][]string{
				{globalID: "h1", file: "f1"}: {drn.QuickFormat([]string{"ns"}, "field")},
			},
			expectCount: 1,
			expectError: "",
		},
		{
			name: "Multiple valid DRNs",
			pending: map[originKey][]string{
				{globalID: "h1", file: "f1"}: {drn.QuickFormat([]string{"a"}, "b"), drn.QuickFormat([]string{"c"}, "d")},
				{globalID: "h2", file: "f2"}: {drn.QuickFormat([]string{"x"}, "y")},
			},
			expectCount: 3,
			expectError: "",
		},
		{
			name: "Multiple keys accumulate correctly",
			pending: map[originKey][]string{
				{globalID: "h1", file: "f1", headerField: "PreDeploy"}: {drn.QuickFormat([]string{"a"}, "b")},
				{globalID: "h1", file: "f1", headerField: "Install"}:   {drn.QuickFormat([]string{"c"}, "d")},
			},
			expectCount: 2,
			expectError: "",
		},
		{
			name: "Invalid DRN returns error",
			pending: map[originKey][]string{
				{globalID: "h1", file: "f1"}: {"not-a-drn"},
			},
			expectCount: 0,
			expectError: "h1 file 'f1'",
		},
		{
			name: "Invalid DRN in header field includes field name",
			pending: map[originKey][]string{
				{globalID: "h1", file: "f1", headerField: "PreDeploy"}: {"not-a-drn"},
			},
			expectCount: 0,
			expectError: "h1 file 'f1' header PreDeploy",
		},
		{
			name: "Mixed valid and invalid halts at first invalid",
			pending: map[originKey][]string{
				{globalID: "h1", file: "f1"}: {drn.Prefix + "a" + "bad-drn"},
			},
			expectCount: 0,
			expectError: "h1 file 'f1'",
		},
		{
			name: "Resets unvalidatedDRNs after init",
			pending: map[originKey][]string{
				{globalID: "h1", file: "f1"}: {drn.QuickFormat([]string{"ns"}, "f")},
			},
			expectCount: 1,
			expectError: "",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			replacer := NewReplacer("/tmp", nil)
			replacer.unvalidatedDRNs = test.pending

			err := replacer.initDRConfigs()
			matches, err := utils.MatchErrorString(err, test.expectError)
			if err != nil {
				t.Fatalf("%v", err)
			} else if matches {
				return
			}

			// Count total DRNs in originOfDRN
			total := 0
			for _, drns := range replacer.originOfDRN {
				total += len(drns)
			}

			if total != test.expectCount {
				t.Errorf("originOfDRN count = %d, expected %d", total, test.expectCount)
			}

			// unvalidatedDRNs should be reset (empty map)
			if len(replacer.unvalidatedDRNs) != 0 {
				t.Errorf("unvalidatedDRNs should be empty after init, got %d entries", len(replacer.unvalidatedDRNs))
			}
		})
	}
}
