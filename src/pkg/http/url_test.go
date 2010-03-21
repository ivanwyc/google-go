// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package http

import (
	"fmt"
	"os"
	"reflect"
	"testing"
)

// TODO(rsc):
//	test URLUnescape
//	test URLEscape
//	test ParseURL

type URLTest struct {
	in        string
	out       *URL
	roundtrip string // expected result of reserializing the URL; empty means same as "in".
}

var urltests = []URLTest{
	// no path
	URLTest{
		"http://www.google.com",
		&URL{
			"http://www.google.com",
			"http", "//www.google.com",
			"www.google.com", "", "www.google.com",
			"", "", "",
		},
		"",
	},
	// path
	URLTest{
		"http://www.google.com/",
		&URL{
			"http://www.google.com/",
			"http", "//www.google.com/",
			"www.google.com", "", "www.google.com",
			"/", "", "",
		},
		"",
	},
	// path with hex escaping... note that space roundtrips to +
	URLTest{
		"http://www.google.com/file%20one%26two",
		&URL{
			"http://www.google.com/file%20one%26two",
			"http", "//www.google.com/file%20one%26two",
			"www.google.com", "", "www.google.com",
			"/file one&two", "", "",
		},
		"http://www.google.com/file%20one%26two",
	},
	// user
	URLTest{
		"ftp://webmaster@www.google.com/",
		&URL{
			"ftp://webmaster@www.google.com/",
			"ftp", "//webmaster@www.google.com/",
			"webmaster@www.google.com", "webmaster", "www.google.com",
			"/", "", "",
		},
		"",
	},
	// escape sequence in username
	URLTest{
		"ftp://john%20doe@www.google.com/",
		&URL{
			"ftp://john%20doe@www.google.com/",
			"ftp", "//john%20doe@www.google.com/",
			"john doe@www.google.com", "john doe", "www.google.com",
			"/", "", "",
		},
		"ftp://john%20doe@www.google.com/",
	},
	// query
	URLTest{
		"http://www.google.com/?q=go+language",
		&URL{
			"http://www.google.com/?q=go+language",
			"http", "//www.google.com/?q=go+language",
			"www.google.com", "", "www.google.com",
			"/", "q=go+language", "",
		},
		"",
	},
	// query with hex escaping: NOT parsed
	URLTest{
		"http://www.google.com/?q=go%20language",
		&URL{
			"http://www.google.com/?q=go%20language",
			"http", "//www.google.com/?q=go%20language",
			"www.google.com", "", "www.google.com",
			"/", "q=go%20language", "",
		},
		"",
	},
	// %20 outside query
	URLTest{
		"http://www.google.com/a%20b?q=c+d",
		&URL{
			"http://www.google.com/a%20b?q=c+d",
			"http", "//www.google.com/a%20b?q=c+d",
			"www.google.com", "", "www.google.com",
			"/a b", "q=c+d", "",
		},
		"",
	},
	// path without /, so no query parsing
	URLTest{
		"http:www.google.com/?q=go+language",
		&URL{
			"http:www.google.com/?q=go+language",
			"http", "www.google.com/?q=go+language",
			"", "", "",
			"www.google.com/?q=go+language", "", "",
		},
		"http:www.google.com/%3fq%3dgo%2blanguage",
	},
	// non-authority
	URLTest{
		"mailto:/webmaster@golang.org",
		&URL{
			"mailto:/webmaster@golang.org",
			"mailto", "/webmaster@golang.org",
			"", "", "",
			"/webmaster@golang.org", "", "",
		},
		"",
	},
	// non-authority
	URLTest{
		"mailto:webmaster@golang.org",
		&URL{
			"mailto:webmaster@golang.org",
			"mailto", "webmaster@golang.org",
			"", "", "",
			"webmaster@golang.org", "", "",
		},
		"",
	},
	// unescaped :// in query should not create a scheme
	URLTest{
		"/foo?query=http://bad",
		&URL{
			"/foo?query=http://bad",
			"", "/foo?query=http://bad",
			"", "", "",
			"/foo", "query=http://bad", "",
		},
		"",
	},
}

