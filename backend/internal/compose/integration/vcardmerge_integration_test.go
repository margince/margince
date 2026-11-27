// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A vCard near-match waits in the Worklist as a review. Its decider creates a
// separate contact or files the card on the contact it resembled. The merge
// asks the decider's own authority over that contact at decision time. An
// undecided review expires after 30 days and takes the card's data with it.

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const (
	mergeCandidateEmail = "anna.weber@example.com"
	mergeCardEmail      = "a.weber@webers.example"
	mergeCardTitle      = "Head of Sales"
)

// mergeDeciderPerms is a rep who may decide a vcard_create review (the create,
// update and edge grants it spends) and sees only the rows they own.
func mergeDeciderPerms() principal.Permissions {
	perms := RepPerms
	perms.Objects = maps.Clone(RepPerms.Objects)
	perms.Objects["company"] = principal.ObjectGrant{Create: true, Read: true, Update: true}
	perms.RowScope = principal.RowScopeOwn
	return perms
}

func mergeCard() contacts.VCardEntry {
	return contacts.VCardEntry{
		FullName: "Anna Weber", Company: "Weber Consulting", Title: mergeCardTitle,
		Emails: []contacts.VCardChannel{{Value: mergeCardEmail, Kind: "work"}},
	}
}

type mergeReview struct {
	e         *Env
	svc       *approvals.Service
	decider   context.Context
	candidate ids.ContactID
	approval  ids.ApprovalID
}

// stagedMergeReview seeds the contact the card resembles, owned by Rep1, and
// stages the card against it through the production stager as Rep1.
func stagedMergeReview(t *testing.T) mergeReview {
	t.Helper()
	e := Setup(t)
	decider := e.As(e.Rep1, nil, mergeDeciderPerms())
	candidate, err := e.Contacts.CreateContact(decider, contacts.CreateContactInput{
		FullName: "Anna Weber", Source: "manual", OwnerID: new(ids.From[ids.UserKind](e.Rep1)),
		Emails: []contacts.ContactEmailInput{{Email: mergeCandidateEmail, EmailType: "work", IsPrimary: true}},
	})
	if err != nil {
		t.Fatalf("seeding the contact the card resembles: %v", err)
	}
	candidateID := ids.From[ids.ContactKind](ids.UUID(candidate.Id))
	if err := compose.VCardCreateStager(e.Pool)(decider, mergeCard(), &candidateID); err != nil {
		t.Fatalf("staging the review: %v", err)
	}
	svc := compose.NewApprovalsServiceWithEffects(e.Pool)
	status, kind := "pending", approvals.KindVCardCreate
	rows, _, err := svc.ListWire(decider, approvals.ListInput{Status: &status, Kind: &kind, Limit: 10})
	if err != nil || len(rows) != 1 {
		t.Fatalf("the importer's pending reviews = %d (err %v), want 1", len(rows), err)
	}
	return mergeReview{e: e, svc: svc, decider: decider, candidate: candidateID,
		approval: ids.From[ids.ApprovalKind](ids.UUID(rows[0].Id))}
}

// mergeChoice is the staged payload with the decider's box ticked, as the card
// sends it.
func (r mergeReview) mergeChoice(t *testing.T) json.RawMessage {
	t.Helper()
	var staged []byte
	if err := r.e.Pool.QueryRow(context.Background(),
		`SELECT proposed_change::text FROM approval WHERE id = $1`, r.approval).Scan(&staged); err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(staged, &payload); err != nil {
		t.Fatal(err)
	}
	payload["merge_into_candidate"] = true
	edited, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return edited
}

