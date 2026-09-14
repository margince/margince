// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose_test

// One message is one activity, whichever mail connector read it.
//
// These drive the REAL mapper every mail adapter calls (mailmap.Parse →
// ToRecord) through the REAL sink, because the defect this proves absent lived
// exactly in the seam between them: the mapper stamped the adapter's name into
// the identity, and the sink's unique index then filed one message under as
// many rows as a workspace had connectors. A hand-built NormalizedRecord would
// have asserted the fixture's opinion of the key instead of the mapper's.

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/capture/mailmap"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// theSameMessage is one RFC822 message, byte-identical however it is fetched —
// which is the honest shape of the defect: two connectors pulling one mailbox
// hand the sink the same bytes twice.
func theSameMessage(messageID, from, to string) []byte {
	return []byte(strings.Join([]string{
		"From: Pat Counterparty <" + from + ">",
		"To: " + to,
		"Subject: Angebot fuer 10 Plaetze",
		"Date: Wed, 04 Jun 2026 08:00:00 +0000",
		"Message-ID: <" + messageID + ">",
		"Content-Type: text/plain; charset=utf-8",
		"",
		"Anbei das Angebot.",
		"",
	}, "\r\n"))
}

// connectorCtx binds the principal the registry builds for ONE adapter. The
// connector name lives here and nowhere in the record: that is the whole point
// — provenance is authenticated, identity is not adapter-specific.
func connectorCtx(e *integration.Env, adapter string, owner ids.UUID) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalConnector, ID: "connector:" + adapter,
		UserID: owner, OnBehalfOf: owner,
		Permissions: principal.Permissions{
			Objects: map[string]principal.ObjectGrant{
				"activity": {Create: true, Read: true},
				"contact":  {Create: true, Read: true, Update: true},
				"company":  {Create: true, Read: true, Update: true},
			},
			RowScope: principal.RowScopeAll,
		},
	})
}

// captureThrough runs one message through the adapter's own mapping and the
// production sink, exactly as gmail.go / imap.go / graph.go do.
func captureThrough(t *testing.T, e *integration.Env, adapter string, owner ids.UUID, ownerAddr string, raw []byte) ids.UUID {
	t.Helper()
	parsed, err := mailmap.Parse(raw, ownerAddr)
	if err != nil {
		t.Fatalf("%s: parsing the message: %v", adapter, err)
	}
	ref, err := capture.NewSink(e.DB()).Upsert(connectorCtx(e, adapter, owner), parsed.ToRecord(adapter, raw))
	if err != nil {
		t.Fatalf("%s: capturing the message: %v", adapter, err)
	}
	return ref.ID
}

// liveActivitiesFor counts the timeline rows a Message-ID produced. It asks by
// source_id alone, deliberately: the question is how many rows one message
// made, and asking with the source_system the writer used would assume the
// answer.
func liveActivitiesFor(t *testing.T, e *integration.Env, messageID string) []ids.UUID {
	t.Helper()
	var out []ids.UUID
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(context.Background(),
			`SELECT id FROM activity WHERE source_id = $1 AND archived_at IS NULL ORDER BY id`, messageID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id ids.UUID
			if err := rows.Scan(&id); err != nil {
				return err
			}
			out = append(out, id)
		}
		return rows.Err()
	}); err != nil {
		t.Fatalf("counting the activities for %s: %v", messageID, err)
	}
	return out
}

func scalar[T any](t *testing.T, e *integration.Env, query string, args ...any) T {
	t.Helper()
	var got T
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), query, args...).Scan(&got)
	}); err != nil {
		t.Fatalf("reading %q: %v", query, err)
	}
	return got
}

