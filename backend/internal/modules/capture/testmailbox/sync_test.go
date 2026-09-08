// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package testmailbox

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

type fakeSink struct{ upserted []connector.NormalizedRecord }

func (f *fakeSink) Upsert(_ context.Context, rec connector.NormalizedRecord) (datasource.EntityRef, error) {
	f.upserted = append(f.upserted, rec)
	return datasource.EntityRef{}, nil
}

func TestSyncEchoesEveryUnechoedSendAndMarksItEchoed(t *testing.T) {
	userID := ids.NewV7()
	msgID := ids.NewV7()
	ledger := &fakeLedger{unechoed: []capture.SentMessage{
		{ID: msgID, MessageID: "sent-1@test.example", To: []string{"buyer@example.com"}, Subject: "Hello"},
	}}
	sink := &fakeSink{}
	c := New(ledger)

	_, err := c.Sync(context.Background(), testAuth(t, userID), nil, sink)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}

	if len(sink.upserted) != 1 {
		t.Fatalf("sink got %d upserts, want 1", len(sink.upserted))
	}
	rec := sink.upserted[0]
	if rec.NaturalKey.SourceSystem != connector.EmailSourceSystem {
		t.Errorf("NaturalKey.SourceSystem = %q, want %q — this is what makes the echo reconcile against the outbound activity instead of duplicating it", rec.NaturalKey.SourceSystem, connector.EmailSourceSystem)
	}
	if rec.NaturalKey.SourceID != "sent-1@test.example" {
		t.Errorf("NaturalKey.SourceID = %q, want the same message_id the send used", rec.NaturalKey.SourceID)
	}
	// WithOwnerAttestation may be minted only by capture/mailmap
	// (TestOnlyTheMailMapperMintsTheOutboundAttestation) — the echo must
	// never claim it, however genuinely sent the message was.
	if rec.Counterparty.SentByOwner() {
		t.Error("Counterparty.SentByOwner() = true, want false — test_mailbox must never mint the T1 attestation itself")
	}
	if len(ledger.marked) != 1 || ledger.marked[0].id != msgID || ledger.marked[0].userID != userID.String() {
		t.Errorf("MarkEchoed called with %+v, want one entry for (%v, %v)", ledger.marked, userID, msgID)
	}
}

func TestSyncWithNothingUnechoedUpsertsNothing(t *testing.T) {
	ledger := &fakeLedger{}
	sink := &fakeSink{}
	c := New(ledger)
	if _, err := c.Sync(context.Background(), testAuth(t, ids.NewV7()), nil, sink); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if len(sink.upserted) != 0 {
		t.Errorf("sink got %d upserts, want 0", len(sink.upserted))
	}
}

func TestNormalizeIsTheInverseOfWhatSyncWrote(t *testing.T) {
	original := echoedRecord{MessageID: "round-trip@test.example", To: []string{"buyer@example.com"}, Subject: "Hi"}.record()
	got, err := New(&fakeLedger{}).Normalize(context.Background(), connector.RawRecord(original.Raw))
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if len(got) != 1 || got[0].NaturalKey.SourceID != "round-trip@test.example" {
		t.Errorf("Normalize(Raw) = %+v, want one record keyed on round-trip@test.example", got)
	}
}
