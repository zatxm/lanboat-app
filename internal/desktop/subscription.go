package desktop

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"

	"github.com/zatxm/lanboat-app/internal/profile"
)

var errSubscriptionRedirect = errors.New("订阅重定向无效（最多 5 次，HTTPS 不能降级）")

func subscriptionDownloadError(err error) error {
	// net/http errors include the full subscription URL, which can contain credentials.
	switch {
	case errors.Is(err, errSubscriptionRedirect):
		return errSubscriptionRedirect
	case errors.Is(err, context.DeadlineExceeded):
		return errors.New("下载订阅超时，请检查网络后重试")
	case errors.Is(err, io.EOF), errors.Is(err, io.ErrUnexpectedEOF):
		return errors.New("订阅服务器提前断开了连接，请重试")
	}
	var dns *net.DNSError
	if errors.As(err, &dns) {
		return errors.New("无法解析订阅服务器域名，请检查 DNS 设置")
	}
	var cert *tls.CertificateVerificationError
	if errors.As(err, &cert) {
		return errors.New("订阅服务器的 TLS 证书验证失败")
	}
	return errors.New("下载订阅失败，请检查网络和订阅地址")
}

func syncSubscriptionRules(c *profile.Config, sub profile.Subscription, content string, nodes []profile.Node) error {
	if content != "" {
		var err error
		content, err = c.ImportPolicyGroups(content, "overlay:"+sub.ID, nodes)
		if err != nil {
			return err
		}
	}
	overlays := make([]profile.RuleOverlay, 0, len(c.RuleOverlays)+1)
	found := false
	for _, old := range c.RuleOverlays {
		if old.ID != sub.ID {
			overlays = append(overlays, old)
			continue
		}
		found = true
		if content != "" {
			old.Name = sub.Name
			old.Content = content
			old.NodeIDs = nil
			for _, n := range nodes {
				old.NodeIDs = append(old.NodeIDs, n.ID)
			}
			overlays = append(overlays, old)
		}
	}
	if content != "" && !found {
		if len(overlays) >= 1000 {
			return errors.New("附加规则数量超过限制")
		}
		overlay := profile.RuleOverlay{ID: sub.ID, Name: sub.Name, Content: content}
		for _, n := range nodes {
			overlay.NodeIDs = append(overlay.NodeIDs, n.ID)
		}
		for _, s := range c.Subscriptions {
			if s.ID == sub.ID {
				overlay.SubscriptionID = sub.ID
				break
			}
		}
		// New subscriptions are appended to the profile after their rules are prepared.
		if sub.URL != "" {
			overlay.SubscriptionID = sub.ID
		}
		overlays = append(overlays, overlay)
	}
	c.RuleOverlays = overlays
	return nil
}

func removeOverlayNodes(c *profile.Config, deleted map[string]bool) {
	overlays := make([]profile.RuleOverlay, 0, len(c.RuleOverlays))
	for _, o := range c.RuleOverlays {
		ids := make([]string, 0, len(o.NodeIDs))
		for _, id := range o.NodeIDs {
			if !deleted[id] {
				ids = append(ids, id)
			}
		}
		o.NodeIDs = ids
		if len(ids) > 0 || o.SubscriptionID != "" {
			overlays = append(overlays, o)
		}
	}
	c.RuleOverlays = overlays
}
