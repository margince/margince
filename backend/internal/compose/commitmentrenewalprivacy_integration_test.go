// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/modules/search"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

type renewalPrivacyFixture struct {
	e          *integration.Env
	contact    ids.ContactID
	messages   []ids.UUID
	work       ids.UUID
	commitment Commitment
}

func TestErasingEitherCitationWithdrawsTheRenewedProposal(t *testing.T) {
	for _, message := range []int{0, 1} {
		t.Run([]string{"original", "renewal"}[message], func(t *testing.T) {
			f := setupRenewalPrivacy(t, 0.75)
			subject := f.e.SeedContact(t, "Citation subject", &f.e.Rep1)
			integration.LinkActivity(t, integration.OwnerConn(t), f.messages[message], "contact", subject)
			if err := privacy.NewEraser(f.e.DB()).WithRawCapturePurger(RawCapturePurgerFor(f.e.DB())).EraseContact(f.e.Admin(), subject, "Subject requested erasure"); err != nil {
				t.Fatal(err)
			}
			if got := f.e.WsCount(t, `SELECT count(*) FROM activity WHERE id=$1 AND body IS NULL`, f.messages[message]); got != 1 {
				t.Fatal("the selected source was not erased")
			}
			if got := f.e.WsCount(t, `SELECT count(*) FROM approval WHERE id=$1 AND status='expired' AND proposed_change='{}'::jsonb`, f.work); got != 1 {
				t.Fatal("the proposal retained content from an erased citation")
			}
		})
	}
}

func TestErasingRenewalRemovesTheWorkDerivedFromItsWords(t *testing.T) {
	f := setupRenewalPrivacy(t, 0.95)
	subject := f.e.SeedContact(t, "Renewal subject", &f.e.Rep1)
	integration.LinkActivity(t, integration.OwnerConn(t), f.messages[1], "contact", subject)
	if err := privacy.NewEraser(f.e.DB()).WithRawCapturePurger(RawCapturePurgerFor(f.e.DB())).EraseContact(f.e.Admin(), subject, "Subject requested erasure"); err != nil {
		t.Fatal(err)
	}
	if got := f.e.WsCount(t, `SELECT count(*) FROM activity WHERE id=$1 AND body IS NULL`, f.messages[1]); got != 1 {
		t.Fatal("the renewal message was not erased")
	}
	if got := f.e.WsCount(t, `SELECT count(*) FROM activity WHERE id=$1 AND archived_at IS NOT NULL AND coalesce(subject,'') NOT LIKE '%confidential%'`, f.work); got != 1 {
		t.Error("the task still retains current work derived from the erased renewal")
	}
	if got := f.e.WsCount(t, `SELECT count(*) FROM conversation_claim WHERE source_activity_id=$1 AND body LIKE '%confidential%'`, f.messages[0]); got != 0 {
		t.Error("the claim retains words read from the erased renewal")
	}
	if got := f.e.WsCount(t, `SELECT count(*) FROM activity_commitment_admission WHERE source_activity_id=$1 AND summary LIKE '%confidential%'`, f.messages[0]); got != 0 {
		t.Error("the admission retains words read from the erased renewal")
	}
}

func setupRenewalPrivacy(t *testing.T, confidence float64) renewalPrivacyFixture {
	t.Helper()
	e := integration.Setup(t)
	contact := ids.From[ids.ContactKind](e.SeedContact(t, "Pat Buyer", &e.Rep1))
	now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	var messages []ids.UUID
	for i, body := range []string{"I will send pricing.", "I will send the confidential enterprise pricing tomorrow."} {
		at := now.Add(time.Duration(i-2) * 24 * time.Hour)
		row, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
			Kind: "email", Body: &body, OccurredAt: &at, Source: "manual", ThreadKey: "renewal-privacy", Direction: new("outbound"),
		})
		if err != nil {
			t.Fatal(err)
		}
		messages = append(messages, ids.UUID(row.Id))
	}
	c := Commitment{
		AsOf: now, ReadMessages: messages, Extractor: signalScanActor, SourceActivityID: messages[0],
		AdmissionSourceID: &messages[1], RenewedAt: new(now.Add(-24 * time.Hour)),
		Summary: "Send confidential enterprise pricing", Quote: "I will send pricing.", Seat: &e.Rep1, PromisedTo: &contact,
		Confidence: confidence, Locator: "private-renewal", Body: "A promise from the pricing conversation.", DueDate: "2026-10-19",
		Evidence:        approvals.Evidence{SourceType: "activity", SourceID: messages[0], Snippet: "I will send pricing."},
		RenewalEvidence: &approvals.Evidence{SourceType: "activity", SourceID: messages[1], Snippet: "I will send the confidential enterprise pricing tomorrow."},
	}
	ctx := withTranscriptReader(principal.WithWorkspaceID(context.Background(), e.WS), "human:"+e.Rep1.String(), ids.NewV7())
	dispatcher := NewCommitmentDispatcher(e.Pool, approvals.NewService(e.DB()))
	var task ids.UUID
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		var err error
		_, task, err = dispatcher.DispatchTx(ctx, tx, c, ids.NewV7())
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return renewalPrivacyFixture{e: e, contact: contact, messages: messages, work: task, commitment: c}
}

