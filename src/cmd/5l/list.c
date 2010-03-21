// Inferno utils/5l/list.h
// http://code.google.com/p/inferno-os/source/browse/utils/5l/list.c
//
//	Copyright © 1994-1999 Lucent Technologies Inc.  All rights reserved.
//	Portions Copyright © 1995-1997 C H Forsyth (forsyth@terzarima.net)
//	Portions Copyright © 1997-1999 Vita Nuova Limited
//	Portions Copyright © 2000-2007 Vita Nuova Holdings Limited (www.vitanuova.com)
//	Portions Copyright © 2004,2006 Bruce Ellis
//	Portions Copyright © 2005-2007 C H Forsyth (forsyth@terzarima.net)
//	Revisions Copyright © 2000-2007 Lucent Technologies Inc. and others
//	Portions Copyright © 2009 The Go Authors.  All rights reserved.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT.  IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

#include "l.h"
#include "../ld/lib.h"

void
listinit(void)
{

	fmtinstall('A', Aconv);
	fmtinstall('C', Cconv);
	fmtinstall('D', Dconv);
	fmtinstall('P', Pconv);
	fmtinstall('S', Sconv);
	fmtinstall('N', Nconv);
	fmtinstall('O', Oconv);		// C_type constants
}

void
prasm(Prog *p)
{
	print("%P\n", p);
}

int
Pconv(Fmt *fp)
{
	char str[STRINGSZ], *s;
	Prog *p;
	int a;

	p = va_arg(fp->args, Prog*);
	curp = p;
	a = p->as;
	switch(a) {
	default:
		s = str;
		s += sprint(s, "(%d)", p->line);
		if(p->reg == NREG)
			sprint(s, "	%A%C	%D,%D",
				a, p->scond, &p->from, &p->to);
		else
		if(p->from.type != D_FREG)
			sprint(s, "	%A%C	%D,R%d,%D",
				a, p->scond, &p->from, p->reg, &p->to);
		else
			sprint(s, "	%A%C	%D,F%d,%D",
				a, p->scond, &p->from, p->reg, &p->to);
		break;

	case ASWPW:
	case ASWPBU:
		sprint(str, "(%d)	%A%C	R%d,%D,%D",
			p->line, a, p->scond, p->reg, &p->from, &p->to);
		break;

	case ADATA:
	case AINIT:
	case ADYNT:
		sprint(str, "(%d)	%A%C	%D/%d,%D",
			p->line, a, p->scond, &p->from, p->reg, &p->to);
		break;

	case AWORD:
		sprint(str, "WORD %x", p->to.offset);
		break;

	case ADWORD:
		sprint(str, "DWORD %x %x", p->from.offset, p->to.offset);
		break;
	}
	return fmtstrcpy(fp, str);
}

int
Aconv(Fmt *fp)
{
	char *s;
	int a;

	a = va_arg(fp->args, int);
	s = "???";
	if(a >= AXXX && a < ALAST)
		s = anames[a];
	return fmtstrcpy(fp, s);
}

char*	strcond[16] =
{
	".EQ",
	".NE",
	".HS",
	".LO",
	".MI",
	".PL",
	".VS",
	".VC",
	".HI",
	".LS",
	".GE",
	".LT",
	".GT",
	".LE",
	"",
	".NV"
};

int
Cconv(Fmt *fp)
{
	char s[20];
	int c;

	c = va_arg(fp->args, int);
	strcpy(s, strcond[c & C_SCOND]);
	if(c & C_SBIT)
		strcat(s, ".S");
	if(c & C_PBIT)
		strcat(s, ".P");
	if(c & C_WBIT)
		strcat(s, ".W");
	if(c & C_UBIT)		/* ambiguous with FBIT */
		strcat(s, ".U");
	return fmtstrcpy(fp, s);
}

