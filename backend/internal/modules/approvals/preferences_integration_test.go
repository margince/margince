// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package approvals

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestAPersonalMutePreservesAnotherReadersQueueAndTheSharedProposal(t *testing.T) {
	e := setupStaging(t)
	ctx := e.asRep(e.rep)
	other := e.secondRep(t)
	otherCtx := e.asRep(other)
	id := newDealSeeder(t, e).stageCounted(ctx, t)
	for _, reader := range []context.Context{ctx, otherCtx} {
		rows, _, err := e.svc.List(reader, ListInput{Kind: new(countedKind)})
		if err != nil || len(rows) != 1 {
			t.Fatalf("before mute: %d rows, %v", len(rows), err)
		}
	}
	if _, err := e.svc.SetAutoApply(otherCtx, countedKind, true); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.SetApprovalPreference(ctx, countedKind, false); err != nil {
		t.Fatal(err)
	}
	if mode, err := e.svc.AutoApplyMode(ctx, countedKind); err != nil || mode != ModeAuto {
		t.Fatalf("unchanged preference rewrote policy: %s, %v", mode, err)
	}
	if _, err := e.svc.SetApprovalPreference(ctx, countedKind, true); err != nil {
		t.Fatal(err)
	}
	rows, _, err := e.svc.List(ctx, ListInput{Kind: new(countedKind)})
	if err != nil || len(rows) != 0 {
		t.Fatalf("muted reader: %d rows, %v", len(rows), err)
	}
	rows, _, err = e.svc.List(otherCtx, ListInput{Kind: new(countedKind)})
	if err != nil || len(rows) != 1 {
		t.Fatalf("other reader: %d rows, %v", len(rows), err)
	}
	rows, _, err = e.svc.List(ctx, ListInput{Kind: new(countedKind), IncludeMuted: true})
	if err != nil || len(rows) != 1 || rows[0].ID != id {
		t.Fatalf("explicit review: %v, %v", rows, err)
	}
	proposal, err := e.svc.Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.svc.db.Tx(ctx, func(tx pgx.Tx) error {
		targeted, readErr := e.svc.PendingForTarget(ctx, tx, *proposal.TargetType, *proposal.TargetID)
		if readErr == nil && len(targeted.Approvals) != 1 {
			t.Fatalf("target review lost muted proposal: %v", targeted.Approvals)
		}
		return readErr
	}); err != nil {
		t.Fatal(err)
	}
	if status := e.statusOf(t, id); status != statusPending {
		t.Fatalf("shared status = %s", status)
	}
	if mode, err := e.svc.AutoApplyMode(ctx, countedKind); err != nil || mode != ModeManual {
		t.Fatalf("muted mode %s: %v", mode, err)
	}
	if mode, err := e.svc.AutoApplyMode(otherCtx, countedKind); err != nil || mode != ModeAuto {
		t.Fatalf("other mode %s: %v", mode, err)
	}
	if _, err := e.svc.SetAutoApply(ctx, countedKind, true); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("hidden automatic writes allowed: %v", err)
	}
	if _, err := e.svc.SetApprovalPreference(ctx, countedKind, false); err != nil {
		t.Fatal(err)
	}
	rows, _, err = e.svc.List(ctx, ListInput{Kind: new(countedKind)})
	if err != nil || len(rows) != 1 {
		t.Fatalf("restored reader: %d rows, %v", len(rows), err)
	}
	if mode, err := e.svc.AutoApplyMode(ctx, countedKind); err != nil || mode != ModeManual {
		t.Fatalf("unmute restored automatic mode %s: %v", mode, err)
	}
	var verdicts int
	if err := e.owner.QueryRow(context.Background(), `SELECT count(*) FROM audit_log WHERE entity_type = 'approval' AND entity_id = @id AND action IN ('approve', 'reject', 'expire')`, pgx.NamedArgs{"id": id}).Scan(&verdicts); err != nil {
		t.Fatal(err)
	}
	if verdicts != 0 {
		t.Fatalf("mute fabricated %d verdicts", verdicts)
	}
}

func TestMuteFilteringPrecedesPaginationAndNeverHidesPassportRows(t *testing.T) {
	e := setupStaging(t)
	ctx := e.asRep(e.rep)
	seed := newDealSeeder(t, e)
	visible := seed.stageCounted(ctx, t)
	passport := e.lentPassport(t, e.rep)
	actor, ok := principal.Actor(passport)
	if !ok {
		t.Fatal("passport actor missing")
	}
	actor.Permissions.Objects[tableDeal] = principal.ObjectGrant{Read: true, Update: true}
	passport = principal.WithActor(passport, actor)
	passportID := seed.stageCounted(passport, t)
	for range 4 {
		seed.stageCounted(ctx, t)
	}
	if _, err := e.svc.SetApprovalPreference(ctx, countedKind, true); err != nil {
		t.Fatal(err)
	}
	rows, _, err := e.svc.List(ctx, ListInput{Kind: new(countedKind), Limit: 1})
	if err != nil || len(rows) != 1 || rows[0].ID != passportID {
		t.Fatalf("passport row behind muted rows: %v, %v", rows, err)
	}
	rows, _, err = e.svc.List(passport, ListInput{Kind: new(countedKind), Limit: 1})
	if err != nil || len(rows) != 1 || rows[0].ID != passportID {
		t.Fatalf("passport reader preference parity: %v, %v", rows, err)
	}
	if _, err := e.svc.GetWire(ctx, visible); err != nil {
		t.Fatalf("muted direct review refused: %v", err)
	}
}

