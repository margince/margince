// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestAcceptanceRecordsOneDecisionForTheDisplayedCorrection(t *testing.T) {
	e := setupCloseDate(t)
	deal := e.seedSweepDeal(t, "Review this correction", e.early, nil, intp(-12), 3)
	if err := e.sweep(); err != nil {
		t.Fatal(err)
	}
	change := e.correctionAuditIDFor(t, deal)
	dealID := ids.From[ids.DealKind](deal)
	review, err := e.Deals.AppliedChangeReview(e.Admin(), dealID, change)
	if err != nil {
		t.Fatal(err)
	}
	key := deals.AppliedChangeKey{DealID: dealID, ChangeID: change}
	wrong := deals.AppliedChangeKey{DealID: ids.From[ids.DealKind](e.Rep2), ChangeID: change}
	batch, err := e.Deals.AppliedChangeReviews(e.Admin(), []deals.AppliedChangeKey{key, wrong})
	if err != nil || len(batch) != 1 || batch[key] != review {
		t.Fatalf("batch review must bind each change to its deal: %+v, %v", batch, err)
	}
	if review.Accepted || !review.CanAccept || !review.CanUndo {
		t.Fatalf("new correction: %+v", review)
	}
	if err := e.Deals.AcceptAppliedChange(e.Admin(), dealID, change, review.Version-1); !errors.Is(err, apperrors.ErrVersionSkew) {
		t.Fatalf("stale acceptance: %v", err)
	}
	for range 2 {
		if err := e.Deals.AcceptAppliedChange(e.Admin(), dealID, change, review.Version); err != nil {
			t.Fatal(err)
		}
	}
	accepted, err := e.Deals.AppliedChangeReview(e.Admin(), dealID, change)
	if err != nil || !accepted.Accepted {
		t.Fatalf("acceptance was not persisted: %+v, %v", accepted, err)
	}
	var audits, events int
	if err := e.owner.QueryRow(context.Background(), `SELECT count(*), count(o.id) FROM audit_log a LEFT JOIN event_outbox o ON o.envelope->'trace'->>'audit_log_id' = a.id::text WHERE a.entity_id = $1 AND a.after ? 'applied_change'`, deal).Scan(&audits, &events); err != nil {
		t.Fatal(err)
	}
	if audits != 1 || events != 1 {
		t.Fatalf("acceptance must record one audit and linked event: %d, %d", audits, events)
	}
	if _, err := e.Deals.RevertCorrection(e.Admin(), e.correctionFor(t, deal), nil, nil); err != nil {
		t.Fatal(err)
	}
	reversed, err := e.Deals.AppliedChangeReview(e.Admin(), dealID, change)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.Deals.AcceptAppliedChange(e.Admin(), dealID, change, reversed.Version); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("accepted a reversed change: %v", err)
	}
}

func TestAnEditedCorrectionCannotBeAcceptedAsTheCurrentChange(t *testing.T) {
	e := setupCloseDate(t)
	deal := e.seedSweepDeal(t, "Edit after correction", e.early, nil, intp(-12), 3)
	if err := e.sweep(); err != nil {
		t.Fatal(err)
	}
	change := e.correctionAuditIDFor(t, deal)
	dealID := ids.From[ids.DealKind](deal)
	date := today().AddDate(0, 0, 90)
	if _, err := e.Deals.UpdateDeal(e.Admin(), dealID, deals.UpdateDealInput{ExpectedClose: &date}); err != nil {
		t.Fatal(err)
	}
	review, err := e.Deals.AppliedChangeReview(e.Admin(), dealID, change)
	if err != nil {
		t.Fatal(err)
	}
	if review.CanAccept {
		t.Fatalf("a superseded correction offers acceptance: %+v", review)
	}
	if err := e.Deals.AcceptAppliedChange(e.Admin(), dealID, change, review.Version); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("accepted a superseded change: %v", err)
	}
}

func TestAcceptanceChecksWriteAuthorityBeforeReturningAConflict(t *testing.T) {
	e := setupCloseDate(t)
	deal := e.seedSweepDeal(t, "Another owner's correction", e.early, nil, intp(-12), 3)
	if err := e.sweep(); err != nil {
		t.Fatal(err)
	}
	change := e.correctionAuditIDFor(t, deal)
	permissions := principal.Permissions{RoleKeys: []string{"rep"}, Objects: map[string]principal.ObjectGrant{"deal": {Read: true, Update: true}}, RowScope: principal.RowScopeOwn}
	reader := e.As(e.Rep2, []ids.UUID{e.Team2}, permissions)
	dealID := ids.From[ids.DealKind](deal)
	review, err := e.Deals.AppliedChangeReview(reader, dealID, change)
	if err != nil || review.Writable {
		t.Fatalf("a colleague may read this deal but may not change it: %+v, %v", review, err)
	}
	if err := e.Deals.AcceptAppliedChange(reader, dealID, change, -1); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("acceptance reported state before write authority: %v", err)
	}
}
