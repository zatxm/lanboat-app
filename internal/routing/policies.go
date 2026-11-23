package routing

import "strings"

// InspectPolicies visits rule targets, including disabled rules and explicit FINAL/MATCH lines.
func InspectPolicies(content string, parsed Parsed, visit func(string)) {
	policyLines(content, parsed, func(ref string) string {
		visit(ref)
		return ref
	}, nil)
}

// RewritePolicies changes policy fields and removes the listed group definitions.
func RewritePolicies(content string, parsed Parsed, replace func(string) string) string {
	var output strings.Builder
	output.Grow(len(content))
	policyLines(content, parsed, replace, &output)
	return output.String()
}

func policyLines(content string, parsed Parsed, replace func(string) string, output *strings.Builder) {
	line := 0
	ruleIndex, groupIndex := 0, 0
	section := ""
	for raw := range strings.SplitAfterSeq(content, "\n") {
		line++
		text := strings.TrimSpace(strings.TrimPrefix(raw, "\ufeff"))
		if strings.HasPrefix(text, "[") && strings.HasSuffix(text, "]") {
			section = strings.ToLower(strings.TrimSpace(text[1 : len(text)-1]))
		}
		if groupIndex < len(parsed.Groups) && parsed.Groups[groupIndex].Line == line {
			groupIndex++
			continue
		}
		if section == "rule" {
			rule := Rule{}
			if ruleIndex < len(parsed.Rules) && parsed.Rules[ruleIndex].Line == line {
				rule = parsed.Rules[ruleIndex]
				ruleIndex++
			}
			prefix := ""
			if strings.HasPrefix(text, DisabledPrefix) {
				prefix = DisabledPrefix
				text = strings.TrimPrefix(text, prefix)
				if p, err := Parse("[Rule]\n" + text); err == nil && len(p.Rules) == 1 {
					rule = p.Rules[0]
				}
			}
			kind, rest, _ := strings.Cut(text, ",")
			next := raw
			switch {
			case rule.Action != "":
				if action := replace(rule.Action); action != rule.Action {
					rule.Action = action
					next = prefix + rule.Text()
					if strings.Contains(text, ",force-remote-dns") {
						next += ",force-remote-dns"
					}
				}
			case strings.EqualFold(kind, "FINAL") || strings.EqualFold(kind, "MATCH"):
				value, suffix, extra := strings.Cut(rest, ",")
				if action, ok := policy(strings.TrimSpace(value)); ok {
					if changed := replace(action); changed != action {
						next = prefix + kind + "," + changed
						if extra {
							next += "," + suffix
						}
					}
				}
			}
			if raw != next {
				switch {
				case strings.HasSuffix(raw, "\r\n"):
					next += "\r\n"
				case strings.HasSuffix(raw, "\n"):
					next += "\n"
				}
				raw = next
			}
		}
		if output != nil {
			output.WriteString(raw)
		}
	}
}
