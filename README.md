# snip-cli

Terminal snippet manager written in Go. Search, copy, run and manage shell snippets.

## Install

```bash
go install github.com/marnguye/snip-cli/cmd/snip@latest
```

From a clone:

```bash
make build
```

## Usage

| Command                                       | Description                               |
| --------------------------------------------- | ----------------------------------------- |
| `snip`                                        | Interactive picker over all snippets      |
| `snip <file.md>`                              | Interactive picker for one group          |
| `snip find <query>`                           | Picker pre-filtered by query              |
| `snip run <description>`                      | Run a snippet by description              |
| `snip add <file.md> <description> <command>`  | Add a snippet                             |
| `snip edit <file.md> <description> <command>` | Replace a snippet command                 |
| `snip remove <file.md> <description>`         | Remove a snippet                          |
| `snip migrate [--dry-run]`                    | Migrate Markdown snippets to TOML storage |
| `snip storage status`                         | Show the active storage backend           |
| `snip --version`                              | Print the version                         |
| `snip storage use <toml\|legacy> [--confirm]` | Select the storage backend                |

The picker opens with search focused. Type to filter by title, command, group and tags; results are ranked by relevance.

| Key                                | Action                                   |
| ---------------------------------- | ---------------------------------------- |
| `up` / `down`, `ctrl+k` / `ctrl+j` | Move selection                           |
| `enter`                            | Copy selected command                    |
| `ctrl+r`                           | Run selected command                     |
| `ctrl+n` / `ctrl+e` / `ctrl+d`     | Add / edit / delete                      |
| `esc`                              | Clear query, then quit (cancel in forms) |
| `ctrl+c`                           | Quit                                     |

Commands may contain `<placeholder>` tokens; values are prompted for before copy or execution.
Values are substituted verbatim into a `sh -c` command line without quoting, so a value such as `x; other-command` runs both commands.
Only enter values you trust. Safe quoting depends on where the placeholder sits in the command and is not implemented yet.

## Storage

- Markdown (legacy): `~/.config/snip/snippets/*.md`, each snippet is a `## description` line followed by one command line.
- TOML: one file per snippet in `$XDG_DATA_HOME/snip/snippets` (default `~/.local/share/snip/snippets`).
- Active backend is stored in `$XDG_CONFIG_HOME/snip/state.toml` (default `~/.config/snip/state.toml`). Without it, Markdown is used when `.md` files exist, otherwise TOML.

Writes to TOML storage are serialized with an OS file lock (`flock`) and time out after 10 seconds. The lock is released automatically when a process exits.

## Platforms

macOS and Linux. Snippets run through `sh -c`, and locking uses `flock`.

## Shell completion

Files are in `scripts/`: `snip.bash`, `_snip` (zsh), `snip.fish`.

## Development

```bash
go vet ./...
go test ./...
go build ./cmd/snip
```
