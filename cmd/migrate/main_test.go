package main

import (
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestCommands(t *testing.T) {
	for _, arguments := range [][]string{nil, {"drop"}, {"down"}, {"down", "0"}, {"down", "-1"}, {"force", "-2"}, {"create", "../bad"}, {"up", "extra"}, {"status"}} {
		if err := run(arguments, func(string) string { return "" }, io.Discard); err == nil {
			t.Fatalf("expected rejection: %v", arguments)
		}
	}
}

func TestCreate(t *testing.T) {
	directory := t.TempDir()
	if err := run([]string{"create", "test_schema"}, func(name string) string {
		if name == "MIGRATIONS_DIR" {
			return directory
		}
		return ""
	}, io.Discard); err != nil {
		t.Fatal(err)
	}
	for _, direction := range []string{"up", "down"} {
		matches, err := filepath.Glob(filepath.Join(directory, "*_test_schema."+direction+".sql"))
		if err != nil || len(matches) != 1 {
			t.Fatalf("missing %s migration", direction)
		}
		contents, err := os.ReadFile(matches[0])
		if err != nil || len(contents) != 0 {
			t.Fatal("unexpected business schema")
		}
	}
}
