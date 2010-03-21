// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gob

// TODO(rsc): When garbage collector changes, revisit
// the allocations in this file that use unsafe.Pointer.

import (
	"bytes"
	"io"
	"math"
	"os"
	"reflect"
	"runtime"
	"unsafe"
)

var (
	errBadUint   = os.ErrorString("gob: encoded unsigned integer out of range")
	errBadType   = os.ErrorString("gob: unknown type id or corrupted data")
	errRange     = os.ErrorString("gob: internal error: field numbers out of bounds")
	errNotStruct = os.ErrorString("gob: TODO: can only handle structs")
)

// The global execution state of an instance of the decoder.
type decodeState struct {
	b        *bytes.Buffer
	err      os.Error
	fieldnum int // the last field number read.
	buf      []byte
}

func newDecodeState(b *bytes.Buffer) *decodeState {
	d := new(decodeState)
	d.b = b
	d.buf = make([]byte, uint64Size)
	return d
}

func overflow(name string) os.ErrorString {
	return os.ErrorString(`value for "` + name + `" out of range`)
}

// decodeUintReader reads an encoded unsigned integer from an io.Reader.
// Used only by the Decoder to read the message length.
func decodeUintReader(r io.Reader, buf []byte) (x uint64, err os.Error) {
	_, err = r.Read(buf[0:1])
	if err != nil {
		return
	}
	b := buf[0]
	if b <= 0x7f {
		return uint64(b), nil
	}
	nb := -int(int8(b))
	if nb > uint64Size {
		err = errBadUint
		return
	}
	var n int
	n, err = io.ReadFull(r, buf[0:nb])
	if err != nil {
		if err == os.EOF {
			err = io.ErrUnexpectedEOF
		}
		return
	}
	// Could check that the high byte is zero but it's not worth it.
	for i := 0; i < n; i++ {
		x <<= 8
		x |= uint64(buf[i])
	}
	return
}

// decodeUint reads an encoded unsigned integer from state.r.
// Sets state.err.  If state.err is already non-nil, it does nothing.
// Does not check for overflow.
func decodeUint(state *decodeState) (x uint64) {
	if state.err != nil {
		return
	}
	var b uint8
	b, state.err = state.b.ReadByte()
	if b <= 0x7f { // includes state.err != nil
		return uint64(b)
	}
	nb := -int(int8(b))
	if nb > uint64Size {
		state.err = errBadUint
		return
	}
	var n int
	n, state.err = state.b.Read(state.buf[0:nb])
	// Don't need to check error; it's safe to loop regardless.
	// Could check that the high byte is zero but it's not worth it.
	for i := 0; i < n; i++ {
		x <<= 8
		x |= uint64(state.buf[i])
	}
	return x
}

// decodeInt reads an encoded signed integer from state.r.
// Sets state.err.  If state.err is already non-nil, it does nothing.
// Does not check for overflow.
func decodeInt(state *decodeState) int64 {
	x := decodeUint(state)
	if state.err != nil {
		return 0
	}
	if x&1 != 0 {
		return ^int64(x >> 1)
	}
	return int64(x >> 1)
}

type decOp func(i *decInstr, state *decodeState, p unsafe.Pointer)

// The 'instructions' of the decoding machine
type decInstr struct {
	op     decOp
	field  int            // field number of the wire type
	indir  int            // how many pointer indirections to reach the value in the struct
	offset uintptr        // offset in the structure of the field to encode
	ovfl   os.ErrorString // error message for overflow/underflow (for arrays, of the elements)
}

// Since the encoder writes no zeros, if we arrive at a decoder we have
// a value to extract and store.  The field number has already been read
// (it's how we knew to call this decoder).
// Each decoder is responsible for handling any indirections associated
// with the data structure.  If any pointer so reached is nil, allocation must
// be done.

// Walk the pointer hierarchy, allocating if we find a nil.  Stop one before the end.
func decIndirect(p unsafe.Pointer, indir int) unsafe.Pointer {
	for ; indir > 1; indir-- {
		if *(*unsafe.Pointer)(p) == nil {
			// Allocation required
			*(*unsafe.Pointer)(p) = unsafe.Pointer(new(unsafe.Pointer))
		}
		p = *(*unsafe.Pointer)(p)
	}
	return p
}

