package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestResolveProjectAcceptsParentDirectory(t *testing.T) {
	want, err := canonicalPath("..")

	if err != nil {
		t.Fatal(err)
	}

	p, err := resolveProject(Config{}, "..")

	if err != nil || p.Path != want {
		t.Fatalf("parent directory was not resolved: %v, %v", p, err)
	}
}

func TestConfigRoundTripAndInvalidInput(t *testing.T) {
	path := filepath.Join(t.TempDir(), "devdeck", "config.toml")
	c, err := loadConfig(path)

	if err != nil || len(c.Projects) != 0 {
		t.Fatalf("missing config: %v, %v", c, err)
	}

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("reading created a config")
	}

	c.Projects["demo"] = Project{Path: t.TempDir(), Favorite: true, Windows: []Window{{Name: "editor", Command: "nvim"}, {Name: "shell"}}}

	if err := saveConfig(path, c); err != nil {
		t.Fatal(err)
	}

	loaded, err := loadConfig(path)

	if err != nil || !reflect.DeepEqual(loaded.Projects, c.Projects) {
		t.Fatalf("round trip: %v, %v", loaded, err)
	}

	for _, input := range []string{
		"invalid = [",
		"unexpected = true",
		"[projects.demo]\npath = 42",
		"[projects.demo]\npath = '/tmp'\nwindows = [{name = ''}]",
	} {
		if err := os.WriteFile(path, []byte(input), 0600); err != nil {
			t.Fatal(err)
		}

		if _, err := loadConfig(path); err == nil {
			t.Fatalf("accepted invalid input %q", input)
		}

		after, _ := os.ReadFile(path)

		if string(after) != input {
			t.Fatal("invalid config was modified")
		}
	}
}

func TestSaveConfigPreservesSymlink(t *testing.T) {
	for _, relative := range []bool{false, true} {
		name := "absolute target"

		if relative {
			name = "relative target"
		}

		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			targetDir := filepath.Join(root, "dotfiles")

			if err := os.Mkdir(targetDir, 0700); err != nil {
				t.Fatal(err)
			}

			target := filepath.Join(targetDir, "config.toml")

			if err := os.WriteFile(target, []byte("sources = []\n"), 0600); err != nil {
				t.Fatal(err)
			}

			link := filepath.Join(root, "config.toml")
			linkTarget := target

			if relative {
				linkTarget = filepath.Join("dotfiles", "config.toml")
			}

			if err := os.Symlink(linkTarget, link); err != nil {
				t.Fatal(err)
			}

			c := Config{Sources: []Source{{Path: root}}}

			if err := saveConfig(link, c); err != nil {
				t.Fatal(err)
			}

			actualTarget, err := os.Readlink(link)

			if err != nil || actualTarget != linkTarget {
				t.Fatalf("symlink changed: %q, %v", actualTarget, err)
			}

			loaded, err := loadConfig(target)

			if err != nil || !reflect.DeepEqual(loaded.Sources, c.Sources) {
				t.Fatalf("target not updated: %v, %v", loaded, err)
			}
		})
	}
}

func TestSaveConfigRejectsDanglingSymlink(t *testing.T) {
	root := t.TempDir()
	link := filepath.Join(root, "config.toml")
	target := filepath.Join(root, "missing.toml")

	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	if err := saveConfig(link, Config{}); err == nil {
		t.Fatal("accepted a dangling configuration symlink")
	}

	if actual, err := os.Readlink(link); err != nil || actual != target {
		t.Fatalf("dangling symlink changed: %q, %v", actual, err)
	}

	if _, err := os.Lstat(target); !os.IsNotExist(err) {
		t.Fatalf("missing target was created: %v", err)
	}
}

func TestDiscoveryDeduplicatesSymlinksAndKeepsExplicitSettings(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "demo")

	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}

	if err := os.Symlink(path, filepath.Join(root, "alias")); err != nil {
		t.Fatal(err)
	}

	c := Config{
		Sources: []Source{{Path: root}, {Path: root}},
		Projects: map[string]Project{
			"custom": {
				Path:     path,
				Favorite: true,
				Windows:  []Window{{Name: "editor", Command: "nvim"}},
			},
		},
	}
	list, err := projects(c)

	if err != nil || len(list) != 1 {
		t.Fatalf("discovery: %v, %v", list, err)
	}

	p := list[0]

	if p.Name != "custom" || !p.Favorite || len(p.Windows) != 1 {
		t.Fatalf("lost explicit settings: %v", p)
	}

	if err := favorite(&c, p, false); err != nil {
		t.Fatal(err)
	}

	if c.Projects["custom"].Favorite || len(c.Projects["custom"].Windows) != 1 {
		t.Fatal("favorite toggle lost settings")
	}
}