func TestNarrowingOnlyTheRenewalWithholdsDerivedContentButKeepsTheOriginalReadable(t *testing.T) {
	f := setupRenewalPrivacy(t, 0.95)
	e, contact, messages, task := f.e, f.contact, f.messages, f.work
	reader := e.As(e.Rep2, []ids.UUID{e.Team1}, integration.AdminPerms)
	check := func(visible bool) {
		t.Helper()
		row, err := e.Activities.GetActivity(reader, ids.From[ids.ActivityKind](task), storekit.LiveOnly)
		if err != nil {
			t.Fatal(err)
		}
		if (row.Subject != nil) != visible {
			t.Fatalf("task content visible = %v, want %v", row.Subject != nil, visible)
		}
		source, err := e.Activities.GetActivity(reader, ids.From[ids.ActivityKind](messages[0]), storekit.LiveOnly)
		if err != nil || source.ContentState == nil || *source.ContentState != crmcontracts.ActivityContentStateAvailable {
			t.Fatalf("the original message became unreadable: %v", err)
		}
		if (source.CommitmentAdmissions != nil) != visible {
			t.Fatalf("renewal review visible = %v, want %v", source.CommitmentAdmissions != nil, visible)
		}
		if err := database.WithWorkspaceTx(reader, e.Pool, func(tx pgx.Tx) error {
			claims, err := e.Contacts.ClaimsForContact(reader, tx, contact, nil, 10)
			if err == nil && (len(claims) > 0) != visible {
				t.Errorf("claim visible = %v, want %v", len(claims) > 0, visible)
			}
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
	check(true)
	if _, err := e.Activities.SetAudience(e.Admin(), ids.From[ids.ActivityKind](messages[1]), activities.SetAudienceInput{
		Audience: "selected", Members: []activities.AudienceMember{{SubjectType: "user", SubjectID: e.Rep1}},
	}); err != nil {
		t.Fatal(err)
	}
	check(false)
}

func TestRenewedProposalRequiresBothMessagesAtReadAndDecisionTime(t *testing.T) {
	f := setupRenewalPrivacy(t, 0.75)
	e := f.e
	reader := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms)
	svc := approvals.NewService(e.DB())
	id := ids.From[ids.ApprovalKind](f.work)
	before, err := svc.GetWire(reader, id)
	if err != nil || before.Evidence == nil || len(*before.Evidence) != 2 {
		t.Fatalf("renewal proposal must cite both messages: %v", err)
	}
	if _, err := e.Activities.SetAudience(e.Admin(), ids.From[ids.ActivityKind](f.messages[1]), activities.SetAudienceInput{
		Audience: "selected", Members: []activities.AudienceMember{{SubjectType: "user", SubjectID: e.Rep2}},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.GetWire(reader, id); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("proposal exposed unreadable renewal: %v", err)
	}
	for _, scoped := range []bool{false, true} {
		in := approvals.ListInput{Limit: 10, Status: new("pending")}
		if scoped {
			in.TargetType, in.TargetID = new("activity"), &f.messages[0]
		}
		rows, _, err := svc.ListWire(reader, in)
		if err != nil || len(rows) != 0 {
			t.Fatalf("proposal list (target scoped=%v) exposed evidence: rows=%d err=%v", scoped, len(rows), err)
		}
	}
	if _, err := svc.Decide(reader, id, true, nil); !errors.Is(err, apperrors.ErrNotFound) && !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("unreadable proposal was not refused by authority: %v", err)
	}
	if got := e.WsScalar(t, `SELECT status FROM approval WHERE id=$1`, f.work); got != "pending" {
		t.Fatalf("unreadable proposal changed state to %s", got)
	}
	claim := ids.MustParse(e.WsScalar(t, `SELECT id::text FROM conversation_claim WHERE source_activity_id=$1`, f.messages[0]))
	if err := e.Contacts.SettleConversationClaim(reader, claim, "done"); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("settled a claim backed by unreadable renewal: %v", err)
	}
}

func TestHeldDerivedWorkSurvivesErasureWithoutReleasingItsContent(t *testing.T) {
	f := setupRenewalPrivacy(t, 0.95)
	subject := f.e.SeedContact(t, "Renewal subject", &f.e.Rep1)
	integration.LinkActivity(t, integration.OwnerConn(t), f.messages[1], "contact", subject)
	integration.LinkActivity(t, integration.OwnerConn(t), f.work, "contact", f.contact.UUID)
	f.e.WsExec(t, `UPDATE contact SET legal_hold = true WHERE id = $1`, f.contact)
	if !embedHeldCommitment(t, f) {
		t.Fatal("fixture task was not indexed before erasure")
	}
	before := f.e.WsScalar(t, `SELECT to_jsonb(a)::text FROM activity a WHERE id=$1`, f.work)
	if err := privacy.NewEraser(f.e.DB()).WithRawCapturePurger(RawCapturePurgerFor(f.e.DB())).EraseContact(f.e.Admin(), subject, "Subject requested erasure"); err != nil {
		t.Fatal(err)
	}
	if got := f.e.WsCount(t, `SELECT count(*) FROM activity WHERE id=$1 AND body IS NULL`, f.messages[1]); got != 1 {
		t.Fatal("held work prevented source erasure")
	}
	if after := f.e.WsScalar(t, `SELECT to_jsonb(a)::text FROM activity a WHERE id=$1`, f.work); after != before {
		t.Fatal("erasure altered legally held work")
	}
	task, err := f.e.Activities.GetActivity(f.e.Admin(), ids.From[ids.ActivityKind](f.work), storekit.LiveOnly)
	if err != nil || task.Subject != nil {
		t.Fatalf("erased source released held task content: %v", err)
	}
	if got := f.e.WsCount(t, `SELECT count(*) FROM activity_commitment_admission WHERE renewed_by=$1 AND summary='' AND source_quote='' AND NOT current_work`, f.messages[1]); got != 1 {
		t.Fatal("held work lost its redacted access boundary")
	}
	if got := f.e.WsCount(t, `SELECT count(*) FROM embedding WHERE entity_type='activity' AND entity_id=$1`, f.work); got != 0 {
		t.Fatal("held work retained its erased-source vector")
	}
	if embedHeldCommitment(t, f) {
		t.Fatal("a delayed embed reindexed held erased-source work")
	}
	systemTask, err := f.e.Activities.GetActivity(principal.SystemActing(f.e.Admin(), "content-read"), ids.From[ids.ActivityKind](f.work), storekit.LiveOnly)
	if err != nil || systemTask.Subject != nil {
		t.Fatalf("system reader exposed held task content: %v", err)
	}
	if err := database.WithWorkspaceTx(f.e.Admin(), f.e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(f.e.Admin(), `DELETE FROM activity WHERE id=$1`, f.messages[0])
		return err
	}); err == nil {
		t.Fatal("hard deletion removed held evidence provenance")
	} else {
		pgerr, ok := errors.AsType[*pgconn.PgError](err)
		if !ok || pgerr.Code != "23503" {
			t.Fatalf("hard deletion failed for another reason: %v", err)
		}
	}
	assertHeldRenewalRefused(t, f, before)
	assertReleasedHoldFinishesErasure(t, f, before)
}

