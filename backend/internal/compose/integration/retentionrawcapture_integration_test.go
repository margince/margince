// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The retention sweep's activity/erase destroys the message, not one copy of it.
//
// Clearing activity.body while raw_capture kept the verbatim provider payload
// erased nothing: the two are correlated by activity.raw_capture_id, which the
// erase deliberately preserves so the record of the message survives, and
// privacy/sar.go exports raw_capture by email match. An Art. 15 package
// therefore handed back the full original of a message whose retention window
// had closed years earlier.
//
// The reference, not a shared key: a mail capture's raw_capture row happens to
// carry the activity's own (source_system, source_id), but a channel poll's
// raw_capture row is keyed on its own redelivery counter instead — so a purge
// correlating by that pair destroyed every mail original and silently left
// every channel one standing, joined to nothing the erase could find.
//
// This is the ACTIVITY-driven path, and it stays the only one that answers an
// erase of the record: the `raw_capture` scope beside it ages an original on its
// own clock, which is a different question from destroying the message. Art. 17
// erasure is the third, and it is scoped to a CONTACT.
//
// Driven through compose.NewRetentionServiceFor rather than a service this test
// wires itself, because the defect was a MISSING seam — a test that supplies
// its own wiring proves nothing about the one the worker runs.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/capture/telegram"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

func TestTheRetentionSweepDestroysTheProviderOriginalToo(t *testing.T) {
	e := Setup(t)
	const sourceSystem, sourceID = connector.EmailSourceSystem, "msg-aged-out"

	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `
			INSERT INTO retention_policy (object_type, category, retain_days, action)
			VALUES ('activity', NULL, 100, 'erase')`)
		return err
	}); err != nil {
		t.Fatalf("seeding the retention policy: %v", err)
	}

	// The activity and its original, seeded through the Sink every mail
	// connector shares — linked by raw_capture_id rather than by the natural
	// key an erase deliberately keeps.
	activity := seedMailOriginal(t, e, sourceID, time.Now().Add(-400*24*time.Hour))

	// Provenance for the body about to be erased: without the delete it
	// goes on naming who captured the text and from where, and it is
	// SAR-exported.
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `
			INSERT INTO field_provenance (object_type, object_id, field_name, source, captured_by)
			VALUES ('activity', $1, 'body', 'capture', 'connector:test')`, activity)
		return err
	}); err != nil {
		t.Fatalf("seeding the aged-out message: %v", err)
	}

	svc := compose.NewRetentionServiceFor(e.DB(), nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := svc.EvaluateInstallation(RetentionPassCtx(e.WS)); err != nil {
		t.Fatalf("running the retention sweep: %v", err)
	}

	var body *string
	var keptKey bool
	var originals, provenance int
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		ctx := context.Background()
		if err := tx.QueryRow(ctx, `
			SELECT body, source_system IS NOT DISTINCT FROM $2 AND source_id IS NOT DISTINCT FROM $3
			  FROM activity WHERE id = $1`,
			activity, sourceSystem, sourceID).Scan(&body, &keptKey); err != nil {
			return err
		}
		// Counted by the seeded key rather than through the join to activity.
		// The join reads zero for two different reasons — the original is gone,
		// or the erase stopped keeping the pair — and only one of those is this
		// test passing. Which pair the activity still carries is asserted
		// separately, above.
		if err := tx.QueryRow(ctx, `
			SELECT count(*) FROM raw_capture WHERE source_system = $1 AND source_id = $2`,
			sourceSystem, sourceID).Scan(&originals); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `
			SELECT count(*) FROM field_provenance
			 WHERE object_type = 'activity' AND object_id = $1`, activity).Scan(&provenance)
	}); err != nil {
		t.Fatalf("reading the record after the sweep: %v", err)
	}

	if body != nil {
		t.Errorf("activity.body = %q after an erase policy fired, want cleared", *body)
	}
	if !keptKey {
		t.Error("the erase dropped the activity's (source_system, source_id) — the record of the message " +
			"is supposed to survive its content, and the purge below is keyed on that pair")
	}
	if originals != 0 {
		t.Errorf("%d raw_capture original(s) survived the erasure of the activity they duplicate — "+
			"the content is one join away, and SAR exports raw_capture by email match", originals)
	}
	if provenance != 0 {
		t.Errorf("%d field_provenance row(s) survived, still naming who captured the erased body "+
			"and from where", provenance)
	}
}

