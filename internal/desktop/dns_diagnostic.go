package desktop

import (
	"context"
	"github.com/zatxm/lanboat-app/internal/profile"
	"time"
)

func (a *App) DNSDiagnostics() (string, error) {
	a.mu.Lock()
	c := a.config
	a.mu.Unlock()
	r, err := profile.EffectiveRouting(c)
	if err != nil {
		return "", err
	}
	c.Routing = &r
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	return a.engine.DNSDiagnostics(ctx, c)
}
