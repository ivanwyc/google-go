// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package net

import (
	"os"
	"testing"
	"time"
)

func testTimeout(t *testing.T, network, addr string, readFrom bool) {
	fd, err := Dial(network, "", addr)
	if err != nil {
		t.Errorf("dial %s %s failed: %v", network, addr, err)
		return
	}
	defer fd.Close()
	t0 := time.Nanoseconds()
	fd.SetReadTimeout(1e8) // 100ms
	var b [100]byte
	var n int
	var err1 os.Error
	if readFrom {
		n, _, err1 = fd.(PacketConn).ReadFrom(&b)
	} else {
		n, err1 = fd.Read(&b)
	}
	t1 := time.Nanoseconds()
	what := "Read"
	if readFrom {
		what = "ReadFrom"
	}
	if n != 0 || !isEAGAIN(err1) {
		t.Errorf("fd.%s on %s %s did not return 0, EAGAIN: %v, %v", what, network, addr, n, err1)
	}
	if t1-t0 < 0.5e8 || t1-t0 > 1.5e8 {
		t.Errorf("fd.%s on %s %s took %f seconds, expected 0.1", what, network, addr, float64(t1-t0)/1e9)
	}
}

func TestTimeoutUDP(t *testing.T) {
	testTimeout(t, "udp", "127.0.0.1:53", false)
	testTimeout(t, "udp", "127.0.0.1:53", true)
}

func TestTimeoutTCP(t *testing.T) {
	// 74.125.19.99 is www.google.com.
	// could use dns, but dns depends on
	// timeouts and this is the timeout test.
	testTimeout(t, "tcp", "74.125.19.99:80", false)
}
