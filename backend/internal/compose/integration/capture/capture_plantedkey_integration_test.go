// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

// A colliding Message-ID costs a mailbox the shared key, never the message.
//
// The natural key of a captured email is the Message-ID, and that header is
// typed by whoever sent the mail. So a collision is evidence of nothing, and
// replayClaimIsProvenTx refuses to give the arriving seat anything over the
// incumbent whenever it cannot prove the two mailboxes hold one message. That
// refusal used to be a DROP: the skip advances the connector's watermark, so
// the arriving seat's own mail was denied by a row they could prove nothing
// about and no later pass retried it.
//
// Through the production sink and two real connected mailboxes, because the
// whole question is which of four SQL arms admit a seat — a fake proof would
// only ever answer whatever it was told to.

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// capturedMail is one message whose subject and body are the caller's, so two
// copies under one Message-ID can be made to differ. email() fixes both, which
// would satisfy the content-equivalence arm and prove the claim rather than
// exercising the refusal this suite is about.
func capturedMail(from, to, msgID, subject, body string) []byte {
	return []byte(strings.Join([]string{
		"From: " + from,
		"To: " + to,
		"Subject: " + subject,
		"Date: Wed, 04 Jun 2026 08:00:00 +0000",
		"Message-ID: <" + msgID + ">",
		"Content-Type: text/plain", "", body, "",
	}, "\r\n"))
}

// mailRow is one captured email as stored, by the columns this suite asks about.
type mailRow struct {
	ID         ids.UUID
	SourceID   string
	Subject    string
	CapturedBy string
}

// capturedMailRows answers every captured email, oldest first.
func capturedMailRows(t *testing.T, e *integration.SearchEnv) []mailRow {
	t.Helper()
	var out []mailRow
	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(context.Background(),
			`SELECT id, coalesce(source_id, ''), coalesce(subject, ''), captured_by
			   FROM activity WHERE kind = 'email' ORDER BY id`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var r mailRow
			if err := rows.Scan(&r.ID, &r.SourceID, &r.Subject, &r.CapturedBy); err != nil {
				return err
			}
			out = append(out, r)
		}
		return rows.Err()
	})
	if err != nil {
		t.Fatalf("listing the captured mail: %v", err)
	}
	return out
}

func TestAMailboxHoldingTheMessageIDDoesNotDenyAnotherSeatTheirOwnMessage(t *testing.T) {
	env := newCaptureEnv(t)
	e, sync := env.e, env.sync
	syncSecond := secondMailbox(t, e, e.Rep3)

	const key = "contested@acme.example"
	// The first mailbox files the key, with content of its own and the second
	// seat on nothing: no import row and no participant row names them, and the
	// content cannot match — so every arm of the replay proof answers no when
	// the second mailbox arrives.
	sync(t, capturedMail("anna@acme.example", captureOwner, key,
		"unrelated", "a note to the first mailbox alone"))

	// The second mailbox now syncs the message it actually holds.
	syncSecond(t, capturedMail("jonas@acme.example", secondSeatAddress, key,
		"Telematics renewal", "please send the signed contract"))

	rows := capturedMailRows(t, e)
	if len(rows) != 2 {
		t.Fatalf("want both mailboxes' messages captured, got %d row(s): %+v — "+
			"a seat's own mail was denied by a row it can prove nothing about", len(rows), rows)
	}

	var own, incumbent *mailRow
	for i := range rows {
		if strings.Contains(rows[i].CapturedBy, e.Rep3.String()) {
			own = &rows[i]
		} else {
			incumbent = &rows[i]
		}
	}
	if own == nil || incumbent == nil {
		t.Fatalf("want one row per mailbox, got %+v", rows)
	}
	if own.Subject != "Telematics renewal" {
		t.Errorf("the second mailbox's row carries subject %q; want the message its own mailbox delivered", own.Subject)
	}
	// Its key is the seat's own, and the Message-ID behind it is still the
	// shared one — which is what lets a reply thread onto it.
	if own.SourceID == key {
		t.Errorf("the second mailbox took the shared natural key; it may keep its message, never the incumbent's key")
	}
	if got := connector.SharedMailKey(own.SourceID); got != key {
		t.Errorf("the shared key behind %q is %q, want %q", own.SourceID, got, key)
	}

	// And the incumbent is untouched. The arriving seat gets its own message
	// assembled from its own payload and takes nothing from the row that held
	// the key — which is what makes this safe where widening the proof is not.
	if incumbent.Subject != "unrelated" || incumbent.SourceID != key {
		t.Errorf("the incumbent was rewritten: subject %q, key %q", incumbent.Subject, incumbent.SourceID)
	}
}

