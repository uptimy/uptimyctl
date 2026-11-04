package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/uptimy/uptimyctl/internal/output"
	"github.com/uptimy/uptimyctl/internal/selfupdate"
	"github.com/uptimy/uptimyctl/internal/version"
)

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update uptimyctl to the latest release",
	Long: `Downloads the latest uptimyctl release from GitHub for this platform, checks
it against the release's checksums and replaces this binary.

The install script puts uptimyctl in /usr/local/bin, which needs root to
write: there, run 'sudo uptimyctl update'.

--check only reports whether a newer release exists.`,
	Example: `  uptimyctl update
  sudo uptimyctl update
  uptimyctl update --check -o json`,
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		check, _ := cmd.Flags().GetBool("check")
		u := selfupdate.New()
		latest, err := u.Latest(cmd.Context())
		if err != nil {
			exitErr(err)
		}
		current := version.Version
		available := current == "dev" || selfupdate.Newer(latest.Version, current)

		if check || !available {
			if output.IsJSON() {
				output.PrintJSON(map[string]any{"current": current, "latest": latest.Version, "updateAvailable": available, "updated": false})
			} else if available {
				fmt.Printf("uptimyctl %s is available (you have %s). Run 'uptimyctl update'.\n", latest.Version, current)
			} else {
				fmt.Printf("uptimyctl %s is the latest release.\n", current)
			}
			return
		}

		if current == "dev" {
			exitErr(errors.New("this uptimyctl was built from source (version dev), so it can't tell which release it is: update it the way you built it, or reinstall with scripts/install.sh"))
		}
		exe, err := os.Executable()
		if err == nil {
			exe, err = filepath.EvalSymlinks(exe)
		}
		if err != nil {
			exitErr(fmt.Errorf("find this binary: %w", err))
		}
		if err := u.Install(cmd.Context(), latest, exe); err != nil {
			var perm *selfupdate.PermissionError
			if errors.As(err, &perm) {
				exitErr(fmt.Errorf("can't replace %s: %v. Run 'sudo uptimyctl update'", exe, perm))
			}
			exitErr(err)
		}

		if output.IsJSON() {
			output.PrintJSON(map[string]any{"current": current, "latest": latest.Version, "updateAvailable": true, "updated": true})
			return
		}
		fmt.Printf("Updated uptimyctl %s → %s (%s)\nChanges: %s\n", current, latest.Version, exe, latest.URL)
	},
}

func init() {
	updateCmd.Flags().Bool("check", false, "Only report whether a newer release exists")
	rootCmd.AddCommand(updateCmd)
}
