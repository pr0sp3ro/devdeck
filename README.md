# DevDeck

Open projects in tmux with a searchable project picker, favorites, and per-project startup windows.

DevDeck discovers projects from directories you choose. Projects can also be registered individually, with their own window names, working directories, and commands. Existing tmux sessions are reused, so opening a project again preserves your work.

## Requirements

- Go 1.22 or newer to build DevDeck.
- `tmux` on `PATH`.
- `fzf` on `PATH` for the interactive picker.
- A Unix environment with a POSIX shell and `sleep`.

## Installation

```sh
go build -o devdeck .
mkdir -p ~/.local/bin
install -m 755 devdeck ~/.local/bin/devdeck
```

Make sure `~/.local/bin` is on your `PATH`.

Optionally, use the Bash installer from the repository directory:

```sh
./install
```

It builds and installs DevDeck in `~/.local/bin` and creates an empty configuration at `$XDG_CONFIG_HOME/devdeck/config.toml`, or `~/.config/devdeck/config.toml` by default. Missing configuration directories are created automatically. Existing configuration is preserved when the installer is run again.

Alternatively, install directly with Go:

```sh
go install github.com/pr0sp3ro/devdeck@latest
```

Go installs the executable in `$GOBIN`, or `$(go env GOPATH)/bin` when `GOBIN` is unset. That directory must be on your `PATH`.

## Quick start

Add a directory containing projects:

```sh
devdeck source add ~/src
devdeck
```

Every immediate subdirectory is a project, including directories without Git repositories. Discovery is not recursive. Symlinks to directories are followed and duplicate paths appear once.

Register an individual project and mark it as a favorite:

```sh
devdeck project add devdeck ~/src/devdeck
devdeck favorite add devdeck
devdeck open devdeck
```

Open any directory directly, including one outside your configured sources:

```sh
devdeck open .
devdeck open ~/bin
```

### Zsh shortcut

Add this to `~/.zshrc` to open the project picker with `Ctrl-G`:

```zsh
devdeck-picker() {
    devdeck
    zle reset-prompt
}

zle -N devdeck-picker
bindkey '^G' devdeck-picker
```

Open a new shell or run `source ~/.zshrc` to activate the shortcut. Replace `^G` with your preferred key binding. DevDeck must be on your `PATH`.

## Project picker

Run `devdeck` without arguments to open the fzf picker.

| Key | Action |
| --- | --- |
| Type | Search project names and paths |
| Up / Down or Ctrl-N / Ctrl-P | Select a project |
| Enter | Open the selected project |
| Ctrl-F | Toggle its favorite status and refresh the list |
| Esc / Ctrl-C | Cancel without opening a session |

Favorites appear first. `*` marks a favorite and `+` marks an existing tmux session. Paths are displayed to distinguish projects with the same name. The search query is retained after toggling a favorite.

DevDeck supplies its own fzf options and ignores `FZF_DEFAULT_OPTS` and `FZF_DEFAULT_OPTS_FILE` to keep selection and keyboard bindings consistent.

## Configuration

The configuration file is `$XDG_CONFIG_HOME/devdeck/config.toml`, or `~/.config/devdeck/config.toml` by default. Use an absolute path for `XDG_CONFIG_HOME`.

Reading a missing configuration does not create a file. The optional `./install` script or the first configuration-changing command creates it. With no projects available, DevDeck prints instructions for adding a source or project.

```toml
[[sources]]
path = "~/src"

[[sources]]
path = "/srv/http"

[projects.example]
path = "~/src/example-app"
favorite = true
windows = [
    { name = "editor", command = "nvim" },
    { name = "terminal" },
    { name = "logs", command = "tail -f storage/logs/laravel.log" },
    { name = "terminal", command = "git status; exec zsh -i" },
]

[projects.devdeck]
path = "~/src/devdeck"
favorite = true
windows = [
    { name = "editor", command = "nvim" },
    { name = "terminal" },
    { name = "config", directory = "~/.config", command = "nvim" },
]
```

Each `[projects.<name>]` section configures one project. Project identifiers accept ASCII letters, numbers, underscores, and hyphens. The identifier is used by commands such as `devdeck open example`.

| Setting | Meaning |
| --- | --- |
| `path` | Required project directory |
| `favorite` | Whether the project appears first, default `false` |
| `windows` | Ordered list of startup windows, default one terminal window |
| Window `name` | Required window name |
| Window `command` | Optional startup command |
| Window `directory` | Optional working directory, default the project directory |

The number of windows comes from the list. An empty or omitted list opens one terminal window. Relative window directories are resolved against the project directory. All directories must exist when a new session is created. The example above opens the `config` window in `~/.config`, which must exist.

