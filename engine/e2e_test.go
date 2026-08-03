package engine

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"testing"
	"time"

	"golang.org/x/net/proxy"
)

// startEchoServer returns a TCP server that echoes any bytes it receives.
func startEchoServer(t *testing.T) net.Listener {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				_, _ = io.Copy(c, c)
			}(conn)
		}
	}()
	return l
}

// startSocks5Server runs a minimal no-auth SOCKS5 CONNECT server forwarding to
// the echo listener.
func startSocks5Server(t *testing.T, targetAddr string) net.Listener {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			go handleSocks5(conn, targetAddr)
		}
	}()
	return l
}

func handleSocks5(conn net.Conn, targetAddr string) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))

	buf := make([]byte, 1)
	if _, err := io.ReadFull(conn, buf); err != nil || buf[0] != 0x05 {
		return
	}
	var nmethods [1]byte
	if _, err := io.ReadFull(conn, nmethods[:]); err != nil {
		return
	}
	methods := make([]byte, int(nmethods[0]))
	if _, err := io.ReadFull(conn, methods); err != nil {
		return
	}
	if _, err := conn.Write([]byte{0x05, 0x00}); err != nil { // no-auth
		return
	}

	header := make([]byte, 4)
	if _, err := io.ReadFull(conn, header); err != nil {
		return
	}
	var host string
	switch header[3] {
	case 0x01: // IPv4
		ip := make([]byte, 4)
		if _, err := io.ReadFull(conn, ip); err != nil {
			return
		}
		host = net.IP(ip).String()
	case 0x03: // domain
		var lenb [1]byte
		if _, err := io.ReadFull(conn, lenb[:]); err != nil {
			return
		}
		dom := make([]byte, int(lenb[0]))
		if _, err := io.ReadFull(conn, dom); err != nil {
			return
		}
		host = string(dom)
	default:
		return
	}
	var portb [2]byte
	if _, err := io.ReadFull(conn, portb[:]); err != nil {
		return
	}
	port := binary.BigEndian.Uint16(portb[:])

	target, err := net.DialTimeout("tcp", net.JoinHostPort(host, fmt.Sprint(port)), 5*time.Second)
	if err != nil {
		_, _ = conn.Write([]byte{0x05, 0x04, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
		return
	}
	defer target.Close()
	_, _ = conn.Write([]byte{0x05, 0x00, 0x00, 0x01, 0, 0, 0, 0, 0, 0})

	done := make(chan struct{}, 2)
	go func() { _, _ = io.Copy(target, conn); done <- struct{}{} }()
	go func() { _, _ = io.Copy(conn, target); done <- struct{}{} }()
	<-done
}

// TestEngineRoutesTraffic proves the full pipeline: mixed inbound -> router ->
// socks outbound -> upstream SOCKS5 server -> echo target.
func TestEngineRoutesTraffic(t *testing.T) {
	echo := startEchoServer(t)
	socks := startSocks5Server(t, echo.Addr().String())

	enginePort, err := freePort()
	if err != nil {
		t.Fatal(err)
	}
	eng := New(nil)
	defer eng.Close()
	if err := eng.Start(Options{
		Mode:          ModeProxy,
		LogLevel:      LevelError,
		CacheFilePath: t.TempDir() + "/cache.db",
		Outbound: Outbound{
			Protocol: ProtocolSOCKS,
			Address:  "127.0.0.1",
			Port:     uint16(socks.Addr().(*net.TCPAddr).Port),
		},
		Proxy: ProxyOptions{Listen: "127.0.0.1", Port: enginePort},
	}); err != nil {
		t.Fatalf("start: %v", err)
	}

	client, err := proxy.SOCKS5("tcp", net.JoinHostPort("127.0.0.1", fmt.Sprint(enginePort)), nil, &net.Dialer{})
	if err != nil {
		t.Fatalf("socks5 client: %v", err)
	}
	conn, err := client.Dial("tcp", echo.Addr().String())
	if err != nil {
		t.Fatalf("dial through engine: %v", err)
	}
	defer conn.Close()

	payload := []byte("omniproxy-engine-roundtrip")
	if _, err := conn.Write(payload); err != nil {
		t.Fatal(err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	reply := make([]byte, len(payload))
	if _, err := io.ReadFull(conn, reply); err != nil {
		t.Fatalf("read reply: %v", err)
	}
	if string(reply) != string(payload) {
		t.Fatalf("echo mismatch: got %q want %q", reply, payload)
	}
}