// The defect, in both arrival orders: every ordered pair of mail connectors
// must land ONE activity for one message.
//
// Both orders matter and they are not symmetric in the code — one arrival
// inserts and the other takes the replay path — so testing gmail-then-imap
// alone would leave the reverse unproven.
func TestOneMessageIsOneActivityWhicheverConnectorReadsItFirst(t *testing.T) {
	e := integration.Setup(t)
	owner := e.Rep1
	const ownerAddr = "rep@ws.example"

	for _, order := range [][2]string{
		{"gmail", "imap"},
		{"imap", "gmail"},
		{"graph", "gmail"},
		{"imap", "graph"},
	} {
		first, second := order[0], order[1]
		t.Run(first+" then "+second, func(t *testing.T) {
			messageID := fmt.Sprintf("dedupe-%s-%s@acme.test", first, second)
			raw := theSameMessage(messageID, "pat@counterparty.test", ownerAddr)

			firstID := captureThrough(t, e, first, owner, ownerAddr, raw)
			secondID := captureThrough(t, e, second, owner, ownerAddr, raw)

			if firstID != secondID {
				t.Errorf("%s landed %s and %s landed %s — one message became two timeline rows",
					first, firstID, second, secondID)
			}
			if live := liveActivitiesFor(t, e, messageID); len(live) != 1 {
				t.Errorf("%d live activities carry message id %s, want 1: %v", len(live), messageID, live)
			}

			// Provenance still names the connector that got there first, and it
			// is the FIRST one: the replay writes no second row, so it must not
			// rewrite the row's history either.
			if got := scalar[string](t, e,
				`SELECT captured_by FROM activity WHERE id = $1`, firstID); !strings.HasPrefix(got, "connector:"+first) {
				t.Errorf("captured_by = %q, want the first reader %q — a replay rewrote provenance", got, first)
			}
			if got := scalar[string](t, e,
				`SELECT source FROM activity WHERE id = $1`, firstID); got != first+":"+messageID {
				t.Errorf("source = %q, want %q — the transport belongs on source even though the identity is shared",
					got, first+":"+messageID)
			}
			// The identity itself is transport-independent.
			if got := scalar[string](t, e,
				`SELECT source_system FROM activity WHERE id = $1`, firstID); got != connector.EmailSourceSystem {
				t.Errorf("source_system = %q, want %q", got, connector.EmailSourceSystem)
			}
			// One message, one stored original — raw_capture keys on the same
			// pair, so a per-connector key would have banked the bytes twice.
			if got := scalar[int](t, e,
				`SELECT count(*) FROM raw_capture WHERE source_id = $1`, messageID); got != 1 {
				t.Errorf("%d raw originals stored for one message, want 1", got)
			}
		})
	}
}

// The original date survives the second connector. A replay that refreshed
// occurred_at to sync time would silently re-order everybody's timeline the
// first time a second mailbox was connected.
func TestASecondConnectorDoesNotMoveTheMessageInTime(t *testing.T) {
	e := integration.Setup(t)
	owner := e.Rep1
	const ownerAddr = "rep@ws.example"
	const messageID = "dedupe-dates@acme.test"
	raw := theSameMessage(messageID, "pat@counterparty.test", ownerAddr)

	id := captureThrough(t, e, "gmail", owner, ownerAddr, raw)
	before := scalar[string](t, e, `SELECT occurred_at::text FROM activity WHERE id = $1`, id)

	captureThrough(t, e, "imap", owner, ownerAddr, raw)
	after := scalar[string](t, e, `SELECT occurred_at::text FROM activity WHERE id = $1`, id)

	if before != after {
		t.Errorf("occurred_at moved from %s to %s when a second connector delivered the same message", before, after)
	}
}

// Two seats, two mailboxes, one message: one activity, and BOTH contributions
// recorded. The dedupe must not cost the second seat their claim on the mail —
// that row is what carries their own posture and verdict.
func TestBothMailboxesThatDeliveredOneMessageAreRecorded(t *testing.T) {
	e := integration.Setup(t)
	const messageID = "dedupe-two-seats@acme.test"
	const firstAddr, secondAddr = "rep1@ws.example", "rep2@ws.example"

	// One message addressed to both colleagues, so each mailbox genuinely
	// received it — capture requires that proof before recording a contribution.
	raw := []byte(strings.Join([]string{
		"From: Pat Counterparty <pat@counterparty.test>",
		"To: " + firstAddr + ", " + secondAddr,
		"Subject: Angebot fuer beide",
		"Date: Wed, 04 Jun 2026 08:00:00 +0000",
		"Message-ID: <" + messageID + ">",
		"Content-Type: text/plain; charset=utf-8",
		"",
		"Anbei.",
		"",
	}, "\r\n"))

	// Each seat claims their own address through the real writer. Capture will
	// not record a contribution on a Message-ID alone — a header the sender
	// types — so without this the message lands once and belongs to nobody,
	// which is the refusal working rather than the dedupe failing.
	claimOwnAddress(t, e, e.Rep1, firstAddr)
	claimOwnAddress(t, e, e.Rep2, secondAddr)

	first := captureThrough(t, e, "gmail", e.Rep1, firstAddr, raw)
	second := captureThrough(t, e, "imap", e.Rep2, secondAddr, raw)

	if first != second {
		t.Fatalf("two colleagues' mailboxes filed one message as %s and %s", first, second)
	}
	if got := scalar[int](t, e,
		`SELECT count(*) FROM capture_import WHERE activity_id = $1`, first); got != 2 {
		t.Errorf("%d mailbox contributions recorded, want 2 — one message, two seats that each received it", got)
	}
}

