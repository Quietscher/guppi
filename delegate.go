package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/bubbles/list"
)

// repoDelegate is a custom delegate that renders both Repo and GroupItem
type repoDelegate struct {
	list.DefaultDelegate
	favorites  map[string]bool   // maps are reference types, so this shares data with model
	repoGroups map[string]string // repo path -> group name for display when filtering
}

func newRepoDelegate(favorites map[string]bool) repoDelegate {
	d := repoDelegate{
		DefaultDelegate: list.NewDefaultDelegate(),
		favorites:       favorites,
		repoGroups:      make(map[string]string),
	}
	d.ShowDescription = true
	return d
}

func (d repoDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	isSelected := index == m.Index()
	itemStyles := d.Styles

	// Handle GroupItem
	if group, ok := item.(GroupItem); ok {
		title := "📁 " + group.Name
		var descParts []string
		descParts = append(descParts, fmt.Sprintf("%d repos", group.RepoCount))
		if group.DirtyCount > 0 {
			descParts = append(descParts, statusDirtyStyle.Render(fmt.Sprintf("%d dirty", group.DirtyCount)))
		}
		if group.BehindCount > 0 {
			descParts = append(descParts, statusDirtyStyle.Render(fmt.Sprintf("%d behind", group.BehindCount)))
		}
		desc := strings.Join(descParts, " • ")

		if isSelected {
			title = itemStyles.SelectedTitle.Render(title)
			desc = itemStyles.SelectedDesc.Render(desc)
		} else {
			title = itemStyles.NormalTitle.Render(title)
			desc = itemStyles.NormalDesc.Render(desc)
		}
		fmt.Fprintf(w, "%s\n%s", title, desc)
		return
	}

	// Handle Repo
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

	// Show group prefix if we have one (used when filtering on homepage)
	if groupName, hasGroup := d.repoGroups[repo.Path]; hasGroup && groupName != "" {
		title = "[" + groupName + "] " + title
	}

	if repo.Branch != "" {
		title += " " + branchStyle.Render("["+repo.Branch+"]")
	}

	desc := repo.Description()

	if isSelected {
		title = itemStyles.SelectedTitle.Render(title)
		desc = itemStyles.SelectedDesc.Render(desc)
	} else {
		title = itemStyles.NormalTitle.Render(title)
		desc = itemStyles.NormalDesc.Render(desc)
	}

	fmt.Fprintf(w, "%s\n%s", title, desc)
}
