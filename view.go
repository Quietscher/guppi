package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

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

	if m.mode == groupInputView {
		action := "Create New Group"
		if m.groupAction == "rename" {
			action = "Rename Group"
		}
		title := detailTitleStyle.Render(action)
		help := helpStyle.Render("enter: save • esc: cancel")
		input := m.groupInput.View()
		return title + "\n\n" + input + "\n\n" + help
	}

	if m.mode == groupDeleteView && m.currentGroup != nil {
		title := statusErrorStyle.Render("Delete Group: " + m.currentGroup.Name + "?")
		subtitle := helpStyle.Render(fmt.Sprintf("This group contains %d repos. They will be ungrouped.", len(m.currentGroup.Repos)))
		help := helpStyle.Render("y/enter: delete • n/esc: cancel")
		return title + "\n\n" + subtitle + "\n\n" + help
	}

	if m.mode == groupSelectView && m.selectedRepo != nil {
		title := detailTitleStyle.Render("Move " + m.selectedRepo.Name + " to group:")

		var list strings.Builder
		for i, g := range m.groups {
			prefix := "  "
			style := lipgloss.NewStyle()
			if i == m.groupIndex {
				prefix = "> "
				style = style.Bold(true).Foreground(lipgloss.Color("205"))
			}
			inGroup := false
			for _, p := range g.Repos {
				if p == m.selectedRepo.Path {
					inGroup = true
					break
				}
			}
			indicator := ""
			if inGroup {
				indicator = " ✓"
			}
			list.WriteString(prefix + style.Render("📁 "+g.Name+indicator) + "\n")
		}
		prefix := "  "
		style := lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
		if m.groupIndex == len(m.groups) {
			prefix = "> "
			style = style.Bold(true).Foreground(lipgloss.Color("205"))
		}
		list.WriteString(prefix + style.Render("(Remove from group)") + "\n")

		help := helpStyle.Render("↑/↓: select • enter: move • esc: cancel")
		return title + "\n\n" + list.String() + "\n" + help
	}

	if m.mode == groupAddReposView && m.currentGroup != nil {
		title := detailTitleStyle.Render("Add repos to " + m.currentGroup.Name + ":")

		var list strings.Builder
		maxShow := 15
		startIdx := 0
		if m.addRepoIndex >= maxShow {
			startIdx = m.addRepoIndex - maxShow + 1
		}
		for i := startIdx; i < len(m.ungroupedRepos) && i < startIdx+maxShow; i++ {
			repo := m.ungroupedRepos[i]
			prefix := "  "
			style := lipgloss.NewStyle()
			if i == m.addRepoIndex {
				prefix = "> "
				style = style.Bold(true).Foreground(lipgloss.Color("205"))
			}
			list.WriteString(prefix + style.Render(repo.Name) + "\n")
		}
		if len(m.ungroupedRepos) > maxShow {
			list.WriteString(helpStyle.Render(fmt.Sprintf("  ... %d more", len(m.ungroupedRepos)-maxShow)))
		}

		help := helpStyle.Render("↑/↓: select • enter/space: add • esc: done")
		status := ""
		if m.statusMsg != "" {
			status = successStyle.Render(m.statusMsg) + "\n"
		}
		return title + "\n\n" + list.String() + "\n" + status + help
	}

	if m.mode == detailView && m.detailRepo != nil {
		title := detailTitleStyle.Render(fmt.Sprintf(" %s [%s]", m.detailRepo.Name, m.detailRepo.Branch))

		totalWidth := m.width
		if totalWidth < 80 {
			totalWidth = 80
		}
		leftWidth := (totalWidth * 60) / 100
		rightWidth := (totalWidth * 40) / 100

		focusedBorder := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("205")).
			Padding(0, 1)
		normalBorder := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("241")).
			Padding(0, 1)

		statusTitle := "Status"
		if m.detailFocus == paneStatus {
			statusTitle = "● " + statusTitle
		}
		statusStyle := normalBorder.Width(leftWidth - 4)
		if m.detailFocus == paneStatus {
			statusStyle = focusedBorder.Width(leftWidth - 4)
		}

		statusHeight := (m.height - 12) / 2
		if statusHeight < 5 {
			statusHeight = 5
		}
		m.viewport.Width = leftWidth - 6
		m.viewport.Height = statusHeight
		statusContent := m.viewport.View()
		statusPane := statusStyle.Height(statusHeight + 2).Render(branchStyle.Render(statusTitle) + "\n" + statusContent)

		branchTitle := "Branches"
		if m.detailFocus == paneBranches {
			branchTitle = "● " + branchTitle
		}
		branchPaneStyle := normalBorder.Width(rightWidth - 4)
		if m.detailFocus == paneBranches {
			branchPaneStyle = focusedBorder.Width(rightWidth - 4)
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

				indicator := ""
				if branch.IsLocal && branch.IsRemote {
					indicator = " ↕"
				} else if branch.IsLocal && !branch.IsRemote {
					indicator = " ⚠"
					style = style.Foreground(lipgloss.Color("214"))
				} else if !branch.IsLocal && branch.IsRemote {
					indicator = " ☁"
					style = style.Foreground(lipgloss.Color("39"))
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
		branchPane := branchPaneStyle.Height(statusHeight + 2).Render(lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Render(branchTitle) + "\n" + branchList.String())

		topRow := lipgloss.JoinHorizontal(lipgloss.Top, statusPane, branchPane)

		cmdTitle := "Command"
		if m.detailFocus == paneCommand {
			cmdTitle = "● " + cmdTitle
		}
		cmdStyle := normalBorder.Width(totalWidth - 4)
		if m.detailFocus == paneCommand {
			cmdStyle = focusedBorder.Width(totalWidth - 4)
		}

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
			if i == 1 {
				style = style.Foreground(lipgloss.Color("196"))
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

	var help, help2 string
	if m.currentGroup != nil {
		// Inside a group - always showing repos
		help = helpStyle.Render("s: lazygit • d: details • o: open web • f: fav • p: pull • P: pull all • g: goto • r: refresh • x: remove")
		help2 = helpStyle.Render("a: add repos • 1: dirty • 2: behind • 0: clear • /: search • m: move • esc: back • q: quit")
	} else if _, isGroup := m.list.SelectedItem().(GroupItem); isGroup {
		// Homepage with a group selected
		help = helpStyle.Render("enter: open group • P: pull group • r: refresh group • e: rename • x: delete group • n: new group • /: search")
		help2 = helpStyle.Render("A: pull behind • ctrl+r: refresh all • c: config • S: settings • q: quit")
	} else {
		// Homepage with a repo selected
		help = helpStyle.Render("s: lazygit • d: details • o: open web • f: fav • p: pull • P: pull favs • g: goto • r/ctrl+r: refresh")
		help2 = helpStyle.Render("A: pull behind • n: new group • m: move repo • /: search • c: config • S: settings • q: quit")
	}

	return m.list.View() + "\n" + status + "\n" + help + "\n" + help2
}
