// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package crc64

import (
	"io"
	"testing"
)

type test struct {
	out uint64
	in  string
}

var golden = []test{
	test{0x0, ""},
	test{0x3420000000000000, "a"},
	test{0x36c4200000000000, "ab"},
	test{0x3776c42000000000, "abc"},
	test{0x336776c420000000, "abcd"},
	test{0x32d36776c4200000, "abcde"},
	test{0x3002d36776c42000, "abcdef"},
	test{0x31b002d36776c420, "abcdefg"},
	test{0xe21b002d36776c4, "abcdefgh"},
	test{0x8b6e21b002d36776, "abcdefghi"},
	test{0x7f5b6e21b002d367, "abcdefghij"},
	test{0x8ec0e7c835bf9cdf, "Discard medicine more than two years old."},
	test{0xc7db1759e2be5ab4, "He who has a shady past knows that nice guys finish last."},
	test{0xfbf9d9603a6fa020, "I wouldn't marry him with a ten foot pole."},
	test{0xeafc4211a6daa0ef, "Free! Free!/A trip/to Mars/for 900/empty jars/Burma Shave"},
	test{0x3e05b21c7a4dc4da, "The days of the digital watch are numbered.  -Tom Stoppard"},
	test{0x5255866ad6ef28a6, "Nepal premier won't resign."},
	test{0x8a79895be1e9c361, "For every action there is an equal and opposite government program."},
	test{0x8878963a649d4916, "His money is twice tainted: 'taint yours and 'taint mine."},
	test{0xa7b9d53ea87eb82f, "There is no reason for any individual to have a computer in their home. -Ken Olsen, 1977"},
	test{0xdb6805c0966a2f9c, "It's a tiny change to the code and not completely disgusting. - Bob Manchek"},
	test{0xf3553c65dacdadd2, "size:  a.out:  bad magic"},
	test{0x9d5e034087a676b9, "The major problem is with sendmail.  -Mark Horton"},
	test{0xa6db2d7f8da96417, "Give me a rock, paper and scissors and I will move the world.  CCFestoon"},
	test{0x325e00cd2fe819f9, "If the enemy is within range, then so are you."},
	test{0x88c6600ce58ae4c6, "It's well we cannot hear the screams/That we create in others' dreams."},
	test{0x28c4a3f3b769e078, "You remind me of a TV show, but that's all right: I watch it anyway."},
	test{0xa698a34c9d9f1dca, "C is as portable as Stonehedge!!"},
	test{0xf6c1e2a8c26c5cfc, "Even if I could be Shakespeare, I think I should still choose to be Faraday. - A. Huxley"},
	test{0xd402559dfe9b70c, "The fugacity of a constituent in a mixture of gases at a given temperature is proportional to its mole fraction.  Lewis-Randall Rule"},
	test{0xdb6efff26aa94946, "How can you write a big system without C++?  -Paul Glick"},
}

var tab = MakeTable(ISO)

func TestGolden(t *testing.T) {
	for i := 0; i < len(golden); i++ {
		g := golden[i]
		c := New(tab)
		io.WriteString(c, g.in)
		s := c.Sum64()
		if s != g.out {
			t.Errorf("crc64(%s) = 0x%x want 0x%x", g.in, s, g.out)
			t.FailNow()
		}
	}
}

func BenchmarkCrc64KB(b *testing.B) {
	b.StopTimer()
	data := make([]uint8, 1024)
	for i := 0; i < 1024; i++ {
		data[i] = uint8(i)
	}
	c := New(tab)
	b.StartTimer()

	for i := 0; i < b.N; i++ {
		c.Write(data)
	}
}
