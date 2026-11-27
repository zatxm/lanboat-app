package profile

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/zatxm/lanboat-app/internal/ruledata"
)

const regionalA, regionalZ = '🇦', '🇿'

// A leading country flag belongs in the list's flag slot, outside the text renderer.
func SplitNodeFlag(name string) (label, code string) {
	text := strings.TrimLeftFunc(name, unicode.IsSpace)
	a, n := utf8.DecodeRuneInString(text)
	if a < regionalA || a > regionalZ {
		return name, ""
	}
	b, m := utf8.DecodeRuneInString(text[n:])
	if b < regionalA || b > regionalZ {
		return name, ""
	}
	code = string([]byte{byte(a-regionalA) + 'a', byte(b-regionalA) + 'a'})
	label = strings.TrimSpace(strings.TrimLeft(text[n+m:], "\uFE0E\uFE0F"))
	if label == "" {
		label = strings.ToUpper(code)
	}
	return label, code
}

func NodeFlagCountry(flag string) string {
	_, code := SplitNodeFlag(flag)
	if !ruledata.ValidRegion(code) {
		return ""
	}
	return code
}

func CountryFlag(code string) string {
	if len(code) != 2 || !ruledata.ValidRegion(code) {
		return ""
	}
	return string([]rune{regionalA + rune(code[0]-'a'), regionalA + rune(code[1]-'a')})
}

// FillFlagFromName captures the flag in a provider's remark when it is the only
// geographic metadata available.
func (n *Node) FillFlagFromName() {
	if n.Flag == "" {
		_, code := SplitNodeFlag(n.Name)
		n.Flag = CountryFlag(code)
	}
}
