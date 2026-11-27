package native

import (
	"strings"
	"testing"
	"time"
)

func TestNativeWindowRuleDataSettings(t *testing.T) {
	u := testUI(t)
	u.notify = requestIdle
	v, err := newView(u)
	if err != nil {
		t.Fatal(err)
	}
	active = v
	defer func() { active = nil; v.dispose() }()
	wait := func() {
		t.Helper()
		limit := time.Now().Add(5 * time.Second)
		for {
			pump()
			if !u.busy {
				break
			}
			if time.Now().After(limit) {
				t.Fatal("timeout")
			}
			time.Sleep(time.Millisecond)
		}
		if u.err != "" {
			t.Fatal(u.err)
		}
	}
	find := func(label string) *widget {
		t.Helper()
		for _, c := range v.controls {
			if c.widget.text == label || strings.HasPrefix(c.widget.text, label+"\t") {
				return c.widget
			}
		}
		t.Fatalf("missing %s", label)
		return nil
	}
	v.render()
	pump()
	if modes[0].label != "规则" {
		t.Fatal("incorrect home mode")
	}
	u.navigate("settings")
	wait()
	_, proxyY, _, _ := find("代理").bounds()
	_, ruleY, _, _ := find("路由规则").bounds()
	_, dnsY, _, _ := find("DNS设置").bounds()
	_, testY, _, _ := find("连通性测试").bounds()
	_, leakY, _, _ := find("代理泄漏检测").bounds()
	_, aboutY, _, _ := find("关于").bounds()
	if !(proxyY < ruleY && ruleY < dnsY && dnsY < testY && testY < leakY && leakY < aboutY) {
		t.Fatalf("settings order: %d %d %d %d %d %d", proxyY, ruleY, dnsY, testY, leakY, aboutY)
	}
	find("路由规则").click()
	wait()
	if !u.state.Config.Settings.RulesAutoUpdate() {
		t.Fatal("automatic updates not default")
	}
	find("设置").click()
	pump()
	if u.popup == nil || u.popup.kind != "rule-update-settings" {
		t.Fatal("missing settings dialog")
	}
	auto := find("自动更新")
	var region, interval *widget
	for _, c := range v.controls {
		w := c.widget
		if w.kind == "select" && len(w.options) > 100 {
			region = w
		}
		if w.kind == "select" && len(w.options) == 7 && w.options[0].value == "1" {
			interval = w
		}
	}
	if region == nil || interval == nil {
		t.Fatal("missing update selectors")
	}
	auto.click()
	pump()
	if u.field("ruleUpdate.auto").Text() != "false" || !u.state.Config.Settings.RulesAutoUpdate() {
		t.Fatal("switch did not preserve draft")
	}
	if interval.visible {
		t.Fatal("disabled update interval still visible")
	}
	var save *widget
	for _, c := range v.controls {
		if c.widget.text == "确定" {
			for p := c.widget; p != nil; p = p.parent {
				if p == v.popupRoot {
					save = c.widget
					break
				}
			}
		}
	}
	if save == nil {
		t.Fatal("missing dialog save")
	}
	save.click()
	wait()
	if u.backend.GetState().Config.Settings.RulesAutoUpdate() {
		t.Fatal("disabled state not saved")
	}
	find("设置").click()
	pump()
	auto = find("自动更新")
	auto.click()
	pump()
	interval = v.inputs["ruleUpdate.days"]
	if interval == nil || !interval.visible {
		t.Fatal("enabled update interval hidden")
	}
	interval.edit("7")
	pump()
	for _, c := range v.controls {
		if c.widget.text == "确定" {
			for p := c.widget; p != nil; p = p.parent {
				if p == v.popupRoot {
					save = c.widget
					break
				}
			}
		}
	}
	save.click()
	wait()
	region.edit("us")
	wait()
	saved := u.backend.GetState().Config.Settings
	if saved.RuleUpdateDays != 7 || saved.RuleRegion != "us" {
		t.Fatalf("settings not persisted: %+v", saved)
	}
	var summary *widget
	for _, w := range v.widgets {
		if strings.Contains(w.classes, "rule-update-summary") {
			summary = w
			break
		}
	}
	if summary == nil || summary.text != "每7天更新" {
		t.Fatalf("incorrect update summary: %v", summary)
	}
	region.edit("")
	wait()
	if u.backend.GetState().Config.Settings.RuleRegion != "" {
		t.Fatal("automatic region not restored")
	}
}
