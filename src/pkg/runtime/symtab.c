// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Runtime symbol table access.  Work in progress.
// The Plan 9 symbol table is not in a particularly convenient form.
// The routines here massage it into a more usable form; eventually
// we'll change 6l to do this for us, but it is easier to experiment
// here than to change 6l and all the other tools.
//
// The symbol table also needs to be better integrated with the type
// strings table in the future.  This is just a quick way to get started
// and figure out exactly what we want.

#include "runtime.h"
#include "defs.h"
#include "os.h"

// TODO(rsc): Move this *under* the text segment.
// Then define names for these addresses instead of hard-coding magic ones.
#define SYMCOUNTS ((int32*)(0x99LL<<24))   // known to 6l, 8l; see src/cmd/ld/lib.h
#define SYMDATA ((byte*)(0x99LL<<24) + 8)

typedef struct Sym Sym;
struct Sym
{
	uintptr value;
	byte symtype;
	byte *name;
//	byte *gotype;
};

// Walk over symtab, calling fn(&s) for each symbol.
static void
walksymtab(void (*fn)(Sym*))
{
	int32 *v;
	byte *p, *ep, *q;
	Sym s;

	// TODO(rsc): Remove once TODO at top of file is done.
	if(goos != nil && strcmp((uint8*)goos, (uint8*)"nacl") == 0)
		return;
	if(goos != nil && strcmp((uint8*)goos, (uint8*)"pchw") == 0)
		return;

#ifdef __MINGW__
	v = get_symdat_addr();
	p = (byte*)v+8;
#else
	v = SYMCOUNTS;
	p = SYMDATA;
#endif
	ep = p + v[0];
	while(p < ep) {
		if(p + 7 > ep)
			break;
		s.value = ((uint32)p[0]<<24) | ((uint32)p[1]<<16) | ((uint32)p[2]<<8) | ((uint32)p[3]);
		if(!(p[4]&0x80))
			break;
		s.symtype = p[4] & ~0x80;
		p += 5;
		s.name = p;
		if(s.symtype == 'z' || s.symtype == 'Z') {
			// path reference string - skip first byte,
			// then 2-byte pairs ending at two zeros.
			q = p+1;
			for(;;) {
				if(q+2 > ep)
					return;
				if(q[0] == '\0' && q[1] == '\0')
					break;
				q += 2;
			}
			p = q+2;
		}else{
			q = mchr(p, '\0', ep);
			if(q == nil)
				break;
			p = q+1;
		}
		p += 4;	// go type
		fn(&s);
	}
}

// Symtab walker; accumulates info about functions.

static Func *func;
static int32 nfunc;

static byte **fname;
static int32 nfname;

static Lock funclock;

static void
dofunc(Sym *sym)
{
	Func *f;

	switch(sym->symtype) {
	case 't':
	case 'T':
		if(strcmp(sym->name, (byte*)"etext") == 0)
			break;
		if(func == nil) {
			nfunc++;
			break;
		}
		f = &func[nfunc++];
		f->name = gostring(sym->name);
		f->entry = sym->value;
		break;
	case 'm':
		if(nfunc > 0 && func != nil)
			func[nfunc-1].frame = sym->value;
		break;
	case 'p':
		if(nfunc > 0 && func != nil) {
			f = &func[nfunc-1];
			// args counts 32-bit words.
			// sym->value is the arg's offset.
			// don't know width of this arg, so assume it is 64 bits.
			if(f->args < sym->value/4 + 2)
				f->args = sym->value/4 + 2;
		}
		break;
	case 'f':
		if(fname == nil) {
			if(sym->value >= nfname)
				nfname = sym->value+1;
			break;
		}
		fname[sym->value] = sym->name;
		break;
	}
}

// put together the path name for a z entry.
// the f entries have been accumulated into fname already.
static void
makepath(byte *buf, int32 nbuf, byte *path)
{
	int32 n, len;
	byte *p, *ep, *q;

	if(nbuf <= 0)
		return;

	p = buf;
	ep = buf + nbuf;
	*p = '\0';
	for(;;) {
		if(path[0] == 0 && path[1] == 0)
			break;
		n = (path[0]<<8) | path[1];
		path += 2;
		if(n >= nfname)
			break;
		q = fname[n];
		len = findnull(q);
		if(p+1+len >= ep)
			break;
		if(p > buf && p[-1] != '/')
			*p++ = '/';
		mcpy(p, q, len+1);
		p += len;
	}
}

