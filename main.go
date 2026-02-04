package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const version = "1.5.5"

func getShellConfig() (string, string) {
	shell := os.Getenv("SHELL")
	home, _ := os.UserHomeDir()

	switch {
	case strings.Contains(shell, "zsh"):
		return filepath.Join(home, ".zshrc"), "zsh"
	case strings.Contains(shell, "bash"):
		// Check for .bash_profile first (macOS), then .bashrc
		bashProfile := filepath.Join(home, ".bash_profile")
		if _, err := os.Stat(bashProfile); err == nil {
			return bashProfile, "bash"
		}
		return filepath.Join(home, ".bashrc"), "bash"
	case strings.Contains(shell, "fish"):
		return filepath.Join(home, ".config", "fish", "config.fish"), "fish"
	default:
		return filepath.Join(home, ".bashrc"), "bash"
	}
}

func getShellFunction(shellType string) string {
	gotoFile := getGotoFilePath()

	// Use "command guppi" to call the binary, not the shell function (avoids recursion)
	binaryPath := "command guppi"

	switch shellType {
	case "fish":
		return fmt.Sprintf(`
# guppi - git repository manager
function guppi
  %s
  if test -f "%s"
    set goto_path (cat "%s")
    rm -f "%s"
    if test -d "$goto_path"
      cd "$goto_path"
    end
  end
end
alias gpi guppi
`, binaryPath, gotoFile, gotoFile, gotoFile)
	default: // bash/zsh
		return fmt.Sprintf(`
# guppi - git repository manager
guppi() {
  %s
  if [[ -f "%s" ]]; then
    local goto_path
    goto_path=$(cat "%s")
    rm -f "%s"
    if [[ -d "$goto_path" ]]; then
      cd "$goto_path"
    fi
  fi
}
alias gpi=guppi
`, binaryPath, gotoFile, gotoFile, gotoFile)
	}
}

func checkShellSetup() bool {
	rcPath, _ := getShellConfig()
	data, err := os.ReadFile(rcPath)
	if err != nil {
		return false
	}
	return strings.Contains(string(data), "guppi()")
}

// checkShellNeedsUpdate returns true if the shell function has issues needing update
func checkShellNeedsUpdate() bool {
	rcPath, _ := getShellConfig()
	data, err := os.ReadFile(rcPath)
	if err != nil {
		return false
	}
	content := string(data)
	if !strings.Contains(content, "guppi()") {
		return false
	}
	// Check for hardcoded paths
	if strings.Contains(content, "/Cellar/guppi/") || strings.Contains(content, "/bin/guppi") {
		return true
	}
	// Check for recursive call (guppi without "command" prefix)
	// Look for the pattern: guppi() { followed by just "guppi" without "command"
	if strings.Contains(content, "guppi()") && !strings.Contains(content, "command guppi") {
		return true
	}
	// Check for missing gpi alias
	if !strings.Contains(content, "alias gpi") {
		return true
	}
	return false
}

