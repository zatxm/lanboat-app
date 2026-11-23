package profile

import (
	"errors"
	"github.com/zatxm/lanboat-app/internal/routing"
	"github.com/zatxm/lanboat-app/internal/ruledata"
	"strings"
	"time"
)

type ConfigFile struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Content   string `json:"content"`
	UpdatedAt string `json:"updatedAt"`
	Source    string `json:"source"`
}

// RuleOverlay belongs to one subscription or imported node batch, independent of the selected configuration.
type RuleOverlay struct {
	ID             string   `json:"id"`
	SubscriptionID string   `json:"subscriptionId,omitempty"`
	Name           string   `json:"name"`
	Content        string   `json:"content"`
	NodeIDs        []string `json:"nodeIds,omitempty"`
}

type Module struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Content string `json:"content"`
	Enabled bool   `json:"enabled"`
}

func DefaultFile() ConfigFile {
	return ConfigFile{ID: "default", Name: "default.conf", Content: routing.DefaultConfig, UpdatedAt: time.Now().Format(time.RFC3339), Source: "内置默认配置"}
}
func EnsureFiles(c *Config) bool {
	if len(c.Files) == 0 {
		c.Files = []ConfigFile{DefaultFile()}
		c.ActiveFile = "default"
		return true
	}
	for i, f := range c.Files {
		if f.ID == "default" && f.Content != routing.DefaultConfig {
			c.Files[i] = DefaultFile()
			return true
		}
	}
	return false
}
func ValidateFiles(c Config) error {
	if len(c.Files) > 30 {
		return errors.New("最多保存 30 个配置文件")
	}
	ids := map[string]bool{}
	names := map[string]bool{}
	found := len(c.Files) == 0 && c.ActiveFile == ""
	for _, f := range c.Files {
		if f.ID == "" || ids[f.ID] || !ValidFileName(f.Name) || names[f.Name] || (f.ID == "default" && f.Name != "default.conf") || (f.ID != "default" && f.Name == "default.conf") {
			return errors.New("配置文件标识或名称无效")
		}
		ids[f.ID] = true
		names[f.Name] = true
		if f.ID == c.ActiveFile {
			found = true
		}
		if _, err := routing.Parse(f.Content); err != nil {
			return err
		}
	}
	if !found {
		return errors.New("正在使用的配置不存在")
	}

	return nil
}
func EffectiveRouting(c Config) (routing.Parsed, error) {
	var p routing.Parsed
	var err error
	for _, f := range c.Files {
		if f.ID == c.ActiveFile {
			p, err = routing.Parse(f.Content)
			break
		}
	}
	if err != nil {
		return p, err
	}

	region := ruledata.Region(c.Settings.RuleRegion)
	filtered := p.Rules[:0]
	for _, r := range p.Rules {
		if r.Value == "lanboat://geoip/region" || r.Value == "lanboat://geosite/region" {
			if region == "" || strings.HasPrefix(r.Value, "lanboat://geosite/") && region != "cn" {
				continue
			}
			r.Value = strings.TrimSuffix(r.Value, "region") + region
		}
		filtered = append(filtered, r)
	}
	p.Rules = filtered
	if err := applyNodeRuleOverlay(&p, c); err != nil {
		return p, err
	}
	c.appendUsedPolicyGroups(&p)
	return p, nil
}

func ValidFileName(name string) bool {
	return name != "." && name != ".." && strings.TrimSpace(name) != "" && len(name) <= 160 && !strings.ContainsAny(name, "/\\\n\r\x00")
}
