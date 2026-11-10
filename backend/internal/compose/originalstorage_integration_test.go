// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/originalstore"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

type originalStorageRow struct {
	id      ids.UUID
	inline  []byte
	object  *originalstore.Reference
	checked bool
}

func originalRow(ctx context.Context, t *testing.T, db *database.DB, system, source string) originalStorageRow {
	t.Helper()
	var row originalStorageRow
	var object []byte
	if err := db.Tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT id, payload, payload_object, payload_offload_checked_at IS NOT NULL
 FROM raw_capture WHERE source_system = @system AND source_id = @source`,
			pgx.NamedArgs{"system": system, "source": source}).Scan(&row.id, &row.inline, &object, &row.checked)
	}); err != nil {
		t.Fatal(err)
	}
	if len(object) > 0 {
		if err := json.Unmarshal(object, &row.object); err != nil {
			t.Fatal(err)
		}
	}
	return row
}

func readOriginalBytes(ctx context.Context, t *testing.T, db *database.DB, id ids.UUID) []byte {
	t.Helper()
	var stored []byte
	if err := db.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		stored, err = capture.GetRawCapturePayloadTx(ctx, tx, id)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	original, err := capture.DecodeStoredOriginal(stored)
	if err != nil {
		t.Fatal(err)
	}
	return original
}

func TestLargeCapturedOriginalsStayAppendOnceAndReplayExactly(t *testing.T) {
	e := integration.Setup(t)
	blob := blobstore.NewMemory()
	ctx := originalstore.Bind(asConnectorFor(e.Admin(), "connector:imap", e.AdminUser), e.DB(), blob)
	record := mailRecord("large-original")
	record.Raw = []byte("From: her@example.com\r\n\r\n" + strings.Repeat("original text ", originalstore.InlineLimit/10))
	want := bytes.Clone(record.Raw)
	sink := capture.NewSink(e.DB())
	if _, err := sink.Upsert(ctx, record); err != nil {
		t.Fatal(err)
	}
	row := originalRow(ctx, t, e.DB(), connector.EmailSourceSystem, record.NaturalKey.SourceID)
	if row.object == nil || !row.checked || len(row.inline) > 64 || row.object.Bytes <= originalstore.InlineLimit {
		t.Fatalf("large original is not capped: has_reference=%v checked=%v inline_bytes=%d", row.object != nil, row.checked, len(row.inline))
	}
	if got := readOriginalBytes(ctx, t, e.DB(), row.id); !bytes.Equal(got, want) {
		t.Fatal("replay changed the original's bytes")
	}
	record.Raw = []byte(strings.Repeat("different delivery ", originalstore.InlineLimit/10))
	if _, err := sink.Upsert(ctx, record); err != nil {
		t.Fatal(err)
	}
	after := originalRow(ctx, t, e.DB(), connector.EmailSourceSystem, record.NaturalKey.SourceID)
	if after.id != row.id || after.object == nil || after.object.Key != row.object.Key {
		t.Fatal("a duplicate delivery replaced the first evidence")
	}
	if got := readOriginalBytes(ctx, t, e.DB(), row.id); !bytes.Equal(got, want) {
		t.Fatal("a duplicate delivery changed the evidence")
	}
}

func TestRawRedeliveryRetiresItsOldObjectAndRollbackKeepsAnIntent(t *testing.T) {
	e := integration.Setup(t)
	ctx := originalstore.Bind(e.Admin(), e.DB(), blobstore.NewMemory())
	payload := []byte(`{"text":"` + strings.Repeat("a", originalstore.InlineLimit) + `"}`)
	record := capture.RawRecord{SourceSystem: "telegram", SourceID: "update", Payload: payload}
	put := func() error {
		return e.DB().Tx(ctx, func(tx pgx.Tx) error {
			_, err := capture.InsertRawCaptureTx(ctx, tx, record)
			return err
		})
	}
	if err := put(); err != nil {
		t.Fatal(err)
	}
	first := originalRow(ctx, t, e.DB(), record.SourceSystem, record.SourceID)
	record.Payload = bytes.ReplaceAll(payload, []byte("a"), []byte("b"))
	if err := put(); err != nil {
		t.Fatal(err)
	}
	second := originalRow(ctx, t, e.DB(), record.SourceSystem, record.SourceID)
	if first.object == nil || second.object == nil || first.id != second.id || first.object.Key == second.object.Key {
		t.Fatal("redelivery did not replace the reference under the same record")
	}
	if got := readOriginalBytes(ctx, t, e.DB(), second.id); !bytes.Equal(got, record.Payload) {
		t.Fatal("redelivery replay returned the previous body")
	}
	var retired int
	if err := e.DB().Tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM stored_object_intent WHERE storage_key = @key AND kind = 'raw_original'`, pgx.NamedArgs{"key": first.object.Key}).Scan(&retired)
	}); err != nil {
		t.Fatal(err)
	}
	if retired != 1 {
		t.Fatal("the replaced object's cleanup intent was lost")
	}
	record.SourceID = "rolled-back-update"
	stop := errors.New("rollback the domain write")
	if err := e.DB().Tx(ctx, func(tx pgx.Tx) error {
		if _, err := capture.InsertRawCaptureTx(ctx, tx, record); err != nil {
			return err
		}
		return stop
	}); !errors.Is(err, stop) {
		t.Fatalf("rollback returned %v", err)
	}
	var rows, intents int
	if err := e.DB().Tx(ctx, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM raw_capture WHERE source_system = @system AND source_id = @source`, pgx.NamedArgs{"system": record.SourceSystem, "source": record.SourceID}).Scan(&rows); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `SELECT count(*) FROM stored_object_intent WHERE kind = 'raw_original'`).Scan(&intents)
	}); err != nil {
		t.Fatal(err)
	}
	if rows != 0 || intents != 2 {
		t.Fatalf("rollback lost its provisional object: rows=%d intents=%d", rows, intents)
	}
}

func TestTheOriginalSweepResumesAndChecksSmallLegacyRowsOnce(t *testing.T) {
	e := integration.Setup(t)
	ctx := principal.SystemActing(e.Admin(), "original_offload_fixture")
	blob := blobstore.NewMemory()
	ctx = originalstore.Bind(ctx, e.DB(), blob)
	large := []byte(`{"text":"` + strings.Repeat("a", originalstore.InlineLimit) + `"}`)
	// Legacy inline rows predate the cap; current capture cannot produce them.
	for source, payload := range map[string][]byte{"small-a": []byte(`{}`), "small-b": []byte(`[]`), "large": large} {
		if err := e.DB().Tx(ctx, func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, `INSERT INTO raw_capture (source_system, source_id, payload)
 VALUES ('fixture', @source, @payload::jsonb)`, pgx.NamedArgs{"source": source, "payload": payload})
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
	before := originalRow(ctx, t, e.DB(), "fixture", "large")
	store := capture.NewPartSlimStore(e.DB(), blob)
	first, movedFirst, err := store.OffloadBatch(ctx, 2)
	if err != nil {
		t.Fatal(err)
	}
	second, movedSecond, err := store.OffloadBatch(ctx, 2)
	if err != nil {
		t.Fatal(err)
	}
	last, movedLast, err := store.OffloadBatch(ctx, 2)
	if err != nil {
		t.Fatal(err)
	}
	if first != 2 || second != 1 || last != 0 || movedFirst+movedSecond != 1 || movedLast != 0 {
		t.Fatalf("sweep did not resume at its bound: %d/%d/%d considered, %d/%d/%d moved", first, second, last, movedFirst, movedSecond, movedLast)
	}
	row := originalRow(ctx, t, e.DB(), "fixture", "large")
	if row.object == nil || len(row.inline) > 64 || !row.checked {
		t.Fatal("legacy large original was not capped")
	}
	if got := readOriginalBytes(ctx, t, e.DB(), row.id); !bytes.Equal(got, before.inline) {
		t.Fatal("offloading changed the legacy evidence")
	}
}

func TestOversizedOriginalsKeepTheDatabaseAdmissionRules(t *testing.T) {
	e := integration.Setup(t)
	blob := &failingOriginalBlob{Store: blobstore.NewMemory()}
	ctx := originalstore.Bind(asConnectorFor(e.Admin(), "connector:imap", e.AdminUser), e.DB(), blob)
	for _, tail := range []string{`"bad":"\u0000"}`, `"bad":1e999999}`} {
		payload := []byte(`{"padding":"` + strings.Repeat("x", originalstore.InlineLimit) + `",` + tail)
		if !json.Valid(payload) {
			t.Fatal("fixture must reach PostgreSQL admission")
		}
		if _, _, err := originalstore.Prepare(ctx, payload); err == nil {
			t.Fatal("offloading bypassed jsonb admission")
		}
	}
	if n := e.WsCount(t, `SELECT count(*) FROM stored_object_intent WHERE kind='raw_original'`); n != 0 {
		t.Fatal("invalid evidence staged an object")
	}
	record := mailRecord("unavailable-original")
	record.Raw = []byte("From: sender@example.test\r\n\r\n" + strings.Repeat("x", originalstore.InlineLimit))
	if _, err := capture.NewSink(e.DB()).Upsert(asConnectorFor(e.Admin(), "connector:imap", e.AdminUser), record); err == nil {
		t.Fatal("oversized evidence silently used inline storage without a store")
	}
	blob.failPut = true
	if _, err := capture.NewSink(e.DB()).Upsert(ctx, record); err == nil {
		t.Fatal("failed object storage admitted capture")
	}
	if n := e.WsCount(t, `SELECT count(*) FROM raw_capture WHERE source_id=$1`, record.NaturalKey.SourceID); n != 0 {
		t.Fatal("failed storage left a domain original")
	}
	if n := e.WsCount(t, `SELECT count(*) FROM activity WHERE source_id=$1`, record.NaturalKey.SourceID); n != 0 {
		t.Fatal("failed evidence storage committed its activity")
	}
	if n := e.WsCount(t, `SELECT count(*) FROM stored_object_intent WHERE kind='raw_original'`); n != 1 {
		t.Fatal("failed upload lost its provisional cleanup intent")
	}
}