func ignoreUint(i *decInstr, state *decodeState, p unsafe.Pointer) {
	decodeUint(state)
}

func decBool(i *decInstr, state *decodeState, p unsafe.Pointer) {
	if i.indir > 0 {
		if *(*unsafe.Pointer)(p) == nil {
			*(*unsafe.Pointer)(p) = unsafe.Pointer(new(bool))
		}
		p = *(*unsafe.Pointer)(p)
	}
	*(*bool)(p) = decodeInt(state) != 0
}

func decInt8(i *decInstr, state *decodeState, p unsafe.Pointer) {
	if i.indir > 0 {
		if *(*unsafe.Pointer)(p) == nil {
			*(*unsafe.Pointer)(p) = unsafe.Pointer(new(int8))
		}
		p = *(*unsafe.Pointer)(p)
	}
	v := decodeInt(state)
	if v < math.MinInt8 || math.MaxInt8 < v {
		state.err = i.ovfl
	} else {
		*(*int8)(p) = int8(v)
	}
}

func decUint8(i *decInstr, state *decodeState, p unsafe.Pointer) {
	if i.indir > 0 {
		if *(*unsafe.Pointer)(p) == nil {
			*(*unsafe.Pointer)(p) = unsafe.Pointer(new(uint8))
		}
		p = *(*unsafe.Pointer)(p)
	}
	v := decodeUint(state)
	if math.MaxUint8 < v {
		state.err = i.ovfl
	} else {
		*(*uint8)(p) = uint8(v)
	}
}

func decInt16(i *decInstr, state *decodeState, p unsafe.Pointer) {
	if i.indir > 0 {
		if *(*unsafe.Pointer)(p) == nil {
			*(*unsafe.Pointer)(p) = unsafe.Pointer(new(int16))
		}
		p = *(*unsafe.Pointer)(p)
	}
	v := decodeInt(state)
	if v < math.MinInt16 || math.MaxInt16 < v {
		state.err = i.ovfl
	} else {
		*(*int16)(p) = int16(v)
	}
}

func decUint16(i *decInstr, state *decodeState, p unsafe.Pointer) {
	if i.indir > 0 {
		if *(*unsafe.Pointer)(p) == nil {
			*(*unsafe.Pointer)(p) = unsafe.Pointer(new(uint16))
		}
		p = *(*unsafe.Pointer)(p)
	}
	v := decodeUint(state)
	if math.MaxUint16 < v {
		state.err = i.ovfl
	} else {
		*(*uint16)(p) = uint16(v)
	}
}

func decInt32(i *decInstr, state *decodeState, p unsafe.Pointer) {
	if i.indir > 0 {
		if *(*unsafe.Pointer)(p) == nil {
			*(*unsafe.Pointer)(p) = unsafe.Pointer(new(int32))
		}
		p = *(*unsafe.Pointer)(p)
	}
	v := decodeInt(state)
	if v < math.MinInt32 || math.MaxInt32 < v {
		state.err = i.ovfl
	} else {
		*(*int32)(p) = int32(v)
	}
}

func decUint32(i *decInstr, state *decodeState, p unsafe.Pointer) {
	if i.indir > 0 {
		if *(*unsafe.Pointer)(p) == nil {
			*(*unsafe.Pointer)(p) = unsafe.Pointer(new(uint32))
		}
		p = *(*unsafe.Pointer)(p)
	}
	v := decodeUint(state)
	if math.MaxUint32 < v {
		state.err = i.ovfl
	} else {
		*(*uint32)(p) = uint32(v)
	}
}

func decInt64(i *decInstr, state *decodeState, p unsafe.Pointer) {
	if i.indir > 0 {
		if *(*unsafe.Pointer)(p) == nil {
			*(*unsafe.Pointer)(p) = unsafe.Pointer(new(int64))
		}
		p = *(*unsafe.Pointer)(p)
	}
	*(*int64)(p) = int64(decodeInt(state))
}

