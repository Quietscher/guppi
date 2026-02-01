package main

import (
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// PullResultsCursor tracks position in the three-level tree
type PullResultsCursor struct {
	Level     int // 0=repo, 1=commit, 2=file
	RepoIdx   int
	CommitIdx int
	FileIdx   int
}

// FileChange represents a changed file in a commit
type FileChange struct {
	Path      string
	Additions int
	Deletions int
}

// GoDeeper moves cursor to next level, returns true if moved
func (c *PullResultsCursor) GoDeeper() bool {
	if c.Level < 2 {
		c.Level++
		if c.Level == 1 {
			c.CommitIdx = 0
		} else if c.Level == 2 {
			c.FileIdx = 0
		}
		return true
	}
	return false
}

// GoUp moves cursor to parent level, returns true if moved
func (c *PullResultsCursor) GoUp() bool {
	if c.Level > 0 {
		c.Level--
		return true
	}
	return false
}

// MoveDown moves cursor down within current level
// Returns false if at bottom (caller may want to stay or wrap)
func (c *PullResultsCursor) MoveDown(maxItems int) bool {
	switch c.Level {
	case 0:
		if c.RepoIdx < maxItems-1 {
			c.RepoIdx++
			return true
		}
	case 1:
		if c.CommitIdx < maxItems-1 {
			c.CommitIdx++
			return true
		}
	case 2:
		if c.FileIdx < maxItems-1 {
			c.FileIdx++
			return true
		}
	}
	return false
}

// MoveUp moves cursor up within current level
// Returns false if at top (caller should go up a level)
func (c *PullResultsCursor) MoveUp() bool {
	switch c.Level {
	case 0:
		if c.RepoIdx > 0 {
			c.RepoIdx--
			return true
		}
	case 1:
		if c.CommitIdx > 0 {
			c.CommitIdx--
			return true
		}
	case 2:
		if c.FileIdx > 0 {
			c.FileIdx--
			return true
		}
	}
	return false
}

// Reset resets cursor to initial state
func (c *PullResultsCursor) Reset() {
	c.Level = 0
	c.RepoIdx = 0
	c.CommitIdx = 0
	c.FileIdx = 0
}

// fetchFilesForCommit gets the list of changed files for a commit
func fetchFilesForCommit(repoPath, commitHash string) ([]FileChange, error) {
	cmd := exec.Command("git", "-C", repoPath, "show", "--stat", "--format=", commitHash)
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	return parseGitStatOutput(string(output)), nil
}

// parseGitStatOutput parses git diff --stat or git show --stat output
func parseGitStatOutput(output string) []FileChange {
	var files []FileChange
	lines := strings.Split(strings.TrimSpace(output), "\n")

	// Pattern: " path/to/file | 10 +++---" or " path/to/file | Bin 0 -> 123 bytes"
	statPattern := regexp.MustCompile(`^\s*(.+?)\s*\|\s*(\d+)?\s*(\+*)(-*)`)

	for _, line := range lines {
		// Skip summary line (e.g., "3 files changed, 10 insertions(+)")
		if strings.Contains(line, "files changed") || strings.Contains(line, "file changed") {
			continue
		}
		if strings.TrimSpace(line) == "" {
			continue
		}

		matches := statPattern.FindStringSubmatch(line)
		if matches != nil {
			path := strings.TrimSpace(matches[1])
			additions := len(matches[3])
			deletions := len(matches[4])

			// If we have a number but no +/-, try to parse it
			if matches[2] != "" && additions == 0 && deletions == 0 {
				// Binary file or just a count
				if count, err := strconv.Atoi(matches[2]); err == nil {
					additions = count / 2
					deletions = count - additions
				}
			}

			files = append(files, FileChange{
				Path:      path,
				Additions: additions,
				Deletions: deletions,
			})
		}
	}

	return files
}

// Styles for pull results
var (
	prRepoStyle     = lipgloss.NewStyle().Bold(true)
	prCommitHash    = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	prAdditions     = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	prDeletions     = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	prSelected      = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
	prDim           = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
)

// renderPullResultsView renders the entire pull results screen
func renderPullResultsView(m model) string {
	title := detailTitleStyle.Render("Pull Results")

	// Calculate summary stats
	totalCommits := 0
	totalFiles := 0
	updatedRepos := 0
	for _, r := range m.pullResults {
		if r.Updated {
			updatedRepos++
			totalCommits += len(r.Commits)
			totalFiles += r.FilesChanged
		}
	}

	summary := successStyle.Render(fmt.Sprintf("%d repos updated • %d commits • %d files changed",
		updatedRepos, totalCommits, totalFiles))

	// Render tree
	var content strings.Builder
	cursor := m.pullResultsCursor

	for i, result := range m.pullResults {
		// Level 0: Repo line
		isRepoSelected := cursor.Level == 0 && i == cursor.RepoIdx
		isRepoExpanded := i == cursor.RepoIdx && cursor.Level >= 1

		repoLine := renderRepoLine(result, isRepoSelected, isRepoExpanded)
		content.WriteString(repoLine + "\n")

		// Level 1: Commits (only if this repo is expanded)
		if isRepoExpanded {
			for j, commit := range result.Commits {
				isCommitSelected := cursor.Level == 1 && j == cursor.CommitIdx
				isCommitExpanded := j == cursor.CommitIdx && cursor.Level == 2

				commitLine := renderCommitLine(commit, isCommitSelected, isCommitExpanded)
				content.WriteString(commitLine + "\n")

				// Level 2: Files (only if this commit is expanded)
				if isCommitExpanded {
					cacheKey := result.RepoPath + ":" + commit.Hash
					files := m.filesCache[cacheKey]

					// Calculate max path width for alignment
					maxPathWidth := 20
					for _, f := range files {
						if len(f.Path) > maxPathWidth {
							maxPathWidth = len(f.Path)
						}
					}
					if maxPathWidth > 40 {
						maxPathWidth = 40
					}

					for k, file := range files {
						isFileSelected := cursor.Level == 2 && k == cursor.FileIdx
						fileLine := renderFileLine(file, isFileSelected, maxPathWidth)
						content.WriteString(fileLine + "\n")
					}

					if len(files) == 0 {
						content.WriteString("          " + prDim.Render("(loading files...)") + "\n")
					}
				}
			}
		}
	}

	if len(m.pullResults) == 0 {
		content.WriteString(prDim.Render("  No pull results to show"))
	}

	help := helpStyle.Render("↑/↓: navigate • →/enter: expand • ←: collapse • esc: back")

	return title + "\n\n" + summary + "\n\n" + content.String() + "\n" + help
}

// renderRepoLine renders a single repo line
func renderRepoLine(result PullResultInfo, isSelected, isExpanded bool) string {
	prefix := "  "
	if isSelected {
		prefix = "> "
	}

	expandIcon := "▶"
	if isExpanded {
		expandIcon = "▼"
	}

	statusIcon := "✓"
	if !result.Updated {
		statusIcon = "−"
	}

	info := fmt.Sprintf(" (%d commits, %d files)", len(result.Commits), result.FilesChanged)
	if !result.Updated {
		info = " (up to date)"
	}

	line := fmt.Sprintf("%s %s %s%s", expandIcon, statusIcon, result.RepoName, info)

	if isSelected {
		return prefix + prSelected.Render(line)
	}
	if result.Updated {
		return prefix + statusCleanStyle.Render(line)
	}
	return prefix + prDim.Render(line)
}

// renderCommitLine renders a single commit line
func renderCommitLine(commit CommitInfo, isSelected, isExpanded bool) string {
	prefix := "      "
	if isSelected {
		prefix = "    > "
	}

	expandIcon := "▶"
	if isExpanded {
		expandIcon = "▼"
	}

	hash := prCommitHash.Render(commit.Hash)
	message := commit.Message
	if len(message) > 50 {
		message = message[:47] + "..."
	}

	line := fmt.Sprintf("%s %s %s", expandIcon, hash, message)

	if isSelected {
		return prefix + prSelected.Render(line)
	}
	return prefix + line
}

// renderFileLine renders a single file change line with aligned columns
func renderFileLine(file FileChange, isSelected bool, maxPathWidth int) string {
	prefix := "          "
	if isSelected {
		prefix = "        > "
	}

	// Truncate path if too long
	path := file.Path
	if len(path) > maxPathWidth {
		path = "..." + path[len(path)-maxPathWidth+3:]
	}

	// Right-align path
	pathStyle := lipgloss.NewStyle().Width(maxPathWidth).Align(lipgloss.Right)
	pathStr := pathStyle.Render(path)

	// Build change indicator
	total := file.Additions + file.Deletions
	changeStr := fmt.Sprintf("%3d ", total)

	// Build +/- visualization (max 20 chars)
	maxBars := 20
	addBars := 0
	delBars := 0
	if total > 0 {
		addBars = (file.Additions * maxBars) / total
		delBars = (file.Deletions * maxBars) / total
		if file.Additions > 0 && addBars == 0 {
			addBars = 1
		}
		if file.Deletions > 0 && delBars == 0 {
			delBars = 1
		}
	}

	bars := prAdditions.Render(strings.Repeat("+", addBars)) +
		prDeletions.Render(strings.Repeat("-", delBars))

	line := pathStr + " │ " + changeStr + bars

	if isSelected {
		return prefix + prSelected.Render(line)
	}
	return prefix + line
}
