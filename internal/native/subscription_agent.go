package native

import (
	"fmt"
	"net/url"
	"slices"

	"github.com/zatxm/lanboat-app/internal/appmeta"
	"github.com/zatxm/lanboat-app/internal/profile"
)

func subscriptionAgentLabel(raw string) string {
	if address, err := url.Parse(raw); err == nil {
		if agent, fallback := profile.SubscriptionFormatAgent(address); agent != "" {
			return agent
		} else if fallback != "" {
			return "Lanboat / " + fallback
		}
	}
	return "Lanboat/" + appmeta.Version
}

func (v *nativeView) subscriptionAgent(key, addressKey string) *widget {
	u := v.ui
	agent := u.field(key)
	custom := u.field(key + ".custom")
	mode := agent.Text()
	if custom.Text() == "true" || mode != "" && !slices.Contains(profile.SubscriptionUserAgents, mode) {
		mode = "custom"
	}
	auto := func() option {
		return option{"", fmt.Sprintf("自动（%s）", subscriptionAgentLabel(u.field(addressKey).Text()))}
	}
	choices := []option{auto()}
	for _, value := range profile.SubscriptionUserAgents {
		choices = append(choices, option{value, value})
	}
	choices = append(choices, option{"custom", "自定义"})

	manual := v.input(key, "", false, false, false)
	manual.pointer.Label("UserAgent")
	manual.setVisible(mode == "custom")

	selector := v.selectWidget(mode, choices, false, func(value string) {
		if value == "custom" {
			if mode != value {
				agent.SetText("")
				manual.setValue("")
			}
			custom.SetText("true")
		} else {
			custom.SetText("")
			agent.SetText(value)
		}
		mode = value
		manual.setVisible(mode == "custom")
	})
	v.inputs[key+".choice"] = selector
	if field := v.inputs[addressKey]; field != nil {
		change := field.event
		field.event = func(event, value string) {
			change(event, value)
			if next := auto(); choices[0] != next {
				choices[0] = next
				selector.setOptions(choices, mode)
			}
		}
	}
	return v.nodeRow("UserAgent", selector, manual)
}
