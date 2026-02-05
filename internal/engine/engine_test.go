package engine

import (
	"context"
	"net"
	"net/netip"
	"strconv"
	"testing"
	"time"
)

func TestProbeCandidate(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			_ = c.Close()
		}
	}()

	addr := ln.Addr().String()
	_, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatal(err)
	}

	ip := netip.MustParseAddr("127.0.0.1")
	st := ProbeCandidate(context.Background(), ip, port, 500*time.Millisecond, 2)
	if st.Successes == 0 {
		t.Fatalf("expected success, got failures=%d last=%s", st.Failures, st.LastError)
	}
}
