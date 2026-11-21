package profile

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/zatxm/lanboat-app/internal/routing"
	"github.com/zatxm/lanboat-app/internal/ruledata"
	"golang.org/x/net/idna"
)

const MaxImport = 2 << 20
const MaxNodes = 100000
const MaxSubscription = 32 << 20
const MaxConfig = 128 << 20
const MaxNodeNameBytes = 160

var Methods = []string{"none", "table", "rc4", "rc4-md5", "rc4-md5-6", "salsa20", "chacha20", "chacha20-ietf", "aes-256-cfb", "aes-192-cfb", "aes-128-cfb", "aes-256-cfb1", "aes-192-cfb1", "aes-128-cfb1", "aes-256-cfb8", "aes-192-cfb8", "aes-128-cfb8", "aes-256-ctr", "aes-192-ctr", "aes-128-ctr", "bf-cfb", "camellia-128-cfb", "camellia-192-cfb", "camellia-256-cfb", "cast5-cfb", "des-cfb", "idea-cfb", "rc2-cfb", "seed-cfb", "aes-256-gcm", "aes-192-gcm", "aes-128-gcm", "chacha20-poly1305", "chacha20-ietf-poly1305", "xchacha20-ietf-poly1305", "2022-blake3-aes-256-gcm", "2022-blake3-aes-128-gcm", "2022-blake3-chacha20-poly1305"}

type Node struct {
	DialProxy  *UpstreamProxy `json:"-"`
	TCP        *TCPSettings   `json:"-"`
	Type       string         `json:"type,omitempty"`
	Options    map[string]any `json:"options,omitempty"`
	ID         string         `json:"id"`
	Name       string         `json:"name"`
	Server     string         `json:"server"`
	Port       int            `json:"port"`
	Method     string         `json:"method"`
	Password   string         `json:"password"`
	Plugin     string         `json:"plugin"`
	PluginOpts string         `json:"pluginOpts"`
	Source     string         `json:"source"`
}

type Rule struct {
	ID        string `json:"id"`
	Type      string `json:"type"`
	Value     string `json:"value"`
	Action    string `json:"action"`
	NoResolve bool   `json:"noResolve,omitempty"`
	Disabled  bool   `json:"disabled,omitempty"`
}

type Subscription struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	URL       string `json:"url"`
	UpdatedAt string `json:"updatedAt"`
	UDP       string `json:"udp,omitempty"`
	Insecure  string `json:"insecure,omitempty"`
	FastOpen  string `json:"fastOpen,omitempty"`
	MaxNodes  int    `json:"maxNodes,omitempty"`
	UserAgent string `json:"userAgent,omitempty"`
	Password  string `json:"password,omitempty"`
	Filter    string `json:"filter,omitempty"`
	KeepRules bool   `json:"keepRules,omitempty"`
}

type Settings struct {
	UDP              UDPSettings   `json:"udp,omitzero"`
	TCP              TCPSettings   `json:"tcp,omitzero"`
	Proxy            ProxySettings `json:"proxy,omitzero"`
	DNS              *DNSSettings  `json:"dns,omitempty"`
	Language         string        `json:"language,omitempty"`
	AutoSelectGroups []string      `json:"autoSelectGroups,omitempty"`
	Appearance       string        `json:"appearance,omitempty"`
	// Legacy controls are decoded for migration, then discarded.
	LegacyAutoSelect bool     `json:"autoSelect,omitempty"`
	TestMethod       string   `json:"testMethod,omitempty"`
	TestURL          string   `json:"testURL,omitempty"`
	TestTimeout      int      `json:"testTimeout,omitempty"`
	TestConcurrency  int      `json:"testConcurrency,omitempty"`
	SpeedURL         string   `json:"speedURL,omitempty"`
	SpeedURLs        []string `json:"speedURLs,omitempty"`
	TestURLs         []string `json:"testURLs,omitempty"`
	ProxyLog         bool     `json:"proxyLog,omitempty"`
	DNSLog           bool     `json:"dnsLog,omitempty"`
	LogRetention     int      `json:"logRetention,omitempty"`
	RuleAutoUpdate   *bool    `json:"ruleAutoUpdate,omitempty"`
	RuleUpdateDays   int      `json:"ruleUpdateDays,omitempty"`
	RuleRegion       string   `json:"ruleRegion,omitempty"`
	UpdateOnStart    bool     `json:"updateOnStart,omitempty"`
	Port             int      `json:"port"`
	Mode             string   `json:"mode"`
	SystemProxy      bool     `json:"systemProxy"`
}

