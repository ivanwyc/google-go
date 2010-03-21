// Copyright 2009 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Annotate Crefs in Prog with C types by parsing gcc debug output.
// Conversion of debug output to Go types.

package main

import (
	"bytes"
	"debug/dwarf"
	"debug/elf"
	"debug/macho"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strconv"
	"strings"
)

func (p *Prog) loadDebugInfo() {
	var b bytes.Buffer

	b.WriteString(p.Preamble)
	stdout := p.gccPostProc(b.Bytes())
	defines := make(map[string]string)
	for _, line := range strings.Split(stdout, "\n", 0) {
		if len(line) < 9 || line[0:7] != "#define" {
			continue
		}

		line = strings.TrimSpace(line[8:])

		var key, val string
		spaceIndex := strings.Index(line, " ")
		tabIndex := strings.Index(line, "\t")

		if spaceIndex == -1 && tabIndex == -1 {
			continue
		} else if tabIndex == -1 || (spaceIndex != -1 && spaceIndex < tabIndex) {
			key = line[0:spaceIndex]
			val = strings.TrimSpace(line[spaceIndex:])
		} else {
			key = line[0:tabIndex]
			val = strings.TrimSpace(line[tabIndex:])
		}

		// Only allow string, character, and numeric constants. Ignoring #defines for
		// symbols allows those symbols to be referenced in Go, as they will be
		// translated by gcc later.
		_, err := strconv.Atoi(string(val[0]))
		if err == nil || val[0] == '\'' || val[0] == '"' {
			defines[key] = val
		}
	}

	// Construct a slice of unique names from p.Crefs.
	m := make(map[string]int)
	for _, c := range p.Crefs {
		// If we've already found this name as a define, it is not a Cref.
		if val, ok := defines[c.Name]; ok {
			_, err := parser.ParseExpr("", val, nil)
			if err != nil {
				fmt.Fprintf(os.Stderr, "The value in C.%s does not parse as a Go expression; cannot use.\n", c.Name)
				os.Exit(2)
			}

			c.Context = "const"
			c.TypeName = false
			p.Constdef[c.Name] = val
			continue
		}
		m[c.Name] = -1
	}
	names := make([]string, 0, len(m))
	for name, _ := range m {
		i := len(names)
		names = names[0 : i+1]
		names[i] = name
		m[name] = i
	}

	// Coerce gcc into telling us whether each name is
	// a type, a value, or undeclared.  We compile a function
	// containing the line:
	//	name;
	// If name is a type, gcc will print:
	//	x.c:2: warning: useless type name in empty declaration
	// If name is a value, gcc will print
	//	x.c:2: warning: statement with no effect
	// If name is undeclared, gcc will print
	//	x.c:2: error: 'name' undeclared (first use in this function)
	// A line number directive causes the line number to
	// correspond to the index in the names array.
	b.Reset()
	b.WriteString(p.Preamble)
	b.WriteString("void f(void) {\n")
	b.WriteString("#line 0 \"cgo-test\"\n")
	for _, n := range names {
		b.WriteString(n)
		b.WriteString(";\n")
	}
	b.WriteString("}\n")

	kind := make(map[string]string)
	_, stderr := p.gccDebug(b.Bytes())
	if stderr == "" {
		fatal("gcc produced no output")
	}
	for _, line := range strings.Split(stderr, "\n", 0) {
		if len(line) < 9 || line[0:9] != "cgo-test:" {
			continue
		}
		line = line[9:]
		colon := strings.Index(line, ":")
		if colon < 0 {
			continue
		}
		i, err := strconv.Atoi(line[0:colon])
		if err != nil {
			continue
		}
		what := ""
		switch {
		default:
			continue
		case strings.Index(line, ": useless type name in empty declaration") >= 0:
			what = "type"
		case strings.Index(line, ": statement with no effect") >= 0:
			what = "value"
		case strings.Index(line, "undeclared") >= 0:
			what = "error"
		}
		if old, ok := kind[names[i]]; ok && old != what {
			error(noPos, "inconsistent gcc output about C.%s", names[i])
		}
		kind[names[i]] = what
	}
	for _, n := range names {
		if _, ok := kind[n]; !ok {
			error(noPos, "could not determine kind of name for C.%s", n)
		}
	}

	if nerrors > 0 {
		fatal("failed to interpret gcc output:\n%s", stderr)
	}

	// Extract the types from the DWARF section of an object
	// from a well-formed C program.  Gcc only generates DWARF info
	// for symbols in the object file, so it is not enough to print the
	// preamble and hope the symbols we care about will be there.
	// Instead, emit
	//	typeof(names[i]) *__cgo__i;
	// for each entry in names and then dereference the type we
	// learn for __cgo__i.
	b.Reset()
	b.WriteString(p.Preamble)
	for i, n := range names {
		fmt.Fprintf(&b, "typeof(%s) *__cgo__%d;\n", n, i)
	}
	d, stderr := p.gccDebug(b.Bytes())
	if d == nil {
		fatal("gcc failed:\n%s\non input:\n%s", stderr, b.Bytes())
	}

	// Scan DWARF info for top-level TagVariable entries with AttrName __cgo__i.
	types := make([]dwarf.Type, len(names))
	enums := make([]dwarf.Offset, len(names))
	r := d.Reader()
	for {
		e, err := r.Next()
		if err != nil {
			fatal("reading DWARF entry: %s", err)
		}
		if e == nil {
			break
		}
		switch e.Tag {
		case dwarf.TagEnumerationType:
			offset := e.Offset
			for {
				e, err := r.Next()
				if err != nil {
					fatal("reading DWARF entry: %s", err)
				}
				if e.Tag == 0 {
					break
				}
				if e.Tag == dwarf.TagEnumerator {
					entryName := e.Val(dwarf.AttrName).(string)
					i, ok := m[entryName]
					if ok {
						enums[i] = offset
					}
				}
			}
		case dwarf.TagVariable:
			name, _ := e.Val(dwarf.AttrName).(string)
			typOff, _ := e.Val(dwarf.AttrType).(dwarf.Offset)
			if name == "" || typOff == 0 {
				fatal("malformed DWARF TagVariable entry")
			}
			if !strings.HasPrefix(name, "__cgo__") {
				break
			}
			typ, err := d.Type(typOff)
			if err != nil {
				fatal("loading DWARF type: %s", err)
			}
			t, ok := typ.(*dwarf.PtrType)
			if !ok || t == nil {
				fatal("internal error: %s has non-pointer type", name)
			}
			i, err := strconv.Atoi(name[7:])
			if err != nil {
				fatal("malformed __cgo__ name: %s", name)
			}
			if enums[i] != 0 {
				t, err := d.Type(enums[i])
				if err != nil {
					fatal("loading DWARF type: %s", err)
				}
				types[i] = t
			} else {
				types[i] = t.Type
			}
		}
		if e.Tag != dwarf.TagCompileUnit {
			r.SkipChildren()
		}
	}

	// Record types and typedef information in Crefs.
	var conv typeConv
	conv.Init(p.PtrSize)
	for _, c := range p.Crefs {
		i, ok := m[c.Name]
		if !ok {
			if _, ok := p.Constdef[c.Name]; !ok {
				fatal("Cref %s is no longer around", c.Name)
			}
			continue
		}
		c.TypeName = kind[c.Name] == "type"
		f, fok := types[i].(*dwarf.FuncType)
		if c.Context == "call" && !c.TypeName && fok {
			c.FuncType = conv.FuncType(f)
		} else {
			c.Type = conv.Type(types[i])
		}
	}
	p.Typedef = conv.typedef
}

