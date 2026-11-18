package profile

import (
	"errors"
	"net"
	"strings"
)

type ProxySettings struct {
	AllowLAN       bool          `json:"allowLAN,omitempty"`
	Authentication bool          `json:"authentication,omitempty"`
	Username       string        `json:"username,omitempty"`
	Password       string        `json:"password,omitempty"`
	Upstream       UpstreamProxy `json:"upstream,omitzero"`
}

type UpstreamProxy struct {
	Enabled  bool   `json:"enabled,omitempty"`
	Type     string `json:"type,omitempty"`
	Server   string `json:"server,omitempty"`
	Port     int    `json:"port,omitempty"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
}

func (p ProxySettings) ListenHost() string {
	if p.AllowLAN {
		return "0.0.0.0"
	}
	return "127.0.0.1"
}

func proxyCredentials(user, password string, required bool) error {
	if required && (user == "" || password == "") || user == "" && password != "" {
		return errors.New("请填写代理用户和密码")
	}
	if len(user) > 255 || len(password) > 255 || strings.ContainsAny(user, ":\r\n\x00") || strings.ContainsAny(password, "\r\n\x00") {
		return errors.New("代理用户和密码最长255字节，用户不能包含冒号或控制字符")
	}
	return nil
}

func (p ProxySettings) Validate(localPort int) error {
	if p.Authentication {
		if err := proxyCredentials(p.Username, p.Password, true); err != nil {
			return err
		}
	}
	u := p.Upstream
	if !u.Enabled {
		return nil
	}
	if u.Type != "" && u.Type != "socks5" && u.Type != "http" {
		return errors.New("前置代理类型无效")
	}
	if !ValidHost(u.Server) {
		return errors.New("请填写有效的前置代理地址")
	}
	if u.Port < 1 || u.Port > 65535 {
		return errors.New("前置代理端口必须为1–65535")
	}
	if u.Port == localPort && (strings.EqualFold(u.Server, "localhost") || net.ParseIP(u.Server).IsLoopback() || net.ParseIP(u.Server).IsUnspecified()) {
		return errors.New("前置代理不能指向本地代理自身")
	}
	return proxyCredentials(u.Username, u.Password, false)
}