func TestSubjectExportIncludesRenewalProvenanceWithoutPrivateWords(t *testing.T) {
	f := setupRenewalPrivacy(t, 0.95)
	subject := ids.From[ids.ContactKind](f.e.SeedContact(t, "Renewal subject", &f.e.Rep1))
	integration.LinkActivity(t, integration.OwnerConn(t), f.messages[1], "contact", subject.UUID)
	check := func(private bool) {
		t.Helper()
		exported, err := privacy.AssembleSAR(f.e.Admin(), f.e.DB(), subject)
		if err != nil {
			t.Fatal(err)
		}
		if len(exported.CommitmentAdmissions) != 1 {
			t.Fatalf("renewal subject has %d admissions", len(exported.CommitmentAdmissions))
		}
		admission := exported.CommitmentAdmissions[0]
		if admission["renewed_by"] != f.messages[1].String() || admission["renewed_at"] == nil {
			t.Fatalf("export lost renewal provenance: %#v", admission)
		}
		if private && admission["summary"] != nil {
			t.Fatal("export disclosed private renewal words")
		}
		if !private && admission["summary"] != "Send confidential enterprise pricing" {
			t.Fatal("export lost the subject's held conclusion")
		}
	}
	check(false)
	if _, err := f.e.Activities.SetAudience(f.e.Admin(), ids.From[ids.ActivityKind](f.messages[1]), activities.SetAudienceInput{
		Audience: "selected", Members: []activities.AudienceMember{{SubjectType: "user", SubjectID: f.e.Rep1}},
	}); err != nil {
		t.Fatal(err)
	}
	check(true)
}

