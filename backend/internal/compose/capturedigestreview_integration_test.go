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
	e := b.env
	reg := capture.NewRegistry(e.DB(), capture.NewSink(e.DB()), authority, keyvault.NewMemory()).
		WithDigestReview(newDigestReviewSource(e.Pool, approvals.NewService(e.DB())))
	if err := reg.BuildDigests(b.human, now); err != nil {
		t.Fatalf("BuildDigests: %v", err)
	}
	_, digest := b.readDigest(t, nil)
	if digest.Review.ApprovalsPending == nil {
		t.Fatal("a wired build reported no count at all, which is the answer reserved " +
			"for a build that could not count")
	}
	return *digest.Review.ApprovalsPending
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
// admittedFromPair for why the body is not written out here.
func (r decidesNothing) AdmittedAuthority(ctx context.Context, ws, human, _ ids.UUID) (authz.RBAC, principal.SeatType, error) {
	return authztest.AdmittedFromPair(ctx, ws, human, r.EffectiveRBAC, r.SeatType)
}

// AdmittedAuthority delegates to this fixture's own two reads; see
// admittedFromPair for why the body is not written out here.
func (r decidesDeals) AdmittedAuthority(ctx context.Context, ws, human, _ ids.UUID) (authz.RBAC, principal.SeatType, error) {
	return authztest.AdmittedFromPair(ctx, ws, human, r.EffectiveRBAC, r.SeatType)
}
