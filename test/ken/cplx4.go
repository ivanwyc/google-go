// $G $D/$F.go && $L $F.$A && ./$A.out

// Copyright 2010 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import "fmt"

const (
	R = 5
	I = 6i

	C1 = R + I // ADD(5,6)
)

func doprint(c complex) { fmt.Printf("c = %f\n", c) }

func main() {

	// constants
	fmt.Printf("c = %f\n", -C1)
	doprint(C1)

	// variables
	c1 := C1
	fmt.Printf("c = %f\n", c1)
	doprint(c1)

	// 128
	c2 := complex128(C1)
	fmt.Printf("c = %G\n", c2)

	// real, imag, cmplx
	c3 := cmplx(real(c2)+3, imag(c2)-5) + c2
	fmt.Printf("c = %G\n", c3)
}
