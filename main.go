package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	titleStyle        = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
	statusCleanStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	statusDirtyStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	statusErrorStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	favoriteStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("226"))
	branchStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))
	helpStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	successStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	pullResultStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))
	detailTitleStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205")).Padding(0, 1)
	detailBorderStyle = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("62")).Padding(1, 2)
)

type GitStatus int

const (
	StatusUnknown GitStatus = iota
	StatusClean
	StatusCleanBehind // clean locally but behind remote
	StatusDirty
	StatusError
)

type Repo struct {
	Path         string
	Name         string
	Branch       string
	Status       GitStatus
	StatusText   string
	IsFavorite   bool
	PullResult   string
	BehindCount  int
}

func (r Repo) Title() string {
	// Don't use lipgloss styling here - it breaks filter highlighting
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

// Custom delegate that looks up favorites from shared map for instant updates
type repoDelegate struct {
	list.DefaultDelegate
	favorites map[string]bool // maps are reference types, so this shares data with model
}

func newRepoDelegate(favorites map[string]bool) repoDelegate {
	d := repoDelegate{
		DefaultDelegate: list.NewDefaultDelegate(),
		favorites:       favorites,
	}
	d.ShowDescription = true
	return d
}

func (d repoDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	repo, ok := item.(Repo)
	if !ok {
		return
	}

	// Look up favorite from shared map for instant updates
	isFavorite := d.favorites[repo.Path]

	// Render with updated favorite state
	var title string
	if isFavorite {
		title = favoriteStyle.Render("★") + " " + repo.Name
	} else {
		title = "  " + repo.Name
	}
	if repo.Branch != "" {
		title += " " + branchStyle.Render("["+repo.Branch+"]")
	}

	desc := repo.Description()

	// Handle selection styling
	isSelected := index == m.Index()
	itemStyles := d.Styles

	if isSelected {
		title = itemStyles.SelectedTitle.Render(title)
		desc = itemStyles.SelectedDesc.Render(desc)
	} else {
		title = itemStyles.NormalTitle.Render(title)
		desc = itemStyles.NormalDesc.Render(desc)
	}

	fmt.Fprintf(w, "%s\n%s", title, desc)
}

type viewMode int

const (
	listView viewMode = iota
	detailView
	configView
	actionSelectView
	errorView
	settingsView
)

type switchAction int

const (
	actionNone switchAction = iota
	actionStash
	actionDiscard
	actionCancel
)

type detailPane int

const (
	paneStatus detailPane = iota
	paneBranches
	paneCommand
)

type model struct {
	list          list.Model
	repos         []Repo
	favorites     map[string]bool
	scanning      bool
	pulling       bool
	spinner       spinner.Model
	statusMsg     string
	errorMsg      string
	width         int
	height        int
	gitDir        string
	mode           viewMode
	previousMode   viewMode // for returning from error view
	savedFilter    string   // saved filter text for restoring after error
	detailRepo     *Repo
	detailContent string
	viewport      viewport.Model
	dirInput      textinput.Model
	gotoPath      string // path to cd to after exit

	// Branch switching
	branches       []BranchInfo
	branchIndex    int
	targetBranch   string
	actionIndex    int
	hasChanges     bool

	// Status filters
	filterDirty  bool // show only repos with local changes
	filterBehind bool // show only repos behind remote

	// Detail view panes
	detailFocus   detailPane      // which pane has focus
	cmdInput      textinput.Model // command input
	cmdOutput     string          // command output
	cmdViewport   viewport.Model  // viewport for command output
	cmdRunning    bool            // is a command running

	// Performance config
	fetchMode      FetchMode // How to fetch repo status
	settingsIndex  int       // Current selection in settings view
	forceFullFetch bool      // Force full fetch on next scan (for ctrl+r)
}

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

type BranchInfo struct {
	Name       string
	IsLocal    bool // exists locally
	IsRemote   bool // exists on remote
	IsCurrent  bool
	RemoteName string // e.g., "origin/main" if tracking
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

func initialModel(gitDir string) model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	favorites := loadFavorites()
	config := loadConfig()

	// Create delegate with shared favorites map for instant updates
	delegate := newRepoDelegate(favorites)

	l := list.New([]list.Item{}, delegate, 0, 0)
	l.Title = "guppi - Git Repository Manager"
	l.SetShowStatusBar(true)
	l.SetFilteringEnabled(true)
	l.Styles.Title = titleStyle

	vp := viewport.New(80, 20)

	ti := textinput.New()
	ti.Placeholder = "Enter git directory path..."
	ti.CharLimit = 256
	ti.Width = 60
	ti.SetValue(gitDir)

	// Command input for detail view
	cmdInput := textinput.New()
	cmdInput.Placeholder = "Enter command (e.g., git log --oneline -5)..."
	cmdInput.CharLimit = 512
	cmdInput.Width = 60

	cmdVp := viewport.New(80, 10)

	return model{
		list:        l,
		repos:       []Repo{},
		favorites:   favorites,
		scanning:    true,
		spinner:     s,
		gitDir:      gitDir,
		mode:        listView,
		viewport:    vp,
		dirInput:    ti,
		cmdInput:    cmdInput,
		cmdViewport: cmdVp,
		fetchMode:   config.FetchMode,
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
		scanForRepos(m.gitDir),
	)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.list.SetSize(msg.Width, msg.Height-5)
		m.viewport.Width = msg.Width - 4
		m.viewport.Height = msg.Height - 8

	case tea.KeyMsg:
		// Handle config view keys
		if m.mode == configView {
			switch msg.String() {
			case "esc":
				m.mode = listView
				m.dirInput.SetValue(m.gitDir)
				return m, nil
			case "enter":
				newDir := m.dirInput.Value()
				if info, err := os.Stat(newDir); err == nil && info.IsDir() {
					m.gitDir = newDir
					m.mode = listView
					m.scanning = true
					m.repos = []Repo{}
					m.list.SetItems([]list.Item{})
					m.statusMsg = "Scanning..."
					saveConfig(newDir)
					return m, tea.Batch(m.spinner.Tick, scanForRepos(m.gitDir))
				}
				m.statusMsg = "Invalid directory"
				return m, nil
			}
			var cmd tea.Cmd
			m.dirInput, cmd = m.dirInput.Update(msg)
			return m, cmd
		}

		// Handle detail view keys
		if m.mode == detailView {
			switch msg.String() {
			case "q", "esc":
				if m.detailFocus == paneCommand && m.cmdInput.Value() != "" {
					// Clear command input first
					m.cmdInput.SetValue("")
					return m, nil
				}
				m.mode = listView
				m.detailRepo = nil
				m.detailContent = ""
				m.cmdOutput = ""
				m.branches = nil
				m.detailFocus = paneStatus
				return m, nil
			case "tab":
				// Cycle through panes: status -> branches -> command -> status
				m.detailFocus = (m.detailFocus + 1) % 3
				if m.detailFocus == paneCommand {
					m.cmdInput.Focus()
				} else {
					m.cmdInput.Blur()
				}
				return m, nil
			case "shift+tab":
				// Cycle backwards
				m.detailFocus = (m.detailFocus + 2) % 3
				if m.detailFocus == paneCommand {
					m.cmdInput.Focus()
				} else {
					m.cmdInput.Blur()
				}
				return m, nil
			case "r":
				// Refresh detail view
				if m.detailRepo != nil && m.detailFocus != paneCommand {
					return m, tea.Batch(loadGitDetail(m.detailRepo.Path), loadBranches(m.detailRepo.Path))
				}
			}

			// Handle pane-specific keys
			switch m.detailFocus {
			case paneStatus:
				var cmd tea.Cmd
				m.viewport, cmd = m.viewport.Update(msg)
				return m, cmd
			case paneBranches:
				switch msg.String() {
				case "up", "k":
					if m.branchIndex > 0 {
						m.branchIndex--
					}
					return m, nil
				case "down", "j":
					if m.branchIndex < len(m.branches)-1 {
						m.branchIndex++
					}
					return m, nil
				case "enter":
					if len(m.branches) > 0 && m.detailRepo != nil {
						branch := m.branches[m.branchIndex]
						if branch.IsCurrent {
							m.statusMsg = "Already on " + branch.Name
							return m, nil
						}
						// For remote-only branches, checkout will create local tracking branch
						checkoutName := branch.Name
						if !branch.IsLocal && branch.IsRemote {
							checkoutName = branch.RemoteName
						}
						m.targetBranch = branch.Name
						// Check for uncommitted changes
						if hasUncommittedChanges(m.detailRepo.Path) {
							m.hasChanges = true
							m.mode = actionSelectView
							m.actionIndex = 0
							return m, nil
						}
						// No changes, switch directly
						m.statusMsg = "Switching to " + branch.Name + "..."
						return m, switchBranch(m.detailRepo.Path, checkoutName)
					}
					return m, nil
				case "x":
					// Delete local-only branch
					if len(m.branches) > 0 && m.detailRepo != nil {
						branch := m.branches[m.branchIndex]
						if branch.IsCurrent {
							m.statusMsg = "Cannot delete current branch"
							return m, nil
						}
						if !branch.IsLocal {
							m.statusMsg = "Branch is remote-only, nothing to delete locally"
							return m, nil
						}
						if branch.IsRemote {
							m.statusMsg = "Branch exists on remote. Use 'X' to force delete."
							return m, nil
						}
						// Local-only branch, safe to delete
						return m, deleteBranch(m.detailRepo.Path, branch.Name, false)
					}
					return m, nil
				case "X":
					// Force delete local branch (even if not merged/has remote)
					if len(m.branches) > 0 && m.detailRepo != nil {
						branch := m.branches[m.branchIndex]
						if branch.IsCurrent {
							m.statusMsg = "Cannot delete current branch"
							return m, nil
						}
						if !branch.IsLocal {
							m.statusMsg = "Branch is remote-only"
							return m, nil
						}
						return m, deleteBranch(m.detailRepo.Path, branch.Name, true)
					}
					return m, nil
				case "p":
					// Pull/fetch remote branch to local (create tracking branch without switching)
					if len(m.branches) > 0 && m.detailRepo != nil {
						branch := m.branches[m.branchIndex]
						if branch.IsLocal {
							m.statusMsg = "Branch already exists locally"
							return m, nil
						}
						if !branch.IsRemote {
							m.statusMsg = "Branch is not on remote"
							return m, nil
						}
						m.statusMsg = "Creating local branch " + branch.Name + "..."
						return m, createLocalBranch(m.detailRepo.Path, branch.Name, branch.RemoteName)
					}
					return m, nil
				}
				return m, nil
			case paneCommand:
				switch msg.String() {
				case "enter":
					if m.cmdInput.Value() != "" && !m.cmdRunning {
						cmd := m.cmdInput.Value()
						m.cmdRunning = true
						m.cmdOutput = "Running: " + cmd + "\n\n"
						m.cmdViewport.SetContent(m.cmdOutput)
						return m, runCommand(m.detailRepo.Path, cmd)
					}
					return m, nil
				}
				var cmd tea.Cmd
				m.cmdInput, cmd = m.cmdInput.Update(msg)
				return m, cmd
			}
			return m, nil
		}

		// Handle action select view keys (what to do with uncommitted changes)
		if m.mode == actionSelectView {
			actions := []string{"Stash changes", "Discard changes", "Cancel"}
			switch msg.String() {
			case "q", "esc":
				m.mode = detailView
				m.detailFocus = paneBranches
				return m, nil
			case "up", "k":
				if m.actionIndex > 0 {
					m.actionIndex--
				}
				return m, nil
			case "down", "j":
				if m.actionIndex < len(actions)-1 {
					m.actionIndex++
				}
				return m, nil
			case "enter":
				if m.detailRepo == nil {
					m.mode = detailView
					return m, nil
				}
				switch m.actionIndex {
				case 0: // Stash
					m.statusMsg = "Stashing changes..."
					return m, stashChanges(m.detailRepo.Path)
				case 1: // Discard
					m.statusMsg = "Discarding changes..."
					return m, discardChanges(m.detailRepo.Path)
				case 2: // Cancel
					m.mode = detailView
					m.detailFocus = paneBranches
					return m, nil
				}
				return m, nil
			}
			return m, nil
		}

		// Handle error view keys
		if m.mode == errorView {
			switch msg.String() {
			case "q", "esc", "enter":
				m.errorMsg = ""
				// Return to previous mode
				if m.previousMode == detailView && m.detailRepo != nil {
					m.mode = detailView
					return m, loadGitDetail(m.detailRepo.Path)
				}
				m.mode = listView
				m.detailRepo = nil
				// Restore filter if there was one
				if m.savedFilter != "" {
					m.list.SetFilterText(m.savedFilter)
					m.savedFilter = ""
				}
				return m, nil
			}
			var cmd tea.Cmd
			m.viewport, cmd = m.viewport.Update(msg)
			return m, cmd
		}

		// Handle settings view keys
		if m.mode == settingsView {
			switch msg.String() {
			case "q", "esc":
				m.mode = listView
				return m, nil
			case "up", "k":
				if m.settingsIndex > 0 {
					m.settingsIndex--
				}
				return m, nil
			case "down", "j":
				if m.settingsIndex < 2 { // 3 options: all, on-demand, favorites
					m.settingsIndex++
				}
				return m, nil
			case "enter", " ":
				// Select fetch mode (radio button style)
				config := loadConfig()
				newMode := FetchMode(m.settingsIndex)
				if m.fetchMode != newMode {
					m.fetchMode = newMode
					config.FetchMode = newMode
					switch newMode {
					case FetchAll:
						m.statusMsg = "Fetch mode: All repos"
					case FetchOnDemand:
						m.statusMsg = "Fetch mode: On-demand (visible only)"
					case FetchFavorites:
						m.statusMsg = "Fetch mode: Favorites only"
					}
					saveConfigFull(config)
				}
				return m, nil
			}
			return m, nil
		}

		if m.list.FilterState() == list.Filtering {
			break
		}

		switch msg.String() {
		case "q", "ctrl+c":
			saveFavorites(m.favorites)
			return m, tea.Quit

		case "f":
			if item, ok := m.list.SelectedItem().(Repo); ok {
				m.favorites[item.Path] = !m.favorites[item.Path]
				// Update repo favorites in our data
				for i := range m.repos {
					m.repos[i].IsFavorite = m.favorites[m.repos[i].Path]
				}
				// Only update list display when not filtering (SetItems clears filter)
				if m.list.FilterState() != list.Filtering && m.list.FilterState() != list.FilterApplied {
					m.updateList()
				}
				saveFavorites(m.favorites)
				if m.favorites[item.Path] {
					m.statusMsg = "Added to favorites: " + item.Name
				} else {
					m.statusMsg = "Removed from favorites: " + item.Name
				}
			}

		case "enter", "p":
			if item, ok := m.list.SelectedItem().(Repo); ok {
				m.pulling = true
				m.statusMsg = "Pulling " + item.Name + "..."
				return m, tea.Batch(m.spinner.Tick, pullRepo(item.Path))
			}

		case "P":
			var pullCmds []tea.Cmd
			count := 0
			for _, repo := range m.repos {
				if repo.IsFavorite {
					pullCmds = append(pullCmds, pullRepo(repo.Path))
					count++
				}
			}
			if count > 0 {
				m.pulling = true
				m.statusMsg = fmt.Sprintf("Pulling %d favorites...", count)
				pullCmds = append(pullCmds, m.spinner.Tick)
				return m, tea.Batch(pullCmds...)
			}

		case "r":
			switch m.fetchMode {
			case FetchOnDemand:
				// On-demand: only refresh the selected repo
				if item, ok := m.list.SelectedItem().(Repo); ok {
					m.statusMsg = "Refreshing " + item.Name + "..."
					return m, checkGitStatus(item.Path)
				}
				return m, nil
			case FetchFavorites:
				// Favorites: only refresh favorite repos
				var cmds []tea.Cmd
				count := 0
				for _, repo := range m.repos {
					if repo.IsFavorite {
						cmds = append(cmds, checkGitStatus(repo.Path))
						count++
					}
				}
				if count > 0 {
					m.statusMsg = fmt.Sprintf("Refreshing %d favorites...", count)
					return m, tea.Batch(cmds...)
				}
				m.statusMsg = "No favorites to refresh"
				return m, nil
			default:
				// Normal refresh: rescan all repos
				m.scanning = true
				m.repos = []Repo{}
				// Save filter before clearing list
				if m.list.FilterState() == list.FilterApplied {
					m.savedFilter = m.list.FilterValue()
				}
				m.list.SetItems([]list.Item{})
				m.statusMsg = "Scanning..."
				return m, tea.Batch(m.spinner.Tick, scanForRepos(m.gitDir))
			}

		case "ctrl+r":
			// Full refresh: rescan all repos (works in all modes)
			m.scanning = true
			m.repos = []Repo{}
			m.forceFullFetch = true // Override fetch mode for this refresh
			// Save filter before clearing list
			if m.list.FilterState() == list.FilterApplied {
				m.savedFilter = m.list.FilterValue()
			}
			m.list.SetItems([]list.Item{})
			m.statusMsg = "Scanning all..."
			return m, tea.Batch(m.spinner.Tick, scanForRepos(m.gitDir))

		case "s":
			// Open lazygit for the selected repo
			if item, ok := m.list.SelectedItem().(Repo); ok {
				m.detailRepo = &item
				c := exec.Command("lazygit")
				c.Dir = item.Path
				return m, tea.ExecProcess(c, func(err error) tea.Msg {
					return lazygitExitMsg{path: item.Path, err: err}
				})
			}

		case "d":
			// Built-in detail view (multi-pane)
			if item, ok := m.list.SelectedItem().(Repo); ok {
				m.mode = detailView
				m.detailRepo = &item
				m.detailContent = "Loading..."
				m.viewport.SetContent(m.detailContent)
				m.detailFocus = paneStatus
				m.cmdOutput = ""
				m.cmdInput.SetValue("")
				m.cmdInput.Blur()
				m.branches = []BranchInfo{}
				m.branchIndex = 0
				return m, tea.Batch(loadGitDetail(item.Path), loadBranches(item.Path))
			}

		case "c":
			m.mode = configView
			m.dirInput.SetValue(m.gitDir)
			m.dirInput.Focus()
			return m, textinput.Blink

		case "g":
			if item, ok := m.list.SelectedItem().(Repo); ok {
				m.gotoPath = item.Path
				saveFavorites(m.favorites)
				return m, tea.Quit
			}

		case "1":
			// Toggle filter: repos with local changes
			m.filterDirty = !m.filterDirty
			m.updateList()
			if m.filterDirty {
				m.statusMsg = "Filter: showing repos with local changes"
			} else if m.filterBehind {
				m.statusMsg = "Filter: showing repos behind remote"
			} else {
				m.statusMsg = "Filter cleared"
			}

		case "2":
			// Toggle filter: repos behind remote
			m.filterBehind = !m.filterBehind
			m.updateList()
			if m.filterBehind {
				m.statusMsg = "Filter: showing repos behind remote"
			} else if m.filterDirty {
				m.statusMsg = "Filter: showing repos with local changes"
			} else {
				m.statusMsg = "Filter cleared"
			}

		case "0":
			// Clear all status filters
			m.filterDirty = false
			m.filterBehind = false
			m.updateList()
			m.statusMsg = "Filters cleared"

		case "A":
			// Pull all filtered repos that are behind remote
			filtered := m.getFilteredRepos()
			var pullCmds []tea.Cmd
			count := 0
			for _, repo := range filtered {
				if repo.BehindCount > 0 {
					pullCmds = append(pullCmds, pullRepo(repo.Path))
					count++
				}
			}
			if count > 0 {
				m.pulling = true
				m.statusMsg = fmt.Sprintf("Pulling %d repos behind remote...", count)
				pullCmds = append(pullCmds, m.spinner.Tick)
				return m, tea.Batch(pullCmds...)
			} else {
				m.statusMsg = "No repos behind remote to pull"
			}

		case "S":
			// Open settings view, highlight current mode
			m.mode = settingsView
			m.settingsIndex = int(m.fetchMode)
			return m, nil
		}

	case spinner.TickMsg:
		if m.scanning || m.pulling {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			cmds = append(cmds, cmd)
		}

	case repoFoundMsg:
		for i := range msg.repos {
			msg.repos[i].IsFavorite = m.favorites[msg.repos[i].Path]
		}
		m.repos = msg.repos
		m.scanning = false
		m.statusMsg = fmt.Sprintf("Found %d repositories", len(m.repos))
		m.updateList()
		// Restore filter if there was one (e.g., after refresh)
		if m.savedFilter != "" {
			m.list.SetFilterText(m.savedFilter)
			m.savedFilter = ""
		}

		// Fetch status based on performance settings
		var statusCmds []tea.Cmd
		if m.forceFullFetch {
			// Full refresh requested (ctrl+r), fetch all regardless of mode
			m.forceFullFetch = false
			for _, repo := range m.repos {
				statusCmds = append(statusCmds, checkGitStatus(repo.Path))
			}
		} else {
			switch m.fetchMode {
			case FetchOnDemand:
				// On-demand: don't fetch anything on startup, user presses 'r' to refresh selected
			case FetchFavorites:
				// Only fetch favorites
				for _, repo := range m.repos {
					if repo.IsFavorite {
						statusCmds = append(statusCmds, checkGitStatus(repo.Path))
					}
				}
			default: // FetchAll
				// Fetch all (default behavior)
				for _, repo := range m.repos {
					statusCmds = append(statusCmds, checkGitStatus(repo.Path))
				}
			}
		}
		if len(statusCmds) > 0 {
			cmds = append(cmds, tea.Batch(statusCmds...))
		}

	case statusUpdatedMsg:
		for i := range m.repos {
			if m.repos[i].Path == msg.path {
				m.repos[i].Status = msg.status
				m.repos[i].StatusText = msg.text
				m.repos[i].Branch = msg.branch
				m.repos[i].BehindCount = msg.behindCount
				break
			}
		}
		// Don't update while actively typing in filter
		if m.list.FilterState() == list.Filtering {
			break
		}
		// Save filter, update list, restore filter
		filterText := ""
		if m.list.FilterState() == list.FilterApplied {
			filterText = m.list.FilterValue()
		}
		m.updateList()
		if filterText != "" {
			m.list.SetFilterText(filterText)
		}

	case pullCompleteMsg:
		m.pulling = false
		repoName := filepath.Base(msg.path)
		for i := range m.repos {
			if m.repos[i].Path == msg.path {
				repoName = m.repos[i].Name
				if msg.err != nil {
					m.repos[i].PullResult = "error"
				} else {
					m.repos[i].PullResult = msg.shortResult // use short version for list
					m.errorMsg = ""
				}
				break
			}
		}
		if msg.err != nil {
			// Don't call updateList() on error - it clears the filter
			m.statusMsg = ""
			m.errorMsg = fmt.Sprintf("Pull failed for %s:\n\n%s", repoName, msg.result) // full error
			m.previousMode = m.mode
			// Save filter for restoration after error dismissal
			if m.list.FilterState() == list.FilterApplied {
				m.savedFilter = m.list.FilterValue()
			}
			m.mode = errorView
			m.viewport.SetContent(m.errorMsg)
		} else {
			m.updateList()
			m.statusMsg = fmt.Sprintf("Pulled %s: %s", repoName, msg.shortResult)
		}
		cmds = append(cmds, checkGitStatus(msg.path))

	case detailLoadedMsg:
		if m.mode == detailView && m.detailRepo != nil && m.detailRepo.Path == msg.path {
			m.detailContent = msg.content
			m.viewport.SetContent(m.detailContent)
		}

	case branchesLoadedMsg:
		if m.detailRepo != nil && m.detailRepo.Path == msg.path {
			m.branches = msg.branches
			// Find current branch index
			for i, b := range m.branches {
				if b.IsCurrent {
					m.branchIndex = i
					break
				}
			}
		}

	case branchDeleteMsg:
		if msg.success {
			m.statusMsg = "Deleted branch: " + msg.branch
			// Refresh branches
			if m.detailRepo != nil {
				cmds = append(cmds, loadBranches(m.detailRepo.Path))
			}
		} else {
			m.errorMsg = "Delete failed: " + msg.err
		}

	case branchCreateMsg:
		if msg.success {
			m.statusMsg = "Created local branch: " + msg.branch
			// Refresh branches
			if m.detailRepo != nil {
				cmds = append(cmds, loadBranches(m.detailRepo.Path))
			}
		} else {
			m.errorMsg = "Create failed: " + msg.err
		}

	case branchSwitchMsg:
		if msg.success {
			m.statusMsg = "Switched to " + msg.branch
			m.errorMsg = ""
			m.mode = detailView
			// Update the repo's branch in our data
			if m.detailRepo != nil {
				m.detailRepo.Branch = msg.branch
				for i := range m.repos {
					if m.repos[i].Path == msg.path {
						m.repos[i].Branch = msg.branch
						break
					}
				}
			}
			// Refresh detail view, branches and status
			cmds = append(cmds, loadGitDetail(msg.path), loadBranches(msg.path), checkGitStatus(msg.path))
		} else {
			m.errorMsg = "Branch switch failed:\n\n" + msg.err
			m.previousMode = m.mode
			// Save filter for restoration after error dismissal
			if m.list.FilterState() == list.FilterApplied {
				m.savedFilter = m.list.FilterValue()
			}
			m.mode = errorView
			m.viewport.SetContent(m.errorMsg)
		}

	case stashResultMsg:
		if msg.success {
			// Stash/discard succeeded, now switch branch
			if m.detailRepo != nil && m.targetBranch != "" {
				m.mode = detailView
				m.statusMsg = "Switching to " + m.targetBranch + "..."
				m.errorMsg = ""
				cmds = append(cmds, switchBranch(m.detailRepo.Path, m.targetBranch))
			}
		} else {
			m.errorMsg = "Operation failed:\n\n" + msg.err
			m.previousMode = m.mode
			// Save filter for restoration after error dismissal
			if m.list.FilterState() == list.FilterApplied {
				m.savedFilter = m.list.FilterValue()
			}
			m.mode = errorView
			m.viewport.SetContent(m.errorMsg)
		}

	case lazygitExitMsg:
		// Refresh repo status after lazygit exits
		m.statusMsg = "Back from lazygit"
		if msg.path != "" {
			cmds = append(cmds, checkGitStatus(msg.path))
		}
		m.detailRepo = nil

	case cmdResultMsg:
		m.cmdRunning = false
		if msg.err != nil {
			m.cmdOutput += statusErrorStyle.Render("Error: "+msg.err.Error()) + "\n\n"
		}
		if msg.output != "" {
			m.cmdOutput += msg.output
		}
		if msg.output == "" && msg.err == nil {
			m.cmdOutput += "(no output)\n"
		}
		m.cmdViewport.SetContent(m.cmdOutput)
		m.cmdViewport.GotoBottom()
		// Refresh status and branches after command
		if m.detailRepo != nil {
			cmds = append(cmds, loadGitDetail(m.detailRepo.Path), loadBranches(m.detailRepo.Path), checkGitStatus(m.detailRepo.Path))
		}
	}

	// Track filter state changes
	wasFiltering := m.list.FilterState() == list.Filtering || m.list.FilterState() == list.FilterApplied

	if m.mode == listView {
		var cmd tea.Cmd
		m.list, cmd = m.list.Update(msg)
		cmds = append(cmds, cmd)
	}

	// If filter was just cleared, rebuild the list with updated data
	isFiltering := m.list.FilterState() == list.Filtering || m.list.FilterState() == list.FilterApplied
	if wasFiltering && !isFiltering {
		m.updateList()
	}

	return m, tea.Batch(cmds...)
}

