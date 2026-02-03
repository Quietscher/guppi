package main

import "fmt"

// GitStatus represents the status of a git repository
type GitStatus int

const (
	StatusUnknown GitStatus = iota
	StatusClean
	StatusCleanBehind // clean locally but behind remote
	StatusDirty
	StatusError
)

// Repo represents a git repository
type Repo struct {
	Path        string
	Name        string
	Branch      string
	Status      GitStatus
	StatusText  string
	IsFavorite  bool
	PullResult  string
	BehindCount int
}

func (r Repo) Title() string {
	star := ""
	if r.IsFavorite {
		star = "★ "
	}
	branch := ""
	if r.Branch != "" {
		branch = " [" + r.Branch + "]"
	}
	return star + r.Name + branch
}

func (r Repo) Description() string {
	var status string
	switch r.Status {
	case StatusClean:
		status = statusCleanStyle.Render("✓ clean")
	case StatusCleanBehind:
		status = statusDirtyStyle.Render(fmt.Sprintf("↓ %d behind", r.BehindCount))
	case StatusDirty:
		if r.BehindCount > 0 {
			status = statusDirtyStyle.Render(fmt.Sprintf("● %s | ↓ %d behind", r.StatusText, r.BehindCount))
		} else {
			status = statusDirtyStyle.Render("● " + r.StatusText)
		}
	case StatusError:
		status = statusErrorStyle.Render("✗ " + r.StatusText)
	default:
		status = "..."
	}

	if r.PullResult != "" {
		status += " | " + pullResultStyle.Render(r.PullResult)
	}

	return status
}

func (r Repo) FilterValue() string { return r.Name }

// Group represents a collection of repos
type Group struct {
	Name      string   `json:"name"`
	Repos     []string `json:"repos"` // repo paths
	IsBuiltIn bool     `json:"-"`     // runtime flag for Favorites
}

// GroupItem is used for list display
type GroupItem struct {
	Name        string
	RepoCount   int
	DirtyCount  int // repos with changes
	BehindCount int // repos behind remote
}

func (g GroupItem) Title() string       { return "📁 " + g.Name }
func (g GroupItem) Description() string { return fmt.Sprintf("%d repos", g.RepoCount) }
func (g GroupItem) FilterValue() string { return g.Name }

// BranchInfo contains information about a git branch
type BranchInfo struct {
	Name       string
	IsLocal    bool   // exists locally
	IsRemote   bool   // exists on remote
	IsCurrent  bool
	RemoteName string // e.g., "origin/main" if tracking
}

// viewMode represents the current view state
type viewMode int

const (
	listView viewMode = iota
	detailView
	configView
	actionSelectView
	errorView
	settingsView
	groupInputView     // text input for group name (new/rename)
	groupDeleteView    // confirm group deletion
	groupSelectView    // select group to move repo to
	groupAddReposView  // select repos to add to group
	pullResultsView    // show results after pull operations
)

// switchAction represents actions for handling uncommitted changes
type switchAction int

const (
	actionNone switchAction = iota
	actionStash
	actionDiscard
	actionCancel
)

// detailPane represents which pane has focus in detail view
type detailPane int

const (
	paneStatus detailPane = iota
	paneBranches
	paneCommand
)

// Message types for async operations

type repoFoundMsg struct {
	repos []Repo
}

type statusUpdatedMsg struct {
	path        string
	branch      string
	status      GitStatus
	text        string
	behindCount int
}

type pullCompleteMsg struct {
	path        string
	result      string // full output for error display
	shortResult string // shortened for list display
	err         error
}

type detailLoadedMsg struct {
	path    string
	content string
}

type branchesLoadedMsg struct {
	path     string
	branches []BranchInfo
	current  string
}

type branchDeleteMsg struct {
	path    string
	branch  string
	success bool
	err     string
}

type branchCreateMsg struct {
	path    string
	branch  string
	success bool
	err     string
}

type branchSwitchMsg struct {
	path    string
	branch  string
	success bool
	err     string
}

type stashResultMsg struct {
	path    string
	success bool
	err     string
}

type lazygitExitMsg struct {
	path string
	err  error
}

type cmdResultMsg struct {
	output string
	err    error
}

// Pull results screen types

type CommitInfo struct {
	Hash    string
	Message string
	Author  string
	Time    string // relative, e.g. "2 hours ago"
}

type PullResultInfo struct {
	RepoPath     string
	RepoName     string
	Commits      []CommitInfo
	FilesChanged int
	Updated      bool // true if actually pulled new commits
}

type pullResultsReadyMsg struct {
	results []PullResultInfo
}

// Progress tracking messages
type progressTickMsg struct{}

type batchOperationMsg struct {
	paths     []string // remaining paths to process
	operation string   // "fetch" or "pull"
}