var urlnofragtests = []URLTest{
	URLTest{
		"http://www.google.com/?q=go+language#foo",
		&URL{
			"http://www.google.com/?q=go+language#foo",
			"http", "//www.google.com/?q=go+language#foo",
			"www.google.com", "", "www.google.com",
			"/", "q=go+language#foo", "",
		},
		"",
	},
}

var urlfragtests = []URLTest{
	URLTest{
		"http://www.google.com/?q=go+language#foo",
		&URL{
			"http://www.google.com/?q=go+language",
			"http", "//www.google.com/?q=go+language",
			"www.google.com", "", "www.google.com",
			"/", "q=go+language", "foo",
		},
		"",
	},
	URLTest{
		"http://www.google.com/?q=go+language#foo%26bar",
		&URL{
			"http://www.google.com/?q=go+language",
			"http", "//www.google.com/?q=go+language",
			"www.google.com", "", "www.google.com",
			"/", "q=go+language", "foo&bar",
		},
		"",
	},
}

// more useful string for debugging than fmt's struct printer
func ufmt(u *URL) string {
	return fmt.Sprintf("%q, %q, %q, %q, %q, %q, %q, %q, %q",
		u.Raw, u.Scheme, u.RawPath, u.Authority, u.Userinfo,
		u.Host, u.Path, u.RawQuery, u.Fragment)
}

func DoTest(t *testing.T, parse func(string) (*URL, os.Error), name string, tests []URLTest) {
	for _, tt := range tests {
		u, err := parse(tt.in)
		if err != nil {
			t.Errorf("%s(%q) returned error %s", name, tt.in, err)
			continue
		}
		if !reflect.DeepEqual(u, tt.out) {
			t.Errorf("%s(%q):\n\thave %v\n\twant %v\n",
				name, tt.in, ufmt(u), ufmt(tt.out))
		}
	}
}

func TestParseURL(t *testing.T) {
	DoTest(t, ParseURL, "ParseURL", urltests)
	DoTest(t, ParseURL, "ParseURL", urlnofragtests)
}

func TestParseURLReference(t *testing.T) {
	DoTest(t, ParseURLReference, "ParseURLReference", urltests)
	DoTest(t, ParseURLReference, "ParseURLReference", urlfragtests)
}

func DoTestString(t *testing.T, parse func(string) (*URL, os.Error), name string, tests []URLTest) {
	for _, tt := range tests {
		u, err := parse(tt.in)
		if err != nil {
			t.Errorf("%s(%q) returned error %s", name, tt.in, err)
			continue
		}
		s := u.String()
		expected := tt.in
		if len(tt.roundtrip) > 0 {
			expected = tt.roundtrip
		}
		if s != expected {
			t.Errorf("%s(%q).String() == %q (expected %q)", name, tt.in, s, expected)
		}
	}
}

func TestURLString(t *testing.T) {
	DoTestString(t, ParseURL, "ParseURL", urltests)
	DoTestString(t, ParseURL, "ParseURL", urlfragtests)
	DoTestString(t, ParseURL, "ParseURL", urlnofragtests)
	DoTestString(t, ParseURLReference, "ParseURLReference", urltests)
	DoTestString(t, ParseURLReference, "ParseURLReference", urlfragtests)
	DoTestString(t, ParseURLReference, "ParseURLReference", urlnofragtests)
}

type URLEscapeTest struct {
	in  string
	out string
	err os.Error
}

