package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/uptimy/uptimyctl/internal/config"
	"github.com/uptimy/uptimyctl/internal/output"
	"github.com/uptimy/uptimyctl/internal/selfupdate"
	"github.com/uptimy/uptimyctl/internal/updatecheck"
	"github.com/uptimy/uptimyctl/internal/version"
)

// updateNotice gets this run's "newer release" line, looked up while the
// command runs; nil when the notice is off.
var updateNotice chan string

// startUpdateCheck starts the update check for someone at a terminal. It's off
// for -o json, CI, pipes, source builds, UPTIMYCTL_NO_UPDATE_CHECK and
// 'update' itself, so scripts and agents never see it.
func startUpdateCheck(cmd *cobra.Command) {
	path := config.UpdateCheckPath()
	if path == "" || version.Version == "dev" || output.IsJSON() || cmd == updateCmd ||
		os.Getenv("CI") != "" || os.Getenv("UPTIMYCTL_NO_UPDATE_CHECK") != "" || !isTerminal(os.Stderr) {
		return
	}
	updateNotice = make(chan string, 1)
	go func() {
		updateNotice <- updatecheck.Notice(context.Background(), path, version.Version, time.Now(), func(ctx context.Context) (string, error) {
			r, err := selfupdate.New().Latest(ctx)
			if err != nil {
				return "", err
			}
			return r.Version, nil
		})
	}()
}

// printUpdateNotice waits for the check (at most its lookup timeout, once a
// day) and prints its line to stderr, after the command's own output.
func printUpdateNotice() {
	if updateNotice == nil {
		return
	}
	if msg := <-updateNotice; msg != "" {
		fmt.Fprintf(os.Stderr, "\n%s\n", msg)
	}
	updateNotice = nil
}

// exit ends a failed command. It prints the update notice first: a command
// failing on an API change is when an outdated uptimyctl most needs to say so.
func exit(code int) {
	printUpdateNotice()
	os.Exit(code)
}

func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}
