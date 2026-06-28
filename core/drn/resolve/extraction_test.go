package resolve

import (
	"fmt"
	"reflect"
	"scmp/core/deployment"
	"scmp/core/drn"
	"scmp/internal/str"
	"sort"
	"strings"
	"testing"
)

func TestExtractHeaderDRNs(t *testing.T) {
	tests := []struct {
		name        string
		state       *Replacer
		hostAlias   str.RepoRootDir
		file        str.LocalRepoPath
		header      deployment.FileInfo
		expect      map[originKey][]string
		expectCount int
	}{
		{
			name:      "Basic",
			state:     NewReplacer("/tmp", nil),
			hostAlias: "host1",
			file:      "conf1",
			header: deployment.FileInfo{
				Predeploy: []string{"command1", "command2 -" + drn.QuickFormat([]string{"conf1"}, "field1")},
			},
			expect: map[originKey][]string{
				{
					globalID:    "host1",
					file:        "conf1",
					headerField: headerPreDeploy,
				}: {drn.QuickFormat([]string{"conf1"}, "field1")},
			},
			expectCount: 1,
		},
		{
			name:      "Empty/nil header fields are skipped",
			hostAlias: "host1",
			file:      "conf1",
			header: deployment.FileInfo{
				Predeploy: nil,
				Install:   []string{},
			},
			expect: map[originKey][]string{},
		},
		{
			name:      "Multiple fields populated independently",
			hostAlias: "host2",
			file:      "conf2",
			header: deployment.FileInfo{
				Predeploy: []string{"cmd " + drn.QuickFormat([]string{"a"}, "b")},
				Install:   []string{"cmd " + drn.QuickFormat([]string{"c"}, "d")},
			},
			expect: map[originKey][]string{
				{globalID: "host2", file: "conf2", headerField: headerPreDeploy}: {drn.QuickFormat([]string{"a"}, "b")},
				{globalID: "host2", file: "conf2", headerField: headerInstall}:   {drn.QuickFormat([]string{"c"}, "d")},
			},
			expectCount: 2,
		},
		{
			name:      "Multiple DRNs extracted from a single command",
			hostAlias: "host1",
			file:      "conf1",
			header: deployment.FileInfo{
				Checks: []string{"validate " + drn.QuickFormat([]string{"x"}, "1") + " " + drn.QuickFormat([]string{"y"}, "2")},
			},
			expect: map[originKey][]string{
				{globalID: "host1", file: "conf1", headerField: headerChecks}: {drn.QuickFormat([]string{"x"}, "1"), drn.QuickFormat([]string{"y"}, "2")},
			},
			expectCount: 2,
		},
		{
			name:      "Commands with no DRNs produce no entries",
			hostAlias: "host1",
			file:      "conf1",
			header: deployment.FileInfo{
				Reload: []string{"echo 'hello'", "ls -la", "true"},
			},
			expect: map[originKey][]string{},
		},
		{
			name:      "Multiple commands in one field append to same key",
			hostAlias: "host3",
			file:      "conf3",
			header: deployment.FileInfo{
				Install: []string{
					"install " + drn.QuickFormat([]string{"pkg1"}, "v1"),
					"install " + drn.QuickFormat([]string{"pkg2"}, "v2"),
					"install plain-command",
				},
			},
			expect: map[originKey][]string{
				{globalID: "host3", file: "conf3", headerField: headerInstall}: {drn.QuickFormat([]string{"pkg1"}, "v1"), drn.QuickFormat([]string{"pkg2"}, "v2")},
			},
			expectCount: 2,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.state == nil {
				test.state = NewReplacer("/", nil)
			}

			test.state.ExtractHeaderDRNs(test.hostAlias, test.file, test.header)

			got := test.state.unvalidatedDRNs

			equal, diff := CompareOriginKeyMaps(test.expect, got)
			if !equal {
				t.Errorf("got: %#v\n", got)
				t.Errorf("exp: %#v\n", test.expect)
				t.Fatalf("%s", diff)
			}

			if test.state.ExtractedCount() != test.expectCount {
				t.Errorf("expected extracted count %d but got %d", test.expectCount, test.state.ExtractedCount())
			}

			err := test.state.initDRConfigs()
			if err != nil {
				t.Fatalf("failed to test: init DRN configs: %v", err)
			}
		})
	}
}

