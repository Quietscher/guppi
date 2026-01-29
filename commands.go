package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

func scanForRepos(gitDir string) tea.Cmd {
	return func() tea.Msg {
		var repos []Repo

		entries, err := os.ReadDir(gitDir)
		if err != nil {
			return repoFoundMsg{repos: repos}
		}

		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}

			path := filepath.Join(gitDir, entry.Name())
			gitPath := filepath.Join(path, ".git")

			if info, err := os.Stat(gitPath); err == nil && info.IsDir() {
				repos = append(repos, Repo{
					Path:   path,
					Name:   entry.Name(),
					Status: StatusUnknown,
				})
			}
		}

		return repoFoundMsg{repos: repos}
	}
}

func checkGitStatus(path string) tea.Cmd {
	return func() tea.Msg {
		// Get branch name
		branchCmd := exec.Command("git", "-C", path, "rev-parse", "--abbrev-ref", "HEAD")
		branchOut, _ := branchCmd.Output()
		branch := strings.TrimSpace(string(branchOut))
		if branch == "" {
			branch = "?"
		}

		// Fetch from remote (silent, don't block on network issues)
		fetchCmd := exec.Command("git", "-C", path, "fetch", "--quiet")
		fetchCmd.Run() // ignore errors

		// Check how many commits behind remote
		behindCount := 0
		behindCmd := exec.Command("git", "-C", path, "rev-list", "--count", "HEAD..@{u}")
		behindOut, err := behindCmd.Output()
		if err == nil {
			if count, parseErr := strconv.Atoi(strings.TrimSpace(string(behindOut))); parseErr == nil {
				behindCount = count
			}
		}

		// Get local status
		cmd := exec.Command("git", "-C", path, "status", "--porcelain")
		output, err := cmd.Output()

		if err != nil {
			return statusUpdatedMsg{
				path:        path,
				branch:      branch,
				status:      StatusError,
				text:        "failed to get status",
				behindCount: 0,
			}
		}

		lines := strings.TrimSpace(string(output))
		if lines == "" {
			// Clean locally
			if behindCount > 0 {
				return statusUpdatedMsg{
					path:        path,
					branch:      branch,
					status:      StatusCleanBehind,
					text:        "",
					behindCount: behindCount,
				}
			}
			return statusUpdatedMsg{
				path:        path,
				branch:      branch,
				status:      StatusClean,
				text:        "",
				behindCount: 0,
			}
		}

		lineCount := len(strings.Split(lines, "\n"))
		return statusUpdatedMsg{
			path:        path,
			branch:      branch,
			status:      StatusDirty,
			text:        fmt.Sprintf("%d changed", lineCount),
			behindCount: behindCount,
		}
	}
}

func loadGitDetail(path string) tea.Cmd {
	return func() tea.Msg {
		var sb strings.Builder

		// Get full status
		statusCmd := exec.Command("git", "-C", path, "status", "--short", "--branch")
		statusOut, _ := statusCmd.Output()
		sb.WriteString("--- Status ---\n")
		sb.WriteString(string(statusOut))

		// If there are changes, show diff stat
		diffCmd := exec.Command("git", "-C", path, "diff", "--stat")
		diffOut, _ := diffCmd.Output()
		if len(diffOut) > 0 {
			sb.WriteString("\n--- Unstaged Changes ---\n")
			sb.WriteString(string(diffOut))
		}

		// Show staged diff stat
		stagedCmd := exec.Command("git", "-C", path, "diff", "--cached", "--stat")
		stagedOut, _ := stagedCmd.Output()
		if len(stagedOut) > 0 {
			sb.WriteString("\n--- Staged Changes ---\n")
			sb.WriteString(string(stagedOut))
		}

		// Show recent local commits
		logCmd := exec.Command("git", "-C", path, "log", "--oneline", "-10", "--pretty=format:%C(yellow)%h%C(reset) %s %C(dim)(%cr)%C(reset)")
		logOut, _ := logCmd.Output()
		if len(logOut) > 0 {
			sb.WriteString("\n--- Recent Commits ---\n")
			sb.WriteString(string(logOut))
			sb.WriteString("\n")
		}

		// Show incoming commits from remote (if any)
		incomingCmd := exec.Command("git", "-C", path, "log", "--oneline", "-10", "--pretty=format:%C(green)%h%C(reset) %s %C(dim)(%cr)%C(reset)", "HEAD..@{u}")
		incomingOut, _ := incomingCmd.Output()
		if len(incomingOut) > 0 {
			sb.WriteString("\n--- Incoming from Remote ---\n")
			sb.WriteString(string(incomingOut))
			sb.WriteString("\n")
		}

		return detailLoadedMsg{
			path:    path,
			content: sb.String(),
		}
	}
}

