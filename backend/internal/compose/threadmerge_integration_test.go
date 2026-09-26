// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// One conversation is one thread, however its mail arrives.
//
// The defect these hold was found on a real mailbox: eleven messages Gmail
// shows as one thread landed under six thread keys. iPhone Mail sends only the
// direct parent in References, so every reply it made started a new root; an
// imported CRM history kept its own key; and a mailbox taking over an imported
// row kept the import's key beside the mailbox's. Every test here drives the
// production sink (newCaptureSink) and the real mail parser.

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/capture/mailmap"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// threadMail is one message of a conversation with an outside party. inReplyTo
// and references are written exactly as given, so a test can shorten the chain
// the way a real mail program does.
func threadMail(to, messageID, at, inReplyTo string, references ...string) []byte {
	lines := []string{
		"From: Pat Counterparty <pat@counterparty.example>",
		"To: " + to,
		"Subject: Re: Angebot",
		"Date: " + at,
		"Message-ID: <" + messageID + ">",
	}
	if inReplyTo != "" {
		lines = append(lines, "In-Reply-To: <"+inReplyTo+">")
	}
	if len(references) > 0 {
		refs := make([]string, len(references))
		for i, r := range references {
			refs[i] = "<" + r + ">"
		}
		lines = append(lines, "References: "+strings.Join(refs, " "))
	}
	lines = append(lines, "Content-Type: text/plain; charset=utf-8", "", "Text.", "")
	return []byte(strings.Join(lines, "\r\n"))
}

func threadConnectorCtx(e *integration.Env, owner ids.UUID) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalConnector, ID: "connector:gmail",
		UserID: owner, OnBehalfOf: owner,
		Permissions: principal.Permissions{
			Objects: map[string]principal.ObjectGrant{
				"activity": {Create: true, Read: true, Update: true},
				"contact":  {Create: true, Read: true, Update: true},
				"company":  {Create: true, Read: true, Update: true},
			},
			RowScope: principal.RowScopeAll,
		},
	})
}

// seatAddress connects a Gmail mailbox for the seat and answers its address. A
// capture records a seat as holding a message only when the seat's own
// connected mailbox is on it (mailboxWasARecipientTx), so the mail is
// addressed to it.
func seatAddress(t *testing.T, e *integration.Env, seat ids.UUID) string {
	t.Helper()
	addr := "seat-" + seat.String()[:8] + "@ws.example"
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `
			INSERT INTO capture_connection (user_id, provider, status, credential_ref, account_label)
			VALUES ($1, 'gmail', 'connected', 'vault:test', $2)
			ON CONFLICT (user_id, provider) DO UPDATE SET account_label = $2, archived_at = NULL`, seat, addr)
		return err
	}); err != nil {
		t.Fatalf("connecting the seat's mailbox: %v", err)
	}
	return addr
}

func captureThreadMail(t *testing.T, e *integration.Env, owner ids.UUID, raw []byte) {
	t.Helper()
	parsed, err := mailmap.Parse(raw, seatAddress(t, e, owner))
	if err != nil {
		t.Fatalf("parsing the message: %v", err)
	}
	if _, err := newCaptureSink(e.Pool, CaptureConfig{}).Upsert(
		threadConnectorCtx(e, owner), parsed.ToRecord("gmail", raw)); err != nil {
		t.Fatalf("capturing the message: %v", err)
	}
}

func threadKeyOfMessage(t *testing.T, e *integration.Env, messageID string) string {
	t.Helper()
	var key string
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `
			SELECT a.thread_key FROM activity a
			  JOIN activity_identity i ON i.activity_id = a.id
			 WHERE i.identity_kind = 'mail' AND i.identity_key = $1`, messageID).Scan(&key)
	}); err != nil {
		t.Fatalf("reading the thread key of %s: %v", messageID, err)
	}
	return key
}

