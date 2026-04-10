package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/uptimy/uptimyctl/internal/version"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version of uptimyctl",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("uptimyctl %s (commit: %s, built: %s)\n", version.Version, version.Commit, version.BuildDate)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
