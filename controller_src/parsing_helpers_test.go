// controller
package main

import (
	"fmt"
	"testing"
)

// Unit test for checkForOverride
func TestCheckForOverride(t *testing.T) {
	tests := []struct {
		override     string
		current      string
		expectedSkip bool
	}{
		{"", "host1", false},
		{"host1", "host1", false},
		{"host1,host2", "host1", false},
		{"host1,host2", "host3", true},
		{"host1, host2", "host3", true},
		{"host1, host2, host3, host4, host5, host6", "host3", true},
		{"file1.txt,file2.txt", "file1.txt", false},
		{"file1.txt,file2.txt", "file3.txt", true},
		{"file!@%$^&*(4.txt,file6.txt", "file6.txt", false},
		{"file!@%$^&*(4.txt,file6.txt", "file!@%$^&*(4.txt", false},
	}

	for _, test := range tests {
		testTitle := fmt.Sprintf("Available Items:'%s'-Current Item:'%s'", test.override, test.current)
		t.Run(testTitle, func(t *testing.T) {
			skip := checkForOverride(test.override, test.current)
			if skip != test.expectedSkip {
				t.Errorf("Skip current item? %v; Should skip current item? %v", skip, test.expectedSkip)
			}
		})
	}
}

func TestDetermineFileType(t *testing.T) {
	tests := []struct {
		fileMode string
		expected string
	}{
		{"0100644", "regular"},     // Text file
		{"0120000", "symlink"},     // Special, but able to be handled
		{"0040000", "unsupported"}, // Directory
		{"0160000", "unsupported"}, // Git submodule
		{"0100755", "unsupported"}, // Executable
		{"0100664", "unsupported"}, // Deprecated
		{"0", "unsupported"},       // Empty (no file)
		{"", "unsupported"},        // Empty string
		{"unknown", "unsupported"}, // Unknown - don't process
	}

	for _, tt := range tests {
		t.Run(tt.fileMode, func(t *testing.T) {
			result := determineFileType(tt.fileMode)
			if result != tt.expected {
				t.Errorf("determineFileType(%v) = %v; want %v", tt.fileMode, result, tt.expected)
			}
		})
	}
}

func TestSeparateHostDirFromPath(t *testing.T) {
	tests := []struct {
		localRepoPath    string
		expectedHostDir  string
		expectedFilePath string
	}{
		{"host/dir/file.txt", "host", "/dir/file.txt"},
		{"host2/dir/subdir/file.txt", "host2", "/dir/subdir/file.txt"},
		{"file1.txt", "", ""},
		{"", "", ""},
		{"/home/user/repo/host1/file", "", "/home/user/repo/host1/file"},
		{"!@#$%^&*()_+/etc/file", "!@#$%^&*()_+", "/etc/file"},
	}

	for _, test := range tests {
		t.Run(test.localRepoPath, func(t *testing.T) {
			hostDir, targetFilePath := separateHostDirFromPath(test.localRepoPath)
			if hostDir != test.expectedHostDir {
				t.Errorf("expected hostDir '%v', got '%v'", test.expectedHostDir, hostDir)
			}
			if targetFilePath != test.expectedFilePath {
				t.Errorf("expected targetFilePath '%v', got '%v'", test.expectedFilePath, targetFilePath)
			}
		})
	}
}

func TestSHA256Sum(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"abc", "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"},
		{"", "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"},
		{"abcdbcdecdefdefgefghfghighijhijkijkljklmklmnlmnomnopnopq", "248d6a61d20638b8e5c026930c3e6039a33ce45964ff2167f6ecedd419db06c1"},
		{"abcdefghbcdefghicdefghijdefghijkefghijklfghijklmghijklmnhijklmnoijklmnopjklmnopqklmnopqrlmnopqrsmnopqrstnopqrstu", "cf5b16a778af8380036ce59e7b0492370b249b11e8f07a51afac45037afee9d1"},
		{"!@#$%^&*()_+1234567890", "01cb750d216a2f11937c113cbfe06f01886adfd11f2de1c83891fe5d0f44ff23"},
	}

	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			hash := SHA256Sum(test.input)
			if hash != test.expected {
				t.Errorf("SHA256Sum(%v) = %v, want %v", test.input, hash, test.expected)
			}
		})
	}
}
