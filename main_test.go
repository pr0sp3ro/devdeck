package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestConfigSetRejectsDuplicateProjectPaths(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	alphaPath := t.TempDir()
	betaPath := t.TempDir()
	alias := filepath.Join(t.TempDir(), "alias")

	if err := os.Symlink(alphaPath, alias); err != nil {
		t.Fatal(err)
	}

	path, err := configPath()

	if err != nil {
		t.Fatal(err)
	}

	c := Config{Projects: map[string]Project{
		"alpha": {Path: alphaPath},
		"beta":  {Path: betaPath},
	}}

	if err := saveConfig(path, c); err != nil {
		t.Fatal(err)
	}

	before, err := os.ReadFile(path)

	if err != nil {
		t.Fatal(err)
	}

	for _, duplicate := range []string{alphaPath, alias} {
		if err := run([]string{"config", "set", "projects.beta.path", duplicate}); err == nil {
			t.Fatalf("accepted duplicate path: %s", duplicate)
		}

		after, err := os.ReadFile(path)

		if err != nil || !bytes.Equal(before, after) {
			t.Fatalf("rejected update changed configuration: %v", err)
		}
	}

	if err := run([]string{"config", "set", "projects.beta.path", betaPath}); err != nil {
		t.Fatalf("rejected project's own path: %v", err)
	}

	newPath := t.TempDir()

	if err := run([]string{"config", "set", "projects.beta.path", newPath}); err != nil {
		t.Fatalf("rejected unique path: %v", err)
	}

	loaded, err := loadConfig(path)

	if err != nil || loaded.Projects["beta"].Path != newPath {
		t.Fatalf("unique path was not saved: %v, %v", loaded, err)
	}
}

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