type Config struct {
	PolicyPorts     map[string]int  `json:"-"`
	DNSOutboundPort int             `json:"-"`
	ForceProxyPort  int             `json:"-"`
	ForceDirectPort int             `json:"-"`
	Files           []ConfigFile    `json:"files,omitempty"`
	ActiveFile      string          `json:"activeFile,omitempty"`
	Modules         []Module        `json:"modules,omitempty"` // Legacy backup data, no longer executed.
	Routing         *routing.Parsed `json:"-"`
	Version         int             `json:"version"`
	Nodes           []Node          `json:"nodes"`
	Selected        string          `json:"selected"`
	Rules           []Rule          `json:"rules"`
	Subscriptions   []Subscription  `json:"subscriptions"`
	RuleOverlays    []RuleOverlay   `json:"ruleOverlays,omitempty"`
	Settings        Settings        `json:"settings"`
}

func (s Settings) AutoSelectEnabled(group string) bool {
	return slices.Contains(s.AutoSelectGroups, group)
}

// An orphaned subscription node appears under local nodes.
func (c Config) NodeGroup(n Node) string {
	if n.Source != "" && c.HasNodeGroup(n.Source) {
		return n.Source
	}
	return ""
}

func (c Config) HasNodeGroup(group string) bool {
	return group == "" || slices.ContainsFunc(c.Subscriptions, func(sub Subscription) bool { return sub.ID == group })
}

func (c Config) SelectedNodeGroup() string {
	for _, n := range c.Nodes {
		if n.ID == c.Selected {
			return c.NodeGroup(n)
		}
	}
	return ""
}

func ID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

func Default() Config {
	return Config{Version: 1, Nodes: []Node{}, Subscriptions: []Subscription{}, Rules: []Rule{}, Settings: Settings{Port: 1080, Mode: "rule"}}
}

func (n Node) Validate() error {
	if strings.TrimSpace(n.Name) == "" || len(n.Name) > MaxNodeNameBytes {
		return errors.New("节点名称不能为空，且不能超过 160 字节")
	}
	if len(n.Server) > 253 || strings.TrimSpace(n.Server) != n.Server || strings.ContainsAny(n.Server, "/\\@?# \t\r\n") || n.Server == "" {
		return errors.New("服务器地址无效，请填写域名或 IP")
	}
	if !ValidHost(n.Server) {
		return errors.New("服务器域名或 IP 地址无效")
	}
	if n.Port < 1 || n.Port > 65535 {
		return errors.New("服务器端口必须为 1–65535")
	}
	if n.Kind() != "ss" && n.Kind() != "ssr" {
		return n.validateProtocol()
	}
	if n.Kind() == "ssr" && n.Method == "" {
		n.Method = n.TextOption("cipher")
	}
	found := false
	for _, method := range Methods {
		if n.Method == method {
			found = true
			break
		}
	}
	if !found {
		return errors.New("不支持的 Shadowsocks 加密方式")
	}
	if n.Password == "" || len(n.Password) > 4096 {
		return errors.New("密码不能为空或超过 4096 字节")
	}
	if strings.HasPrefix(n.Method, "2022-") {
		keyLen := 32
		if n.Method == "2022-blake3-aes-128-gcm" {
			keyLen = 16
		}
		for _, part := range strings.Split(n.Password, ":") {
			key, err := base64.StdEncoding.DecodeString(part)
			if err != nil || len(key) != keyLen {
				return fmt.Errorf("SS2022 密钥必须为 Base64 编码的 %d 字节密钥", keyLen)
			}
		}
	}
	return n.validateShadowsocks()
}

func (s Settings) RulesAutoUpdate() bool { return s.RuleAutoUpdate == nil || *s.RuleAutoUpdate }
func (s Settings) RulesUpdateDays() int {
	if s.RuleUpdateDays == 0 {
		return 1
	}
	return s.RuleUpdateDays
}

