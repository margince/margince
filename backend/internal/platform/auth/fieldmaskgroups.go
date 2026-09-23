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

// The names the table below repeats. maskObjDeal is DERIVED from the table
// constant rather than respelt: the two vocabularies coincide on that name, and
// two constants for one string is the drift this package exists to refuse. An
// offer has no table here, so it gets a name of its own.
const (
	maskObjDeal          = tableDeal
	maskObjCommission    = "commission"
	maskFieldAmountMinor = "amount_minor"
	maskFieldCurrency    = "currency"
)

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
	{maskObjDeal, maskFieldAmountMinor}: {{maskObjDeal, "expected_arr_minor"}, {maskObjDeal, maskFieldCurrency}},
	{maskObjDeal, "expected_arr_minor"}: {{maskObjDeal, maskFieldAmountMinor}, {maskObjDeal, maskFieldCurrency}},
	// A commission entry says the tier three ways: frozen at accrual, as the
	// rate it became (tier2_20 is 2000bps), and as the amount over the basis it
	// produced that rate from. None of the three is a member an administrator
	// configures — each is withheld because the partner's mask reaches it, and a
	// second configuration for one fact is how the leak comes back.
	//
	// The ledger has no rendering for these: the rate and the amount are
	// required integers on the wire, so it answers this group by leaving the ROW
	// out of its reads rather than the column out of the row.
	{"partner", "margin_tier"}: {
		{maskObjCommission, "margin_tier_at_accrual"},
		{maskObjCommission, "rate_bps"},
		{maskObjCommission, maskFieldAmountMinor},
	},
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

// String spells a subject the way the maskable-field catalog spells an offered
// pair, so a reader comparing the two compares like against like.
func (s maskSubject) String() string { return s.object + " " + s.field }

// MaskGroupCrossings reports the group entries that reach ANOTHER object: each
// configured pair, against what it withholds on a record that does not own the
// fact.
//
// Exported for the gates holding this closure against the maskable-field
// catalog, which cannot read an unexported table and must not keep a second
// copy of one. Crossings alone, because only they raise the question — a
// consequence inside one object is that object's own field to offer.
func MaskGroupCrossings() map[string][]string {
	crossings := make(map[string][]string, len(maskGroups))
	for configured, members := range maskGroups {
		for _, m := range members {
			if m.object != configured.object {
				crossings[configured.String()] = append(crossings[configured.String()], m.String())
			}
		}
	}
	for _, members := range crossings {
		slices.Sort(members)
	}
	return crossings
}

// MaskConditionAnswerable reports whether a mask on the object may be
// conditioned on write authority at all. The question needs an owner and a
// grant, which only a shareable record's rows carry; configured anywhere else
// the condition never lifts, so an operator who asked for "hidden on the rows
// they cannot change" gets the field hidden on every row instead.
//
// Exported for the gate refusing an undeclared such pair in the catalog.
func MaskConditionAnswerable(object string) bool { return shareableObject(object) }