func TestUnsafeApprovalKindsCannotBeMuted(t *testing.T) {
	e := setupStaging(t)
	ctx := e.asRep(e.rep)
	// Direct sends, bookings and merges have no shared approval effect classifier; exercise their public kind contracts here.
	for _, kind := range []string{"send_email", "send_company_email", "send_message", "held_draft", KindScheduledSendHeld, "book_meeting", "invite_meeting", "merge_records", "merge_tags", "unregistered_future_kind"} {
		if _, err := e.svc.SetApprovalPreference(ctx, kind, true); !errors.Is(err, apperrors.ErrNotFound) {
			t.Errorf("%s mute = %v", kind, err)
		}
	}
	if _, err := e.svc.SetApprovalPreference(e.lentPassport(t, e.rep), countedKind, true); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("agent changed personal preference: %v", err)
	}
}

func TestAMixedBundleKeepsItsNonMuteableMembers(t *testing.T) {
	e := setupStaging(t)
	ctx := e.asHumanWith(decidesEverything())
	company, bundle := e.company(t), ids.NewV7()
	muted := e.stageInto(ctx, t, bundle, company, "company_name_promotion", "rename")
	visible := e.stageInto(ctx, t, bundle, company, kindDeepRead, "read")
	if _, err := e.svc.SetApprovalPreference(ctx, "company_name_promotion", true); err != nil {
		t.Fatal(err)
	}
	rows, _, err := e.svc.List(ctx, ListInput{BundleID: &bundle})
	if err != nil || len(rows) != 1 || rows[0].ID != visible {
		t.Fatalf("filtered bundle: %v, %v", rows, err)
	}
	rows, _, err = e.svc.List(ctx, ListInput{BundleID: &bundle, IncludeMuted: true})
	if err != nil || len(rows) != 2 {
		t.Fatalf("bundle review: %v, %v", rows, err)
	}
	if e.statusOf(t, muted) != statusPending {
		t.Fatal("muted bundle member was decided")
	}
}

func TestASoleDecidersMutedProposalRemainsReachableAndExpiresNormally(t *testing.T) {
	e := setupStaging(t)
	target := newDealSeeder(t, e).deal(t)
	reader := seatFor(e.ws, e.rep, tableDeal, objectActivity)
	worker := principal.WithActor(principal.WithCorrelationID(principal.WithWorkspaceID(context.Background(), e.ws), ids.NewV7()), principal.Principal{Type: principal.PrincipalSystem, ID: "system:follow-up", OnBehalfOf: e.rep})
	id, err := e.svc.Stage(worker, StageInput{Kind: kindDealFollowUp, ProposedChange: []byte(`{"subject":"Call the buyer"}`), DiffHash: "personal-follow-up", TargetType: tableDeal, TargetID: target})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.SetApprovalPreference(reader, kindDealFollowUp, true); err != nil {
		t.Fatal(err)
	}
	rows, _, err := e.svc.ListWire(reader, ListInput{IncludeMuted: true, Kind: new(kindDealFollowUp)})
	if err != nil || len(rows) != 1 || rows[0].Status != "pending" {
		t.Fatalf("sole-decider review: %v, %v", rows, err)
	}
	colleague := seatFor(e.ws, e.secondRep(t), tableDeal, objectActivity)
	rows, _, err = e.svc.ListWire(colleague, ListInput{IncludeMuted: true, Kind: new(kindDealFollowUp)})
	if err != nil || len(rows) != 0 {
		t.Fatalf("private proposal exposed to colleague: %v, %v", rows, err)
	}
	var expires time.Time
	if err := e.owner.QueryRow(t.Context(), `SELECT expires_at FROM approval WHERE id = @id`, pgx.NamedArgs{"id": id}).Scan(&expires); err != nil {
		t.Fatal(err)
	}
	e.svc.now = func() time.Time { return expires.Add(time.Second) }
	rows, _, err = e.svc.ListWire(reader, ListInput{IncludeMuted: true, Kind: new(kindDealFollowUp)})
	if err != nil || len(rows) != 1 || rows[0].Status != "expired" {
		t.Fatalf("ordinary expiry: %v, %v", rows, err)
	}
	if _, err := e.svc.ExpireDue(e.sweeping()); err != nil {
		t.Fatal(err)
	}
	rows, _, err = e.svc.ListWire(reader, ListInput{IncludeMuted: true, Kind: new(kindDealFollowUp)})
	if err != nil || len(rows) != 1 || rows[0].Status != "expired" {
		t.Fatalf("swept history: %v, %v", rows, err)
	}
	if e.statusOf(t, id) != StatusExpired {
		t.Fatal("muted proposal did not expire through the ordinary sweep")
	}
}
