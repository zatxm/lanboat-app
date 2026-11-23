package desktop

import (
	"errors"
	"slices"

	"github.com/zatxm/lanboat-app/internal/profile"
)

func (a *App) SavePolicyGroup(group profile.PolicyGroup) error {
	group.Members = slices.Clone(group.Members)
	a.connectionMu.Lock()
	defer a.connectionMu.Unlock()
	a.mu.Lock()
	defer a.mu.Unlock()

	c := a.config
	c.PolicyGroups = slices.Clone(c.PolicyGroups)
	if group.ID == "" {
		group.ID = profile.ID()
		c.PolicyGroups = append(c.PolicyGroups, group)
		return a.commit(c)
	}
	for i, old := range c.PolicyGroups {
		if old.ID == group.ID {
			group.Source, group.SourceName = old.Source, old.SourceName
			c.PolicyGroups[i] = group
			return a.commit(c)
		}
	}
	return errors.New("策略组不存在")
}

func (a *App) DeletePolicyGroup(id string) error {
	a.connectionMu.Lock()
	defer a.connectionMu.Unlock()
	a.mu.Lock()
	defer a.mu.Unlock()

	c := a.config
	for i, g := range c.PolicyGroups {
		if g.ID == id {
			c.PolicyGroups = slices.Delete(slices.Clone(c.PolicyGroups), i, i+1)
			return a.commit(c)
		}
	}
	return errors.New("策略组不存在")
}
