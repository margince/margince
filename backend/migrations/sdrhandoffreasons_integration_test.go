// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

// Every handoff transition has a reason somebody can pick.
//
// `sdr_handoff_reason` is a system catalogue the migration seeds, and the write
// path requires a reason whose `applies_to` matches the transition being made.
// Seeded for one of the two and not the other, a transition would have an empty
// list in front of a rep and no way to complete — and nothing would fail,
// because the store test seeds a reason of its own.
//
// It does so deliberately: the harness truncates before each case, so a test
// reading the migration's rows there would be asserting that truncation had not
// happened, which is a fact about the harness rather than about handoffs. That
// left the claim about the SEED unmade anywhere, which is what this closes.
//
// THE VOCABULARY IS READ FROM THE DATABASE, not listed here. The transitions a
// reason may apply to are the ones `sdr_handoff_reason_applies_to` admits, so a
// third transition added to that constraint is covered by this the day it
// lands — where a list would pass, having never asked about the new one.

import (
	"context"
	"regexp"
	"testing"
)

// appliesToValues reads the transition vocabulary out of the CHECK constraint
// itself. A list here would be a second spelling of the column's own, and the
// day they parted the test would be asking about transitions the database does
// not have.
var appliesToValues = regexp.MustCompile(`'([a-z_]+)'`)

func TestTheSeededHandoffReasonsCoverBothTransitions(t *testing.T) {
	ownerDSN, _ := dsns(t)
	owner := connect(t, ownerDSN)
	headSchema(t, owner)
	ctx := context.Background()

	var clause string
	if err := owner.QueryRow(ctx, `
		SELECT pg_get_constraintdef(oid) FROM pg_constraint
		 WHERE conname = 'sdr_handoff_reason_applies_to'`).Scan(&clause); err != nil {
		t.Fatalf("reading the applies_to constraint: %v", err)
	}
	matches := appliesToValues.FindAllStringSubmatch(clause, -1)
	// A constraint this cannot read leaves nothing to check, and reports PASS
	// exactly like a catalogue seeded for every transition.
	if len(matches) < 2 {
		t.Fatalf("read %d transition(s) out of %q — the constraint's shape has changed and this test "+
			"is now asking about nothing", len(matches), clause)
	}

	for _, match := range matches {
		transition := match[1]
		var usable int
		if err := owner.QueryRow(ctx, `
			SELECT count(*) FROM sdr_handoff_reason
			 WHERE applies_to = $1 AND active AND system`, transition).Scan(&usable); err != nil {
			t.Fatalf("counting seeded reasons for %s: %v", transition, err)
		}
		if usable == 0 {
			t.Errorf("no active system reason is seeded for the %q transition, so a rep making it is "+
				"offered an empty list and cannot complete the handoff at all", transition)
		}
	}
}