func TestRefusedRenewalPreservesTheClaimAndItsEvidence(t *testing.T) {
	for _, state := range []string{"pending", "unread", "legacy_unread"} {
		t.Run(state, func(t *testing.T) {
			unread := state != "pending"
			f := setupRenewalPrivacy(t, 0.75)
			if state == "legacy_unread" {
				f.e.WsExec(t, `DELETE FROM activity_commitment_admission WHERE source_activity_id=$1`, f.messages[0])
			}
			now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
			later, _, err := f.e.Activities.LogActivity(f.e.Admin(), activities.LogActivityInput{
				Kind: "email", Body: new("I will send pricing next Friday."), OccurredAt: &now, Source: "manual", Direction: new("outbound"), ThreadKey: "renewal-privacy",
			})
			if err != nil {
				t.Fatal(err)
			}
			if unread {
				after := now.Add(time.Minute)
				if _, _, err := f.e.Activities.LogActivity(f.e.Admin(), activities.LogActivityInput{
					Kind: "email", Body: new("Cancel that promise."), OccurredAt: &after, Source: "manual", Direction: new("outbound"), ThreadKey: "renewal-privacy",
				}); err != nil {
					t.Fatal(err)
				}
			}
			renewal := ids.UUID(later.Id)
			c := Commitment{
				AsOf: now.Add(time.Hour), ReadMessages: append(f.messages, renewal), Extractor: signalScanActor,
				SourceActivityID: f.messages[0], AdmissionSourceID: &renewal, RenewedAt: &now,
				Summary: "Send pricing next Friday", Quote: "I will send pricing.", Seat: &f.e.Rep1, PromisedTo: &f.contact,
				Confidence: 0.75, Locator: "private-renewal", DueDate: "2026-10-23",
			}
			ctx := withTranscriptReader(principal.WithWorkspaceID(context.Background(), f.e.WS), "human:"+f.e.Rep1.String(), ids.NewV7())
			dispatcher := NewCommitmentDispatcher(f.e.Pool, approvals.NewService(f.e.DB()))
			if err := database.WithWorkspaceTx(ctx, f.e.Pool, func(tx pgx.Tx) error {
				_, _, err := dispatcher.DispatchTx(ctx, tx, c, ids.NewV7())
				return err
			}); err != nil {
				t.Fatal(err)
			}
			if got := f.e.WsScalar(t, `SELECT to_char(due_at,'YYYY-MM-DD') FROM conversation_claim WHERE source_activity_id=$1`, f.messages[0]); got != "2026-10-19" {
				t.Fatalf("pending proposal's claim moved to %s", got)
			}
			if state == "legacy_unread" {
				if got := f.e.WsCount(t, `SELECT count(*) FROM activity_commitment_admission WHERE source_activity_id=$1`, f.messages[0]); got != 0 {
					t.Fatal("unread renewal created admission for a legacy claim")
				}
				return
			}
			if got := f.e.WsScalar(t, `SELECT renewed_by::text FROM activity_commitment_admission WHERE source_activity_id=$1`, f.messages[0]); got != f.messages[1].String() {
				t.Fatalf("pending proposal's evidence changed to %s", got)
			}
		})
	}
}

