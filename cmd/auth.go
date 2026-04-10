package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/uptimy/uptimyctl/internal/client"
	"github.com/uptimy/uptimyctl/internal/config"
)

var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "Manage authentication",
}

var authLoginCmd = &cobra.Command{
	Use:   "login",
	Short: "Save your API key to the local config",
	Run: func(cmd *cobra.Command, args []string) {
		reader := bufio.NewReader(os.Stdin)

		cfg := config.Load()

		// Only prompt for API URL if --api-url flag was explicitly passed
		if cmd.Flags().Changed("api-url") {
			cfg.APIURL = flagAPIURL
		} else if cfg.APIURL == "" {
			cfg.APIURL = config.DefaultAPIURL
		}

		fmt.Print("API Key: ")
		keyInput, _ := reader.ReadString('\n')
		keyInput = strings.TrimSpace(keyInput)
		if keyInput == "" {
			fmt.Fprintln(os.Stderr, "Error: API key is required.")
			os.Exit(1)
		}
		if !strings.HasPrefix(keyInput, "upt_") {
			fmt.Fprintln(os.Stderr, "Error: API key must start with 'upt_'.")
			os.Exit(1)
		}
		cfg.APIKey = keyInput

		// Verify the key works
		c := client.New(cfg.APIURL, cfg.APIKey)
		_, err := c.Get("/v1/api/applications/", nil)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to verify API key: %v\n", err)
			os.Exit(1)
		}

		if err := config.Save(cfg); err != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to save config: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("Authenticated successfully. Config saved.")
	},
}

var authStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show current authentication status",
	Run: func(cmd *cobra.Command, args []string) {
		apiKey := config.GetAPIKey(flagAPIKey)
		apiURL := config.GetAPIURL(flagAPIURL)

		if apiKey == "" {
			fmt.Println("Not authenticated. Run 'uptimyctl auth login' to configure.")
			return
		}

		masked := apiKey[:8] + strings.Repeat("•", 8)
		fmt.Printf("API URL: %s\n", apiURL)
		fmt.Printf("API Key: %s\n", masked)

		c := client.New(apiURL, apiKey)
		_, err := c.Get("/v1/api/applications/", nil)
		if err != nil {
			fmt.Printf("Status:  Invalid (%v)\n", err)
		} else {
			fmt.Println("Status:  Valid")
		}
	},
}

func init() {
	authCmd.AddCommand(authLoginCmd)
	authCmd.AddCommand(authStatusCmd)
	rootCmd.AddCommand(authCmd)
}
