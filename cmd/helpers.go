package cmd

import (
	"fmt"
	"time"

	"github.com/uptimy/uptimyctl/internal/output"
)

func nowISO() string {
	return time.Now().UTC().Format(time.RFC3339)
}

func printActionResult(action, resource, uuid string) {
	if output.IsJSON() {
		payload := map[string]interface{}{
			"ok":       true,
			"action":   action,
			"resource": resource,
		}
		if uuid != "" {
			payload["uuid"] = uuid
		}
		output.PrintJSON(payload)
		return
	}

	if uuid != "" {
		fmt.Printf("%s %s: %s\n", resource, action, uuid)
		return
	}
	fmt.Printf("%s %s.\n", resource, action)
}
