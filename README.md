# Dirhaven

Open projects in tmux with a searchable project picker, favorites, and per-project startup windows.

Dirhaven discovers projects from directories you choose. Projects can also be registered individually, with their own window names, working directories, and commands. Existing tmux sessions are reused, so opening a project again preserves your work.

## Requirements

- Go 1.22 or newer to build Dirhaven.
- `tmux` on `PATH`.
- `fzf` on `PATH` for the interactive picker.
- A Unix environment with a POSIX shell and `sleep`.

## Installation

```sh
go build -o dirhaven .
mkdir -p ~/.local/bin
install -m 755 dirhaven ~/.local/bin/dirhaven
```

Make sure `~/.local/bin` is on your `PATH`.

Optionally, use the Bash installer from the repository directory:

```sh
./install
```

It builds and installs Dirhaven in `~/.local/bin` and creates an empty configuration at `$XDG_CONFIG_HOME/dirhaven/config.toml`, or `~/.config/dirhaven/config.toml` by default. Missing configuration directories are created automatically. Existing configuration is preserved when the installer is run again.

Alternatively, install directly with Go:

```sh
go install github.com/pr0sp3ro/dirhaven@latest
```

Go installs the executable in `$GOBIN`, or `$(go env GOPATH)/bin` when `GOBIN` is unset. That directory must be on your `PATH`.

## Quick start

Add a directory containing projects:

```sh
dirhaven source add ~/src
dirhaven
```

Every immediate subdirectory is a project, including directories without Git repositories. Discovery is not recursive. Symlinks to directories are followed and duplicate paths appear once.

Register an individual project and mark it as a favorite:

```sh
dirhaven project add dirhaven ~/src/dirhaven
dirhaven favorite add dirhaven
dirhaven open dirhaven
```

Open any directory directly, including one outside your configured sources:

```sh
dirhaven open .
dirhaven open ~/bin
```

### Shell alias

Add this to your shell configuration (`~/.zshrc` or `~/.bashrc`) to use `dh` as a shorter command:

```sh
alias dh='dirhaven'
```

Open a new shell or reload your shell configuration, then run `dh` or `dh open .`.

### Zsh shortcut

Add this to `~/.zshrc` to open the project picker with `Ctrl-G`:

```zsh
dirhaven-picker() {
    dirhaven
    zle reset-prompt
}

zle -N dirhaven-picker
bindkey '^G' dirhaven-picker
```

Open a new shell or run `source ~/.zshrc` to activate the shortcut. Replace `^G` with your preferred key binding. Dirhaven must be on your `PATH`.

## Project picker

Run `dirhaven` without arguments to open the fzf picker.

| Key | Action |
| --- | --- |
| Type | Search project names and paths |
| Up / Down or Ctrl-N / Ctrl-P | Select a project |
| Enter | Open the selected project |
| Ctrl-F | Toggle its favorite status and refresh the list |
| Esc / Ctrl-C | Cancel without opening a session |

Favorites appear first. `*` marks a favorite and `+` marks an existing tmux session. Paths are displayed to distinguish projects with the same name. The search query is retained after toggling a favorite.

Dirhaven supplies its own fzf options and ignores `FZF_DEFAULT_OPTS` and `FZF_DEFAULT_OPTS_FILE` to keep selection and keyboard bindings consistent.

## Configuration

The configuration file is `$XDG_CONFIG_HOME/dirhaven/config.toml`, or `~/.config/dirhaven/config.toml` by default. Use an absolute path for `XDG_CONFIG_HOME`.

Reading a missing configuration does not create a file. The optional `./install` script or the first configuration-changing command creates it. With no projects available, Dirhaven prints instructions for adding a source or project.

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