int
Dconv(Fmt *fp)
{
	char str[STRINGSZ];
	char *op;
	Adr *a;
	int32 v;

	a = va_arg(fp->args, Adr*);
	switch(a->type) {

	default:
		sprint(str, "GOK-type(%d)", a->type);
		break;

	case D_NONE:
		str[0] = 0;
		if(a->name != D_NONE || a->reg != NREG || a->sym != S)
			sprint(str, "%N(R%d)(NONE)", a, a->reg);
		break;

	case D_CONST:
		if(a->reg == NREG)
			sprint(str, "$%N", a);
		else
			sprint(str, "$%N(R%d)", a, a->reg);
		break;

	case D_CONST2:
		sprint(str, "$%d-%d", a->offset, a->offset2);
		break;

	case D_SHIFT:
		v = a->offset;
		op = "<<>>->@>" + (((v>>5) & 3) << 1);
		if(v & (1<<4))
			sprint(str, "R%d%c%cR%d", v&15, op[0], op[1], (v>>8)&15);
		else
			sprint(str, "R%d%c%c%d", v&15, op[0], op[1], (v>>7)&31);
		if(a->reg != NREG)
			sprint(str+strlen(str), "(R%d)", a->reg);
		break;

	case D_OCONST:
		sprint(str, "$*$%N", a);
		if(a->reg != NREG)
			sprint(str, "%N(R%d)(CONST)", a, a->reg);
		break;

	case D_OREG:
		if(a->reg != NREG)
			sprint(str, "%N(R%d)", a, a->reg);
		else
			sprint(str, "%N", a);
		break;

	case D_REG:
		sprint(str, "R%d", a->reg);
		if(a->name != D_NONE || a->sym != S)
			sprint(str, "%N(R%d)(REG)", a, a->reg);
		break;

	case D_REGREG:
		sprint(str, "(R%d,R%d)", a->reg, (int)a->offset);
		if(a->name != D_NONE || a->sym != S)
			sprint(str, "%N(R%d)(REG)", a, a->reg);
		break;

	case D_FREG:
		sprint(str, "F%d", a->reg);
		if(a->name != D_NONE || a->sym != S)
			sprint(str, "%N(R%d)(REG)", a, a->reg);
		break;

	case D_PSR:
		switch(a->reg) {
		case 0:
			sprint(str, "CPSR");
			break;
		case 1:
			sprint(str, "SPSR");
			break;
		default:
			sprint(str, "PSR%d", a->reg);
			break;
		}
		if(a->name != D_NONE || a->sym != S)
			sprint(str, "%N(PSR%d)(REG)", a, a->reg);
		break;

	case D_FPCR:
		switch(a->reg){
		case 0:
			sprint(str, "FPSR");
			break;
		case 1:
			sprint(str, "FPCR");
			break;
		default:
			sprint(str, "FCR%d", a->reg);
			break;
		}
		if(a->name != D_NONE || a->sym != S)
			sprint(str, "%N(FCR%d)(REG)", a, a->reg);

		break;

	case D_BRANCH:	/* botch */
		if(curp->cond != P) {
			v = curp->cond->pc;
			if(a->sym != S)
				sprint(str, "%s+%.5lux(BRANCH)", a->sym->name, v);
			else
				sprint(str, "%.5lux(BRANCH)", v);
		} else
			if(a->sym != S)
				sprint(str, "%s+%d(APC)", a->sym->name, a->offset);
			else
				sprint(str, "%d(APC)", a->offset);
		break;

	case D_FCONST:
		sprint(str, "$%e", ieeedtod(a->ieee));
		break;

	case D_SCONST:
		sprint(str, "$\"%S\"", a->sval);
		break;
	}
	return fmtstrcpy(fp, str);
}

int
Nconv(Fmt *fp)
{
	char str[STRINGSZ];
	Adr *a;
	Sym *s;

	a = va_arg(fp->args, Adr*);
	s = a->sym;
	switch(a->name) {
	default:
		sprint(str, "GOK-name(%d)", a->name);
		break;

	case D_NONE:
		sprint(str, "%d", a->offset);
		break;

	case D_EXTERN:
		if(s == S)
			sprint(str, "%d(SB)", a->offset);
		else
			sprint(str, "%s+%d(SB)", s->name, a->offset);
		break;

	case D_STATIC:
		if(s == S)
			sprint(str, "<>+%d(SB)", a->offset);
		else
			sprint(str, "%s<>+%d(SB)", s->name, a->offset);
		break;

	case D_AUTO:
		if(s == S)
			sprint(str, "%d(SP)", a->offset);
		else
			sprint(str, "%s-%d(SP)", s->name, -a->offset);
		break;

	case D_PARAM:
		if(s == S)
			sprint(str, "%d(FP)", a->offset);
		else
			sprint(str, "%s+%d(FP)", s->name, a->offset);
		break;
	}
	return fmtstrcpy(fp, str);
}

