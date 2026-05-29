package api

import (
	"context"
	"scmp/internal/config"
	"scmp/internal/global"
	"scmp/internal/tests/utils"
	"testing"
)

func TestParseEndpointAddress(t *testing.T) {
	ctx := t.Context()

	tests := []struct {
		name          string
		repoPath      string
		input         string
		expected      string
		expectedError string
	}{
		{
			name:     "Safe path",
			repoPath: "/home/user/myrepo",
			input:    "Host1/etc/file",
			expected: "Host1/etc/file",
		},
		{
			name:     "Root of repository",
			repoPath: "/home/user/myrepo",
			input:    "",
			expected: ".",
		},
		{
			name:          "Missing config",
			repoPath:      "",
			input:         "/etc/passwd",
			expectedError: "repository path not set",
		},
		{
			name:          "Invalid path with ..",
			repoPath:      "/home/user/myrepo",
			input:         "Host1/../etc/file",
			expectedError: "illegal path",
		},
		{
			name:          "Path to .git",
			repoPath:      "/home/user/myrepo",
			input:         ".git/config",
			expectedError: "illegal file path",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := config.Config{
				RepositoryPath: test.repoPath,
			}
			ctx = context.WithValue(ctx, global.ConfKey, config)

			result, err := validateRequestedFilePath(ctx, test.input)
			matches, err := utils.MatchErrorString(err, test.expectedError)
			if err != nil {
				t.Fatalf("%v", err)
			} else if matches {
				return
			}

			if result != test.expected {
				t.Errorf("Expected clean path '%s', but got '%s'", test.expected, result)
			}
		})
	}
}
