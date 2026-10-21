package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

type Window struct {
	Name      string `toml:"name"`
	Command   string `toml:"command,omitempty"`
	Directory string `toml:"directory,omitempty"`
}

type Project struct {
	Path     string   `toml:"path"`
	Favorite bool     `toml:"favorite,omitempty"`
	Windows  []Window `toml:"windows,omitempty"`
	Name     string   `toml:"-"`
}

type Source struct {
	Path string `toml:"path"`
}

type Config struct {
	Sources  []Source           `toml:"sources,omitempty"`
	Projects map[string]Project `toml:"projects,omitempty"`
}

var validName = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
var validWindowName = regexp.MustCompile(`^[A-Za-z0-9 _.-]+$`)
var validPath = regexp.MustCompile(`^[A-Za-z0-9 _./~-]+$`)

func configPath() (string, error) {
	dir := os.Getenv("XDG_CONFIG_HOME")

	if dir == "" {
		home, err := os.UserHomeDir()

		if err != nil {
			return "", err
		}

		dir = filepath.Join(home, ".config")
	}

	if !filepath.IsAbs(dir) {
		return "", fmt.Errorf("XDG_CONFIG_HOME must be an absolute path")
	}

	return filepath.Join(dir, "dirhaven", "config.toml"), nil
}

func loadConfig(path string) (Config, error) {
	c := Config{Projects: map[string]Project{}}
	data, err := os.ReadFile(path)

	if os.IsNotExist(err) {
		return c, nil
	}

	if err != nil {
		return c, err
	}

	if err = toml.NewDecoder(bytes.NewReader(data)).DisallowUnknownFields().Decode(&c); err != nil {
		return c, fmt.Errorf("invalid config %s: %w", path, err)
	}

	if c.Projects == nil {
		c.Projects = map[string]Project{}
	}

	return c, c.validate()
}

func (c Config) validate() error {
	for _, s := range c.Sources {
		if strings.TrimSpace(s.Path) == "" {
			return fmt.Errorf("source path cannot be empty")
		}

		if _, err := expandPath(s.Path); err != nil {
			return fmt.Errorf("source: %w", err)
		}
	}

	paths := map[string]string{}

	for name, p := range c.Projects {
		if !validName.MatchString(name) {
			return fmt.Errorf("invalid project name %q: use letters, numbers, underscores or hyphens", name)
		}

		if strings.TrimSpace(p.Path) == "" {
			return fmt.Errorf("project %s has an empty path", name)
		}

		if _, err := expandPath(p.Path); err != nil {
			return fmt.Errorf("project %s: %w", name, err)
		}

		path, err := canonicalPath(p.Path)

		if err == nil {
			if existing, ok := paths[path]; ok {
				return fmt.Errorf("projects %q and %q use the same directory: %s", existing, name, path)
			}

			paths[path] = name
		}

		for _, w := range p.Windows {
			if strings.ContainsRune(w.Name+w.Command+w.Directory, 0) {
				return fmt.Errorf("project %s has a NUL character in its window settings", name)
			}

			if strings.TrimSpace(w.Name) == "" || !validWindowName.MatchString(w.Name) {
				return fmt.Errorf("project %s: invalid window name %q, use ASCII letters, digits, spaces, _, - or .", name, w.Name)
			}

			if w.Directory != "" {
				if _, err := expandPath(w.Directory); err != nil {
					return fmt.Errorf("project %s, window %s: %w", name, w.Name, err)
				}
			}
		}
	}

	return nil
}

func saveConfig(path string, c Config) error {
	if err := c.validate(); err != nil {
		return err
	}

	data, err := toml.Marshal(c)

	if err != nil {
		return err
	}

	if _, err := os.Lstat(path); err == nil {
		resolved, err := filepath.EvalSymlinks(path)

		if err != nil {
			return fmt.Errorf("resolve configuration path %s: %w", path, err)
		}

		path = resolved
	} else if !os.IsNotExist(err) {
		return err
	}

	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}

	f, err := os.CreateTemp(filepath.Dir(path), ".config-*")

	if err != nil {
		return err
	}

	defer os.Remove(f.Name())

	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}

	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}

	if err = f.Close(); err != nil {
		return err
	}

	return os.Rename(f.Name(), path)
}

