package routing

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

type Group struct {
	Line                      int
	Name, Type, Selected, URL string
	Members                   []string
	Interval, Tolerance       int
}

func parseGroup(text string) (Group, error) {
	name, rest, ok := strings.Cut(text, "=")
	g := Group{Name: strings.TrimSpace(name), Interval: 600, URL: "https://www.gstatic.com/generate_204"}
	if !ok || !ValidPolicy(g.Name) || strings.EqualFold(g.Name, "proxy") || strings.EqualFold(g.Name, "direct") || strings.EqualFold(g.Name, "reject") || strings.HasPrefix(g.Name, "node:") {
		return g, fmt.Errorf("策略组名称无效")
	}
	fields := strings.Split(rest, ",")
	g.Type = strings.TrimSpace(fields[0])
	if g.Type != "select" && g.Type != "url-test" && g.Type != "fallback" {
		return g, fmt.Errorf("策略组类型支持select、url-test、fallback")
	}
	for _, raw := range fields[1:] {
		raw = strings.TrimSpace(raw)
		key, value, param := strings.Cut(raw, "=")
		if !param {
			action, ok := policy(raw)
			if !ok || action == "reject" {
				return g, fmt.Errorf("策略组成员无效")
			}
			g.Members = append(g.Members, action)
			continue
		}
		switch key {
		case "policy-select-name":
			g.Selected = value
		case "url":
			g.URL = value
		case "interval", "tolerance":
			n, err := strconv.Atoi(value)
			if err != nil || n < 0 || key == "interval" && (n < 1 || n > 86400) || key == "tolerance" && n > 65535 {
				return g, fmt.Errorf("策略组参数无效")
			}
			if key == "interval" {
				g.Interval = n
			} else {
				g.Tolerance = n
			}
		default:
			return g, fmt.Errorf("不支持的策略组参数%s", key)
		}
	}
	u, err := url.Parse(g.URL)
	if len(g.Members) == 0 || len(g.Members) > 200 || err != nil || u.Hostname() == "" || u.User != nil || u.Scheme != "http" && u.Scheme != "https" {
		return g, fmt.Errorf("策略组成员或测试URL无效")
	}
	if g.Selected != "" {
		selected, _ := policy(g.Selected)
		found := false
		for _, v := range g.Members {
			if selected == v {
				found = true
			}
		}
		if !found {
			return g, fmt.Errorf("所选策略不在分组内")
		}
		g.Selected = selected
	}
	return g, nil
}
func (g Group) Text() string {
	text := g.Name + " = " + g.Type + "," + strings.Join(g.Members, ",")
	if g.Selected != "" && g.Type == "select" {
		text += ",policy-select-name=" + g.Selected
	}
	if g.Type == "url-test" || g.Type == "fallback" {
		text += fmt.Sprintf(",url=%s,interval=%d,tolerance=%d", g.URL, g.Interval, g.Tolerance)
	}
	return text
}