func TestArchivingEvidenceKeepsAnAdoptedCommitmentReadable(t *testing.T) {
	f := setupRenewalPrivacy(t, 0.95)
	if _, err := f.e.Activities.UpdateActivity(f.e.Admin(), ids.From[ids.ActivityKind](f.work), activities.UpdateActivityInput{Subject: new("My negotiated pricing task")}); err != nil {
		t.Fatal(err)
	}
	for _, source := range f.messages {
		if _, err := f.e.Activities.ArchiveActivity(f.e.Admin(), ids.From[ids.ActivityKind](source), nil); err != nil {
			t.Fatal(err)
		}
		task, err := f.e.Activities.GetActivity(f.e.Admin(), ids.From[ids.ActivityKind](f.work), storekit.LiveOnly)
		if err != nil || task.Subject == nil || *task.Subject != "My negotiated pricing task" {
			t.Fatalf("archiving evidence hid adopted work: %v", err)
		}
	}
}

func TestAnonymizingAContactPreservesDerivedWorkAndItsPrivateBoundary(t *testing.T) {
	for _, state := range []string{"automatic", "edited", "completed", "held"} {
		t.Run(state, func(t *testing.T) {
			f := setupRenewalPrivacy(t, 0.95)
			subject := f.e.SeedContact(t, "Renewal subject", &f.e.Rep1)
			integration.LinkActivity(t, integration.OwnerConn(t), f.messages[1], "contact", subject)
			if state == "edited" || state == "completed" {
				patch := activities.UpdateActivityInput{Subject: new("My negotiated task")}
				if state == "completed" {
					patch = activities.UpdateActivityInput{IsDone: new(true)}
				}
				if _, err := f.e.Activities.UpdateActivity(f.e.Admin(), ids.From[ids.ActivityKind](f.work), patch); err != nil {
					t.Fatal(err)
				}
			}
			if state == "held" {
				integration.LinkActivity(t, integration.OwnerConn(t), f.work, "contact", f.contact.UUID)
				f.e.WsExec(t, `UPDATE contact SET legal_hold=true WHERE id=$1`, f.contact)
			}
			if _, err := f.e.Activities.SetAudience(f.e.Admin(), ids.From[ids.ActivityKind](f.messages[1]), activities.SetAudienceInput{
				Audience: "selected", Members: []activities.AudienceMember{{SubjectType: "user", SubjectID: f.e.Rep1}},
			}); err != nil {
				t.Fatal(err)
			}
			reader := f.e.As(f.e.Rep2, []ids.UUID{f.e.Team1}, integration.AdminPerms)
			hidden, err := f.e.Activities.GetActivity(reader, ids.From[ids.ActivityKind](f.work), storekit.LiveOnly)
			if err != nil || hidden.Subject != nil {
				t.Fatalf("fixture did not limit renewal content: %v", err)
			}
			before := f.e.WsScalar(t, `SELECT to_jsonb(a)::text FROM activity a WHERE id=$1`, f.work)
			service := privacy.NewRetentionService(f.e.DB(), blobstore.NewMemory(), slog.Default(), RawCapturePurgerFor(f.e.DB()))
			count, err := service.AnonymiseContacts(f.e.Admin(), []ids.UUID{subject}, privacy.PurgeOwnerRule, func(context.Context, pgx.Tx, ids.UUID) (bool, error) { return true, nil })
			if err != nil || count != 1 {
				t.Fatalf("anonymize = %d, %v", count, err)
			}
			if after := f.e.WsScalar(t, `SELECT to_jsonb(a)::text FROM activity a WHERE id=$1`, f.work); after != before {
				t.Fatal("anonymization altered derived work")
			}
			if got := f.e.WsCount(t, `SELECT count(*) FROM activity WHERE id=$1 AND body IS NOT NULL`, f.messages[1]); got != 1 {
				t.Fatal("anonymization erased the retained message")
			}
			task, err := f.e.Activities.GetActivity(reader, ids.From[ids.ActivityKind](f.work), storekit.LiveOnly)
			if err != nil || task.Subject != nil {
				t.Fatalf("anonymization widened the private source boundary: %v", err)
			}
			if got := f.e.WsCount(t, `SELECT count(*) FROM activity_commitment_admission WHERE renewed_by=$1 AND summary='' AND source_quote=''`, f.messages[1]); got != 1 {
				t.Fatal("the decision retained personal prose or lost provenance")
			}
		})
	}
}

