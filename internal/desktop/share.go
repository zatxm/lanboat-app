package desktop

import (
	"errors"
	"github.com/zatxm/lanboat-app/internal/profile"
)

func (a *App) ShareLink(kind, id string) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if kind == "subscription" {
		for _, s := range a.config.Subscriptions {
			if s.ID == id {
				return s.URL, nil
			}
		}
		return "", errors.New("订阅不存在")
	}
	for _, n := range a.config.Nodes {
		if n.ID == id {
			return profile.ShareURI(n), nil
		}
	}
	return "", errors.New("节点不存在")
}
