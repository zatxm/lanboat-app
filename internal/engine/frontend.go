package engine

import (
	"bufio"
	"context"
	"crypto/subtle"
	"encoding/base64"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/zatxm/lanboat-app/internal/profile"
	"github.com/zatxm/lanboat-app/internal/routing"
)

type proxyFront struct {
	listener    net.Listener
	server      *http.Server
	mu          sync.Mutex
	connections map[net.Conn]bool
	upstream    string
	ready       chan net.Conn
	done        chan struct{}
	once        sync.Once
	settings    profile.ProxySettings
}
type bufferedConn struct {
	net.Conn
	reader  *bufio.Reader
	onClose func()
	once    sync.Once
}

func (c *bufferedConn) Read(b []byte) (int, error) { return c.reader.Read(b) }
func (c *bufferedConn) Close() error               { c.once.Do(c.onClose); return c.Conn.Close() }

type httpMuxListener struct{ *proxyFront }

func (l httpMuxListener) Addr() net.Addr { return l.listener.Addr() }
func (l httpMuxListener) Close() error   { l.stop(); return nil }
func (l httpMuxListener) Accept() (net.Conn, error) {
	select {
	case c := <-l.ready:
		return c, nil
	case <-l.done:
		return nil, net.ErrClosed
	}
}
func (f *proxyFront) stop() { f.once.Do(func() { close(f.done); _ = f.listener.Close() }) }
func (f *proxyFront) accept() {
	for {
		conn, err := f.listener.Accept()
		if err != nil {
			f.stop()
			return
		}
		f.mu.Lock()
		if len(f.connections) >= 128 {
			f.mu.Unlock()
			conn.Close()
			continue
		}
		f.connections[conn] = true
		f.mu.Unlock()
		go func() {
			reader := bufio.NewReader(conn)
			c := &bufferedConn{Conn: conn, reader: reader, onClose: func() { f.mu.Lock(); delete(f.connections, conn); f.mu.Unlock() }}
			_ = conn.SetReadDeadline(time.Now().Add(10 * time.Second))
			first, err := reader.Peek(1)
			if err != nil {
				c.Close()
				return
			}
			_ = conn.SetReadDeadline(time.Time{})
			if first[0] == 5 {
				f.relay(c)
				return
			}
			select {
			case f.ready <- c:
			case <-f.done:
				c.Close()
			}
		}()
	}
}
func newProxyFront(port, corePort, forcePort, directPort int, rules routing.Parsed, p profile.ProxySettings, policyPorts map[string]int, core string) (*proxyFront, error) {
	listener, err := net.Listen("tcp", net.JoinHostPort(p.ListenHost(), strconv.Itoa(port)))
	if err != nil {
		return nil, err
	}
	f := &proxyFront{listener: listener, upstream: net.JoinHostPort("127.0.0.1", strconv.Itoa(corePort)), connections: map[net.Conn]bool{}, ready: make(chan net.Conn), done: make(chan struct{})}
	f.settings = p
	target, _ := url.Parse("http://" + f.upstream)
	transport := &http.Transport{Proxy: http.ProxyURL(target), MaxIdleConns: 8, IdleConnTimeout: 30 * time.Second}
	directURL, _ := url.Parse("http://127.0.0.1:" + strconv.Itoa(directPort))
	direct := &http.Transport{Proxy: http.ProxyURL(directURL), MaxIdleConns: 8, IdleConnTimeout: 30 * time.Second}
	forceURL, _ := url.Parse("http://127.0.0.1:" + strconv.Itoa(forcePort))
	forced := &http.Transport{Proxy: http.ProxyURL(forceURL), MaxIdleConns: 8, IdleConnTimeout: 30 * time.Second}
	policies := map[string]*http.Transport{}
	for action, port := range policyPorts {
		target, _ := url.Parse("http://127.0.0.1:" + strconv.Itoa(port))
		policies[action] = &http.Transport{Proxy: http.ProxyURL(target), MaxIdleConns: 4, IdleConnTimeout: 30 * time.Second}
	}
	reverse := &httputil.ReverseProxy{Director: func(r *http.Request) { r.Header.Del("Proxy-Connection"); r.Header.Del("Proxy-Authorization") }, Transport: transport}
	rewrites := make([]*regexp.Regexp, len(rules.Rewrites))
	for i, r := range rules.Rewrites {
		rewrites[i] = regexp.MustCompile(r.Pattern)
	}
	binaryMatches := make(chan struct{}, 1)
	f.server = &http.Server{ReadHeaderTimeout: 10 * time.Second, MaxHeaderBytes: 32 << 10, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if p.Authentication {
			value := strings.TrimPrefix(r.Header.Get("Proxy-Authorization"), "Basic ")
			auth, err := base64.StdEncoding.DecodeString(value)
			if err != nil || subtle.ConstantTimeCompare(auth, []byte(p.Username+":"+p.Password)) != 1 {
				w.Header().Set("Proxy-Authenticate", `Basic realm="Lanboat"`)
				http.Error(w, "Proxy authentication required", http.StatusProxyAuthRequired)
				return
			}
		}
		r.Header.Del("Proxy-Authorization")
		if r.Method == http.MethodConnect {
			client, buffer, err := w.(http.Hijacker).Hijack()
			if err != nil {
				return
			}
			defer client.Close()
			up, err := net.DialTimeout("tcp", f.upstream, 3*time.Second)
			if err != nil {
				return
			}
			defer up.Close()
			if r.WriteProxy(up) != nil {
				return
			}
			go func() { _, _ = io.Copy(up, buffer); up.Close() }()
			_, _ = io.Copy(client, up)
			return
		}
		for i, re := range rewrites {
			if re.MatchString(r.URL.String()) {
				rule := rules.Rewrites[i]
				http.Redirect(w, r, re.ReplaceAllString(r.URL.String(), rule.Replacement), rule.Status)
				return
			}
		}
		port, _ := strconv.Atoi(r.URL.Port())
		if port == 0 {
			port = 80
		}
		if action, ok := rules.HTTPPolicyWithMatcher(r.URL.Hostname(), port, r.UserAgent(), func(rule routing.Rule, req routing.Request) (bool, error) {
			ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
			defer cancel()
			select {
			case binaryMatches <- struct{}{}:
				defer func() { <-binaryMatches }()
			case <-ctx.Done():
				return false, ctx.Err()
			}
			return routing.MatchBinary(ctx, core, rule.SetPath, req)
		}); ok {
			if action == "reject" {
				http.Error(w, "Request rejected by configuration", http.StatusForbidden)
				return
			}
			selected := *reverse
			if target := policies[action]; target != nil {
				selected.Transport = target
			} else if action == "direct" {
				selected.Transport = direct
			} else if forcePort > 0 {
				selected.Transport = forced
			}
			selected.ServeHTTP(w, r)
			return
		}
		reverse.ServeHTTP(w, r)
	})}
	go f.accept()
	go func() {
		_ = f.server.Serve(httpMuxListener{f})
		transport.CloseIdleConnections()
		direct.CloseIdleConnections()
		forced.CloseIdleConnections()
		for _, transport := range policies {
			transport.CloseIdleConnections()
		}
	}()
	return f, nil
}
func (f *proxyFront) relay(client net.Conn) {
	defer client.Close()
	if f.settings.Authentication || f.settings.AllowLAN {
		f.socks(client)
		return
	}
	up, err := net.DialTimeout("tcp", f.upstream, 3*time.Second)
	if err != nil {
		return
	}
	defer up.Close()
	go func() { _, _ = io.Copy(up, client); up.Close() }()
	_, _ = io.Copy(client, up)
}
func (f *proxyFront) Close() {
	f.stop()
	_ = f.server.Close()
	f.mu.Lock()
	conns := make([]net.Conn, 0, len(f.connections))
	for c := range f.connections {
		conns = append(conns, c)
	}
	f.mu.Unlock()
	for _, c := range conns {
		_ = c.Close()
	}
}
