package drnconfig

import (
	"scmp/core/drn"
	"scmp/internal/str"
	"scmp/internal/tests/utils"
	"strings"
	"testing"
)

func TestCfgNodeLookupValue(t *testing.T) {
	tests := []struct {
		name        string
		node        CfgNode
		drc         drn.DRC
		expectValue str.DRNVal
		expectError string
	}{
		{
			name: "Single field lookup",
			node: CfgNode{
				"key": CfgValue{kind: kindString, str: "value"},
			},
			drc:         drn.DRC{Fields: []string{"key"}},
			expectValue: str.DRNVal("value"),
		},
		{
			name:        "Two field lookup",
			node:        MapToCfgNode(map[string]string{"top" + drn.FieldSeparator + "bottom": "deep"}),
			drc:         drn.DRC{Fields: []string{"top", "bottom"}},
			expectValue: str.DRNVal("deep"),
		},
		{
			name:        "Three field deep lookup",
			node:        MapToCfgNode(map[string]string{"a" + drn.FieldSeparator + "b" + drn.FieldSeparator + "c": "deep-value"}),
			drc:         drn.DRC{Fields: []string{"a", "b", "c"}},
			expectValue: str.DRNVal("deep-value"),
		},
		{
			name:        "Nil node returns error for missing field",
			node:        nil,
			drc:         drn.DRC{Fields: []string{"key"}},
			expectError: "field 'key' not found at depth 0 in config",
		},
		{
			name:        "Empty node returns error for missing field",
			node:        CfgNode{},
			drc:         drn.DRC{Fields: []string{"key"}},
			expectError: "field 'key' not found at depth 0 in config",
		},
		{
			name: "Missing intermediate field",
			node: CfgNode{
				"a": CfgValue{kind: kindString, str: "leaf"},
			},
			drc:         drn.DRC{Fields: []string{"a", "b"}},
			expectError: "field 'a' at depth 0 is a string, but expected an object",
		},
		{
			name: "Final field is object instead of string",
			node: CfgNode{
				"a": CfgValue{
					kind: kindObject,
					obj: CfgNode{
						"b": CfgValue{
							kind: kindObject,
							obj: CfgNode{
								"c": CfgValue{kind: kindString, str: "val"},
							},
						},
					},
				},
			},
			drc:         drn.DRC{Fields: []string{"a", "b"}},
			expectError: "field 'b' must be value of type string (got object)",
		},
		{
			name:        "Missing field at deep level",
			node:        MapToCfgNode(map[string]string{"a" + drn.FieldSeparator + "b": "val"}),
			drc:         drn.DRC{Fields: []string{"a", "b", "c"}},
			expectError: "field 'b' at depth 1 is a string, but expected an object",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value, err := test.node.LookupValue(test.drc.Fields)
			matches, err := utils.MatchErrorString(err, test.expectError)
			if err != nil {
				t.Fatalf("%v", err)
			} else if matches {
				return
			}

			if value != test.expectValue {
				t.Errorf("got %q, expected %q", value, test.expectValue)
			}
		})
	}
}

