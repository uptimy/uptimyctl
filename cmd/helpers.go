package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/uptimy/uptimyctl/internal/output"
)

func nowISO() string {
	return time.Now().UTC().Format(time.RFC3339)
}

// readJSONInput reads a JSON payload from a file path, or stdin when path is "-".
func readJSONInput(path string) (interface{}, error) {
	var data []byte
	var err error

	if path == "-" {
		data, err = os.ReadFile("/dev/stdin")
	} else {
		data, err = os.ReadFile(path)
	}
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}

	var payload interface{}
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}
	return payload, nil
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
