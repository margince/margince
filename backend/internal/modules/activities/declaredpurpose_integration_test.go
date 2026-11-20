// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"context"
	"errors"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// A send is allowed under the class its purpose declares. What it declared is
// kept on the outbound row and in its audit entry, so a compliance reader can
// count it afterwards.

// seedPurposes defines the purposes the tests name, as an installation seeds them.
func (e *sendEnv) seedPurposes(t *testing.T) {
	t.Helper()
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO consent_purpose (key, label, class) VALUES
		  ('business_correspondence', 'Correspondence', 'business_correspondence'),
		  ('transactional', 'Transactional', 'transactional'),
		  ('newsletter', 'Newsletter', 'marketing')
		ON CONFLICT (key) DO NOTHING`); err != nil {
		t.Fatalf("seeding the purposes: %v", err)
	}
}

// declaredOn reads what the outbound row and its create audit entry say the
// send declared. NULL reads as "".
func (e *sendEnv) declaredOn(t *testing.T, id ids.UUID) (row, audited declaredPurpose) {
	t.Helper()
	if err := e.owner.QueryRow(context.Background(), `
		SELECT coalesce(a.consent_purpose, ''), coalesce(a.consent_class, ''),
		       coalesce(l.after->>'consent_purpose', ''), coalesce(l.after->>'consent_class', '')
		  FROM activity a
		  JOIN audit_log l ON l.entity_id = a.id AND l.entity_type = 'activity' AND l.action = 'create'
		 WHERE a.id = $1`, id).Scan(&row.key, &row.class, &audited.key, &audited.class); err != nil {
		t.Fatalf("reading the declared purpose: %v", err)
	}
	return row, audited
}

func (e *sendEnv) assertDeclared(t *testing.T, id ids.UUID, want declaredPurpose) {
	t.Helper()
	row, audited := e.declaredOn(t, id)
	if row != want {
		t.Fatalf("the outbound row declares %+v, want %+v", row, want)
	}
	if audited != want {
		t.Fatalf("the send's audit entry names %+v, want %+v", audited, want)
	}
}

// Mutation: drop the declared field from outboundMessage.activity and every
// case reads an empty declaration.
func TestEachMailSendRecordsThePurposeItDeclaredAndItsClass(t *testing.T) {
	cases := []struct {
		name   string
		reply  bool
		in     func() SendEmailInput
		wanted declaredPurpose
	}{
		{"a reply re-labelled as correspondence", true, func() SendEmailInput { return soloSendInput("business_correspondence") },
			declaredPurpose{"business_correspondence", "business_correspondence"}},
		{"an account-started transactional send", false, func() SendEmailInput { return soloSendInput("Transactional ") },
			declaredPurpose{"transactional", "transactional"}},
		{"a send naming only its marketing purpose", false, func() SendEmailInput {
			in := soloSendInput("")
			in.MarketingPurpose = "newsletter"
			return in
		}, declaredPurpose{"newsletter", "marketing"}},
		{"a key no purpose defines", true, func() SendEmailInput { return soloSendInput("made_up") },
			declaredPurpose{"made_up", ""}},
		{"a send naming no purpose", true, func() SendEmailInput { return soloSendInput("") },
			declaredPurpose{}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := setupSend(t)
			e.seedPurposes(t)
			store := e.store(stubUnsubscribeLinker{token: testUnsubscribeTok, ok: true}).WithRecipientDirectory(knownRecipients{})
			origin := accountOrigin(e.seedCompany(t))
			if c.reply {
				origin = FromActivity(e.seedAnchor(t, "", ""))
			}
			sent, err := store.SendEmail(
				e.as(principal.RowScopeAll), origin, c.in(), stubConsentGate{}, &recordingStager{})
			if err != nil {
				t.Fatalf("SendEmail: %v", err)
			}
			e.assertDeclared(t, ids.UUID(sent.Id), c.wanted)
		})
	}
}

// A held message and a scheduled one are written by the same writer when they
// finally go, and keep the purpose they were frozen with.
func TestAHeldAndAScheduledSendRecordThePurposeTheyWereFrozenWith(t *testing.T) {
	t.Run("held, then resumed", func(t *testing.T) {
		e := setupSend(t)
		e.seedPurposes(t)
		ctx := e.as(principal.RowScopeAll)
		store := e.store(stubUnsubscribeLinker{token: testUnsubscribeTok, ok: true})
		intent := store.holdForReview(ctx, FromActivity(e.seedAnchor(t, "", "")), soloSendInput("business_correspondence")).id
		if intent.IsZero() {
			t.Fatal("the refused message was not held")
		}
		sent, err := store.ResumeHeldSend(ctx, intent, stubConsentGate{}, &deliveryWritingStager{userID: e.rep})
		if err != nil {
			t.Fatalf("ResumeHeldSend: %v", err)
		}
		e.assertDeclared(t, ids.UUID(sent.Id), declaredPurpose{"business_correspondence", "business_correspondence"})
	})
	t.Run("scheduled, then fired", func(t *testing.T) {
		e := setupSend(t)
		e.seedPurposes(t)
		ctx := e.as(principal.RowScopeAll)
		out, err := draftStore(e).SendOrSchedule(ctx, FromActivity(e.seedAnchor(t, "", "")), soloSendInput("transactional"),
			&SendSchedule{At: draftClock.Add(time.Hour), TZ: "Europe/Berlin"}, stubConsentGate{}, &recordingStager{}, &recordingTimer{})
		if err != nil || out.Scheduled == nil {
			t.Fatalf("SendOrSchedule = %+v, %v", out, err)
		}
		fired, err := e.store(stubUnsubscribeLinker{}).WithClock(func() time.Time { return draftClock.Add(time.Hour + time.Minute) }).
			FireScheduledSend(ctx, out.Scheduled.ID, 10*time.Minute, stubConsentGate{}, &deliveryWritingStager{userID: e.rep})
		if err != nil || !fired.Sent {
			t.Fatalf("FireScheduledSend: sent=%v held=%q err=%v", fired.Sent, fired.Held, err)
		}
		e.assertDeclared(t, ids.UUID(fired.Released.Id), declaredPurpose{"transactional", "transactional"})
	})
}

// Mutation: drop the declared field from outboundChannelMessage.activity.
func TestAChannelReplyRecordsThePurposeItDeclared(t *testing.T) {
	e := setupSend(t)
	e.seedPurposes(t)
	anchor := e.seedChannelAnchor(t)
	contact := e.linkContact(t, anchor, "Telegram Buyer")
	in := channelInput()
	in.ConsentPurpose = "business_correspondence"
	sent, err := e.channelStore(reaches(map[ids.UUID]string{contact: testChannelAccount})).SendMessage(
		e.as(principal.RowScopeAll), anchor, in, &recordingConsentGate{}, &recordingChannelStager{})
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	e.assertDeclared(t, ids.UUID(sent.Id), declaredPurpose{"business_correspondence", "business_correspondence"})
}

// asCompliance is a seat holding the compliance read grant and nothing else.
func (e *sendEnv) asCompliance(grant principal.ObjectGrant) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.ws)
	return principal.WithActor(principal.WithCorrelationID(ctx, ids.NewV7()), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + e.other.String(), UserID: e.other,
		Permissions: principal.Permissions{
			RoleKeys: []string{"ops"},
			Objects:  map[string]principal.ObjectGrant{"consent_config": grant},
			RowScope: principal.RowScopeAll,
		},
	})
}

// Each purpose is counted on its own, only inside the window, and only for a
// seat that may read the compliance configuration.
//
// Mutations: drop the occurred_at bound and the old send is counted; drop the
// auth.Require and the rep's read answers.
func TestPurposeSendCountsCountEachPurposeInsideTheWindow(t *testing.T) {
	e := setupSend(t)
	e.seedPurposes(t)
	store := e.store(stubUnsubscribeLinker{token: testUnsubscribeTok, ok: true})
	send := func(purpose string) ids.UUID {
		sent, err := store.SendEmail(e.as(principal.RowScopeAll), FromActivity(e.seedAnchor(t, "", "")),
			soloSendInput(purpose), stubConsentGate{}, &recordingStager{})
		if err != nil {
			t.Fatalf("SendEmail(%s): %v", purpose, err)
		}
		return ids.UUID(sent.Id)
	}
	send("business_correspondence")
	send("business_correspondence")
	send("transactional")
	old := send("transactional")
	send("")
	if _, err := e.owner.Exec(context.Background(),
		`UPDATE activity SET occurred_at = now() - interval '40 days' WHERE id = $1`, old); err != nil {
		t.Fatal(err)
	}

	counts, err := store.PurposeSendCounts(e.asCompliance(principal.ObjectGrant{Read: true}), 30)
	if err != nil {
		t.Fatalf("PurposeSendCounts: %v", err)
	}
	correspondence, transactional := crmcontracts.ConsentPurposeClassBusinessCorrespondence, crmcontracts.ConsentPurposeClassTransactional
	want := []crmcontracts.ConsentPurposeSendCount{
		{PurposeKey: "business_correspondence", Class: &correspondence, Sends: 2},
		{PurposeKey: "transactional", Class: &transactional, Sends: 1},
	}
	if len(counts) != len(want) {
		t.Fatalf("counts = %+v, want %+v", counts, want)
	}
	for i := range want {
		if counts[i].PurposeKey != want[i].PurposeKey || counts[i].Sends != want[i].Sends ||
			counts[i].Class == nil || *counts[i].Class != *want[i].Class {
			t.Fatalf("count %d = %+v, want %+v", i, counts[i], want[i])
		}
	}

	wider, err := store.PurposeSendCounts(e.asCompliance(principal.ObjectGrant{Read: true}), 60)
	if err != nil {
		t.Fatalf("PurposeSendCounts over 60 days: %v", err)
	}
	if len(wider) != 2 || wider[1].PurposeKey != "transactional" || wider[1].Sends != 2 {
		t.Fatalf("a 60-day window counts %+v, want the old transactional send as well", wider)
	}

	if _, err := store.PurposeSendCounts(e.as(principal.RowScopeAll), 30); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("a rep without consent_config:read: err = %v, want permission denied", err)
	}
}