func concat(a, b []string) []string {
	c := make([]string, len(a)+len(b))
	for i, s := range a {
		c[i] = s
	}
	for i, s := range b {
		c[i+len(a)] = s
	}
	return c
}

// gccDebug runs gcc -gdwarf-2 over the C program stdin and
// returns the corresponding DWARF data and any messages
// printed to standard error.
func (p *Prog) gccDebug(stdin []byte) (*dwarf.Data, string) {
	machine := "-m32"
	if p.PtrSize == 8 {
		machine = "-m64"
	}

	tmp := "_cgo_.o"
	base := []string{
		"gcc",
		machine,
		"-Wall",                             // many warnings
		"-Werror",                           // warnings are errors
		"-o" + tmp,                          // write object to tmp
		"-gdwarf-2",                         // generate DWARF v2 debugging symbols
		"-fno-eliminate-unused-debug-types", // gets rid of e.g. untyped enum otherwise
		"-c",                                // do not link
		"-xc",                               // input language is C
		"-",                                 // read input from standard input
	}
	_, stderr, ok := run(stdin, concat(base, p.GccOptions))
	if !ok {
		return nil, string(stderr)
	}

	// Try to parse f as ELF and Mach-O and hope one works.
	var f interface {
		DWARF() (*dwarf.Data, os.Error)
	}
	var err os.Error
	if f, err = elf.Open(tmp); err != nil {
		if f, err = macho.Open(tmp); err != nil {
			fatal("cannot parse gcc output %s as ELF or Mach-O object", tmp)
		}
	}

	d, err := f.DWARF()
	if err != nil {
		fatal("cannot load DWARF debug information from %s: %s", tmp, err)
	}
	return d, ""
}

