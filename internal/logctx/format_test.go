package logctx

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestEventFormat(t *testing.T) {
	ts := time.Date(2026, 1, 31, 12, 34, 56, 123456789, time.UTC)
	tests := []struct {
		name   string
		event  Event
		expect string
	}{
		{
			name: "all fields",
			event: Event{
				Timestamp: ts,
				Severity:  "Info",
				Tags:      []string{"tag1", "tag2"},
				Message:   "hello world",
			},
			expect: "hello world",
		},
		{
			name: "all fields except message",
			event: Event{
				Timestamp: ts,
				Severity:  "Info",
				Tags:      []string{"tag1", "tag2"},
				Message:   "",
			},
		},
		{
			name: "no tags",
			event: Event{
				Timestamp: ts,
				Severity:  "Warn",
				Message:   "something happened",
			},
			expect: fmt.Sprintf("[%s] Warn: something happened", padTimestamp(ts)),
		},
		{
			name: "no severity",
			event: Event{
				Timestamp: ts,
				Tags:      []string{"tag"},
				Message:   "just a message",
			},
			expect: fmt.Sprintf("[%s] [tag] just a message", padTimestamp(ts)),
		},
		{
			name: "only message",
			event: Event{
				Message: "bare message",
			},
			expect: "bare message",
		},
		{
			name:   "empty event",
			event:  Event{},
			expect: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.event.Format(VerbosityStandard)
			if got != tt.expect {
				t.Errorf("\ngot  %q\nwant %q", got, tt.expect)
			}
		})
	}
}

func TestPadTimestamp(t *testing.T) {
	tests := []struct {
		name      string
		input     time.Time
		expected  string
		expectEnd string
	}{
		{
			name:      "normal timestamp",
			input:     time.Date(2026, 1, 31, 12, 34, 56, 123456, time.UTC),
			expected:  "2026-01-31T12:34:56.123456000Z",
			expectEnd: "Z",
		},
		{
			name:      "normal timestamp short nanoseconds",
			input:     time.Date(2026, 1, 31, 12, 34, 56, 4832, time.UTC),
			expected:  "2026-01-31T12:34:56.000004832Z",
			expectEnd: "Z",
		},
		{
			name:      "normal timestamp short nanoseconds other",
			input:     time.Date(2026, 1, 31, 12, 34, 56, 7, time.UTC),
			expected:  "2026-01-31T12:34:56.000000007Z",
			expectEnd: "Z",
		},
		{
			name:      "full nanoseconds with timezone offset",
			input:     time.Date(2026, 1, 31, 12, 34, 56, 987654321, time.FixedZone("UTC+2", 2*3600)),
			expected:  "2026-01-31T12:34:56.987654321+02:00",
			expectEnd: "+02:00",
		},
		{
			name:      "partial nanoseconds with timezone offset",
			input:     time.Date(2026, 1, 31, 12, 34, 56, 765, time.FixedZone("UTC-8", -8*3600)),
			expected:  "2026-01-31T12:34:56.000000765-08:00",
			expectEnd: "-08:00",
		},
		{
			name:      "zero nanoseconds",
			input:     time.Date(2026, 1, 31, 12, 34, 56, 0, time.UTC),
			expected:  "2026-01-31T12:34:56.000000000Z",
			expectEnd: "Z",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expectLen := len(tt.expected)

			got := padTimestamp(tt.input)
			if len(got) != expectLen {
				t.Errorf("\nlength:\n got  %d\n want %d\ncontent:\n got  %q\n want %q", len(got), expectLen, got, tt.expected)
			}
			if !strings.HasSuffix(got, tt.expectEnd) {
				t.Errorf("\nsuffix:\n got  %q\n want %q", got, tt.expectEnd)
			}
		})
		t.Run(tt.name+"_roundtrip", func(t *testing.T) {
			got := padTimestamp(tt.input)

			parsed, err := time.Parse(time.RFC3339Nano, got)
			if err != nil {
				t.Fatalf("failed to parse formatted timestamp: %s", err.Error())
			}

			if !parsed.Equal(tt.input) {
				t.Errorf("roundtrip mismatch:\n got  %v\n want %v", parsed, tt.input)
			}
		})
		t.Run(tt.name+"_nanoseconds_preserved", func(t *testing.T) {
			got := padTimestamp(tt.input)

			parsed, _ := time.Parse(time.RFC3339Nano, got)

			if parsed.Nanosecond() != tt.input.Nanosecond() {
				t.Errorf("nanoseconds changed:\n got  %d\n want %d",
					parsed.Nanosecond(),
					tt.input.Nanosecond())
			}
		})
		t.Run(tt.name+"_fraction_width", func(t *testing.T) {
			got := padTimestamp(tt.input)

			parts := strings.Split(got, ".")
			if len(parts) < 2 {
				t.Fatalf("missing fractional part: %s", got)
			}

			frac := parts[1]
			frac = strings.TrimSuffix(frac, tt.expectEnd)

			if len(frac) != 9 {
				t.Errorf("fraction not 9 digits:\n got %q (len=%d)", frac, len(frac))
			}
		})
	}
}