func (r mergeReview) count(t *testing.T, query string, args ...any) int {
	t.Helper()
	var n int
	if err := r.e.Pool.QueryRow(context.Background(), query, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func (r mergeReview) titleOfCandidate(t *testing.T) string {
	t.Helper()
	var title *string
	if err := r.e.Pool.QueryRow(context.Background(), `SELECT title FROM contact WHERE id = $1`, r.candidate).Scan(&title); err != nil {
		t.Fatal(err)
	}
	if title == nil {
		return ""
	}
	return *title
}

func TestKeepingAVCardReviewSeparateCreatesTheContact(t *testing.T) {
	r := stagedMergeReview(t)
	if _, err := r.svc.Decide(r.decider, r.approval, true, nil); err != nil {
		t.Fatalf("keeping the card separate: %v", err)
	}
	if n := r.count(t, `SELECT count(*) FROM contact_email WHERE lower(email) = $1`, mergeCardEmail); n != 1 {
		t.Errorf("contacts holding the card's address = %d, want the one created", n)
	}
	if got := r.titleOfCandidate(t); got != "" {
		t.Errorf("the resembled contact's title = %q, want it untouched by a separate create", got)
	}
}

func TestMergingAVCardReviewFilesTheCardOnTheResembledContact(t *testing.T) {
	r := stagedMergeReview(t)
	before := r.count(t, `SELECT count(*) FROM contact`)
	if _, err := r.svc.DecideEdited(r.decider, r.approval, r.mergeChoice(t)); err != nil {
		t.Fatalf("merging the card: %v", err)
	}
	if got := r.titleOfCandidate(t); got != mergeCardTitle {
		t.Errorf("the resembled contact's title = %q, want the card's %q", got, mergeCardTitle)
	}
	if after := r.count(t, `SELECT count(*) FROM contact`); after != before {
		t.Errorf("contacts after the merge = %d, want %d: a merge creates nobody", after, before)
	}
	if n := r.count(t, `SELECT count(*) FROM audit_log WHERE entity_id = $1 AND action = 'update'
		AND actor_id = $2`, r.candidate, "human:"+r.e.Rep1.String()); n != 1 {
		t.Errorf("update audit rows on the resembled contact by the decider = %d, want 1", n)
	}
	if n := r.count(t, `SELECT count(*) FROM approval WHERE id = $1 AND status = 'approved'`, r.approval); n != 1 {
		t.Errorf("the review is not approved after its merge")
	}
}

func TestMergingIsRefusedForADeciderWhoNoLongerSeesTheResembledContact(t *testing.T) {
	r := stagedMergeReview(t)
	r.e.WsExec(t, `UPDATE contact SET owner_id = $2 WHERE id = $1`, r.candidate, r.e.Rep2)
	before := r.count(t, `SELECT count(*) FROM contact`)

	_, err := r.svc.DecideEdited(r.decider, r.approval, r.mergeChoice(t))
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("merging into a contact the decider cannot see = %v, want not found", err)
	}
	r.assertNothingHappened(t, before)
	if _, err := r.svc.Decide(r.decider, r.approval, true, nil); err != nil {
		t.Errorf("keeping the same card separate after the refused merge: %v", err)
	}
}

func TestMergingIsRefusedForAResembledContactThatWasArchived(t *testing.T) {
	r := stagedMergeReview(t)
	r.e.WsExec(t, `UPDATE contact SET archived_at = now() WHERE id = $1`, r.candidate)
	before := r.count(t, `SELECT count(*) FROM contact`)

	if _, err := r.svc.DecideEdited(r.decider, r.approval, r.mergeChoice(t)); err == nil {
		t.Fatal("a merge into an archived contact was approved")
	}
	r.assertNothingHappened(t, before)
}

// assertNothingHappened holds what a refused merge leaves: the review still
// waiting, no contact made, and the resembled contact as it was.
func (r mergeReview) assertNothingHappened(t *testing.T, contactsBefore int) {
	t.Helper()
	if n := r.count(t, `SELECT count(*) FROM approval WHERE id = $1 AND status = 'pending'`, r.approval); n != 1 {
		t.Errorf("the review is no longer pending after a refused merge")
	}
	if after := r.count(t, `SELECT count(*) FROM contact`); after != contactsBefore {
		t.Errorf("contacts after a refused merge = %d, want %d", after, contactsBefore)
	}
	if got := r.titleOfCandidate(t); got != "" {
		t.Errorf("the resembled contact's title = %q after a refused merge, want none", got)
	}
}

func TestAMergeChoiceNamingNoContactIsRefusedWhileStillPending(t *testing.T) {
	e := Setup(t)
	decider := e.As(e.Rep1, nil, mergeDeciderPerms())
	if err := compose.VCardCreateStager(e.Pool)(decider, mergeCard(), nil); err != nil {
		t.Fatalf("staging a card that resembled nobody visible: %v", err)
	}
	svc := compose.NewApprovalsServiceWithEffects(e.Pool)
	status, kind := "pending", approvals.KindVCardCreate
	rows, _, err := svc.ListWire(decider, approvals.ListInput{Status: &status, Kind: &kind, Limit: 10})
	if err != nil || len(rows) != 1 {
		t.Fatalf("pending reviews = %d (err %v), want 1", len(rows), err)
	}
	id := ids.From[ids.ApprovalKind](ids.UUID(rows[0].Id))
	change := *rows[0].ProposedChange
	change["merge_into_candidate"] = true
	edited, err := json.Marshal(change)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.DecideEdited(decider, id, edited); err == nil {
		t.Fatal("a merge with no contact to merge into was approved")
	}
}

func TestAnUndecidedVCardReviewLastsThirtyDays(t *testing.T) {
	r := stagedMergeReview(t)
	var window time.Duration
	if err := r.e.Pool.QueryRow(context.Background(),
		`SELECT expires_at - created_at FROM approval WHERE id = $1`, r.approval).Scan(&window); err != nil {
		t.Fatal(err)
	}
	if want := 30 * 24 * time.Hour; window < want-time.Minute || window > want+time.Minute {
		t.Errorf("an undecided review waits %v, want %v", window, want)
	}
}

// cardTraces counts the places of one approval row where the card could still
// be read: its name or address anywhere in the row, as text.
func (r mergeReview) cardTraces(t *testing.T) int {
	t.Helper()
	return r.count(t, `SELECT count(*) FROM approval a WHERE a.id = $1
		AND (a.proposed_change::text ILIKE '%Weber%' OR a.proposed_change::text ILIKE '%webers.example%'
		  OR coalesce(a.summary, '') ILIKE '%Weber%' OR coalesce(a.summary_parts::text, '') ILIKE '%Weber%'
		  OR coalesce(a.evidence::text, '') ILIKE '%Weber%')`, r.approval)
}

func TestAnExpiredVCardReviewNoLongerHoldsTheCard(t *testing.T) {
	r := stagedMergeReview(t)
	if r.cardTraces(t) != 1 {
		t.Fatal("the staged review does not hold the card, so this test would prove nothing")
	}
	r.e.WsExec(t, `UPDATE approval SET expires_at = now() - interval '1 minute' WHERE id = $1`, r.approval)
	sweep := principal.SystemActing(principal.WithWorkspaceID(context.Background(), r.e.WS), approvals.ExpiryActor)
	if _, err := compose.NewApprovalsService(r.e.DB()).ExpireDue(sweep); err != nil {
		t.Fatalf("sweeping: %v", err)
	}
	if n := r.count(t, `SELECT count(*) FROM approval WHERE id = $1 AND status = 'expired'`, r.approval); n != 1 {
		t.Fatalf("the sweep did not expire the review")
	}
	if n := r.cardTraces(t); n != 0 {
		t.Errorf("the expired review still holds the card")
	}
	if n := r.count(t, `SELECT count(*) FROM audit_log WHERE entity_id = $1 AND evidence::text ILIKE '%Weber%'`, r.approval); n != 0 {
		t.Errorf("the review's audit rows hold the card's name")
	}
}

func TestAReviewPastItsWindowIsNotShownBeforeTheSweepReachesIt(t *testing.T) {
	r := stagedMergeReview(t)
	r.e.WsExec(t, `UPDATE approval SET expires_at = now() - interval '1 minute' WHERE id = $1`, r.approval)
	row, err := r.svc.GetWire(r.decider, r.approval)
	if err != nil {
		t.Fatalf("reading the lapsed review: %v", err)
	}
	if row.Status != "expired" {
		t.Fatalf("the review past its window reads %q, want expired", row.Status)
	}
	shown, err := json.Marshal(row)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(shown), "Weber") {
		t.Errorf("an expired review is shown with the card: %s", shown)
	}
}