func (p *Prog) gccPostProc(stdin []byte) string {
	machine := "-m32"
	if p.PtrSize == 8 {
		machine = "-m64"
	}

	base := []string{"gcc", machine, "-E", "-dM", "-xc", "-"}
	stdout, stderr, ok := run(stdin, concat(base, p.GccOptions))
	if !ok {
		return string(stderr)
	}

	return string(stdout)
}

// A typeConv is a translator from dwarf types to Go types
// with equivalent memory layout.
type typeConv struct {
	// Cache of already-translated or in-progress types.
	m       map[dwarf.Type]*Type
	typedef map[string]ast.Expr

	// Predeclared types.
	bool                                   ast.Expr
	byte                                   ast.Expr // denotes padding
	int8, int16, int32, int64              ast.Expr
	uint8, uint16, uint32, uint64, uintptr ast.Expr
	float32, float64                       ast.Expr
	void                                   ast.Expr
	unsafePointer                          ast.Expr
	string                                 ast.Expr

	ptrSize int64

	tagGen int
}

func (c *typeConv) Init(ptrSize int64) {
	c.ptrSize = ptrSize
	c.m = make(map[dwarf.Type]*Type)
	c.typedef = make(map[string]ast.Expr)
	c.bool = c.Ident("bool")
	c.byte = c.Ident("byte")
	c.int8 = c.Ident("int8")
	c.int16 = c.Ident("int16")
	c.int32 = c.Ident("int32")
	c.int64 = c.Ident("int64")
	c.uint8 = c.Ident("uint8")
	c.uint16 = c.Ident("uint16")
	c.uint32 = c.Ident("uint32")
	c.uint64 = c.Ident("uint64")
	c.uintptr = c.Ident("uintptr")
	c.float32 = c.Ident("float32")
	c.float64 = c.Ident("float64")
	c.unsafePointer = c.Ident("unsafe.Pointer")
	c.void = c.Ident("void")
	c.string = c.Ident("string")
}

// base strips away qualifiers and typedefs to get the underlying type
func base(dt dwarf.Type) dwarf.Type {
	for {
		if d, ok := dt.(*dwarf.QualType); ok {
			dt = d.Type
			continue
		}
		if d, ok := dt.(*dwarf.TypedefType); ok {
			dt = d.Type
			continue
		}
		break
	}
	return dt
}

// Map from dwarf text names to aliases we use in package "C".
var cnameMap = map[string]string{
	"long int":               "long",
	"long unsigned int":      "ulong",
	"unsigned int":           "uint",
	"short unsigned int":     "ushort",
	"short int":              "short",
	"long long int":          "longlong",
	"long long unsigned int": "ulonglong",
	"signed char":            "schar",
}