func pullRepo(path string) tea.Cmd {
	return func() tea.Msg {
		cmd := exec.Command("git", "-C", path, "pull", "--ff-only")
		output, err := cmd.CombinedOutput()

		result := strings.TrimSpace(string(output))
		shortResult := result

		// Only shorten for success display in list
		if err == nil {
			if strings.Contains(result, "Already up to date") {
				shortResult = "up to date"
			} else if strings.Contains(result, "Fast-forward") {
				shortResult = "updated"
			} else if len(result) > 30 {
				shortResult = result[:30] + "..."
			}
		}

		return pullCompleteMsg{
			path:        path,
			result:      result,      // full result for error view
			shortResult: shortResult, // short result for list display
			err:         err,
		}
	}
}

func loadBranches(path string) tea.Cmd {
	return func() tea.Msg {
		// Fetch from remote to get latest branches
		fetchCmd := exec.Command("git", "-C", path, "fetch", "--all", "--prune", "--quiet")
		fetchCmd.Run() // ignore errors

		// Get current branch
		currentCmd := exec.Command("git", "-C", path, "rev-parse", "--abbrev-ref", "HEAD")
		currentOut, _ := currentCmd.Output()
		current := strings.TrimSpace(string(currentOut))

		// Get all local branches with their upstream
		branchCmd := exec.Command("git", "-C", path, "for-each-ref", "--format=%(refname:short) %(upstream:short)", "refs/heads/")
		branchOut, _ := branchCmd.Output()

		localBranches := make(map[string]string) // local name -> remote tracking branch
		for _, line := range strings.Split(strings.TrimSpace(string(branchOut)), "\n") {
			parts := strings.Fields(line)
			if len(parts) >= 1 && parts[0] != "" {
				localName := parts[0]
				remoteName := ""
				if len(parts) >= 2 {
					remoteName = parts[1]
				}
				localBranches[localName] = remoteName
			}
		}

		// Get all remote branches
		remoteCmd := exec.Command("git", "-C", path, "for-each-ref", "--format=%(refname:short)", "refs/remotes/")
		remoteOut, _ := remoteCmd.Output()

		remoteBranches := make(map[string]bool)
		for _, b := range strings.Split(strings.TrimSpace(string(remoteOut)), "\n") {
			b = strings.TrimSpace(b)
			if b != "" && !strings.HasSuffix(b, "/HEAD") {
				remoteBranches[b] = true
			}
		}

		// Build branch info list
		var branches []BranchInfo
		seenRemotes := make(map[string]bool)

		// Add local branches
		for localName, remoteName := range localBranches {
			hasRemote := false
			if remoteName != "" {
				hasRemote = remoteBranches[remoteName]
				seenRemotes[remoteName] = true
			} else {
				// Check if origin/<name> exists
				possibleRemote := "origin/" + localName
				if remoteBranches[possibleRemote] {
					hasRemote = true
					remoteName = possibleRemote
					seenRemotes[possibleRemote] = true
				}
			}

			branches = append(branches, BranchInfo{
				Name:       localName,
				IsLocal:    true,
				IsRemote:   hasRemote,
				IsCurrent:  localName == current,
				RemoteName: remoteName,
			})
		}

		// Add remote-only branches
		for remoteName := range remoteBranches {
			if seenRemotes[remoteName] {
				continue
			}
			// Get local name from remote name
			localName := remoteName
			if strings.HasPrefix(remoteName, "origin/") {
				localName = strings.TrimPrefix(remoteName, "origin/")
			}

			branches = append(branches, BranchInfo{
				Name:       localName,
				IsLocal:    false,
				IsRemote:   true,
				IsCurrent:  false,
				RemoteName: remoteName,
			})
		}

		// Sort branches: current first, then local+remote, then local-only, then remote-only
		sort.Slice(branches, func(i, j int) bool {
			if branches[i].IsCurrent {
				return true
			}
			if branches[j].IsCurrent {
				return false
			}
			// Both local and remote first
			iBoth := branches[i].IsLocal && branches[i].IsRemote
			jBoth := branches[j].IsLocal && branches[j].IsRemote
			if iBoth != jBoth {
				return iBoth
			}
			// Local-only before remote-only
			if branches[i].IsLocal != branches[j].IsLocal {
				return branches[i].IsLocal
			}
			return branches[i].Name < branches[j].Name
		})

		return branchesLoadedMsg{
			path:     path,
			branches: branches,
			current:  current,
		}
	}
}