// TestTheSweepDestroysAChannelOriginalToo is the sibling defect the mail-keyed
// join above could never surface: a channel poll's raw_capture row never
// shares the activity's natural key, so a purge correlating on that pair
// leaves it standing.
func TestTheSweepDestroysAChannelOriginalToo(t *testing.T) {
	e := Setup(t)
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `
			INSERT INTO retention_policy (object_type, category, retain_days, action)
			VALUES ('activity', NULL, 100, 'erase')`)
		return err
	}); err != nil {
		t.Fatalf("seeding the retention policy: %v", err)
	}

	seedTelegramOriginal(t, e, 5150, time.Now().Add(-400*24*time.Hour))

	svc := compose.NewRetentionServiceFor(e.DB(), nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := svc.EvaluateInstallation(RetentionPassCtx(e.WS)); err != nil {
		t.Fatalf("running the retention sweep: %v", err)
	}

	var survivors int
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT count(*) FROM raw_capture WHERE source_system = $1`, capture.ProviderTelegram).Scan(&survivors)
	}); err != nil {
		t.Fatalf("counting the originals: %v", err)
	}
	if survivors != 0 {
		t.Fatalf("%d channel original(s) survived the sweep that destroyed the message they hold", survivors)
	}
}

// TestAPassMissingItsPurgerRefusesBeforeDestroyingAnything holds the half of the
// fix a passing sweep cannot show. The constructor is what stops a purger-less
// service being built; this is what happens if one is anyway, and the two things
// it asserts are the ones that make the refusal safe rather than merely loud.
//
// It refuses with the sentinel — not some other failure this test would have
// accepted as proof.
//
// And it refuses having destroyed NOTHING. A destructive pass that discovered a
// missing dependency partway would abort with earlier records already erased and
// every later policy unrun, nightly: the failure mode retentionActions' own
// comment is written to avoid. So the aged-out activity must still be intact
// after the refusal, which is only true while the check precedes the work.
func TestAPassMissingItsPurgerRefusesBeforeDestroyingAnything(t *testing.T) {
	e := Setup(t)
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `
			INSERT INTO retention_policy (object_type, category, retain_days, action)
			VALUES ('activity', NULL, 100, 'erase')`)
		return err
	}); err != nil {
		t.Fatalf("seeding the retention policy: %v", err)
	}
	seedMailOriginal(t, e, "msg-purgerless", time.Now().Add(-400*24*time.Hour))

	// nil, spelled out: the argument the constructor now demands, supplied as
	// the value compose never passes.
	bare := privacy.NewRetentionService(e.DB(), nil, slog.New(slog.NewTextHandler(io.Discard, nil)), nil)
	err := bare.EvaluateInstallation(RetentionPassCtx(e.WS))
	if !errors.Is(err, privacy.ErrRetentionSeamMissing) {
		t.Fatalf("a pass with no raw-capture purger returned %v, want ErrRetentionSeamMissing — "+
			"it must refuse, because the provider original has no other way to age out", err)
	}

	var body *string
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT body FROM activity WHERE subject = 'Quarterly figures'`).Scan(&body)
	}); err != nil {
		t.Fatalf("reading the record after the refusal: %v", err)
	}
	if body == nil || *body != "the numbers themselves" {
		t.Error("the refused pass had already erased the activity's body — a pass that cannot finish " +
			"must destroy nothing, or it leaves the installation half-swept with no record of where it stopped")
	}
}

