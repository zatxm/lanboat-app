package profile

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Give imported group names a namespace so names such as PROXY remain unambiguous.
func clashGroups(doc map[string]any, lines *[]string) (map[string]string, error) {
	names := map[string]string{}
	groups := mapList(doc, "proxy-groups")
	for _, item := range groups {
		g, ok := item.(map[string]any)
		if !ok {
			return nil, errors.New("订阅代理分组格式无效")
		}
		name := mapString(g, "name")
		if name == "" || names[name] != "" {
			return nil, errors.New("订阅代理分组名称无效或重复")
		}
		names[name] = "group:" + name
	}
	for _, item := range groups {
		g := item.(map[string]any)
		name := mapString(g, "name")
		typ := mapString(g, "type")
		if typ != "select" && typ != "url-test" && typ != "fallback" {
			return nil, fmt.Errorf("暂不支持代理分组类型 %s", typ)
		}
		if err := unsupportedKeys(g, "name", "type", "proxies", "url", "interval", "tolerance", "default"); err != nil {
			return nil, err
		}
		members := mapList(g, "proxies")
		if len(members) == 0 {
			return nil, fmt.Errorf("代理分组 %s 没有节点", name)
		}
		values := []string{names[name] + " = " + typ}
		for _, v := range members {
			member := fmt.Sprint(v)
			if mapped := names[member]; mapped != "" {
				member = mapped
			}
			values = append(values, member)
		}
		if typ == "select" && mapString(g, "default") != "" {
			selected := mapString(g, "default")
			if mapped := names[selected]; mapped != "" {
				selected = mapped
			}
			values = append(values, "policy-select-name="+selected)
		}
		if typ == "url-test" || typ == "fallback" {
			if v := mapString(g, "url"); v != "" {
				values = append(values, "url="+v)
			}
			for _, key := range []string{"interval", "tolerance"} {
				if v := mapString(g, key); v != "" {
					if _, err := strconv.Atoi(v); err != nil {
						return nil, fmt.Errorf("分组 %s 参数 %s 无效", name, key)
					}
					values = append(values, key+"="+v)
				}
			}
		}
		*lines = append(*lines, "@GROUP "+strings.Join(values, ","))
	}
	return names, nil
}
