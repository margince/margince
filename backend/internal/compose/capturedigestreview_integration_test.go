// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// What the digest says is waiting, against a real database.
//
// The two counts name records whose visibility is per-reader — a duplicate pair
// needs BOTH sides visible, a staged proposal needs the authority to decide it —
// so the question this suite answers is whether the stored payload reports what
// the READER could open or what the workspace happens to hold. A count that
// travels workspace-wide is an existence oracle: the same number for everybody,
// moving as records they cannot see come and go.

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/authz"
	"github.com/margince/margince/backend/internal/shared/ports/authz/authztest"
)

// decidesNothing is the harness authority with no grant that admits a staged
// proposal or a duplicate pair: the reader is connected, so they get a digest,
// and neither queue is theirs to see.
type decidesNothing struct{ backfillAuthority }

func (decidesNothing) EffectiveRBAC(context.Context, ids.UUID, ids.UUID) (authz.RBAC, error) {
	return authz.RBAC{Permissions: principal.Permissions{
		Objects:  map[string]principal.ObjectGrant{"activity": {Create: true, Read: true}},
		RowScope: principal.RowScopeAll,
	}}, nil
}

// decidesDeals holds exactly what a close-date proposal asks of its decider.
type decidesDeals struct{ backfillAuthority }

func (decidesDeals) EffectiveRBAC(context.Context, ids.UUID, ids.UUID) (authz.RBAC, error) {
	return authz.RBAC{Permissions: principal.Permissions{
		Objects: map[string]principal.ObjectGrant{
			"activity": {Create: true, Read: true},
			"deal":     {Read: true, Update: true},
		},
		RowScope: principal.RowScopeAll,
	}}, nil
}

// seedDecidableDeal creates the deal a staged proposal points at, with the
// pipeline and stage a deal cannot exist without.
//
// The decision's target-visibility probe reads that deal, so a proposal aimed
// at nothing would be refused before the count is ever reached — and the test
// would then pass for the wrong reason.
func (b *backfillWireEnv) seedDecidableDeal(t *testing.T) ids.UUID {
	t.Helper()
	owner := integration.OwnerConn(t)
	pipeline := integration.SeedIDRow(t, owner,
		`INSERT INTO pipeline (id, name, is_default, position) VALUES ($1, 'Digest', false, 0)`)
	stage := integration.SeedIDRow(t, owner,
		`INSERT INTO stage (id, pipeline_id, name, position, semantic, win_probability)
		 VALUES ($1, '`+pipeline.String()+`', 'Open', 0, 'open', 30)`)
	return b.env.SeedDeal(t, "Digest deal",
		ids.From[ids.PipelineKind](pipeline), ids.From[ids.StageKind](stage), &b.env.Rep1)
}

// stageCloseDateProposal stages one proposal through the engine that stages
// every other one, so the row the count reads is the row production writes.
func (b *backfillWireEnv) stageCloseDateProposal(t *testing.T, deal ids.UUID) {
	t.Helper()
	svc := approvals.NewService(b.env.DB())
	if _, err := svc.Stage(b.env.Admin(), approvals.StageInput{
		Kind:           deals.CloseDateCorrectionKind,
		ProposedChange: []byte(`{"deal_id":"` + deal.String() + `","expected_close_date":"2026-12-01"}`),
		DiffHash:       deal.String(),
		TargetType:     "deal",
		TargetID:       deal,
		Summary:        "Confirm the real close date",
	}); err != nil {
		t.Fatalf("staging a close-date proposal: %v", err)
	}
}

// buildWith runs the nightly pass under one authority and returns the counts
// the reader's stored payload carries.
//
// The registry is rebuilt per authority rather than mutated, matching the
// projects suite beside it: the authority is a constructor argument, and a
// digest is only honest about a reader if it was built as that reader.
func (b *backfillWireEnv) buildWith(
	t *testing.T, authority authz.Resolver, now time.Time,
) (approvalsPending int) {
	t.Helper()
	pending, _ := b.buildCounts(t, authority, now)
	return pending
}

