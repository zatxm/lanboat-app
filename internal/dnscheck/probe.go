package dnscheck

import (
	"context"
	"github.com/miekg/dns"
	"net/netip"
	"time"
)

const Sentinel = "lanboat-dns-check.invalid."
const FakeDomain = "lanboat-fakeip-check.invalid."

type Probe struct {
	Network string   `json:"network"`
	Type    string   `json:"type"`
	Name    string   `json:"name"`
	Answers []string `json:"answers,omitempty"`
	Error   string   `json:"error,omitempty"`
}

func Query(ctx context.Context, network, address, name string, qtype uint16) Probe {
	p := Probe{Network: network, Type: dns.TypeToString[qtype], Name: name}
	q := new(dns.Msg)
	q.SetQuestion(dns.Fqdn(name), qtype)
	r, _, err := (&dns.Client{Net: network, Timeout: 3 * time.Second}).ExchangeContext(ctx, q, address)
	if err != nil {
		p.Error = err.Error()
		return p
	}
	if r.Rcode != dns.RcodeSuccess {
		p.Error = dns.RcodeToString[r.Rcode]
		return p
	}
	for _, rr := range r.Answer {
		switch v := rr.(type) {
		case *dns.A:
			p.Answers = append(p.Answers, v.A.String())
		case *dns.AAAA:
			p.Answers = append(p.Answers, v.AAAA.String())
		}
	}
	if len(p.Answers) == 0 {
		p.Error = "no address answer"
	}
	return p
}
func TUN(ctx context.Context) []Probe {
	out := make([]Probe, 4)
	done := make(chan int, 4)
	for i := range out {
		go func(i int) {
			network, name, typ := "udp", Sentinel, uint16(dns.TypeA)
			if i == 1 {
				network = "tcp"
			}
			if i >= 2 {
				name = FakeDomain
			}
			if i == 3 {
				typ = dns.TypeAAAA
			}
			out[i] = Query(ctx, network, "192.0.2.53:53", name, typ)
			done <- i
		}(i)
	}
	for range out {
		<-done
	}
	return out
}
func InRange(p Probe, prefix string) bool {
	r, e := netip.ParsePrefix(prefix)
	if e != nil || p.Error != "" || len(p.Answers) == 0 {
		return false
	}
	for _, s := range p.Answers {
		a, e := netip.ParseAddr(s)
		if e != nil || !r.Contains(a) {
			return false
		}
	}
	return true
}
