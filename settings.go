package main

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

func configValue(c Config, key string) (any, error) {
	configured := map[string]any{}

	for name, p := range c.Projects {
		windows := make([]any, 0, len(p.Windows))

		for _, w := range p.Windows {
			windows = append(windows, map[string]any{
				"name":      w.Name,
				"command":   w.Command,
				"directory": w.Directory,
			})
		}

		configured[name] = map[string]any{
			"path":     p.Path,
			"favorite": p.Favorite,
			"windows":  windows,
		}
	}

	sources := make([]any, 0, len(c.Sources))

	for _, source := range c.Sources {
		sources = append(sources, map[string]any{"path": source.Path})
	}

	root := map[string]any{"projects": configured, "sources": sources}
	var value any = root

	for _, part := range strings.Split(key, ".") {
		switch node := value.(type) {
		case map[string]any:
			var ok bool
			value, ok = node[part]

			if !ok {
				return nil, fmt.Errorf("unknown setting %q", key)
			}

		case []any:
			i, err := strconv.Atoi(part)

			if err != nil || i < 0 || i >= len(node) {
				return nil, fmt.Errorf("invalid index in %q", key)
			}

			value = node[i]

		default:
			return nil, fmt.Errorf("unknown setting %q", key)
		}
	}

	return value, nil
}

func setConfigValue(c *Config, key, value string) error {
	parts := strings.Split(key, ".")

	if len(parts) == 3 && parts[0] == "sources" && parts[2] == "path" {
		i, err := strconv.Atoi(parts[1])

		if err != nil || i < 0 || i >= len(c.Sources) {
			return fmt.Errorf("source index out of range")
		}

		path, err := canonicalPath(value)

		if err != nil {
			return err
		}

		c.Sources[i].Path = path
		return nil
	}

	if len(parts) < 3 || parts[0] != "projects" {
		return fmt.Errorf("set supports sources.<index>.path, projects.<name>.path, favorite, windows, or windows.<index>.<field>")
	}

	p, ok := c.Projects[parts[1]]

	if !ok {
		return fmt.Errorf("project %q not configured", parts[1])
	}

	if len(parts) == 3 {
		switch parts[2] {
		case "path":
			path, err := canonicalPath(value)

			if err != nil {
				return err
			}

			p.Path = path

		case "favorite":
			if value != "true" && value != "false" {
				return fmt.Errorf("favorite must be true or false")
			}

			p.Favorite = value == "true"

		case "windows":
			var decoded struct {
				Windows []Window `toml:"windows"`
			}

			if err := toml.NewDecoder(strings.NewReader("windows = " + value)).DisallowUnknownFields().Decode(&decoded); err != nil {
				return err
			}

			p.Windows = decoded.Windows

		default:
			return fmt.Errorf("unknown setting %q", key)
		}
	} else if len(parts) == 5 && parts[2] == "windows" {
		i, err := strconv.Atoi(parts[3])

		if err != nil || i < 0 || i >= len(p.Windows) {
			return fmt.Errorf("window index out of range")
		}

		// Copy the slice so failed validation does not mutate the caller's config.
		p.Windows = append([]Window(nil), p.Windows...)

		switch parts[4] {
		case "name":
			p.Windows[i].Name = value

		case "command":
			p.Windows[i].Command = value

		case "directory":
			p.Windows[i].Directory = value

		default:
			return fmt.Errorf("unknown setting %q", key)
		}
	} else {
		return fmt.Errorf("unknown setting %q", key)
	}

	previous := c.Projects[parts[1]]
	c.Projects[parts[1]] = p

	if err := c.validate(); err != nil {
		c.Projects[parts[1]] = previous
		return err
	}

	return nil
}

func editConfig(path string, c Config) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err = saveConfig(path, c); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}

	editor := os.Getenv("VISUAL")

	if editor == "" {
		editor = os.Getenv("EDITOR")
	}

	if editor == "" {
		editor = "vi"
	}

	// Shell syntax is allowed for the user-controlled editor command, never the filename.
	cmd := exec.Command("sh", "-c", "exec "+editor+" \"$1\"", "devdeck-editor", path)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr

	if err := cmd.Run(); err != nil {
		return err
	}

	_, err := loadConfig(path)
	return err
}

func favorite(c *Config, p Project, enabled bool) error {
	name := strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-' {
			return r
		}

		return '_'
	}, p.Name)

	if existing, ok := c.Projects[name]; ok {
		path, err := canonicalPath(existing.Path)

		if err != nil {
			return err
		}

		if path != p.Path {
			return fmt.Errorf("name %q already configured, use project add <unique-name> <path>", name)
		}

		p = existing
	} else if !enabled {
		return nil
	}

	if !validName.MatchString(name) {
		return fmt.Errorf("use project add <name> <path> to assign a valid name first")
	}

	p.Name = ""
	p.Favorite = enabled
	c.Projects[name] = p
	return nil
}
