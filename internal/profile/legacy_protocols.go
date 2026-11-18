package profile

import "encoding/json"

// Keep saved nodes, backups and share links usable after merging duplicate types.
func (n *Node) UnmarshalJSON(data []byte) error {
	type plainNode Node
	if err := json.Unmarshal(data, (*plainNode)(n)); err != nil {
		return err
	}
	n.normalizeProtocol()
	return nil
}

func (n *Node) normalizeProtocol() {
	switch n.Type {
	case "amneziawg":
		n.Type = "wireguard"
		n.SetOption("obfs", "amneziawg")
	case "shadowtls":
		if n.Option("ss-method") == nil && n.Option("ss-password") == nil {
			return
		}
		n.Type, n.Plugin = "ss", "shadow-tls"
		n.Method = n.TextOption("ss-method")
		password := n.Password
		n.Password = n.TextOption("ss-password")
		n.SetOption("plugin-opts.password", password)
		n.SetOption("plugin-opts.version", n.EffectiveOption("version", "3"))
		n.SetOption("plugin-opts.host", n.TextOption("sni"))
		n.SetOption("plugin-opts.skip-cert-verify", n.TextOption("skip-cert-verify") == "true")
		for _, key := range []string{"ss-method", "ss-password", "version", "sni", "skip-cert-verify"} {
			delete(n.Options, key)
		}
	}
}
