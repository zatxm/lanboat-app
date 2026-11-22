package corebackend

import (
	"context"
	"errors"
	"github.com/miekg/dns"
	allinone "github.com/zatxm/lanboat-allinone/v26"
	"github.com/zatxm/lanboat-app/internal/traffic"
	"net"
	"path/filepath"
	"time"
)

func ConnectionEntry(c allinone.ConnectionLog) (traffic.Entry, bool) {
	if c.Event == allinone.ConnectionOpened {
		return traffic.Entry{}, false
	}
	// A client abandoning its own dial is not a network event.
	if errors.Is(c.Error, context.Canceled) || errors.Is(c.Error, net.ErrClosed) {
		return traffic.Entry{}, false
	}
	entry := traffic.Entry{Time: c.Time.Format(time.RFC3339Nano), Kind: "proxy", URL: c.Destination, Network: c.Network, Domain: c.Domain, Policy: c.RouteOutbound, Rule: c.Rule, DurationMS: c.Elapsed.Milliseconds(), Upload: c.Upload, Download: c.Download, Process: c.ProcessName}
	if entry.Process == "" && c.ProcessPath != "" {
		entry.Process = filepath.Base(c.ProcessPath)
	}
	if c.Outbound != c.RouteOutbound {
		entry.Outbound = c.Outbound
	}
	if entry.Policy == "REJECT" {
		entry.Policy, entry.Outbound = "reject", ""
	}
	if c.Error != nil {
		entry.Error = c.Error.Error()
	}
	return entry, true
}

func dnsEntry(q allinone.DNSLog) (traffic.Entry, bool) {
	var question, response dns.Msg
	if question.Unpack(q.Query) != nil || len(question.Question) == 0 {
		return traffic.Entry{}, false
	}
	entry := traffic.Entry{Kind: "dns", Domain: question.Question[0].Name, QueryType: dns.TypeToString[question.Question[0].Qtype], DurationMS: q.Duration.Milliseconds()}
	if q.Error != nil {
		entry.Error = q.Error.Error()
	} else if response.Unpack(q.Response) == nil {
		entry.Result = dns.RcodeToString[response.Rcode]
		for _, answer := range response.Answer {
			entry.Result += "\n" + answer.String()
		}
	}
	entry.Time = time.Now().Format(time.RFC3339Nano)
	return entry, true
}
