package main

import (
	"sort"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type model struct {
	list         list.Model
	delegate     *repoDelegate
	repos        []Repo
	favorites    map[string]bool
	scanning     bool
	pulling      bool
	spinner      spinner.Model
	statusMsg    string
	errorMsg     string
	width        int
	height       int
	gitDir       string
	mode         viewMode
	previousMode viewMode // for returning from error view
	savedFilter  string   // saved filter text for restoring after error
	detailRepo   *Repo
	detailContent string
	viewport     viewport.Model
	dirInput     textinput.Model
	gotoPath     string // path to cd to after exit

	// Branch switching
	branches     []BranchInfo
	branchIndex  int
	targetBranch string
	actionIndex  int
	hasChanges   bool

	// Status filters
	filterDirty  bool // show only repos with local changes
	filterBehind bool // show only repos behind remote

	// Detail view panes
	detailFocus detailPane      // which pane has focus
	cmdInput    textinput.Model // command input
	cmdOutput   string          // command output
	cmdViewport viewport.Model  // viewport for command output
	cmdRunning  bool            // is a command running

	// Performance config
	fetchMode      FetchMode // How to fetch repo status
	settingsIndex  int       // Current selection in settings view
	forceFullFetch bool      // Force full fetch on next scan (for ctrl+r)

	// Groups
	groups         []Group           // all groups including Favorites
	groupsMap      map[string]*Group // by name for quick lookup
	currentGroup   *Group            // nil = homepage, non-nil = inside group
	groupInput     textinput.Model   // text input for group name
	groupAction    string            // "new", "rename", "delete"
	selectedRepo   *Repo             // repo selected for move operation
	groupIndex     int               // selection in group picker
	addRepoIndex   int               // selection in add repos picker
	ungroupedRepos []Repo            // repos not in current group for picker
}

func initialModel(gitDir string) model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	favorites := loadFavorites()
	config := loadConfig()

	// Load groups and create Favorites as built-in group
	groups := loadGroups()

	// Create Favorites group from favorites.json
	favRepos := make([]string, 0, len(favorites))
	for path, isFav := range favorites {
		if isFav {
			favRepos = append(favRepos, path)
		}
	}
	favGroup := Group{
		Name:      "Favorites",
		Repos:     favRepos,
		IsBuiltIn: true,
	}
	// Prepend Favorites group
	groups = append([]Group{favGroup}, groups...)
	groupsMap := buildGroupsMap(groups)

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

	// Group name input
	groupInput := textinput.New()
	groupInput.Placeholder = "Enter group name..."
	groupInput.CharLimit = 50
	groupInput.Width = 40

	cmdVp := viewport.New(80, 10)

	return model{
		list:        l,
		delegate:    &delegate,
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
		groups:      groups,
		groupsMap:   groupsMap,
		groupInput:  groupInput,
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
		scanForRepos(m.gitDir),
	)
}

// Helper methods for model

func (m *model) updateRepoFavorites() {
	for i := range m.repos {
		m.repos[i].IsFavorite = m.favorites[m.repos[i].Path]
	}
	m.updateList()
}

// getRepoGroup returns the group name for a repo, empty if ungrouped
func (m *model) getRepoGroup(path string) string {
	for _, g := range m.groups {
		for _, r := range g.Repos {
			if r == path {
				return g.Name
			}
		}
	}
	return ""
}

// isRepoInGroup checks if a repo is in any group
func (m *model) isRepoInGroup(path string) bool {
	return m.getRepoGroup(path) != ""
}

// getGroupRepos returns all repos that belong to a specific group
func (m *model) getGroupRepos(groupName string) []Repo {
	group, ok := m.groupsMap[groupName]
	if !ok {
		return nil
	}
	repoSet := make(map[string]bool)
	for _, path := range group.Repos {
		repoSet[path] = true
	}
	var result []Repo
	for _, repo := range m.repos {
		if repoSet[repo.Path] {
			result = append(result, repo)
		}
	}
	return result
}

