# guppi detail view demo - multi-pane inspection
Output assets/detail-view.gif

Set Shell zsh
Set Width 800
Set Height 500
Set FontSize 14
Set Theme "Catppuccin Mocha"
Set Padding 10

# Setup: source zshrc and clear terminal
Hide
Type "source ~/.zshrc"
Enter
Sleep 500ms
Type "clear"
Enter
Sleep 300ms
Show

Type "guppi"
Sleep 800ms
Enter

# Wait for repos to load
Sleep 2s

# Search for guppi
Type "/"
Sleep 300ms
Type "guppi"
Sleep 500ms
Enter
Sleep 1s

# Open detail view
Type "d"
Sleep 4500ms

# Back to list and exit
Escape
Sleep 200ms
Type "q"
Sleep 200ms
