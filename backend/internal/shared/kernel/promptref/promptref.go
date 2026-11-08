// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package promptref names the records one model request shows by short refs,
// such as m1 or d2. It reads a ref in the reply back to its record id.
//
// A model never sees a record's UUID. A small model copies a short ref it was
// given. It garbles a 36-character id, cites it in the wrong field or writes
// it into prose. A ref names one record of one request only. The reply is read
// back through the same Refs, and a ref it never minted is refused.
package promptref

import "strconv"

// Refs is the ref table of one request: one prefix, refs minted in the order
// the records were shown.
type Refs struct {
	prefix string
	refs   []string
	idOf   map[string]string
	refOf  map[string]string
}

// New starts a table whose refs read prefix1, prefix2, and so on, and mints a
// ref for each id given, in order.
func New(prefix string, ids ...string) *Refs {
	r := &Refs{prefix: prefix, idOf: map[string]string{}, refOf: map[string]string{}}
	for _, id := range ids {
		r.For(id)
	}
	return r
}

// For returns the ref of id, minting the next one the first time id is seen.
func (r *Refs) For(id string) string {
	if ref, ok := r.refOf[id]; ok {
		return ref
	}
	ref := r.prefix + strconv.Itoa(len(r.refs)+1)
	r.refs = append(r.refs, ref)
	r.refOf[id] = ref
	r.idOf[ref] = id
	return ref
}

// RefOf returns the ref of id without minting one, and false for an id this
// table has not shown.
func (r *Refs) RefOf(id string) (string, bool) {
	ref, ok := r.refOf[id]
	return ref, ok
}

// ID returns the record id ref names, and false for a ref this table never
// minted.
func (r *Refs) ID(ref string) (string, bool) {
	id, ok := r.idOf[ref]
	return id, ok
}

// All returns every ref minted, in minting order: the closed set a reply may
// cite.
func (r *Refs) All() []string {
	return append([]string(nil), r.refs...)
}
