# Uninstalling guppi

## Remove the Binary

### If installed via Homebrew

```bash
brew uninstall guppi
brew untap Quietscher/guppi
```

### If built from source

```bash
rm /path/to/guppi/guppi
# or if copied to PATH:
sudo rm /usr/local/bin/guppi
```

## Remove Configuration Files

```bash
rm -rf ~/.config/guppi
```

This removes:
- `config.json` - settings and git directory path
- `favorites.json` - your favorited repos
- `groups.json` - custom groups

## Remove Shell Function

Edit your `~/.zshrc` (or `~/.bashrc`) and remove the guppi function block:

```bash
# guppi - git repository manager
guppi() {
  /path/to/guppi
  if [[ -f "..." ]]; then
    ...
  fi
}
```

Then reload your shell:

```bash
source ~/.zshrc
```

## Complete One-Liner

For Homebrew installs:

```bash
brew uninstall guppi && brew untap Quietscher/guppi && rm -rf ~/.config/guppi && sed -i '' '/# guppi/,/^}/d' ~/.zshrc && source ~/.zshrc
```