// buildCounts is buildWith's two-count form: the digest carries an approvals
// number and a duplicates number, both written under the reader's own
// authority, and each needs its own end-to-end case.
func (b *backfillWireEnv) buildCounts(
	t *testing.T, authority authz.Resolver, now time.Time,
) (approvalsPending, dedupeOpen int) {
	t.Helper()
	e := b.env
	reg := capture.NewRegistry(e.DB(), capture.NewSink(e.DB()), authority, keyvault.NewMemory()).
		WithDigestReview(newDigestReviewSource(e.Pool, approvals.NewService(e.DB())))
	b.handlers.registry = reg
	if err := reg.BuildDigests(b.human, now); err != nil {
		t.Fatalf("BuildDigests: %v", err)
	}
	_, digest := b.readDigest(t, nil)
	if digest.Review.ApprovalsPending == nil || digest.Review.DedupeOpen == nil {
		t.Fatalf("a wired build reported no count at all (approvals=%v duplicates=%v), which is "+
			"the answer reserved for a build that could not count",
			digest.Review.ApprovalsPending, digest.Review.DedupeOpen)
	}
	return *digest.Review.ApprovalsPending, *digest.Review.DedupeOpen
}

// seedDuplicatePair plants one open dedupe candidate through the real writer,
// and answers the id of the side a caller may be denied.
//
// Through CreateContact rather than an INSERT, because the candidate row is a
// side effect of the near-match scan a create runs: a hand-written row would
// prove the count reads a table, not that it reads what production writes.
func (b *backfillWireEnv) seedDuplicatePair(t *testing.T, name, email, dupName, dupEmail string) ids.UUID {
	t.Helper()
	incumbent, err := b.env.Contacts.CreateContact(b.env.Admin(), contacts.CreateContactInput{
		FullName: name, Source: "manual",
		Emails: []contacts.ContactEmailInput{{Email: email, EmailType: "work", IsPrimary: true}},
	})
	if err != nil {
		t.Fatalf("seeding the incumbent: %v", err)
	}
	if _, err := b.env.Contacts.CreateContact(b.env.Admin(), contacts.CreateContactInput{
		FullName: dupName, Source: "manual",
		Emails: []contacts.ContactEmailInput{{Email: dupEmail, EmailType: "work", IsPrimary: true}},
	}); err != nil {
		t.Fatalf("seeding the near-duplicate: %v", err)
	}
	return ids.UUID(incumbent.Id)
}

// The duplicates count carries the reader's own scope, end to end.
//
// #1690 is named for this number: the detector reads across row scope on
// purpose — a duplicate you cannot see is still a duplicate — so creating a
// record that near-matches a colleague's PRIVATE one used to bump a counter
// every digest reader saw. That is a weak existence oracle over records the
// reader cannot open.
//
// The store-level rule is held next to the clause. What this adds is the wiring
// above it: the digest builds one payload per reader, and a count that is
// correct in the store and mis-plumbed in the builder discloses exactly what
// the store refused. The approvals half has had this case since the seam
// landed; this is its twin.
func TestTheDigestCountsOnlyTheDuplicatesItsReaderCouldOpen(t *testing.T) {
	b := setupBackfillWire(t)
	now := time.Now().UTC()

	// One pair both readers may open, so the assertion below is about the
	// SECOND pair rather than about a count that hides from everybody.
	b.seedDuplicatePair(t, "Ada Vance", "ada@wire.test", "Ada Vanse", "ada.v@wire.test")

	// And one whose incumbent is capture-private to somebody else. Capture
	// privacy bounds even an all-scope human (dedupescope.go says so), so this
	// side is unreadable without giving the reader a narrower row scope than
	// the first pair needs.
	hidden := b.seedDuplicatePair(t, "Bea Lund", "bea@wire.test", "Bea Lundt", "bea.l@wire.test")
	b.env.MakeCapturePrivate(t, "contact", hidden, b.env.Rep3)

	_, open := b.buildCounts(t, readsContacts{}, now)
	if open != 1 {
		t.Errorf("the digest reported %d open duplicates, want 1 — the second pair's incumbent "+
			"is private to another seat, and counting it tells this reader that somebody "+
			"else's record exists", open)
	}
}