// Every table with a thread_key column is one the merge rewrites. A table
// added later with such a column and left out would keep the retired key, and
// its rows would silently stop belonging to their conversation.
func TestEveryThreadKeyedTableIsMerged(t *testing.T) {
	e := integration.Setup(t)
	var tables []string
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(context.Background(), `
			SELECT c.table_name FROM information_schema.columns c
			  JOIN information_schema.tables tb
			    ON tb.table_schema = c.table_schema AND tb.table_name = c.table_name
			 WHERE c.table_schema = current_schema() AND c.column_name = 'thread_key'
			   AND tb.table_type = 'BASE TABLE'
			 ORDER BY 1`)
		if err != nil {
			return err
		}
		tables, err = pgx.CollectRows(rows, pgx.RowTo[string])
		return err
	}); err != nil {
		t.Fatalf("reading the schema: %v", err)
	}
	merged := slices.Sorted(slices.Values(threadMergedTables))
	if !slices.Equal(tables, merged) {
		t.Fatalf("tables with a thread_key column: %v\nmergeThreadsTx rewrites: %v\n"+
			"add the missing table to mergeThreadsTx (compose/threadmerge.go) through the module that owns it",
			tables, merged)
	}
}

// A reply chain shortened by the sender's mail program, arriving newest first
// the way a backfill walks a mailbox, still lands as one thread — under the
// opener's key.
func TestAShortenedReferencesChainArrivingOutOfOrderIsOneThread(t *testing.T) {
	e := integration.Setup(t)
	owner := e.Rep1
	addr := seatAddress(t, e, owner)
	opener := threadMail(addr, "opener@counterparty.example", "Mon, 01 Jun 2026 08:00:00 +0000", "")
	second := threadMail(addr, "second@counterparty.example", "Tue, 02 Jun 2026 08:00:00 +0000",
		"opener@counterparty.example", "opener@counterparty.example")
	// iPhone Mail: only the direct parent, so its References root is `second`.
	third := threadMail(addr, "third@counterparty.example", "Wed, 03 Jun 2026 08:00:00 +0000",
		"second@counterparty.example", "second@counterparty.example")

	captureThreadMail(t, e, owner, third)
	captureThreadMail(t, e, owner, opener)
	captureThreadMail(t, e, owner, second)

	for _, id := range []string{"opener@counterparty.example", "second@counterparty.example", "third@counterparty.example"} {
		if got := threadKeyOfMessage(t, e, id); got != "opener@counterparty.example" {
			t.Errorf("%s is filed under thread %q, want the opener's", id, got)
		}
	}
}

// A References entry naming a message this seat does not hold joins nothing.
// The header is the sender's text; an outsider quoting somebody else's
// Message-ID must not move that seat's conversation.
func TestAReferenceToAMessageTheSeatDoesNotHoldMergesNothing(t *testing.T) {
	e := integration.Setup(t)
	// Rep2's conversation, rooted on a message Rep1 never received.
	captureThreadMail(t, e, e.Rep2, threadMail(seatAddress(t, e, e.Rep2), "private@counterparty.example",
		"Mon, 01 Jun 2026 08:00:00 +0000", "private-root@counterparty.example", "private-root@counterparty.example"))
	// Rep1 receives mail claiming to reply to it.
	captureThreadMail(t, e, e.Rep1, threadMail(seatAddress(t, e, e.Rep1), "forged@counterparty.example",
		"Tue, 02 Jun 2026 08:00:00 +0000", "private@counterparty.example", "private@counterparty.example"))

	if got := threadKeyOfMessage(t, e, "private@counterparty.example"); got != "private-root@counterparty.example" {
		t.Fatalf("the other seat's message moved to thread %q; a reference it never held must not reach it", got)
	}
	// Nor is the forged mail merged INTO that conversation: it keeps the root
	// its own header gives it, which the private thread does not carry.
	if got := threadKeyOfMessage(t, e, "forged@counterparty.example"); got == "private-root@counterparty.example" {
		t.Fatal("a mail naming a message its seat never held was merged into that message's thread")
	}
}

