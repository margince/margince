// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package briefs

// A rank computed from a withheld input is a WRONG answer, not a short one.
//
// Every seat on a deal is a `deal_stakeholder` edge, so a caller without the
// relationship grant reads no stakeholders for any deal and the warmth factor
// floors across the whole queue. Silently, the brief then hands back an order
// that is not the order — deals sit lower than they are, and nothing on the
// page says the reader is missing the reason.

import (
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// edgeBlindPerms is RepPerms with the relationship grant taken away and
// everything else the brief needs left in place — the caller who may read the
// deals and not who sits on them. A deep copy, because a plain struct copy
// shares the Objects map with every other suite using the fixture.
func edgeBlindPerms() principal.Permissions {
	perms := integration.RepPerms
	perms.Objects = make(map[string]principal.ObjectGrant, len(integration.RepPerms.Objects))
	for object, grant := range integration.RepPerms.Objects {
		if object == "relationship" {
			continue
		}
		perms.Objects[object] = grant
	}
	return perms
}

func TestABriefNamesTheFactorItCouldNotRead(t *testing.T) {
	b := setupBrief(t)

	blind, err := b.engine.SnapshotRun(b.As(b.Rep1, []ids.UUID{b.Team1}, edgeBlindPerms()), briefClock)
	if err != nil {
		t.Fatalf("snapshot for a caller without the edge grant: %v", err)
	}
	if !slices.Contains(blind.FactorsOmitted, "warmth") {
		t.Errorf("the run omits %v, want warmth named — the factor floored on every deal and the "+
			"queue came back ordered as though that were a reading", blind.FactorsOmitted)
	}
}

// The other half, and the half that makes the first mean something: a caller
// who CAN read the seats gets an empty list, never a nil one. Empty and
// withheld are different answers, and a client that had to tell them apart by
// guessing would render every brief as withheld or none of them.
func TestABriefThatReadEveryFactorOmitsNone(t *testing.T) {
	b := setupBrief(t)

	seeing, err := b.engine.SnapshotRun(b.As(b.Rep1, []ids.UUID{b.Team1}, integration.RepPerms), briefClock)
	if err != nil {
		t.Fatalf("snapshot for a caller holding the edge grant: %v", err)
	}
	if len(seeing.FactorsOmitted) != 0 {
		t.Errorf("a run that read everything omits %v, want nothing — if this names warmth too, "+
			"the sibling test proves the caller was refused rather than the factor withheld",
			seeing.FactorsOmitted)
	}
}