func TestSettingsValidateTypesAndPreserveWindowsOnFailure(t *testing.T) {
	c := Config{Projects: map[string]Project{"demo": {Path: t.TempDir()}}}

	if err := setConfigValue(&c, "projects.demo.windows", `[{name="editor", command="nvim"}, {name="shell"}]`); err != nil {
		t.Fatal(err)
	}

	if err := setConfigValue(&c, "projects.demo.windows.0.name", ""); err == nil {
		t.Fatal("accepted empty name")
	}

	if c.Projects["demo"].Windows[0].Name != "editor" {
		t.Fatal("failed update mutated windows")
	}

	if err := setConfigValue(&c, "projects.demo.favorite", "maybe"); err == nil {
		t.Fatal("accepted invalid bool")
	}

	if value, err := configValue(c, "projects.demo.favorite"); err != nil || value != false {
		t.Fatalf("default bool: %v, %v", value, err)
	}

	if value, err := configValue(c, "projects.demo.windows.1.command"); err != nil || value != "" {
		t.Fatalf("default command: %v, %v", value, err)
	}
}

func TestFavoriteNormalizesDiscoveredNamesWithoutOverwritingProjects(t *testing.T) {
	for _, name := range []string{"example.app", "example app"} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, name)

			if err := os.Mkdir(path, 0700); err != nil {
				t.Fatal(err)
			}

			c := Config{Sources: []Source{{Path: root}}, Projects: map[string]Project{}}
			list, err := projects(c)

			if err != nil || len(list) != 1 {
				t.Fatalf("discovery: %v, %v", list, err)
			}

			if err := favorite(&c, list[0], true); err != nil {
				t.Fatal(err)
			}

			configFile := filepath.Join(t.TempDir(), "config.toml")

			if err := saveConfig(configFile, c); err != nil {
				t.Fatal(err)
			}

			loaded, err := loadConfig(configFile)

			if err != nil {
				t.Fatal(err)
			}

			p := loaded.Projects["example_app"]

			if !p.Favorite || p.Path != path {
				t.Fatalf("favorite not saved correctly: %v", p)
			}

			other := Project{Path: t.TempDir()}
			loaded.Projects["example_app"] = other

			if err := favorite(&loaded, list[0], true); err == nil {
				t.Fatal("accepted a conflicting identifier")
			}

			if !reflect.DeepEqual(loaded.Projects["example_app"], other) {
				t.Fatal("conflicting project was overwritten")
			}
		})
	}
}

func TestUnreadableSourceDoesNotBlockOtherProjects(t *testing.T) {
	unreadable := t.TempDir()

	if err := os.Chmod(unreadable, 0111); err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		if err := os.Chmod(unreadable, 0700); err != nil {
			t.Error(err)
		}
	})

	if _, err := os.ReadDir(unreadable); err == nil {
		t.Skip("current user can read directories without read permission")
	} else if !os.IsPermission(err) {
		t.Fatal(err)
	}

	root := t.TempDir()
	discovered := filepath.Join(root, "discovered")

	if err := os.Mkdir(discovered, 0700); err != nil {
		t.Fatal(err)
	}

	configured := t.TempDir()
	direct := t.TempDir()
	c := Config{
		Sources: []Source{{Path: unreadable}, {Path: root}},
		Projects: map[string]Project{
			"configured": {Path: configured},
		},
	}
	list, err := projects(c)

	if err != nil || len(list) != 2 {
		t.Fatalf("unreadable source blocked discovery: %v, %v", list, err)
	}

	for _, selection := range []struct{ value, path string }{
		{"configured", configured},
		{"discovered", discovered},
		{direct, direct},
	} {
		p, err := resolveProject(c, selection.value)

		if err != nil || p.Path != selection.path {
			t.Fatalf("unreadable source blocked %q: %v, %v", selection.value, p, err)
		}
	}
}
