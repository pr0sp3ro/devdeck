package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

func selectProject(path string, c Config) error {
	query := ""

	for {
		list, err := projects(c)

		if err != nil {
			return err
		}

		if len(list) == 0 {
			fmt.Println("No projects found. Use devdeck source add <path> or devdeck project add <name> <path>.")
			return nil
		}

		for _, tool := range []string{"fzf", "tmux"} {
			if _, err := exec.LookPath(tool); err != nil {
				return fmt.Errorf("%s is required: %w", tool, err)
			}
		}

		sessions, err := newTmux().sessions()

		if err != nil {
			return err
		}

		var input bytes.Buffer

		for i, p := range list {
			star, active := " ", " "

			if p.Favorite {
				star = "*"
			}

			if sessions[sessionName(p.Path)] {
				active = "+"
			}

			// Numeric IDs keep names and paths separate from selection parsing.
			fmt.Fprintf(&input, "%d\t%s %s %s  %s%c", i, star, active, display(p.Name), display(p.Path), 0)
		}

		cmd := exec.Command(
			"fzf",
			"--read0",
			"--print0",
			"--delimiter=\t",
			"--with-nth=2..",
			"--layout=reverse",
			"--height=80%",
			"--prompt=Project> ",
			"--header=* favorite | + active | Enter: open | Ctrl-F: favorite | Esc: cancel",
			"--expect=enter,ctrl-f",
			"--no-multi",
			"--no-sort",
			"--print-query",
			"--query="+query,
		)

		// User defaults must not alter the output protocol or execute extra bindings.
		cmd.Env = append(os.Environ(), "FZF_DEFAULT_OPTS=", "FZF_DEFAULT_OPTS_FILE=")
		cmd.Stdin, cmd.Stderr = &input, os.Stderr
		out, err := cmd.Output()

		if err != nil {
			var exit *exec.ExitError

			if errors.As(err, &exit) && (exit.ExitCode() == 1 || exit.ExitCode() == 130) {
				return nil
			}

			return fmt.Errorf("fzf: %w", err)
		}

		fields := strings.Split(string(out), "\x00")

		if len(fields) < 3 {
			return fmt.Errorf("unexpected fzf output")
		}

		query = fields[0]
		key := fields[1]
		id, err := strconv.Atoi(strings.SplitN(fields[2], "\t", 2)[0])

		if err != nil || id < 0 || id >= len(list) {
			return fmt.Errorf("invalid fzf selection")
		}

		p := list[id]

		if key != "ctrl-f" {
			return openProject(p)
		}

		// Reload before changing a setting, preserving other edits made while fzf ran.
		c, err = loadConfig(path)

		if err != nil {
			return err
		}

		if err = favorite(&c, p, !p.Favorite); err != nil {
			return err
		}

		if err = saveConfig(path, c); err != nil {
			return err
		}
	}
}

func display(value string) string {
	return strings.Map(func(r rune) rune {
		if r < 32 || r == 127 {
			return ' '
		}

		return r
	}, value)
}
