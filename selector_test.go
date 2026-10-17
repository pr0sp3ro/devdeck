package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPickerReloadsSettingsBeforeOpening(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	root := t.TempDir()
	configFile := filepath.Join(root, "config.toml")
	updatedFile := filepath.Join(root, "updated.toml")
	argsFile := filepath.Join(root, "tmux-args")
	initial := Config{Projects: map[string]Project{
		"demo": {Path: root, Windows: []Window{{Name: "old", Command: "printf old"}}},
	}}
	updated := Config{Projects: map[string]Project{
		"renamed": {Path: root, Windows: []Window{{Name: "new", Command: "printf latest"}}},
	}}

	if err := saveConfig(configFile, initial); err != nil {
		t.Fatal(err)
	}

	if err := saveConfig(updatedFile, updated); err != nil {
		t.Fatal(err)
	}

	// The picker fixture changes configuration while the selection is open.
	if err := os.WriteFile(filepath.Join(root, "fzf"), []byte(`#!/bin/sh
cat "$DEVDECK_TEST_UPDATED_CONFIG" > "$DEVDECK_TEST_CONFIG"
printf '%s\000' '' enter '0	project'
`), 0700); err != nil {
		t.Fatal(err)
	}

	// Record startup arguments without creating or attaching to a real session.
	if err := os.WriteFile(filepath.Join(root, "tmux"), []byte(`#!/bin/sh
case "$1" in
    has-session) exit 1 ;;
    new-session) printf '%s\n' '$42 @7' ;;
    show-options)
        if [ "$3" = default-shell ]; then
            printf '%s\n' /bin/sh
        fi
        ;;
    respawn-window) printf '%s\n' "$@" > "$DEVDECK_TEST_ARGS" ;;
esac
exit 0
`), 0700); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PATH", root+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("DEVDECK_TEST_CONFIG", configFile)
	t.Setenv("DEVDECK_TEST_UPDATED_CONFIG", updatedFile)
	t.Setenv("DEVDECK_TEST_ARGS", argsFile)

	if err := selectProject(configFile, initial); err != nil {
		t.Fatal(err)
	}

	args, err := os.ReadFile(argsFile)

	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(string(args), "\nprintf latest\n") || strings.Contains(string(args), "printf old") {
		t.Fatalf("picker did not use updated settings: %s", args)
	}
}