func (m *model) updateRepoFavorites() {
	for i := range m.repos {
		m.repos[i].IsFavorite = m.favorites[m.repos[i].Path]
	}
	m.updateList()
}

func (m *model) updateList() {
	sort.Slice(m.repos, func(i, j int) bool {
		if m.repos[i].IsFavorite != m.repos[j].IsFavorite {
			return m.repos[i].IsFavorite
		}
		return m.repos[i].Name < m.repos[j].Name
	})

	// Apply status filters
	var filtered []Repo
	for _, repo := range m.repos {
		if m.filterDirty && repo.Status != StatusDirty {
			continue
		}
		if m.filterBehind && repo.BehindCount == 0 {
			continue
		}
		filtered = append(filtered, repo)
	}

	items := make([]list.Item, len(filtered))
	for i, repo := range filtered {
		items[i] = repo
	}
	m.list.SetItems(items)
}

// getFilteredRepos returns repos matching current status filters
func (m *model) getFilteredRepos() []Repo {
	var filtered []Repo
	for _, repo := range m.repos {
		if m.filterDirty && repo.Status != StatusDirty {
			continue
		}
		if m.filterBehind && repo.BehindCount == 0 {
			continue
		}
		filtered = append(filtered, repo)
	}
	return filtered
}


func (m model) View() string {
	if m.mode == configView {
		title := detailTitleStyle.Render("Configure Git Directory")
		help := helpStyle.Render("enter: save • esc: cancel")
		input := m.dirInput.View()
		if m.statusMsg == "Invalid directory" {
			input += "\n" + statusErrorStyle.Render("Invalid directory path")
		}
		return title + "\n\n" + input + "\n\n" + help
	}

	if m.mode == detailView && m.detailRepo != nil {
		title := detailTitleStyle.Render(fmt.Sprintf(" %s [%s]", m.detailRepo.Name, m.detailRepo.Branch))

		// Calculate pane widths
		totalWidth := m.width
		if totalWidth < 80 {
			totalWidth = 80
		}
		leftWidth := (totalWidth * 60) / 100  // 60% for status
		rightWidth := (totalWidth * 40) / 100 // 40% for branches

		// Pane border styles
		focusedBorder := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("205")).
			Padding(0, 1)
		normalBorder := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("241")).
			Padding(0, 1)

		// Status pane (left)
		statusTitle := "Status"
		if m.detailFocus == paneStatus {
			statusTitle = "● " + statusTitle
		}
		statusStyle := normalBorder.Width(leftWidth - 4)
		if m.detailFocus == paneStatus {
			statusStyle = focusedBorder.Width(leftWidth - 4)
		}

		// Adjust viewport size for status pane
		statusHeight := (m.height - 12) / 2
		if statusHeight < 5 {
			statusHeight = 5
		}
		m.viewport.Width = leftWidth - 6
		m.viewport.Height = statusHeight
		statusContent := m.viewport.View()
		statusPane := statusStyle.Height(statusHeight + 2).Render(branchStyle.Render(statusTitle) + "\n" + statusContent)

		// Branches pane (right)
		branchTitle := "Branches"
		if m.detailFocus == paneBranches {
			branchTitle = "● " + branchTitle
		}
		branchStyle := normalBorder.Width(rightWidth - 4)
		if m.detailFocus == paneBranches {
			branchStyle = focusedBorder.Width(rightWidth - 4)
		}

		var branchList strings.Builder
		if len(m.branches) == 0 {
			branchList.WriteString("Loading...")
		} else {
			maxBranches := statusHeight
			startIdx := 0
			if m.branchIndex >= maxBranches {
				startIdx = m.branchIndex - maxBranches + 1
			}
			for i := startIdx; i < len(m.branches) && i < startIdx+maxBranches; i++ {
				branch := m.branches[i]
				prefix := "  "
				style := lipgloss.NewStyle()
				displayName := branch.Name

				// Add indicator for local/remote status
				indicator := ""
				if branch.IsLocal && branch.IsRemote {
					indicator = " ↕" // both local and remote
				} else if branch.IsLocal && !branch.IsRemote {
					indicator = " ⚠" // local only (can delete)
					style = style.Foreground(lipgloss.Color("214")) // orange
				} else if !branch.IsLocal && branch.IsRemote {
					indicator = " ☁" // remote only (can checkout)
					style = style.Foreground(lipgloss.Color("39")) // blue
				}

				if i == m.branchIndex {
					prefix = "> "
					style = style.Bold(true).Foreground(lipgloss.Color("205"))
				}
				if branch.IsCurrent {
					displayName = branch.Name + " ✓"
					if i != m.branchIndex {
						style = style.Foreground(lipgloss.Color("42"))
					}
					indicator = ""
				}
				branchList.WriteString(prefix + style.Render(displayName+indicator) + "\n")
			}
			if len(m.branches) > maxBranches {
				branchList.WriteString(helpStyle.Render(fmt.Sprintf("  ... %d more", len(m.branches)-maxBranches)))
			}
		}
		branchPane := branchStyle.Height(statusHeight + 2).Render(lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Render(branchTitle) + "\n" + branchList.String())

		// Join left and right panes horizontally
		topRow := lipgloss.JoinHorizontal(lipgloss.Top, statusPane, branchPane)

		// Command pane (bottom)
		cmdTitle := "Command"
		if m.detailFocus == paneCommand {
			cmdTitle = "● " + cmdTitle
		}
		cmdStyle := normalBorder.Width(totalWidth - 4)
		if m.detailFocus == paneCommand {
			cmdStyle = focusedBorder.Width(totalWidth - 4)
		}

		// Adjust command viewport size
		cmdHeight := 6
		m.cmdViewport.Width = totalWidth - 8
		m.cmdViewport.Height = cmdHeight - 2

		cmdContent := m.cmdInput.View() + "\n" + helpStyle.Render("─────────────────────────────────────") + "\n"
		if m.cmdOutput != "" {
			m.cmdViewport.SetContent(m.cmdOutput)
			cmdContent += m.cmdViewport.View()
		} else {
			cmdContent += helpStyle.Render("Output will appear here...")
		}
		cmdPane := cmdStyle.Render(lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Render(cmdTitle) + "\n" + cmdContent)

		// Status message
		var statusLine string
		if m.errorMsg != "" {
			statusLine = statusErrorStyle.Render("Error: " + m.errorMsg)
		} else if m.statusMsg != "" {
			statusLine = successStyle.Render(m.statusMsg)
		}

		help := helpStyle.Render("tab: pane • ↑/↓: scroll • enter: switch/run • p: pull remote • x: delete local • r: refresh • esc: back")
		help2 := helpStyle.Render("↕ local+remote • ⚠ local only • ☁ remote only")

		return title + "\n" + topRow + "\n" + cmdPane + "\n" + statusLine + "\n" + help + "\n" + help2
	}

	if m.mode == actionSelectView && m.detailRepo != nil {
		title := detailTitleStyle.Render("Uncommitted Changes Detected")
		subtitle := statusDirtyStyle.Render(fmt.Sprintf("Cannot switch to '%s' with uncommitted changes.\nWhat would you like to do?", m.targetBranch))

		actions := []string{"Stash changes (can restore later)", "Discard changes (permanent)", "Cancel"}
		var actionList strings.Builder
		for i, action := range actions {
			prefix := "  "
			style := lipgloss.NewStyle()
			if i == m.actionIndex {
				prefix = "> "
				style = style.Bold(true).Foreground(lipgloss.Color("205"))
			}
			if i == 1 { // Discard is dangerous
				if i == m.actionIndex {
					style = style.Foreground(lipgloss.Color("196"))
				} else {
					style = style.Foreground(lipgloss.Color("196"))
				}
			}
			actionList.WriteString(prefix + style.Render(action) + "\n")
		}

		help := helpStyle.Render("↑/↓: select • enter: confirm • esc: cancel")
		return title + "\n\n" + subtitle + "\n\n" + actionList.String() + "\n" + help
	}

	if m.mode == errorView {
		title := statusErrorStyle.Render("Error")
		help := helpStyle.Render("↑/↓: scroll • esc/enter: dismiss")
		content := m.viewport.View()
		return title + "\n\n" + content + "\n\n" + help
	}

	if m.mode == settingsView {
		title := detailTitleStyle.Render("Settings - Fetch Mode")

		options := []struct {
			name string
			desc string
		}{
			{"Fetch all repos", "Fetch all on startup; 'r' refreshes all (default)"},
			{"On-demand fetch", "No auto-fetch; 'r' refreshes selected, 'ctrl+r' refreshes all"},
			{"Favorites only", "Fetch favorites on startup; 'r' refreshes favorites, 'ctrl+r' all"},
		}

		var optionsList strings.Builder
		optionsList.WriteString("\n")
		for i, opt := range options {
			prefix := "  "
			style := lipgloss.NewStyle()
			if i == m.settingsIndex {
				prefix = "> "
				style = style.Bold(true).Foreground(lipgloss.Color("205"))
			}

			radio := "( )"
			if FetchMode(i) == m.fetchMode {
				radio = "(●)"
			}

			optionsList.WriteString(prefix + style.Render(radio+" "+opt.name) + "\n")
			optionsList.WriteString("     " + helpStyle.Render(opt.desc) + "\n\n")
		}

		help := helpStyle.Render("↑/↓: select • enter/space: choose • esc: back")
		return title + "\n" + optionsList.String() + help
	}

	// Build filter indicator
	var filterIndicator string
	if m.filterDirty || m.filterBehind {
		var filters []string
		if m.filterDirty {
			filters = append(filters, "local changes")
		}
		if m.filterBehind {
			filters = append(filters, "behind remote")
		}
		filterIndicator = statusDirtyStyle.Render("[Filter: " + strings.Join(filters, " + ") + "] ")
	}

	var status string
	if m.scanning {
		status = m.spinner.View() + " Scanning for repositories..."
	} else if m.pulling {
		status = m.spinner.View() + " " + m.statusMsg
	} else if m.errorMsg != "" {
		status = statusErrorStyle.Render(m.errorMsg)
	} else if m.statusMsg != "" {
		status = filterIndicator + successStyle.Render(m.statusMsg)
	} else if filterIndicator != "" {
		status = filterIndicator
	}

	help := helpStyle.Render("s: lazygit • d: details • f: fav • p: pull • P: pull favs • A: pull behind • g: goto • r/ctrl+r: refresh")
	help2 := helpStyle.Render("1: filter dirty • 2: filter behind • 0: clear • /: search • c: config • S: settings • q: quit")

	return m.list.View() + "\n" + status + "\n" + help + "\n" + help2
}

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
		// git branch <local-name> <remote-name>
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

func getConfigDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "guppi")
}

func getFavoritesPath() string {
	return filepath.Join(getConfigDir(), "favorites.json")
}

func getConfigPath() string {
	return filepath.Join(getConfigDir(), "config.json")
}

type FetchMode int

const (
	FetchAll        FetchMode = iota // Fetch all repos (default)
	FetchOnDemand                    // Only fetch visible repos
	FetchFavorites                   // Only fetch favorites
)

type Config struct {
	GitDir        string    `json:"gitDir"`
	SetupComplete bool      `json:"setupComplete"`
	FetchMode     FetchMode `json:"fetchMode"` // How to fetch repo status
}

func loadConfig() Config {
	var config Config

	data, err := os.ReadFile(getConfigPath())
	if err != nil {
		return config
	}

	json.Unmarshal(data, &config)
	return config
}

func saveConfigFull(config Config) {
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return
	}

	os.MkdirAll(getConfigDir(), 0755)
	os.WriteFile(getConfigPath(), data, 0644)
}

func saveConfig(gitDir string) {
	config := loadConfig()
	config.GitDir = gitDir
	saveConfigFull(config)
}

func loadFavorites() map[string]bool {
	favorites := make(map[string]bool)

	data, err := os.ReadFile(getFavoritesPath())
	if err != nil {
		return favorites
	}

	var paths []string
	if err := json.Unmarshal(data, &paths); err != nil {
		return favorites
	}

	for _, path := range paths {
		favorites[path] = true
	}
	return favorites
}

