package main

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"testing"
)

func TestSessionIdentity(t *testing.T) {
	first := sessionName("/one/my project")

	if !regexp.MustCompile(`^my_project-[a-f0-9]{16}$`).MatchString(first) {
		t.Fatalf("unexpected name: %s", first)
	}

	if first != sessionName("/one/my project") || first == sessionName("/two/my project") {
		t.Fatal("session identity is not path-specific and stable")
	}
}

func TestExistingSessionDoesNotInitializeAgain(t *testing.T) {
	var calls [][]string
	p := Project{Path: "/missing/project", Windows: []Window{{Name: "editor", Command: "nvim"}}}
	client := tmux{run: func(args ...string) (string, error) { calls = append(calls, args); return "", nil }}
	name, err := client.ensure(p)

	if err != nil || name != sessionName(p.Path) {
		t.Fatalf("ensure: %s, %v", name, err)
	}

	want := [][]string{{"has-session", "-t", "=" + name}}

	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("existing session was modified: %v", calls)
	}
}

func TestCreationFailureOnlyCleansUpOwnedSession(t *testing.T) {
	for _, created := range []bool{false, true} {
		t.Run(map[bool]string{false: "creation rejected", true: "window setup failed"}[created], func(t *testing.T) {
			var calls [][]string
			client := tmux{run: func(args ...string) (string, error) {
				calls = append(calls, args)

				switch args[0] {
				case "has-session":
					return "", errors.New("absent")

				case "new-session":
					if created {
						return "$42 @7", nil
					}

					return "", errors.New("already exists")

				case "show-options":
					return "/bin/sh", nil

				case "set-option":
					return "", errors.New("setup failed")

				default:
					return "", nil
				}

			}}

			if _, err := client.ensure(Project{Path: t.TempDir()}); err == nil {
				t.Fatal("expected failure")
			}

			killed := false

			for _, call := range calls {
				if call[0] == "kill-session" {
					killed = true

					if !reflect.DeepEqual(call, []string{"kill-session", "-t", "$42"}) {
						t.Fatalf("wrong cleanup: %v", call)
					}
				}
			}

			if killed != created {
				t.Fatalf("cleanup ownership mismatch: %v", calls)
			}
		})
	}
}

func TestStartupRetainsFastCommandsAndSelectsFirstWindow(t *testing.T) {
	dir := t.TempDir()
	windowDir := filepath.Join(dir, "~cache")

	if err := os.Mkdir(windowDir, 0700); err != nil {
		t.Fatal(err)
	}

	var calls [][]string
	client := tmux{run: func(args ...string) (string, error) {
		calls = append(calls, args)

		switch args[0] {
		case "has-session":
			return "", errors.New("absent")

		case "new-session":
			return "$42 @7", nil

		case "new-window":
			return "@8", nil

		case "show-options":
			if args[2] == "default-shell" {
				return "/bin/sh", nil
			}
		}

		return "", nil
	}}
	p := Project{Path: dir, Windows: []Window{
		{Name: "output", Command: "printf done", Directory: "~cache"},
		{Name: "shell"},
	}}

	if _, err := client.ensure(p); err != nil {
		t.Fatal(err)
	}

	retained := map[string]bool{}
	commands := [][]string{}

	for _, call := range calls {
		if call[0] == "set-option" {
			retained[call[3]] = true
		}

		if call[0] == "respawn-window" {
			if !retained[call[3]] {
				t.Fatalf("command starts before remain-on-exit: %v", call)
			}

			commands = append(commands, call[7:])

			if call[3] == "@7" && call[5] != windowDir {
				t.Fatalf("relative window directory resolved outside the project: %s", call[5])
			}
		}
	}

	if !reflect.DeepEqual(commands, [][]string{{"printf done"}, {"/bin/sh", "-l"}}) {
		t.Fatalf("unexpected commands: %v", commands)
	}

	if !reflect.DeepEqual(calls[len(calls)-1], []string{"select-window", "-t", "@7"}) {
		t.Fatal("first window not selected")
	}
}

func TestTmuxRejectsUnsafeWindowSettingsBeforeCreation(t *testing.T) {
	for _, window := range []Window{
		{Name: "editor;"},
		{Name: "editor[1]"},
		{Name: "editor\nname"},
		{Name: "editor", Directory: "bad;"},
	} {
		client := tmux{run: func(args ...string) (string, error) {
			if args[0] != "has-session" {
				t.Fatalf("invalid settings reached tmux initialization: %v", args)
			}

			return "", errors.New("absent")
		}}

		if _, err := client.ensure(Project{Path: t.TempDir(), Windows: []Window{window}}); err == nil {
			t.Fatalf("accepted unsafe settings: %v", window)
		}
	}
}