[projects.dirhaven]
path = "~/src/dirhaven"
favorite = true
windows = [
    { name = "editor", command = "nvim" },
    { name = "terminal" },
    { name = "config", directory = "~/.config", command = "nvim" },
]
```

Each `[projects.<name>]` section configures one project. Project identifiers accept ASCII letters, numbers, underscores, and hyphens. The identifier is used by commands such as `dirhaven open example`.

| Setting | Meaning |
| --- | --- |
| `path` | Required project directory |
| `favorite` | Whether the project appears first, default `false` |
| `windows` | Ordered list of startup windows, default one terminal window |
| Window `name` | Required window name |
| Window `command` | Optional startup command |
| Window `directory` | Optional working directory, default the project directory |

The number of windows comes from the list. An empty or omitted list opens one terminal window. Relative window directories are resolved against the project directory. All directories must exist when a new session is created. The example above opens the `config` window in `~/.config`, which must exist.

Paths support `~` and `~/...`. Use absolute project and source paths in manually edited configuration. CLI commands save canonical absolute paths. Relative project or source paths in a manually edited file resolve against the directory from which Dirhaven is invoked. Environment variables in path fields are not expanded.

Window names accept only ASCII letters, digits, spaces, underscores, hyphens, and periods. Source, project, and window directory paths additionally allow `/` and `~`. Resolved symlink targets must follow the same path rules. Other characters, including semicolons, quotes, brackets, and control characters, are rejected. Discovered projects with unsupported paths are skipped. These restrictions do not apply to the shell syntax in `command`, so commands such as `git status; exec zsh -i` remain supported.

Projects discovered from sources use their directory names and open one terminal window unless the same canonical path has an explicit project configuration. Explicit settings take precedence for that path. Multiple configured project identifiers pointing to the same canonical directory are rejected when configuration is loaded, saved, or validated after editing.

Unavailable sources and projects are skipped with warnings in the picker and project listing. Opening an unavailable project fails with an error.

### Session behavior

Dirhaven names each session using the sanitized directory basename and the first 16 hexadecimal characters of its full-path SHA-256 hash. Paths are made absolute and symlinks are resolved first. Projects with identical directory names in different locations therefore get different sessions.

When a session already exists, Dirhaven attaches to it outside tmux or switches the current client inside tmux. It does not recreate windows or rerun commands. Configuration changes apply when the session is next created.

For new sessions, windows are created in configuration order and the first window is selected. Commands run as tmux startup commands, with shell syntax available for pipelines and redirection. Windows without a command use tmux's `default-command`, or its `default-shell` as a login shell when no default command is set.

Windows remain visible after their command or shell exits, preserving output and the exit status. Close them using tmux's normal window controls, or restart their process using `respawn-window -k` from the tmux command prompt.

All working directories are checked before creation. If window setup fails, Dirhaven attempts to remove only the session created by that invocation. Commands that started before a later failure may already have had effects outside tmux.

### Editing settings

```sh
dirhaven config get
dirhaven config get projects.example.path
dirhaven config set projects.example.favorite true
dirhaven config set projects.example.windows.0.command 'nvim .'
dirhaven config set sources.0.path /srv/http
```

Window indexes and source indexes are zero-based. String values are passed as ordinary shell arguments, without TOML quotes. Boolean values use `true` or `false`.

Replace the complete window list using a TOML array:

```sh
dirhaven config set projects.example.windows '[{ name = "editor", command = "nvim" }, { name = "terminal" }]'
```

`config set` updates configured projects or existing source entries. Add a project with `project add` before setting its fields. Supported keys are `sources.<index>.path`, `projects.<name>.path`, `projects.<name>.favorite`, `projects.<name>.windows`, and `projects.<name>.windows.<index>.name|command|directory`.

Open the configuration in your editor:

```sh
dirhaven config edit
dirhaven project edit example
```

`project edit` checks that the project is configured and prints the relevant section before opening the complete file. The editor is selected from `$VISUAL`, then `$EDITOR`, then `vi`. Editor commands may include arguments. Configuration is validated after the editor exits. Invalid edits remain available for manual correction.

Configuration-changing CLI commands validate the current file before writing. Writes use an atomic replacement and may reformat TOML and remove comments. Unknown fields and invalid configuration cause an error without overwriting the file. Use `dirhaven config edit` to open and repair invalid configuration. This command validates the file after the editor exits.

Dirhaven serializes configuration changes using lock files in `$XDG_STATE_HOME/dirhaven/locks`, defaulting to `~/.local/state/dirhaven/locks`. `XDG_STATE_HOME` must be an absolute path. Each lock filename is a SHA-256 hash of the canonical configuration path, so symlinks to the same file share a lock. Reading configuration does not require write access to its directory, but the state directory must be writable. Parallel commands wait for the current update to finish before reading and changing settings. Read-only commands and the picker use shared locks while loading configuration, so they also wait until an editor launched by Dirhaven exits. `Ctrl-F` locks only while updating a favorite. `config edit` and `project edit` hold the lock until the editor exits. The lock file remains on disk, but the lock is released automatically when the process exits. Editors launched directly outside Dirhaven do not participate in locking. When upgrading from a version that stores `config.toml.lock` beside the configuration, close its running configuration commands and editors before using the new version, because the lock locations differ.

## Command reference

```text
dirhaven
dirhaven open <name|path>
dirhaven list

dirhaven source add <path>
dirhaven source remove <path>

dirhaven project add <name> <path>
dirhaven project show <name>
dirhaven project edit <name>

dirhaven favorite add <name|path>
dirhaven favorite remove <name|path>

dirhaven config get [key]
dirhaven config set <key> <value>
dirhaven config edit

dirhaven -h
dirhaven --help
dirhaven --version
```

`list` prints project names and paths, with favorites first. `project show` prints the saved configuration for one project.

Adding a discovered project to favorites registers its path in the configuration. Characters outside ASCII letters, numbers, underscores, and hyphens are replaced with underscores in its saved identifier. For example, `example.app` is saved as `example_app`. Removing its favorite status keeps that project configuration and its window settings. If its saved identifier is already used for another path, register it with `project add <unique-name> <path>` first. An ambiguous name passed to `open` or `favorite` produces an error listing the matching paths. Use an absolute path to select one.

Remove a source without removing its projects or tmux sessions:

```sh
dirhaven source remove ~/src
```

Explicit project configurations remain available after a source is removed. To remove an explicit project, delete its section from the configuration.

## Checks

Run the automated tests locally:

```sh
go test ./...
```

To check interactive behavior, open a project outside tmux and then from a tmux client. Confirm window order, working directories, startup commands, and the selected first window. Reopen the project to confirm existing windows remain unchanged. In the picker, toggle a favorite, check that it persists, and cancel with Esc to confirm no session is opened.

## License

Dirhaven is available under the [MIT License](LICENSE).
