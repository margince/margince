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

	"github.com/margince/margince/backend/internal/compose/attention"
	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

var homeReadTime = time.Date(2026, 9, 15, 8, 0, 0, 0, time.UTC)

func homeNotice(t *testing.T, e *integration.Env, owner ids.UUID, private bool, due time.Time) consent.OpenNoticeCase {
	t.Helper()
	ctx := e.As(owner, nil, integration.AdminPerms)
	if owner.IsZero() {
		ctx = principal.WithActor(ctx, principal.Principal{Type: principal.PrincipalSystem, ID: "system", Permissions: integration.AdminPerms})
	}
	contact, err := e.Contacts.CreateContact(ctx, contacts.CreateContactInput{
		FullName: "Agenda contact", Source: "manual",
		Acquisition: contacts.Acquisition{Kind: contacts.AcquiredPurchasedOrImported},
	})
	if err != nil {
		t.Fatal(err)
	}
	contactID := ids.From[ids.ContactKind](ids.UUID(contact.Id))
	if private {
		visibility := "owner"
		if _, err := e.Contacts.UpdateContact(ctx, contactID, contacts.UpdateContactInput{Visibility: &visibility, IfVersion: contact.Version}); err != nil {
			t.Fatal(err)
		}
	}
	var caseID ids.UUID
	err = e.DB().Tx(ctx, func(tx pgx.Tx) error {
		var acquisition ids.UUID
		args := []any{contactID}
		if err := tx.QueryRow(ctx, storekit.SQLf("SELECT id FROM contact_acquisition_evidence WHERE contact_id = $%d", len(args)), args...).Scan(&acquisition); err != nil {
			return err
		}
		if err := consent.OpenNoticeCaseTx(ctx, tx, consent.NoticeCaseInput{ContactID: contactID, AcquisitionID: acquisition, Rule: consent.RuleArt14, DueAt: due}); err != nil {
			return err
		}
		args = []any{acquisition}
		return tx.QueryRow(ctx, storekit.SQLf("SELECT id FROM privacy_notice_case WHERE acquisition_id = $%d", len(args)), args...).Scan(&caseID)
	})
	if err != nil {
		t.Fatal(err)
	}
	return consent.OpenNoticeCase{ID: caseID, ContactID: contactID, DueAt: due}
}

func noticeIDs(rows []crmcontracts.WorklistItem) []string {
	var out []string
	for _, row := range rows {
		if row.Source == "notice_case" {
			out = append(out, row.Id)
		}
	}
	return out
}