func saveFavorites(favorites map[string]bool) {
	var paths []string
	for path, isFav := range favorites {
		if isFav {
			paths = append(paths, path)
		}
	}

	data, err := json.MarshalIndent(paths, "", "  ")
	if err != nil {
		return
	}

	os.MkdirAll(getConfigDir(), 0755)
	os.WriteFile(getFavoritesPath(), data, 0644)
}

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

func getGotoFilePath() string {
	return filepath.Join(getConfigDir(), ".goto")
}

func getShellFunction(shellType string) string {
	gotoFile := getGotoFilePath()

	// Get the actual binary path
	binaryPath, err := os.Executable()
	if err != nil {
		// Fallback to command name (assumes it's in PATH)
		binaryPath = "guppi"
	} else {
		// Resolve symlinks to get the real path
		binaryPath, _ = filepath.EvalSymlinks(binaryPath)
	}

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
`, binaryPath, gotoFile, gotoFile, gotoFile)
	}
}

func checkShellSetup() bool {
	rcPath, _ := getShellConfig()
	data, err := os.ReadFile(rcPath)
	if err != nil {
		return false
	}
	return strings.Contains(string(data), "guppi")
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
	if shellAlreadySetup {
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
	saveConfigFull(config)

	fmt.Fprintln(os.Stderr, successStyle.Render("Setup complete! Starting guppi..."))
	fmt.Fprintln(os.Stderr)
	return true
}

const version = "1.1.0"

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
	fmt.Println("Key bindings (list view):")
	fmt.Println("  s         Open lazygit for selected repo")
	fmt.Println("  d         Open detail view (multi-pane)")
	fmt.Println("  f         Toggle favorite")
	fmt.Println("  p/Enter   Pull selected repo")
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
