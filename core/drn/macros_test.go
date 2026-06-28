package drn

import (
	"reflect"
	"scmp/internal/config"
	"scmp/internal/str"
	"slices"
	"strings"
	"testing"
)

func TestExtractMacros(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect []string
	}{
		{
			name:   "Empty string returns no macros",
			input:  "",
			expect: nil,
		},
		{
			name:   "No macros returns no macros",
			input:  "repo",
			expect: nil,
		},
		{
			name:   "Normal text with special chars returns no macros",
			input:  "field_name-with-digits123",
			expect: nil,
		},
		{
			name:   "Single macro",
			input:  "{{FILENAME}}",
			expect: []string{"{{FILENAME}}"},
		},
		{
			name:   "Single macro with surrounding text",
			input:  "prefix{{MACRO}}suffix",
			expect: []string{"{{MACRO}}"},
		},
		{
			name:   "Two macros in same string",
			input:  "{{A}}{{B}}",
			expect: []string{"{{A}}", "{{B}}"},
		},
		{
			name:   "Two macros with text between",
			input:  "{{A}} text {{B}}",
			expect: []string{"{{A}}", "{{B}}"},
		},
		{
			name:   "Three macros",
			input:  "{{X}} and {{Y}} and {{Z}}",
			expect: []string{"{{X}}", "{{Y}}", "{{Z}}"},
		},
		{
			name:   "Macro name with uppercase and digits",
			input:  "{{HOST123ADDRESS}}",
			expect: []string{"{{HOST123ADDRESS}}"},
		},
		{
			name:   "Unclosed macro returns partial",
			input:  "{{MACRO",
			expect: []string{"{{MACRO"},
		},
		{
			name:   "Closed first then unclosed",
			input:  "{{A}} then {{B",
			expect: []string{"{{A}}", "{{B"},
		},
		{
			name:   "Known internal macro names",
			input:  "{{FILENAME}} and {{HOSTALIAS}} and {{FILEPATH}}",
			expect: []string{"{{FILENAME}}", "{{HOSTALIAS}}", "{{FILEPATH}}"},
		},
		{
			name:   "Unclosed macro drops well-formed macros that follow",
			input:  "{{UNCLOSED then {{VALID}}",
			expect: []string{"{{VALID}}"},
		},
		{
			name:   "Empty macro name is silently dropped",
			input:  "{{}}{{A}}",
			expect: []string{"{{A}}"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := ExtractMacros(test.input)
			if !reflect.DeepEqual(got, test.expect) {
				t.Errorf("input=%q - got: %#v, expected %#v", test.input, got, test.expect)
			}
		})
	}
}

