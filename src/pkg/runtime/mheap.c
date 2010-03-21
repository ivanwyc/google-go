// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Page heap.
//
// See malloc.h for overview.
//
// When a MSpan is in the heap free list, state == MSpanFree
// and heapmap(s->start) == span, heapmap(s->start+s->npages-1) == span.
//
// When a MSpan is allocated, state == MSpanInUse
// and heapmap(i) == span for all s->start <= i < s->start+s->npages.

#include "runtime.h"
#include "malloc.h"

static MSpan *MHeap_AllocLocked(MHeap*, uintptr, int32);
static bool MHeap_Grow(MHeap*, uintptr);
static void MHeap_FreeLocked(MHeap*, MSpan*);
static MSpan *MHeap_AllocLarge(MHeap*, uintptr);
static MSpan *BestFit(MSpan*, uintptr, MSpan*);

static void
RecordSpan(void *vh, byte *p)
{
	MHeap *h;
	MSpan *s;

	h = vh;
	s = (MSpan*)p;
	s->allnext = h->allspans;
	h->allspans = s;
}

// Initialize the heap; fetch memory using alloc.
void
MHeap_Init(MHeap *h, void *(*alloc)(uintptr))
{
	uint32 i;

	FixAlloc_Init(&h->spanalloc, sizeof(MSpan), alloc, RecordSpan, h);
	FixAlloc_Init(&h->cachealloc, sizeof(MCache), alloc, nil, nil);
	MHeapMap_Init(&h->map, alloc);
	// h->mapcache needs no init
	for(i=0; i<nelem(h->free); i++)
		MSpanList_Init(&h->free[i]);
	MSpanList_Init(&h->large);
	for(i=0; i<nelem(h->central); i++)
		MCentral_Init(&h->central[i], i);
}

// Allocate a new span of npage pages from the heap
// and record its size class in the HeapMap and HeapMapCache.
MSpan*
MHeap_Alloc(MHeap *h, uintptr npage, int32 sizeclass, int32 acct)
{
	MSpan *s;

	lock(h);
	mstats.heap_alloc += m->mcache->local_alloc;
	m->mcache->local_alloc = 0;
	s = MHeap_AllocLocked(h, npage, sizeclass);
	if(s != nil) {
		mstats.inuse_pages += npage;
		if(acct)
			mstats.heap_alloc += npage<<PageShift;
	}
	unlock(h);
	return s;
}

static MSpan*
MHeap_AllocLocked(MHeap *h, uintptr npage, int32 sizeclass)
{
	uintptr n;
	MSpan *s, *t;

	// Try in fixed-size lists up to max.
	for(n=npage; n < nelem(h->free); n++) {
		if(!MSpanList_IsEmpty(&h->free[n])) {
			s = h->free[n].next;
			goto HaveSpan;
		}
	}

	// Best fit in list of large spans.
	if((s = MHeap_AllocLarge(h, npage)) == nil) {
		if(!MHeap_Grow(h, npage))
			return nil;
		if((s = MHeap_AllocLarge(h, npage)) == nil)
			return nil;
	}

HaveSpan:
	// Mark span in use.
	if(s->state != MSpanFree)
		throw("MHeap_AllocLocked - MSpan not free");
	if(s->npages < npage)
		throw("MHeap_AllocLocked - bad npages");
	MSpanList_Remove(s);
	s->state = MSpanInUse;

	if(s->npages > npage) {
		// Trim extra and put it back in the heap.
		t = FixAlloc_Alloc(&h->spanalloc);
		MSpan_Init(t, s->start + npage, s->npages - npage);
		s->npages = npage;
		MHeapMap_Set(&h->map, t->start - 1, s);
		MHeapMap_Set(&h->map, t->start, t);
		MHeapMap_Set(&h->map, t->start + t->npages - 1, t);
		t->state = MSpanInUse;
		MHeap_FreeLocked(h, t);
	}

	// Record span info, because gc needs to be
	// able to map interior pointer to containing span.
	s->sizeclass = sizeclass;
	for(n=0; n<npage; n++)
		MHeapMap_Set(&h->map, s->start+n, s);
	return s;
}

// Allocate a span of exactly npage pages from the list of large spans.
static MSpan*
MHeap_AllocLarge(MHeap *h, uintptr npage)
{
	return BestFit(&h->large, npage, nil);
}

// Search list for smallest span with >= npage pages.
// If there are multiple smallest spans, take the one
// with the earliest starting address.
static MSpan*
BestFit(MSpan *list, uintptr npage, MSpan *best)
{
	MSpan *s;

	for(s=list->next; s != list; s=s->next) {
		if(s->npages < npage)
			continue;
		if(best == nil
		|| s->npages < best->npages
		|| (s->npages == best->npages && s->start < best->start))
			best = s;
	}
	return best;
}

