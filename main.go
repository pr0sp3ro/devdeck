package main

import (
	"fmt"
	"os"
	"strings"
	"syscall"

	"github.com/pelletier/go-toml/v2"
)

var version = "dev"

const usage = `DevDeck - open projects in tmux

Usage:
  devdeck
  devdeck open <name|path>
  devdeck list
  devdeck source add|remove <path>
  devdeck project add <name> <path>
  devdeck project show|edit <name>
  devdeck favorite add|remove <name|path>
  devdeck config get [key]
  devdeck config set <key> <value>
  devdeck config edit
  devdeck -h|--help
  devdeck --version

Selector: Enter opens, Ctrl-F toggles favorite, Esc cancels.
Config: $XDG_CONFIG_HOME/devdeck/config.toml (default ~/.config/devdeck/config.toml).
`

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "devdeck:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 1 {
		switch args[0] {
		case "-h", "--help":
			fmt.Print(usage)
			return nil

		case "--version":
			fmt.Println("devdeck", version)
			return nil
		}
	}

	path, err := configPath()

	if err != nil {
		return err
	}

	writesConfig := false

	if len(args) >= 2 {
		switch args[0] {
		case "source", "favorite":
			writesConfig = len(args) == 3 && (args[1] == "add" || args[1] == "remove")

		case "project":
			writesConfig = (args[1] == "add" && len(args) == 4) || (args[1] == "edit" && len(args) == 3)

		case "config":
			writesConfig = (args[1] == "set" && len(args) == 4) || (args[1] == "edit" && len(args) == 2)
		}

		if writesConfig {
			lock, err := lockConfig(path, syscall.LOCK_EX)

			if err != nil {
				return err
			}

			defer lock.Close()
		}
	}

	if len(args) >= 2 && args[0] == "config" && args[1] == "edit" {
		if len(args) != 2 {
			return fmt.Errorf("usage: devdeck config edit")
		}

		return editConfig(path, Config{})
	}

	var c Config

	if writesConfig {
		c, err = loadConfig(path)
	} else {
		c, err = readConfig(path)
	}

	if err != nil {
		return err
	}

	if len(args) == 0 {
		return selectProject(path, c)
	}

	switch args[0] {
	case "open":
		if len(args) != 2 {
			return fmt.Errorf("usage: devdeck open <name|path>")
		}

		p, err := resolveProject(c, args[1])

		if err != nil {
			return err
		}

		return openProject(p)

	case "list":
		if len(args) != 1 {
			return fmt.Errorf("usage: devdeck list")
		}

		list, err := projects(c)

		if err != nil {
			return err
		}

		if len(list) == 0 {
			fmt.Println("No projects found. Use devdeck source add <path> or devdeck project add <name> <path>.")
			return nil
		}

		for _, p := range list {
			marker := " "

			if p.Favorite {
				marker = "*"
			}

			fmt.Printf("%s %s\t%s\n", marker, display(p.Name), display(p.Path))
		}

		return nil

	case "source":
		if len(args) != 3 || (args[1] != "add" && args[1] != "remove") {
			return fmt.Errorf("usage: devdeck source add|remove <path>")
		}

		target, err := expandPath(args[2])

		if err != nil {
			return err
		}

		if resolved, resolveErr := canonicalPath(args[2]); resolveErr == nil {
			target = resolved
		}

		if args[1] == "add" {
			target, err = canonicalPath(args[2])

			if err != nil {
				return err
			}
		}

		found := false
		sources := make([]Source, 0, len(c.Sources))

		for _, s := range c.Sources {
			resolved, err := canonicalPath(s.Path)

			if err != nil {
				resolved, err = expandPath(s.Path)

				if err != nil {
					return err
				}
			}

			same := resolved == target

			if same {
				found = true
			}

			if !same || args[1] == "add" {
				sources = append(sources, s)
			}
		}

		if args[1] == "add" && !found {
			sources = append(sources, Source{Path: target})
		}

		if args[1] == "remove" && !found {
			return fmt.Errorf("source %q not configured", args[2])
		}

		c.Sources = sources
		return saveConfig(path, c)

	case "project":
		if len(args) < 3 {
			return fmt.Errorf("usage: devdeck project add <name> <path> | show|edit <name>")
		}

		name := args[2]

		switch args[1] {
		case "add":
			if len(args) != 4 {
				return fmt.Errorf("usage: devdeck project add <name> <path>")
			}

			if !validName.MatchString(name) {
				return fmt.Errorf("invalid project name: use letters, numbers, underscores or hyphens")
			}

			if _, ok := c.Projects[name]; ok {
				return fmt.Errorf("project %q already configured", name)
			}

			target, err := canonicalPath(args[3])

			if err != nil {
				return err
			}

			c.Projects[name] = Project{Path: target}
			return saveConfig(path, c)

		case "show", "edit":
			if len(args) != 3 {
				return fmt.Errorf("usage: devdeck project show|edit <name>")
			}

			p, ok := c.Projects[name]

			if !ok {
				return fmt.Errorf("project %q not configured, use project add first", name)
			}

			if args[1] == "show" {
				return printTOML(p)
			}

			fmt.Fprintf(os.Stderr, "Editing %s, section [projects.%s]\n", path, name)
			return editConfig(path, c)
		}

	case "favorite":
		if len(args) != 3 || (args[1] != "add" && args[1] != "remove") {
			return fmt.Errorf("usage: devdeck favorite add|remove <name|path>")
		}

		// Allow removing a favorite even when its directory is no longer available.
		p, ok := c.Projects[args[2]]

		if ok && args[1] == "remove" {
			if _, pathErr := canonicalPath(p.Path); pathErr != nil {
				p.Favorite = false
				c.Projects[args[2]] = p
				return saveConfig(path, c)
			}
		}

		p, err = resolveProject(c, args[2])

		if err != nil {
			return err
		}

		if err = favorite(&c, p, args[1] == "add"); err != nil {
			return err
		}

		return saveConfig(path, c)

	case "config":
		if len(args) < 2 {
			return fmt.Errorf("usage: devdeck config get [key] | set <key> <value> | edit")
		}

		switch args[1] {
		case "get":
			if len(args) == 2 {
				return printTOML(c)
			}

			if len(args) != 3 {
				return fmt.Errorf("usage: devdeck config get [key]")
			}

			value, err := configValue(c, args[2])

			if err != nil {
				return err
			}

			switch v := value.(type) {
			case string:
				fmt.Println(v)

			case bool:
				fmt.Println(v)

			default:
				return printTOML(map[string]any{"value": value})
			}

			return nil

		case "set":
			if len(args) != 4 {
				return fmt.Errorf("usage: devdeck config set <key> <value>")
			}

			if err = setConfigValue(&c, args[2], args[3]); err != nil {
				return err
			}

			return saveConfig(path, c)

		}
	}

	return fmt.Errorf("unknown command %q\n%s", strings.Join(args, " "), usage)
}

func printTOML(value any) error {
	data, err := toml.Marshal(value)

	if err != nil {
		return err
	}

	_, err = os.Stdout.Write(data)
	return err
}
