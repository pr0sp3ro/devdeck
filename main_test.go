package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigEditCanRepairInvalidConfiguration(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("VISUAL", `sh -c 'printf "sources = []\n" > "$1"' devdeck-test`)

	path, err := configPath()

	if err != nil {
		t.Fatal(err)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(path, []byte("invalid = ["), 0600); err != nil {
		t.Fatal(err)
	}

	if err := run([]string{"config", "edit"}); err != nil {
		t.Fatalf("could not repair configuration: %v", err)
	}

	if _, err := loadConfig(path); err != nil {
		t.Fatalf("configuration was not repaired: %v", err)
	}

	t.Setenv("VISUAL", `sh -c 'printf "invalid = [" > "$1"' devdeck-test`)

	if err := run([]string{"config", "edit"}); err == nil {
		t.Fatal("invalid configuration was accepted after editing")
	}
}