Paths support `~` and `~/...`. Use absolute project and source paths in manually edited configuration. CLI commands save canonical absolute paths. Relative project or source paths in a manually edited file resolve against the directory from which DevDeck is invoked. Environment variables in path fields are not expanded.

Projects discovered from sources use their directory names and open one terminal window unless the same canonical path has an explicit project configuration. Explicit settings take precedence for that path. Multiple configured project identifiers pointing to the same canonical directory are rejected during discovery.

Unavailable sources and projects are skipped with warnings in the picker and project listing. Opening an unavailable project fails with an error.

### Session behavior

DevDeck names each session using the sanitized directory basename and the first 16 hexadecimal characters of its full-path SHA-256 hash. Paths are made absolute and symlinks are resolved first. Projects with identical directory names in different locations therefore get different sessions.

When a session already exists, DevDeck attaches to it outside tmux or switches the current client inside tmux. It does not recreate windows or rerun commands. Configuration changes apply when the session is next created.

For new sessions, windows are created in configuration order and the first window is selected. Commands run as tmux startup commands, with shell syntax available for pipelines and redirection. Windows without a command use tmux's `default-command`, or its `default-shell` as a login shell when no default command is set.

Windows remain visible after their command or shell exits, preserving output and the exit status. Close them using tmux's normal window controls, or restart their process using `respawn-window -k` from the tmux command prompt.

All working directories are checked before creation. If window setup fails, DevDeck attempts to remove only the session created by that invocation. Commands that started before a later failure may already have had effects outside tmux.

### Editing settings

```sh
devdeck config get
devdeck config get projects.example.path
devdeck config set projects.example.favorite true
devdeck config set projects.example.windows.0.command 'nvim .'
devdeck config set sources.0.path /srv/http
```

Window indexes and source indexes are zero-based. String values are passed as ordinary shell arguments, without TOML quotes. Boolean values use `true` or `false`.

Replace the complete window list using a TOML array:

```sh
devdeck config set projects.example.windows '[{ name = "editor", command = "nvim" }, { name = "terminal" }]'
```

`config set` updates configured projects or existing source entries. Add a project with `project add` before setting its fields. Supported keys are `sources.<index>.path`, `projects.<name>.path`, `projects.<name>.favorite`, `projects.<name>.windows`, and `projects.<name>.windows.<index>.name|command|directory`.

Open the configuration in your editor:

```sh
devdeck config edit
devdeck project edit example
```

`project edit` checks that the project is configured and prints the relevant section before opening the complete file. The editor is selected from `$VISUAL`, then `$EDITOR`, then `vi`. Editor commands may include arguments. Configuration is validated after the editor exits. Invalid edits remain available for manual correction.

Configuration-changing CLI commands validate the current file before writing. Writes use an atomic replacement and may reformat TOML and remove comments. Unknown fields and invalid configuration cause an error without overwriting the file. Use `devdeck config edit` to open and repair invalid configuration. This command validates the file after the editor exits.

DevDeck serializes configuration changes using a `config.toml.lock` file next to the configuration's actual target, including when the configuration is a symlink. Parallel commands wait for the current update to finish before reading and changing settings. `Ctrl-F` locks only while updating a favorite. `config edit` and `project edit` hold the lock until the editor exits. The lock file remains on disk, but the lock is released automatically when the process exits. Editors launched directly outside DevDeck do not participate in locking.

## Command reference

```text
devdeck
devdeck open <name|path>
devdeck list

devdeck source add <path>
devdeck source remove <path>

devdeck project add <name> <path>
devdeck project show <name>
devdeck project edit <name>

devdeck favorite add <name|path>
devdeck favorite remove <name|path>

devdeck config get [key]
devdeck config set <key> <value>
devdeck config edit

devdeck -h
devdeck --help
devdeck --version
```

`list` prints project names and paths, with favorites first. `project show` prints the saved configuration for one project.

Adding a discovered project to favorites registers its path in the configuration. Characters outside ASCII letters, numbers, underscores, and hyphens are replaced with underscores in its saved identifier. For example, `example.app` is saved as `example_app`. Removing its favorite status keeps that project configuration and its window settings. If its saved identifier is already used for another path, register it with `project add <unique-name> <path>` first. An ambiguous name passed to `open` or `favorite` produces an error listing the matching paths. Use an absolute path to select one.

Remove a source without removing its projects or tmux sessions:

```sh
devdeck source remove ~/src
```

Explicit project configurations remain available after a source is removed. To remove an explicit project, delete its section from the configuration.

## Checks

Run the automated tests locally:

```sh
go test ./...
```

To check interactive behavior, open a project outside tmux and then from a tmux client. Confirm window order, working directories, startup commands, and the selected first window. Reopen the project to confirm existing windows remain unchanged. In the picker, toggle a favorite, check that it persists, and cancel with Esc to confirm no session is opened.

## License

DevDeck is available under the [MIT License](LICENSE).