// A staged proposal this reader could not decide is not in their count.
//
// This is the disclosure the seam closes: before it, the digest counted every
// pending approval in the workspace and wrote that number into every payload,
// so a reader holding no deal grant was told how many deal decisions exist.
func TestTheDigestCountsOnlyTheProposalsItsReaderCouldDecide(t *testing.T) {
	b := setupBackfillWire(t)
	b.stageCloseDateProposal(t, b.seedDecidableDeal(t))
	now := time.Now().UTC()

	if pending := b.buildWith(t, decidesNothing{}, now); pending != 0 {
		t.Errorf("a reader who cannot decide a deal proposal was told %d are waiting, want 0", pending)
	}
	if pending := b.buildWith(t, decidesDeals{}, now); pending != 1 {
		t.Errorf("a reader who CAN decide it was told %d are waiting, want 1 — "+
			"a count that hides from everyone proves nothing about scope", pending)
	}
}

// An installation that never wired the seam reports NO count, rather than zero
// and rather than the workspace-wide number the seam exists to retire. Zero
// would say "nothing is waiting for you", which is a claim this build cannot
// make about a reader whose queues it never asked about.
func TestAnUnwiredDigestReportsNothingWaitingRatherThanEverything(t *testing.T) {
	b := setupBackfillWire(t)
	e := b.env
	b.stageCloseDateProposal(t, b.seedDecidableDeal(t))

	unwired := capture.NewRegistry(e.DB(), capture.NewSink(e.DB()), decidesDeals{}, keyvault.NewMemory())
	if err := unwired.BuildDigests(b.human, time.Now().UTC()); err != nil {
		t.Fatalf("BuildDigests without the review seam: %v", err)
	}
	_, digest := b.readDigest(t, nil)
	if digest.Review.ApprovalsPending != nil || digest.Review.DedupeOpen != nil {
		t.Fatalf("an unwired build reported counts (%v approvals, %v duplicates) — "+
			"absent is the honest answer, and the workspace-wide number this seam "+
			"replaces is not something to fall back to",
			digest.Review.ApprovalsPending, digest.Review.DedupeOpen)
	}
}

// AdmittedAuthority delegates to this fixture's own two reads; see
// authztest.AdmittedFromPair for why the body is not written out here.
func (r decidesNothing) AdmittedAuthority(ctx context.Context, ws, human, _ ids.UUID) (authz.RBAC, principal.SeatType, error) {
	return authztest.AdmittedFromPair(ctx, ws, human, r.EffectiveRBAC, r.SeatType)
}

// AdmittedAuthority delegates to this fixture's own two reads; see
// authztest.AdmittedFromPair for why the body is not written out here.
func (r decidesDeals) AdmittedAuthority(ctx context.Context, ws, human, _ ids.UUID) (authz.RBAC, principal.SeatType, error) {
	return authztest.AdmittedFromPair(ctx, ws, human, r.EffectiveRBAC, r.SeatType)
}

// readsContacts holds what the duplicates count asks of its reader.
//
// ALL THREE record grants, because requireDedupeRead asks for contact, company
// AND lead when no entity type narrows the queue — the queue spans the three
// vocabularies, so a reader missing one may not be told how many pairs it
// holds. Worth stating in the fixture, because a missing grant does not surface
// as an error: countOrNoneVisible reads the refusal as an empty queue, so the
// count silently reads 0 and a test asserting "hidden" would pass for the
// wrong reason. Mine did, until this line.
//
// Full row scope deliberately — capture privacy bounds an all-scope human
// anyway, so a narrower scope would hide the private side for the wrong reason
// and the test would pass without the clause under test.
type readsContacts struct{ backfillAuthority }

func (readsContacts) EffectiveRBAC(context.Context, ids.UUID, ids.UUID) (authz.RBAC, error) {
	return authz.RBAC{Permissions: principal.Permissions{
		Objects: map[string]principal.ObjectGrant{
			"activity": {Create: true, Read: true},
			"contact":  {Read: true, Update: true},
			"company":  {Read: true},
			"lead":     {Read: true},
		},
		RowScope: principal.RowScopeAll,
	}}, nil
}

// AdmittedAuthority delegates to this fixture's own two reads; see
// authztest.AdmittedFromPair for why the body is not written out here.
func (r readsContacts) AdmittedAuthority(ctx context.Context, ws, human, _ ids.UUID) (authz.RBAC, principal.SeatType, error) {
	return authztest.AdmittedFromPair(ctx, ws, human, r.EffectiveRBAC, r.SeatType)
}
