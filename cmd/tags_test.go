package cmd

import (
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

var testTags = []workspaceTag{{ID: 4, Name: "Production"}, {ID: 9, Name: "api"}, {ID: 12, Name: "Staging"}}

func TestResolveTagIDsByNameOrID(t *testing.T) {
	got, err := resolveTagIDs([]string{"production", " API ", "12", "Production"}, testTags)
	if err != nil || !reflect.DeepEqual(got, []int{4, 9, 12}) {
		t.Fatalf("got %v, %v; want [4 9 12] (any case, ids, each once)", got, err)
	}
}

func TestResolveTagIDsNamesTheUnknown(t *testing.T) {
	_, err := resolveTagIDs([]string{"prod", "api", "99"}, testTags)
	if err == nil || !strings.Contains(err.Error(), "prod, 99") || !strings.Contains(err.Error(), "Production, Staging, api") {
		t.Fatalf("err = %v; want the unknown tags and the known ones", err)
	}
	if _, err := resolveTagIDs([]string{"x"}, nil); err == nil || !strings.Contains(err.Error(), "none yet") {
		t.Fatalf("err = %v; want a hint that the workspace has no tags", err)
	}
}

func TestTagFlagsNeedTagsOrClear(t *testing.T) {
	newCmd := func(args ...string) *cobra.Command {
		c := &cobra.Command{}
		c.Flags().StringSlice("tag", nil, "")
		c.Flags().Bool("clear", false, "")
		if err := c.ParseFlags(args); err != nil {
			t.Fatal(err)
		}
		return c
	}
	if _, err := tagIDsFromFlags(newCmd()); err == nil {
		t.Error("neither --tag nor --clear must be an error")
	}
	if _, err := tagIDsFromFlags(newCmd("--tag", "api", "--clear")); err == nil {
		t.Error("--tag with --clear must be an error")
	}
	if ids, err := tagIDsFromFlags(newCmd("--clear")); err != nil || ids == nil || len(ids) != 0 {
		t.Errorf("--clear: got %v, %v; want an empty list", ids, err)
	}
}
