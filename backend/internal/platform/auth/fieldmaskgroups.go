// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

// What ELSE a configured mask withholds. A field is seldom a fact on its own —
// the money on a deal is one fact in three columns — and an operator masking
// the amount asked for the figure to be unreadable, not for two of its three
// spellings to go out. The relation is declared here once, where every
// rendering of a mask already asks, rather than inside the one module whose
// wire read happened to need it first.

import (
	"slices"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// maskSubject is one (object, field) pair as a mask NAMES it: the RBAC object
// an administrator configures under, and the wire field, neither of which is
// renamed by renaming the table or column they coincide with.
type maskSubject struct{ object, field string }

// maskGroups says what a mask on the key also withholds.
//
// Directed, not symmetric. A currency beside a withheld amount reads as a
// priced deal with its figure missing, so masking the amount takes the currency
// with it; a withheld currency says nothing about the figure, so it takes
// nothing and the mask hides no more than was asked for.
//
// Both sides are pairs because a group may cross OBJECTS: a fact republished on
// another record is disclosed as completely as one left where it was written,
// and a partner's margin tier is republished on every commission entry accrued
// under it. A crossing hangs only off a mask that withholds on every row —
// per-row conditioning is write authority over the mask's OWN record, and "the
// partners you may write" cannot say which commission entries to withhold.
var maskGroups = map[maskSubject][]maskSubject{
	// An ARR left standing beside a withheld one-off amount discloses the size
	// of the deal the mask was meant to hide, so the two travel with the
	// currency that would otherwise still read as a priced deal.
	{"deal", "amount_minor"}:        {{"deal", "expected_arr_minor"}, {"deal", "currency"}},
	{"deal", "expected_arr_minor"}:  {{"deal", "amount_minor"}, {"deal", "currency"}},
	{"product", "unit_price_minor"}: {{"product", "currency"}},
	// An offer's money surface is one thing or nothing. Quantity and discount
	// stay readable, so a line net gives the unit price back by division, and a
	// one-line offer's gross simply is that price plus its tax.
	{"offer", "unit_price_minor"}: {
		{"offer", "line_net_minor"}, {"offer", "line_tax_minor"}, {"offer", "line_total_minor"},
		{"offer", "net_minor"}, {"offer", "tax_minor"}, {"offer", "gross_minor"},
		{"offer", "net_tcv_minor"}, {"offer", "arr_minor"}, {"offer", "currency"},
	},
	{"partner", "margin_tier"}: {{"commission", "margin_tier_at_accrual"}},
}

// withheldSubjects is the closure of ONE configured mask: the pair it names,
// and everything that would give that pair back. Its own subject is first in
// the answer so no caller has to remember to add it.
func withheldSubjects(object, field string) []maskSubject {
	own := maskSubject{object, field}
	return append([]maskSubject{own}, maskGroups[own]...)
}

// masksWithholding answers which of this principal's configured masks withhold
// (object, field): the one naming it, and any whose group reaches it. Empty
// means the caller reads the field.
//
// It returns every such mask rather than the first, because two masks can name
// one field under different conditions and the STRICTER decides — a field
// readable on some rows through one mask and on no rows through another is
// readable on none. Which is stricter is the caller's to resolve, since only
// the caller knows what it can render.
func masksWithholding(p principal.Principal, object, field string) []principal.FieldMask {
	if Unbounded(p) {
		return nil
	}
	subject := maskSubject{object, field}
	var out []principal.FieldMask
	for _, m := range p.Permissions.FieldMasks {
		if slices.Contains(withheldSubjects(m.Object, m.Field), subject) {
			out = append(out, m)
		}
	}
	return out
}

// shareableObject reports whether write authority over the object's rows is a
// question that can be asked at all — a mask conditioned on it elsewhere has no
// owner and no grant to resolve against.
//
// The mask vocabulary and the table vocabulary coincide on every name in the
// set today and are still two vocabularies: an object is what an administrator
// configures a mask under, a table is what carries the columns the write arm
// reads.
func shareableObject(object string) bool {
	return shareableTables[object]
}
