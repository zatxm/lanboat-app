package routing

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

const DisabledPrefix = "# lanboat-disabled "

type Entry struct {
	Rule     Rule
	Disabled bool
}

func Entries(content string) ([]Entry, error) {
	var out []Entry
	section := ""
	for i, line := range strings.Split(content, "\n") {
		text := strings.TrimSpace(line)
		if strings.HasPrefix(text, "[") && strings.HasSuffix(text, "]") {
			section = strings.ToLower(text[1 : len(text)-1])
			continue
		}
		if section != "rule" {
			continue
		}
		disabled := strings.HasPrefix(text, DisabledPrefix)
		if disabled {
			text = strings.TrimPrefix(text, DisabledPrefix)
		}
		if text == "" || strings.HasPrefix(text, "#") || strings.HasPrefix(text, "//") {
			continue
		}
		if strings.HasPrefix(strings.ToUpper(text), "FINAL,") || strings.HasPrefix(strings.ToUpper(text), "MATCH,") {
			continue
		}
		p, err := Parse("[Rule]\n" + text)
		if err != nil {
			return nil, err
		}
		r := p.Rules[0]
		r.Line = i + 1
		out = append(out, Entry{r, disabled})
	}
	return out, nil
}
func (r Rule) Text() string {
	action := r.Action
	if action == "direct" || action == "proxy" || action == "reject" {
		action = strings.ToUpper(action)
	}
	text := r.Type + "," + r.Value + "," + action
	if r.NoResolve {
		text += ",no-resolve"
	}
	if r.SetInterval > 0 {
		text += ",interval=" + strconv.Itoa(r.SetInterval)
	}
	if len(r.SetHeaders) > 0 {
		if encoded, err := json.Marshal(r.SetHeaders); err == nil {
			text += ",headers=" + base64.RawURLEncoding.EncodeToString(encoded)
		}
	}
	return text
}
func ReplaceLine(content string, line int, text string) (string, error) {
	lines := strings.Split(content, "\n")
	if line < 1 || line > len(lines) {
		return "", fmt.Errorf("规则已变化，请重新打开")
	}
	if text == "" {
		lines = append(lines[:line-1], lines[line:]...)
	} else {
		if strings.HasSuffix(lines[line-1], "\r") {
			text += "\r"
		}
		lines[line-1] = text
	}
	result := strings.Join(lines, "\n")
	if _, err := Parse(result); err != nil {
		return "", err
	}
	return result, nil
}
func InsertSectionLine(content, section, text string) (string, error) {
	lines := strings.Split(content, "\n")
	start, end := -1, len(lines)
	for i, line := range lines {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "[") && strings.HasSuffix(trim, "]") {
			if start >= 0 {
				end = i
				break
			}
			if strings.EqualFold(trim, "["+section+"]") {
				start = i
			}
		}
	}
	if start < 0 {
		content = strings.TrimRight(content, "\n") + "\n\n[" + section + "]\n" + text + "\n"
	} else {
		if section == "Rule" {
			for i := start + 1; i < end; i++ {
				t := strings.ToUpper(strings.TrimSpace(lines[i]))
				if strings.HasPrefix(t, "FINAL,") || strings.HasPrefix(t, "MATCH,") {
					end = i
					break
				}
			}
		}
		lines = append(lines[:end], append([]string{text}, lines[end:]...)...)
		content = strings.Join(lines, "\n")
	}
	if _, err := Parse(content); err != nil {
		return "", err
	}
	return content, nil
}
func SetFinal(content, action string) (string, error) {
	if !ValidPolicy(action) {
		return "", fmt.Errorf("规则策略无效")
	}
	line := 0
	section := ""
	for i, raw := range strings.Split(content, "\n") {
		text := strings.TrimSpace(raw)
		if strings.HasPrefix(text, "[") && strings.HasSuffix(text, "]") {
			section = strings.ToLower(text[1 : len(text)-1])
		}
		t := strings.ToUpper(text)
		if section == "rule" && (strings.HasPrefix(t, "FINAL,") || strings.HasPrefix(t, "MATCH,")) {
			line = i + 1
		}
	}
	if line > 0 {
		return ReplaceLine(content, line, "FINAL,"+action)
	}
	return InsertSectionLine(content, "Rule", "FINAL,"+action)
}
