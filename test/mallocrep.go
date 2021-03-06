// $G $D/$F.go && $L $F.$A && ./$A.out

// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Repeated malloc test.

package main

import (
	"flag"
	"runtime"
)

var chatty = flag.Bool("v", false, "chatty")

var oldsys uint64

func bigger() {
	if st := runtime.MemStats; oldsys < st.Sys {
		oldsys = st.Sys
		if *chatty {
			println(st.Sys, " system bytes for ", st.Alloc, " Go bytes")
		}
		if st.Sys > 1e9 {
			panicln("too big")
		}
	}
}

func main() {
	flag.Parse()
	runtime.MemStats.Alloc = 0 // ignore stacks
	for i := 0; i < 1<<7; i++ {
		for j := 1; j <= 1<<22; j <<= 1 {
			if i == 0 && *chatty {
				println("First alloc:", j)
			}
			if a := runtime.MemStats.Alloc; a != 0 {
				panicln("no allocations but stats report", a, "bytes allocated")
			}
			b := runtime.Alloc(uintptr(j))
			during := runtime.MemStats.Alloc
			runtime.Free(b)
			if a := runtime.MemStats.Alloc; a != 0 {
				panic("allocated ", j, ": wrong stats: during=", during, " after=", a, " (want 0)")
			}
			bigger()
		}
		if i%(1<<10) == 0 && *chatty {
			println(i)
		}
		if i == 0 {
			if *chatty {
				println("Primed", i)
			}
			//	runtime.frozen = true;
		}
	}
}