// Type returns a *Type with the same memory layout as
// dtype when used as the type of a variable or a struct field.
func (c *typeConv) Type(dtype dwarf.Type) *Type {
	if t, ok := c.m[dtype]; ok {
		if t.Go == nil {
			fatal("type conversion loop at %s", dtype)
		}
		return t
	}

	t := new(Type)
	t.Size = dtype.Size()
	t.Align = -1
	t.C = dtype.Common().Name
	t.EnumValues = nil
	c.m[dtype] = t
	if t.Size < 0 {
		// Unsized types are [0]byte
		t.Size = 0
		t.Go = c.Opaque(0)
		if t.C == "" {
			t.C = "void"
		}
		return t
	}

	switch dt := dtype.(type) {
	default:
		fatal("unexpected type: %s", dtype)

	case *dwarf.AddrType:
		if t.Size != c.ptrSize {
			fatal("unexpected: %d-byte address type - %s", t.Size, dtype)
		}
		t.Go = c.uintptr
		t.Align = t.Size

	case *dwarf.ArrayType:
		if dt.StrideBitSize > 0 {
			// Cannot represent bit-sized elements in Go.
			t.Go = c.Opaque(t.Size)
			break
		}
		gt := &ast.ArrayType{
			Len: c.intExpr(dt.Count),
		}
		t.Go = gt // publish before recursive call
		sub := c.Type(dt.Type)
		t.Align = sub.Align
		gt.Elt = sub.Go
		t.C = fmt.Sprintf("typeof(%s[%d])", sub.C, dt.Count)

	case *dwarf.BoolType:
		t.Go = c.bool
		t.Align = c.ptrSize

	case *dwarf.CharType:
		if t.Size != 1 {
			fatal("unexpected: %d-byte char type - %s", t.Size, dtype)
		}
		t.Go = c.int8
		t.Align = 1

	case *dwarf.EnumType:
		switch t.Size {
		default:
			fatal("unexpected: %d-byte enum type - %s", t.Size, dtype)
		case 1:
			t.Go = c.uint8
		case 2:
			t.Go = c.uint16
		case 4:
			t.Go = c.uint32
		case 8:
			t.Go = c.uint64
		}
		if t.Align = t.Size; t.Align >= c.ptrSize {
			t.Align = c.ptrSize
		}
		t.C = "enum " + dt.EnumName
		t.EnumValues = make(map[string]int64)
		for _, ev := range dt.Val {
			t.EnumValues[ev.Name] = ev.Val
		}

	case *dwarf.FloatType:
		switch t.Size {
		default:
			fatal("unexpected: %d-byte float type - %s", t.Size, dtype)
		case 4:
			t.Go = c.float32
		case 8:
			t.Go = c.float64
		}
		if t.Align = t.Size; t.Align >= c.ptrSize {
			t.Align = c.ptrSize
		}

	case *dwarf.FuncType:
		// No attempt at translation: would enable calls
		// directly between worlds, but we need to moderate those.
		t.Go = c.uintptr
		t.Align = c.ptrSize

	case *dwarf.IntType:
		if dt.BitSize > 0 {
			fatal("unexpected: %d-bit int type - %s", dt.BitSize, dtype)
		}
		switch t.Size {
		default:
			fatal("unexpected: %d-byte int type - %s", t.Size, dtype)
		case 1:
			t.Go = c.int8
		case 2:
			t.Go = c.int16
		case 4:
			t.Go = c.int32
		case 8:
			t.Go = c.int64
		}
		if t.Align = t.Size; t.Align >= c.ptrSize {
			t.Align = c.ptrSize
		}

	case *dwarf.PtrType:
		t.Align = c.ptrSize

		// Translate void* as unsafe.Pointer
		if _, ok := base(dt.Type).(*dwarf.VoidType); ok {
			t.Go = c.unsafePointer
			t.C = "void*"
			break
		}

		gt := &ast.StarExpr{}
		t.Go = gt // publish before recursive call
		sub := c.Type(dt.Type)
		gt.X = sub.Go
		t.C = sub.C + "*"

	case *dwarf.QualType:
		// Ignore qualifier.
		t = c.Type(dt.Type)
		c.m[dtype] = t
		return t

	case *dwarf.StructType:
		// Convert to Go struct, being careful about alignment.
		// Have to give it a name to simulate C "struct foo" references.
		tag := dt.StructName
		if tag == "" {
			tag = "__" + strconv.Itoa(c.tagGen)
			c.tagGen++
		} else if t.C == "" {
			t.C = dt.Kind + " " + tag
		}
		name := c.Ident("_C" + dt.Kind + "_" + tag)
		t.Go = name // publish before recursive calls
		switch dt.Kind {
		case "union", "class":
			c.typedef[name.Name()] = c.Opaque(t.Size)
			if t.C == "" {
				t.C = fmt.Sprintf("typeof(unsigned char[%d])", t.Size)
			}
		case "struct":
			g, csyntax, align := c.Struct(dt)
			if t.C == "" {
				t.C = csyntax
			}
			t.Align = align
			c.typedef[name.Name()] = g
		}

	case *dwarf.TypedefType:
		// Record typedef for printing.
		if dt.Name == "_GoString_" {
			// Special C name for Go string type.
			// Knows string layout used by compilers: pointer plus length,
			// which rounds up to 2 pointers after alignment.
			t.Go = c.string
			t.Size = c.ptrSize * 2
			t.Align = c.ptrSize
			break
		}
		name := c.Ident("_C_" + dt.Name)
		t.Go = name // publish before recursive call
		sub := c.Type(dt.Type)
		t.Size = sub.Size
		t.Align = sub.Align
		if _, ok := c.typedef[name.Name()]; !ok {
			c.typedef[name.Name()] = sub.Go
		}

	case *dwarf.UcharType:
		if t.Size != 1 {
			fatal("unexpected: %d-byte uchar type - %s", t.Size, dtype)
		}
		t.Go = c.uint8
		t.Align = 1

	case *dwarf.UintType:
		if dt.BitSize > 0 {
			fatal("unexpected: %d-bit uint type - %s", dt.BitSize, dtype)
		}
		switch t.Size {
		default:
			fatal("unexpected: %d-byte uint type - %s", t.Size, dtype)
		case 1:
			t.Go = c.uint8
		case 2:
			t.Go = c.uint16
		case 4:
			t.Go = c.uint32
		case 8:
			t.Go = c.uint64
		}
		if t.Align = t.Size; t.Align >= c.ptrSize {
			t.Align = c.ptrSize
		}

	case *dwarf.VoidType:
		t.Go = c.void
		t.C = "void"
	}

	switch dtype.(type) {
	case *dwarf.AddrType, *dwarf.BoolType, *dwarf.CharType, *dwarf.IntType, *dwarf.FloatType, *dwarf.UcharType, *dwarf.UintType:
		s := dtype.Common().Name
		if s != "" {
			if ss, ok := cnameMap[s]; ok {
				s = ss
			}
			s = strings.Join(strings.Split(s, " ", 0), "") // strip spaces
			name := c.Ident("_C_" + s)
			c.typedef[name.Name()] = t.Go
			t.Go = name
		}
	}

	if t.C == "" {
		fatal("internal error: did not create C name for %s", dtype)
	}

	return t
}

