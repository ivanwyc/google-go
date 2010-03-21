// $G $D/$F.go && $L $F.$A && ./$A.out

// Copyright 2010 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import "unsafe"
import "reflect"

const (
	R = 5
	I = 6i

	C1 = R + I // ADD(5,6)
)

var complexBits = reflect.Typeof(complex(0i)).Size() * 8

func main() {
	c0 := C1
	c0 = (c0 + c0 + c0) / (c0 + c0 + 3i)
	println(c0)

	c := *(*complex)(unsafe.Pointer(&c0))
	println(c)

	println(complexBits)

	var a interface{}
	switch c := reflect.NewValue(a).(type) {
	case *reflect.Complex64Value:
		v := c.Get()
		_, _ = complex64(v), true
	case *reflect.ComplexValue:
		if complexBits == 64 {
			v := c.Get()
			_, _ = complex64(v), true
		}
	}
}
