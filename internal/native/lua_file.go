package native

import (
	"io"
	"os"
	"unicode/utf8"
)

func (u *UI) importLuaFile(path string) {
	f, err := os.Open(path)
	if err != nil {
		u.err = "无法读取 Lua 文件"
		u.render()
		return
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, 32769))
	if err != nil || len(data) == 0 || len(data) > 32768 || !utf8.Valid(data) {
		u.err = "Lua 文件必须为 UTF-8 文本且不超过 32 KiB"
		u.render()
		return
	}
	u.changeSSChoice("lua-file", "custom")
	u.draft.SetOption("script", string(data))
	u.field("opt.script").SetText(string(data))
	u.err = ""
	u.render()
}