func assertHeldRenewalRefused(t *testing.T, f renewalPrivacyFixture, before string) {
	t.Helper()
	c := f.commitment
	at := c.AsOf
	body := "I will send the revised confidential pricing next week."
	message, _, err := f.e.Activities.LogActivity(f.e.Admin(), activities.LogActivityInput{
		Kind: "email", Body: &body, OccurredAt: &at, Source: "manual", ThreadKey: "renewal-privacy", Direction: new("outbound"),
	})
	if err != nil {
		t.Fatal(err)
	}
	renewal := ids.UUID(message.Id)
	c.AdmissionSourceID, c.RenewedAt = &renewal, &at
	c.AsOf, c.DueDate, c.Summary = at.Add(time.Hour), "2026-10-25", "Send revised confidential pricing"
	c.ReadMessages = append(c.ReadMessages, renewal)
	c.RenewalEvidence = &approvals.Evidence{SourceType: "activity", SourceID: renewal, Snippet: body}
	ctx := withTranscriptReader(principal.WithWorkspaceID(context.Background(), f.e.WS), "human:"+f.e.Rep1.String(), ids.NewV7())
	dispatcher := NewCommitmentDispatcher(f.e.Pool, approvals.NewService(f.e.DB()))
	if err := database.WithWorkspaceTx(ctx, f.e.Pool, func(tx pgx.Tx) error {
		_, _, err := dispatcher.DispatchTx(ctx, tx, c, ids.NewV7())
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if after := f.e.WsScalar(t, `SELECT to_jsonb(a)::text FROM activity a WHERE id=$1`, f.work); after != before {
		t.Fatal("automation rewrote legally held work after erasure")
	}
	if got := f.e.WsCount(t, `SELECT count(*) FROM conversation_claim WHERE source_activity_id=$1`, f.messages[0]); got != 0 {
		t.Fatal("automation recreated a claim from held work")
	}
	if got := f.e.WsScalar(t, `SELECT outcome FROM activity_commitment_admission WHERE source_activity_id=$1`, f.messages[0]); got != string(activities.CommitmentSourceUnavailable) {
		t.Fatalf("automation replaced the erased-source boundary: %s", got)
	}
}

func assertReleasedHoldFinishesErasure(t *testing.T, f renewalPrivacyFixture, before string) {
	t.Helper()
	service := privacy.NewRetentionService(f.e.DB(), blobstore.NewMemory(), slog.Default(), RawCapturePurgerFor(f.e.DB()))
	run := func() {
		t.Helper()
		if err := service.EvaluateInstallation(integration.RetentionPassCtx(f.e.WS)); err != nil {
			t.Fatal(err)
		}
	}
	run()
	if after := f.e.WsScalar(t, `SELECT to_jsonb(a)::text FROM activity a WHERE id=$1`, f.work); after != before {
		t.Fatal("retention altered work while the hold remained active")
	}
	if err := NewLegalHoldSeam(f.e.Pool).SetLegalHold(f.e.Admin(), "contact", f.contact.UUID, false, "Proceedings concluded"); err != nil {
		t.Fatal(err)
	}
	run()
	if got := f.e.WsCount(t, `SELECT count(*) FROM activity WHERE id=$1 AND body IS NULL AND archived_at IS NOT NULL AND subject NOT LIKE '%confidential%'`, f.work); got != 1 {
		t.Fatal("released work retained erased-source content")
	}
	if got := f.e.WsCount(t, `SELECT count(*) FROM activity_commitment_admission WHERE source_activity_id=$1`, f.messages[0]); got != 0 {
		t.Fatal("completed erasure retained the suspended decision")
	}
	if got := f.e.WsScalar(t, `SELECT evidence->>'cause' FROM audit_log WHERE entity_id=$1 AND action='erase' ORDER BY occurred_at DESC, id DESC LIMIT 1`, f.work); got != "legal_hold_ended" {
		t.Fatalf("deferred erasure cause = %s", got)
	}
	audits := f.e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_id=$1`, f.work)
	run()
	if got := f.e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_id=$1`, f.work); got != audits {
		t.Fatal("completed erasure repeated its audit on the next pass")
	}
}

func TestErasingLegacyCommitmentsAlsoPreservesHeldEvidence(t *testing.T) {
	for _, held := range []bool{false, true} {
		t.Run(fmt.Sprintf("held_%t", held), func(t *testing.T) {
			f := setupRenewalPrivacy(t, 0.95)
			// The task writer predates the ledger; reproduce that persisted shape.
			f.e.WsExec(t, `DELETE FROM activity_commitment_admission WHERE source_activity_id=$1`, f.messages[0])
			subject := f.e.SeedContact(t, "Original promise subject", &f.e.Rep1)
			integration.LinkActivity(t, integration.OwnerConn(t), f.messages[0], "contact", subject)
			if held {
				integration.LinkActivity(t, integration.OwnerConn(t), f.work, "contact", f.contact.UUID)
				if err := NewLegalHoldSeam(f.e.Pool).SetLegalHold(f.e.Admin(), "contact", f.contact.UUID, true, "Preserve proceedings evidence"); err != nil {
					t.Fatal(err)
				}
			}
			before := f.e.WsScalar(t, `SELECT to_jsonb(a)::text FROM activity a WHERE id=$1`, f.work)
			if err := privacy.NewEraser(f.e.DB()).WithRawCapturePurger(RawCapturePurgerFor(f.e.DB())).EraseContact(f.e.Admin(), subject, "Subject requested erasure"); err != nil {
				t.Fatal(err)
			}
			if held {
				if after := f.e.WsScalar(t, `SELECT to_jsonb(a)::text FROM activity a WHERE id=$1`, f.work); after != before {
					t.Fatal("erasure altered held legacy work")
				}
				task, err := f.e.Activities.GetActivity(f.e.Admin(), ids.From[ids.ActivityKind](f.work), storekit.LiveOnly)
				if err != nil || task.Subject != nil {
					t.Fatalf("legacy task exposed erased source content: %v", err)
				}
				assertReleasedHoldFinishesErasure(t, f, before)
			} else if got := f.e.WsCount(t, `SELECT count(*) FROM activity WHERE id=$1 AND body IS NULL AND archived_at IS NOT NULL AND subject NOT LIKE '%confidential%'`, f.work); got != 1 {
				t.Fatal("legacy task retained erased-source content")
			}
		})
	}
}

func embedHeldCommitment(t *testing.T, f renewalPrivacyFixture) bool {
	t.Helper()
	vector := make([]float32, 1024)
	vector[0] = 1
	wrote, err := search.NewStore(f.e.DB()).UpsertEmbedding(principal.SystemActing(f.e.Admin(), "embedding"), "activity", f.work,
		"Send confidential enterprise pricing", fixedVectorEmbedder{vectors: [][]float32{vector}, dims: len(vector)})
	if err != nil {
		t.Fatal(err)
	}
	return wrote
}

func TestDirectTaskErasureCompletesItsSuspendedEvidenceCleanup(t *testing.T) {
	f := setupRenewalPrivacy(t, 0.95)
	subject := f.e.SeedContact(t, "Renewal subject", &f.e.Rep1)
	integration.LinkActivity(t, integration.OwnerConn(t), f.messages[1], "contact", subject)
	integration.LinkActivity(t, integration.OwnerConn(t), f.work, "contact", f.contact.UUID)
	holds := NewLegalHoldSeam(f.e.Pool)
	if err := holds.SetLegalHold(f.e.Admin(), "contact", f.contact.UUID, true, "Preserve proceedings evidence"); err != nil {
		t.Fatal(err)
	}
	eraser := privacy.NewEraser(f.e.DB()).WithRawCapturePurger(RawCapturePurgerFor(f.e.DB()))
	if err := eraser.EraseContact(f.e.Admin(), subject, "Subject requested erasure"); err != nil {
		t.Fatal(err)
	}
	if err := holds.SetLegalHold(f.e.Admin(), "contact", f.contact.UUID, false, "Proceedings concluded"); err != nil {
		t.Fatal(err)
	}
	if err := eraser.EraseContact(f.e.Admin(), f.contact.UUID, "Task subject requested erasure"); err != nil {
		t.Fatal(err)
	}
	if got := f.e.WsCount(t, `SELECT count(*) FROM activity_commitment_admission WHERE source_activity_id=$1`, f.messages[0]); got != 0 {
		t.Fatal("direct erasure left a second erasure pending")
	}
	if got := f.e.WsCount(t, `SELECT count(*) FROM activity WHERE id=$1 AND body IS NULL AND archived_at IS NOT NULL`, f.work); got != 1 {
		t.Fatal("the task was not erased through its contact")
	}
}
