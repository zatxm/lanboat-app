package native

import (
	"github.com/zatxm/lanboat-app/internal/appmeta"
	"github.com/zatxm/lanboat-app/internal/traffic"
	"runtime"
	"strings"

	qrcode "github.com/skip2/go-qrcode"
)

type popup struct {
	networkLog                         *traffic.Entry
	kind, title, link, bitmap, qrError string
	deletion                           *deletion
	previous                           *popup
}

func (u *UI) share(kind, id, name string) {
	var link string
	u.submit(func() error { var err error; link, err = u.backend.ShareLink(kind, id); return err }, func() {
		p := &popup{kind: "share", title: name, link: link}
		code, err := qrcode.New(link, qrcode.Low)
		if err != nil {
			p.qrError = "链接超出单个二维码容量，请复制链接。"
		} else {
			var b strings.Builder
			for _, row := range code.Bitmap() {
				for _, dark := range row {
					if dark {
						b.WriteByte('1')
					} else {
						b.WriteByte('0')
					}
				}
				b.WriteByte('\n')
			}
			p.bitmap = b.String()
		}
		u.popup = p
	})
}
func (u *UI) closePopup() {
	if !u.busy {
		if u.popup != nil && u.popup.kind == "confirm" {
			u.confirm = nil
		}
		if u.popup != nil && u.popup.kind == "message" {
			u.popup = u.popup.previous
		} else {
			u.popup = nil
		}
		u.err = ""
		u.notify()
	}
}
func (v *nativeView) syncPopup() {
	u := v.ui
	if u.confirm != nil && (u.popup == nil || u.popup.kind == "confirm" && u.popup.deletion != u.confirm) {
		u.popup = &popup{kind: "confirm", title: "确认", deletion: u.confirm}
	}
	if u.confirm == nil && u.popup != nil && u.popup.kind == "confirm" {
		u.popup = nil
	}
	if v.popupShown != u.popup {
		if v.popupRoot != nil {
			old := v.popupRoot
			for id, w := range v.widgets {
				for p := w; p != nil; p = p.parent {
					if p == old {
						delete(v.widgets, id)
						break
					}
				}
			}
			for id, w := range v.inputs {
				if _, ok := v.widgets[w.id]; !ok {
					delete(v.inputs, id)
				}
			}
			controls := v.controls[:0]
			for _, c := range v.controls {
				if _, ok := v.widgets[c.widget.id]; ok {
					controls = append(controls, c)
				}
			}
			v.controls = controls
			closeNativeDialog()
			v.popupRoot = nil
			v.popupError = nil
		}
		v.popupShown = u.popup
		if u.popup != nil {
			p := u.popup
			box := v.box("dialog-surface", false, 14, false)
			v.popupRoot = box
			box.event = func(event, value string) { u.closePopup() }
			title := "分享"
			if p.kind != "share" {
				title = p.title
			}
			if p.kind == "message" {
				v.pack(box, v.label(title, "dialog-title"))
			} else {
				v.pack(box, v.box("", true, 12, false, v.label(title, "dialog-title"), v.spacer(), v.button("关闭", "link icon-only icon-close", false, false, u.closePopup)))
			}
			width, height := 600, 580
			if p.kind == "message" {
				width, height = 520, min(480, max(200, 176+28*(len([]rune(p.link))/30+strings.Count(p.link, "\n"))))
				content := v.create("scroll", props{expand: true})
				v.pack(content, v.box("", false, 0, false, v.label(p.link, "node-name")))
				v.pack(box, content, v.box("", true, 10, false, v.spacer(), v.button("确认", "primary icon-check", false, false, u.closePopup)))
			} else if p.kind == "confirm" {
				width, height = 500, 280
				d := p.deletion
				prompt := confirmText(d)
				if d.prompt != "" {
					prompt = d.prompt
				}
				prompt = v.translate(prompt)
				var wrapped strings.Builder
				column := 0
				for _, r := range prompt {
					if r == '\n' {
						column = 0
						wrapped.WriteRune(r)
						continue
					}
					size := 1
					if r >= 0x2e80 {
						size = 2
					}
					if column+size > 54 {
						wrapped.WriteByte('\n')
						column = 0
					}
					wrapped.WriteRune(r)
					column += size
				}
				confirmClass := "primary icon-check"
				if d.kind != "backup-restore" && d.kind != "backup-overwrite" {
					confirmClass += " danger"
				}
				v.pack(box, v.dataLabel(wrapped.String(), "node-name"), v.spacer(), v.box("", true, 12, false, v.spacer(), v.button("取消", "secondary icon-close", false, false, u.closePopup), v.button("确认", confirmClass, false, false, func() {
					if d.run != nil {
						u.confirm = nil
						u.popup = nil
						d.run()
					} else {
						u.deleteConfirmed()
					}
				})))
				v.popupError = v.label("", "help error")
				v.popupError.setVisible(false)
				v.pack(box, v.popupError)
			} else if p.kind == "about" {
				width, height = 560, 400
				row := func(label, value string) {
					v.pack(box, v.box("", true, 16, false, v.label(label, "field-label about-key"), v.dataLabel(value, "node-name")))
				}
				row("名称", appmeta.Name)
				v.pack(box, v.box("", true, 16, false, v.label("版本号", "field-label about-key"), v.dataLabel(appmeta.Version, "node-name"), v.button("检查更新", "link icon-refresh", false, true, nil)))
				row("Golang 版本", runtime.Version())
				row(toolkitName+" 版本", toolkitVersion())
				memory := v.dataLabel(formatBytes(u.status.Memory.RSS), "node-name")
				memory.update = func() { memory.setText(formatBytes(u.status.Memory.RSS)) }
				memory.setTooltip("所有进程RSS合计")
				v.pack(box, v.box("", true, 16, false, v.label("内存", "field-label about-key"), memory))
				folder := v.button(displayDataDir(u.state.DataDir), "link icon-folder flush-left", false, false, func() { u.openExternal(u.state.DataDir) })
				folder.raw = true
				folder.localize()
				folder.setTooltip(u.state.DataDir)
				v.pack(box, v.box("", true, 16, false, v.label("文件目录", "field-label about-key"), folder), v.spacer())
			} else if p.kind == "group-test-settings" {
				width, height = 620, 0
				v.groupTestURLEditor(box)
			} else if p.kind == "speed-settings" || p.kind == "test-settings" {
				width, height = 620, 0
				v.connectivityEditor(box, p.kind == "speed-settings")
			} else if strings.HasPrefix(p.kind, "proxy-") {
				width, height = 620, 0
				v.proxyFields(box, strings.TrimPrefix(p.kind, "proxy-"))
			} else if p.kind == "rule-update-settings" {
				width, height = 560, 0
				v.ruleUpdateEditor(box)
			} else if p.kind == "tcp-settings" {
				width, height = 620, 0
				v.tcpEditor(box)
			} else if p.kind == "udp-settings" {
				width, height = 660, 0
				v.udpEditor(box)
			} else if p.kind == "network-log" {
				width, height = 700, 580
				v.networkLogDetail(box, p.networkLog)
			} else if p.kind == "dns-report" {
				width, height = 700, 620
				content := v.create("scroll", props{expand: true})
				v.pack(content, v.box("", false, 0, false, v.dataLabel(u.field("dns.report").Text(), "node-name")))
				v.pack(box, content, v.box("", true, 8, false, v.spacer(), v.button("网页检测", "link icon-globe", false, false, func() { u.openExternal("https://browserleaks.com/dns") })))
			} else if p.kind == "dns-rule" {
				width, height = 520, 450
				targets := []option{{"direct", "直连DNS"}, {"proxy", "代理DNS"}, {"reject", "拒绝"}}
				targets = append(targets, option{"bootstrap", "引导DNS"})
				choice := func(label, key string, opts []option) *widget {
					return v.selectField(label, u.field(key).Text(), opts, func(value string) { u.field(key).SetText(value) })
				}
				v.pack(box, choice("类型", "dnsRuleType", []option{{"DOMAIN-SUFFIX", "域名后缀"}, {"DOMAIN", "完整域名"}, {"DOMAIN-KEYWORD", "域名关键词"}, {"DOMAIN-REGEX", "正则表达式"}, {"QUERY-TYPE", "查询类型"}}), v.field("dnsRuleValue", "匹配内容", "example.com", false), choice("解析器", "dnsRuleTarget", targets), v.spacer(), v.box("", true, 8, false, v.spacer(), v.button("添加", "primary icon-check", false, false, u.addDNSPolicy)))
				v.popupError = v.label("", "help error")
				v.popupError.setVisible(false)
				v.pack(box, v.popupError)
			} else {
				v.pack(box, v.dataLabel(p.title, "node-name node-title"))
				if p.bitmap != "" {
					qr := v.create("qr", props{})
					qr.raw = true
					qr.setText(p.bitmap)
					v.pack(box, qr)
				} else {
					v.pack(box, v.label(p.qrError, "help error"))
				}
				link := v.create("text", props{value: p.link, readOnly: true, classes: "share-link"})
				v.pack(box, v.box("field", false, 7, false, v.label("链接地址", "field-label"), link))
				copied := v.label("", "help inline-value")
				v.pack(box, v.box("", true, 8, false, copied, v.spacer(), v.button("复制链接", "primary icon-copy", false, false, func() { copyClipboard(p.link); copied.setText("已复制") })))
			}
			openNativeDialog(box, width, height)
		}
	}
	if v.popupError != nil {
		v.popupError.setText(u.err)
		v.popupError.setVisible(u.err != "")
	}
}