// updateShellFunctionInPlace replaces the old guppi function with the new one
func updateShellFunctionInPlace() error {
	rcPath, shellType := getShellConfig()
	data, err := os.ReadFile(rcPath)
	if err != nil {
		return err
	}

	content := string(data)
	newFunc := getShellFunction(shellType)

	// Find function start - look for "guppi() {" or "function guppi"
	var startMarker string
	if shellType == "fish" {
		startMarker = "function guppi"
	} else {
		startMarker = "guppi() {"
	}

	startIdx := strings.Index(content, startMarker)
	if startIdx == -1 {
		return fmt.Errorf("function not found")
	}

	// Include comment line before function if present
	if startIdx > 0 {
		beforeFunc := content[:startIdx]
		lastNewline := strings.LastIndex(beforeFunc, "\n")
		if lastNewline != -1 {
			prevLine := strings.TrimSpace(beforeFunc[lastNewline:])
			if strings.HasPrefix(prevLine, "# guppi") {
				startIdx = lastNewline + 1
			}
		}
	}

	// Find end of function block
	endIdx := startIdx
	remaining := content[startIdx:]
	lines := strings.Split(remaining, "\n")
	braceCount := 0
	foundEnd := false

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		if strings.Contains(line, "{") {
			braceCount++
		}
		if strings.Contains(line, "}") {
			braceCount--
			if braceCount == 0 {
				// End of function
				endIdx = startIdx
				for j := 0; j <= i; j++ {
					endIdx += len(lines[j]) + 1
				}
				// Check for alias line after
				if i+1 < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[i+1]), "alias gpi") {
					endIdx += len(lines[i+1]) + 1
				}
				foundEnd = true
				break
			}
		}
		if shellType == "fish" && trimmed == "end" {
			endIdx = startIdx
			for j := 0; j <= i; j++ {
				endIdx += len(lines[j]) + 1
			}
			if i+1 < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[i+1]), "alias gpi") {
				endIdx += len(lines[i+1]) + 1
			}
			foundEnd = true
			break
		}
	}

	if !foundEnd {
		return fmt.Errorf("could not find end of function")
	}

	// Replace old function with new
	newContent := content[:startIdx] + newFunc + content[endIdx:]
	return os.WriteFile(rcPath, []byte(newContent), 0644)
}

func getCurrentBinaryPath() string {
	binaryPath, err := os.Executable()
	if err != nil {
		return ""
	}
	binaryPath, _ = filepath.EvalSymlinks(binaryPath)
	return binaryPath
}

func updateShellFunction() {
	config := loadConfig()
	currentPath := getCurrentBinaryPath()
	rcPath, _ := getShellConfig()

	successStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("241"))

	// Check if shell function needs a full update (missing alias, hardcoded paths, etc.)
	if checkShellNeedsUpdate() {
		fmt.Fprintln(os.Stderr, "Updating shell function...")
		if err := updateShellFunctionInPlace(); err != nil {
			fmt.Fprintf(os.Stderr, "Error updating %s: %v\n", rcPath, err)
			fmt.Fprintln(os.Stderr, "Run 'guppi --setup' to fix manually.")
		} else {
			fmt.Fprintln(os.Stderr, successStyle.Render("✓ Shell function updated"))
			fmt.Fprintf(os.Stderr, dimStyle.Render("  Run: source %s\n"), rcPath)
			fmt.Fprintln(os.Stderr)
		}
		config.BinaryPath = currentPath
		saveConfigFull(config)
		return
	}

	// If no saved path or path hasn't changed, nothing to do
	if config.BinaryPath == "" || config.BinaryPath == currentPath {
		// Save current path if not set
		if config.BinaryPath == "" && currentPath != "" {
			config.BinaryPath = currentPath
			saveConfigFull(config)
		}
		return
	}

	// Binary path changed - update shell function
	data, err := os.ReadFile(rcPath)
	if err != nil {
		return
	}

	content := string(data)

	// Check if old path is in the shell config
	if !strings.Contains(content, config.BinaryPath) {
		// Old path not found, just update config
		config.BinaryPath = currentPath
		saveConfigFull(config)
		return
	}

	fmt.Fprintln(os.Stderr, "guppi binary location changed, updating shell function...")

	// Replace old path with new path in shell config
	newContent := strings.Replace(content, config.BinaryPath, currentPath, -1)

	if err := os.WriteFile(rcPath, []byte(newContent), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error updating %s: %v\n", rcPath, err)
		fmt.Fprintln(os.Stderr, "Run 'guppi --setup' to fix manually.")
	} else {
		fmt.Fprintln(os.Stderr, successStyle.Render("✓ Shell function updated"))
		fmt.Fprintf(os.Stderr, dimStyle.Render("  Run: source %s\n"), rcPath)
		fmt.Fprintln(os.Stderr)
	}

	// Save new binary path
	config.BinaryPath = currentPath
	saveConfigFull(config)
}

