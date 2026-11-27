package desktop

import (
	"context"
	"net"
	"net/netip"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/zatxm/lanboat-app/internal/profile"
	"github.com/zatxm/lanboat-app/internal/ruledata"
)

func (a *App) ResolveNodeFlag(host string) string {
	nodes := [...]profile.Node{{Server: host}}
	a.fillNodeFlags(nodes[:])
	return nodes[0].Flag
}

// Fill flags during an explicit save or import, within one DNS time budget.
func (a *App) fillNodeFlags(groups ...[]profile.Node) {
	var hosts map[string][]*profile.Node
	for _, nodes := range groups {
		for i := range nodes {
			n := &nodes[i]
			n.FillFlagFromName()
			if n.Flag != "" {
				continue
			}
			if ip, err := netip.ParseAddr(n.Server); err == nil {
				ip = ip.Unmap()
				if !ip.IsGlobalUnicast() || ip.IsPrivate() {
					continue
				}
			} else if !profile.ValidHost(n.Server) {
				continue
			}
			if hosts == nil {
				hosts = make(map[string][]*profile.Node)
			}
			hosts[n.Server] = append(hosts[n.Server], n)
		}
	}
	if len(hosts) == 0 {
		return
	}
	index, err := a.CountryIndex()
	if err != nil {
		return
	}
	set := func(host string, code string) {
		flag := profile.CountryFlag(code)
		for _, node := range hosts[host] {
			node.Flag = flag
		}
	}
	var domains []string
	for host := range hosts {
		if addr, err := netip.ParseAddr(host); err == nil {
			set(host, index.Country(addr))
		} else {
			domains = append(domains, host)
		}
	}
	if len(domains) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	var next atomic.Int64
	var workers sync.WaitGroup
	for range min(4, len(domains)) {
		workers.Go(func() {
			for ctx.Err() == nil {
				i := int(next.Add(1)) - 1
				if i >= len(domains) {
					return
				}
				host := domains[i]
				set(host, lookupHostCountry(ctx, index, host))
			}
		})
	}
	workers.Wait()
}

func lookupHostCountry(ctx context.Context, index ruledata.CountryIndex, host string) string {
	ips, _ := net.DefaultResolver.LookupNetIP(ctx, "ip", strings.TrimSuffix(host, "."))
	for _, ip := range ips {
		if code := index.Country(ip); code != "" {
			return code
		}
	}
	return ""
}
