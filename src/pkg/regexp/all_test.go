// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package regexp

import (
	"os"
	"strings"
	"testing"
)

var good_re = []string{
	``,
	`.`,
	`^.$`,
	`a`,
	`a*`,
	`a+`,
	`a?`,
	`a|b`,
	`a*|b*`,
	`(a*|b)(c*|d)`,
	`[a-z]`,
	`[a-abc-c\-\]\[]`,
	`[a-z]+`,
	`[]`,
	`[abc]`,
	`[^1234]`,
	`[^\n]`,
}

type stringError struct {
	re  string
	err os.Error
}

var bad_re = []stringError{
	stringError{`*`, ErrBareClosure},
	stringError{`(abc`, ErrUnmatchedLpar},
	stringError{`abc)`, ErrUnmatchedRpar},
	stringError{`x[a-z`, ErrUnmatchedLbkt},
	stringError{`abc]`, ErrUnmatchedRbkt},
	stringError{`[z-a]`, ErrBadRange},
	stringError{`abc\`, ErrExtraneousBackslash},
	stringError{`a**`, ErrBadClosure},
	stringError{`a*+`, ErrBadClosure},
	stringError{`a??`, ErrBadClosure},
	stringError{`*`, ErrBareClosure},
	stringError{`\x`, ErrBadBackslash},
}

type vec []int

type tester struct {
	re    string
	text  string
	match vec
}

var matches = []tester{
	tester{`^abcdefg`, "abcdefg", vec{0, 7}},
	tester{`a+`, "baaab", vec{1, 4}},
	tester{"abcd..", "abcdef", vec{0, 6}},
	tester{``, "", vec{0, 0}},
	tester{`a`, "a", vec{0, 1}},
	tester{`x`, "y", vec{}},
	tester{`b`, "abc", vec{1, 2}},
	tester{`.`, "a", vec{0, 1}},
	tester{`.*`, "abcdef", vec{0, 6}},
	tester{`^`, "abcde", vec{0, 0}},
	tester{`$`, "abcde", vec{5, 5}},
	tester{`^abcd$`, "abcd", vec{0, 4}},
	tester{`^bcd'`, "abcdef", vec{}},
	tester{`^abcd$`, "abcde", vec{}},
	tester{`a+`, "baaab", vec{1, 4}},
	tester{`a*`, "baaab", vec{0, 0}},
	tester{`[a-z]+`, "abcd", vec{0, 4}},
	tester{`[^a-z]+`, "ab1234cd", vec{2, 6}},
	tester{`[a\-\]z]+`, "az]-bcz", vec{0, 4}},
	tester{`[^\n]+`, "abcd\n", vec{0, 4}},
	tester{`[日本語]+`, "日本語日本語", vec{0, 18}},
	tester{`日本語+`, "日本語", vec{0, 9}},
	tester{`日本語+`, "日本語語語語", vec{0, 18}},
	tester{`()`, "", vec{0, 0, 0, 0}},
	tester{`(a)`, "a", vec{0, 1, 0, 1}},
	tester{`(.)(.)`, "日a", vec{0, 4, 0, 3, 3, 4}},
	tester{`(.*)`, "", vec{0, 0, 0, 0}},
	tester{`(.*)`, "abcd", vec{0, 4, 0, 4}},
	tester{`(..)(..)`, "abcd", vec{0, 4, 0, 2, 2, 4}},
	tester{`(([^xyz]*)(d))`, "abcd", vec{0, 4, 0, 4, 0, 3, 3, 4}},
	tester{`((a|b|c)*(d))`, "abcd", vec{0, 4, 0, 4, 2, 3, 3, 4}},
	tester{`(((a|b|c)*)(d))`, "abcd", vec{0, 4, 0, 4, 0, 3, 2, 3, 3, 4}},
	tester{`a*(|(b))c*`, "aacc", vec{0, 4, 2, 2, -1, -1}},
	tester{`(.*).*`, "ab", vec{0, 2, 0, 2}},
	tester{`[.]`, ".", vec{0, 1}},
	tester{`/$`, "/abc/", vec{4, 5}},
	tester{`/$`, "/abc", vec{}},

	// fixed bugs
	tester{`ab$`, "cab", vec{1, 3}},
	tester{`axxb$`, "axxcb", vec{}},
}

func compileTest(t *testing.T, expr string, error os.Error) *Regexp {
	re, err := Compile(expr)
	if err != error {
		t.Error("compiling `", expr, "`; unexpected error: ", err.String())
	}
	return re
}

func printVec(t *testing.T, m []int) {
	l := len(m)
	if l == 0 {
		t.Log("\t<no match>")
	} else {
		if m[len(m)-1] == -1 {
			m = m[0 : len(m)-2]
		}
		t.Log("\t", m)
	}
}

func equal(m1, m2 []int) bool {
	l := len(m1)
	if l != len(m2) {
		return false
	}
	for i := 0; i < l; i++ {
		if m1[i] != m2[i] {
			return false
		}
	}
	return true
}

func equalStrings(m1, m2 []string) bool {
	l := len(m1)
	if l != len(m2) {
		return false
	}
	for i := 0; i < l; i++ {
		if m1[i] != m2[i] {
			return false
		}
	}
	return true
}

func executeTest(t *testing.T, expr string, str string, match []int) {
	re := compileTest(t, expr, nil)
	if re == nil {
		return
	}
	m := re.ExecuteString(str)
	if !equal(m, match) {
		t.Errorf("ExecuteString failure on %#q matching %q:", expr, str)
		printVec(t, m)
		t.Log("should be:")
		printVec(t, match)
	}
	// now try bytes
	m = re.Execute([]byte(str))
	if !equal(m, match) {
		t.Errorf("Execute failure on %#q matching %q:", expr, str)
		printVec(t, m)
		t.Log("should be:")
		printVec(t, match)
	}
}

func TestGoodCompile(t *testing.T) {
	for i := 0; i < len(good_re); i++ {
		compileTest(t, good_re[i], nil)
	}
}

func TestBadCompile(t *testing.T) {
	for i := 0; i < len(bad_re); i++ {
		compileTest(t, bad_re[i].re, bad_re[i].err)
	}
}

func TestExecute(t *testing.T) {
	for i := 0; i < len(matches); i++ {
		test := &matches[i]
		executeTest(t, test.re, test.text, test.match)
	}
}

func matchTest(t *testing.T, expr string, str string, match []int) {
	re := compileTest(t, expr, nil)
	if re == nil {
		return
	}
	m := re.MatchString(str)
	if m != (len(match) > 0) {
		t.Errorf("MatchString failure on %#q matching %q: %t should be %t", expr, str, m, len(match) > 0)
	}
	// now try bytes
	m = re.Match([]byte(str))
	if m != (len(match) > 0) {
		t.Errorf("Match failure on %#q matching %q: %t should be %t", expr, str, m, len(match) > 0)
	}
}

func TestMatch(t *testing.T) {
	for i := 0; i < len(matches); i++ {
		test := &matches[i]
		matchTest(t, test.re, test.text, test.match)
	}
}

func TestMatchStrings(t *testing.T) {
	for i := 0; i < len(matches); i++ {
		test := &matches[i]
		matchTest(t, test.re, test.text, test.match)
	}
}

func matchFunctionTest(t *testing.T, expr string, str string, match []int) {
	m, err := MatchString(expr, str)
	if err == nil {
		return
	}
	if m != (len(match) > 0) {
		t.Errorf("Match failure on %#q matching %q: %d should be %d", expr, str, m, len(match) > 0)
	}
}

func TestMatchFunction(t *testing.T) {
	for i := 0; i < len(matches); i++ {
		test := &matches[i]
		matchFunctionTest(t, test.re, test.text, test.match)
	}
}

type ReplaceTest struct {
	pattern, replacement, input, output string
}

var replaceTests = []ReplaceTest{
	// Test empty input and/or replacement, with pattern that matches the empty string.
	ReplaceTest{"", "", "", ""},
	ReplaceTest{"", "x", "", "x"},
	ReplaceTest{"", "", "abc", "abc"},
	ReplaceTest{"", "x", "abc", "xaxbxcx"},

	// Test empty input and/or replacement, with pattern that does not match the empty string.
	ReplaceTest{"b", "", "", ""},
	ReplaceTest{"b", "x", "", ""},
	ReplaceTest{"b", "", "abc", "ac"},
	ReplaceTest{"b", "x", "abc", "axc"},
	ReplaceTest{"y", "", "", ""},
	ReplaceTest{"y", "x", "", ""},
	ReplaceTest{"y", "", "abc", "abc"},
	ReplaceTest{"y", "x", "abc", "abc"},

	// Multibyte characters -- verify that we don't try to match in the middle
	// of a character.
	ReplaceTest{"[a-c]*", "x", "\u65e5", "x\u65e5x"},
	ReplaceTest{"[^\u65e5]", "x", "abc\u65e5def", "xxx\u65e5xxx"},

	// Start and end of a string.
	ReplaceTest{"^[a-c]*", "x", "abcdabc", "xdabc"},
	ReplaceTest{"[a-c]*$", "x", "abcdabc", "abcdx"},
	ReplaceTest{"^[a-c]*$", "x", "abcdabc", "abcdabc"},
	ReplaceTest{"^[a-c]*", "x", "abc", "x"},
	ReplaceTest{"[a-c]*$", "x", "abc", "x"},
	ReplaceTest{"^[a-c]*$", "x", "abc", "x"},
	ReplaceTest{"^[a-c]*", "x", "dabce", "xdabce"},
	ReplaceTest{"[a-c]*$", "x", "dabce", "dabcex"},
	ReplaceTest{"^[a-c]*$", "x", "dabce", "dabce"},
	ReplaceTest{"^[a-c]*", "x", "", "x"},
	ReplaceTest{"[a-c]*$", "x", "", "x"},
	ReplaceTest{"^[a-c]*$", "x", "", "x"},

	ReplaceTest{"^[a-c]+", "x", "abcdabc", "xdabc"},
	ReplaceTest{"[a-c]+$", "x", "abcdabc", "abcdx"},
	ReplaceTest{"^[a-c]+$", "x", "abcdabc", "abcdabc"},
	ReplaceTest{"^[a-c]+", "x", "abc", "x"},
	ReplaceTest{"[a-c]+$", "x", "abc", "x"},
	ReplaceTest{"^[a-c]+$", "x", "abc", "x"},
	ReplaceTest{"^[a-c]+", "x", "dabce", "dabce"},
	ReplaceTest{"[a-c]+$", "x", "dabce", "dabce"},
	ReplaceTest{"^[a-c]+$", "x", "dabce", "dabce"},
	ReplaceTest{"^[a-c]+", "x", "", ""},
	ReplaceTest{"[a-c]+$", "x", "", ""},
	ReplaceTest{"^[a-c]+$", "x", "", ""},

	// Other cases.
	ReplaceTest{"abc", "def", "abcdefg", "defdefg"},
	ReplaceTest{"bc", "BC", "abcbcdcdedef", "aBCBCdcdedef"},
	ReplaceTest{"abc", "", "abcdabc", "d"},
	ReplaceTest{"x", "xXx", "xxxXxxx", "xXxxXxxXxXxXxxXxxXx"},
	ReplaceTest{"abc", "d", "", ""},
	ReplaceTest{"abc", "d", "abc", "d"},
	ReplaceTest{".+", "x", "abc", "x"},
	ReplaceTest{"[a-c]*", "x", "def", "xdxexfx"},
	ReplaceTest{"[a-c]+", "x", "abcbcdcdedef", "xdxdedef"},
	ReplaceTest{"[a-c]*", "x", "abcbcdcdedef", "xdxdxexdxexfx"},
}

type ReplaceFuncTest struct {
	pattern       string
	replacement   func(string) string
	input, output string
}

var replaceFuncTests = []ReplaceFuncTest{
	ReplaceFuncTest{"[a-c]", func(s string) string { return "x" + s + "y" }, "defabcdef", "defxayxbyxcydef"},
	ReplaceFuncTest{"[a-c]+", func(s string) string { return "x" + s + "y" }, "defabcdef", "defxabcydef"},
	ReplaceFuncTest{"[a-c]*", func(s string) string { return "x" + s + "y" }, "defabcdef", "xydxyexyfxabcydxyexyfxy"},
}

func TestReplaceAll(t *testing.T) {
	for _, tc := range replaceTests {
		re, err := Compile(tc.pattern)
		if err != nil {
			t.Errorf("Unexpected error compiling %q: %v", tc.pattern, err)
			continue
		}
		actual := re.ReplaceAllString(tc.input, tc.replacement)
		if actual != tc.output {
			t.Errorf("%q.Replace(%q,%q) = %q; want %q",
				tc.pattern, tc.input, tc.replacement, actual, tc.output)
		}
		// now try bytes
		actual = string(re.ReplaceAll([]byte(tc.input), []byte(tc.replacement)))
		if actual != tc.output {
			t.Errorf("%q.Replace(%q,%q) = %q; want %q",
				tc.pattern, tc.input, tc.replacement, actual, tc.output)
		}
	}
}

func TestReplaceAllFunc(t *testing.T) {
	for _, tc := range replaceFuncTests {
		re, err := Compile(tc.pattern)
		if err != nil {
			t.Errorf("Unexpected error compiling %q: %v", tc.pattern, err)
			continue
		}
		actual := re.ReplaceAllStringFunc(tc.input, tc.replacement)
		if actual != tc.output {
			t.Errorf("%q.ReplaceFunc(%q,%q) = %q; want %q",
				tc.pattern, tc.input, tc.replacement, actual, tc.output)
		}
		// now try bytes
		actual = string(re.ReplaceAllFunc([]byte(tc.input), func(s []byte) []byte { return []byte(tc.replacement(string(s))) }))
		if actual != tc.output {
			t.Errorf("%q.ReplaceFunc(%q,%q) = %q; want %q",
				tc.pattern, tc.input, tc.replacement, actual, tc.output)
		}
	}
}

type QuoteMetaTest struct {
	pattern, output string
}

var quoteMetaTests = []QuoteMetaTest{
	QuoteMetaTest{``, ``},
	QuoteMetaTest{`foo`, `foo`},
	QuoteMetaTest{`!@#$%^&*()_+-=[{]}\|,<.>/?~`, `!@#\$%\^&\*\(\)_\+-=\[{\]}\\\|,<\.>/\?~`},
}

func TestQuoteMeta(t *testing.T) {
	for _, tc := range quoteMetaTests {
		// Verify that QuoteMeta returns the expected string.
		quoted := QuoteMeta(tc.pattern)
		if quoted != tc.output {
			t.Errorf("QuoteMeta(`%s`) = `%s`; want `%s`",
				tc.pattern, quoted, tc.output)
			continue
		}

		// Verify that the quoted string is in fact treated as expected
		// by Compile -- i.e. that it matches the original, unquoted string.
		if tc.pattern != "" {
			re, err := Compile(quoted)
			if err != nil {
				t.Errorf("Unexpected error compiling QuoteMeta(`%s`): %v", tc.pattern, err)
				continue
			}
			src := "abc" + tc.pattern + "def"
			repl := "xyz"
			replaced := re.ReplaceAllString(src, repl)
			expected := "abcxyzdef"
			if replaced != expected {
				t.Errorf("QuoteMeta(`%s`).Replace(`%s`,`%s`) = `%s`; want `%s`",
					tc.pattern, src, repl, replaced, expected)
			}
		}
	}
}

type matchCase struct {
	matchfunc string
	input     string
	n         int
	regexp    string
	expected  []string
}

var matchCases = []matchCase{
	matchCase{"match", " aa b", 0, "[^ ]+", []string{"aa", "b"}},
	matchCase{"match", " aa b", 0, "[^ ]*", []string{"", "aa", "b"}},
	matchCase{"match", "a b c", 0, "[^ ]*", []string{"a", "b", "c"}},
	matchCase{"match", "a:a: a:", 0, "^.:", []string{"a:"}},
	matchCase{"match", "", 0, "[^ ]*", []string{""}},
	matchCase{"match", "", 0, "", []string{""}},
	matchCase{"match", "a", 0, "", []string{"", ""}},
	matchCase{"match", "ab", 0, "^", []string{""}},
	matchCase{"match", "ab", 0, "$", []string{""}},
	matchCase{"match", "ab", 0, "X*", []string{"", "", ""}},
	matchCase{"match", "aX", 0, "X*", []string{"", "X"}},
	matchCase{"match", "XabX", 0, "X*", []string{"X", "", "X"}},

	matchCase{"matchit", "", 0, ".", []string{}},
	matchCase{"matchit", "abc", 2, ".", []string{"a", "b"}},
	matchCase{"matchit", "abc", 0, ".", []string{"a", "b", "c"}},
}

func printStringSlice(t *testing.T, s []string) {
	t.Logf("%#v", s)
}

func TestAllMatches(t *testing.T) {
	ch := make(chan matchCase)
	go func() {
		for _, c := range matchCases {
			ch <- c
			stringCase := matchCase{
				"string" + c.matchfunc,
				c.input,
				c.n,
				c.regexp,
				c.expected,
			}
			ch <- stringCase
		}
		close(ch)
	}()

	for c := range ch {
		var result []string
		re, _ := Compile(c.regexp)

		switch c.matchfunc {
		case "matchit":
			result = make([]string, len(c.input)+1)
			i := 0
			b := []byte(c.input)
			for match := range re.AllMatchesIter(b, c.n) {
				result[i] = string(match)
				i++
			}
			result = result[0:i]
		case "stringmatchit":
			result = make([]string, len(c.input)+1)
			i := 0
			for match := range re.AllMatchesStringIter(c.input, c.n) {
				result[i] = match
				i++
			}
			result = result[0:i]
		case "match":
			result = make([]string, len(c.input)+1)
			b := []byte(c.input)
			i := 0
			for _, match := range re.AllMatches(b, c.n) {
				result[i] = string(match)
				i++
			}
			result = result[0:i]
		case "stringmatch":
			result = re.AllMatchesString(c.input, c.n)
		}

		if !equalStrings(result, c.expected) {
			t.Errorf("testing '%s'.%s('%s', %d), expected: ",
				c.regexp, c.matchfunc, c.input, c.n)
			printStringSlice(t, c.expected)
			t.Log("got: ")
			printStringSlice(t, result)
			t.Log("\n")
		}
	}
}

type numSubexpCase struct {
	input    string
	expected int
}

var numSubexpCases = []numSubexpCase{
	numSubexpCase{``, 0},
	numSubexpCase{`.*`, 0},
	numSubexpCase{`abba`, 0},
	numSubexpCase{`ab(b)a`, 1},
	numSubexpCase{`ab(.*)a`, 1},
	numSubexpCase{`(.*)ab(.*)a`, 2},
	numSubexpCase{`(.*)(ab)(.*)a`, 3},
	numSubexpCase{`(.*)((a)b)(.*)a`, 4},
	numSubexpCase{`(.*)(\(ab)(.*)a`, 3},
	numSubexpCase{`(.*)(\(a\)b)(.*)a`, 3},
}

func TestNumSubexp(t *testing.T) {
	for _, c := range numSubexpCases {
		re, _ := Compile(c.input)
		n := re.NumSubexp()
		if n != c.expected {
			t.Errorf("NumSubexp for %q returned %d, expected %d", c.input, n, c.expected)
		}
	}
}

func BenchmarkLiteral(b *testing.B) {
	x := strings.Repeat("x", 50)
	b.StopTimer()
	re, _ := Compile(x)
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		if !re.MatchString(x) {
			println("no match!")
			break
		}
	}
}

func BenchmarkNotLiteral(b *testing.B) {
	x := strings.Repeat("x", 49)
	b.StopTimer()
	re, _ := Compile("^" + x)
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		if !re.MatchString(x) {
			println("no match!")
			break
		}
	}
}

func BenchmarkReplaceAll(b *testing.B) {
	x := "abcdefghijklmnopqrstuvwxyz"
	b.StopTimer()
	re, _ := Compile("[cjrw]")
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		re.ReplaceAllString(x, "")
	}
}
