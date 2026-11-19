// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ownedPromise is the looks-kept promise, recorded by the rep who reads it.
func ownedPromise(t *testing.T) looksKeptFixture {
	t.Helper()
	f := looksKeptPromise(t)
	f.e.execAs(f.ctx, t, `UPDATE conversation_claim SET captured_by = $2 WHERE id = $1`, f.claim, "human:"+f.e.rep.String())
	return f
}

// sweep runs one pass the way the job does: the system writes, the owner reads.
func (f looksKeptFixture) sweep(t *testing.T) {
	t.Helper()
	writer := principal.SystemActing(principal.WithWorkspaceID(context.Background(), f.e.ws), PromiseKeptActor)
	if err := f.e.store.tx(writer, func(tx pgx.Tx) error {
		candidates, err := f.e.store.KeptPromiseCandidatesTx(writer, tx)
		if err != nil {
			return err
		}
		return f.e.store.ReconcileKeptPromisesTx(writer, f.ctx, tx, candidates)
	}); err != nil {
		t.Fatalf("sweep: %v", err)
	}
}

func (f looksKeptFixture) status(t *testing.T) string {
	t.Helper()
	var status string
	if err := f.e.store.tx(f.ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(f.ctx, `SELECT status FROM conversation_claim WHERE id = $1`, f.claim).Scan(&status)
	}); err != nil {
		t.Fatal(err)
	}
	return status
}