// Try to add at least npage pages of memory to the heap,
// returning whether it worked.
static bool
MHeap_Grow(MHeap *h, uintptr npage)
{
	uintptr ask;
	void *v;
	MSpan *s;

	// Ask for a big chunk, to reduce the number of mappings
	// the operating system needs to track; also amortizes
	// the overhead of an operating system mapping.
	// For Native Client, allocate a multiple of 64kB (16 pages).
	npage = (npage+15)&~15;
	ask = npage<<PageShift;
	if(ask < HeapAllocChunk)
		ask = HeapAllocChunk;

	v = SysAlloc(ask);
	if(v == nil) {
		if(ask > (npage<<PageShift)) {
			ask = npage<<PageShift;
			v = SysAlloc(ask);
		}
		if(v == nil)
			return false;
	}

	if((byte*)v < h->min || h->min == nil)
		h->min = v;
	if((byte*)v+ask > h->max)
		h->max = (byte*)v+ask;

	// NOTE(rsc): In tcmalloc, if we've accumulated enough
	// system allocations, the heap map gets entirely allocated
	// in 32-bit mode.  (In 64-bit mode that's not practical.)
	if(!MHeapMap_Preallocate(&h->map, ((uintptr)v>>PageShift) - 1, (ask>>PageShift) + 2)) {
		SysFree(v, ask);
		return false;
	}

	// Create a fake "in use" span and free it, so that the
	// right coalescing happens.
	s = FixAlloc_Alloc(&h->spanalloc);
	MSpan_Init(s, (uintptr)v>>PageShift, ask>>PageShift);
	MHeapMap_Set(&h->map, s->start, s);
	MHeapMap_Set(&h->map, s->start + s->npages - 1, s);
	s->state = MSpanInUse;
	MHeap_FreeLocked(h, s);
	return true;
}

// Look up the span at the given page number.
// Page number is guaranteed to be in map
// and is guaranteed to be start or end of span.
MSpan*
MHeap_Lookup(MHeap *h, PageID p)
{
	return MHeapMap_Get(&h->map, p);
}

// Look up the span at the given page number.
// Page number is *not* guaranteed to be in map
// and may be anywhere in the span.
// Map entries for the middle of a span are only
// valid for allocated spans.  Free spans may have
// other garbage in their middles, so we have to
// check for that.
MSpan*
MHeap_LookupMaybe(MHeap *h, PageID p)
{
	MSpan *s;

	s = MHeapMap_GetMaybe(&h->map, p);
	if(s == nil || p < s->start || p - s->start >= s->npages)
		return nil;
	if(s->state != MSpanInUse)
		return nil;
	return s;
}

// Free the span back into the heap.
void
MHeap_Free(MHeap *h, MSpan *s, int32 acct)
{
	lock(h);
	mstats.heap_alloc += m->mcache->local_alloc;
	m->mcache->local_alloc = 0;
	mstats.inuse_pages -= s->npages;
	if(acct)
		mstats.heap_alloc -= s->npages<<PageShift;
	MHeap_FreeLocked(h, s);
	unlock(h);
}

static void
MHeap_FreeLocked(MHeap *h, MSpan *s)
{
	MSpan *t;

	if(s->state != MSpanInUse || s->ref != 0) {
		printf("MHeap_FreeLocked - span %p ptr %p state %d ref %d\n", s, s->start<<PageShift, s->state, s->ref);
		throw("MHeap_FreeLocked - invalid free");
	}
	s->state = MSpanFree;
	MSpanList_Remove(s);

	// Coalesce with earlier, later spans.
	if((t = MHeapMap_Get(&h->map, s->start - 1)) != nil && t->state != MSpanInUse) {
		s->start = t->start;
		s->npages += t->npages;
		MHeapMap_Set(&h->map, s->start, s);
		MSpanList_Remove(t);
		t->state = MSpanDead;
		FixAlloc_Free(&h->spanalloc, t);
	}
	if((t = MHeapMap_Get(&h->map, s->start + s->npages)) != nil && t->state != MSpanInUse) {
		s->npages += t->npages;
		MHeapMap_Set(&h->map, s->start + s->npages - 1, s);
		MSpanList_Remove(t);
		t->state = MSpanDead;
		FixAlloc_Free(&h->spanalloc, t);
	}

	// Insert s into appropriate list.
	if(s->npages < nelem(h->free))
		MSpanList_Insert(&h->free[s->npages], s);
	else
		MSpanList_Insert(&h->large, s);

	// TODO(rsc): IncrementalScavenge() to return memory to OS.
}

// Initialize a new span with the given start and npages.
void
MSpan_Init(MSpan *span, PageID start, uintptr npages)
{
	span->next = nil;
	span->prev = nil;
	span->start = start;
	span->npages = npages;
	span->freelist = nil;
	span->ref = 0;
	span->sizeclass = 0;
	span->state = 0;
}

// Initialize an empty doubly-linked list.
void
MSpanList_Init(MSpan *list)
{
	list->state = MSpanListHead;
	list->next = list;
	list->prev = list;
}

void
MSpanList_Remove(MSpan *span)
{
	if(span->prev == nil && span->next == nil)
		return;
	span->prev->next = span->next;
	span->next->prev = span->prev;
	span->prev = nil;
	span->next = nil;
}

bool
MSpanList_IsEmpty(MSpan *list)
{
	return list->next == list;
}

void
MSpanList_Insert(MSpan *list, MSpan *span)
{
	if(span->next != nil || span->prev != nil)
		throw("MSpanList_Insert");
	span->next = list->next;
	span->prev = list;
	span->next->prev = span;
	span->prev->next = span;
}