// seedMailOriginal captures one mail-shaped record through the ONE guarded
// Sink every mail connector shares (capture.Sink.Upsert), rather than a literal
// INSERT that could not reproduce the invariant these tests hold the purge to:
// the activity's raw_capture_id is whatever the Sink itself settled on.
//
// The Counterparty is left empty on purpose: the ladder that decides whether to
// auto-create a contact then names nobody and decides nothing, which is what
// lets a bare Sink run here with no counterparty ensurer wired — these tests
// are about the purge, not about who a message is with.
func seedMailOriginal(t *testing.T, e *Env, sourceID string, occurredAt time.Time) ids.UUID {
	t.Helper()
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	ctx = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalConnector, ID: "connector:test",
		Permissions: principal.Permissions{
			Objects:  map[string]principal.ObjectGrant{"activity": {Create: true}},
			RowScope: principal.RowScopeAll,
		},
	})

	ref, err := capture.NewSink(e.DB()).Upsert(ctx, connector.NormalizedRecord{
		EntityType: "activity",
		NaturalKey: connector.NaturalKey{SourceSystem: connector.EmailSourceSystem, SourceID: sourceID},
		Fields: capture.ActivityFields{
			Kind: "email", Subject: "Quarterly figures", Body: "the numbers themselves",
			OccurredAt: occurredAt, Direction: connector.DirectionInbound,
		},
		Source:     "imap:" + sourceID,
		CapturedBy: "connector:test",
		Raw:        []byte(`{"subject":"Quarterly figures","body":"the numbers themselves"}`),
	})
	if err != nil {
		t.Fatalf("seeding the captured mail: %v", err)
	}
	return ref.ID
}

// seedTelegramOriginal captures one channel message the way the real poll and
// ingest worker do: InsertRawCaptureTx stores the poll's own copy under the
// per-update redelivery key, and Normalize+Sink.Upsert then names that row by
// reference — never by the chat-and-message key the activity itself carries,
// which is the pair a redelivered poll can share across two different bots'
// conversations and a purge must therefore never key on.
//
// The channel ensurer is left unwired, like seedMailOriginal's counterparty:
// with none set, decideChannelCounterparty declines to create a contact
// instead of dereferencing one, so a bare Sink is enough here too.
func seedTelegramOriginal(t *testing.T, e *Env, updateID int64, occurredAt time.Time) ids.UUID {
	t.Helper()
	const botID = "bot-test"
	update := telegramUpdateJSON(updateID, 9002, 12, 9001, occurredAt, "the numbers themselves")

	var rawID ids.UUID
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		var err error
		rawID, err = capture.InsertRawCaptureTx(context.Background(), tx, capture.RawRecord{
			SourceSystem: capture.ProviderTelegram,
			SourceID:     fmt.Sprintf("update:%d", updateID),
			Payload:      update,
		})
		return err
	}); err != nil {
		t.Fatalf("seeding the channel poll's own original: %v", err)
	}

	env, err := telegram.BuildRawEnvelope(botID, update)
	if err != nil {
		t.Fatalf("building the normalize envelope: %v", err)
	}
	records, err := telegram.Normalize(context.Background(), env)
	if err != nil {
		t.Fatalf("normalizing the update: %v", err)
	}
	rec := records[0]
	rec.RawCaptureID = rawID
	fields, ok := rec.Fields.(telegram.ActivityFields)
	if !ok {
		t.Fatalf("normalized record carries %T, want telegram.ActivityFields", rec.Fields)
	}
	rec.Fields = capture.ActivityFields{
		Kind: fields.Kind, ChannelProvider: fields.ChannelProvider,
		Body: fields.Body, OccurredAt: fields.OccurredAt, Direction: fields.Direction,
	}

	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	ctx = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalConnector, ID: telegram.CapturedByTelegram,
		Permissions: principal.Permissions{
			Objects:  map[string]principal.ObjectGrant{"activity": {Create: true}, "contact": {Create: true}},
			RowScope: principal.RowScopeAll,
		},
	})
	ref, err := capture.NewSink(e.DB()).Upsert(ctx, rec)
	if err != nil {
		t.Fatalf("capturing the channel message: %v", err)
	}
	return ref.ID
}

// telegramUpdateJSON renders one Telegram private-chat message update, verbatim
// enough for telegram.BuildRawEnvelope and telegram.Normalize to read: the
// package's own decoder types are unexported, so a test outside it builds the
// bytes those functions accept rather than the struct they never expose.
func telegramUpdateJSON(updateID, chatID, messageID, senderID int64, occurredAt time.Time, text string) []byte {
	return []byte(fmt.Sprintf(
		`{"update_id":%d,"message":{"message_id":%d,"date":%d,"text":%q,`+
			`"chat":{"id":%d,"type":"private"},"from":{"id":%d,"username":"customer"}}}`,
		updateID, messageID, occurredAt.Unix(), text, chatID, senderID))
}
