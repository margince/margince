// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// mailAt writes one message on thread, at the instant given, between us and buyer.
func (e *loadEnv) mailAt(t *testing.T, thread, direction, buyer string, at time.Time) ids.UUID {
	t.Helper()
	activity := ids.NewV7()
	e.exec(t, `INSERT INTO activity (id, kind, direction, subject, occurred_at, thread_key, counterparty_email, source, captured_by)
		VALUES ($1, 'email', $2, 'the order', $3, $4, $5, 'manual', 'system')`,
		activity, direction, at, thread, buyer)
	return activity
}

// owedAt answers, per message, whether a reply is owed on it at each instant.
func (e *loadEnv) owedAt(t *testing.T, messages []ids.UUID, instants []time.Time) map[ids.UUID][]bool {
	t.Helper()
	owed := map[ids.UUID][]bool{}
	for _, at := range instants {
		rows, err := e.owner.Query(context.Background(), `SELECT a.id, coalesce(`+
			owedSQL("$1::timestamptz", neverRelaxed, neverRelaxed, neverRelaxed)+`, false)
			FROM activity a WHERE a.id = ANY($2)`, at, messages)
		if err != nil {
			t.Fatalf("asking what is owed: %v", err)
		}
		for rows.Next() {
			var id ids.UUID
			var isOwed bool
			if err := rows.Scan(&id, &isOwed); err != nil {
				t.Fatalf("reading what is owed: %v", err)
			}
			owed[id] = append(owed[id], isOwed)
		}
		if err := rows.Err(); err != nil {
			t.Fatalf("reading what is owed: %v", err)
		}
	}
	return owed
}

func (e *loadEnv) stampSettled(t *testing.T) {
	t.Helper()
	ctx := principal.WithWorkspaceID(context.Background(), e.ws)
	ctx = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalSystem, ID: "system:reply-settled",
		Permissions: principal.Permissions{
			Objects:  map[string]principal.ObjectGrant{"activity": {Read: true, Update: true}},
			RowScope: principal.RowScopeAll,
		},
	})
	store := NewStore(database.BindTo(e.pool, ids.From[ids.WorkspaceKind](e.ws)))
	for {
		n, err := store.StampReplySettledBatch(ctx, 50)
		if err != nil {
			t.Fatalf("stamping: %v", err)
		}
		if n == 0 {
			return
		}
	}
}

func (e *loadEnv) stampOf(t *testing.T, message ids.UUID) (stamp *time.Time, version int64) {
	t.Helper()
	if err := e.owner.QueryRow(context.Background(),
		`SELECT CASE WHEN reply_settled_at = 'infinity' THEN NULL ELSE reply_settled_at END, version
		   FROM activity WHERE id = $1`, message).Scan(&stamp, &version); err != nil {
		t.Fatalf("reading the stamp: %v", err)
	}
	return stamp, version
}

// The stamp only lets the owed check stop early: at every instant, a message is
// owed exactly when it was before the stamp existed.
func TestTheSettledStampNeverChangesWhatIsOwed(t *testing.T) {
	e := setupLoad(t)
	buyer := "buyer-" + ids.NewV7().String() + "@customer.test"
	now := clocktest.Now(t)
	answered := "thread-" + ids.NewV7().String()
	asked := e.mailAt(t, answered, "inbound", buyer, now.Add(-5*time.Hour))
	reply := e.mailAt(t, answered, "outbound", buyer, now.Add(-3*time.Hour))
	followed := "thread-" + ids.NewV7().String()
	first := e.mailAt(t, followed, "inbound", buyer, now.Add(-4*time.Hour))
	second := e.mailAt(t, followed, "inbound", buyer, now.Add(-2*time.Hour))
	silent := e.mailAt(t, "thread-"+ids.NewV7().String(), "inbound", buyer, now.Add(-time.Hour))
	messages := []ids.UUID{asked, first, second, silent}
	instants := []time.Time{now.Add(-4 * time.Hour), now.Add(-150 * time.Minute), now.Add(-90 * time.Minute), now}

	before := e.owedAt(t, messages, instants)
	if !before[asked][0] || before[asked][3] || !before[silent][3] || !before[first][0] || before[first][3] {
		t.Fatalf("the fixture does not exercise the check: owed %v", before)
	}
	_, versionBefore := e.stampOf(t, asked)
	e.stampSettled(t)
	after := e.owedAt(t, messages, instants)
	for _, message := range messages {
		for i := range instants {
			if before[message][i] != after[message][i] {
				t.Errorf("message %s at instant %d: owed %v before the stamp, %v after", message, i, before[message][i], after[message][i])
			}
		}
	}
	if stamp, version := e.stampOf(t, asked); stamp == nil || version != versionBefore {
		t.Errorf("the answered message has stamp %v at version %d, want our reply's time at version %d", stamp, version, versionBefore)
	}
	if stamp, _ := e.stampOf(t, first); stamp == nil {
		t.Error("a message the buyer followed up on carries no stamp")
	}
	if stamp, _ := e.stampOf(t, silent); stamp != nil {
		t.Errorf("a message nobody answered is stamped settled at %v", stamp)
	}

	e.exec(t, `UPDATE activity SET archived_at = $2 WHERE id = $1`, reply, now)
	var cleared bool
	if err := e.owner.QueryRow(context.Background(),
		`SELECT reply_settled_at IS NULL FROM activity WHERE id = $1`, asked).Scan(&cleared); err != nil || !cleared {
		t.Fatalf("archiving the reply left the answered message's stamp in place (cleared %v, err %v)", cleared, err)
	}
	e.stampSettled(t)
	if owed := e.owedAt(t, []ids.UUID{asked}, []time.Time{now}); !owed[asked][0] {
		t.Error("with its only reply archived, the message is still read as settled")
	}

	e.exec(t, `DELETE FROM activity WHERE id = $1`, second)
	if stamp, _ := e.stampOf(t, first); stamp != nil {
		t.Errorf("deleting the follow-up left the first message settled at %v", stamp)
	}
}