func TestCfgNodeInsertValue(t *testing.T) {
	tests := []struct {
		name        string
		node        CfgNode
		drc         drn.DRC
		value       str.DRNVal
		expectNode  CfgNode
		expectError string
	}{
		{
			name:       "Insert into nil node",
			node:       nil,
			drc:        drn.DRC{Fields: []string{"key"}},
			value:      str.DRNVal("val"),
			expectNode: MapToCfgNode(map[string]string{"key": "val"}),
		},
		{
			name:       "Insert into empty node",
			node:       CfgNode{},
			drc:        drn.DRC{Fields: []string{"key"}},
			value:      str.DRNVal("val"),
			expectNode: MapToCfgNode(map[string]string{"key": "val"}),
		},
		{
			name:       "Overwrite existing value",
			node:       MapToCfgNode(map[string]string{"key": "old"}),
			drc:        drn.DRC{Fields: []string{"key"}},
			value:      str.DRNVal("new"),
			expectNode: MapToCfgNode(map[string]string{"key": "new"}),
		},
		{
			name:       "Insert with new intermediate node",
			node:       CfgNode{},
			drc:        drn.DRC{Fields: []string{"a", "b"}},
			value:      str.DRNVal("val"),
			expectNode: MapToCfgNode(map[string]string{"a" + drn.FieldSeparator + "b": "val"}),
		},
		{
			name:  "Insert under existing intermediate node",
			node:  MapToCfgNode(map[string]string{"a" + drn.FieldSeparator + "existing": "other"}),
			drc:   drn.DRC{Fields: []string{"a", "new"}},
			value: str.DRNVal("val"),
			expectNode: MapToCfgNode(map[string]string{
				"a" + drn.FieldSeparator + "existing": "other",
				"a" + drn.FieldSeparator + "new":      "val",
			}),
		},
		{
			name:       "Deep insert creating multiple levels",
			node:       CfgNode{},
			drc:        drn.DRC{Fields: []string{"a", "b", "c"}},
			value:      str.DRNVal("deep"),
			expectNode: MapToCfgNode(map[string]string{"a" + drn.FieldSeparator + "b" + drn.FieldSeparator + "c": "deep"}),
		},
		{
			name: "Conflict: final field is object",
			node: CfgNode{
				"a": CfgValue{
					kind: kindObject,
					obj: CfgNode{
						"leaf": CfgValue{kind: kindString, str: "val"},
					},
				},
			},
			drc:         drn.DRC{Fields: []string{"a"}},
			value:       str.DRNVal("new"),
			expectError: "field 0 (a) expected final field to be string but found object",
		},
		{
			name: "Conflict: intermediate field is string",
			node: CfgNode{
				"a": CfgValue{kind: kindString, str: "val"},
			},
			drc:         drn.DRC{Fields: []string{"a", "b"}},
			value:       str.DRNVal("deep"),
			expectError: "field 0 (a) expected object but found string",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			node := test.node
			err := node.InsertValue(test.drc.Fields, test.value)
			matches, err := utils.MatchErrorString(err, test.expectError)
			if err != nil {
				t.Fatalf("%v", err)
			} else if matches {
				return
			}

			// Compare structure
			if !cfgNodesEqual(node, test.expectNode) {
				t.Errorf("cfgNode mismatch")
				t.Errorf("got:  %+v", dumpCfgNode(node, ""))
				t.Errorf("want: %+v", dumpCfgNode(test.expectNode, ""))
			}
		})
	}
}

func cfgNodesEqual(a, b CfgNode) (isEqual bool) {
	if a == nil && b == nil {
		isEqual = true
		return
	}
	if len(a) != len(b) {
		return
	}
	for k, v := range a {
		other, ok := b[k]
		if !ok {
			return
		}
		if v.kind != other.kind {
			return
		}
		if v.kind == kindString && v.str != other.str {
			return
		}
		if v.kind == kindObject && !cfgNodesEqual(v.obj, other.obj) {
			return
		}
	}
	isEqual = true
	return
}

func dumpCfgNode(node CfgNode, indent string) (txt string) {
	var sb strings.Builder
	for k, v := range node {
		if v.kind == kindString {
			sb.WriteString(indent)
			sb.WriteString(k)
			sb.WriteString(": ")
			sb.WriteString(v.str)
			sb.WriteString("\n")
		} else {
			sb.WriteString(indent)
			sb.WriteString(k)
			sb.WriteString(":\n")
			sb.WriteString(dumpCfgNode(v.obj, indent+"  "))
		}
	}
	txt = sb.String()
	return
}

func TestVisitNode(t *testing.T) {
	tests := []struct {
		name     string
		node     CfgNode
		expected map[string]str.DRNVal
	}{
		{
			name:     "Nil node returns empty bucket",
			node:     nil,
			expected: map[string]str.DRNVal{},
		},
		{
			name:     "Empty node returns empty bucket",
			node:     CfgNode{},
			expected: map[string]str.DRNVal{},
		},
		{
			name:     "Single string leaf",
			node:     MapToCfgNode(map[string]string{"key": "val"}),
			expected: map[string]str.DRNVal{"key": "val"},
		},
		{
			name: "Two string leaves",
			node: MapToCfgNode(map[string]string{
				"a": "1",
				"b": "2",
			}),
			expected: map[string]str.DRNVal{
				"a": "1",
				"b": "2",
			},
		},
		{
			name: "One level nesting",
			node: MapToCfgNode(map[string]string{
				"top" + drn.FieldSeparator + "bottom": "deep",
			}),
			expected: map[string]str.DRNVal{
				"top" + drn.FieldSeparator + "bottom": "deep",
			},
		},
		{
			name: "Mixed string and object",
			node: MapToCfgNode(map[string]string{
				"leaf":                                  "value",
				"nested" + drn.FieldSeparator + "inner": "inner-value",
			}),
			expected: map[string]str.DRNVal{
				"leaf":                                  "value",
				"nested" + drn.FieldSeparator + "inner": "inner-value",
			},
		},
		{
			name: "Deep nesting",
			node: MapToCfgNode(map[string]string{
				"a" + drn.FieldSeparator + "b" + drn.FieldSeparator + "c": "deep",
			}),
			expected: map[string]str.DRNVal{
				"a" + drn.FieldSeparator + "b" + drn.FieldSeparator + "c": "deep",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			bucket := make(map[string]str.DRNVal)
			visitNode(test.node, []string{}, bucket)

			if len(bucket) != len(test.expected) {
				t.Fatalf("bucket length = %d, expected %d", len(bucket), len(test.expected))
			}

			for k, v := range test.expected {
				got, ok := bucket[k]
				if !ok {
					t.Errorf("key %q not found in bucket", k)
					continue
				}
				if got != v {
					t.Errorf("bucket[%q] = %q, expected %q", k, got, v)
				}
			}
		})
	}
}

