//go:build linux

package engine

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/miekg/dns"
	allinone "github.com/zatxm/lanboat-allinone"
	"github.com/zatxm/lanboat-app/internal/profile"
	"github.com/zatxm/lanboat-app/internal/traffic"
	"github.com/zatxm/lanboat-app/internal/tunmode"
	"github.com/zatxm/lanboat-app/internal/tunworker"
	"go.uber.org/zap"
)

func TestMain(m *testing.M) {
	if len(os.Args) == 2 {
		switch os.Args[1] {
		case "--tun-worker":
			if app := os.Getenv("LANBOAT_TUN_TEST_APP"); app != "" {
				if err := syscall.Exec(app, []string{app, "--tun-worker"}, os.Environ()); err != nil {
					fmt.Fprintln(os.Stderr, err)
					os.Exit(1)
				}
			}
			if err := tunworker.Run(); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			os.Exit(0)
		case "--tun-probe":
			if err := tunIntegrationProbe(); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			os.Exit(0)
		}
	}
	os.Exit(m.Run())
}

func tunIntegrationProbe() error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	for _, typ := range []uint16{dns.TypeA, dns.TypeAAAA} {
		question := new(dns.Msg)
		question.SetQuestion("lanboat-tun.test.", typ)
		reply, _, err := (&dns.Client{Net: "udp", Timeout: 3 * time.Second}).ExchangeContext(ctx, question, "1.1.1.1:53")
		if err != nil {
			return fmt.Errorf("DNS/%d: %w", typ, err)
		}
		var host string
		for _, rr := range reply.Answer {
			switch rr := rr.(type) {
			case *dns.A:
				host = rr.A.String()
			case *dns.AAAA:
				host = rr.AAAA.String()
			}
		}
		if host == "" {
			return fmt.Errorf("no FakeIP answer: %s", reply)
		}
		body := strings.Repeat("TUN request body\n", 8192)
		tr := &http.Transport{Proxy: nil, DisableKeepAlives: true}
		client := &http.Client{Transport: tr, Timeout: 5 * time.Second}
		req, _ := http.NewRequestWithContext(ctx, "POST", "http://"+net.JoinHostPort(host, "80")+"/complete?via=tun", strings.NewReader(body))
		req.Host = "lanboat-tun.test"
		req.Header.Set("X-TUN-Probe", "captured")
		response, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("TCP/%d: %w", typ, err)
		}
		got, err := io.ReadAll(response.Body)
		response.Body.Close()
		tr.CloseIdleConnections()
		if err != nil || string(got) != body {
			return fmt.Errorf("TCP/%d payload changed (%d): %v", typ, len(got), err)
		}
		conn, err := net.DialTimeout("udp", net.JoinHostPort(host, "443"), 3*time.Second)
		if err != nil {
			return err
		}
		conn.SetDeadline(time.Now().Add(5 * time.Second))
		payload := bytes.Repeat([]byte("native datagram"), 700)
		_, err = conn.Write(payload)
		if err != nil {
			conn.Close()
			return err
		}
		packet := make([]byte, 65535)
		n, err := conn.Read(packet)
		conn.Close()
		if err != nil || !bytes.Equal(packet[:n], payload) {
			return fmt.Errorf("UDP/%d payload changed (%d): %v", typ, n, err)
		}
	}
	return nil
}

