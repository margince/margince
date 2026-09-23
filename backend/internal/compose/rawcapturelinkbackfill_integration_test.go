// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The pass that closes the window every activity captured before
// raw_capture_id existed left open: once the purge, the export and the
// retention selector follow the column alone, an unlinked original is
// invisible to all three.
//
// Both cases run the real ingest worker rather than hand-inserting an
// activity row: a fixture that typed the natural key itself would agree with
// whatever spelling it chose, not with what the two real writers actually
// produce — and this pass exists precisely because those two disagree.

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// TestTheBackfillNamesATelegramOriginalCapturedBeforeTheColumn proves the
// channel arm: over a message the real ingest worker captured, it recovers the
// bot id from the redelivery key the poll wrote and the chat/message from the
// stored update, and re-links what a null column left it unable to find.
func TestTheBackfillNamesATelegramOriginalCapturedBeforeTheColumn(t *testing.T) {
	e := integration.Setup(t)
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))

	connID, rawID := ids.NewV7(), ids.NewV7()
	e.WsExec(t, `
		INSERT INTO channel_connection
			(id, provider, channel_id, channel_label, credential_ref, status, connected_by)
		VALUES ($1, 'telegram', '42', 'acme_bot', 'cred-ref', 'connected', $2)`,
		connID, e.Rep1)
	e.WsExec(t, `
		INSERT INTO raw_capture (id, source_system, source_id, payload)
		VALUES ($1, 'telegram', $2, $3)`,
		rawID, telegramRawSourceID("42", 900), []byte(telegramIngestUpdateJSON))

	worker := newTelegramIngestWorker(e.Pool, CaptureConfig{}, quiet)
	if err := worker.Work(context.Background(), &river.Job[TelegramIngestArgs]{
		Args: TelegramIngestArgs{Workspace: e.WS, ConnectionID: connID.String(), BotID: "42", RawCaptureID: rawID.String()},
	}); err != nil {
		t.Fatalf("capturing the original: %v", err)
	}

	// The state every row captured before the column existed is in.
	e.WsExec(t, `UPDATE activity SET raw_capture_id = NULL WHERE source_system = 'telegram'`)

	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	linked, err := backfillRawCaptureLinksBatch(ctx, e.Pool, 100, quiet)
	if err != nil {
		t.Fatalf("backfilling: %v", err)
	}
	if linked != 1 {
		t.Fatalf("linked %d record(s), want 1", linked)
	}
	if n := e.WsCount(t, `
		SELECT count(*) FROM activity WHERE source_system = 'telegram' AND raw_capture_id = $1`, rawID); n != 1 {
		t.Errorf("%d activity rows point back at the original the poll wrote, want 1", n)
	}
}

// TestTheBackfillReachesPastTheRecordsThatWillNeverHaveAnOriginal is the case
// a batch limit taken on the wrong side of the join loses for good: an
// activity typed by hand has no original and stays NULL forever, so a window
// of candidates drawn before the join fills with those, links nothing, and the
// drain stops on an empty batch — leaving every higher id unlinked, and every
// unlinked original standing through the erasure of the message it holds.
//
// The limit is one here, and the hand-typed rows sort ahead of the capture,
// which is the shape that separates a limit on the join from a limit on the
// scan behind it.
func TestTheBackfillReachesPastTheRecordsThatWillNeverHaveAnOriginal(t *testing.T) {
	e := integration.Setup(t)
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))

	// Hand-typed notes, seeded first so their v7 ids sort below the capture's.
	for range 3 {
		e.WsExec(t, `
			INSERT INTO activity (id, kind, subject, occurred_at, source, captured_by)
			VALUES ($1, 'note', 'a note nobody captured', now(), 'manual', 'human:x')`, ids.NewV7())
	}

	connID, rawID := ids.NewV7(), ids.NewV7()
	e.WsExec(t, `
		INSERT INTO channel_connection
			(id, provider, channel_id, channel_label, credential_ref, status, connected_by)
		VALUES ($1, 'telegram', '42', 'acme_bot', 'cred-ref', 'connected', $2)`,
		connID, e.Rep1)
	e.WsExec(t, `
		INSERT INTO raw_capture (id, source_system, source_id, payload)
		VALUES ($1, 'telegram', $2, $3)`,
		rawID, telegramRawSourceID("42", 902), []byte(telegramIngestUpdateJSON))
	worker := newTelegramIngestWorker(e.Pool, CaptureConfig{}, quiet)
	if err := worker.Work(context.Background(), &river.Job[TelegramIngestArgs]{
		Args: TelegramIngestArgs{Workspace: e.WS, ConnectionID: connID.String(), BotID: "42", RawCaptureID: rawID.String()},
	}); err != nil {
		t.Fatalf("capturing the original: %v", err)
	}
	e.WsExec(t, `UPDATE activity SET raw_capture_id = NULL WHERE source_system = 'telegram'`)

	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	linked, err := backfillRawCaptureLinksBatch(ctx, e.Pool, 1, quiet)
	if err != nil {
		t.Fatalf("backfilling: %v", err)
	}
	if linked != 1 {
		t.Fatalf("linked %d record(s) behind three that can never be linked, want 1 — a batch that "+
			"spends its limit on rows with no original never reaches the ones that have one", linked)
	}
}

// TestTheBackfillCountsWhatItCannotPlace proves a my_chat_member update — which
// telegram.ParseMembership consumes before Normalize ever runs, so it becomes
// no activity at all — is counted as unmatched rather than guessed at, and
// links nothing in its place.
func TestTheBackfillCountsWhatItCannotPlace(t *testing.T) {
	e := integration.Setup(t)
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))

	rawID := ids.NewV7()
	e.WsExec(t, `
		INSERT INTO raw_capture (id, source_system, source_id, payload)
		VALUES ($1, 'telegram', $2, $3)`,
		rawID, telegramRawSourceID("42", 901), []byte(telegramIngestKickedJSON))

	worker := newTelegramIngestWorker(e.Pool, CaptureConfig{}, quiet)
	if err := worker.Work(context.Background(), &river.Job[TelegramIngestArgs]{
		Args: TelegramIngestArgs{Workspace: e.WS, ConnectionID: "unused", BotID: "42", RawCaptureID: rawID.String()},
	}); err != nil {
		t.Fatalf("applying the membership update: %v", err)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM activity WHERE source_system = 'telegram'`); n != 0 {
		t.Fatalf("%d activity rows after a my_chat_member update, want 0 — it is a reachability signal, never a message", n)
	}

	// A JSON handler over a buffer, decoded below: unmatched is reported
	// through the log rather than a second return value, and a substring
	// check could not tell a real count from a field that stopped being one.
	var logged bytes.Buffer
	reporter := slog.New(slog.NewJSONHandler(&logged, nil))
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	linked, err := backfillRawCaptureLinksBatch(ctx, e.Pool, 100, reporter)
	if err != nil {
		t.Fatalf("backfilling: %v", err)
	}
	if linked != 0 {
		t.Fatalf("linked %d record(s) for an original that names none, want 0", linked)
	}

	line := strings.TrimSpace(logged.String())
	if line == "" {
		t.Fatal("an unplaceable original was silently dropped rather than counted")
	}
	var record map[string]any
	if decodeErr := json.Unmarshal([]byte(line), &record); decodeErr != nil {
		t.Fatalf("the report line is not one JSON record: %v in %q", decodeErr, line)
	}
	if got, ok := record["unmatched"].(float64); !ok || got != 1 {
		t.Fatalf("record[\"unmatched\"] = %v, want 1 — a membership update becomes no record and must be counted, never guessed at", record["unmatched"])
	}
}