// An imported conversation and the mailbox that later takes its messages over
// end up as one thread: the import's key meets the mailbox's through the reply
// the mailbox holds.
func TestAMailboxTakingOverAnImportJoinsItsThread(t *testing.T) {
	e := integration.Setup(t)
	owner := e.Rep1
	addr := seatAddress(t, e, owner)
	importInto := func(messageID, at string) {
		occurred, err := time.Parse(time.RFC1123Z, at)
		if err != nil {
			t.Fatal(err)
		}
		direction := crmcontracts.CreateActivityRequestDirectionInbound
		from := "pat@counterparty.example"
		subject, body, system, key := "Re: Angebot", "imported rendering", "hubspot", "hubspot-thread-root@counterparty.example"
		sourceID := "engagement-" + messageID
		in, err := activities.LogActivityInputFrom(crmcontracts.CreateActivityRequest{
			Kind:         crmcontracts.CreateActivityRequestKindCreateActivityRequestKindEmail,
			Subject:      &subject,
			Body:         &body,
			Direction:    &direction,
			OccurredAt:   &occurred,
			SourceSystem: &system,
			SourceId:     &sourceID,
			Source:       "hubspot_import:1",
			RfcMessageId: &messageID,
			ThreadKey:    &key,
			Participants: &struct {
				Cc   *[]string `json:"cc,omitempty"`
				From *string   `json:"from,omitempty"`
				To   *[]string `json:"to,omitempty"`
			}{From: &from, To: &[]string{addr}},
		})
		if err != nil {
			t.Fatalf("mapping the import: %v", err)
		}
		ctx := principal.WithWorkspaceID(context.Background(), e.WS)
		ctx = principal.WithCorrelationID(ctx, ids.NewV7())
		ctx = principal.WithActor(ctx, principal.Principal{
			Type: principal.PrincipalHuman, ID: "human:" + owner.String(),
			UserID: owner, OnBehalfOf: owner,
			Permissions: principal.Permissions{
				Objects: map[string]principal.ObjectGrant{
					"activity": {Create: true, Read: true, Update: true},
					"contact":  {Create: true, Read: true, Update: true},
					"company":  {Create: true, Read: true, Update: true},
				},
				RowScope: principal.RowScopeAll,
			},
		})
		if _, _, err := activities.NewStore(e.DB()).LogActivity(ctx, in); err != nil {
			t.Fatalf("importing %s: %v", messageID, err)
		}
	}
	importInto("imported-one@counterparty.example", "Mon, 01 Jun 2026 08:00:00 +0000")
	importInto("imported-two@counterparty.example", "Tue, 02 Jun 2026 08:00:00 +0000")

	// The mailbox first holds a reply the import never had, rooted by its
	// shortened header on the second imported message...
	captureThreadMail(t, e, owner, threadMail(addr, "mailbox-reply@counterparty.example",
		"Wed, 03 Jun 2026 08:00:00 +0000", "imported-two@counterparty.example", "imported-two@counterparty.example"))
	// ...then syncs the second imported message itself, taking the row over.
	captureThreadMail(t, e, owner, threadMail(addr, "imported-two@counterparty.example",
		"Tue, 02 Jun 2026 08:00:00 +0000", "imported-one@counterparty.example", "imported-one@counterparty.example"))

	want := "hubspot-thread-root@counterparty.example"
	for _, id := range []string{"imported-one@counterparty.example", "imported-two@counterparty.example", "mailbox-reply@counterparty.example"} {
		if got := threadKeyOfMessage(t, e, id); got != want {
			t.Errorf("%s is filed under thread %q, want the imported thread %q", id, got, want)
		}
	}
}

// A seat's verdict on the merged thread is never more open than either half:
// a hold on one half survives an opening answer on the other.
func TestAMergedThreadKeepsTheHoldEitherHalfCarried(t *testing.T) {
	e := integration.Setup(t)
	seat := e.Rep1
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		ctx := context.Background()
		if _, err := tx.Exec(ctx, `
			INSERT INTO capture_thread_verdict (thread_key, user_id, status)
			VALUES ('held-half', $1, $2), ('open-half', $1, $3)`,
			seat, capture.VerdictHeld, capture.VerdictCleared); err != nil {
			return err
		}
		return mergeThreadsTx(ctx, tx, "held-half", "open-half")
	}); err != nil {
		t.Fatalf("merging: %v", err)
	}
	var statuses []string
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(context.Background(), `
			SELECT thread_key || ':' || status FROM capture_thread_verdict
			 WHERE user_id = $1 AND thread_key IN ('held-half', 'open-half')`, seat)
		if err != nil {
			return err
		}
		statuses, err = pgx.CollectRows(rows, pgx.RowTo[string])
		return err
	}); err != nil {
		t.Fatalf("reading verdicts: %v", err)
	}
	if !slices.Equal(statuses, []string{"open-half:" + capture.VerdictHeld}) {
		t.Fatalf("the merged thread's verdicts are %v, want the hold alone on the surviving key", statuses)
	}
}