func decUint64(i *decInstr, state *decodeState, p unsafe.Pointer) {
	if i.indir > 0 {
		if *(*unsafe.Pointer)(p) == nil {
			*(*unsafe.Pointer)(p) = unsafe.Pointer(new(uint64))
		}
		p = *(*unsafe.Pointer)(p)
	}
	*(*uint64)(p) = uint64(decodeUint(state))
}

// Floating-point numbers are transmitted as uint64s holding the bits
// of the underlying representation.  They are sent byte-reversed, with
// the exponent end coming out first, so integer floating point numbers
// (for example) transmit more compactly.  This routine does the
// unswizzling.
func floatFromBits(u uint64) float64 {
	var v uint64
	for i := 0; i < 8; i++ {
		v <<= 8
		v |= u & 0xFF
		u >>= 8
	}
	return math.Float64frombits(v)
}

func decFloat32(i *decInstr, state *decodeState, p unsafe.Pointer) {
	if i.indir > 0 {
		if *(*unsafe.Pointer)(p) == nil {
			*(*unsafe.Pointer)(p) = unsafe.Pointer(new(float32))
		}
		p = *(*unsafe.Pointer)(p)
	}
	v := floatFromBits(decodeUint(state))
	av := v
	if av < 0 {
		av = -av
	}
	if math.MaxFloat32 < av { // underflow is OK
		state.err = i.ovfl
	} else {
		*(*float32)(p) = float32(v)
	}
}

func decFloat64(i *decInstr, state *decodeState, p unsafe.Pointer) {
	if i.indir > 0 {
		if *(*unsafe.Pointer)(p) == nil {
			*(*unsafe.Pointer)(p) = unsafe.Pointer(new(float64))
		}
		p = *(*unsafe.Pointer)(p)
	}
	*(*float64)(p) = floatFromBits(uint64(decodeUint(state)))
}

// uint8 arrays are encoded as an unsigned count followed by the raw bytes.
func decUint8Array(i *decInstr, state *decodeState, p unsafe.Pointer) {
	if i.indir > 0 {
		if *(*unsafe.Pointer)(p) == nil {
			*(*unsafe.Pointer)(p) = unsafe.Pointer(new([]uint8))
		}
		p = *(*unsafe.Pointer)(p)
	}
	b := make([]uint8, decodeUint(state))
	state.b.Read(b)
	*(*[]uint8)(p) = b
}

// Strings are encoded as an unsigned count followed by the raw bytes.
func decString(i *decInstr, state *decodeState, p unsafe.Pointer) {
	if i.indir > 0 {
		if *(*unsafe.Pointer)(p) == nil {
			*(*unsafe.Pointer)(p) = unsafe.Pointer(new([]byte))
		}
		p = *(*unsafe.Pointer)(p)
	}
	b := make([]byte, decodeUint(state))
	state.b.Read(b)
	*(*string)(p) = string(b)
}

func ignoreUint8Array(i *decInstr, state *decodeState, p unsafe.Pointer) {
	b := make([]byte, decodeUint(state))
	state.b.Read(b)
}

// Execution engine

// The encoder engine is an array of instructions indexed by field number of the incoming
// decoder.  It is executed with random access according to field number.
type decEngine struct {
	instr    []decInstr
	numInstr int // the number of active instructions
}

func decodeStruct(engine *decEngine, rtyp *reflect.StructType, b *bytes.Buffer, p uintptr, indir int) os.Error {
	if indir > 0 {
		up := unsafe.Pointer(p)
		if indir > 1 {
			up = decIndirect(up, indir)
		}
		if *(*unsafe.Pointer)(up) == nil {
			// Allocate object.
			*(*unsafe.Pointer)(up) = unsafe.New((*runtime.StructType)(unsafe.Pointer(rtyp)))
		}
		p = *(*uintptr)(up)
	}
	state := newDecodeState(b)
	state.fieldnum = -1
	basep := p
	for state.err == nil {
		delta := int(decodeUint(state))
		if delta < 0 {
			state.err = os.ErrorString("gob decode: corrupted data: negative delta")
			break
		}
		if state.err != nil || delta == 0 { // struct terminator is zero delta fieldnum
			break
		}
		fieldnum := state.fieldnum + delta
		if fieldnum >= len(engine.instr) {
			state.err = errRange
			break
		}
		instr := &engine.instr[fieldnum]
		p := unsafe.Pointer(basep + instr.offset)
		if instr.indir > 1 {
			p = decIndirect(p, instr.indir)
		}
		instr.op(instr, state, p)
		state.fieldnum = fieldnum
	}
	return state.err
}

