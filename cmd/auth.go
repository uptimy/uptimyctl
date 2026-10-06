package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/uptimy/uptimyctl/internal/client"
	"github.com/uptimy/uptimyctl/internal/config"
	"github.com/uptimy/uptimyctl/internal/output"
)

var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "Manage authentication",
}

var authLoginCmd = &cobra.Command{
	Use:   "login",
	Short: "Authorize uptimyctl in your browser and save the API key",
	Long: `Opens the Uptimy app in your browser, where you pick a workspace and approve
access. The browser returns a one-time code to uptimyctl, which exchanges it
(OAuth with PKCE) for an API key for this machine, valid for 90 days.

Over SSH or on a headless machine, use --no-browser: open the printed URL on any
device and paste the code it shows. In scripts, pass --api-key or pipe the key
on stdin.`,
	Example: `  uptimyctl auth login
  uptimyctl auth login --no-browser
  echo "$UPTIMY_KEY" | uptimyctl auth login`,
	Run: func(cmd *cobra.Command, args []string) {
		reader := bufio.NewReader(os.Stdin)

		cfg := config.Load()

		// Only prompt for API URL if --api-url flag was explicitly passed
		if cmd.Flags().Changed("api-url") {
			cfg.APIURL = flagAPIURL
		} else if cfg.APIURL == "" {
			cfg.APIURL = config.DefaultAPIURL
		}

		if cmd.Flags().Changed("incidents-api-url") {
			cfg.IncidentsAPIURL = flagIncidentsAPIURL
		} else if cfg.IncidentsAPIURL == "" {
			cfg.IncidentsAPIURL = config.DefaultIncidentsAPIURL
		}

		if cmd.Flags().Changed("heartbeats-api-url") {
			cfg.HeartbeatsURL = flagHeartbeatsURL
		} else if cfg.HeartbeatsURL == "" {
			cfg.HeartbeatsURL = config.DefaultHeartbeatsURL
		}

		keyInput, err := readLoginKey(cmd, reader, cfg.APIURL)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
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
		_, err = c.Get("/v1/api/applications/", nil)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to verify API key: %v\n", err)
			os.Exit(1)
		}

		if err := config.Save(cfg); err != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to save config: %v\n", err)
			os.Exit(1)
		}

		if output.IsJSON() {
			output.PrintJSON(map[string]interface{}{
				"ok":       true,
				"apiUrl":   cfg.APIURL,
				"saved":    true,
				"resource": "auth",
			})
			return
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
		incidentsAPIURL := config.GetIncidentsAPIURL(flagIncidentsAPIURL)

		if apiKey == "" {
			if output.IsJSON() {
				output.PrintJSON(map[string]interface{}{
					"authenticated":   false,
					"apiUrl":          apiURL,
					"incidentsApiUrl": incidentsAPIURL,
				})
				return
			}
			fmt.Println("Not authenticated. Run 'uptimyctl auth login' to configure.")
			return
		}

		masked := apiKey[:8] + strings.Repeat("•", 8)

		c := client.New(apiURL, apiKey)
		_, err := c.Get("/v1/api/applications/", nil)
		if output.IsJSON() {
			payload := map[string]interface{}{
				"authenticated":   true,
				"apiUrl":          apiURL,
				"incidentsApiUrl": incidentsAPIURL,
				"apiKeyMasked":    masked,
				"valid":           err == nil,
			}
			if err != nil {
				payload["error"] = err.Error()
			}
			output.PrintJSON(payload)
			return
		}

		fmt.Printf("API URL:           %s\n", apiURL)
		fmt.Printf("Incidents API URL: %s\n", incidentsAPIURL)
		fmt.Printf("API Key:           %s\n", masked)
		if err != nil {
			fmt.Printf("Status:  Invalid (%v)\n", err)
		} else {
			fmt.Println("Status:  Valid")
		}
	},
}

// readLoginKey picks where the key comes from: --api-key, piped stdin, a pasted
// code or key (--no-browser), or the browser flow.
func readLoginKey(cmd *cobra.Command, reader *bufio.Reader, apiURL string) (string, error) {
	if flagAPIKey != "" {
		return flagAPIKey, nil
	}
	if !stdinIsTerminal() {
		line, _ := reader.ReadString('\n')
		return strings.TrimSpace(line), nil
	}
	appURL := config.GetAppURL()
	if noBrowser, _ := cmd.Flags().GetBool("no-browser"); noBrowser {
		return manualLogin(appURL, apiURL, os.Stderr, func() string {
			line, _ := reader.ReadString('\n')
			return line
		})
	}
	return browserLogin(appURL, apiURL)
}

func stdinIsTerminal() bool {
	fi, err := os.Stdin.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

func init() {
	authLoginCmd.Flags().Bool("no-browser", false, "Print the authorization URL and paste the code it shows instead of opening a browser")
	authCmd.AddCommand(authLoginCmd)
	authCmd.AddCommand(authStatusCmd)
	rootCmd.AddCommand(authCmd)
}
