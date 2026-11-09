// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/agentvolume"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/redistest"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/diffhash"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestAgentSendStagingRefusesConsentAndRollsBackTypedRecipients(t *testing.T) {
	for _, kind := range []string{"send_email", "send_company_email"} {
		for _, rest := range []bool{false, true} {
			t.Run(kind+map[bool]string{false: "/tool", true: "/rest"}[rest], func(t *testing.T) {
				e := integration.Setup(t)
				seedMarketingPurpose(t, e, "marketing_email")
				e.GrantRole(t, e.Rep1, "admin")
				passport := e.SeedPassport(t, integration.OwnerConn(t), "send authority")
				e.WsExec(t, `UPDATE passport SET scopes=ARRAY['read','write','send'] WHERE id=$1`, passport)
				live, err := identity.NewService(e.Pool).AuthenticateAgentByID(sightContext(e), ids.From[ids.PassportKind](passport))
				if err != nil {
					t.Fatal(err)
				}
				ctx := principal.WithActor(sightContext(e), live.Principal())
				anchor := seedReplyAnchor(t, e)
				company := e.SeedCompany(t, "Message account", &e.Rep1)
				payload := map[string]any{"to": []string{"new-recipient@customer.test"}, "subject": "Product newsletter", "body": "News about our products.", "consent_purpose": "marketing_email"}
				if kind == "send_company_email" {
					payload["links"] = []map[string]any{{"entity_type": "company", "entity_id": company}}
				}
				if rest {
					payload = map[string]any{"operation": "send", "body": payload}
				}
				raw, err := json.Marshal(payload)
				if err != nil {
					t.Fatal(err)
				}
				canonical, hash, err := diffhash.Canonical(raw)
				if err != nil {
					t.Fatal(err)
				}
				in := approvals.StageInput{Kind: kind, TargetType: "activity", ProposedChange: canonical, DiffHash: hash}
				if kind == "send_email" {
					in.TargetID = anchor
				}
				before := e.WsCount(t, `SELECT count(*) FROM contact`)
				_, _, err = NewApprovalsService(e.DB()).StageAgentCall(ctx, in)
				if !errors.Is(err, apperrors.ErrConsentNotGranted) {
					t.Fatalf("unconsented message staged: %v", err)
				}
				if count := e.WsCount(t, `SELECT count(*) FROM approval`); count != 0 {
					t.Fatalf("refusal left %d approvals", count)
				}
				if count := e.WsCount(t, `SELECT count(*) FROM contact`); count != before {
					t.Fatal("staging created a typed recipient")
				}
				if count := e.WsCount(t, `SELECT count(*) FROM comms_outbound`); count != 0 {
					t.Fatal("staging queued a delivery")
				}
			})
		}
	}
}

func TestComposedApprovalServiceStillReleasesAStepUp(t *testing.T) {
	e := integration.Setup(t)
	passport := e.SeedPassport(t, integration.OwnerConn(t), "step up")
	live, err := identity.NewService(e.Pool).AuthenticateAgentByID(sightContext(e), ids.From[ids.PassportKind](passport))
	if err != nil {
		t.Fatal(err)
	}
	ctx := principal.WithActor(sightContext(e), live.Principal())
	at := clocktest.Now(t)
	meter := agentvolume.NewWithClock(redistest.Client(t), agentvolume.Limits{Writes: 1}, time.Hour, func() time.Time { return at })
	if err := meter.Consume(ctx, agentvolume.Writes, 1); err != nil {
		t.Fatal(err)
	}
	svc := NewApprovalsService(e.DB()).WithVolumeReleaser(connectionReleaser{meter: meter, pool: e.Pool})
	reading := meter.Read(ctx, agentvolume.Writes)
	proposal := agentvolume.NewReleaseProposal(reading, passport, ids.Nil, "update_record")
	id, staged, err := (approvalsAdapter{svc: svc}).StageVolumeRelease(ctx, agents.VolumeReleaseRequest{Proposal: proposal, Summary: "Continue writing?"})
	if err != nil || !staged {
		t.Fatalf("stage step-up: staged=%v err=%v", staged, err)
	}
	if _, err := svc.Decide(e.As(e.Rep1, nil, integration.AdminPerms), id, true, nil); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := meter.Reserve(ctx, agentvolume.Writes, 1); err != nil || !ok {
		t.Fatalf("approved step-up still refused: ok=%v err=%v", ok, err)
	}
}

func TestAgentInvitationStagingChecksTheAttendeesConsent(t *testing.T) {
	e := integration.Setup(t)
	e.GrantRole(t, e.Rep1, "admin")
	passport := e.SeedPassport(t, integration.OwnerConn(t), "invite authority")
	e.WsExec(t, `UPDATE passport SET scopes=ARRAY['read','write','send'] WHERE id=$1`, passport)
	live, err := identity.NewService(e.Pool).AuthenticateAgentByID(sightContext(e), ids.From[ids.PassportKind](passport))
	if err != nil {
		t.Fatal(err)
	}
	ctx := principal.WithActor(sightContext(e), live.Principal())
	contact, err := e.Contacts.CreateContact(e.Admin(), contacts.CreateContactInput{FullName: "Guest", Source: "manual", Emails: []contacts.ContactEmailInput{{Email: "guest@example.test", EmailType: "work", IsPrimary: true}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.Activities.SaveSchedulingProfile(ctx, crmcontracts.SchedulingProfile{Provider: "gcal", CalendarId: "primary", DurationMinutes: 30, NoticeMinutes: 120, HorizonDays: 30, Title: "Discovery"}); err != nil {
		t.Fatal(err)
	}
	request := crmcontracts.MeetingInvitationRequest{ContactId: contact.Id, AttendeeEmail: "guest@example.test", Subject: "Discovery", Start: time.Date(2027, 1, 12, 9, 0, 0, 0, time.UTC), End: time.Date(2027, 1, 12, 9, 30, 0, 0, time.UTC)}
	raw, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	canonical, hash, err := diffhash.Canonical(raw)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = NewApprovalsService(e.DB()).StageAgentCall(ctx, approvals.StageInput{Kind: operationInviteMeeting, TargetType: "contact", TargetID: ids.UUID(contact.Id), ProposedChange: canonical, DiffHash: hash})
	if !errors.Is(err, apperrors.ErrConsentNotGranted) {
		t.Fatalf("unconsented invitation staged: %v", err)
	}
	if e.WsCount(t, `SELECT count(*) FROM approval`)+e.WsCount(t, `SELECT count(*) FROM meeting_invitation`) != 0 {
		t.Fatal("refusal wrote an approval or invitation")
	}
}