func expandPath(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("path cannot be empty")
	}

	if !validPath.MatchString(path) {
		return "", fmt.Errorf("invalid path %q: use ASCII letters, digits, spaces, _, -, ., / or ~", path)
	}

	if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()

		if err != nil {
			return "", err
		}

		path = filepath.Join(home, strings.TrimPrefix(path, "~"))
	}

	return filepath.Abs(path)
}

func canonicalPath(path string) (string, error) {
	path, err := expandPath(path)

	if err != nil {
		return "", err
	}

	path, err = filepath.EvalSymlinks(path)

	if err != nil {
		return "", err
	}

	if !validPath.MatchString(path) {
		return "", fmt.Errorf("invalid resolved path %q: use ASCII letters, digits, spaces, _, -, ., / or ~", path)
	}

	info, err := os.Stat(path)

	if err != nil {
		return "", err
	}

	if !info.IsDir() {
		return "", fmt.Errorf("not a directory: %s", path)
	}

	return path, nil
}

func projects(c Config) ([]Project, error) {
	result := []Project{}
	seen := map[string]bool{}
	names := make([]string, 0, len(c.Projects))

	for name := range c.Projects {
		names = append(names, name)
	}

	sort.Strings(names)

	for _, name := range names {
		p := c.Projects[name]
		p.Name = name
		path, err := canonicalPath(p.Path)

		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: project %s: %v\n", name, err)
			continue
		}

		if seen[path] {
			return nil, fmt.Errorf("multiple configured projects use %s", path)
		}

		p.Path = path
		seen[path] = true
		result = append(result, p)
	}

	for _, source := range c.Sources {
		root, err := canonicalPath(source.Path)

		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: source %s: %v\n", source.Path, err)
			continue
		}

		entries, err := os.ReadDir(root)

		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: source %s: %v\n", source.Path, err)
			continue
		}

		for _, entry := range entries {
			path, err := canonicalPath(filepath.Join(root, entry.Name()))

			if err != nil || seen[path] {
				continue
			}

			seen[path] = true
			result = append(result, Project{Name: entry.Name(), Path: path})
		}
	}

	sort.Slice(result, func(i, j int) bool {
		a, b := result[i], result[j]

		if a.Favorite != b.Favorite {
			return a.Favorite
		}

		if a.Name != b.Name {
			return a.Name < b.Name
		}

		return a.Path < b.Path
	})
	return result, nil
}

func resolveProject(c Config, value string) (Project, error) {
	if p, ok := c.Projects[value]; ok {
		if _, err := canonicalPath(p.Path); err != nil {
			return Project{}, fmt.Errorf("project %s: %w", value, err)
		}
	}

	list, err := projects(c)

	if err != nil {
		return Project{}, err
	}

	if value == "." || value == ".." || value == "~" || strings.ContainsRune(value, filepath.Separator) {
		path, err := canonicalPath(value)

		if err != nil {
			return Project{}, err
		}

		for _, p := range list {
			if p.Path == path {
				return p, nil
			}
		}

		return Project{Name: filepath.Base(path), Path: path}, nil
	}

	matches := []Project{}

	for _, p := range list {
		if p.Name == value {
			matches = append(matches, p)
		}
	}

	if len(matches) == 1 {
		return matches[0], nil
	}

	if len(matches) == 0 {
		return Project{}, fmt.Errorf("project %q not found", value)
	}

	paths := []string{}

	for _, p := range matches {
		paths = append(paths, p.Path)
	}

	return Project{}, fmt.Errorf("ambiguous project %q, use a path:\n%s", value, strings.Join(paths, "\n"))
}
