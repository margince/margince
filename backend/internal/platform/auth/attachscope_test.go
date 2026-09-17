// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

// The attach direction as the predicate SPELLS it.
//
// One arm moves and the rest must not. A share admits an attach only at
// `write`; the own/team arm, capture privacy and the unbounded shortcut are the
// same predicate the read direction renders — which is the whole reason this is
// not the write-authority predicate, whose own/team arm answers a different
// question (may a rep file work against another team's record: yes, on purpose).

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// attachSQL renders the attach predicate for one table, with the arg registrar
// the production callers use.
func attachSQL(p principal.Principal, table string) string {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	return AttachPredicate(p, table, arg)("t")
}

// The difference, stated as a diff rather than as two independent claims: the
// read direction counts every live grant, the attach direction counts only a
// write one, and nothing else about the SQL moves.
func TestAnAttachCountsOnlyAWriteShare(t *testing.T) {
	p := human(principal.RowScopeTeam)
	read, attach := rendered(p, tableCompany), attachSQL(p, tableCompany)

	if strings.Contains(read, "rg.access") {
		t.Errorf("the READ predicate narrows on the grant level; a read share must open the record:\n%s", read)
	}
	if !strings.Contains(attach, "rg.access = 'write'") {
		t.Errorf("the ATTACH predicate admits a read-only share:\n%s", attach)
	}
	// Everything but that clause is the same predicate. Asserting the
	// equality rather than the two halves is what catches a change that
	// tightened the owner arm as a side effect — the mistake the ruling behind
	// this one was corrected for.
	if stripped := strings.Replace(attach, "\n\t\t     AND rg.access = 'write'", "", 1); stripped != read {
		t.Errorf("the attach predicate differs from the read one by more than the grant level.\nread:   %s\nattach: %s", read, stripped)
	}
}

// A caller who reads every row of the table needs no grant arm, so neither
// direction renders one — and the attach predicate must not grow a narrowing
// that applies to a clause that is not there.
func TestAnUnboundedCallerAttachesWithoutAShare(t *testing.T) {
	p := human(principal.RowScopeAll)
	if got := attachSQL(p, tableDeal); strings.Contains(got, "record_grant") {
		t.Errorf("an all-scope caller's attach predicate consults a share: %s", got)
	}
}

// Capture privacy is unchanged by the direction: an owner-private row answers
// to its owner, and a share is what widens it — at `write` for an attach.
func TestAnAttachStillFacesCapturePrivacy(t *testing.T) {
	got := attachSQL(human(principal.RowScopeAll), tableContact)
	if !strings.Contains(got, "t.visibility") {
		t.Errorf("the attach predicate skips capture privacy: %s", got)
	}
	if !strings.Contains(got, "rg.access = 'write'") {
		t.Errorf("a capture-private row admits an attach through a read share: %s", got)
	}
}