func (s Settings) Validate() error {
	if s.RuleUpdateDays < 0 || s.RuleUpdateDays > 7 || !ruledata.ValidRegion(s.RuleRegion) {
		return errors.New("规则更新间隔或国家和地区无效")
	}
	if err := s.UDP.Validate(); err != nil {
		return err
	}
	if err := s.TCP.Validate(); err != nil {
		return err
	}
	if s.DNS != nil {
		if err := s.DNS.Validate(); err != nil {
			return err
		}
	}
	if s.Language != "" && s.Language != "zh" && s.Language != "en" {
		return errors.New("语言选项无效")
	}
	if s.Appearance != "" && s.Appearance != "light" && s.Appearance != "dark" {
		return errors.New("外观选项无效")
	}
	if s.TestTimeout < 0 || s.TestTimeout > 300 {
		return errors.New("测试超时需在1–300秒之间")
	}
	if s.LogRetention < 0 || s.LogRetention > 365 {
		return errors.New("日志保留天数需在 0–365 之间")
	}
	if s.TestConcurrency < 0 || s.TestConcurrency > 20 {
		return errors.New("并发数量需在1–20之间")
	}
	for _, target := range append(append([]string{s.TestURL, s.SpeedURL}, s.TestURLs...), s.SpeedURLs...) {
		if target == "" {
			continue
		}
		u, err := url.Parse(target)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil {
			return errors.New("测试URL需为有效的HTTP/HTTPS地址")
		}
	}
	if s.Port < 1024 || s.Port > 65535 {
		return errors.New("本地端口必须为 1024–65535")
	}
	if err := s.Proxy.Validate(s.Port); err != nil {
		return err
	}
	if s.Mode != "rule" && s.Mode != "global" && s.Mode != "tun" && s.Mode != "direct" {
		return errors.New("无效的路由模式")
	}
	return nil
}

func (r Rule) Validate() error {
	if (r.Type == "domain" || r.Type == "domain_suffix") && (!ValidHost(strings.TrimPrefix(r.Value, ".")) || strings.ContainsAny(r.Value, "/:@?# \t\r\n")) {
		return errors.New("规则需要域名，不包含协议或路径")
	}
	r.Disabled = false
	_, err := CustomRouting([]Rule{r})
	return err
}

func ValidHost(host string) bool {
	if _, err := netip.ParseAddr(host); err == nil {
		return true
	}
	ascii, err := idna.Lookup.ToASCII(strings.TrimSuffix(host, "."))
	if err != nil || ascii == "" || len(ascii) > 253 {
		return false
	}
	for _, label := range strings.Split(ascii, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, char := range label {
			if !(char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' || char == '-') {
				return false
			}
		}
	}
	return true
}

func DecodeBase64(s string) ([]byte, error) {
	for _, enc := range []*base64.Encoding{base64.RawURLEncoding, base64.URLEncoding, base64.RawStdEncoding, base64.StdEncoding} {
		if b, err := enc.DecodeString(s); err == nil {
			return b, nil
		}
	}
	return nil, errors.New("Base64 内容无效")
}

func parseSS(raw string) (Node, error) {
	n := Node{ID: ID()}
	if !strings.HasPrefix(raw, "ss://") {
		return n, errors.New("仅支持 ss:// Shadowsocks 链接")
	}
	body := strings.TrimPrefix(raw, "ss://")
	if i := strings.IndexByte(body, '#'); i >= 0 {
		name, err := url.PathUnescape(body[i+1:])
		if err != nil {
			return n, errors.New("节点名称编码无效")
		}
		n.Name = name
		body = body[:i]
	}
	query := ""
	if i := strings.IndexByte(body, '?'); i >= 0 {
		query = body[i+1:]
		body = body[:i]
	}
	body = strings.TrimSuffix(body, "/")
	legacy := !strings.Contains(body, "@")
	if legacy {
		decoded, err := DecodeBase64(body)
		if err != nil {
			return n, err
		}
		body = string(decoded)
	}
	i := strings.LastIndexByte(body, '@')
	if i < 1 {
		return n, errors.New("节点链接缺少认证信息")
	}
	credentials, addr := body[:i], body[i+1:]
	if !strings.Contains(credentials, ":") {
		decoded, err := DecodeBase64(credentials)
		if err != nil {
			return n, err
		}
		credentials = string(decoded)
	} else if !legacy {
		decoded, err := url.PathUnescape(credentials)
		if err != nil {
			return n, errors.New("认证信息编码无效")
		}
		credentials = decoded
	}
	n.Method, n.Password, _ = strings.Cut(credentials, ":")
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return n, errors.New("服务器地址或端口无效；IPv6 地址需要方括号")
	}
	n.Server = host
	n.Port, err = strconv.Atoi(port)
	if err != nil {
		return n, errors.New("服务器端口无效")
	}
	values, err := url.ParseQuery(query)
	if err != nil {
		return n, errors.New("链接参数无效")
	}
	for key := range values {
		if key != "plugin" {
			return n, fmt.Errorf("不支持的链接参数：%s", key)
		}
	}
	if plugin := values.Get("plugin"); plugin != "" {
		n.Plugin, n.PluginOpts, _ = strings.Cut(plugin, ";")
	}
	if n.Name == "" {
		n.Name = net.JoinHostPort(host, port)
	}
	return n, n.Validate()
}

