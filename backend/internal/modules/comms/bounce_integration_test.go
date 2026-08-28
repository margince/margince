// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package comms

// What RecordBounce leaves behind, against a real database: the mark on the
// row, the audit row and the outbox event in one transaction, the first
// report winning a replay, and the answers for mail this installation never
// sent or never finished sending.

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// asCapturingConnector is who really reports a bounce: the mail connector
// that read the delivery report out of the owner's mailbox.
func (e *storeEnv) asCapturingConnector(owner ids.UUID) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.ws)
	ctx = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalConnector, ID: "connector:gmail",
		UserID: owner, OnBehalfOf: owner,
	})
	return principal.WithCorrelationID(ctx, ids.NewV7())
}

// bounceFor is the report a real DSN for this suite's staged mail yields.
func bounceFor(messageID string, kind connector.BounceKind, reason string) connector.BounceReport {
	return connector.BounceReport{
		MessageID: messageID,
		Recipient: "buyer@example.com",
		Kind:      kind,
		Reason:    reason,
	}
}

// sentDelivery stages and sends one delivery the real writers' way, so the
// row RecordBounce marks is one production actually produces.
func (e *storeEnv) sentDelivery(t *testing.T, messageID string) ids.UUID {
	t.Helper()
	id := e.stage(t, e.baseInput(e.activity, messageID))
	if err := e.store.RecordSent(e.ctx, id, connector.SendReceipt{ProviderMessageID: "prov-" + messageID}); err != nil {
		t.Fatalf("recording the send: %v", err)
	}
	return id
}

func (e *storeEnv) bounceMark(t *testing.T, id ids.UUID) (kind, reason string, marked bool) {
	t.Helper()
	var k, r *string
	if err := e.owner.QueryRow(context.Background(),
		`SELECT bounce_kind, bounce_reason FROM comms_outbound WHERE id = $1`, id).Scan(&k, &r); err != nil {
		t.Fatalf("reading the bounce mark: %v", err)
	}
	if k == nil {
		return "", "", false
	}
	if r == nil {
		return *k, "", true
	}
	return *k, *r, true
}

func TestRecordBounceMarksTheSentRowWithAuditAndEvent(t *testing.T) {
	e := setupStore(t)
	id := e.sentDelivery(t, "bounced@myco.test")

	marked, err := e.store.RecordBounce(e.asCapturingConnector(e.user.UUID), bounceFor("bounced@myco.test", connector.BounceHard, "550 5.1.1 user unknown"))
	if err != nil {
		t.Fatalf("RecordBounce: %v", err)
	}
	if !marked {
		t.Fatal("the sent row was not marked")
	}
	kind, reason, ok := e.bounceMark(t, id)
	if !ok || kind != "hard" || reason != "550 5.1.1 user unknown" {
		t.Errorf("mark = (%q, %q, %v), want (hard, 550 5.1.1 user unknown, true)", kind, reason, ok)
	}
	var audits, events int
	if err := e.owner.QueryRow(context.Background(),
		`SELECT count(*) FROM audit_log WHERE action = 'update' AND entity_type = 'activity' AND entity_id = $1`,
		e.activity).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if err := e.owner.QueryRow(context.Background(),
		`SELECT count(*) FROM event_outbox WHERE envelope->>'type' = 'comms.delivery_bounced' AND envelope->'entity'->>'id' = $1::text`,
		e.activity.String()).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if audits != 1 || events != 1 {
		t.Errorf("audit rows = %d, outbox events = %d, want 1 and 1 — the write shape is one transaction", audits, events)
	}
}

// The bus is at-least-once and two mail clients can hold the same mailbox, so
// the same report can arrive twice. The FIRST mark is the one kept — its
// timestamp says when the failure was learned — and a replay is a no-op that
// emits nothing new.
func TestRecordBounceKeepsTheFirstMarkOnReplay(t *testing.T) {
	e := setupStore(t)
	e.sentDelivery(t, "twice@myco.test")
	ctx := e.asCapturingConnector(e.user.UUID)

	if marked, err := e.store.RecordBounce(ctx, bounceFor("twice@myco.test", connector.BounceHard, "550 first report")); err != nil || !marked {
		t.Fatalf("first report: marked=%v err=%v", marked, err)
	}
	marked, err := e.store.RecordBounce(ctx, bounceFor("twice@myco.test", connector.BounceSoft, "452 second report"))
	if err != nil {
		t.Fatalf("second report: %v", err)
	}
	if marked {
		t.Error("a replay re-marked the row")
	}
	var events int
	if err := e.owner.QueryRow(context.Background(),
		`SELECT count(*) FROM event_outbox WHERE envelope->>'type' = 'comms.delivery_bounced'`).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if events != 1 {
		t.Errorf("outbox events = %d, want 1 — a no-op replay emits nothing", events)
	}
}

// A report naming mail this installation never sent is a normal input
// answered with no mark and no error. A PENDING delivery is markable — the
// report proves the wire carried it and the receipt write can lose the race
// against it — but a report failing the owner or recipient match records
// nothing, which is what makes a forged report inert.
func TestRecordBounceRefusesWhatItCannotVerify(t *testing.T) {
	e := setupStore(t)
	pending := e.stage(t, e.baseInput(e.activity, "raced@myco.test"))
	ctx := e.asCapturingConnector(e.user.UUID)

	if marked, err := e.store.RecordBounce(ctx, bounceFor("not-ours@elsewhere.test", connector.BounceHard, "x")); err != nil || marked {
		t.Errorf("unknown message: marked=%v err=%v, want false and nil", marked, err)
	}
	stranger := e.asCapturingConnector(ids.NewV7())
	if marked, err := e.store.RecordBounce(stranger, bounceFor("raced@myco.test", connector.BounceHard, "x")); err != nil || marked {
		t.Errorf("another mailbox's capture: marked=%v err=%v, want false and nil", marked, err)
	}
	wrongAddress := bounceFor("raced@myco.test", connector.BounceHard, "x")
	wrongAddress.Recipient = "nobody@customer.example"
	if marked, err := e.store.RecordBounce(ctx, wrongAddress); err != nil || marked {
		t.Errorf("an address the mail never went to: marked=%v err=%v, want false and nil", marked, err)
	}
	if _, _, ok := e.bounceMark(t, pending); ok {
		t.Fatal("a refused report left a mark")
	}
	if marked, err := e.store.RecordBounce(ctx, bounceFor("raced@myco.test", connector.BounceHard, "x")); err != nil || !marked {
		t.Errorf("the report that lost the receipt race: marked=%v err=%v, want the pending row marked", marked, err)
	}
}
