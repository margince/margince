// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

// What a mask withholds on ANOTHER record. The fields one mask takes with it
// where it is configured are the fieldmask package's table, which every caller
// here expands through; a fact republished on a second record is disclosed as
// completely as one left where it was written, and a tier-0 table keyed by one
// object cannot say so.

import (
	"slices"

	"github.com/margince/margince/backend/internal/shared/kernel/fieldmask"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// maskSubject is one (object, field) pair as a mask NAMES it: the RBAC object
// an administrator configures under, and the wire field, neither of which is
// renamed by renaming the table or column they coincide with.
type maskSubject struct{ object, field string }

// A commission has no table in this package, so it gets a name of its own.
const (
	maskObjPartner       = "partner"
	maskObjCommission    = "commission"
	maskFieldAmountMinor = "amount_minor"
)

// maskCrossings says what a mask on the key withholds on a record that does not
// own the fact.
//
// A crossing hangs only off a mask that withholds on every row: per-row
// conditioning is write authority over the mask's OWN record, and "the partners
// you may write" cannot say which commission entries to withhold.
var maskCrossings = map[maskSubject][]maskSubject{
	// A commission entry says the tier three ways: frozen at accrual, as the
	// rate it became (tier2_20 is 2000bps), and as the amount over the basis it
	// produced that rate from. None of the three is a member an administrator
	// configures — each is withheld because the partner's mask reaches it, and a
	// second configuration for one fact is how the leak comes back.
	//
	// The ledger has no rendering for these: the rate and the amount are
	// required integers on the wire, so it answers this group by leaving the ROW
	// out of its reads rather than the column out of the row.
	{maskObjPartner, "margin_tier"}: {
		{maskObjCommission, "margin_tier_at_accrual"},
		{maskObjCommission, "rate_bps"},
		{maskObjCommission, maskFieldAmountMinor},
	},
}

// reachedFields names what ONE configured mask withholds on the object asked
// about: the field it names where it is configured there, and the members of
// its crossing that land there when it is not. What each of those drags along
// beside it is the fieldmask package's answer, so a caller hands this to it.
func reachedFields(m principal.FieldMask, object string) []string {
	if m.Object == object {
		return []string{m.Field}
	}
	var out []string
	for _, s := range maskCrossings[maskSubject{m.Object, m.Field}] {
		if s.object == object {
			out = append(out, s.field)
		}
	}
	return out
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
	var out []principal.FieldMask
	for _, m := range p.Permissions.FieldMasks {
		if fieldmask.Covers(object, reachedFields(m, object), field) {
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

// MaskGroupCrossings reports each configured pair against what it withholds on
// a record that does not own the fact.
//
// Exported for the gates holding this closure against the maskable-field
// catalog, which cannot read an unexported table and must not keep a second
// copy of one. Crossings alone, because only they raise the question — a
// consequence inside one object is that object's own field to offer.
func MaskGroupCrossings() map[string][]string {
	crossings := make(map[string][]string, len(maskCrossings))
	for configured, members := range maskCrossings {
		for _, m := range members {
			crossings[configured.String()] = append(crossings[configured.String()], m.String())
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
