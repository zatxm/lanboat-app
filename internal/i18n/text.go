package i18n

import (
	_ "embed"
	"encoding/json"
	"regexp"
	"sort"
	"strings"
)

//go:embed en.json
var englishJSON []byte
var english map[string]string
var formats []translation
var token = regexp.MustCompile(`%[ds]`)

type translation struct {
	source, target string
	pattern        *regexp.Regexp
}

func init() {
	if err := json.Unmarshal(englishJSON, &english); err != nil {
		panic(err)
	}
	for source, target := range english {
		if !token.MatchString(source) {
			continue
		}
		var pattern strings.Builder
		pattern.WriteString("^")
		start := 0
		for _, at := range token.FindAllStringIndex(source, -1) {
			pattern.WriteString(regexp.QuoteMeta(source[start:at[0]]))
			if source[at[0]:at[1]] == "%d" {
				pattern.WriteString(`([0-9]+)`)
			} else {
				pattern.WriteString(`(.*?)`)
			}
			start = at[1]
		}
		pattern.WriteString(regexp.QuoteMeta(source[start:]))
		pattern.WriteString("$")
		formats = append(formats, translation{source, target, regexp.MustCompile(pattern.String())})
	}
	sort.Slice(formats, func(i, j int) bool { return len(formats[i].source) > len(formats[j].source) })
}

// Text translates owned interface copy only. Callers keep user data out of this path.
func Text(language, text string) string {
	if language != "en" || text == "" {
		return text
	}
	if translated, ok := english[text]; ok {
		return translated
	}
	if strings.Contains(text, "\n") {
		parts := strings.Split(text, "\n")
		for i := range parts {
			parts[i] = Text(language, parts[i])
		}
		return strings.Join(parts, "\n")
	}
	if strings.Contains(text, "\t") {
		parts := strings.Split(text, "\t")
		for i := range parts {
			parts[i] = Text(language, parts[i])
		}
		return strings.Join(parts, "\t")
	}
	trim := strings.TrimSpace(text)
	if trim != text && trim != "" {
		return strings.Replace(text, trim, Text(language, trim), 1)
	}
	for _, suffix := range []string{" *", "（留空保留）", "  ✓"} {
		if strings.HasSuffix(text, suffix) {
			return Text(language, strings.TrimSuffix(text, suffix)) + Text(language, suffix)
		}
	}
	for _, entry := range formats {
		values := entry.pattern.FindStringSubmatch(text)
		if values == nil {
			continue
		}
		i := 0
		return token.ReplaceAllStringFunc(entry.target, func(_ string) string {
			i++
			value := values[i]
			if entry.source == "%s不能为空" || entry.source == "%s参数过长" || entry.source == "%s必须是 JSON 对象" || entry.source == "%s必须为有效的非负整数" || entry.source == "当前：%s" || entry.source == "%s必须是数字" || entry.source == "%s过长" || entry.source == "请填写%s" || entry.source == "%s选项无效" || entry.source == "%s必须是有效的非负整数" || strings.HasSuffix(entry.source, "：%s") {
				value = Text(language, value)
			}
			return value
		})
	}
	return text
}
