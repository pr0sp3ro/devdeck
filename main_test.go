package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/pelletier/go-toml/v2"
)

func TestCommandsRejectEmptyPathsWithoutChangingConfig(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", root)
	path, err := configPath()

	if err != nil {
		t.Fatal(err)
	}

	c := Config{
		Sources:  []Source{{Path: root}},
		Projects: map[string]Project{"demo": {Path: root}},
	}

	if err := saveConfig(path, c); err != nil {
		t.Fatal(err)
	}

	before, err := os.ReadFile(path)

	if err != nil {
		t.Fatal(err)
	}

	for _, value := range []string{"", "  "} {
		for _, args := range [][]string{
			{"source", "add", value},
			{"source", "remove", value},
			{"project", "add", "other", value},
			{"config", "set", "projects.demo.path", value},
			{"config", "set", "sources.0.path", value},
		} {
			if err := run(args); err == nil {
				t.Fatalf("accepted an empty path: %q", args)
			}

			after, err := os.ReadFile(path)

			if err != nil || !bytes.Equal(before, after) {
				t.Fatalf("rejected command changed configuration: %q, %v", args, err)
			}
		}
	}
}

func TestConfigEditRejectsDuplicateCanonicalPaths(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", root)
	t.Setenv("VISUAL", `sh -c 'cat "$DEVDECK_TEST_EDIT_CONTENT" > "$1"' devdeck-test`)
	path, err := configPath()

	if err != nil {
		t.Fatal(err)
	}

	projectPath := t.TempDir()
	alias := filepath.Join(root, "alias")

	if err := os.Symlink(projectPath, alias); err != nil {
		t.Fatal(err)
	}

	content := filepath.Join(root, "edited.toml")
	t.Setenv("DEVDECK_TEST_EDIT_CONTENT", content)

	for _, duplicate := range []string{projectPath, alias} {
		if err := saveConfig(path, Config{}); err != nil {
			t.Fatal(err)
		}

		c := Config{Projects: map[string]Project{
			"alpha": {Path: projectPath},
			"beta":  {Path: duplicate},
		}}
		data, err := toml.Marshal(c)

		if err != nil {
			t.Fatal(err)
		}

		if err := os.WriteFile(content, data, 0600); err != nil {
			t.Fatal(err)
		}

		if err := saveConfig(path, c); err == nil {
			t.Fatal("save accepted duplicate canonical paths")
		}

		if err := run([]string{"config", "edit"}); err == nil {
			t.Fatal("editor validation accepted duplicate canonical paths")
		}

		if _, err := loadConfig(path); err == nil {
			t.Fatal("load accepted duplicate canonical paths")
		}
	}
}

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

func TestConfigEditPreservesSymlinkWithReplacingEditor(t *testing.T) {
	for _, args := range [][]string{{"config", "edit"}, {"project", "edit", "demo"}} {
		t.Run(args[0], func(t *testing.T) {
			root := t.TempDir()
			t.Setenv("XDG_CONFIG_HOME", root)
			t.Setenv("VISUAL", `sh -c 'printf "sources = []\n" > "$1.new" && mv "$1.new" "$1"' devdeck-test`)

			path, err := configPath()

			if err != nil {
				t.Fatal(err)
			}

			target := filepath.Join(root, "target.toml")
			c := Config{Projects: map[string]Project{"demo": {Path: root}}}

			if err := saveConfig(target, c); err != nil {
				t.Fatal(err)
			}

			if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				t.Fatal(err)
			}

			linkTarget := filepath.Join("..", "target.toml")

			if err := os.Symlink(linkTarget, path); err != nil {
				t.Fatal(err)
			}

			if err := run(args); err != nil {
				t.Fatal(err)
			}

			actual, err := os.Readlink(path)

			if err != nil || actual != linkTarget {
				t.Fatalf("editor replaced the symlink: %q, %v", actual, err)
			}

			data, err := os.ReadFile(target)

			if err != nil || string(data) != "sources = []\n" {
				t.Fatalf("editor did not update the target: %q, %v", data, err)
			}
		})
	}
}
