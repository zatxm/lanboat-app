package native

import (
	"context"
	"net"
	"net/netip"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type flagEntry struct {
	code  string
	until time.Time
}

func countryFlag(code string) string {
	if len(code) != 2 {
		return "🌐"
	}
	return string([]rune{0x1f1e6 + rune(code[0]-'a'), 0x1f1e6 + rune(code[1]-'a')})
}

func (u *UI) refreshFlags(visible bool) {
	if !visible || !u.flagsEnabled || u.flagsBusy {
		u.flagDeadline.Store(0)
		return
	}
	now := time.Now()
	var next time.Time
	var pending []string
	for _, host := range u.flagHosts {
		until := u.flags[host].until
		if !until.After(now) {
			pending = append(pending, host)
		} else if next.IsZero() || until.Before(next) {
			next = until
		}
	}
	if len(pending) > 0 || next.IsZero() {
		u.flagDeadline.Store(0)
	} else {
		u.flagDeadline.Store(next.UnixNano())
	}
	u.loadFlags(pending)
}

func (u *UI) loadFlags(hosts []string) {
	if !u.flagsEnabled || u.flagsBusy || len(hosts) == 0 {
		return
	}
	u.flagsBusy = true
	go func() {
		current, index, err := u.backend.CountryIndex(u.flagGeneration, u.flagIndex)
		flags := map[string]flagEntry{}
		results := make([]flagEntry, len(hosts))
		var next atomic.Int64
		var workers sync.WaitGroup
		for range min(4, len(hosts)) {
			workers.Go(func() {
				for {
					i := int(next.Add(1)) - 1
					if i >= len(hosts) {
						return
					}
					host := hosts[i]
					select {
					case <-u.done:
						return
					default:
					}
					code := ""
					if err == nil {
						ip, e := netip.ParseAddr(host)
						if e == nil {
							code = index.Country(ip)
						} else {
							ctx, cancel := context.WithTimeout(context.Background(), time.Second)
							ips, _ := net.DefaultResolver.LookupNetIP(ctx, "ip", strings.TrimSuffix(host, "."))
							cancel()
							for _, ip := range ips {
								if code = index.Country(ip); code != "" {
									break
								}
							}
						}
					}
					ttl := time.Hour
					if code == "" {
						ttl = time.Minute
					}
					results[i] = flagEntry{code, time.Now().Add(ttl)}
				}
			})
		}
		workers.Wait()
		for i, host := range hosts {
			flags[host] = results[i]
		}
		u.post(update{background: true, apply: func() {
			if current != u.flagGeneration || len(u.flags) > 2048 {
				clear(u.flags)
			}
			u.flagIndex, u.flagGeneration, u.flagsBusy = index, current, false
			for k, val := range flags {
				u.flags[k] = val
			}
		}})
	}()
}