// The check that keeps the rule true as the tree grows: a mail record keyed on
// an adapter's name is refused at the sink door, before anything is written.
func TestTheSinkRefusesMailKeyedOnAConnectorName(t *testing.T) {
	e := integration.Setup(t)
	const messageID = "dedupe-refused@acme.test"

	_, err := capture.NewSink(e.DB()).Upsert(connectorCtx(e, "gmail", e.Rep1), connector.NormalizedRecord{
		EntityType: "activity",
		// The old shape: the adapter's name as the identity.
		NaturalKey:   connector.NaturalKey{SourceSystem: "gmail", SourceID: messageID},
		Counterparty: connector.Counterparty{Email: "pat@counterparty.test", DisplayName: "Pat"},
		Fields:       capture.ActivityFields{Kind: "email", Subject: "Angebot", Body: "Anbei.", Direction: "inbound"},
		Source:       "gmail:" + messageID, CapturedBy: "connector:gmail",
	})
	if err == nil {
		t.Fatal("the sink accepted an email keyed on a connector name — the identity rule is not enforced")
	}
	if got := liveActivitiesFor(t, e, messageID); len(got) != 0 {
		t.Errorf("the refused record still wrote %d activities: %v", len(got), got)
	}
	if got := scalar[int](t, e,
		`SELECT count(*) FROM raw_capture WHERE source_id = $1`, messageID); got != 0 {
		t.Errorf("the refused record still banked %d raw originals — the refusal must precede every write", got)
	}
}

// claimOwnAddress records a seat's own mailbox address the way the product
// does — through the owner-identity store under that seat's own principal.
// Capture reads it as the proof that this mailbox really received a message
// before it credits the seat with importing it.
func claimOwnAddress(t *testing.T, e *integration.Env, seat ids.UUID, address string) {
	t.Helper()
	ctx := e.As(seat, nil, principal.Permissions{RowScope: principal.RowScopeOwn})
	if _, err := capture.NewOwnerIdentityStore(e.DB()).Add(ctx, capture.IdentityKindAddress, address); err != nil {
		t.Fatalf("claiming %s for the seat: %v", address, err)
	}
}

// The offline demo is a registered connector like any other, so its mail meets
// the same door. Its EMAIL shares the one identity (a demo mailbox behaves like
// a mailbox); its MEETINGS keep the demo's own system, because those never
// carried an RFC822 identity to share. Both must be admitted — a check that
// refused the demo would break `make seed-dev` for everyone.
func TestTheOfflineDemoLandsBothItsKindsThroughTheSameDoor(t *testing.T) {
	e := integration.Setup(t)
	sink := capture.NewSink(e.DB())

	for _, tc := range []struct {
		name, kind, system string
	}{
		{"demo mail shares the mail identity", "email", connector.EmailSourceSystem},
		{"a demo meeting keeps the demo's system", "meeting", "offline_demo"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sourceID := "offline-demo." + tc.kind + "@offline-demo.invalid"
			rec := connector.NormalizedRecord{
				EntityType: "activity",
				NaturalKey: connector.NaturalKey{SourceSystem: tc.system, SourceID: sourceID},
				Fields: capture.ActivityFields{
					Kind: tc.kind, Subject: "Demo", Body: "Demo.", Direction: "inbound",
				},
				Source: "offline_demo:" + sourceID, CapturedBy: "connector:offline_demo",
			}
			if tc.kind == "email" {
				rec.Counterparty = connector.Counterparty{Email: "pat@counterparty.test", DisplayName: "Pat"}
			}
			if _, err := sink.Upsert(connectorCtx(e, "offline_demo", e.Rep1), rec); err != nil {
				t.Fatalf("the demo's %s was refused: %v", tc.kind, err)
			}
		})
	}
}