// The SECOND denial path, one step earlier than the proof. When the row holding
// the key is not even discoverable to the arriving seat, EnsureActivityVisible
// refuses inside the upsert and replayClaimIsProvenTx is never reached from
// there — so the seat used to lose the message without any arm of the proof
// having answered. It keeps its own message here for the same reason.
func TestAMailboxDeniedByAnIncumbentItCannotSeeStillKeepsItsMessage(t *testing.T) {
	env := newCaptureEnv(t)
	e, sync := env.e, env.sync
	syncSecond := secondMailbox(t, e, e.Rep3)

	const key = "contested-invisible@acme.example"
	sync(t, capturedMail("anna@acme.example", captureOwner, key,
		"unrelated", "a note to the first mailbox alone"))

	// Capture-private to the FIRST seat, which takes the activity out of the
	// second seat's discover scope through the link walk — the one state that
	// hides a row from a colleague entirely.
	hidden := e.SeedID(t, `INSERT INTO contact (id, full_name, owner_id, visibility, source, captured_by)
		VALUES ($1, 'Private Counterparty', $2, 'owner', 'manual', 'human:x')`, e.Rep1)
	incumbent := capturedMailRows(t, e)[0].ID
	if _, err := e.Owner.Exec(context.Background(), `
		INSERT INTO activity_link (activity_id, entity_type, contact_id)
		VALUES ($1, 'contact', $2)`, incumbent, hidden); err != nil {
		t.Fatalf("hiding the incumbent from the second seat: %v", err)
	}

	syncSecond(t, capturedMail("jonas@acme.example", secondSeatAddress, key,
		"Telematics renewal", "please send the signed contract"))

	rows := capturedMailRows(t, e)
	if len(rows) != 2 {
		t.Fatalf("want both mailboxes' messages captured, got %d row(s): %+v — a seat's own mail "+
			"was denied by a row it cannot even see", len(rows), rows)
	}
	for _, row := range rows {
		if row.ID == incumbent {
			continue
		}
		if row.Subject != "Telematics renewal" {
			t.Errorf("the second mailbox's row carries subject %q; want the message it holds", row.Subject)
		}
		if got := connector.SharedMailKey(row.SourceID); got != key {
			t.Errorf("the shared key behind %q is %q, want %q", row.SourceID, got, key)
		}
	}
}

// The bytes each row names must be its OWN. storeRawCapture runs under the
// SHARED key before the collision is known, and its conflict path answers with
// the row the first delivery stored — so a scoped activity that kept that answer
// would name the incumbent's original. Two things would then be wrong at once:
// the participant replay joins the stored link and would derive this seat's
// parties from somebody else's message, and the redaction purge deletes by that
// same link and would destroy the other mailbox's bytes.
func TestTheSeatsOwnRowNamesItsOwnProviderOriginal(t *testing.T) {
	env := newCaptureEnv(t)
	e, sync := env.e, env.sync
	syncSecond := secondMailbox(t, e, e.Rep3)

	const key = "contested-bytes@acme.example"
	sync(t, capturedMail("anna@acme.example", captureOwner, key,
		"unrelated", "a note to the first mailbox alone"))
	syncSecond(t, capturedMail("jonas@acme.example", secondSeatAddress, key,
		"Telematics renewal", "please send the signed contract"))

	originals := map[ids.UUID]ids.UUID{}
	for _, row := range capturedMailRows(t, e) {
		originals[row.ID] = rawOriginalOf(t, e, row.ID)
	}
	if len(originals) != 2 {
		t.Fatalf("want a row per mailbox, got %d", len(originals))
	}
	var seen []ids.UUID
	for id, raw := range originals {
		if raw == ids.Nil {
			t.Fatalf("activity %s names no provider original; this suite compares the two", id)
		}
		seen = append(seen, raw)
	}
	if seen[0] == seen[1] {
		t.Errorf("both rows name one provider original (%s) — the second mailbox's row points at "+
			"the first's bytes, so a replay reads the wrong message and a purge destroys theirs", seen[0])
	}
}

// rawOriginalOf answers the provider original one activity names.
func rawOriginalOf(t *testing.T, e *integration.SearchEnv, activityID ids.UUID) ids.UUID {
	t.Helper()
	var raw *ids.UUID
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT raw_capture_id FROM activity WHERE id = $1`, activityID).Scan(&raw)
	}); err != nil {
		t.Fatalf("reading the activity's provider original: %v", err)
	}
	if raw == nil {
		return ids.Nil
	}
	return *raw
}

// The seat's own key is deterministic, so its next sync of the same message is
// the ordinary replay it always was rather than a second row every pass.
func TestTheSeatsOwnCopyIsStillAReplayOnTheNextSync(t *testing.T) {
	env := newCaptureEnv(t)
	e, sync := env.e, env.sync
	syncSecond := secondMailbox(t, e, e.Rep3)

	const key = "contested-twice@acme.example"
	sync(t, capturedMail("anna@acme.example", captureOwner, key,
		"unrelated", "a note to the first mailbox alone"))
	genuine := capturedMail("jonas@acme.example", secondSeatAddress, key,
		"Telematics renewal", "please send the signed contract")
	syncSecond(t, genuine)
	syncSecond(t, genuine)

	if rows := capturedMailRows(t, e); len(rows) != 2 {
		t.Fatalf("want two rows after the second mailbox synced twice, got %d: %+v", len(rows), rows)
	}
}