func TestASupersededVCardReviewNoLongerHoldsTheCard(t *testing.T) {
	r := stagedMergeReview(t)
	changed := mergeCard()
	changed.Title = "Managing Director"
	candidate := r.candidate
	if err := compose.VCardCreateStager(r.e.Pool)(r.decider, changed, &candidate); err != nil {
		t.Fatalf("staging the same card again with a changed title: %v", err)
	}
	var proposals int
	if err := r.e.Pool.QueryRow(context.Background(),
		`SELECT count(*) FROM approval WHERE kind = 'vcard_create' AND status = 'pending'`).Scan(&proposals); err != nil {
		t.Fatal(err)
	}
	if proposals != 1 {
		t.Fatalf("pending reviews of the card = %d, want the newer one only", proposals)
	}
	if n := r.cardTraces(t); n != 0 {
		t.Errorf("the superseded review still holds the card")
	}
}

func TestAnExpiredVCardReviewIsStagedAnewNotRemembered(t *testing.T) {
	r := stagedMergeReview(t)
	r.e.WsExec(t, `UPDATE approval SET expires_at = now() - interval '1 minute' WHERE id = $1`, r.approval)
	sweep := principal.SystemActing(principal.WithWorkspaceID(context.Background(), r.e.WS), approvals.ExpiryActor)
	if _, err := compose.NewApprovalsService(r.e.DB()).ExpireDue(sweep); err != nil {
		t.Fatalf("sweeping: %v", err)
	}
	candidate := r.candidate
	if err := compose.VCardCreateStager(r.e.Pool)(r.decider, mergeCard(), &candidate); err != nil {
		t.Fatalf("staging the card after its first review expired: %v", err)
	}
	status, kind := "pending", approvals.KindVCardCreate
	rows, _, err := r.svc.ListWire(r.decider, approvals.ListInput{Status: &status, Kind: &kind, Limit: 10})
	if err != nil || len(rows) != 1 {
		t.Errorf("pending reviews after re-importing an expired card = %d (err %v), want 1: expiry is not a decline", len(rows), err)
	}
}
