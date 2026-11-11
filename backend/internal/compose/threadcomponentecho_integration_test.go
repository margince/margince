// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"regexp"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/multitracer"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture/mailmap"
	"github.com/margince/margince/backend/internal/modules/comms"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

type threadSQLBarrier struct {
	match   *regexp.Regexp
	once    sync.Once
	entered chan int
	release chan struct{}
}

var echoActivityInsert = regexp.MustCompile(`(?i)\bINSERT\s+INTO\s+"?activity"?\s*\(`)

func (b *threadSQLBarrier) TraceQueryStart(ctx context.Context, _ *pgx.Conn, q pgx.TraceQueryStartData) context.Context {
	if b.match.MatchString(q.SQL) {
		return context.WithValue(ctx, b, true)
	}
	return ctx
}

func (b *threadSQLBarrier) TraceQueryEnd(ctx context.Context, conn *pgx.Conn, q pgx.TraceQueryEndData) {
	marked, ok := ctx.Value(b).(bool)
	if !ok || !marked || q.Err != nil {
		return
	}
	b.once.Do(func() {
		b.entered <- int(conn.PgConn().PID())
		select {
		case <-b.release:
		case <-ctx.Done():
		}
	})
}

func TestReceiptReconciliationWaitsForAnUncommittedCapturedEcho(t *testing.T) {
	o := setupOwed(t)
	const staged, stamped, recipient = "staged-race@myco.example", "stamped-race@provider.example", "buyer@customer.example"
	actor := o.e.As(o.owner, []ids.UUID{o.e.Team1}, integration.AdminPerms)
	row, _, err := o.e.Activities.LogActivity(actor, activities.LogActivityInput{Kind: "email", Source: "manual", SourceSystem: new("email"), SourceID: new(staged), Direction: new("outbound"), ThreadKey: staged, CounterpartyEmail: recipient, CounterpartyOutboundAttested: true})
	if err != nil {
		t.Fatal(err)
	}
	store := comms.NewStore(o.e.DB(), func() time.Time { return o.now }, o.e.Activities)
	var delivery ids.UUID
	if err := database.WithWorkspaceTx(actor, o.e.Pool, func(tx pgx.Tx) error {
		var err error
		delivery, err = store.StageTx(actor, tx, comms.StageInput{ActivityID: ids.From[ids.ActivityKind](ids.UUID(row.Id)), Provider: "gmail", MessageID: staged, Recipients: []string{recipient}, ThreadKey: staged})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	barrier := &threadSQLBarrier{match: echoActivityInsert, entered: make(chan int, 1), release: make(chan struct{})}
	config := o.e.Pool.Config()
	config.ConnConfig.Tracer = multitracer.New(config.ConnConfig.Tracer, barrier)
	pool, err := testdb.OwnPoolFromConfig(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	raw := (owedMail{from: o.seat, to: recipient, subject: "Agreement", body: "The sent copy.", messageID: stamped, at: o.now}).raw()
	parsed, err := mailmap.Parse(raw, o.seat)
	if err != nil {
		t.Fatal(err)
	}
	parsed = parsed.AttestSentByOwner(true)
	ctx, cancel := context.WithCancel(threadConnectorCtx(o.e, o.owner))
	defer cancel()
	captured := make(chan error, 1)
	captureDone := make(chan struct{})
	record := parsed.ToRecord("gmail", raw)
	record.ProviderMessageID = "accepted-provider-id"
	go func() {
		defer close(captureDone)
		_, err := newCaptureSink(pool, CaptureConfig{}).Upsert(ctx, record)
		captured <- err
	}()
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(barrier.release) }) }
	t.Cleanup(func() { cancel(); release(); <-captureDone })
	var echoPID int
	select {
	case echoPID = <-barrier.entered:
	case err := <-captured:
		t.Fatalf("echo did not reach insert barrier: %v", err)
	}
	sent := make(chan error, 1)
	sendDone := make(chan struct{})
	worker := principal.SystemActing(o.e.Admin(), "system:comms-send")
	go func() {
		defer close(sendDone)
		sent <- store.RecordSent(worker, delivery, connector.SendReceipt{ProviderMessageID: "accepted-provider-id", RFC822MessageID: stamped})
	}()
	t.Cleanup(func() { release(); <-sendDone })
	if err := database.WithWorkspaceTx(actor, o.e.Pool, func(tx pgx.Tx) error { waitForBackendBlockedBy(t, tx, echoPID, sent); return nil }); err != nil {
		t.Fatal(err)
	}
	if state := o.e.WsCount(t, `SELECT count(*) FROM comms_outbound WHERE id=$1 AND status='sent' AND provider_message_id='accepted-provider-id'`, delivery); state != 1 {
		t.Fatal("receipt was not durable before reconciliation waited")
	}
	release()
	if err := <-captured; err != nil {
		t.Fatal(err)
	}
	if err := <-sent; err != nil {
		t.Fatal(err)
	}
	if live := o.e.WsCount(t, `SELECT count(*) FROM activity WHERE source_system='email' AND source_id=$1 AND archived_at IS NULL`, stamped); live != 1 {
		t.Fatalf("live stamped messages=%d, want one", live)
	}
	if failed := o.e.WsCount(t, `SELECT count(*) FROM system_log WHERE action='comms_identity_reconcile_failed'`); failed != 0 {
		t.Fatal("reconciliation degraded despite a completed echo")
	}
}