// This must run under unshare -Urn with a disposable default-route interface.
func TestTUNLinuxIntegratedRuntime(t *testing.T) {
	for _, kind := range []string{"socks5", "hysteria2"} {
		for _, auth := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/authentication=%t", kind, auth), func(t *testing.T) { runTUNLinuxIntegratedRuntime(t, kind, auth) })
		}
	}
}
func runTUNLinuxIntegratedRuntime(t *testing.T, kind string, auth bool) {
	if os.Getenv("LANBOAT_TEST_TUN") != "1" {
		t.Skip("isolated network namespace required")
	}
	own, _ := os.Readlink("/proc/self/ns/net")
	initial := os.Getenv("LANBOAT_TEST_HOST_NETNS")
	if os.Geteuid() != 0 || initial == "" || own == initial {
		t.Fatal("refusing to alter the host network")
	}
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.Copy(w, r.Body) }))
	defer origin.Close()
	_, tcpPort, _ := net.SplitHostPort(strings.TrimPrefix(origin.URL, "http://"))
	udp, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer udp.Close()
	_, udpPort, _ := net.SplitHostPort(udp.LocalAddr().String())
	go func() {
		b := make([]byte, 65535)
		for {
			n, a, err := udp.ReadFrom(b)
			if err != nil {
				return
			}
			udp.WriteTo(b[:n], a)
		}
	}()
	tcpTarget, _ := strconv.Atoi(tcpPort)
	udpTarget, _ := strconv.Atoi(udpPort)
	inbound := map[string]any{"type": "socks", "tag": "peer", "listen": "127.0.0.1"}
	if kind == "hysteria2" {
		fixture := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
		pair := fixture.TLS.Certificates[0]
		fixture.Close()
		key, _ := x509.MarshalPKCS8PrivateKey(pair.PrivateKey)
		inbound["type"] = "hysteria2"
		inbound["users"] = []any{map[string]any{"password": "hy2-node-private"}}
		inbound["obfs"] = map[string]any{"type": "salamander", "password": "local-obfs"}
		inbound["tls"] = map[string]any{"enabled": true, "certificate": []string{string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: pair.Certificate[0]}))}, "key": []string{string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: key}))}}
	}
	raw, _ := json.Marshal(map[string]any{"log": map[string]any{"disabled": true}, "inbounds": []any{inbound}, "outbounds": []any{map[string]any{"type": "direct", "tag": "direct"}}, "route": map[string]any{"final": "direct", "rules": []any{map[string]any{"network": "tcp", "action": "route-options", "override_address": "127.0.0.1", "override_port": tcpTarget}, map[string]any{"network": "udp", "action": "route-options", "override_address": "127.0.0.1", "override_port": udpTarget}}}})
	peer, err := allinone.New(context.Background(), raw)
	if err != nil {
		t.Fatal(err)
	}
	defer peer.Close()
	if err = peer.Start(); err != nil {
		t.Fatal(err)
	}
	host, port, _ := net.SplitHostPort(peer.Address("peer"))
	var nodePort int
	fmt.Sscan(port, &nodePort)
	c := profile.Default()
	c.Settings.Mode = "tun"
	c.Settings.Port = freePort(t)
	c.Settings.ProxyLog = true
	c.Settings.ProxyLogFull = true
	c.Settings.DNSLog = true
	if auth {
		c.Settings.Proxy.AllowLAN = true
		c.Settings.Proxy.Authentication = true
		c.Settings.Proxy.Username = "lanboat-test"
		c.Settings.Proxy.Password = "lanboat-private"
	}
	c.Nodes = []profile.Node{{ID: "peer", Name: "local peer", Type: kind, Server: host, Port: nodePort}}
	if kind == "hysteria2" {
		c.Nodes[0].Password = "hy2-node-private"
		c.Nodes[0].SetOption("skip-cert-verify", true)
		c.Nodes[0].SetOption("obfs-password", "local-obfs")
	}
	c.Selected = "peer"
	settings := profile.DefaultDNS()
	settings.FakeIP = true
	settings.FakeIPFilter = ""
	settings.Persist = true
	c.Settings.DNS = &settings
	store := traffic.New(t.TempDir())
	store.SetEnabled(true, true)
	engine := New(FindCore(), t.TempDir(), zap.NewNop())
	engine.SetTrafficStore(store)
	defer engine.Stop()
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	if err = engine.StartContext(ctx, c); err != nil {
		t.Fatal(err)
	}
	probe, cleanup, err := prepareDNSProbe(FindCore(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	output, err := exec.CommandContext(ctx, probe, "--tun-probe").CombinedOutput()
	if err != nil {
		t.Fatalf("TUN traffic: %v\n%s", err, output)
	}
	page, err := store.Page("proxy", 0)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, entry := range page.Entries {
		if entry.Method == "POST" {
			found = true
			if entry.URL != "http://lanboat-tun.test/complete?via=tun" || !entry.BodyTruncated || entry.Headers["X-Tun-Probe"][0] != "captured" {
				t.Fatalf("TUN request log: %+v", entry)
			}
		}
	}
	if !found {
		t.Fatal("TUN did not reach shared HTTP logger")
	}
	dnsPage, err := store.Page("dns", 0)
	if err != nil || dnsPage.Total < 2 {
		t.Fatal("DNS logging absent", dnsPage, err)
	}
	engine.Stop()
	if _, err = net.InterfaceByName(tunmode.Interface); err == nil {
		t.Fatal("TUN interface was not removed")
	}
	for _, family := range []string{"-4", "-6"} {
		output, _ := exec.Command("ip", family, "rule", "show").Output()
		if strings.Contains(string(output), "lookup 20245") {
			t.Fatal("TUN routing rule leaked", string(output))
		}
	}
}
