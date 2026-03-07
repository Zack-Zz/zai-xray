package proxy

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"strings"
	"testing"
	"time"
)

func TestRecorder_CONNECTTunnel(t *testing.T) {
	t.Parallel()

	upstreamLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		if strings.Contains(err.Error(), "operation not permitted") {
			t.Skipf("sandbox does not allow local listener in this run: %v", err)
		}
		t.Fatalf("listen upstream failed: %v", err)
	}
	defer upstreamLn.Close()

	go func() {
		for {
			conn, err := upstreamLn.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				_, _ = io.Copy(c, c)
			}(conn)
		}
	}()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rec := NewRecorder()
	proxyAddr, err := rec.Start(ctx)
	if err != nil {
		t.Fatalf("start proxy failed: %v", err)
	}
	if err := WaitForProxyReady(proxyAddr, 2*time.Second); err != nil {
		t.Fatalf("proxy not ready: %v", err)
	}

	conn, err := net.DialTimeout("tcp", proxyAddr, 2*time.Second)
	if err != nil {
		t.Fatalf("dial proxy failed: %v", err)
	}
	defer conn.Close()

	upstreamAddr := upstreamLn.Addr().String()
	connectReq := fmt.Sprintf("CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n", upstreamAddr, upstreamAddr)
	if _, err := conn.Write([]byte(connectReq)); err != nil {
		t.Fatalf("write CONNECT failed: %v", err)
	}

	br := bufio.NewReader(conn)
	line, err := br.ReadString('\n')
	if err != nil {
		t.Fatalf("read CONNECT response failed: %v", err)
	}
	if !strings.Contains(line, "200") {
		t.Fatalf("unexpected CONNECT status line: %s", line)
	}
	for {
		l, err := br.ReadString('\n')
		if err != nil {
			t.Fatalf("read CONNECT headers failed: %v", err)
		}
		if l == "\r\n" {
			break
		}
	}

	payload := []byte("ping-via-connect")
	if _, err := conn.Write(payload); err != nil {
		t.Fatalf("write payload failed: %v", err)
	}
	buf := make([]byte, len(payload))
	if _, err := io.ReadFull(br, buf); err != nil {
		t.Fatalf("read echoed payload failed: %v", err)
	}
	if string(buf) != string(payload) {
		t.Fatalf("payload mismatch: got=%q want=%q", string(buf), string(payload))
	}

	_ = conn.Close()
	time.Sleep(100 * time.Millisecond)

	stats := rec.Stats()
	if stats.HTTPStatus != 200 {
		t.Fatalf("unexpected status: %d", stats.HTTPStatus)
	}
	if stats.RequestBytes == 0 || stats.ResponseBytes == 0 {
		t.Fatalf("expected byte counts > 0, got req=%d resp=%d", stats.RequestBytes, stats.ResponseBytes)
	}
}
