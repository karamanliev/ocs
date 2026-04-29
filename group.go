package main

import (
	"strings"
)

type groupInfo struct {
	path        string
	worktree    string
	sessionIDs  []string
	collapsed   bool
	filterValue string
}

type groupHeaderItem struct {
	path        string
	worktree    string
	count       int
	collapsed   bool
	filterValue string
}

type groupSeparatorItem struct{}

func (i groupHeaderItem) FilterValue() string {
	return i.filterValue
}

func (i groupSeparatorItem) FilterValue() string {
	return ""
}

type itemRef struct {
	sessionID string
	groupPath string
	header    bool
}

func buildGroups(sessions []Session, collapsedByPath map[string]bool) []groupInfo {
	groups := make([]groupInfo, 0)
	groupIndex := make(map[string]int)
	sessionByID := make(map[string]Session, len(sessions))

	for _, s := range sessions {
		sessionByID[s.ID] = s
		ix, ok := groupIndex[s.Directory]
		if !ok {
			ix = len(groups)
			groupIndex[s.Directory] = ix
			groups = append(groups, groupInfo{
				path:      s.Directory,
				worktree:  s.Worktree,
				collapsed: collapsedByPath[s.Directory],
			})
		}
		groups[ix].sessionIDs = append(groups[ix].sessionIDs, s.ID)
	}

	for ix := range groups {
		var b strings.Builder
		b.WriteString(groups[ix].path)
		for _, id := range groups[ix].sessionIDs {
			s := sessionByID[id]
			b.WriteByte(' ')
			b.WriteString(s.Title)
			b.WriteByte(' ')
			b.WriteString(s.Directory)
		}
		groups[ix].filterValue = b.String()
	}

	return groups
}

func sessionFromItem(item any) (Session, bool) {
	sessItem, ok := item.(sessionItem)
	if !ok {
		return Session{}, false
	}
	return sessItem.session, true
}

func groupPathFromItem(item any) (string, bool) {
	switch v := item.(type) {
	case groupHeaderItem:
		return v.path, true
	case sessionItem:
		if v.groupPath == "" {
			return "", false
		}
		return v.groupPath, true
	default:
		return "", false
	}
}

func itemRefFromItem(item any) itemRef {
	switch v := item.(type) {
	case groupHeaderItem:
		return itemRef{groupPath: v.path, header: true}
	case sessionItem:
		return itemRef{sessionID: v.session.ID, groupPath: v.groupPath}
	default:
		return itemRef{}
	}
}

func itemMatchesRef(item any, ref itemRef) bool {
	if _, ok := item.(groupSeparatorItem); ok {
		return false
	}
	if ref.header {
		header, ok := item.(groupHeaderItem)
		return ok && header.path == ref.groupPath
	}
	if ref.sessionID == "" {
		return false
	}
	sessItem, ok := item.(sessionItem)
	return ok && sessItem.session.ID == ref.sessionID
}
