package updatecheck

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

var now = time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)

// fakeLatest answers with version (or err) and counts lookups.
type fakeLatest struct {
	version string
	err     error
	calls   int
}

func (f *fakeLatest) lookup(context.Context) (string, error) {
	f.calls++
	return f.version, f.err
}

func cachePath(t *testing.T) string {
	return filepath.Join(t.TempDir(), "uptimyctl", "update-check.json")
}

func TestNoticeLooksUpOnceADay(t *testing.T) {
	path := cachePath(t)
	gh := &fakeLatest{version: "1.9.0"}
	want := "uptimyctl 1.9.0 is available (you have 1.8.0). Run 'uptimyctl update'."

	if got := Notice(context.Background(), path, "1.8.0", now, gh.lookup); got != want {
		t.Fatalf("first run: %q", got)
	}
	// Later the same day the cache answers, and the notice shows on every run.
	if got := Notice(context.Background(), path, "1.8.0", now.Add(23*time.Hour), gh.lookup); got != want {
		t.Fatalf("cached run: %q", got)
	}
	if gh.calls != 1 {
		t.Fatalf("%d lookups, want 1", gh.calls)
	}
	// A day later it looks up again.
	gh.version = "1.10.0"
	if got := Notice(context.Background(), path, "1.8.0", now.Add(24*time.Hour), gh.lookup); got != "uptimyctl 1.10.0 is available (you have 1.8.0). Run 'uptimyctl update'." {
		t.Fatalf("next day: %q", got)
	}
	if gh.calls != 2 {
		t.Fatalf("%d lookups, want 2", gh.calls)
	}
}

func TestNoNoticeOnTheLatestOrANewerVersion(t *testing.T) {
	gh := &fakeLatest{version: "1.9.0"}
	for _, current := range []string{"1.9.0", "1.10.0", "dev"} {
		if got := Notice(context.Background(), cachePath(t), current, now, gh.lookup); got != "" {
			t.Errorf("%s: %q", current, got)
		}
	}
}

func TestFailedLookupKeepsWhatItKnewAndWaitsADay(t *testing.T) {
	path := cachePath(t)
	gh := &fakeLatest{version: "1.9.0"}
	Notice(context.Background(), path, "1.8.0", now, gh.lookup)

	gh.err = errors.New("rate limited")
	if got := Notice(context.Background(), path, "1.8.0", now.Add(25*time.Hour), gh.lookup); got == "" {
		t.Fatal("a failed lookup dropped the known newer version")
	}
	Notice(context.Background(), path, "1.8.0", now.Add(26*time.Hour), gh.lookup)
	if gh.calls != 2 {
		t.Fatalf("%d lookups, want 2 (no retry within the day)", gh.calls)
	}
}

func TestOfflineFirstRunShowsNothing(t *testing.T) {
	gh := &fakeLatest{err: errors.New("offline")}
	if got := Notice(context.Background(), cachePath(t), "1.8.0", now, gh.lookup); got != "" {
		t.Fatalf("%q", got)
	}
}