func ignoreStruct(engine *decEngine, b *bytes.Buffer) os.Error {
	state := newDecodeState(b)
	state.fieldnum = -1
	for state.err == nil {
		delta := int(decodeUint(state))
		if delta < 0 {
			state.err = os.ErrorString("gob ignore decode: corrupted data: negative delta")
			break
		}
		if state.err != nil || delta == 0 { // struct terminator is zero delta fieldnum
			break
		}
		fieldnum := state.fieldnum + delta
		if fieldnum >= len(engine.instr) {
			state.err = errRange
			break
		}
		instr := &engine.instr[fieldnum]
		instr.op(instr, state, unsafe.Pointer(nil))
		state.fieldnum = fieldnum
	}
	return state.err
}

func decodeArrayHelper(state *decodeState, p uintptr, elemOp decOp, elemWid uintptr, length, elemIndir int, ovfl os.ErrorString) os.Error {
	instr := &decInstr{elemOp, 0, elemIndir, 0, ovfl}
	for i := 0; i < length && state.err == nil; i++ {
		up := unsafe.Pointer(p)
		if elemIndir > 1 {
			up = decIndirect(up, elemIndir)
		}
		elemOp(instr, state, up)
		p += uintptr(elemWid)
	}
	return state.err
}

func decodeArray(atyp *reflect.ArrayType, state *decodeState, p uintptr, elemOp decOp, elemWid uintptr, length, indir, elemIndir int, ovfl os.ErrorString) os.Error {
	if indir > 0 {
		up := unsafe.Pointer(p)
		if *(*unsafe.Pointer)(up) == nil {
			// Allocate object.
			*(*unsafe.Pointer)(up) = unsafe.New(atyp)
		}
		p = *(*uintptr)(up)
	}
	if n := decodeUint(state); n != uint64(length) {
		return os.ErrorString("gob: length mismatch in decodeArray")
	}
	return decodeArrayHelper(state, p, elemOp, elemWid, length, elemIndir, ovfl)
}

func ignoreArrayHelper(state *decodeState, elemOp decOp, length int) os.Error {
	instr := &decInstr{elemOp, 0, 0, 0, os.ErrorString("no error")}
	for i := 0; i < length && state.err == nil; i++ {
		elemOp(instr, state, nil)
	}
	return state.err
}

func ignoreArray(state *decodeState, elemOp decOp, length int) os.Error {
	if n := decodeUint(state); n != uint64(length) {
		return os.ErrorString("gob: length mismatch in ignoreArray")
	}
	return ignoreArrayHelper(state, elemOp, length)
}

func decodeSlice(atyp *reflect.SliceType, state *decodeState, p uintptr, elemOp decOp, elemWid uintptr, indir, elemIndir int, ovfl os.ErrorString) os.Error {
	n := int(uintptr(decodeUint(state)))
	if indir > 0 {
		up := unsafe.Pointer(p)
		if *(*unsafe.Pointer)(up) == nil {
			// Allocate the slice header.
			*(*unsafe.Pointer)(up) = unsafe.Pointer(new([]unsafe.Pointer))
		}
		p = *(*uintptr)(up)
	}
	// Allocate storage for the slice elements, that is, the underlying array.
	// Always write a header at p.
	hdrp := (*reflect.SliceHeader)(unsafe.Pointer(p))
	hdrp.Data = uintptr(unsafe.NewArray(atyp.Elem(), n))
	hdrp.Len = n
	hdrp.Cap = n
	return decodeArrayHelper(state, hdrp.Data, elemOp, elemWid, n, elemIndir, ovfl)
}

