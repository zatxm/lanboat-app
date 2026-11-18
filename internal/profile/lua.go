package profile

import (
	"embed"
	"fmt"
	"strings"
)

//go:embed lua/*.lua
var luaFiles embed.FS

func (n Node) LuaFile() string {
	if name := n.TextOption("lua-file"); name != "" {
		return name
	}
	if n.TextOption("script") != "" {
		return "custom"
	}
	return "lightsword-backend.lua"
}

func (n Node) LuaScript() (string, error) {
	switch name := n.LuaFile(); name {
	case "custom":
		return n.TextOption("script"), nil
	case "http-backend.lua":
		return LuaHTTPExample, nil
	case "lightsword-backend.lua", "trojan-backend.lua":
		if name == "lightsword-backend.lua" {
			method := n.EffectiveOption("cipher", DefaultSSMethod)
			if strings.Contains(method, "gcm") || strings.Contains(method, "poly1305") {
				return "", fmt.Errorf("Lightsword 使用流式加密，不支持 AEAD 算法；请选择 CFB、CTR、RC4、Salsa20 或 ChaCha20")
			}
		}
		data, err := luaFiles.ReadFile("lua/" + name)
		return string(data), err
	default:
		return "", fmt.Errorf("Lua 文件选项无效")
	}
}

// LuaHTTPExample uses the public backend callbacks and validates fragmented CONNECT responses.
const LuaHTTPExample = `local backend = require 'backend'
local crypto = require 'crypto'
local pending = ''
local sent = false
local ready = false
function wa_lua_on_handshake_cb(ctx)
  if ready then return true end
  if not sent then
    local host = backend.get_address_host(ctx)
    local port = backend.get_address_port(ctx)
    local auth = ''
    if settings.username ~= '' then auth = 'Proxy-Authorization: Basic '..crypto.base64.encode(settings.username..':'..settings.password)..'\r\n' end
    backend.write(ctx, 'CONNECT '..host..':'..port..' HTTP/1.1\r\nHost: '..host..':'..port..'\r\n'..auth..'\r\n')
    sent = true
  end
  return false
end
function wa_lua_on_read_cb(ctx, data)
  if ready then return backend.RESULT.DIRECT, data end
  pending = pending .. data
  if #pending > 65536 then error('proxy response is too large') end
  local boundary = string.find(pending, '\r\n\r\n', 1, true)
  if not boundary then return backend.RESULT.HANDSHAKE, nil end
  if not string.match(pending, '^HTTP/%d%.%d 200 ') then error('CONNECT rejected') end
  ready = true
  local extra = string.sub(pending, boundary + 4)
  pending = ''
  return backend.RESULT.HANDSHAKE, extra
end
function wa_lua_on_write_cb(ctx, data) return backend.RESULT.DIRECT, data end
function wa_lua_on_close_cb(ctx) backend.free(ctx) return backend.RESULT.SUCCESS end
`