func runFirstTimeSetup(force bool) bool {
	config := loadConfig()
	if config.SetupComplete && !force {
		return true
	}

	// Check if shell already has guppi configured
	shellAlreadySetup := checkShellSetup()
	if shellAlreadySetup && config.GitDir != "" && !force {
		config.SetupComplete = true
		saveConfigFull(config)
		return true
	}

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
	successStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	featureStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("39"))

	// Welcome message
	fmt.Fprintln(os.Stderr, titleStyle.Render("Welcome to guppi! 🚀"))
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "A TUI for managing your git repositories.")
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, featureStyle.Render("Features:"))
	fmt.Fprintln(os.Stderr, "  • View all repos with status, branch, and remote changes")
	fmt.Fprintln(os.Stderr, "  • Pull repos individually or all favorites at once")
	fmt.Fprintln(os.Stderr, "  • Filter by dirty repos or repos behind remote")
	fmt.Fprintln(os.Stderr, "  • Switch branches, run commands, open lazygit")
	fmt.Fprintln(os.Stderr, "  • Press 'g' to cd into a repo (requires shell setup)")
	fmt.Fprintln(os.Stderr)

	rcPath, shellType := getShellConfig()

	// Step 1: Git directory setup
	fmt.Fprintln(os.Stderr, titleStyle.Render("Step 1: Git Directory"))
	fmt.Fprintln(os.Stderr, "Where are your git repositories located?")
	fmt.Fprintln(os.Stderr)

	home, _ := os.UserHomeDir()

	// Build list of options
	var options []string
	optionPaths := make(map[int]string)
	optNum := 1

	// Add current config if set
	if config.GitDir != "" {
		options = append(options, fmt.Sprintf("  [%d] %s (current)", optNum, config.GitDir))
		optionPaths[optNum] = config.GitDir
		optNum++
	}

	// Check common directories
	commonDirs := []string{
		filepath.Join(home, "git"),
		filepath.Join(home, "repos"),
		filepath.Join(home, "projects"),
		filepath.Join(home, "code"),
		filepath.Join(home, "src"),
		filepath.Join(home, "dev"),
	}

	for _, dir := range commonDirs {
		if dir == config.GitDir {
			continue // Already added as current
		}
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			options = append(options, fmt.Sprintf("  [%d] %s", optNum, dir))
			optionPaths[optNum] = dir
			optNum++
		}
	}

	// Always show custom option
	options = append(options, fmt.Sprintf("  [%d] Enter custom path", optNum))
	customOption := optNum

	for _, opt := range options {
		fmt.Fprintln(os.Stderr, opt)
	}
	fmt.Fprintln(os.Stderr)

	defaultChoice := "1"
	if len(optionPaths) == 0 {
		defaultChoice = fmt.Sprintf("%d", customOption)
	}
	fmt.Fprintf(os.Stderr, "Choose [%s]: ", defaultChoice)

	var choice string
	fmt.Scanln(&choice)
	choice = strings.TrimSpace(choice)
	if choice == "" {
		choice = defaultChoice
	}

	choiceNum, err := strconv.Atoi(choice)
	if err != nil || choiceNum < 1 || choiceNum > customOption {
		choiceNum, _ = strconv.Atoi(defaultChoice)
	}

	var gitPath string
	if choiceNum == customOption {
		fmt.Fprint(os.Stderr, "Enter path: ")
		fmt.Scanln(&gitPath)
		gitPath = strings.TrimSpace(gitPath)
		// Expand ~ to home directory
		if strings.HasPrefix(gitPath, "~/") {
			gitPath = filepath.Join(home, gitPath[2:])
		}
		if gitPath == "" {
			gitPath = filepath.Join(home, "git")
		}
	} else {
		gitPath = optionPaths[choiceNum]
	}

	if _, err := os.Stat(gitPath); os.IsNotExist(err) {
		fmt.Fprintln(os.Stderr, dimStyle.Render("Note: Directory doesn't exist yet. You can change it later with 'c' in the app."))
	} else {
		fmt.Fprintln(os.Stderr, successStyle.Render("✓ "+gitPath))
	}
	config.GitDir = gitPath
	fmt.Fprintln(os.Stderr)

	// Step 2: Shell function setup
	fmt.Fprintln(os.Stderr, titleStyle.Render("Step 2: Shell Integration"))
	if shellAlreadySetup && checkShellNeedsUpdate() {
		fmt.Fprintln(os.Stderr, "Existing shell function needs updating (has hardcoded path).")
		fmt.Fprintf(os.Stderr, "Update in %s? [Y/n] ", rcPath)

		var response string
		fmt.Scanln(&response)
		response = strings.ToLower(strings.TrimSpace(response))

		if response == "" || response == "y" || response == "yes" {
			if err := updateShellFunctionInPlace(); err != nil {
				fmt.Fprintf(os.Stderr, "Error updating: %v\n", err)
				fmt.Fprintln(os.Stderr, dimStyle.Render("You may need to manually update the function."))
			} else {
				fmt.Fprintln(os.Stderr, successStyle.Render("✓ Shell function updated"))
				fmt.Fprintf(os.Stderr, dimStyle.Render("  Run: source %s\n"), rcPath)
			}
		}
	} else if shellAlreadySetup {
		fmt.Fprintln(os.Stderr, successStyle.Render("✓ Already configured in "+rcPath))
	} else {
		fmt.Fprintln(os.Stderr, "To enable 'goto' feature (press 'g' to cd into a repo),")
		fmt.Fprintln(os.Stderr, "guppi needs to add a shell function to your config.")
		fmt.Fprintln(os.Stderr)
		fmt.Fprintf(os.Stderr, "Add to %s? [Y/n] ", rcPath)

		var response string
		fmt.Scanln(&response)
		response = strings.ToLower(strings.TrimSpace(response))

		if response == "" || response == "y" || response == "yes" {
			shellFunc := getShellFunction(shellType)
			f, err := os.OpenFile(rcPath, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: could not open %s: %v\n", rcPath, err)
				fmt.Fprintln(os.Stderr, dimStyle.Render("You can add it manually later."))
			} else {
				if _, err := f.WriteString(shellFunc); err != nil {
					fmt.Fprintf(os.Stderr, "Error writing: %v\n", err)
				} else {
					fmt.Fprintln(os.Stderr, successStyle.Render("✓ Shell function added"))
					fmt.Fprintf(os.Stderr, dimStyle.Render("  Run: source %s\n"), rcPath)
				}
				f.Close()
			}
		} else {
			fmt.Fprintln(os.Stderr, dimStyle.Render("Skipped. Run 'guppi --setup' to configure later."))
		}
	}
	fmt.Fprintln(os.Stderr)

	// Save config
	config.SetupComplete = true
	config.BinaryPath = getCurrentBinaryPath()
	saveConfigFull(config)

	fmt.Fprintln(os.Stderr, successStyle.Render("Setup complete! Starting guppi..."))
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "Note: If 'guppi' doesn't work in new terminals, reload your shell:")
	fmt.Fprintln(os.Stderr, dimStyle.Render("  source ~/.zshrc  (or ~/.bashrc)"))
	fmt.Fprintln(os.Stderr)
	return true
}