// walk symtab accumulating path names for use by pc/ln table.
// don't need the full generality of the z entry history stack because
// there are no includes in go (and only sensible includes in our c);
// assume code only appear in top-level files.
static void
dosrcline(Sym *sym)
{
	static byte srcbuf[1000];
	static struct {
		String srcstring;
		int32 aline;
		int32 delta;
	} files[200];
	static int32 incstart;
	static int32 nfunc, nfile, nhist;
	Func *f;
	int32 i;

	switch(sym->symtype) {
	case 't':
	case 'T':
		if(strcmp(sym->name, (byte*)"etext") == 0)
			break;
		f = &func[nfunc++];
		// find source file
		for(i = 0; i < nfile - 1; i++) {
			if (files[i+1].aline > f->ln0)
				break;
		}
		f->src = files[i].srcstring;
		f->ln0 -= files[i].delta;
		break;
	case 'z':
		if(sym->value == 1) {
			// entry for main source file for a new object.
			makepath(srcbuf, sizeof srcbuf, sym->name+1);
			nhist = 0;
			nfile = 0;
			if(nfile == nelem(files))
				return;
			files[nfile].srcstring = gostring(srcbuf);
			files[nfile].aline = 0;
			files[nfile++].delta = 0;
		} else {
			// push or pop of included file.
			makepath(srcbuf, sizeof srcbuf, sym->name+1);
			if(srcbuf[0] != '\0') {
				if(nhist++ == 0)
					incstart = sym->value;
				if(nhist == 0 && nfile < nelem(files)) {
					// new top-level file
					files[nfile].srcstring = gostring(srcbuf);
					files[nfile].aline = sym->value;
					// this is "line 0"
					files[nfile++].delta = sym->value - 1;
				}
			}else{
				if(--nhist == 0)
					files[nfile-1].delta += sym->value - incstart;
			}
		}
	}
}

enum { PcQuant = 1 };

// Interpret pc/ln table, saving the subpiece for each func.
static void
splitpcln(void)
{
	int32 line;
	uintptr pc;
	byte *p, *ep;
	Func *f, *ef;
	int32 *v;

	// TODO(rsc): Remove once TODO at top of file is done.
	if(goos != nil && strcmp((uint8*)goos, (uint8*)"nacl") == 0)
		return;
	if(goos != nil && strcmp((uint8*)goos, (uint8*)"pchw") == 0)
		return;

	// pc/ln table bounds
#ifdef __MINGW__
	v = get_symdat_addr();
	p = (byte*)v+8;
#else
	v = SYMCOUNTS;
	p = SYMDATA;
#endif
	p += v[0];
	ep = p+v[1];

	f = func;
	ef = func + nfunc;
	pc = func[0].entry;	// text base
	f->pcln.array = p;
	f->pc0 = pc - PcQuant;
	line = 0;
	for(; p < ep; p++) {
		if(f < ef && pc > (f+1)->entry) {
			f->pcln.len = p - f->pcln.array;
			f->pcln.cap = f->pcln.len;
			f++;
			f->pcln.array = p;
			f->pc0 = pc;
			f->ln0 = line;
		}
		if(*p == 0) {
			// 4 byte add to line
			line += (p[1]<<24) | (p[2]<<16) | (p[3]<<8) | p[4];
			p += 4;
		} else if(*p <= 64) {
			line += *p;
		} else if(*p <= 128) {
			line -= *p - 64;
		} else {
			pc += PcQuant*(*p - 129);
		}
		pc += PcQuant;
	}
	if(f < ef) {
		f->pcln.len = p - f->pcln.array;
		f->pcln.cap = f->pcln.len;
	}
}


// Return actual file line number for targetpc in func f.
// (Source file is f->src.)
int32
funcline(Func *f, uint64 targetpc)
{
	byte *p, *ep;
	uintptr pc;
	int32 line;

	p = f->pcln.array;
	ep = p + f->pcln.len;
	pc = f->pc0;
	line = f->ln0;
	for(; p < ep && pc <= targetpc; p++) {
		if(*p == 0) {
			line += (p[1]<<24) | (p[2]<<16) | (p[3]<<8) | p[4];
			p += 4;
		} else if(*p <= 64) {
			line += *p;
		} else if(*p <= 128) {
			line -= *p - 64;
		} else {
			pc += PcQuant*(*p - 129);
		}
		pc += PcQuant;
	}
	return line;
}

static void
buildfuncs(void)
{
	extern byte etext[];

	if(func != nil)
		return;
	// count funcs, fnames
	nfunc = 0;
	nfname = 0;
	walksymtab(dofunc);

	// initialize tables
	func = mal((nfunc+1)*sizeof func[0]);
	func[nfunc].entry = (uint64)etext;
	fname = mal(nfname*sizeof fname[0]);
	nfunc = 0;
	walksymtab(dofunc);

	// split pc/ln table by func
	splitpcln();

	// record src file and line info for each func
	walksymtab(dosrcline);
}

Func*
findfunc(uintptr addr)
{
	Func *f;
	int32 nf, n;

	lock(&funclock);
	if(func == nil)
		buildfuncs();
	unlock(&funclock);

	if(nfunc == 0)
		return nil;
	if(addr < func[0].entry || addr >= func[nfunc].entry)
		return nil;

	// binary search to find func with entry <= addr.
	f = func;
	nf = nfunc;
	while(nf > 0) {
		n = nf/2;
		if(f[n].entry <= addr && addr < f[n+1].entry)
			return &f[n];
		else if(addr < f[n].entry)
			nf = n;
		else {
			f += n+1;
			nf -= n+1;
		}
	}

	// can't get here -- we already checked above
	// that the address was in the table bounds.
	// this can only happen if the table isn't sorted
	// by address or if the binary search above is buggy.
	prints("findfunc unreachable\n");
	return nil;
}