func ignoreSlice(state *decodeState, elemOp decOp) os.Error {
	return ignoreArrayHelper(state, elemOp, int(decodeUint(state)))
}

var decOpMap = map[reflect.Type]decOp{
	valueKind(false):      decBool,
	valueKind(int8(0)):    decInt8,
	valueKind(int16(0)):   decInt16,
	valueKind(int32(0)):   decInt32,
	valueKind(int64(0)):   decInt64,
	valueKind(uint8(0)):   decUint8,
	valueKind(uint16(0)):  decUint16,
	valueKind(uint32(0)):  decUint32,
	valueKind(uint64(0)):  decUint64,
	valueKind(float32(0)): decFloat32,
	valueKind(float64(0)): decFloat64,
	valueKind("x"):        decString,
}

var decIgnoreOpMap = map[typeId]decOp{
	tBool:   ignoreUint,
	tInt:    ignoreUint,
	tUint:   ignoreUint,
	tFloat:  ignoreUint,
	tBytes:  ignoreUint8Array,
	tString: ignoreUint8Array,
}

// Return the decoding op for the base type under rt and
// the indirection count to reach it.
func (dec *Decoder) decOpFor(wireId typeId, rt reflect.Type, name string) (decOp, int, os.Error) {
	typ, indir := indirect(rt)
	op, ok := decOpMap[reflect.Typeof(typ)]
	if !ok {
		// Special cases
		switch t := typ.(type) {
		case *reflect.SliceType:
			name = "element of " + name
			if _, ok := t.Elem().(*reflect.Uint8Type); ok {
				op = decUint8Array
				break
			}
			var elemId typeId
			if tt, ok := builtinIdToType[wireId]; ok {
				elemId = tt.(*sliceType).Elem
			} else {
				elemId = dec.wireType[wireId].slice.Elem
			}
			elemOp, elemIndir, err := dec.decOpFor(elemId, t.Elem(), name)
			if err != nil {
				return nil, 0, err
			}
			ovfl := overflow(name)
			op = func(i *decInstr, state *decodeState, p unsafe.Pointer) {
				state.err = decodeSlice(t, state, uintptr(p), elemOp, t.Elem().Size(), i.indir, elemIndir, ovfl)
			}

		case *reflect.ArrayType:
			name = "element of " + name
			elemId := dec.wireType[wireId].array.Elem
			elemOp, elemIndir, err := dec.decOpFor(elemId, t.Elem(), name)
			if err != nil {
				return nil, 0, err
			}
			ovfl := overflow(name)
			op = func(i *decInstr, state *decodeState, p unsafe.Pointer) {
				state.err = decodeArray(t, state, uintptr(p), elemOp, t.Elem().Size(), t.Len(), i.indir, elemIndir, ovfl)
			}

		case *reflect.StructType:
			// Generate a closure that calls out to the engine for the nested type.
			enginePtr, err := dec.getDecEnginePtr(wireId, typ)
			if err != nil {
				return nil, 0, err
			}
			op = func(i *decInstr, state *decodeState, p unsafe.Pointer) {
				// indirect through enginePtr to delay evaluation for recursive structs
				state.err = decodeStruct(*enginePtr, t, state.b, uintptr(p), i.indir)
			}
		}
	}
	if op == nil {
		return nil, 0, os.ErrorString("gob: decode can't handle type " + rt.String())
	}
	return op, indir, nil
}