var unescapeTests = []URLEscapeTest{
	URLEscapeTest{
		"",
		"",
		nil,
	},
	URLEscapeTest{
		"abc",
		"abc",
		nil,
	},
	URLEscapeTest{
		"1%41",
		"1A",
		nil,
	},
	URLEscapeTest{
		"1%41%42%43",
		"1ABC",
		nil,
	},
	URLEscapeTest{
		"%4a",
		"J",
		nil,
	},
	URLEscapeTest{
		"%6F",
		"o",
		nil,
	},
	URLEscapeTest{
		"%", // not enough characters after %
		"",
		URLEscapeError("%"),
	},
	URLEscapeTest{
		"%a", // not enough characters after %
		"",
		URLEscapeError("%a"),
	},
	URLEscapeTest{
		"%1", // not enough characters after %
		"",
		URLEscapeError("%1"),
	},
	URLEscapeTest{
		"123%45%6", // not enough characters after %
		"",
		URLEscapeError("%6"),
	},
	URLEscapeTest{
		"%zzzzz", // invalid hex digits
		"",
		URLEscapeError("%zz"),
	},
}

func TestURLUnescape(t *testing.T) {
	for _, tt := range unescapeTests {
		actual, err := URLUnescape(tt.in)
		if actual != tt.out || (err != nil) != (tt.err != nil) {
			t.Errorf("URLUnescape(%q) = %q, %s; want %q, %s", tt.in, actual, err, tt.out, tt.err)
		}
	}
}

var escapeTests = []URLEscapeTest{
	URLEscapeTest{
		"",
		"",
		nil,
	},
	URLEscapeTest{
		"abc",
		"abc",
		nil,
	},
	URLEscapeTest{
		"one two",
		"one+two",
		nil,
	},
	URLEscapeTest{
		"10%",
		"10%25",
		nil,
	},
	URLEscapeTest{
		" ?&=#+%!<>#\"{}|\\^[]`☺\t",
		"+%3f%26%3d%23%2b%25!%3c%3e%23%22%7b%7d%7c%5c%5e%5b%5d%60%e2%98%ba%09",
		nil,
	},
}

func TestURLEscape(t *testing.T) {
	for _, tt := range escapeTests {
		actual := URLEscape(tt.in)
		if tt.out != actual {
			t.Errorf("URLEscape(%q) = %q, want %q", tt.in, actual, tt.out)
		}

		// for bonus points, verify that escape:unescape is an identity.
		roundtrip, err := URLUnescape(actual)
		if roundtrip != tt.in || err != nil {
			t.Errorf("URLUnescape(%q) = %q, %s; want %q, %s", actual, roundtrip, err, tt.in, "[no error]")
		}
	}
}

type CanonicalPathTest struct {
	in  string
	out string
}

var canonicalTests = []CanonicalPathTest{
	CanonicalPathTest{"", ""},
	CanonicalPathTest{"/", "/"},
	CanonicalPathTest{".", ""},
	CanonicalPathTest{"./", ""},
	CanonicalPathTest{"/a/", "/a/"},
	CanonicalPathTest{"a/", "a/"},
	CanonicalPathTest{"a/./", "a/"},
	CanonicalPathTest{"./a", "a"},
	CanonicalPathTest{"/a/../b", "/b"},
	CanonicalPathTest{"a/../b", "b"},
	CanonicalPathTest{"a/../../b", "../b"},
	CanonicalPathTest{"a/.", "a/"},
	CanonicalPathTest{"../.././a", "../../a"},
	CanonicalPathTest{"/../.././a", "/../../a"},
	CanonicalPathTest{"a/b/g/../..", "a/"},
	CanonicalPathTest{"a/b/..", "a/"},
	CanonicalPathTest{"a/b/.", "a/b/"},
	CanonicalPathTest{"a/b/../../../..", "../.."},
	CanonicalPathTest{"a./", "a./"},
	CanonicalPathTest{"/../a/b/../../../", "/../../"},
	CanonicalPathTest{"../a/b/../../../", "../../"},
}

func TestCanonicalPath(t *testing.T) {
	for _, tt := range canonicalTests {
		actual := CanonicalPath(tt.in)
		if tt.out != actual {
			t.Errorf("CanonicalPath(%q) = %q, want %q", tt.in, actual, tt.out)
		}
	}
}
