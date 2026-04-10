package output

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/rodaine/table"
)

var Format = "table" // "table" or "json"

func PrintJSON(data interface{}) {
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(string(b))
}

func PrintRawJSON(raw json.RawMessage) {
	var v interface{}
	if err := json.Unmarshal(raw, &v); err != nil {
		fmt.Println(string(raw))
		return
	}
	PrintJSON(v)
}

func PrintTable(headers []string, rows [][]string) {
	if len(rows) == 0 {
		fmt.Println("No results found.")
		return
	}
	ifaces := make([]interface{}, len(headers))
	for i, h := range headers {
		ifaces[i] = h
	}
	tbl := table.New(ifaces...)
	tbl.WithWriter(os.Stdout)
	for _, row := range rows {
		rowIfaces := make([]interface{}, len(row))
		for i, val := range row {
			rowIfaces[i] = val
		}
		tbl.AddRow(rowIfaces...)
	}
	tbl.Print()
}

func Truncate(s string, max int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

func ValueOrDash(s string) string {
	if s == "" || s == "<nil>" || s == "null" {
		return "-"
	}
	return s
}
