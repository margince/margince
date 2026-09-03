// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A card attached to captured mail, against a real database.
//
// Two things are worth a test here and neither is visible to a unit lane. The
// TRIGGER must produce a job — its insert options are read from the job contract
// at runtime, so a kind declared one way and inserted another compiles fine and
// panics on the first mailed card, inside a subscriber goroutine, taking the
// worker with it. And the liveness refusal must have an ADMIT arm: a deny-only
// test passes just as happily against a path that refuses everything, which is
// exactly what that panic produced.

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// quietIngestLog keeps the suite's output about the assertions rather than the
// worker's running commentary.
func quietIngestLog() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// seedCardMail writes one captured inbound email carrying a .vcf attachment,
// the way a mailbox connector writes it, and answers the activity id.
//
// The attachment is stamped text/plain on purpose. Gmail delivers a
// hand-attached card that way, so a fixture using text/vcard would describe a
// message the probe finds for a reason the real one does not.
func seedCardMail(ctx context.Context, t *testing.T, e *integration.Env, grantor ids.UUID, audience string) ids.UUID {
	t.Helper()
	activity := ids.NewV7()
	attachment := ids.NewV7()
	capturedBy := "connector:gmail:" + grantor.String()
	err := pgx.BeginFunc(ctx, e.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
			INSERT INTO capture_connection
			  (id, provider, user_id, scopes, status, sync_cursor, credential_ref,
			   account_label, share_acknowledged_at, mail_posture)
			VALUES ($1, 'gmail', $2, '{read}', 'connected', '{}', 'ref', 'rep@demo.test', now(), 'shared')
			ON CONFLICT DO NOTHING`, ids.NewV7(), grantor); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO activity (id, kind, subject, body, direction, occurred_at,
			                      source_system, source_id, source, captured_by, audience)
			VALUES ($1, 'email', 'my card', 'attached', 'inbound', now(),
			        'gmail', $2, 'gmail:test', $3, $4)`,
			activity, activity.String(), capturedBy, audience); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO attachment (id, entity_type, entity_id, activity_id, filename,
			                        content_type, byte_size, storage_key, checksum,
			                        source, captured_by, category)
			VALUES ($1, 'activity', $2, $2, 'card.vcf', 'text/plain', 120,
			        $3, 'seed', 'gmail', $4, 'email_attachment')`,
			attachment, activity, "seed/attachment/"+attachment.String(), capturedBy)
		return err
	})
	if err != nil {
		t.Fatalf("seeding a captured message with a card: %v", err)
	}
	return activity
}

// capturedEnvelope is the event the capture write shape publishes.
func capturedEnvelope(t *testing.T, activity ids.UUID) events.Envelope {
	t.Helper()
	payload, err := json.Marshal(map[string]string{"kind": "email", "source_system": "gmail"})
	if err != nil {
		t.Fatalf("building the capture payload: %v", err)
	}
	return events.Envelope{
		EventID:    ids.NewV7(),
		Type:       "activity.captured",
		Version:    1,
		OccurredAt: time.Now(),
		Entity:     events.EntityRef{Type: "activity", ID: activity},
		Payload:    payload,
	}
}

// The test that catches the whole class: the trigger's insert options are
// resolved against the job contract at RUNTIME, so a kind whose declaration and
// whose insert disagree compiles and then panics on the first mailed card.
// Nothing short of actually inserting the job says so.
func TestAMailedCardQueuesItsImport(t *testing.T) {
	e := integration.Setup(t)
	// The queue's own tables: this test asserts on a row River writes, so the
	// schema it writes into has to exist.
	integration.ApplyRiverSchema(t)
	ctx := e.Admin()

	activity := seedCardMail(ctx, t, e, e.Rep1, "workspace")

	inserter, err := jobs.NewInserter(e.Pool, quietIngestLog())
	if err != nil {
		t.Fatalf("building the insert-only runner: %v", err)
	}
	trigger := NewVCardIngestTrigger(e.Pool, inserter, quietIngestLog())
	if err := trigger.HandleEvent(ctx, capturedEnvelope(t, activity)); err != nil {
		t.Fatalf("handling the capture event: %v", err)
	}

	var queued, maxAttempts int
	if err := e.Pool.QueryRow(ctx, `
		SELECT count(*), coalesce(max(max_attempts), 0) FROM river_job
		 WHERE kind = 'vcard_ingest' AND args->>'activity_id' = $1`,
		activity.String()).Scan(&queued, &maxAttempts); err != nil {
		t.Fatalf("counting the queued imports: %v", err)
	}
	if queued != 1 {
		t.Fatalf("the trigger queued %d imports for a message carrying a card, want 1 — "+
			"a card that reaches the mailbox and no job is the feature doing nothing", queued)
	}
	// The bound River actually persisted, not the InsertOpts value this
	// package hands it — proof the cap survives insertion rather than being
	// silently dropped for the default.
	if maxAttempts != vcardIngestMaxAttempts {
		t.Errorf("river_job.max_attempts = %d, want the declared ladder %d", maxAttempts, vcardIngestMaxAttempts)
	}
}

// The narrowing refusal, with its ADMIT arm in the same test.
//
// Both arms, deliberately: a deny-only assertion here would pass against a path
// that refuses everything — which is exactly the state this feature shipped in
// before the trigger's insert was fixed. The two messages differ in nothing but
// the audience, so what the admit arm proves is that the audience is the reason.
func TestOnlyWorkspaceReadableMailLendsItsCard(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.Admin()

	narrowed := seedCardMail(ctx, t, e, e.Rep1, "participants")
	open := seedCardMail(ctx, t, e, e.Rep1, "workspace")

	worker := newVCardIngestWorker(e.Pool, blobstore.NewMemory(), quietIngestLog())

	// The refusal is not a fault: a narrowed source is an ANSWER, and returning
	// an error would have River retry into the same refusal until it exhausts.
	if err := worker.importCards(ctx, VCardIngestArgs{Workspace: e.WS, Activity: narrowed}); err != nil {
		t.Fatalf("importing from a narrowed message: %v", err)
	}
	if keys := cardKeysOf(ctx, t, worker, narrowed); len(keys) != 0 {
		t.Errorf("a message the workspace may not read lent %d card(s) to the import — "+
			"their contents would land on a record every seat can open, and narrowing "+
			"the mail afterwards does not take them back", len(keys))
	}

	// The admit arm. Same seed, same mailbox, same card; only the audience
	// differs, and this one must be readable — otherwise the assertion above is
	// satisfied by a path that reads nothing at all.
	if keys := cardKeysOf(ctx, t, worker, open); len(keys) != 1 {
		t.Errorf("an open message lent %d card(s), want 1 — the refusal above proves "+
			"nothing if the reader cannot read a message it is allowed to", len(keys))
	}
}

// cardKeysOf answers the attachments the import would read from one message.
func cardKeysOf(ctx context.Context, t *testing.T, w *vcardIngestWorker, activity ids.UUID) []string {
	t.Helper()
	keys, err := w.liveCardKeys(ctx, activity)
	if err != nil {
		t.Fatalf("listing a message's card attachments: %v", err)
	}
	return keys
}

// A mailbox whose owner turned off "read my mail for contact details" is not a
// mailbox this path may mine either. The switch is the signature pass's, and
// this writes people rather than filling a field, so it may not be the looser of
// the two.
//
// Both arms again, and here the admit arm is doing more work than it looks: the
// refusal answers "no live mailbox grants this import", which is the SAME
// sentence a missing connection, a wrong provenance string or a broken join
// produce. Without the ON case the assertion is satisfied by a query that
// resolves nothing at all, which is what a mutation of the switch clause turned
// it into.
func TestAMailedCardObeysTheMailboxsOwnSwitch(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.Admin()

	activity := seedCardMail(ctx, t, e, e.Rep1, "workspace")
	setMailboxSwitch(ctx, t, e, false)
	if _, err := grantorOf(ctx, e, activity); err == nil {
		t.Fatalf("a switched-off mailbox still granted the import — the owner's refusal " +
			"binds the signature pass and must bind the card path, which writes more")
	}

	// The same mailbox with the switch back ON must resolve, or the refusal
	// above is indistinguishable from a join that matches nothing.
	setMailboxSwitch(ctx, t, e, true)
	grantor, err := grantorOf(ctx, e, activity)
	if err != nil {
		t.Fatalf("a switched-on mailbox did not grant the import: %v — the refusal above "+
			"then proves nothing, because this query answers the same way for a mailbox "+
			"that is off and for one it cannot find at all", err)
	}
	if grantor != e.Rep1 {
		t.Errorf("the import would run as %s, want the mailbox's granting human %s", grantor, e.Rep1)
	}
}

// setMailboxSwitch turns "read my mail for contact details" on or off for the
// seeded mailbox.
func setMailboxSwitch(ctx context.Context, t *testing.T, e *integration.Env, on bool) {
	t.Helper()
	if _, err := e.Pool.Exec(ctx, `
		UPDATE capture_connection SET signature_enrich_enabled = $2
		 WHERE user_id = $1 AND provider = 'gmail'`, e.Rep1, on); err != nil {
		t.Fatalf("setting the mailbox switch to %v: %v", on, err)
	}
}

// grantorOf answers the human the import would run as, or the refusal.
func grantorOf(ctx context.Context, e *integration.Env, activity ids.UUID) (ids.UUID, error) {
	worker := newVCardIngestWorker(e.Pool, blobstore.NewMemory(), quietIngestLog())
	actorCtx, err := worker.asMailboxGrantor(ctx, VCardIngestArgs{Workspace: e.WS, Activity: activity})
	if err != nil {
		return ids.Nil, err
	}
	actor, ok := principal.Actor(actorCtx)
	if !ok {
		return ids.Nil, errNoActorBound
	}
	return actor.UserID, nil
}

// errNoActorBound names the one impossible case, so the helper never returns a
// zero id that reads as a real answer.
var errNoActorBound = errors.New("the import context carries no actor")