// getUngroupedRepos returns repos not in any group
func (m *model) getUngroupedRepos() []Repo {
	grouped := make(map[string]bool)
	for _, g := range m.groups {
		for _, path := range g.Repos {
			grouped[path] = true
		}
	}
	var result []Repo
	for _, repo := range m.repos {
		if !grouped[repo.Path] {
			result = append(result, repo)
		}
	}
	return result
}

// buildGroupStats builds GroupItem with stats from repos
func (m *model) buildGroupStats(group Group) GroupItem {
	item := GroupItem{Name: group.Name}
	repoSet := make(map[string]bool)
	for _, path := range group.Repos {
		repoSet[path] = true
	}
	for _, repo := range m.repos {
		if repoSet[repo.Path] {
			item.RepoCount++
			if repo.Status == StatusDirty {
				item.DirtyCount++
			}
			if repo.BehindCount > 0 {
				item.BehindCount++
			}
		}
	}
	return item
}

func (m *model) updateList() {
	// Update delegate's repoGroups map for display
	m.delegate.repoGroups = make(map[string]string)
	for _, g := range m.groups {
		for _, path := range g.Repos {
			m.delegate.repoGroups[path] = g.Name
		}
	}
	m.list.SetDelegate(*m.delegate)

	// If inside a group, show only that group's repos
	if m.currentGroup != nil {
		repos := m.getGroupRepos(m.currentGroup.Name)
		sort.Slice(repos, func(i, j int) bool {
			return repos[i].Name < repos[j].Name
		})

		// Apply status filters
		var filtered []Repo
		for _, repo := range repos {
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
		m.list.Title = "📁 " + m.currentGroup.Name
		return
	}

	// Homepage view: show groups as folders + ungrouped repos
	m.list.Title = "guppi - Git Repository Manager"

	var items []list.Item

	// Add groups (Favorites first, then alphabetically)
	var sortedGroups []Group
	for _, g := range m.groups {
		// Only show groups with repos
		stats := m.buildGroupStats(g)
		if stats.RepoCount > 0 || !g.IsBuiltIn {
			sortedGroups = append(sortedGroups, g)
		}
	}
	sort.Slice(sortedGroups, func(i, j int) bool {
		// Favorites always first
		if sortedGroups[i].Name == "Favorites" {
			return true
		}
		if sortedGroups[j].Name == "Favorites" {
			return false
		}
		return sortedGroups[i].Name < sortedGroups[j].Name
	})

	for _, g := range sortedGroups {
		stats := m.buildGroupStats(g)
		items = append(items, stats)
	}

	// Add ungrouped repos
	ungrouped := m.getUngroupedRepos()
	sort.Slice(ungrouped, func(i, j int) bool {
		if ungrouped[i].IsFavorite != ungrouped[j].IsFavorite {
			return ungrouped[i].IsFavorite
		}
		return ungrouped[i].Name < ungrouped[j].Name
	})

	// Apply status filters to ungrouped repos
	for _, repo := range ungrouped {
		if m.filterDirty && repo.Status != StatusDirty {
			continue
		}
		if m.filterBehind && repo.BehindCount == 0 {
			continue
		}
		items = append(items, repo)
	}

	m.list.SetItems(items)
}

// updateListFlattened shows all repos in a flat list with group prefixes (used during filtering on homepage)
func (m *model) updateListFlattened() {
	// Update delegate's repoGroups map for display
	m.delegate.repoGroups = make(map[string]string)
	for _, g := range m.groups {
		for _, path := range g.Repos {
			m.delegate.repoGroups[path] = g.Name
		}
	}
	m.list.SetDelegate(*m.delegate)

	// Sort all repos: favorites first, then alphabetically
	allRepos := make([]Repo, len(m.repos))
	copy(allRepos, m.repos)
	sort.Slice(allRepos, func(i, j int) bool {
		if allRepos[i].IsFavorite != allRepos[j].IsFavorite {
			return allRepos[i].IsFavorite
		}
		return allRepos[i].Name < allRepos[j].Name
	})

	// Apply status filters
	var filtered []Repo
	for _, repo := range allRepos {
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
