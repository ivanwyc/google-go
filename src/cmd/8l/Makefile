# Copyright 2009 The Go Authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

include ../../Make.conf

TARG=\
	8l\

OFILES=\
	asm.$O\
	elf.$O\
	enam.$O\
	lib.$O\
	list.$O\
	macho.$O\
	obj.$O\
	optab.$O\
	pass.$O\
	pe.$O\
	span.$O\
	go.$O\

HFILES=\
	l.h\
	../8l/8.out.h\
	../ld/elf.h\
	../ld/macho.h\
	../ld/pe.h\


$(TARG): $(OFILES)
	$(LD) -o $(TARG) -L"$(GOROOT)"/lib $(OFILES) -lbio -l9

$(OFILES): $(HFILES)

enam.c: 8.out.h
	sh mkenam

clean:
	rm -f *.$O $(TARG) *.8 enam.c 8.out a.out

install: $(TARG)
	cp $(TARG) "$(GOBIN)"/$(TARG)

%.$O: ../ld/%.c
	$(CC) $(CFLAGS) -c -I. ../ld/$*.c
