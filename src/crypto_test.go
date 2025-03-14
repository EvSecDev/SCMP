// controller
package main

import (
	"testing"
)

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
			hash := SHA256Sum([]byte(test.input))
			if hash != test.expected {
				t.Errorf("SHA256Sum(%s) = %s, want %s", test.input, hash, test.expected)
			}
		})
	}
}
