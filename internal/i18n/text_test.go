package i18n

import (
	"github.com/zatxm/lanboat-app/internal/profile"
	"strings"
	"testing"
	"unicode"
)

func TestTranslations(t *testing.T) {
	for _, test := range []struct{ source, want string }{
		{"设置", "Settings"}, {"外观\t雅黑", "Appearance\tDark"},
		{"    本地节点", "    Local nodes"}, {"已测试本组 3 / 4 个节点", "Tested 3 / 4 nodes in this group"},
		{"第 2 / 10 页 · 188 个节点", "Page 2 / 10 · 188 nodes"},
		{"用户名 *", "Username *"}, {"私钥口令（留空保留）", "Private key passphrase (leave blank to keep)"},
		{"当前：白色", "Current: Light"}, {"删除「连接」？", "Delete “连接”?"},
		{"保存配置失败：permission denied", "Failed to save configuration: permission denied"},
	} {
		if got := Text("en", test.source); got != test.want {
			t.Errorf("%q: got %q, want %q", test.source, got, test.want)
		}
		if Text("zh", test.source) != test.source {
			t.Fatal("Chinese changed")
		}
	}
}
func TestProtocolMetadataTranslated(t *testing.T) {
	for _, proto := range profile.Protocols {
		for _, field := range proto.Fields {
			for _, suffix := range []string{"", " *", "（留空保留）"} {
				text := Text("en", field.Title+suffix)
				if strings.ContainsFunc(text, func(r rune) bool { return unicode.Is(unicode.Han, r) }) {
					t.Errorf("untranslated %s: %s", proto.ID, text)
				}
			}
		}
	}
	for source, target := range english {
		if len(token.FindAllString(source, -1)) != len(token.FindAllString(target, -1)) {
			t.Errorf("format mismatch: %s", source)
		}
	}
}