// FuncArg returns a Go type with the same memory layout as
// dtype when used as the type of a C function argument.
func (c *typeConv) FuncArg(dtype dwarf.Type) *Type {
	t := c.Type(dtype)
	switch dt := dtype.(type) {
	case *dwarf.ArrayType:
		// Arrays are passed implicitly as pointers in C.
		// In Go, we must be explicit.
		return &Type{
			Size:  c.ptrSize,
			Align: c.ptrSize,
			Go:    &ast.StarExpr{X: t.Go},
			C:     t.C + "*",
		}
	case *dwarf.TypedefType:
		// C has much more relaxed rules than Go for
		// implicit type conversions.  When the parameter
		// is type T defined as *X, simulate a little of the
		// laxness of C by making the argument *X instead of T.
		if ptr, ok := base(dt.Type).(*dwarf.PtrType); ok {
			// Unless the typedef happens to point to void* since
			// Go has special rules around using unsafe.Pointer.
			if _, void := base(ptr.Type).(*dwarf.VoidType); !void {
				return c.Type(ptr)
			}
		}
	}
	return t
}

// FuncType returns the Go type analogous to dtype.
// There is no guarantee about matching memory layout.
func (c *typeConv) FuncType(dtype *dwarf.FuncType) *FuncType {
	p := make([]*Type, len(dtype.ParamType))
	gp := make([]*ast.Field, len(dtype.ParamType))
	for i, f := range dtype.ParamType {
		// gcc's DWARF generator outputs a single DotDotDotType parameter for
		// function pointers that specify no parameters (e.g. void
		// (*__cgo_0)()).  Treat this special case as void.  This case is
		// invalid according to ISO C anyway (i.e. void (*__cgo_1)(...) is not
		// legal).
		if _, ok := f.(*dwarf.DotDotDotType); ok && i == 0 {
			p, gp = nil, nil
			break
		}
		p[i] = c.FuncArg(f)
		gp[i] = &ast.Field{Type: p[i].Go}
	}
	var r *Type
	var gr []*ast.Field
	if _, ok := dtype.ReturnType.(*dwarf.VoidType); !ok && dtype.ReturnType != nil {
		r = c.Type(dtype.ReturnType)
		gr = []*ast.Field{&ast.Field{Type: r.Go}}
	}
	return &FuncType{
		Params: p,
		Result: r,
		Go: &ast.FuncType{
			Params:  &ast.FieldList{List: gp},
			Results: &ast.FieldList{List: gr},
		},
	}
}

