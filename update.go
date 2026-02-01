package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

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
		// Handle pull results view keys
		if m.mode == pullResultsView {
			switch msg.String() {
			case "q", "esc":
				m.mode = listView
				m.pullResults = nil
				return m, nil
			case "up", "k":
				if m.pullResultsCursor > 0 {
					m.pullResultsCursor--
				}
				return m, nil
			case "down", "j":
				if m.pullResultsCursor < len(m.pullResults)-1 {
					m.pullResultsCursor++
				}
				return m, nil
			case "enter", " ":
				// Toggle expand/collapse for current repo
				if m.pullResultsCursor < len(m.pullResults) {
					r := m.pullResults[m.pullResultsCursor]
					m.pullExpanded[r.RepoPath] = !m.pullExpanded[r.RepoPath]
				}
				return m, nil
			case "a":
				// Expand/collapse all
				allExpanded := true
				for _, r := range m.pullResults {
					if !m.pullExpanded[r.RepoPath] {
						allExpanded = false
						break
					}
				}
				for _, r := range m.pullResults {
					m.pullExpanded[r.RepoPath] = !allExpanded
				}
				return m, nil
			}
			return m, nil
		}

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
				m.detailFocus = (m.detailFocus + 1) % 3
				if m.detailFocus == paneCommand {
					m.cmdInput.Focus()
				} else {
					m.cmdInput.Blur()
				}
				return m, nil
			case "shift+tab":
				m.detailFocus = (m.detailFocus + 2) % 3
				if m.detailFocus == paneCommand {
					m.cmdInput.Focus()
				} else {
					m.cmdInput.Blur()
				}
				return m, nil
			case "r":
				if m.detailRepo != nil && m.detailFocus != paneCommand {
					return m, tea.Batch(loadGitDetail(m.detailRepo.Path), loadBranches(m.detailRepo.Path))
				}
			}

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
						checkoutName := branch.Name
						if !branch.IsLocal && branch.IsRemote {
							checkoutName = branch.RemoteName
						}
						m.targetBranch = branch.Name
						if hasUncommittedChanges(m.detailRepo.Path) {
							m.hasChanges = true
							m.mode = actionSelectView
							m.actionIndex = 0
							return m, nil
						}
						m.statusMsg = "Switching to " + branch.Name + "..."
						return m, switchBranch(m.detailRepo.Path, checkoutName)
					}
					return m, nil
				case "x":
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
						return m, deleteBranch(m.detailRepo.Path, branch.Name, false)
					}
					return m, nil
				case "X":
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

		// Handle action select view keys
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
				case 0:
					m.statusMsg = "Stashing changes..."
					return m, stashChanges(m.detailRepo.Path)
				case 1:
					m.statusMsg = "Discarding changes..."
					return m, discardChanges(m.detailRepo.Path)
				case 2:
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
				if m.previousMode == detailView && m.detailRepo != nil {
					m.mode = detailView
					return m, loadGitDetail(m.detailRepo.Path)
				}
				m.mode = listView
				m.detailRepo = nil
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
				if m.settingsIndex < 2 {
					m.settingsIndex++
				}
				return m, nil
			case "enter", " ":
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

		// Handle group input view keys
		if m.mode == groupInputView {
			switch msg.String() {
			case "esc":
				m.mode = listView
				m.groupInput.SetValue("")
				return m, nil
			case "enter":
				name := strings.TrimSpace(m.groupInput.Value())
				if name == "" {
					m.statusMsg = "Group name cannot be empty"
					return m, nil
				}
				if m.groupAction == "new" {
					if _, exists := m.groupsMap[name]; exists {
						m.statusMsg = "Group already exists: " + name
						return m, nil
					}
					newGroup := Group{Name: name, Repos: []string{}}
					m.groups = append(m.groups, newGroup)
					m.groupsMap[name] = &m.groups[len(m.groups)-1]
					saveGroups(m.groups)
					m.statusMsg = "Created group: " + name
				} else if m.groupAction == "rename" && m.currentGroup != nil {
					oldName := m.currentGroup.Name
					if name != oldName {
						if _, exists := m.groupsMap[name]; exists {
							m.statusMsg = "Group already exists: " + name
							return m, nil
						}
						delete(m.groupsMap, oldName)
						m.currentGroup.Name = name
						m.groupsMap[name] = m.currentGroup
						saveGroups(m.groups)
						m.statusMsg = "Renamed group to: " + name
					}
				}
				m.mode = listView
				m.groupInput.SetValue("")
				m.updateList()
				return m, nil
			}
			var cmd tea.Cmd
			m.groupInput, cmd = m.groupInput.Update(msg)
			return m, cmd
		}

		// Handle group delete confirmation view
		if m.mode == groupDeleteView {
			switch msg.String() {
			case "esc", "n":
				m.mode = listView
				return m, nil
			case "y", "enter":
				if m.currentGroup != nil {
					name := m.currentGroup.Name
					newGroups := make([]Group, 0, len(m.groups)-1)
					for _, g := range m.groups {
						if g.Name != name {
							newGroups = append(newGroups, g)
						}
					}
					m.groups = newGroups
					delete(m.groupsMap, name)
					m.groupsMap = buildGroupsMap(m.groups)
					saveGroups(m.groups)
					m.currentGroup = nil
					m.statusMsg = "Deleted group: " + name
				}
				m.mode = listView
				m.updateList()
				return m, nil
			}
			return m, nil
		}

		// Handle group select view
		if m.mode == groupSelectView {
			switch msg.String() {
			case "esc":
				m.mode = listView
				m.selectedRepo = nil
				return m, nil
			case "up", "k":
				if m.groupIndex > 0 {
					m.groupIndex--
				}
				return m, nil
			case "down", "j":
				maxIdx := len(m.groups)
				if m.groupIndex < maxIdx {
					m.groupIndex++
				}
				return m, nil
			case "enter":
				if m.selectedRepo == nil {
					m.mode = listView
					return m, nil
				}
				repoPath := m.selectedRepo.Path

				for i := range m.groups {
					newRepos := make([]string, 0)
					for _, p := range m.groups[i].Repos {
						if p != repoPath {
							newRepos = append(newRepos, p)
						}
					}
					m.groups[i].Repos = newRepos
				}

				if m.groupIndex < len(m.groups) {
					targetGroup := &m.groups[m.groupIndex]
					targetGroup.Repos = append(targetGroup.Repos, repoPath)
					m.statusMsg = "Moved " + m.selectedRepo.Name + " to " + targetGroup.Name
					if targetGroup.Name == "Favorites" {
						m.favorites[repoPath] = true
						for i := range m.repos {
							if m.repos[i].Path == repoPath {
								m.repos[i].IsFavorite = true
								break
							}
						}
						saveFavorites(m.favorites)
					}
				} else {
					m.statusMsg = "Removed " + m.selectedRepo.Name + " from group"
				}

				if m.favorites[repoPath] {
					stillInFavorites := false
					if favGroup, ok := m.groupsMap["Favorites"]; ok {
						for _, p := range favGroup.Repos {
							if p == repoPath {
								stillInFavorites = true
								break
							}
						}
					}
					if !stillInFavorites {
						m.favorites[repoPath] = false
						for i := range m.repos {
							if m.repos[i].Path == repoPath {
								m.repos[i].IsFavorite = false
								break
							}
						}
						saveFavorites(m.favorites)
					}
				}

				saveGroups(m.groups)
				m.groupsMap = buildGroupsMap(m.groups)
				m.mode = listView
				m.selectedRepo = nil
				// Preserve filter text when updating list
				filterText := ""
				if m.list.FilterState() == list.FilterApplied {
					filterText = m.list.FilterValue()
				}
				if filterText != "" && m.currentGroup == nil {
					m.updateListFlattened()
				} else {
					m.updateList()
				}
				if filterText != "" {
					m.list.SetFilterText(filterText)
				}
				return m, nil
			}
			return m, nil
		}

		// Handle add repos to group view
		if m.mode == groupAddReposView {
			switch msg.String() {
			case "esc":
				m.mode = listView
				return m, nil
			case "up", "k":
				if m.addRepoIndex > 0 {
					m.addRepoIndex--
				}
				return m, nil
			case "down", "j":
				if m.addRepoIndex < len(m.ungroupedRepos)-1 {
					m.addRepoIndex++
				}
				return m, nil
			case "enter", " ":
				if m.currentGroup != nil && len(m.ungroupedRepos) > 0 && m.addRepoIndex < len(m.ungroupedRepos) {
					repo := m.ungroupedRepos[m.addRepoIndex]
					m.currentGroup.Repos = append(m.currentGroup.Repos, repo.Path)
					saveGroups(m.groups)
					m.statusMsg = "Added " + repo.Name + " to " + m.currentGroup.Name
					m.ungroupedRepos = m.getUngroupedRepos()
					if m.addRepoIndex >= len(m.ungroupedRepos) {
						m.addRepoIndex = len(m.ungroupedRepos) - 1
					}
					if len(m.ungroupedRepos) == 0 {
						m.mode = listView
						m.updateList()
					}
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

		case "esc", "backspace":
			if m.currentGroup != nil {
				m.currentGroup = nil
				m.updateList()
				m.statusMsg = ""
				return m, nil
			}

		case "f":
			if item, ok := m.list.SelectedItem().(Repo); ok {
				m.favorites[item.Path] = !m.favorites[item.Path]
				for i := range m.repos {
					m.repos[i].IsFavorite = m.favorites[m.repos[i].Path]
				}
				if favGroup, ok := m.groupsMap["Favorites"]; ok {
					if m.favorites[item.Path] {
						favGroup.Repos = append(favGroup.Repos, item.Path)
					} else {
						newRepos := make([]string, 0, len(favGroup.Repos))
						for _, p := range favGroup.Repos {
							if p != item.Path {
								newRepos = append(newRepos, p)
							}
						}
						favGroup.Repos = newRepos
					}
				}
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

		case "enter":
			if group, ok := m.list.SelectedItem().(GroupItem); ok {
				if g, exists := m.groupsMap[group.Name]; exists {
					m.currentGroup = g
					m.updateList()
					m.statusMsg = "Entered group: " + group.Name
				}
				return m, nil
			}
			fallthrough
		case "p":
			if item, ok := m.list.SelectedItem().(Repo); ok {
				m.pulling = true
				m.statusMsg = "Pulling " + item.Name + "..."
				// Capture HEAD before pull for results tracking
				m.pendingPulls[item.Path] = getHeadCommit(item.Path)
				m.pullResults = nil // Clear previous results
				return m, tea.Batch(m.spinner.Tick, pullRepo(item.Path))
			}

		case "P":
			// Clear previous results
			m.pullResults = nil
			m.pendingPulls = make(map[string]string)

			// Inside a group: pull all repos in that group
			if m.currentGroup != nil {
				repos := m.getGroupRepos(m.currentGroup.Name)
				var pullCmds []tea.Cmd
				for _, repo := range repos {
					m.pendingPulls[repo.Path] = getHeadCommit(repo.Path)
					pullCmds = append(pullCmds, pullRepo(repo.Path))
				}
				if len(pullCmds) > 0 {
					m.pulling = true
					m.statusMsg = fmt.Sprintf("Pulling %d repos in %s...", len(pullCmds), m.currentGroup.Name)
					pullCmds = append(pullCmds, m.spinner.Tick)
					return m, tea.Batch(pullCmds...)
				}
				m.statusMsg = "No repos to pull in " + m.currentGroup.Name
				return m, nil
			}
			// On homepage with a group selected: pull all repos in that group
			if group, ok := m.list.SelectedItem().(GroupItem); ok {
				repos := m.getGroupRepos(group.Name)
				var pullCmds []tea.Cmd
				for _, repo := range repos {
					m.pendingPulls[repo.Path] = getHeadCommit(repo.Path)
					pullCmds = append(pullCmds, pullRepo(repo.Path))
				}
				if len(pullCmds) > 0 {
					m.pulling = true
					m.statusMsg = fmt.Sprintf("Pulling %d repos in %s...", len(pullCmds), group.Name)
					pullCmds = append(pullCmds, m.spinner.Tick)
					return m, tea.Batch(pullCmds...)
				}
				m.statusMsg = "No repos to pull in " + group.Name
				return m, nil
			}
			// Otherwise: pull all favorites
			var pullCmds []tea.Cmd
			count := 0
			for _, repo := range m.repos {
				if repo.IsFavorite {
					m.pendingPulls[repo.Path] = getHeadCommit(repo.Path)
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
			if m.currentGroup != nil {
				// Respect fetch mode inside groups too
				switch m.fetchMode {
				case FetchOnDemand:
					// Only refresh selected repo
					if item, ok := m.list.SelectedItem().(Repo); ok {
						m.statusMsg = fmt.Sprintf("Refreshing %s (1 repo)...", item.Name)
						return m, checkGitStatus(item.Path)
					}
					return m, nil
				case FetchFavorites:
					// Refresh all favorites + all repos in current group
					refreshed := make(map[string]bool)
					var cmds []tea.Cmd
					// Add all favorites
					for _, repo := range m.repos {
						if repo.IsFavorite {
							cmds = append(cmds, checkGitStatus(repo.Path))
							refreshed[repo.Path] = true
						}
					}
					// Add repos in current group (if not already added)
					groupRepos := m.getGroupRepos(m.currentGroup.Name)
					for _, repo := range groupRepos {
						if !refreshed[repo.Path] {
							cmds = append(cmds, checkGitStatus(repo.Path))
						}
					}
					if len(cmds) > 0 {
						m.statusMsg = fmt.Sprintf("Refreshing favorites + %s (%d repos)...", m.currentGroup.Name, len(cmds))
						return m, tea.Batch(cmds...)
					}
					m.statusMsg = "No repos to refresh"
					return m, nil
				default: // FetchAll
					repos := m.getGroupRepos(m.currentGroup.Name)
					var cmds []tea.Cmd
					for _, repo := range repos {
						cmds = append(cmds, checkGitStatus(repo.Path))
					}
					if len(cmds) > 0 {
						m.statusMsg = fmt.Sprintf("Refreshing %d repos in %s...", len(cmds), m.currentGroup.Name)
						return m, tea.Batch(cmds...)
					}
					m.statusMsg = "No repos to refresh in " + m.currentGroup.Name
					return m, nil
				}
			}
			// On homepage with a group selected: refresh all repos in that group
			if group, ok := m.list.SelectedItem().(GroupItem); ok {
				repos := m.getGroupRepos(group.Name)
				var cmds []tea.Cmd
				for _, repo := range repos {
					cmds = append(cmds, checkGitStatus(repo.Path))
				}
				if len(cmds) > 0 {
					m.statusMsg = fmt.Sprintf("Refreshing %d repos in %s...", len(cmds), group.Name)
					return m, tea.Batch(cmds...)
				}
				m.statusMsg = "No repos to refresh in " + group.Name
				return m, nil
			}
			switch m.fetchMode {
			case FetchOnDemand:
				if item, ok := m.list.SelectedItem().(Repo); ok {
					m.statusMsg = fmt.Sprintf("Refreshing %s (1 repo)...", item.Name)
					return m, checkGitStatus(item.Path)
				}
				return m, nil
			case FetchFavorites:
				// Refresh all favorites + selected item
				refreshed := make(map[string]bool)
				var cmds []tea.Cmd
				// Add all favorites
				for _, repo := range m.repos {
					if repo.IsFavorite {
						cmds = append(cmds, checkGitStatus(repo.Path))
						refreshed[repo.Path] = true
					}
				}
				// Add selected repo if not already a favorite
				if item, ok := m.list.SelectedItem().(Repo); ok {
					if !refreshed[item.Path] {
						cmds = append(cmds, checkGitStatus(item.Path))
					}
				}
				if len(cmds) > 0 {
					m.statusMsg = fmt.Sprintf("Refreshing favorites + selected (%d repos)...", len(cmds))
					return m, tea.Batch(cmds...)
				}
				m.statusMsg = "No repos to refresh"
				return m, nil
			default:
				m.scanning = true
				m.repos = []Repo{}
				if m.list.FilterState() == list.FilterApplied {
					m.savedFilter = m.list.FilterValue()
				}
				m.list.SetItems([]list.Item{})
				m.statusMsg = "Scanning..."
				return m, tea.Batch(m.spinner.Tick, scanForRepos(m.gitDir))
			}

		case "ctrl+r":
			// Inside a group: refresh all repos in the group
			if m.currentGroup != nil {
				repos := m.getGroupRepos(m.currentGroup.Name)
				var cmds []tea.Cmd
				for _, repo := range repos {
					cmds = append(cmds, checkGitStatus(repo.Path))
				}
				if len(cmds) > 0 {
					m.statusMsg = fmt.Sprintf("Refreshing all %d repos in %s...", len(cmds), m.currentGroup.Name)
					return m, tea.Batch(cmds...)
				}
				m.statusMsg = "No repos to refresh in " + m.currentGroup.Name
				return m, nil
			}
			// Homepage: full rescan
			m.scanning = true
			m.repos = []Repo{}
			m.forceFullFetch = true
			if m.list.FilterState() == list.FilterApplied {
				m.savedFilter = m.list.FilterValue()
			}
			m.list.SetItems([]list.Item{})
			m.statusMsg = "Scanning all..."
			return m, tea.Batch(m.spinner.Tick, scanForRepos(m.gitDir))

		case "s":
			if item, ok := m.list.SelectedItem().(Repo); ok {
				m.detailRepo = &item
				c := exec.Command("lazygit")
				c.Dir = item.Path
				return m, tea.ExecProcess(c, func(err error) tea.Msg {
					return lazygitExitMsg{path: item.Path, err: err}
				})
			}

		case "o":
			if item, ok := m.list.SelectedItem().(Repo); ok {
				url, err := getRepoWebURL(item.Path)
				if err != nil {
					m.statusMsg = "No remote URL found"
					return m, nil
				}
				if err := openInBrowser(url); err != nil {
					m.statusMsg = "Failed to open browser"
					return m, nil
				}
				m.statusMsg = "Opened " + url
			}

		case "d":
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
			m.filterDirty = false
			m.filterBehind = false
			m.updateList()
			m.statusMsg = "Filters cleared"

		case "A":
			// Clear previous results
			m.pullResults = nil
			m.pendingPulls = make(map[string]string)

			filtered := m.getFilteredRepos()
			var pullCmds []tea.Cmd
			count := 0
			for _, repo := range filtered {
				if repo.BehindCount > 0 {
					m.pendingPulls[repo.Path] = getHeadCommit(repo.Path)
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
			m.mode = settingsView
			m.settingsIndex = int(m.fetchMode)
			return m, nil

		case "n":
			if m.currentGroup == nil {
				m.mode = groupInputView
				m.groupAction = "new"
				m.groupInput.SetValue("")
				m.groupInput.Focus()
				return m, textinput.Blink
			}

		case "e":
			if m.currentGroup != nil && !m.currentGroup.IsBuiltIn {
				m.mode = groupInputView
				m.groupAction = "rename"
				m.groupInput.SetValue(m.currentGroup.Name)
				m.groupInput.Focus()
				return m, textinput.Blink
			} else if group, ok := m.list.SelectedItem().(GroupItem); ok {
				if g, exists := m.groupsMap[group.Name]; exists && !g.IsBuiltIn {
					m.currentGroup = g
					m.mode = groupInputView
					m.groupAction = "rename"
					m.groupInput.SetValue(group.Name)
					m.groupInput.Focus()
					return m, textinput.Blink
				} else if g != nil && g.IsBuiltIn {
					m.statusMsg = "Cannot rename built-in group"
				}
			}

		case "x":
			if m.currentGroup != nil {
				if item, ok := m.list.SelectedItem().(Repo); ok {
					newRepos := make([]string, 0)
					for _, p := range m.currentGroup.Repos {
						if p != item.Path {
							newRepos = append(newRepos, p)
						}
					}
					m.currentGroup.Repos = newRepos
					if m.currentGroup.Name == "Favorites" {
						m.favorites[item.Path] = false
						for i := range m.repos {
							if m.repos[i].Path == item.Path {
								m.repos[i].IsFavorite = false
								break
							}
						}
						saveFavorites(m.favorites)
					}
					saveGroups(m.groups)
					m.statusMsg = "Removed " + item.Name + " from " + m.currentGroup.Name
					m.updateList()
				}
				return m, nil
			}
			if group, ok := m.list.SelectedItem().(GroupItem); ok {
				if g, exists := m.groupsMap[group.Name]; exists {
					if g.IsBuiltIn {
						m.statusMsg = "Cannot delete built-in group"
						return m, nil
					}
					m.currentGroup = g
					m.mode = groupDeleteView
				}
			}
			return m, nil

		case "a":
			if m.currentGroup != nil {
				m.ungroupedRepos = m.getUngroupedRepos()
				if len(m.ungroupedRepos) == 0 {
					m.statusMsg = "No ungrouped repos to add"
					return m, nil
				}
				m.addRepoIndex = 0
				m.mode = groupAddReposView
				return m, nil
			}

		case "m":
			if item, ok := m.list.SelectedItem().(Repo); ok {
				m.selectedRepo = &item
				m.groupIndex = 0
				m.mode = groupSelectView
				return m, nil
			}
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
		if m.savedFilter != "" {
			m.list.SetFilterText(m.savedFilter)
			m.savedFilter = ""
		}

		var statusCmds []tea.Cmd
		if m.forceFullFetch {
			m.forceFullFetch = false
			for _, repo := range m.repos {
				statusCmds = append(statusCmds, checkGitStatus(repo.Path))
			}
		} else {
			switch m.fetchMode {
			case FetchOnDemand:
			case FetchFavorites:
				for _, repo := range m.repos {
					if repo.IsFavorite {
						statusCmds = append(statusCmds, checkGitStatus(repo.Path))
					}
				}
			default:
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
		if m.list.FilterState() == list.Filtering {
			break
		}
		filterText := ""
		if m.list.FilterState() == list.FilterApplied {
			filterText = m.list.FilterValue()
		}
		m.updateList()
		if filterText != "" {
			m.list.SetFilterText(filterText)
		}

	case pullCompleteMsg:
		repoName := filepath.Base(msg.path)
		for i := range m.repos {
			if m.repos[i].Path == msg.path {
				repoName = m.repos[i].Name
				if msg.err != nil {
					m.repos[i].PullResult = "error"
				} else {
					m.repos[i].PullResult = msg.shortResult
					m.errorMsg = ""
				}
				break
			}
		}

		// Collect pull results for results screen
		if oldHead, ok := m.pendingPulls[msg.path]; ok {
			delete(m.pendingPulls, msg.path)

			if msg.err == nil && !strings.Contains(msg.result, "Already up to date") {
				newHead := getHeadCommit(msg.path)
				commits := getCommitsBetween(msg.path, oldHead, newHead)
				filesChanged := getFilesChangedCount(msg.path, oldHead, newHead)

				if len(commits) > 0 {
					m.pullResults = append(m.pullResults, PullResultInfo{
						RepoPath:     msg.path,
						RepoName:     repoName,
						Commits:      commits,
						FilesChanged: filesChanged,
						Updated:      true,
					})
				}
			}
		}

		// Check if all pulls are done
		allDone := len(m.pendingPulls) == 0

		if msg.err != nil {
			m.statusMsg = ""
			m.errorMsg = fmt.Sprintf("Pull failed for %s:\n\n%s", repoName, msg.result)
			m.previousMode = m.mode
			if m.list.FilterState() == list.FilterApplied {
				m.savedFilter = m.list.FilterValue()
			}
			m.mode = errorView
			m.viewport.SetContent(m.errorMsg)
			m.pulling = !allDone
		} else {
			filterText := ""
			if m.list.FilterState() == list.FilterApplied {
				filterText = m.list.FilterValue()
			}
			m.updateList()
			if filterText != "" {
				m.list.SetFilterText(filterText)
			}

			if allDone {
				m.pulling = false
				// Show results screen if enabled and there are results
				if m.showPullResults && len(m.pullResults) > 0 {
					m.mode = pullResultsView
					m.pullResultsCursor = 0
					m.pullExpanded = make(map[string]bool)
					// Expand first repo by default
					if len(m.pullResults) > 0 {
						m.pullExpanded[m.pullResults[0].RepoPath] = true
					}
					m.statusMsg = ""
				} else {
					m.statusMsg = fmt.Sprintf("Pulled %s: %s", repoName, msg.shortResult)
				}
			} else {
				m.statusMsg = fmt.Sprintf("Pulled %s: %s", repoName, msg.shortResult)
			}
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
			if m.detailRepo != nil {
				cmds = append(cmds, loadBranches(m.detailRepo.Path))
			}
		} else {
			m.errorMsg = "Delete failed: " + msg.err
		}

	case branchCreateMsg:
		if msg.success {
			m.statusMsg = "Created local branch: " + msg.branch
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
			if m.detailRepo != nil {
				m.detailRepo.Branch = msg.branch
				for i := range m.repos {
					if m.repos[i].Path == msg.path {
						m.repos[i].Branch = msg.branch
						break
					}
				}
			}
			cmds = append(cmds, loadGitDetail(msg.path), loadBranches(msg.path), checkGitStatus(msg.path))
		} else {
			m.errorMsg = "Branch switch failed:\n\n" + msg.err
			m.previousMode = m.mode
			if m.list.FilterState() == list.FilterApplied {
				m.savedFilter = m.list.FilterValue()
			}
			m.mode = errorView
			m.viewport.SetContent(m.errorMsg)
		}

	case stashResultMsg:
		if msg.success {
			if m.detailRepo != nil && m.targetBranch != "" {
				m.mode = detailView
				m.statusMsg = "Switching to " + m.targetBranch + "..."
				m.errorMsg = ""
				cmds = append(cmds, switchBranch(m.detailRepo.Path, m.targetBranch))
			}
		} else {
			m.errorMsg = "Operation failed:\n\n" + msg.err
			m.previousMode = m.mode
			if m.list.FilterState() == list.FilterApplied {
				m.savedFilter = m.list.FilterValue()
			}
			m.mode = errorView
			m.viewport.SetContent(m.errorMsg)
		}

	case lazygitExitMsg:
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
		if m.detailRepo != nil {
			cmds = append(cmds, loadGitDetail(m.detailRepo.Path), loadBranches(m.detailRepo.Path), checkGitStatus(m.detailRepo.Path))
		}
	}

	wasFiltering := m.list.FilterState() == list.Filtering || m.list.FilterState() == list.FilterApplied

	if m.mode == listView {
		var cmd tea.Cmd
		m.list, cmd = m.list.Update(msg)
		cmds = append(cmds, cmd)
	}

	isFiltering := m.list.FilterState() == list.Filtering || m.list.FilterState() == list.FilterApplied

	if !wasFiltering && isFiltering && m.currentGroup == nil {
		m.updateListFlattened()
	}

	if wasFiltering && !isFiltering {
		m.updateList()
	}

	return m, tea.Batch(cmds...)
}
