package engine

import (
	"bytes"
	"crypto/subtle"
	"io"
	"net"
	"time"

	M "github.com/sagernet/sing/common/metadata"
	"github.com/sagernet/sing/common/varbin"
	"github.com/sagernet/sing/protocol/socks"
	"github.com/sagernet/sing/protocol/socks/socks5"
)

func (f *proxyFront) socks(client net.Conn) {
	client.SetDeadline(time.Now().Add(15 * time.Second))
	r := varbin.StubReader(client)
	auth, err := socks5.ReadAuthRequest(r)
	if err != nil {
		return
	}
	method := byte(0)
	if f.settings.Authentication {
		method = 2
	}
	if !bytes.Contains(auth.Methods, []byte{method}) {
		socks5.WriteAuthResponse(client, socks5.AuthResponse{Method: 255})
		return
	}
	if socks5.WriteAuthResponse(client, socks5.AuthResponse{Method: method}) != nil {
		return
	}
	if method == 2 {
		credentials, err := socks5.ReadUsernamePasswordAuthRequest(r)
		valid := err == nil && subtle.ConstantTimeCompare([]byte(credentials.Username), []byte(f.settings.Username)) == 1 && subtle.ConstantTimeCompare([]byte(credentials.Password), []byte(f.settings.Password)) == 1
		status := byte(1)
		if valid {
			status = 0
		}
		if socks5.WriteUsernamePasswordAuthResponse(client, socks5.UsernamePasswordAuthResponse{Status: status}) != nil || !valid {
			return
		}
	}
	req, err := socks5.ReadRequest(r)
	if err != nil {
		return
	}
	if req.Command != socks5.CommandConnect && req.Command != socks5.CommandUDPAssociate {
		socks5.WriteResponse(client, socks5.Response{ReplyCode: 7})
		return
	}
	up, err := net.DialTimeout("tcp", f.upstream, 3*time.Second)
	if err != nil {
		socks5.WriteResponse(client, socks5.Response{ReplyCode: 5})
		return
	}
	defer up.Close()
	up.SetDeadline(time.Now().Add(15 * time.Second))
	response, err := socks.ClientHandshake5(up, req.Command, req.Destination, "", "")
	if err != nil {
		socks5.WriteResponse(client, socks5.Response{ReplyCode: 1})
		return
	}
	if req.Command == socks5.CommandUDPAssociate {
		f.socksUDP(client, up, response.Bind)
		return
	}
	if socks5.WriteResponse(client, response) != nil {
		return
	}
	client.SetDeadline(time.Time{})
	up.SetDeadline(time.Time{})
	go func() { io.Copy(up, client); up.Close() }()
	io.Copy(client, up)
}

// UDP associations are private to the authenticated TCP peer and end with it.
func (f *proxyFront) socksUDP(client, control net.Conn, bind M.Socksaddr) {
	remote, err := net.ResolveUDPAddr("udp", bind.String())
	if err != nil {
		return
	}
	if remote.IP.IsUnspecified() {
		remote.IP = net.IPv4(127, 0, 0, 1)
	}
	up, err := net.DialUDP("udp", nil, remote)
	if err != nil {
		return
	}
	defer up.Close()
	local, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP(f.settings.ListenHost())})
	if err != nil {
		return
	}
	defer local.Close()
	address := local.LocalAddr().(*net.UDPAddr)
	address.IP = client.LocalAddr().(*net.TCPAddr).IP
	if socks5.WriteResponse(client, socks5.Response{Bind: M.SocksaddrFromNet(address)}) != nil {
		return
	}
	client.SetDeadline(time.Time{})
	control.SetDeadline(time.Time{})
	peerIP := client.RemoteAddr().(*net.TCPAddr).IP
	go func() { io.Copy(io.Discard, client); local.Close(); up.Close(); control.Close() }()
	go func() { io.Copy(io.Discard, control); local.Close(); up.Close(); client.Close() }()
	// Pin the first source port; reject datagrams from any other peer.
	var peer *net.UDPAddr
	first := make([]byte, 65535)
	for peer == nil {
		n, from, err := local.ReadFromUDP(first)
		if err != nil {
			return
		}
		if !from.IP.Equal(peerIP) {
			continue
		}
		peer = from
		if _, err = up.Write(first[:n]); err != nil {
			return
		}
	}
	go func() {
		buffer := make([]byte, 65535)
		for {
			n, from, err := local.ReadFromUDP(buffer)
			if err != nil {
				return
			}
			if from.IP.Equal(peer.IP) && from.Port == peer.Port {
				if _, err = up.Write(buffer[:n]); err != nil {
					return
				}
			}
		}
	}()
	buffer := first
	for {
		n, err := up.Read(buffer)
		if err != nil {
			return
		}
		if _, err = local.WriteToUDP(buffer[:n], peer); err != nil {
			return
		}
	}
}
