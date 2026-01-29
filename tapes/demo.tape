# guppi hero demo - pull all repos behind remote
Output assets/demo.gif

Set Shell zsh
Set Width 800
Set Height 500
Set FontSize 14
Set Theme "Catppuccin Mocha"
Set Padding 10

# Setup: source zshrc and reset repos to be behind
Hide
Type "source ~/.zshrc"
Enter
Sleep 500ms
Type "cd ~/git/solvr && git reset --hard HEAD~1 && cd ~/git/solvr-web && git reset --hard HEAD~1 && cd ~/git/guppi"
Enter
Sleep 1s
Type "clear"
Enter
Sleep 300ms
Show

Type "guppi"
Sleep 500ms
Enter

# Wait for repos to load
Sleep 3s

# Search for solvr
Type "/"
Sleep 300ms
Type "solvr"
Sleep 500ms
Enter
Sleep 1s

# Pull all repos behind remote
Type "A"
Sleep 6s

# Exit
Type "q"
Sleep 500ms
