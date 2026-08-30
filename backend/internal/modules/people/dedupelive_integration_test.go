// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package people

// A duplicate decision outlives the records it is about.
//
// Archiving one of two duplicates is a reasonable way to resolve a pair — for a
// company created twice it is the most natural one, because it needs no merge
// decision about which fields survive. Today that leaves the decision open
// forever: the candidate carries its own archived_at and nothing sweeps it when
// a SUBJECT is archived, so the pair keeps the confidence it was filed with and
// holds its rank in a lane that shows ten by score.
//
// The failure is silent in the way that matters: the endpoint answers 200, the
// lane renders ten rows, and every other test passes. It shows only if you ask
// whether the rows still point at anything. So these ask.

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// asArchiver is the queue's own reader plus the grant that retires a record.
//
// Archiving is a separate authority from working the queue, and the shared
// dedupeEnv principal deliberately holds only create/read/update — widening it
// would quietly change what every other test in this package proves. So the one
// capability these cases need is added here, over the same seat.
func asArchiver(e *dedupeEnv) context.Context {
	ctx := e.as()
	p, _ := principal.Actor(ctx)
	objects := map[string]principal.ObjectGrant{}
	for object, grant := range p.Permissions.Objects {
		grant.Delete = true
		objects[object] = grant
	}
	p.Permissions.Objects = objects
	return principal.WithActor(ctx, p)
}

// Archiving one side of a pair closes the decision about it.
//
// Not disposed — nobody judged the pair, and `not_a_duplicate` would be a lie
// that lasts: it suppresses the pair from every future sweep, so a company later
// restored would never be offered against its twin again. Archived is the same
// answer a merge already gives a stale pair (retireStaleCandidates).
func TestArchivingOneSideClosesTheDuplicateDecision(t *testing.T) {
	e := setupDedupe(t)
	ctx := asArchiver(e)
	first, _ := seedOrgPair(ctx, t, e)
	if got := len(openCandidates(ctx, t, e, "organization")); got != 1 {
		t.Fatalf("the seed left %d open candidates, want 1", got)
	}

	if _, err := e.store.ArchiveOrganization(ctx, ids.From[ids.OrganizationKind](first), nil); err != nil {
		t.Fatalf("archiving one side: %v", err)
	}

	if got := len(openCandidates(ctx, t, e, "organization")); got != 0 {
		t.Errorf("after archiving one side the queue still serves %d decisions, want 0 — "+
			"they are about a company nobody can open, and they hold their rank by "+
			"confidence for good", got)
	}
}

// The badge agrees with the lane.
//
// A count that outlived its rows would send a reader to a queue that has nothing
// in it, which is the same defect wearing a number.
func TestTheOpenCountDropsWhenASubjectIsArchived(t *testing.T) {
	e := setupDedupe(t)
	ctx := asArchiver(e)
	first, _ := seedOrgPair(ctx, t, e)
	before, err := e.store.CountOpenDedupeCandidates(ctx)
	if err != nil {
		t.Fatalf("counting before: %v", err)
	}
	if before != 1 {
		t.Fatalf("the seed counted %d open candidates, want 1", before)
	}

	if _, err := e.store.ArchiveOrganization(ctx, ids.From[ids.OrganizationKind](first), nil); err != nil {
		t.Fatalf("archiving one side: %v", err)
	}

	after, err := e.store.CountOpenDedupeCandidates(ctx)
	if err != nil {
		t.Fatalf("counting after: %v", err)
	}
	if after != 0 {
		t.Errorf("the badge still counts %d open decisions about an archived company, want 0", after)
	}
}

// A pair nobody touched is still served.
//
// The control. A sweep that closed everything would pass the two above while
// emptying the queue of exactly the work it exists to show.
func TestAPairOfLiveCompaniesIsStillServed(t *testing.T) {
	e := setupDedupe(t)
	ctx := asArchiver(e)
	seedOrgPair(ctx, t, e)

	// A DIFFERENT company is archived; the pair is about neither of its sides.
	bystander, err := e.store.CreateOrganization(ctx, CreateOrganizationInput{
		DisplayName: "Unrelated Holdings", Source: "manual",
		Domains: []OrgDomainInput{{Domain: "unrelated.test", IsPrimary: true}},
	})
	if err != nil {
		t.Fatalf("seeding the bystander: %v", err)
	}
	if _, err := e.store.ArchiveOrganization(ctx,
		ids.From[ids.OrganizationKind](ids.UUID(bystander.Id)), nil); err != nil {
		t.Fatalf("archiving the bystander: %v", err)
	}

	if got := len(openCandidates(ctx, t, e, "organization")); got != 1 {
		t.Errorf("a pair of live companies is served %d times, want 1 — archiving an "+
			"unrelated record has closed a decision that still has both its subjects", got)
	}
}

// A person archived on their own closes their duplicate decisions too.
//
// The same rule, on the other record type the queue serves. Organizations are
// where it was measured; nothing about the cause is specific to them.
func TestArchivingAPersonClosesTheirDuplicateDecision(t *testing.T) {
	e := setupDedupe(t)
	ctx := asArchiver(e)
	incumbent, _ := seedPersonPair(ctx, t, e,
		"Jane Roe", "jane@live.test", "Jayne Roe", "jayne@live.test", "live.test")
	if got := len(openCandidates(ctx, t, e, "person")); got != 1 {
		t.Fatalf("the seed left %d open candidates, want 1", got)
	}

	if _, err := e.store.ArchivePerson(ctx, ids.From[ids.PersonKind](incumbent), nil); err != nil {
		t.Fatalf("archiving the person: %v", err)
	}

	if got := len(openCandidates(ctx, t, e, "person")); got != 0 {
		t.Errorf("after archiving one side the queue still serves %d decisions about a "+
			"person nobody can open, want 0", got)
	}
}

// A disqualified lead's duplicate decision closes with it.
//
// The third record type the queue serves, and the one whose row-scope clause is
// EMPTY for an all-scope reader — lead is not capture-private, where person and
// organization are. So this is the case where the liveness term stands on its
// own rather than riding a scope predicate that happens to be there anyway.
//
// Disqualifying is the lead's own archive: it stamps archived_at, and it is what
// a rep does to a lead that turned out to be nothing.
func TestADisqualifiedLeadsDuplicateDecisionClosesWithIt(t *testing.T) {
	e := setupDedupe(t)
	ctx := asArchiver(e)
	first := e.createLead(ctx, t, "Jonas Petersen", "jonas@nordwind.test", "Nordwind Logistik")
	e.createLead(ctx, t, "Jonas Peterson", "", "Nordwind Logistik")
	if got := len(openCandidates(ctx, t, e, entityLead)); got != 1 {
		t.Fatalf("the seed left %d open lead candidates, want 1", got)
	}

	if _, err := e.store.DisqualifyLead(ctx, first, DisqualifyLeadInput{}); err != nil {
		t.Fatalf("disqualifying the lead: %v", err)
	}

	if got := len(openCandidates(ctx, t, e, entityLead)); got != 0 {
		t.Errorf("after one side was disqualified the queue still serves %d decisions "+
			"about a lead nobody can open, want 0", got)
	}
}