// Return the decoding op for a field that has no destination.
func (dec *Decoder) decIgnoreOpFor(wireId typeId) (decOp, os.Error) {
	op, ok := decIgnoreOpMap[wireId]
	if !ok {
		// Special cases
		wire := dec.wireType[wireId]
		switch {
		case wire.array != nil:
			elemId := wire.array.Elem
			elemOp, err := dec.decIgnoreOpFor(elemId)
			if err != nil {
				return nil, err
			}
			op = func(i *decInstr, state *decodeState, p unsafe.Pointer) {
				state.err = ignoreArray(state, elemOp, wire.array.Len)
			}

		case wire.slice != nil:
			elemId := wire.slice.Elem
			elemOp, err := dec.decIgnoreOpFor(elemId)
			if err != nil {
				return nil, err
			}
			op = func(i *decInstr, state *decodeState, p unsafe.Pointer) {
				state.err = ignoreSlice(state, elemOp)
			}

		case wire.strct != nil:
			// Generate a closure that calls out to the engine for the nested type.
			enginePtr, err := dec.getIgnoreEnginePtr(wireId)
			if err != nil {
				return nil, err
			}
			op = func(i *decInstr, state *decodeState, p unsafe.Pointer) {
				// indirect through enginePtr to delay evaluation for recursive structs
				state.err = ignoreStruct(*enginePtr, state.b)
			}
		}
	}
	if op == nil {
		return nil, os.ErrorString("ignore can't handle type " + wireId.string())
	}
	return op, nil
}

// Are these two gob Types compatible?
// Answers the question for basic types, arrays, and slices.
// Structs are considered ok; fields will be checked later.
func (dec *Decoder) compatibleType(fr reflect.Type, fw typeId) bool {
	for {
		if pt, ok := fr.(*reflect.PtrType); ok {
			fr = pt.Elem()
			continue
		}
		break
	}
	switch t := fr.(type) {
	default:
		// interface, map, chan, etc: cannot handle.
		return false
	case *reflect.BoolType:
		return fw == tBool
	case *reflect.IntType, *reflect.Int8Type, *reflect.Int16Type, *reflect.Int32Type, *reflect.Int64Type:
		return fw == tInt
	case *reflect.UintType, *reflect.Uint8Type, *reflect.Uint16Type, *reflect.Uint32Type, *reflect.Uint64Type, *reflect.UintptrType:
		return fw == tUint
	case *reflect.FloatType, *reflect.Float32Type, *reflect.Float64Type:
		return fw == tFloat
	case *reflect.StringType:
		return fw == tString
	case *reflect.ArrayType:
		wire, ok := dec.wireType[fw]
		if !ok || wire.array == nil {
			return false
		}
		array := wire.array
		return ok && t.Len() == array.Len && dec.compatibleType(t.Elem(), array.Elem)
	case *reflect.SliceType:
		// Is it an array of bytes?
		et := t.Elem()
		if _, ok := et.(*reflect.Uint8Type); ok {
			return fw == tBytes
		}
		// Extract and compare element types.
		var sw *sliceType
		if tt, ok := builtinIdToType[fw]; ok {
			sw = tt.(*sliceType)
		} else {
			sw = dec.wireType[fw].slice
		}
		elem, _ := indirect(t.Elem())
		return sw != nil && dec.compatibleType(elem, sw.Elem)
	case *reflect.StructType:
		return true
	}
	return true
}

func (dec *Decoder) compileDec(remoteId typeId, rt reflect.Type) (engine *decEngine, err os.Error) {
	srt, ok1 := rt.(*reflect.StructType)
	var wireStruct *structType
	// Builtin types can come from global pool; the rest must be defined by the decoder
	if t, ok := builtinIdToType[remoteId]; ok {
		wireStruct = t.(*structType)
	} else {
		w, ok2 := dec.wireType[remoteId]
		if !ok1 || !ok2 {
			return nil, errNotStruct
		}
		wireStruct = w.strct
	}
	engine = new(decEngine)
	engine.instr = make([]decInstr, len(wireStruct.field))
	// Loop over the fields of the wire type.
	for fieldnum := 0; fieldnum < len(wireStruct.field); fieldnum++ {
		wireField := wireStruct.field[fieldnum]
		// Find the field of the local type with the same name.
		localField, present := srt.FieldByName(wireField.name)
		ovfl := overflow(wireField.name)
		// TODO(r): anonymous names
		if !present {
			op, err := dec.decIgnoreOpFor(wireField.id)
			if err != nil {
				return nil, err
			}
			engine.instr[fieldnum] = decInstr{op, fieldnum, 0, 0, ovfl}
			continue
		}
		if !dec.compatibleType(localField.Type, wireField.id) {
			return nil, os.ErrorString("gob: wrong type (" +
				localField.Type.String() + ") for received field " +
				wireStruct.name + "." + wireField.name)
		}
		op, indir, err := dec.decOpFor(wireField.id, localField.Type, localField.Name)
		if err != nil {
			return nil, err
		}
		engine.instr[fieldnum] = decInstr{op, fieldnum, indir, uintptr(localField.Offset), ovfl}
		engine.numInstr++
	}
	return
}

