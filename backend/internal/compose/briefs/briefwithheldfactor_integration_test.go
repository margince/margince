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
	// Empty AND non-nil, checked separately. len() cannot tell the two apart,
	// and the distinction is the point: nil marshals to `null` under a field
	// documented never to be one, so a client would have to decide for itself
	// whether the server meant "nothing withheld" or "not told".
	if seeing.FactorsOmitted == nil {
		t.Error("a run that read everything omits nil, want an empty list — the two are different " +
			"answers on the wire, and only one of them is the one this run has to give")
	}
	if len(seeing.FactorsOmitted) != 0 {
		t.Errorf("a run that read everything omits %v, want nothing — if this names warmth too, "+
			"the sibling test proves the caller was refused rather than the factor withheld",
			seeing.FactorsOmitted)
	}
}

// The OTHER way to be refused the warmth factor.
//
// Warmth needs two grants, not one: the seat edge to learn who is on the deal,
// and the person grant to score them. A caller holding the first and not the
// second reads the stakeholders and then gets the same refusal for every one of
// them — so the factor floors across the whole queue exactly as it does for an
// edge-blind caller, and the reader is owed the same sentence.
//
// The two refusals are told apart by their SENTINEL, which is the contract the
// whole tree holds: a row-scope miss answers ErrNotFound so existence stays
// hidden, and an object denial answers ErrPermissionDenied. Reading them as one
// thing is what let this case fall through — a stakeholder outside the caller's
// rows should floor that deal and leave the rest alone.
func TestABriefNamesWarmthWhenItMayReadSeatsButNotPeople(t *testing.T) {
	b := setupBrief(t)

	// The fixture seeds no seats, and this case only exists once a seat is read
	// and then cannot be scored — so without one the loop below never runs and
	// the test passes on an empty map.
	seat := b.SeedPerson(t, "Ilse Stakeholder", &b.Rep1)
	b.WsExec(t, `INSERT INTO relationship (kind, deal_id, person_id, source, captured_by)
		VALUES ('deal_stakeholder', $1, $2, 'manual', 'human:x')`, b.dealA, seat)

	perms := integration.RepPerms
	perms.Objects = make(map[string]principal.ObjectGrant, len(integration.RepPerms.Objects))
	for object, grant := range integration.RepPerms.Objects {
		if object == "person" {
			continue
		}
		perms.Objects[object] = grant
	}
	run, err := b.engine.SnapshotRun(b.As(b.Rep1, []ids.UUID{b.Team1}, perms), briefClock)
	if err != nil {
		t.Fatalf("snapshot for a caller who may read seats but not people: %v", err)
	}
	if !slices.Contains(run.FactorsOmitted, "warmth") {
		t.Errorf("the run omits %v, want warmth named — the caller reached every stakeholder and "+
			"could score none of them, so the factor floored across the whole queue",
			run.FactorsOmitted)
	}
}