func switchBranch(path, branch string) tea.Cmd {
	return func() tea.Msg {
		cmd := exec.Command("git", "-C", path, "checkout", branch)
		output, err := cmd.CombinedOutput()

		if err != nil {
			return branchSwitchMsg{
				path:    path,
				branch:  branch,
				success: false,
				err:     strings.TrimSpace(string(output)),
			}
		}

		return branchSwitchMsg{
			path:    path,
			branch:  branch,
			success: true,
			err:     "",
		}
	}
}

func deleteBranch(path, branch string, force bool) tea.Cmd {
	return func() tea.Msg {
		flag := "-d"
		if force {
			flag = "-D"
		}
		cmd := exec.Command("git", "-C", path, "branch", flag, branch)
		output, err := cmd.CombinedOutput()

		if err != nil {
			return branchDeleteMsg{
				path:    path,
				branch:  branch,
				success: false,
				err:     strings.TrimSpace(string(output)),
			}
		}

		return branchDeleteMsg{
			path:    path,
			branch:  branch,
			success: true,
			err:     "",
		}
	}
}

func createLocalBranch(path, localName, remoteName string) tea.Cmd {
	return func() tea.Msg {
		// Create local branch tracking the remote branch
		cmd := exec.Command("git", "-C", path, "branch", "--track", localName, remoteName)
		output, err := cmd.CombinedOutput()

		if err != nil {
			return branchCreateMsg{
				path:    path,
				branch:  localName,
				success: false,
				err:     strings.TrimSpace(string(output)),
			}
		}

		return branchCreateMsg{
			path:    path,
			branch:  localName,
			success: true,
			err:     "",
		}
	}
}

func stashChanges(path string) tea.Cmd {
	return func() tea.Msg {
		cmd := exec.Command("git", "-C", path, "stash", "push", "-m", "guppi: auto-stash before branch switch")
		output, err := cmd.CombinedOutput()

		if err != nil {
			return stashResultMsg{
				path:    path,
				success: false,
				err:     strings.TrimSpace(string(output)),
			}
		}

		return stashResultMsg{
			path:    path,
			success: true,
			err:     "",
		}
	}
}

func discardChanges(path string) tea.Cmd {
	return func() tea.Msg {
		// Reset staged changes
		exec.Command("git", "-C", path, "reset", "HEAD").Run()
		// Discard unstaged changes
		cmd := exec.Command("git", "-C", path, "checkout", "--", ".")
		output, err := cmd.CombinedOutput()

		if err != nil {
			return stashResultMsg{
				path:    path,
				success: false,
				err:     strings.TrimSpace(string(output)),
			}
		}

		return stashResultMsg{
			path:    path,
			success: true,
			err:     "",
		}
	}
}

func hasUncommittedChanges(path string) bool {
	cmd := exec.Command("git", "-C", path, "status", "--porcelain")
	output, _ := cmd.Output()
	return strings.TrimSpace(string(output)) != ""
}

func runCommand(path, command string) tea.Cmd {
	return func() tea.Msg {
		// Split command into parts
		parts := strings.Fields(command)
		if len(parts) == 0 {
			return cmdResultMsg{output: "", err: fmt.Errorf("empty command")}
		}

		cmd := exec.Command(parts[0], parts[1:]...)
		cmd.Dir = path
		output, err := cmd.CombinedOutput()

		return cmdResultMsg{
			output: string(output),
			err:    err,
		}
	}
}