func TestHomeNoticeAgendaUsesResponsibilityAndOpenableContacts(t *testing.T) {
	e := integration.Setup(t)
	private := homeNotice(t, e, e.Rep1, true, homeReadTime.Add(-48*time.Hour))
	for i := range 9 {
		homeNotice(t, e, e.Rep1, false, homeReadTime.Add(time.Duration(i-24)*time.Hour))
	}
	mine := homeNotice(t, e, e.AdminUser, false, homeReadTime.Add(-time.Hour))
	store := consent.NewStore(e.DB())
	rows, err := store.OpenNoticeCasesDueSoonest(e.Admin(), consent.NoticeAgendaInput{OwnerID: &e.AdminUser, Limit: 1})
	if err != nil || len(rows) != 1 || rows[0].ID != mine.ID {
		t.Fatalf("personal bounded agenda = %+v, %v; want the reader's duty behind colleagues' earlier deadlines", rows, err)
	}
	feed := newAttentionService(e.Pool, approvals.NewService(e.DB()), failClosedOverlayMeter(), func() time.Time { return homeReadTime })
	day, err := feed.Worklist(e.Admin(), "mine", "all", ids.Nil, 100, "")
	if err != nil {
		t.Fatal(err)
	}
	if got := noticeIDs(day.Queue); len(got) != 1 || got[0] != mine.ID.String() {
		t.Fatalf("personal queue duties = %v, want %s", got, mine.ID)
	}
	if day.Focus == nil {
		t.Fatal("personal agenda has no focus projection")
	}
	if got := noticeIDs(day.Focus.Items); len(got) != 1 || got[0] != mine.ID.String() {
		t.Fatalf("personal focus duties = %v, want %s", got, mine.ID)
	}
	all, err := store.OpenNoticeCasesDueSoonest(e.Admin(), consent.NoticeAgendaInput{Limit: 100})
	if err != nil || len(all) != 10 {
		t.Fatalf("visible agenda has %d duties, %v; want ten openable contacts", len(all), err)
	}
	for _, duty := range all {
		if duty.ID == private.ID {
			t.Fatal("agenda links to an owner-private contact")
		}
	}
	if _, err := e.Contacts.GetContact(e.Admin(), private.ContactID, storekit.LiveOnly); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("private contact read = %v, want not found", err)
	}
	// The explicit officer queue retains its separate compliance authority.
	if _, err := store.GetNoticeCase(e.Admin(), private.ID); err != nil {
		t.Fatalf("explicit compliance case read: %v", err)
	}
	assigned, err := store.AssignNoticeCase(e.Admin(), mine.ID, e.Rep1)
	if err != nil || assigned.OwnerUserID == nil {
		t.Fatalf("assign duty: %+v, %v", assigned, err)
	}
	rows, err = store.OpenNoticeCasesDueSoonest(e.Admin(), consent.NoticeAgendaInput{OwnerID: &e.AdminUser, Limit: 1})
	if err != nil || len(rows) != 0 {
		t.Fatalf("a reassigned case stayed in the contact owner's agenda: %+v, %v", rows, err)
	}
	named, err := feed.Worklist(e.Admin(), "all", "all", e.Rep1, 100, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(noticeIDs(named.Queue)) != 8 {
		t.Fatalf("named colleague queue = %v, want eight bounded visible duties", noticeIDs(named.Queue))
	}
}

func TestNoticeAgendaWithoutContactReadCannotOfferContactLinks(t *testing.T) {
	e := integration.Setup(t)
	homeNotice(t, e, e.AdminUser, false, homeReadTime)
	perms := integration.AdminPerms
	perms.Objects = map[string]principal.ObjectGrant{"privacy_request": {Read: true}}
	ctx := e.As(e.AdminUser, nil, perms)
	_, err := consent.NewStore(e.DB()).OpenNoticeCasesDueSoonest(ctx, consent.NoticeAgendaInput{})
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("agenda without contact read = %v, want permission denied", err)
	}
}