func TestContainsMacro(t *testing.T) {
	tests := []struct {
		name  string
		input str.DRN
		want  bool
	}{
		{
			name:  "no macro returns false",
			input: str.DRN(QuickFormat([]string{"repo"}, "field")),
			want:  false,
		},
		{
			name:  "host macro returns true",
			input: str.DRN(QuickFormat([]string{"{{HOSTALIAS}}"}, "field")),
			want:  true,
		},
		{
			name:  "file macro returns true",
			input: str.DRN(QuickFormat([]string{"repo"}, "field", "{{FILENAME}}")),
			want:  true,
		},
		{
			name:  "macro open alone not enough",
			input: str.DRN(QuickFormat([]string{"repo"}, "{")),
			want:  false,
		},
		{
			name:  "empty string",
			input: "",
			want:  false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ContainsMacro(tt.input)
			if got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsHostMacro(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{name: "HOSTALIAS is host macro", input: "{{HOSTALIAS}}", want: true},
		{name: "HOSTADDRESS is host macro", input: "{{HOSTADDRESS}}", want: true},
		{name: "HOSTLOGINUSER is host macro", input: "{{HOSTLOGINUSER}}", want: true},
		{name: "FILENAME is not host macro", input: "{{FILENAME}}", want: false},
		{name: "non-existent macro", input: "{{BOGUS}}", want: false},
		{name: "raw text", input: "hello", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsHostMacro(tt.input); got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsFileMacro(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{name: "FILENAME is file macro", input: "{{FILENAME}}", want: true},
		{name: "FILEPATH is file macro", input: "{{FILEPATH}}", want: true},
		{name: "FILEDIR is file macro", input: "{{FILEDIR}}", want: true},
		{name: "REPOBASEDIR is file macro", input: "{{REPOBASEDIR}}", want: true},
		{name: "HOSTALIAS is not file macro", input: "{{HOSTALIAS}}", want: false},
		{name: "non-existent macro", input: "{{BOGUS}}", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsFileMacro(tt.input); got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetAllHostMacros(t *testing.T) {
	macros := GetAllHostMacros()

	if len(macros) == 0 {
		t.Fatal("GetAllHostMacros returned empty slice")
	}
	for _, macro := range macros {
		if !IsHostMacro(macro) {
			t.Errorf("GetAllHostMacros includes %q which is not a host macro", macro)
		}
	}
	if !slices.IsSorted(macros) {
		t.Error("GetAllHostMacros should return sorted slice")
	}
}

func TestGetAllFileMacros(t *testing.T) {
	macros := GetAllFileMacros()

	if len(macros) == 0 {
		t.Fatal("GetAllFileMacros returned empty slice")
	}
	for _, macro := range macros {
		if !IsFileMacro(macro) {
			t.Errorf("GetAllFileMacros includes %q which is not a file macro", macro)
		}
	}
	if !slices.IsSorted(macros) {
		t.Error("GetAllFileMacros should return sorted slice")
	}
}

func TestGetAllInternalDRNs(t *testing.T) {
	drns := GetAllInternalDRNs()

	if len(drns) == 0 {
		t.Fatal("GetAllInternalDRNs returned empty slice")
	}
	for _, drn := range drns {
		_, valid := InternalDRNToMacroName(drn)
		if !valid {
			t.Errorf("GetAllInternalDRNs includes %q which is not a valid internal DRN", drn)
		}
	}
	if !slices.IsSorted(drns) {
		t.Error("GetAllInternalDRNs should return sorted slice")
	}
}

func TestInternalDRNToMacroName(t *testing.T) {
	tests := []struct {
		name      string
		input     str.DRN
		wantName  string
		wantValid bool
	}{
		{
			name:      "HOSTALIAS internal DRN",
			input:     str.DRN(QuickFormat([]string{InternalNamespacePrefix}, "host", "alias")),
			wantName:  "{{HOSTALIAS}}",
			wantValid: true,
		},
		{
			name:      "FILENAME internal DRN",
			input:     str.DRN(QuickFormat([]string{InternalNamespacePrefix}, "repo", "file", "name")),
			wantName:  "{{FILENAME}}",
			wantValid: true,
		},
		{
			name:      "non-internal DRN returns invalid",
			input:     str.DRN(QuickFormat([]string{"repo"}, "field")),
			wantName:  "",
			wantValid: false,
		},
		{
			name:      "empty DRN returns invalid",
			input:     "",
			wantName:  "",
			wantValid: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotName, gotValid := InternalDRNToMacroName(tt.input)
			if gotName != tt.wantName || gotValid != tt.wantValid {
				t.Errorf("got (%q, %v), want (%q, %v)",
					gotName, gotValid, tt.wantName, tt.wantValid)
			}
		})
	}
}

func TestNewMacroReplacer(t *testing.T) {
	repoPath := "/repos/myproject"
	filePath := str.LocalRepoPath("myhost/etc/app.conf")
	hostInfo := config.EndpointInfo{
		EndpointName: "myhost",
		Endpoint:     "10.0.0.1:22",
		EndpointUser: "admin",
	}

	fileReplacer, hostReplacer, err := NewMacroReplacer(repoPath, hostInfo, filePath)
	if err != nil {
		t.Fatalf("replacer returned error: %v", err)
	}
	if fileReplacer == nil {
		t.Fatal("expected non-nil fileReplacer")
	}
	if hostReplacer == nil {
		t.Fatal("expected non-nil hostReplacer")
	}

	replacer := strings.NewReplacer(
		"{{HOSTALIAS}}", hostReplacer.Replace("{{HOSTALIAS}}"),
		"{{HOSTADDRESS}}", hostReplacer.Replace("{{HOSTADDRESS}}"),
		"{{HOSTLOGINUSER}}", hostReplacer.Replace("{{HOSTLOGINUSER}}"),
		"{{FILENAME}}", fileReplacer.Replace("{{FILENAME}}"),
		"{{FILEPATH}}", fileReplacer.Replace("{{FILEPATH}}"),
		"{{FILEDIR}}", fileReplacer.Replace("{{FILEDIR}}"),
		"{{REPOBASEDIR}}", fileReplacer.Replace("{{REPOBASEDIR}}"),
	)

	tests := []struct {
		input string
		want  string
	}{
		{input: "{{HOSTALIAS}}", want: "myhost"},
		{input: "{{HOSTLOGINUSER}}", want: "admin"},
		{input: "{{FILENAME}}", want: "app.conf"},
		{input: "{{FILEPATH}}", want: "/etc/app.conf"},
		{input: "{{FILEDIR}}", want: "/etc"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := replacer.Replace(tt.input)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}
