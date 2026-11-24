package cmd

import (
	"testing"
	"time"
)

func TestParseSince(t *testing.T) {
	now := time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)
	cases := map[string]string{
		"":                          "",
		"2026-11-01T00:00:00Z":      "2026-11-01T00:00:00Z",
		"2026-11-01T01:00:00+01:00": "2026-11-01T00:00:00Z",
		"7d":                        "2026-11-17T12:00:00Z",
		"12h":                       "2026-11-24T00:00:00Z",
		"30m":                       "2026-11-24T11:30:00Z",
	}
	for in, want := range cases {
		got, err := parseSince(in, now)
		if err != nil || got != want {
			t.Errorf("parseSince(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"yesterday", "0d", "-2h", "7w", "2026-11-01"} {
		if got, err := parseSince(bad, now); err == nil {
			t.Errorf("parseSince(%q) = %q, want an error", bad, got)
		}
	}
}

func TestSeriesStep(t *testing.T) {
	for span, want := range map[time.Duration]string{
		2 * time.Hour:        "1m",
		6 * time.Hour:        "1m",
		24 * time.Hour:       "1h",
		30 * 24 * time.Hour:  "1h", // 720 buckets
		90 * 24 * time.Hour:  "1d",
		400 * 24 * time.Hour: "1d",
	} {
		if got := seriesStep(span); got != want {
			t.Errorf("seriesStep(%s) = %s, want %s", span, got, want)
		}
	}
}
