// $G $D/$F.go && $L $F.$A && ./$A.out

// Copyright 2009 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

var (
	nf int
	x, y, z = f(), f(), f()
	m = map[string]string{"a":"A"}
	a, aok = m["a"]
	b, bok = m["b"]
)

func look(s string) (string, bool) {
	x, ok := m[s]
	return x, ok
}

func f() int {
	nf++
	return nf
}

func main() {
	if nf != 3 || x != 1 || y != 2 || z != 3 {
		panic("nf=", nf, " x=", x, " y=", y)
	}
	if a != "A" || aok != true || b != "" || bok != false {
		panic("a=", a, " aok=", aok, " b=", b, " bok=", bok)
	}
}