func (f looksKeptFixture) claimRead(t *testing.T) crmcontracts.ConversationClaim {
	t.Helper()
	var claims []crmcontracts.ConversationClaim
	if err := f.e.store.tx(f.ctx, func(tx pgx.Tx) error {
		var err error
		claims, err = f.e.store.ClaimsForContact(f.ctx, tx, ids.From[ids.ContactKind](f.holder), nil, 10)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	for _, claim := range claims {
		if ids.UUID(claim.Id) == f.claim {
			return claim
		}
	}
	t.Fatal("the claim is not on the contact")
	return crmcontracts.ConversationClaim{}
}

func TestAnAcceptedLaterMeetingClosesTheOwnersPromiseAndNamesTheEvidence(t *testing.T) {
	f := ownedPromise(t)
	f.sweep(t)
	if got := f.status(t); got != "done" {
		t.Fatalf("status = %q, want done", got)
	}
	claim := f.claimRead(t)
	if claim.AutoKept == nil || claim.AutoKept.Evidence == nil || claim.AutoKept.Evidence.GuestName != "Riley Reed" {
		t.Fatalf("auto_kept = %+v, want the evidence naming Riley Reed", claim.AutoKept)
	}
	if got := f.e.countAs(f.ctx, t, `SELECT count(*) FROM audit_log WHERE after->>'claim_id' = $1 AND actor_type = 'system' AND actor_id = $2`,
		f.claim.String(), PromiseKeptActor); got != 1 {
		t.Errorf("system audit rows = %d, want 1", got)
	}
	if got := f.e.countAs(f.ctx, t, `SELECT count(*) FROM event_outbox WHERE envelope->>'type' = 'conversation_claim.changed'
		AND envelope->'payload'->>'claim_id' = $1 AND envelope->'payload'->>'status' = 'done' AND envelope->'actor'->>'id' = $2`,
		f.claim.String(), PromiseKeptActor); got != 1 {
		t.Errorf("done events = %d, want 1", got)
	}
}

func TestAPromiseNobodyIsResponsibleForIsOnlyHinted(t *testing.T) {
	f := looksKeptPromise(t)
	f.sweep(t)
	if got := f.status(t); got != "open" {
		t.Errorf("status = %q, want open", got)
	}
	if f.hint(t) == nil {
		t.Error("the hint went away")
	}
}

func TestOnlyTheRightGuestAndMeetingClosesAPromise(t *testing.T) {
	for name, change := range map[string]string{
		"a guest at no shared company":     `DELETE FROM relationship WHERE contact_id = $1 AND $2::uuid IS NOT NULL`,
		"a meeting before the promise":     `UPDATE activity SET occurred_at = now() - interval '10 days' WHERE id = $2 AND $1::uuid IS NOT NULL`,
		"a meeting the owner may not open": `UPDATE activity SET audience = 'participants' WHERE id = $2 AND $1::uuid IS NOT NULL`,
	} {
		t.Run(name, func(t *testing.T) {
			f := ownedPromise(t)
			f.e.execAs(f.ctx, t, change, f.guest, f.meeting)
			f.sweep(t)
			if got := f.status(t); got != "open" {
				t.Errorf("%s closed the promise: %q", name, got)
			}
		})
	}
}

func TestAnAutomaticCloseReopensWhenItsEvidenceGoes(t *testing.T) {
	for name, change := range map[string]string{
		"declined":      `UPDATE activity_participant SET response = 'declined' WHERE activity_id = $1 AND response IS NOT NULL`,
		"guest removed": `DELETE FROM activity_participant WHERE activity_id = $1 AND response IS NOT NULL`,
		"called off":    `UPDATE activity SET meeting_status = 'canceled' WHERE id = $1`,
		"archived":      `UPDATE activity SET archived_at = now() WHERE id = $1`,
	} {
		t.Run(name, func(t *testing.T) {
			f := ownedPromise(t)
			f.sweep(t)
			f.e.execAs(f.ctx, t, change, f.meeting)
			f.sweep(t)
			if got := f.status(t); got != "open" {
				t.Errorf("after %s the promise is %q, want open", name, got)
			}
			if f.claimRead(t).AutoKept != nil {
				t.Error("a reopened promise still reads as closed by Margince")
			}
		})
	}
}

func TestAHumanCloseIsNotReopenedByADecline(t *testing.T) {
	f := ownedPromise(t)
	if err := f.e.store.SettleConversationClaim(f.ctx, f.claim, "done"); err != nil {
		t.Fatal(err)
	}
	f.e.execAs(f.ctx, t, `UPDATE activity_participant SET response = 'declined' WHERE activity_id = $1 AND response IS NOT NULL`, f.meeting)
	f.sweep(t)
	if got := f.status(t); got != "done" {
		t.Errorf("a person's close became %q", got)
	}
}

func TestUndoReopensTheClaimAndChangesNothingElse(t *testing.T) {
	f := ownedPromise(t)
	task := ids.NewV7()
	f.e.execAs(f.ctx, t, `INSERT INTO activity (id, kind, subject, source, captured_by, occurred_at, is_done, assignee_id)
		VALUES ($1, 'task', 'Invite Riley', 'manual', 'human:test', now(), false, $2)`, task, f.e.rep)
	f.e.execAs(f.ctx, t, `UPDATE conversation_claim SET task_activity_id = $2 WHERE id = $1`, f.claim, task)
	const snapshot = `SELECT (to_jsonb(c) - 'status' - 'version' - 'updated_at')::text FROM conversation_claim c WHERE c.id = $1`
	before := f.rowText(t, snapshot)
	f.sweep(t)
	if got := f.e.countAs(f.ctx, t, `SELECT count(*) FROM activity WHERE id = $1 AND NOT is_done AND archived_at IS NULL`, task); got != 1 {
		t.Fatalf("closing the promise touched its task")
	}
	if err := f.e.store.ReopenAutoKeptClaim(f.ctx, f.claim); err != nil {
		t.Fatalf("undo: %v", err)
	}
	if got := f.status(t); got != "open" {
		t.Fatalf("status = %q after undo", got)
	}
	if after := f.rowText(t, snapshot); after != before {
		t.Errorf("undo changed more than the status:\nbefore %s\nafter  %s", before, after)
	}
	f.sweep(t)
	if got := f.status(t); got != "open" {
		t.Errorf("the sweep closed a promise a person reopened: %q", got)
	}
	if err := f.e.store.ReopenAutoKeptClaim(f.ctx, f.claim); err != nil {
		t.Errorf("undoing an open promise = %v, want success", err)
	}
}

func (f looksKeptFixture) rowText(t *testing.T, sql string) string {
	t.Helper()
	var out string
	if err := f.e.store.tx(f.ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(f.ctx, sql, f.claim).Scan(&out)
	}); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestUndoRefusesAPersonsCloseAndHidesAClaimOutsideTheReadersScope(t *testing.T) {
	f := ownedPromise(t)
	f.sweep(t)
	f.e.execAs(f.ctx, t, `UPDATE activity SET audience = 'participants' WHERE id = $1`, f.source)
	outsider := principal.WithActor(f.ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + f.e.rep.String(), UserID: f.e.rep,
		Permissions: principal.Permissions{
			RoleKeys: []string{"rep"},
			Objects: map[string]principal.ObjectGrant{
				"contact":  {Read: true, Update: true},
				"activity": {Read: true},
			},
			RowScope: principal.RowScopeAll,
		},
	})
	if err := f.e.store.ReopenAutoKeptClaim(outsider, f.claim); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("undo outside the reader's scope = %v, want not found", err)
	}
	if got := f.status(t); got != "done" {
		t.Fatalf("a refused undo moved the claim to %q", got)
	}

	g := ownedPromise(t)
	if err := g.e.store.SettleConversationClaim(g.ctx, g.claim, "done"); err != nil {
		t.Fatal(err)
	}
	if err := g.e.store.ReopenAutoKeptClaim(g.ctx, g.claim); !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("undoing a person's close = %v, want conflict", err)
	}
}
