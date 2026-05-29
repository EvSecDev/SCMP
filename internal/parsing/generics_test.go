package parsing

import "testing"

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		input    int
		expected string
	}{
		// Edge case: 0 bytes
		{0, "0 Bytes"},

		// Small number of bytes
		{500, "500.00 Bytes"},

		// Kilobyte values
		{1024, "1.00 KiB"},
		{2048, "2.00 KiB"},
		{5000, "4.88 KiB"},

		// Megabyte values
		{1048576, "1.00 MiB"},
		{2097152, "2.00 MiB"},
		{5000000, "4.77 MiB"},

		// Gigabyte values
		{1073741824, "1.00 GiB"},
		{2147483648, "2.00 GiB"},
		{5000000000, "4.66 GiB"},

		// Handling the highest unit in the list
		{9223372036854775807, "8192.00 PiB"}, // A very large number

		// Testing the upper bound of the units
		{1099511627776, "1.00 TiB"}, // This should return 1 TiB as the value
	}

	for _, test := range tests {
		t.Run(test.expected, func(t *testing.T) {
			result := FormatBytes(test.input)
			if result != test.expected {
				t.Errorf("For input %d, expected %s but got %s", test.input, test.expected, result)
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
		{"0120000", "unsupported"}, // Special
		{"0040000", "unsupported"}, // Directory
		{"0160000", "unsupported"}, // Git submodule
		{"0100755", "regular"},     // Executable
		{"0100664", "unsupported"}, // Deprecated
		{"0", "unsupported"},       // Empty (no file)
		{"", "unsupported"},        // Empty string
		{"unknown", "unsupported"}, // Unknown - don't process
	}

	for _, test := range tests {
		t.Run(test.fileMode, func(t *testing.T) {
			result := DetermineFileType(test.fileMode)
			if result != test.expected {
				t.Errorf("determineFileType(%s) = %s; want %s", test.fileMode, result, test.expected)
			}
		})
	}
}