func URI(n Node) string {
	if n.Kind() != "ss" || len(n.Options) > 0 {
		return protocolURI(n)
	}
	user := base64.RawURLEncoding.EncodeToString([]byte(n.Method + ":" + n.Password))
	if strings.HasPrefix(n.Method, "2022-") {
		user = url.PathEscape(n.Method) + ":" + url.PathEscape(n.Password)
	}
	query := ""
	if n.Plugin != "" {
		query = "/?" + url.Values{"plugin": {n.Plugin + ";" + n.PluginOpts}}.Encode()
	}
	return "ss://" + user + "@" + net.JoinHostPort(n.Server, strconv.Itoa(n.Port)) + query + "#" + url.PathEscape(n.Name)
}

func Load(path string) (Config, error) {
	c := Default()
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return c, nil
	}
	if err != nil {
		return c, err
	}
	if len(b) > MaxConfig {
		return c, errors.New("配置文件过大")
	}
	if err := json.Unmarshal(b, &c); err != nil {
		return c, fmt.Errorf("配置损坏，已保留原文件：%w", err)
	}
	err = c.ValidateSnapshot()
	return c, err
}
func (c *Config) ValidateSnapshot() error {
	if c.Version != 1 {
		return errors.New("配置版本不受支持")
	}
	if len(c.Nodes) > MaxNodes || len(c.Rules) > 500 || len(c.Subscriptions) > 30 {
		return errors.New("配置条目超过限制")
	}
	ids := map[string]bool{}
	for _, n := range c.Nodes {
		if n.ID == "" || ids[n.ID] {
			return errors.New("配置中节点标识无效或重复")
		}
		ids[n.ID] = true
	}
	if c.Selected != "" && !ids[c.Selected] {
		return errors.New("配置中选定的节点不存在")
	}
	subs := map[string]bool{}
	for _, s := range c.Subscriptions {
		if s.ID == "" || subs[s.ID] || s.Validate() != nil {
			return errors.New("配置中的订阅无效")
		}
		subs[s.ID] = true
	}
	var groups []string
	for _, id := range c.Settings.AutoSelectGroups {
		if (id == "" || subs[id]) && !slices.Contains(groups, id) {
			groups = append(groups, id)
		}
	}
	if c.Settings.LegacyAutoSelect && len(c.Settings.AutoSelectGroups) == 0 {
		groups = []string{c.SelectedNodeGroup()}
	}
	c.Settings.AutoSelectGroups = groups
	c.Settings.LegacyAutoSelect = false
	if d := c.Settings.DNS; d != nil && d.ProxyRoute == "node" && !ids[d.ProxyNode] {
		return errors.New("DNS出站节点不存在")
	}
	c.Settings.TestMethod = ""
	c.Settings.SpeedURLs = nil
	c.Settings.TestURLs = nil
	// Normalize limits accepted by earlier releases without rewriting the profile on disk.
	if d := c.Settings.DNS; d != nil {
		if d.Timeout > 30 && d.Timeout <= 60 {
			d.Timeout = 30
		}
		if d.Capacity >= 64 && d.Capacity < 1024 {
			d.Capacity = 1024
		}
	}
	if err := c.Settings.Validate(); err != nil {
		return err
	}
	for _, n := range c.Nodes {
		if err := n.Validate(); err != nil {
			return err
		}
	}
	for _, r := range c.Rules {
		if err := r.Validate(); err != nil {
			return err
		}
	}
	if err := ValidateRuleOverlays(c.RuleOverlays); err != nil {
		return err
	}
	if err := ValidateFiles(*c); err != nil {
		return err
	}
	return nil
}

func Save(path string, c Config) error {
	b, err := json.Marshal(c)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	if len(b) > MaxConfig {
		return errors.New("配置内容不能超过 128 MiB")
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".config-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(b)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(f.Name(), path)
	}
	return err
}