func TestExtractDRNs(t *testing.T) {
	tests := []struct {
		name        string
		state       *Replacer
		hostAlias   str.RepoRootDir
		file        str.LocalRepoPath
		input       []byte
		expect      map[originKey][]string
		expectCount int
	}{
		{
			name:      "Basic",
			state:     NewReplacer("/tmp", nil),
			hostAlias: "host1",
			file:      "conf1",
			input:     []byte("data data data data" + drn.QuickFormat([]string{"conf1"}, "field1") + "more data more data"),
			expect: map[originKey][]string{
				{
					globalID: "host1",
					file:     "conf1",
				}: {drn.QuickFormat([]string{"conf1"}, "field1")},
			},
			expectCount: 1,
		},
		{
			name:      "Empty input produces no entries",
			hostAlias: "host1",
			file:      "conf1",
			input:     []byte{},
			expect:    map[originKey][]string{},
		},
		{
			name:      "No prefix present",
			hostAlias: "host1",
			file:      "conf1",
			input:     []byte("just plain text without any drn-like strings here"),
			expect:    map[originKey][]string{},
		},
		{
			name:      "Partial prefix should not match",
			hostAlias: "host1",
			file:      "conf1",
			input:     []byte("data " + drn.Prefix[:len(drn.Prefix)-1] + "conf" + drn.PrimarySeparator + "1" + " more"),
			expect:    map[originKey][]string{},
		},
		{
			name:      "Multiple DRNs in same payload",
			hostAlias: "host2",
			file:      "conf2",
			input:     []byte("start " + drn.QuickFormat([]string{"a"}, "1") + " middle " + drn.QuickFormat([]string{"b"}, "2") + " end"),
			expect: map[originKey][]string{
				{globalID: "host2", file: "conf2", headerField: ""}: {drn.QuickFormat([]string{"a"}, "1"), drn.QuickFormat([]string{"b"}, "2")},
			},
			expectCount: 2,
		},
		{
			name:      "DRN at exact end of payload (no trailing space)",
			hostAlias: "host1",
			file:      "conf1",
			input:     []byte("prefix " + drn.QuickFormat([]string{"end"}, "1")),
			expect: map[originKey][]string{
				{globalID: "host1", file: "conf1", headerField: ""}: {drn.QuickFormat([]string{"end"}, "1")},
			},
			expectCount: 1,
		},
		{
			name:      "DRN immediately followed by space",
			hostAlias: "host1",
			file:      "conf1",
			input:     []byte(drn.QuickFormat([]string{"test"}, "x") + " next"),
			expect: map[originKey][]string{
				{globalID: "host1", file: "conf1", headerField: ""}: {drn.QuickFormat([]string{"test"}, "x")},
			},
			expectCount: 1,
		},
		{
			name:      "Adjacent DRNs separated by single space",
			hostAlias: "host1",
			file:      "conf1",
			input:     []byte(drn.QuickFormat([]string{"a"}, "1") + " " + drn.QuickFormat([]string{"b"}, "2")),
			expect: map[originKey][]string{
				{globalID: "host1", file: "conf1", headerField: ""}: {drn.QuickFormat([]string{"a"}, "1"), drn.QuickFormat([]string{"b"}, "2")},
			},
			expectCount: 2,
		},
		{
			name:        "Unclosed DRN at end of payload (no closing bracket)",
			hostAlias:   "host1",
			file:        "conf1",
			input:       []byte("prefix <scmp://a@b"),
			expect:      map[originKey][]string{},
			expectCount: 0,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.state == nil {
				test.state = NewReplacer("/", nil)
			}

			test.state.ExtractDRNs(test.hostAlias, test.file, test.input)

			got := test.state.unvalidatedDRNs

			equal, diff := CompareOriginKeyMaps(test.expect, got)
			if !equal {
				t.Errorf("got: %#v\n", got)
				t.Errorf("exp: %#v\n", test.expect)
				t.Fatalf("%s", diff)
			}

			if test.state.ExtractedCount() != test.expectCount {
				t.Errorf("expected extracted count %d but got %d", test.expectCount, test.state.ExtractedCount())
			}

			err := test.state.initDRConfigs()
			if err != nil {
				t.Fatalf("failed to test: init DRN configs: %v", err)
			}
		})
	}
}

func CompareOriginKeyMaps(expected, actual map[originKey][]string) (equal bool, diff string) {
	if len(expected) != len(actual) {
		diff = fmt.Sprintf("map lengths differ: expected %d, got %d", len(expected), len(actual))
		return
	}

	var diffs []string

	for k, expectedSlice := range expected {
		actualSlice, exists := actual[k]
		if !exists {
			diffs = append(diffs, fmt.Sprintf("key %#v missing in actual map", k))
			continue
		}

		if len(expectedSlice) != len(actualSlice) {
			diffs = append(diffs, fmt.Sprintf("slice length for key %#v differs: expected %d, got %d", k, len(expectedSlice), len(actualSlice)))
			continue
		}

		// Create copies to avoid mutating original data
		expSorted := make([]string, len(expectedSlice))
		copy(expSorted, expectedSlice)
		sort.Strings(expSorted)

		actSorted := make([]string, len(actualSlice))
		copy(actSorted, actualSlice)
		sort.Strings(actSorted)

		if !reflect.DeepEqual(expSorted, actSorted) {
			diffs = append(diffs, fmt.Sprintf("slice content for key %#v differs:\n  expected: %v\n  actual:   %v", k, expSorted, actSorted))
		}
	}

	if len(diffs) > 0 {
		diff = fmt.Sprintf("map mismatch:\n%s", strings.Join(diffs, "\n"))
		return
	}
	equal = true
	return
}
