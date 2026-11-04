// Package updatecheck tells someone running an outdated uptimyctl that a
// newer release exists, so they update before an API change breaks them. The
// latest version is cached in the config directory and looked up at most once
// a day, so almost every command reads a file and makes no request.
package updatecheck

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/uptimy/uptimyctl/internal/selfupdate"
)

// Interval is how long a looked-up latest version is trusted.
const Interval = 24 * time.Hour

// timeout bounds the lookup, which the command waits for before exiting.
const timeout = 2 * time.Second

type state struct {
	CheckedAt time.Time `json:"checkedAt"`
	Latest    string    `json:"latest"`
}

// Notice returns the line to show after a command run with version current,
// or "" when it's the latest. path is the cache file; latest looks up the
// latest release's version.
func Notice(ctx context.Context, path, current string, now time.Time, latest func(context.Context) (string, error)) string {
	st := load(path)
	if now.Sub(st.CheckedAt) >= Interval {
		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		if v, err := latest(ctx); err == nil {
			st.Latest = v
		}
		// A failed lookup also waits a day, so an offline or rate-limited
		// machine doesn't wait for the timeout on every command.
		st.CheckedAt = now
		save(path, st)
	}
	if !selfupdate.Newer(st.Latest, current) {
		return ""
	}
	return fmt.Sprintf("uptimyctl %s is available (you have %s). Run 'uptimyctl update'.", st.Latest, current)
}

func load(path string) state {
	var st state
	if b, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(b, &st)
	}
	return st
}

// save is best effort: without a writable cache the notice still works, it
// just looks up more often.
func save(path string, st state) {
	b, err := json.Marshal(st)
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return
	}
	_ = os.WriteFile(path, b, 0o600)
}
