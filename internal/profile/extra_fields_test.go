package profile

import "testing"

func TestExtraNodeForms(t *testing.T) {
	for _, kind := range []string{"snell", "mieru", "brook", "lua"} {
		n := Node{Type: kind, Server: "example.com", Port: 443, Name: "test", Password: "password"}
		fields := map[string]Field{}
		for _, f := range NodeFields(n) {
			if _, ok := fields[f.Key]; ok {
				t.Fatal(kind, "duplicate", f.Key)
			}
			fields[f.Key] = f
		}
		if fields["tfo"].Default != "false" && kind != "mieru" && kind != "brook" {
			t.Fatal(kind, "TFO default")
		}
		if kind == "snell" {
			if fields["version"].Default != "1" || len(fields["version"].Choices) != 5 || fields["udp"].Default != "false" || fields["idle-timeout"].Placeholder != "30,30,1" || fields["idle-timeout"].Default != "" {
				t.Fatal("Snell defaults")
			}
			for _, obfs := range []string{"http", "tls"} {
				n.SetOption("obfs-opts.mode", obfs)
				seen := map[string]Field{}
				for _, f := range NodeFields(n) {
					seen[f.Key] = f
				}
				if seen["obfs-opts.host"].Group == "" {
					t.Fatal("ungrouped obfs")
				}
				_, path := seen["obfs-opts.path"]
				if path != (obfs == "http") {
					t.Fatal("Snell path condition")
				}
				if path && seen["obfs-opts.path"].Default != "/" {
					t.Fatal("Snell path default")
				}
			}
		}
		if kind == "mieru" && (fields["transport"].Default != "tcp" || fields["user-hint"].Default != "false" || fields["udp"].Default != "true") {
			t.Fatal("Mieru defaults")
		}
		if kind == "brook" {
			if fields["network"].Default != "none" || fields["udp"].Default != "true" {
				t.Fatal("Brook defaults")
			}
			for _, network := range []string{"ws", "wss"} {
				n.SetOption("network", network)
				found := false
				for _, f := range NodeFields(n) {
					if f.Key == "path" {
						found = f.Default == "/" && f.Group != ""
					}
				}
				if !found {
					t.Fatal("Brook conditional path")
				}
			}
		}
		if kind == "lua" {
			if n.LuaFile() != "lightsword-backend.lua" || len(fields["cipher"].Choices) != len(Methods) {
				t.Fatal("Lua defaults")
			}
			for _, file := range []string{"http-backend.lua", "lightsword-backend.lua", "trojan-backend.lua"} {
				n.SetOption("lua-file", file)
				script, err := n.LuaScript()
				if err != nil || script == "" {
					t.Fatal(file, err)
				}
			}
			legacy := Node{Type: "lua"}
			legacy.SetOption("script", "private-script")
			masked := legacy.Masked()
			if masked.LuaFile() != "custom" || masked.TextOption("script") != "" {
				t.Fatal("legacy Lua script masking")
			}
		}
	}
}

func TestSnellAndLuaWireCombinations(t *testing.T) {
	for _, version := range []string{"4", "5"} {
		n := Node{Type: "snell", Name: "fixture", Server: "example.com", Port: 443, Password: "password"}
		n.SetOption("version", version)
		n.SetOption("obfs-opts.mode", "tls")
		if n.Validate() == nil {
			t.Fatal("official server cannot accept TLS obfs", version)
		}
	}
	for _, version := range []string{"1", "3"} {
		n := Node{Type: "snell", Name: "fixture", Server: "example.com", Port: 443, Password: "password"}
		n.SetOption("version", version)
		n.SetOption("mux", true)
		if n.Validate() == nil {
			t.Fatal("legacy version cannot reuse", version)
		}
	}
	n := Node{Type: "lua"}
	n.SetOption("cipher", "aes-256-gcm")
	if _, err := n.LuaScript(); err == nil {
		t.Fatal("Lightsword does not define AEAD record framing")
	}
}