int
Sconv(Fmt *fp)
{
	int i, c;
	char str[STRINGSZ], *p, *a;

	a = va_arg(fp->args, char*);
	p = str;
	for(i=0; i<sizeof(int32); i++) {
		c = a[i] & 0xff;
		if(c >= 'a' && c <= 'z' ||
		   c >= 'A' && c <= 'Z' ||
		   c >= '0' && c <= '9' ||
		   c == ' ' || c == '%') {
			*p++ = c;
			continue;
		}
		*p++ = '\\';
		switch(c) {
		case 0:
			*p++ = 'z';
			continue;
		case '\\':
		case '"':
			*p++ = c;
			continue;
		case '\n':
			*p++ = 'n';
			continue;
		case '\t':
			*p++ = 't';
			continue;
		}
		*p++ = (c>>6) + '0';
		*p++ = ((c>>3) & 7) + '0';
		*p++ = (c & 7) + '0';
	}
	*p = 0;
	return fmtstrcpy(fp, str);
}

static char*
cnames[] =
{
	[C_ADDR]	= "C_ADDR",
	[C_BCON]	= "C_BCON",
	[C_FAUTO]	= "C_FAUTO",
	[C_FCON]	= "C_FCON",
	[C_FCR]		= "C_FCR",
	[C_FEXT]	= "C_FEXT",
	[C_FOREG]	= "C_FOREG",
	[C_FREG]	= "C_FREG",
	[C_GACON]	= "C_GACON",
	[C_GBRA]	= "C_GBRA",
	[C_GCON]	= "C_GCON",
	[C_GOK]		= "C_GOK",
	[C_GOREG]	= "C_GOREG",
	[C_HAUTO]	= "C_HAUTO",
	[C_HEXT]	= "C_HEXT",
	[C_HFAUTO]	= "C_HFAUTO",
	[C_HFEXT]	= "C_HFEXT",
	[C_HFOREG]	= "C_HFOREG",
	[C_HOREG]	= "C_HOREG",
	[C_HREG]	= "C_HREG",
	[C_LACON]	= "C_LACON",
	[C_LAUTO]	= "C_LAUTO",
	[C_LBRA]	= "C_LBRA",
	[C_LCON]	= "C_LCON",
	[C_LECON]	= "C_LECON",
	[C_LEXT]	= "C_LEXT",
	[C_LOREG]	= "C_LOREG",
	[C_NCON]	= "C_NCON",
	[C_NONE]	= "C_NONE",
	[C_OFFPC]	= "C_OFFPC",
	[C_PC]		= "C_PC",
	[C_PSR]		= "C_PSR",
	[C_RACON]	= "C_RACON",
	[C_RCON]	= "C_RCON",
	[C_RECON]	= "C_RECON",
	[C_REG]		= "C_REG",
	[C_REGREG]	= "C_REGREG",
	[C_ROREG]	= "C_ROREG",
	[C_SACON]	= "C_SACON",
	[C_SAUTO]	= "C_SAUTO",
	[C_SBRA]	= "C_SBRA",
	[C_SCON]	= "C_SCON",
	[C_SEXT]	= "C_SEXT",
	[C_SHIFT]	= "C_SHIFT",
	[C_SOREG]	= "C_SOREG",
	[C_SP]		= "C_SP",
	[C_SROREG]	= "C_SROREG"
};

int
Oconv(Fmt *fp)
{
	char buf[500];
	int o;

	o = va_arg(fp->args, int);
	if(o < 0 || o >= nelem(cnames) || cnames[o] == nil) {
		snprint(buf, sizeof(buf), "C_%d", o);
		return fmtstrcpy(fp, buf);
	}
	return fmtstrcpy(fp, cnames[o]);
}

void
diag(char *fmt, ...)
{
	char buf[STRINGSZ], *tn;
	va_list arg;

	tn = "??none??";
	if(curtext != P && curtext->from.sym != S)
		tn = curtext->from.sym->name;
	va_start(arg, fmt);
	vseprint(buf, buf+sizeof(buf), fmt, arg);
	va_end(arg);
	print("%s: %s\n", tn, buf);

	nerrors++;
	if(nerrors > 10) {
		print("too many errors\n");
		errorexit();
	}
}
