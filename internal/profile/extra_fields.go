package profile

func extraNodeFields(n Node) []Field {
	var fields []Field
	switch n.Kind() {
	case "sudoku":
		fields = []Field{secret("key", "密钥", true), choose("aead-method", "加密方式", "chacha20-poly1305", "chacha20-poly1305", "aes-128-gcm", "none"), choose("table-type", "编码", "prefer_entropy", "prefer_entropy", "prefer_ascii", "up_ascii_down_entropy", "up_entropy_down_ascii"), number("padding-min", "最小填充比例", "5"), number("padding-max", "最大填充比例", "15"), placeholder(f("custom-table", "自定义编码表", false), "xpxvvpvv"), placeholder(f("custom-tables", "自定义编码表列表", false), "多个编码表用逗号分隔"), boolean("enable-pure-downlink", "启用纯下行", "true"), choose("multiplex", "单一隧道", "off", "off", "auto", "on"), boolean("http-mask", "HTTP 伪装", "false")}
		if n.TextOption("http-mask") == "true" {
			extra := []Field{choose("http-mask-mode", "方式", "auto", "auto", "legacy", "stream", "poll", "ws")}
			if n.EffectiveOption("http-mask-mode", "auto") != "legacy" {
				extra = append(extra, boolean("http-mask-tls", "TLS（仅隧道模式）", "false"), f("http-mask-host", "Host（仅隧道模式）", false))
			}
			extra = append(extra, f("path-root", "PathRoot", false))
			fields = append(fields, withGroup("HTTP 伪装设置", extra...)...)
		}
	case "naive":
		fields = []Field{f("username", "用户", true), choose("transport", "传输协议", "https", "https", "quic"), {Key: "extra-headers", Title: "附加标头", Kind: "multiline", Secret: true, Placeholder: "每行一个标头，例如 X-Example: value"}, f("sni", "服务器名称", false), {Key: "certificate", Title: "证书（链）", Kind: "multiline", Placeholder: "PEM 证书链或证书文件路径"}, number("insecure-concurrency", "不安全并发", "0"), choose("uot-version", "UDP Over TCP", "0", "0", "1", "2")}
	case "tailscale":
		fields = []Field{secret("auth-key", "密钥", true), f("exit-node", "节点", true), f("control-url", "控制服务 URL", false)}
	case "snell":
		fields = []Field{choose("version", "版本", "1", "1", "2", "3", "4", "5"), choose("obfs-opts.mode", "混淆", "none", "none", "http", "tls")}
		switch n.TextOption("obfs-opts.mode") {
		case "http":
			fields = append(fields, withGroup("混淆设置", f("obfs-opts.host", "Host", false), textDefault("obfs-opts.path", "路径", "/"))...)
		case "tls":
			fields = append(fields, withGroup("混淆设置", f("obfs-opts.host", "Host", false))...)
		}
		fields = append(fields, pluginFields(n)...)
		fields = append(fields, boolean("mux", "多路复用", "false"), placeholder(f("idle-timeout", "空闲超时", false), "30,30,1"), boolean("tfo", "TCP 快速打开", "false"), boolean("udp", "UDP 转发", "false"))
	case "mieru":
		pattern := f("traffic-pattern", "流量特征", false)
		pattern.Placeholder = "Base64"
		fields = []Field{f("username", "用户", false), choose("transport", "传输方式", "tcp", "tcp", "udp"), pattern, boolean("user-hint", "用户提示", "false"), boolean("udp", "UDP 转发", "true")}
	case "brook":
		fields = []Field{choose("network", "传输方式", "none", "none", "ws", "wss")}
		if n.TextOption("network") == "ws" || n.TextOption("network") == "wss" {
			fields = append(fields, withGroup("传输设置", f("host", "Host", false), textDefault("path", "路径", "/"))...)
		}
		fields = append(fields, boolean("udp", "UDP 转发", "true"))
	case "lua":
		fields = []Field{f("username", "用户", false), choose("cipher", "算法", DefaultSSMethod, Methods...)}
		fields = append(fields, tlsNodeFields(n, false, false, false, false)...)
		file := n.LuaFile()
		fields = append(fields, choose("lua-file", "文件", file, "http-backend.lua", "lightsword-backend.lua", "trojan-backend.lua", "custom"))
		if file == "custom" {
			fields = append(fields, withGroup("脚本设置", multilineSecret("script", "Lua 脚本", true))...)
		}
		fields = append(fields, pluginFields(n)...)
		fields = append(fields, boolean("tfo", "TCP 快速打开", "false"))
	}
	return fields
}