// Identifier
func (c *typeConv) Ident(s string) *ast.Ident { return ast.NewIdent(s) }

// Opaque type of n bytes.
func (c *typeConv) Opaque(n int64) ast.Expr {
	return &ast.ArrayType{
		Len: c.intExpr(n),
		Elt: c.byte,
	}
}

// Expr for integer n.
func (c *typeConv) intExpr(n int64) ast.Expr {
	return &ast.BasicLit{
		Kind:  token.INT,
		Value: []byte(strconv.Itoa64(n)),
	}
}

// Add padding of given size to fld.
func (c *typeConv) pad(fld []*ast.Field, size int64) []*ast.Field {
	n := len(fld)
	fld = fld[0 : n+1]
	fld[n] = &ast.Field{Names: []*ast.Ident{c.Ident("_")}, Type: c.Opaque(size)}
	return fld
}

// Struct conversion
func (c *typeConv) Struct(dt *dwarf.StructType) (expr *ast.StructType, csyntax string, align int64) {
	csyntax = "struct { "
	fld := make([]*ast.Field, 0, 2*len(dt.Field)+1) // enough for padding around every field
	off := int64(0)

	// Mangle struct fields that happen to be named Go keywords into
	// _{keyword}.  Create a map from C ident -> Go ident.  The Go ident will
	// be mangled.  Any existing identifier that already has the same name on
	// the C-side will cause the Go-mangled version to be prefixed with _.
	// (e.g. in a struct with fields '_type' and 'type', the latter would be
	// rendered as '__type' in Go).
	ident := make(map[string]string)
	used := make(map[string]bool)
	for _, f := range dt.Field {
		ident[f.Name] = f.Name
		used[f.Name] = true
	}
	for cid, goid := range ident {
		if token.Lookup([]byte(goid)).IsKeyword() {
			// Avoid keyword
			goid = "_" + goid

			// Also avoid existing fields
			for _, exist := used[goid]; exist; _, exist = used[goid] {
				goid = "_" + goid
			}

			used[goid] = true
			ident[cid] = goid
		}
	}

	for _, f := range dt.Field {
		if f.BitSize > 0 && f.BitSize != f.ByteSize*8 {
			continue
		}
		if f.ByteOffset > off {
			fld = c.pad(fld, f.ByteOffset-off)
			off = f.ByteOffset
		}
		t := c.Type(f.Type)
		n := len(fld)
		fld = fld[0 : n+1]

		fld[n] = &ast.Field{Names: []*ast.Ident{c.Ident(ident[f.Name])}, Type: t.Go}
		off += t.Size
		csyntax += t.C + " " + f.Name + "; "
		if t.Align > align {
			align = t.Align
		}
	}
	if off < dt.ByteSize {
		fld = c.pad(fld, dt.ByteSize-off)
		off = dt.ByteSize
	}
	if off != dt.ByteSize {
		fatal("struct size calculation error")
	}
	csyntax += "}"
	expr = &ast.StructType{Fields: &ast.FieldList{List: fld}}
	return
}
