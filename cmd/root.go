package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/uptimy/uptimyctl/internal/client"
	"github.com/uptimy/uptimyctl/internal/config"
	"github.com/uptimy/uptimyctl/internal/output"
)

var (
	flagAPIKey string
	flagAPIURL string
	flagOutput string
)

var rootCmd = &cobra.Command{
	Use:   "uptimyctl",
	Short: "CLI for the upti.my monitoring platform",
	Long:  "uptimyctl is a command-line tool for managing your upti.my workspace — incidents, maintenances, healthchecks, and more.",
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&flagAPIKey, "api-key", "", "API key (overrides config file and env)")
	rootCmd.PersistentFlags().StringVar(&flagAPIURL, "api-url", "", "API base URL (overrides config file and env)")
	rootCmd.PersistentFlags().StringVarP(&flagOutput, "output", "o", "table", "Output format: table, json")

	rootCmd.PersistentPreRun = func(cmd *cobra.Command, args []string) {
		output.Format = flagOutput
	}
}

func newClient() *client.Client {
	apiKey := config.GetAPIKey(flagAPIKey)
	apiURL := config.GetAPIURL(flagAPIURL)
	if apiKey == "" {
		fmt.Fprintln(os.Stderr, "Error: no API key configured. Run 'uptimyctl auth login' or set --api-key.")
		os.Exit(1)
	}
	return client.New(apiURL, apiKey)
}

func exitErr(err error) {
	fmt.Fprintf(os.Stderr, "Error: %v\n", err)
	os.Exit(1)
}
