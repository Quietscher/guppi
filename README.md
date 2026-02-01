# guppi

A terminal UI for managing multiple git repositories.

![Go](https://img.shields.io/badge/Go-1.21+-00ADD8?style=flat&logo=go)

**Stop running `git pull` in 20 different terminals.** See all your repos at a glance, know what's changed, and pull everything with one keypress. Perfect for the morning "sync everything" routine.

![guppi demo](assets/demo.gif)

## Features

- **Repository Overview** - View all repos with status, branch, and remote changes at a glance
- **Bulk Operations** - Pull repos individually, all favorites, or all repos behind remote
- **Pull Results Screen** - See what changed after pulling: commits, files, expandable per-repo details
- **Groups** - Organize repos into custom groups for easier management
- **Smart Filtering** - Filter by name, dirty repos, or repos with pending updates
- **Branch Management** - Switch branches, create local tracking branches, delete local-only branches
- **Multi-pane Detail View** - See status, branches, and run commands in one view
- **Lazygit Integration** - Open lazygit for any repo with one keypress
- **Goto Feature** - Press `g` to cd into a repo directory
- **Performance Settings** - Configurable on-demand fetching for large repo collections

## Installation

### Homebrew (macOS/Linux)

```bash
brew tap Quietscher/guppi
brew install guppi
```

On first run, guppi will guide you through setup. Then reload your shell: `source ~/.zshrc`

### Build from Source

Requires [Go](https://go.dev/dl/) 1.21+

```bash
git clone git@github.com:Quietscher/guppi.git && cd guppi && go build -o guppi . && ./guppi --setup
```

Then reload your shell: `source ~/.zshrc` (or `~/.bashrc`)

### Optional: lazygit Integration

For the lazygit integration (`s` key), install [lazygit](https://github.com/jesseduffield/lazygit):

```bash
brew install lazygit
```

### Updating

```bash
cd /path/to/guppi
git pull
go build -o guppi .
```

## Usage

```bash
guppi              # Start the TUI
gpi                # Short alias (same as guppi)
guppi --setup      # Re-run setup wizard
guppi --help       # Show help and key bindings
guppi --version    # Show version
```

### Environment Variables

- `GUPPI_GIT_DIR` - Override the git repositories directory

## Key Bindings

### List View

| Key | Action |
|-----|--------|
| `s` | Open lazygit for selected repo |
| `d` | Open detail view (multi-pane) |
| `f` | Toggle favorite |
| `p` / `Enter` | Pull selected repo |
| `P` | Pull all favorites |
| `A` | Pull all repos behind remote |
| `g` | Goto repo directory (cd) |
| `1` | Filter: repos with local changes |
| `2` | Filter: repos behind remote |
| `0` | Clear all filters |
| `/` | Search repos by name |
| `r` | Refresh (mode-aware: selected/favorites/all) |
| `ctrl+r` | Full refresh (always refreshes all repos) |
| `c` | Configure git directory |
| `S` | Open settings (performance options) |
| `n` | Create new group |
| `m` | Move repo to group |
| `o` | Open repo in browser |
| `q` | Quit |

### Groups

Groups let you organize repos into folders. On the homepage, groups appear as folders you can enter.

| Key | Action |
|-----|--------|
| `Enter` | Enter group |
| `n` | Create new group |
| `e` | Rename group |
| `x` | Delete group / Remove repo from group |
| `a` | Add repos to current group |
| `m` | Move repo to group |
| `Esc` | Exit group (back to homepage) |

### Pull Results Screen

After pulling multiple repos, guppi shows a summary screen with expandable details per repo.

| Key | Action |
|-----|--------|
| `↑/↓` | Navigate repos |
| `Enter/Space` | Expand/collapse commits |
| `a` | Expand/collapse all |
| `Esc` | Dismiss |

### Detail View

![detail view](assets/detail-view.gif)

| Key | Action |
|-----|--------|
| `Tab` | Switch pane (status/branches/command) |
| `↑/↓` | Scroll or select |
| `Enter` | Switch branch / Run command |
| `p` | Pull remote branch to local (create tracking) |
| `x` | Delete local-only branch |
| `X` | Force delete local branch |
| `r` | Refresh |
| `Esc` | Back to list |

### Branch Indicators

| Icon | Meaning |
|------|---------|
| `↕` | Local + Remote (synced) |
| `⚠` | Local only (no remote) |
| `☁` | Remote only (not checked out) |

## Status Indicators

- **Green ✓** - Clean, up to date with remote
- **Orange ↓** - Behind remote (can pull)
- **Orange ●** - Local changes (dirty)
- **Red ✗** - Error

## Configuration

Configuration is stored in `~/.config/guppi/`:

- `config.json` - Settings (git directory, performance options)
- `favorites.json` - List of favorite repositories

### Fetch Mode Settings

Press `S` in the list view to choose how guppi fetches repository status. Useful when managing many repositories:

| Mode | Description |
|------|-------------|
| Fetch all repos | Fetch all on startup; `r` refreshes all (default) |
| On-demand fetch | No auto-fetch; `r` refreshes selected, `ctrl+r` refreshes all |
| Favorites only | Fetch favorites on startup; `r` refreshes favorites, `ctrl+r` all |

## Dependencies

- [Bubble Tea](https://github.com/charmbracelet/bubbletea) - TUI framework
- [Bubbles](https://github.com/charmbracelet/bubbles) - TUI components
- [Lip Gloss](https://github.com/charmbracelet/lipgloss) - Style definitions

All dependencies are under permissive licenses (MIT/BSD 3-Clause).

## Uninstalling

See [UNINSTALL.md](UNINSTALL.md) for complete removal instructions.

## License

MIT