func TestCounterpartyReviewRemainsWithItsImportingSeat(t *testing.T) {
	e := integration.Setup(t)
	activity := seedCapturedMail(t, e, "review@agenda.example", "A possible new contact")
	disposition := seedPendingDisposition(t, e, "review@agenda.example", "agenda.example", activity)
	retireToUnsure(t, e, disposition)
	engine := NewCounterpartyVerdictEngine(
		e.Pool, &scriptedVerdictBrain{}, CaptureConfig{}, slog.Default(),
	)
	if err := engine.StageReviewsWorkspace(e.Admin(), 0); err != nil {
		t.Fatal(err)
	}
	approval := ids.From[ids.ApprovalKind](stagedProposalID(t, e, disposition))
	svc := approvalsServiceWithEffects(e.Pool)
	assertCaptureReviewDelivery(t, e, approval.UUID)
	status := "pending"
	owner := e.As(e.Rep1, nil, integration.AdminPerms)
	for _, tc := range []struct {
		name  string
		ctx   context.Context
		count int
	}{{"owner", owner, 1}, {"other admin", e.Admin(), 0}} {
		t.Run(tc.name, func(t *testing.T) {
			rows, _, err := svc.ListWire(tc.ctx, approvals.ListInput{Status: &status, Limit: 100})
			if err != nil || len(rows) != tc.count {
				t.Fatalf("inbox = %d, %v, want %d", len(rows), err, tc.count)
			}
			targetType := "activity"
			rows, _, err = svc.ListWire(tc.ctx, approvals.ListInput{TargetType: &targetType, TargetID: &activity, Limit: 100})
			if err != nil || len(rows) != tc.count {
				t.Fatalf("target inbox = %d, %v, want %d", len(rows), err, tc.count)
			}
			err = e.DB().Tx(tc.ctx, func(tx pgx.Tx) error {
				pending, err := svc.PendingForTarget(tc.ctx, tx, "activity", activity, 100)
				if err != nil {
					return err
				}
				if len(pending) != tc.count {
					return fmt.Errorf("target pending = %d, want %d", len(pending), tc.count)
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
		})
	}
	if _, err := svc.GetWire(e.Admin(), approval); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("other admin's proposal read = %v", err)
	}
	if _, err := svc.Decide(e.Admin(), approval, true, nil); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("other admin's decision = %v", err)
	}
	if _, err := svc.Decide(owner, approval, true, nil); err != nil {
		t.Fatalf("importer's decision: %v", err)
	}
}

func TestHomeTeamDutiesAreBoundedAfterMembership(t *testing.T) {
	e := integration.Setup(t)
	for i := range 9 {
		homeNotice(t, e, e.Rep3, false, homeReadTime.Add(time.Duration(i-24)*time.Hour))
	}
	teammate := homeNotice(t, e, e.Rep2, false, homeReadTime)
	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms)
	feed := newAttentionService(e.Pool, approvals.NewService(e.DB()), failClosedOverlayMeter(), func() time.Time { return homeReadTime })
	day, err := feed.Worklist(ctx, "team", "all", ids.Nil, 100, "")
	if err != nil {
		t.Fatal(err)
	}
	if got := noticeIDs(day.Queue); len(got) != 1 || got[0] != teammate.ID.String() {
		t.Fatalf("team duties = %v, want teammate behind earlier outside-team work", got)
	}
}

func TestArchivedContactsLeaveTheHomeAgendaButKeepTheirComplianceCase(t *testing.T) {
	e := integration.Setup(t)
	duty := homeNotice(t, e, e.AdminUser, false, homeReadTime)
	if _, err := e.Contacts.ArchiveContact(e.Admin(), duty.ContactID, nil); err != nil {
		t.Fatal(err)
	}
	store := consent.NewStore(e.DB())
	rows, err := store.OpenNoticeCasesDueSoonest(e.Admin(), consent.NoticeAgendaInput{})
	if err != nil || len(rows) != 0 {
		t.Fatalf("archived contact agenda = %v, %v", rows, err)
	}
	if _, err := store.GetNoticeCase(e.Admin(), duty.ID); err != nil {
		t.Fatal(err)
	}
}

func TestNoticeLaneRejectsAMissingNamedOwner(t *testing.T) {
	_, err := (attentionNoticeCases{}).OpenDueSoonest(context.Background(), 8, attention.TasksOwnedBy, ids.Nil, nil)
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("missing named owner = %v", err)
	}
}

func TestUnownedDisclosureDutyAppearsOnlyInUnassignedHome(t *testing.T) {
	e := integration.Setup(t)
	duty := homeNotice(t, e, ids.Nil, false, homeReadTime)
	feed := newAttentionService(e.Pool, approvals.NewService(e.DB()), failClosedOverlayMeter(), func() time.Time { return homeReadTime })
	for _, scope := range []string{"mine", "unassigned"} {
		day, err := feed.Worklist(e.Admin(), scope, "all", ids.Nil, 100, "")
		if err != nil {
			t.Fatal(err)
		}
		got := noticeIDs(day.Queue)
		if scope == "mine" && len(got) != 0 {
			t.Fatalf("unowned duty in Mine: %v", got)
		}
		if scope == "unassigned" && (len(got) != 1 || got[0] != duty.ID.String()) {
			t.Fatalf("unassigned duties = %v", got)
		}
	}
}