func printHelp() {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
	fmt.Println(titleStyle.Render("guppi") + " - Git Repository Manager TUI")
	fmt.Println()
	fmt.Println("Usage: guppi [options]")
	fmt.Println()
	fmt.Println("Options:")
	fmt.Println("  --help, -h      Show this help message")
	fmt.Println("  --version, -v   Show version")
	fmt.Println("  --setup         Re-run first-time setup")
	fmt.Println()
	fmt.Println("Environment:")
	fmt.Println("  GUPPI_GIT_DIR   Override git directory path")
	fmt.Println()
	fmt.Println("Key bindings (homepage):")
	fmt.Println("  Enter     Enter selected group / Pull selected repo")
	fmt.Println("  n         Create new group")
	fmt.Println("  e         Rename selected group")
	fmt.Println("  x         Delete selected group")
	fmt.Println("  m         Move repo to group")
	fmt.Println("  s         Open lazygit for selected repo")
	fmt.Println("  d         Open detail view (multi-pane)")
	fmt.Println("  f         Toggle favorite")
	fmt.Println("  p         Pull selected repo")
	fmt.Println("  P         Pull all favorites")
	fmt.Println("  A         Pull all repos behind remote")
	fmt.Println("  g         Goto repo directory (cd)")
	fmt.Println("  1         Filter: repos with local changes")
	fmt.Println("  2         Filter: repos behind remote")
	fmt.Println("  0         Clear filters")
	fmt.Println("  /         Search repos")
	fmt.Println("  r         Refresh (mode-aware: selected/favorites/all)")
	fmt.Println("  ctrl+r    Full refresh (always refreshes all)")
	fmt.Println("  c         Configure git directory")
	fmt.Println("  S         Open settings (performance options)")
	fmt.Println("  q         Quit")
	fmt.Println()
	fmt.Println("Key bindings (inside group):")
	fmt.Println("  Esc       Return to homepage")
	fmt.Println("  a         Add repos to group")
	fmt.Println("  x         Remove selected repo from group")
	fmt.Println("  m         Move repo to different group")
	fmt.Println("  r         Refresh repos in current group")
	fmt.Println("  (other keys work same as homepage)")
	fmt.Println()
	fmt.Println("Key bindings (detail view):")
	fmt.Println("  Tab       Switch pane (status/branches/command)")
	fmt.Println("  Enter     Switch branch / Run command")
	fmt.Println("  p         Pull remote branch to local")
	fmt.Println("  x         Delete local-only branch")
	fmt.Println("  X         Force delete local branch")
	fmt.Println("  r         Refresh")
	fmt.Println("  Esc       Back to list")
	fmt.Println()
	fmt.Println("Fetch Mode Settings (press S):")
	fmt.Println("  Fetch all       Fetch status for all repos on startup (default)")
	fmt.Println("  On-demand       No auto-fetch; press 'r' to refresh selected repo only")
	fmt.Println("  Favorites only  Only fetch status for favorite repos on startup")
}

