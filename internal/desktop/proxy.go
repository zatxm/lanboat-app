package desktop

import (
	"context"
	"errors"
	"github.com/goccy/go-json"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

type proxyValue struct {
	Schema  string `json:"schema"`
	Key     string `json:"key"`
	Before  string `json:"before"`
	Applied string `json:"applied"`
}
type SystemProxy struct{ Path string }

func ProxySupported() bool {
	_, err := exec.LookPath("gsettings")
	return err == nil && strings.Contains(strings.ToLower(os.Getenv("XDG_CURRENT_DESKTOP")), "gnome")
}

func gsettings(args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, "gsettings", args...).Output()
	if err != nil {
		return "", errors.New("无法修改 GNOME 系统代理，请检查桌面会话")
	}
	return strings.TrimSpace(string(output)), nil
}

func (p SystemProxy) Enable(port int) error {
	if !ProxySupported() {
		return errors.New("自动系统代理目前支持 GNOME，其他桌面请手动设置本机代理")
	}
	if _, err := os.Stat(p.Path); err == nil {
		return errors.New("上次的系统代理尚未恢复，请先断开并重试")
	}
	values := []proxyValue{
		{Schema: "org.gnome.system.proxy", Key: "use-same-proxy", Applied: "false"},
		{Schema: "org.gnome.system.proxy", Key: "ignore-hosts", Applied: "['localhost', '127.0.0.0/8', '::1']"},
		{Schema: "org.gnome.system.proxy.http", Key: "use-authentication", Applied: "false"},
	}
	for _, protocol := range []string{"http", "https", "socks"} {
		values = append(values, proxyValue{Schema: "org.gnome.system.proxy." + protocol, Key: "host", Applied: "'127.0.0.1'"}, proxyValue{Schema: "org.gnome.system.proxy." + protocol, Key: "port", Applied: strconv.Itoa(port)})
	}
	values = append(values, proxyValue{Schema: "org.gnome.system.proxy", Key: "mode", Applied: "'manual'"})
	for i := range values {
		value, err := gsettings("get", values[i].Schema, values[i].Key)
		if err != nil {
			return err
		}
		values[i].Before = value
		writable, err := gsettings("writable", values[i].Schema, values[i].Key)
		if err != nil || writable != "true" {
			return errors.New("系统代理设置被桌面策略锁定")
		}
	}
	// Save before changing the desktop so the next launch can recover after a crash.
	b, err := json.Marshal(values)
	if err != nil {
		return err
	}
	if err := os.WriteFile(p.Path, b, 0600); err != nil {
		return err
	}
	for _, value := range values {
		if _, err := gsettings("set", value.Schema, value.Key, value.Applied); err != nil {
			_ = p.Restore()
			return err
		}
	}
	return nil
}

func (p SystemProxy) Restore() error {
	b, err := os.ReadFile(p.Path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var values []proxyValue
	if err := json.Unmarshal(b, &values); err != nil {
		return err
	}
	for i := len(values) - 1; i >= 0; i-- {
		value := values[i]
		current, err := gsettings("get", value.Schema, value.Key)
		if err != nil {
			return err
		}
		// Preserve any setting the user changed independently while the app was running.
		if current == value.Applied {
			if _, err := gsettings("set", value.Schema, value.Key, value.Before); err != nil {
				return err
			}
		}
	}
	return os.Remove(p.Path)
}
