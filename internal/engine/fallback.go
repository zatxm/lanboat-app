package engine

import (
	"bytes"
	"context"
	"fmt"
	"github.com/goccy/go-json"
	"io"
	"net/http"
	"net/url"
	"time"
)

type fallbackGroup struct {
	tag      string
	members  []fallbackMember
	url      string
	interval int
}

type fallbackMember struct {
	tag  string
	port int
}

func monitorFallback(control, secret string, done <-chan struct{}, group fallbackGroup) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		select {
		case <-done:
			cancel()
		case <-ctx.Done():
		}
	}()
	client := &http.Client{Timeout: 6 * time.Second, Transport: &http.Transport{Proxy: nil}}
	defer client.CloseIdleConnections()
	probes := make([]*http.Client, len(group.members))
	for i, member := range group.members {
		proxy, _ := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", member.port))
		probes[i] = &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{Proxy: http.ProxyURL(proxy), DisableKeepAlives: true}}
		defer probes[i].CloseIdleConnections()
	}
	chosen := ""
	for {
		for i, member := range group.members {
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, group.url, nil)
			if err != nil {
				continue
			}
			response, err := probes[i].Do(req)
			if err != nil {
				continue
			}
			io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
			response.Body.Close()
			if response.StatusCode < 200 || response.StatusCode >= 400 {
				continue
			}
			if member.tag != chosen {
				body, _ := json.Marshal(map[string]string{"name": member.tag})
				req, err = http.NewRequestWithContext(ctx, http.MethodPut, "http://"+control+"/proxies/"+url.PathEscape(group.tag), bytes.NewReader(body))
				if err != nil {
					continue
				}
				req.Header.Set("Authorization", "Bearer "+secret)
				req.Header.Set("Content-Type", "application/json")
				res, err := client.Do(req)
				if err != nil {
					continue
				}
				io.Copy(io.Discard, io.LimitReader(res.Body, 4096))
				res.Body.Close()
				if res.StatusCode < 200 || res.StatusCode >= 300 {
					continue
				}
				chosen = member.tag
			}
			break
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Duration(group.interval) * time.Second):
		}
	}
}
