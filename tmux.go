package main

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func sessionName(path string) string {
	var name strings.Builder

	for _, b := range []byte(filepath.Base(path)) {
		if b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9' || b == '_' || b == '-' {
			name.WriteByte(b)
		} else {
			name.WriteByte('_')
		}
	}

	if name.Len() == 0 {
		name.WriteString("project")
	}

	hash := sha256.Sum256([]byte(path))
	return fmt.Sprintf("%s-%x", name.String(), hash[:8])
}

type tmux struct {
	run func(...string) (string, error)
}

func newTmux() tmux {
	return tmux{run: func(args ...string) (string, error) {
		cmd := exec.Command("tmux", args...)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		output, err := cmd.Output()

		if err != nil {
			return "", fmt.Errorf("tmux %s: %w: %s", args[0], err, strings.TrimSpace(stderr.String()))
		}

		if stderr.Len() > 0 {
			fmt.Fprint(os.Stderr, stderr.String())
		}

		return strings.TrimSpace(string(output)), nil
	}}
}

func (t tmux) sessions() (map[string]bool, error) {
	// Distinguish an absent server from other failures without hiding errors.
	out, err := exec.Command("tmux", "list-sessions", "-F", "#{session_name}").CombinedOutput()

	if err != nil {
		message := string(out)

		if strings.Contains(message, "no server running") || strings.Contains(message, "No such file or directory") {
			return map[string]bool{}, nil
		}

		return nil, fmt.Errorf("list tmux sessions: %w: %s", err, strings.TrimSpace(message))
	}

	result := map[string]bool{}

	for _, name := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		result[name] = true
	}

	return result, nil
}

func (t tmux) ensure(p Project) (string, error) {
	name := sessionName(p.Path)

	if _, err := t.run("has-session", "-t", "="+name); err == nil {
		return name, nil
	}

	windows := p.Windows

	if len(windows) == 0 {
		windows = []Window{{Name: "terminal"}}
	}

	dirs := make([]string, len(windows))

	for i, w := range windows {
		if strings.ContainsRune(w.Name+w.Command+w.Directory, 0) {
			return "", fmt.Errorf("window settings contain a NUL character")
		}

		if strings.TrimSpace(w.Name) == "" {
			return "", fmt.Errorf("window name cannot be empty")
		}

		dir := w.Directory

		if dir == "" {
			dir = p.Path
		} else if !filepath.IsAbs(dir) && dir != "~" && !strings.HasPrefix(dir, "~/") {
			dir = filepath.Join(p.Path, dir)
		}

		resolved, err := canonicalPath(dir)

		if err != nil {
			return "", fmt.Errorf("window %s: %w", w.Name, err)
		}

		dirs[i] = resolved
	}

	// Start a placeholder first so remain-on-exit is set before a fast command finishes.
	out, err := t.run(
		"new-session", "-d", "-P",
		"-F", "#{session_id} #{window_id}",
		"-s", name,
		"-n", windows[0].Name,
		"-c", dirs[0],
		"sleep 2147483647",
	)

	if err != nil {
		return "", err
	}

	ids := strings.Fields(out)

	if len(ids) != 2 {
		return "", fmt.Errorf("unexpected tmux session identifiers: %q", out)
	}

	session, first := ids[0], ids[1]
	cleanup := func(err error) (string, error) {
		if _, cleanupErr := t.run("kill-session", "-t", session); cleanupErr != nil {
			return "", fmt.Errorf("%w (cleanup failed: %v)", err, cleanupErr)
		}

		return "", err
	}
	shell, err := t.run("show-options", "-gv", "default-shell")

	if err != nil {
		return cleanup(err)
	}

	defaultCommand, err := t.run("show-options", "-gv", "default-command")

	if err != nil {
		return cleanup(err)
	}

	for i, w := range windows {
		id := first

		if i > 0 {
			id, err = t.run(
				"new-window", "-d", "-P",
				"-F", "#{window_id}",
				"-t", session+":",
				"-n", w.Name,
				"-c", dirs[i],
				"sleep 2147483647",
			)

			if err != nil {
				return cleanup(err)
			}
		}

		if _, err = t.run("set-option", "-w", "-t", id, "remain-on-exit", "on"); err != nil {
			return cleanup(err)
		}

		args := []string{"respawn-window", "-k", "-t", id, "-c", dirs[i], "--"}

		if w.Command != "" {
			args = append(args, w.Command)
		} else if defaultCommand != "" {
			args = append(args, defaultCommand)
		} else {
			args = append(args, shell, "-l")
		}

		if _, err = t.run(args...); err != nil {
			return cleanup(err)
		}
	}

	if _, err = t.run("select-window", "-t", first); err != nil {
		return cleanup(err)
	}

	return name, nil
}

func openProject(p Project) error {
	if _, err := exec.LookPath("tmux"); err != nil {
		return fmt.Errorf("tmux is required: %w", err)
	}

	name, err := newTmux().ensure(p)

	if err != nil {
		return err
	}

	action := "attach-session"

	if os.Getenv("TMUX") != "" {
		action = "switch-client"
	}

	cmd := exec.Command("tmux", action, "-t", "="+name)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}
