// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A recipient who writes in after a deal follow-up was scheduled pauses it
// until the rep has read their mail. The mail goes through the real capture
// writer, so its time and its sender checks are capture's own.

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/capture/mailmap"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// inbound is one mail from the recipient as the mailbox delivers it.
type inbound struct {
	dated         time.Time // the Date header the sender wrote
	arrived       time.Time // when the provider says it reached the mailbox
	authenticated bool
	extraHeaders  []string
}

// theyWrite captures one mail from the recipient through the capture sink.
func (p *preflightEnv) theyWrite(t *testing.T, mail inbound) {
	t.Helper()
	headers := []string{
		"From: Consented Buyer <buyer@preflight.test>",
		"To: " + sendingMailbox,
		"Subject: Re: Next steps",
		"Date: " + mail.dated.UTC().Format(time.RFC1123Z),
		"Message-ID: <reply-" + mail.dated.Format("20060102150405") + "@preflight.test>",
		"Content-Type: text/plain; charset=utf-8",
	}
	if mail.authenticated {
		headers = append(headers, "Authentication-Results: mx.google.com; dmarc=pass header.from=preflight.test")
	}
	raw := []byte(strings.Join(append(append(headers, mail.extraHeaders...), "", "Thanks, one question first.", ""), "\r\n"))
	parsed, err := mailmap.Parse(raw, sendingMailbox)
	if err != nil {
		t.Fatalf("parsing the reply: %v", err)
	}
	rec := parsed.ToRecord("gmail", raw)
	rec.ProviderReceivedAt = mail.arrived
	// The mailbox the rep connected is theirs, so capture records which seat it
	// arrived in, and when.
	owned := principal.WithActor(p.mailboxCtx(t), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + p.user, UserID: uuidOf(t, p.user),
		Permissions: principal.Permissions{RowScope: principal.RowScopeOwn},
	})
	if _, err := capture.NewOwnerIdentityStore(p.DB()).Add(owned, capture.IdentityKindAddress, sendingMailbox); err != nil {
		t.Fatalf("claiming the rep's mailbox address: %v", err)
	}
	if _, err := capture.NewSink(p.DB()).Upsert(p.mailboxCtx(t), rec); err != nil {
		t.Fatalf("capturing the reply: %v", err)
	}
}

// mailboxCtx is the rep's own mailbox connector, so capture writes the import
// row that carries the provider's arrival time.
func (p *preflightEnv) mailboxCtx(t *testing.T) context.Context {
	t.Helper()
	rep := uuidOf(t, p.user)
	ctx := principal.WithWorkspaceID(context.Background(), p.workspaceID(t))
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalConnector, ID: "connector:gmail", UserID: rep, OnBehalfOf: rep,
		Scopes: principal.NewScopeSet(principal.ScopeRead),
		Permissions: principal.Permissions{
			Objects:  map[string]principal.ObjectGrant{"activity": {Create: true, Read: true}},
			RowScope: principal.RowScopeAll,
		},
	})
}

// A mailing is not someone writing, so it pauses nothing.
//
// Mutation: drop the pursuitPausedByReply call, and the plain reply is sent.
// Compare the Date header instead of the arrival, and the backdated and the
// future-dated cases flip. Drop the bulk test, and the mailing pauses.
func TestAReplyPausesAScheduledDealFollowUp(t *testing.T) {
	dealClaim := AnyMap{"communication_context": "active_deal_followup"}
	// Each mail is written relative to the moment just after the message was
	// scheduled, so "before" and "after" are about that moment.
	for _, tc := range []struct {
		name     string
		claim    AnyMap
		mail     func(after time.Time) inbound
		wantHeld bool
	}{
		{"a reply after scheduling", dealClaim, func(after time.Time) inbound {
			return inbound{dated: after, arrived: after, authenticated: true}
		}, true},
		{"a reply whose Date header is backdated", dealClaim, func(after time.Time) inbound {
			return inbound{dated: after.AddDate(-1, 0, 0), arrived: after, authenticated: true}
		}, true},
		{"mail that arrived before scheduling with a future Date header", dealClaim, func(after time.Time) inbound {
			return inbound{dated: after.AddDate(0, 0, 7), arrived: after.Add(-time.Hour), authenticated: true}
		}, false},
		{"a mailing from their address", dealClaim, func(after time.Time) inbound {
			return inbound{dated: after, arrived: after, authenticated: true,
				extraHeaders: []string{"List-Unsubscribe: <mailto:unsubscribe@preflight.test>"}}
		}, false},
		{"a forged reply", dealClaim, func(after time.Time) inbound {
			return inbound{dated: after, arrived: after}
		}, false},
		{"a message with no deal claim", AnyMap{"consent_purpose": "transactional"}, func(after time.Time) inbound {
			return inbound{dated: after, arrived: after, authenticated: true}
		}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := setupPreflight(t)
			p.connect(t, gmailReadonlyScope, gmailSendScope)
			id := p.scheduleClaiming(t, tc.claim)

			p.theyWrite(t, tc.mail(p.scheduledCreation(t, id).Add(time.Second)))
			p.makeDue(t, id)
			p.fire(t, id)

			status, reason := p.scheduledStatus(t, id)
			held := status == activities.ScheduledStatusHeld && reason == activities.HeldRecipientReplied
			if held != tc.wantHeld {
				t.Fatalf("status %q reason %q, want held for new mail = %v", status, reason, tc.wantHeld)
			}
			if _, carded := p.heldCardFor(t, id); tc.wantHeld && !carded {
				t.Fatal("the paused follow-up raised no card in its rep's inbox")
			}
		})
	}
}

// A scheduled reply listens on its own thread: mail from the recipient on
// another conversation does not pause it.
//
// Mutation: drop the thread_key condition from pursuitPausedByReply, and the
// reply is paused.
func TestAScheduledReplyListensOnItsOwnThread(t *testing.T) {
	p := setupPreflight(t)
	p.connect(t, gmailReadonlyScope, gmailSendScope)
	if err := apptest.InWorkspace(p.AppEnv, t, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(),
			`UPDATE activity SET thread_key = 'the-quote-thread' WHERE id = $1`, uuidOf(t, p.activityID))
		return err
	}); err != nil {
		t.Fatalf("threading the anchor: %v", err)
	}
	id := p.scheduleClaiming(t, AnyMap{"communication_context": "active_deal_followup"})

	after := p.scheduledCreation(t, id).Add(time.Second)
	p.theyWrite(t, inbound{dated: after, arrived: after, authenticated: true})
	p.makeDue(t, id)
	p.fire(t, id)

	if status, reason := p.scheduledStatus(t, id); reason == activities.HeldRecipientReplied {
		t.Fatalf("status %q: mail on another thread paused the scheduled reply", status)
	}
}
