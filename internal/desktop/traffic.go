package desktop

import (
	"github.com/zatxm/lanboat-app/internal/traffic"
)

func (a *App) NetworkLogs(kind string, page int) (traffic.Page, error) {
	return a.networkLogs.Page(kind, page)
}