func main() {
	// Handle flags
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "--help", "-h":
			printHelp()
			return
		case "--version", "-v":
			fmt.Printf("guppi version %s\n", version)
			return
		case "--setup":
			if !runFirstTimeSetup(true) {
				os.Exit(1)
			}
			return
		}
	}

	// Ensure config directory exists
	os.MkdirAll(getConfigDir(), 0755)

	// Run first-time setup if needed
	if !runFirstTimeSetup(false) {
		os.Exit(0)
	}

	// Check if binary path changed (e.g., installed via Homebrew after local build)
	updateShellFunction()

	// Priority: ENV > config file > default
	config := loadConfig()
	gitDir := os.Getenv("GUPPI_GIT_DIR")
	if gitDir == "" {
		gitDir = config.GitDir
	}
	if gitDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			fmt.Fprintln(os.Stderr, "Error: could not find home directory")
			os.Exit(1)
		}
		gitDir = filepath.Join(home, "git")
	}

	// Expand ~ in git directory
	if strings.HasPrefix(gitDir, "~/") {
		home, _ := os.UserHomeDir()
		gitDir = filepath.Join(home, gitDir[2:])
	}

	if _, err := os.Stat(gitDir); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "Git directory not found: %s\n", gitDir)
		fmt.Fprintln(os.Stderr, "Run 'guppi --setup' to configure or press 'c' in the app")
		os.Exit(1)
	}

	// Clean up any old goto file
	os.Remove(getGotoFilePath())

	p := tea.NewProgram(initialModel(gitDir), tea.WithAltScreen())
	finalModel, err := p.Run()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error running program:", err)
		os.Exit(1)
	}

	// If user pressed 'g' to goto a repo, write path to file for shell wrapper
	if m, ok := finalModel.(model); ok && m.gotoPath != "" {
		os.WriteFile(getGotoFilePath(), []byte(m.gotoPath), 0644)
	}
}