func (dec *Decoder) getDecEnginePtr(remoteId typeId, rt reflect.Type) (enginePtr **decEngine, err os.Error) {
	decoderMap, ok := dec.decoderCache[rt]
	if !ok {
		decoderMap = make(map[typeId]**decEngine)
		dec.decoderCache[rt] = decoderMap
	}
	if enginePtr, ok = decoderMap[remoteId]; !ok {
		// To handle recursive types, mark this engine as underway before compiling.
		enginePtr = new(*decEngine)
		decoderMap[remoteId] = enginePtr
		*enginePtr, err = dec.compileDec(remoteId, rt)
		if err != nil {
			decoderMap[remoteId] = nil, false
		}
	}
	return
}

// When ignoring data, in effect we compile it into this type
type emptyStruct struct{}

var emptyStructType = reflect.Typeof(emptyStruct{})

func (dec *Decoder) getIgnoreEnginePtr(wireId typeId) (enginePtr **decEngine, err os.Error) {
	var ok bool
	if enginePtr, ok = dec.ignorerCache[wireId]; !ok {
		// To handle recursive types, mark this engine as underway before compiling.
		enginePtr = new(*decEngine)
		dec.ignorerCache[wireId] = enginePtr
		*enginePtr, err = dec.compileDec(wireId, emptyStructType)
		if err != nil {
			dec.ignorerCache[wireId] = nil, false
		}
	}
	return
}

func (dec *Decoder) decode(wireId typeId, e interface{}) os.Error {
	// Dereference down to the underlying struct type.
	rt, indir := indirect(reflect.Typeof(e))
	st, ok := rt.(*reflect.StructType)
	if !ok {
		return os.ErrorString("gob: decode can't handle " + rt.String())
	}
	enginePtr, err := dec.getDecEnginePtr(wireId, rt)
	if err != nil {
		return err
	}
	engine := *enginePtr
	if engine.numInstr == 0 && st.NumField() > 0 && len(dec.wireType[wireId].strct.field) > 0 {
		name := rt.Name()
		return os.ErrorString("gob: type mismatch: no fields matched compiling decoder for " + name)
	}
	return decodeStruct(engine, st, dec.state.b, uintptr(reflect.NewValue(e).Addr()), indir)
}

func init() {
	// We assume that the size of float is sufficient to tell us whether it is
	// equivalent to float32 or to float64.   This is very unlikely to be wrong.
	var op decOp
	switch unsafe.Sizeof(float(0)) {
	case unsafe.Sizeof(float32(0)):
		op = decFloat32
	case unsafe.Sizeof(float64(0)):
		op = decFloat64
	default:
		panic("gob: unknown size of float", unsafe.Sizeof(float(0)))
	}
	decOpMap[valueKind(float(0))] = op

	// A similar assumption about int and uint.  Also assume int and uint have the same size.
	var uop decOp
	switch unsafe.Sizeof(int(0)) {
	case unsafe.Sizeof(int32(0)):
		op = decInt32
		uop = decUint32
	case unsafe.Sizeof(int64(0)):
		op = decInt64
		uop = decUint64
	default:
		panic("gob: unknown size of int/uint", unsafe.Sizeof(int(0)))
	}
	decOpMap[valueKind(int(0))] = op
	decOpMap[valueKind(uint(0))] = uop

	// Finally uintptr
	switch unsafe.Sizeof(uintptr(0)) {
	case unsafe.Sizeof(uint32(0)):
		uop = decUint32
	case unsafe.Sizeof(uint64(0)):
		uop = decUint64
	default:
		panic("gob: unknown size of uintptr", unsafe.Sizeof(uintptr(0)))
	}
	decOpMap[valueKind(uintptr(0))] = uop
}
