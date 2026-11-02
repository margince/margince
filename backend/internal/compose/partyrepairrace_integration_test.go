// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

type partyCandidateBarrier struct {
	next   pgx.QueryTracer
	before func() error
	once   sync.Once
	err    error
	fired  bool
}

func (b *partyCandidateBarrier) TraceQueryStart(ctx context.Context, conn *pgx.Conn, query pgx.TraceQueryStartData) context.Context {
	if strings.HasPrefix(strings.TrimSpace(query.SQL), "SELECT DISTINCT lower(p.address)") {
		b.once.Do(func() {
			b.fired = true
			b.err = b.before()
		})
	}
	if b.next != nil {
		return b.next.TraceQueryStart(ctx, conn, query)
	}
	return ctx
}

func (b *partyCandidateBarrier) TraceQueryEnd(ctx context.Context, conn *pgx.Conn, query pgx.TraceQueryEndData) {
	if b.next != nil {
		b.next.TraceQueryEnd(ctx, conn, query)
	}
}

func TestARepairStartingDuringRecipientAdmissionKeepsItsDurableWork(t *testing.T) {
	e := integration.Setup(t)
	sink := newCaptureSink(e.Pool, CaptureConfig{})
	record := partiesMailRecord(t, e, partyMailProof{outbound: true, sentByOwner: true}, "Bob Buyer <bob@buyer.test>", "", "Could we discuss the product requirements?")
	ctx := threadConnectorCtx(e, e.AdminUser)
	ref, err := sink.WithEnsurer(nil, nil).Upsert(ctx, record)
	if err != nil {
		t.Fatal(err)
	}
	activity := ids.From[ids.ActivityKind](ref.ID)
	key := threadKeyOfMessage(t, e, record.NaturalKey.SourceID)
	config := e.Pool.Config()
	barrier := &partyCandidateBarrier{next: config.ConnConfig.Tracer, before: func() error {
		return database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
			return capture.QueueThreadRepairTx(e.Admin(), tx, key, true)
		})
	}}
	config.ConnConfig.Tracer = barrier
	pool, err := testdb.OwnPoolFromConfig(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := newCaptureSink(pool, CaptureConfig{}).AdmitActivityParties(ctx, activity); err != nil {
		t.Fatal(err)
	}
	if !barrier.fired || barrier.err != nil {
		t.Fatalf("concurrent repair: fired=%v error=%v", barrier.fired, barrier.err)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM contact_email WHERE lower(email)='bob@buyer.test'`); got != 0 {
		t.Fatalf("contacts created from unavailable evidence=%d", got)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM capture_party_source WHERE activity_id=$1 AND admission_pending`, activity); got != 1 {
		t.Fatalf("pending source work=%d", got)
	}
	finishUnavailableThread(t, e, ids.NewV7())
	for range 2 {
		if err := sink.RecoverPartyAdmissions(principal.SystemActing(e.Admin(), "system:test_party_recovery"), 200); err != nil {
			t.Fatal(err)
		}
	}
	if got := e.WsCount(t, `SELECT count(*) FROM contact_email WHERE lower(email)='bob@buyer.test'`); got != 1 {
		t.Fatalf("recipient contacts after repair and replay=%d", got)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM capture_party_source WHERE activity_id=$1 AND admission_pending`, activity); got != 0 {
		t.Fatalf("settled source work=%d", got)
	}
}