func TestCfgNodeFormatAll(t *testing.T) {
	tests := []struct {
		name        string
		node        CfgNode
		path        string
		expected    map[str.DRN]str.DRNVal
		expectError string
	}{
		{
			name:        "Nil node returns nil map",
			node:        nil,
			path:        drn.ExternalVariableDirectory + "/hosts",
			expected:    nil,
			expectError: "",
		},
		{
			name:        "Empty node returns empty map",
			node:        CfgNode{},
			path:        drn.ExternalVariableDirectory + "/hosts",
			expected:    make(map[str.DRN]str.DRNVal),
			expectError: "",
		},
		{
			name: "Single value formats correctly",
			node: MapToCfgNode(map[string]string{"type": "production"}),
			path: drn.ExternalVariableDirectory + "/hosts",
			expected: map[str.DRN]str.DRNVal{
				str.DRN(drn.QuickFormat([]string{"hosts"}, "type")): "production",
			},
		},
		{
			name: "Two values format correctly",
			node: MapToCfgNode(map[string]string{
				"type":  "production",
				"count": "3",
			}),
			path: drn.ExternalVariableDirectory + "/hosts",
			expected: map[str.DRN]str.DRNVal{
				str.DRN(drn.QuickFormat([]string{"hosts"}, "type")):  "production",
				str.DRN(drn.QuickFormat([]string{"hosts"}, "count")): "3",
			},
		},
		{
			name: "Nested values format with dots",
			node: MapToCfgNode(map[string]string{
				"server" + drn.FieldSeparator + "host": "127.0.0.1",
				"server" + drn.FieldSeparator + "port": "8080",
			}),
			path: drn.ExternalVariableDirectory + "/config",
			expected: map[str.DRN]str.DRNVal{
				str.DRN(drn.QuickFormat([]string{"config"}, "server", "host")): "127.0.0.1",
				str.DRN(drn.QuickFormat([]string{"config"}, "server", "port")): "8080",
			},
		},
		{
			name: "Sub-namespace path",
			node: MapToCfgNode(map[string]string{"key": "val"}),
			path: drn.ExternalVariableDirectory + "/sub/dir/config.json",
			expected: map[str.DRN]str.DRNVal{
				str.DRN(drn.QuickFormat([]string{"sub", "dir", "config.json"}, "key")): "val",
			},
		},
		{
			name:        "Path without _global returns error",
			node:        MapToCfgNode(map[string]string{"key": "val"}),
			path:        "config/key",
			expectError: drn.ExternalVariableDirectory,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := test.node.FormatAll(test.path)

			if test.expectError != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", test.expectError)
				}
				if !strings.Contains(err.Error(), test.expectError) {
					t.Errorf("error = %q, expected to contain %q", err.Error(), test.expectError)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// nil node should return nil map
			if test.node == nil {
				if got != nil {
					t.Errorf("expected nil map for nil node, got %v", got)
				}
				return
			}

			// Empty node should return empty map
			if len(test.node) == 0 {
				if got == nil {
					t.Errorf("expected non-nil map for empty node")
				} else if len(got) != 0 {
					t.Errorf("expected empty map for empty node, got %v", got)
				}
				return
			}

			if len(got) != len(test.expected) {
				t.Fatalf("got %d keys, expected %d: %#v", len(got), len(test.expected), got)
			}

			for k, v := range test.expected {
				gotVal, ok := got[k]
				if !ok {
					t.Errorf("key %q not found in result", k)
					continue
				}
				if gotVal != v {
					t.Errorf("got[%q] = %q, expected %q", k, gotVal, v)
				}
			}
		})
	}
}
