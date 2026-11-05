package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/uptimy/uptimyctl/internal/output"
	"github.com/uptimy/uptimyctl/internal/workspace"
)

// The workspace as a file: export writes it, plan shows what applying it
// would change, apply makes the changes. See internal/workspace.

func newServices() workspace.Services {
	return workspace.Services{API: newClient(), Workflows: newIncidentsClient(), Heartbeats: newHeartbeatsClient()}
}

var exportCmd = &cobra.Command{
	Use:   "export",
	Short: "Write the workspace (monitors, cron jobs, status pages, alerts) as a file",
	Long: `Write the workspace as a YAML file that plan and apply read: monitors,
heartbeats (cron jobs), status pages and alerts, by name, defaults left out.

Agent-managed monitors and workflows built in the workflow editor aren't
written: they're changed where they were made.

Examples:
  uptimyctl export -f uptimy.yaml          # Save to a file
  uptimyctl export                         # Print it
  uptimyctl export -o json                 # As JSON`,
	Run: func(cmd *cobra.Command, args []string) {
		st, err := workspace.Fetch(newServices())
		if err != nil {
			exitErr(err)
		}
		f := workspace.Export(st)

		var data []byte
		if output.IsJSON() {
			output.PrintJSON(f)
			return
		}
		data, err = f.Marshal(fmt.Sprintf("Uptimy workspace %s, exported %s.\nChange it, then: uptimyctl plan -f <file>, and uptimyctl apply -f <file>.", st.Workspace, time.Now().UTC().Format("2006-01-02")))
		if err != nil {
			exitErr(err)
		}
		file, _ := cmd.Flags().GetString("file")
		if file == "" {
			fmt.Print(string(data))
			return
		}
		if err := os.WriteFile(file, data, 0644); err != nil {
			exitErr(fmt.Errorf("write file: %w", err))
		}
		fmt.Printf("Exported %s to %s: %d monitors, %d heartbeats, %d status pages, %d alerts\n",
			st.Workspace, file, len(f.Monitors), len(f.Heartbeats), len(f.StatusPages), len(f.Alerts))
	},
}

// loadPlan reads the file and the workspace and compares them.
func loadPlan(cmd *cobra.Command) (*workspace.File, *workspace.State, *workspace.Plan, workspace.Options) {
	path, _ := cmd.Flags().GetString("file")
	prune, _ := cmd.Flags().GetBool("prune")
	opt := workspace.Options{Prune: prune}
	f, err := workspace.Load(path)
	if err != nil {
		exitErr(err)
	}
	st, err := workspace.Fetch(newServices())
	if err != nil {
		exitErr(err)
	}
	p, err := workspace.MakePlan(f, st, opt)
	if err != nil {
		exitErr(fmt.Errorf("%s can't be applied:\n%w", path, err))
	}
	return f, st, p, opt
}

func printPlan(p *workspace.Plan, prune bool) {
	if len(p.Changes) == 0 {
		fmt.Printf("%s already matches the file.\n", p.Workspace)
	} else {
		fmt.Printf("Changes to %s:\n\n", p.Workspace)
		for _, c := range p.Changes {
			fmt.Println("  " + c.String())
		}
		fmt.Printf("\n%s.\n", p.Summary())
	}
	if len(p.NotInFile) > 0 && !prune {
		fmt.Printf("\nNot in the file, kept (--prune removes them): %s\n", strings.Join(p.NotInFile, ", "))
	}
	if len(p.EditorWorkflows) > 0 {
		fmt.Printf("Made in the workflow editor, left alone: %s\n", strings.Join(p.EditorWorkflows, ", "))
	}
}

var planCmd = &cobra.Command{
	Use:   "plan",
	Short: "Show what applying a workspace file would change",
	Long: `Compare a workspace file with the live workspace and list the changes
apply would make. Changes nothing.

Examples:
  uptimyctl plan -f uptimy.yaml
  uptimyctl plan -f uptimy.yaml --prune    # Also list what apply --prune removes
  uptimyctl plan -f uptimy.yaml -o json`,
	Run: func(cmd *cobra.Command, args []string) {
		_, _, p, opt := loadPlan(cmd)
		if output.IsJSON() {
			output.PrintJSON(p)
			return
		}
		printPlan(p, opt.Prune)
	},
}

var applyCmd = &cobra.Command{
	Use:   "apply",
	Short: "Make the workspace match a workspace file",
	Long: `Make the changes plan shows: add and change monitors, heartbeats,
status pages and alerts so they match the file. Things the file doesn't
have are kept unless --prune is given.

Shows the plan and asks first; --yes applies without asking (needed when
there's no terminal, e.g. in CI or from an agent). It stops at the first
error; applying again carries on from there.

Examples:
  uptimyctl apply -f uptimy.yaml
  uptimyctl apply -f uptimy.yaml --yes
  uptimyctl apply -f uptimy.yaml --prune --yes`,
	Run: func(cmd *cobra.Command, args []string) {
		f, st, p, opt := loadPlan(cmd)
		yes, _ := cmd.Flags().GetBool("yes")
		if !output.IsJSON() {
			printPlan(p, opt.Prune)
		}
		if len(p.Changes) == 0 {
			if output.IsJSON() {
				output.PrintJSON(map[string]any{"ok": true, "applied": []workspace.Change{}})
			}
			return
		}
		if !yes {
			if output.IsJSON() || !isTerminal(os.Stdin) {
				exitErr(fmt.Errorf("not asking without a terminal: add --yes to apply %s", p.Summary()))
			}
			fmt.Print("\nApply these changes? [y/N] ")
			answer, _ := bufio.NewReader(os.Stdin).ReadString('\n')
			if a := strings.ToLower(strings.TrimSpace(answer)); a != "y" && a != "yes" {
				fmt.Println("Nothing changed.")
				return
			}
			fmt.Println()
		}

		applied := []workspace.Change{}
		err := workspace.Apply(newServices(), f, st, p, opt, func(c workspace.Change) {
			applied = append(applied, c)
			if !output.IsJSON() {
				fmt.Println("  done: " + c.String())
			}
		})
		if output.IsJSON() {
			res := map[string]any{"ok": err == nil, "applied": applied}
			if err != nil {
				res["error"] = err.Error()
			}
			output.PrintJSON(res)
			if err != nil {
				os.Exit(1)
			}
			return
		}
		if err != nil {
			exitErr(fmt.Errorf("stopped: %w\nApply again to carry on", err))
		}
		fmt.Printf("\nApplied to %s.\n", p.Workspace)
	},
}

func init() {
	exportCmd.Flags().StringP("file", "f", "", "Output file path (default: stdout)")
	for _, c := range []*cobra.Command{planCmd, applyCmd} {
		c.Flags().StringP("file", "f", "uptimy.yaml", "Workspace file, - for stdin")
		c.Flags().Bool("prune", false, "Remove what the file doesn't have (monitors, heartbeats, status pages, alerts)")
	}
	applyCmd.Flags().BoolP("yes", "y", false, "Apply without asking")

	rootCmd.AddCommand(exportCmd)
	rootCmd.AddCommand(planCmd)
	rootCmd.AddCommand(applyCmd)
}
