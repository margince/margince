// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"maps"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// backfill drops every proof, the state before proofs were written, and runs
// the data task through the real executor.
func (e proofEnv) backfill(t *testing.T) {
	t.Helper()
	e.WsExec(t, `DELETE FROM privacy_request_proof`)
	task := requestProofDataTask()
	task.Steps[0].Limit = 1
	runMachineAddressTask(principal.SystemActing(e.Admin(), "system:datatask-test"), t, e.Env, task)
}

func (e proofEnv) proofLine(t *testing.T, request ids.UUID) string {
	t.Helper()
	return e.WsScalar(t, `SELECT concat_ws('|', request_kind, result, identity_check, erased_counts_known::text,
		coalesce(requester_email, '-'), coalesce(completed_by_id, '-')) FROM privacy_request_proof
		WHERE request_id = $1`, request)
}

func TestTheProofBackfillRebuildsWhatTheRecordsHoldAndReplaysSafely(t *testing.T) {
	e := setupProofs(t)
	contact := e.SeedContact(t, "Earlier Subject", nil)
	erasure := e.openRequest(t, "erasure", contact.String())
	e.patch(t, erasure, `{"status":"fulfilled","resolution":"erased","identity_check":"id_document"}`)
	access := e.closeAccess(t, "earlier@proof.test")
	erased := e.proofCounts(t, erasure)

	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	step := requestProofDataTask().Steps[0]
	e.WsExec(t, `DELETE FROM privacy_request_proof`)
	var estimated int64
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		var err error
		estimated, _, err = step.Estimate(ctx, tx, 1000)
		return err
	}); err != nil || estimated != 2 {
		t.Fatalf("estimate = %d, %v, want 2", estimated, err)
	}
	e.backfill(t)

	if got := e.proofCounts(t, erasure); !maps.Equal(got, erased) {
		t.Fatalf("backfilled counts %v, want the tombstone's %v", got, erased)
	}
	admin := "human:" + e.AdminUser.String()
	if got, want := e.proofLine(t, access), "access|fulfilled|unknown|true|earlier@proof.test|"+admin; got != want {
		t.Fatalf("backfilled access proof = %s, want %s", got, want)
	}
	if got, want := e.proofLine(t, erasure), "erasure|fulfilled|unknown|true|-|"+admin; got != want {
		t.Fatalf("backfilled erasure proof = %s, want %s", got, want)
	}

	audits := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'privacy_request_proof'`)
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		out, err := step.Batch(ctx, tx, datatask.BatchIn{Limit: 10, RunID: ids.NewV7()})
		if err == nil && (out.Changed != 0 || out.Scanned != 2) {
			t.Errorf("replay changed %d of %d, want 0 of 2", out.Changed, out.Scanned)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if again := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'privacy_request_proof'`); again != audits {
		t.Fatalf("a replay raised the proof audit rows from %d to %d", audits, again)
	}
}

func TestTheProofBackfillNamesTheLatestCloseOfAReopenedRequest(t *testing.T) {
	e := setupProofs(t)
	request := e.closeAccess(t, "reopened@proof.test")
	// The status machine refuses a reopen today; an earlier writer did not,
	// and its audit row is what this request carries.
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(e.Admin(), `UPDATE data_subject_request SET status = 'in_progress' WHERE id = $1`, request); err != nil {
			return err
		}
		_, err := storekit.Audit(e.Admin(), tx, "update", "data_subject_request", request,
			map[string]any{"status": "fulfilled"}, map[string]any{"status": "in_progress"})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	second := ids.NewV7()
	if _, err := e.store.UpdateDSR(e.As(second, nil, integration.AdminPerms), request, consent.UpdateDSRInput{
		Status: new("fulfilled"), Proof: consent.ProofStatement{IdentityCheck: new("id_document")},
	}); err != nil {
		t.Fatal(err)
	}
	e.backfill(t)

	if got := e.proofLine(t, request); !strings.HasSuffix(got, "|human:"+second.String()) {
		t.Fatalf("backfilled proof = %s, want the second close's officer", got)
	}
	closedAt := e.WsScalar(t, `SELECT max(occurred_at)::text FROM audit_log WHERE entity_type = 'data_subject_request'
		AND entity_id = $1 AND after->>'status' = 'fulfilled'`, request)
	if got := e.WsScalar(t, `SELECT answered_at::text FROM privacy_request_proof WHERE request_id = $1`, request); got != closedAt {
		t.Fatalf("backfilled answer date %s, want the latest close %s", got, closedAt)
	}
}

func TestTheProofBackfillFindsTheErasureOfARequestWhoseReferenceIsTombstoned(t *testing.T) {
	e := setupProofs(t)
	contact := e.SeedContact(t, "Long Ago Subject", nil)
	e.seedNoteFor(t, contact, "Bitte loeschen", "Bitte loeschen Sie alles")
	request := e.openRequest(t, "erasure", contact.String())
	e.patch(t, request, `{"status":"fulfilled","resolution":"erased","identity_check":"id_document"}`)
	erased := e.proofCounts(t, request)
	e.answerProofAt(t, request, time.Date(2025, 3, 1, 9, 0, 0, 0, time.UTC))
	e.expire(t, time.Date(2029, 1, 3, 3, 0, 0, 0, time.UTC))
	if ref := e.WsScalar(t, `SELECT subject_ref FROM data_subject_request WHERE id = $1`, request); !strings.HasPrefix(ref, "erased+") {
		t.Fatalf("the request still names %q, so this proves nothing", ref)
	}
	e.backfill(t)

	if got := e.proofCounts(t, request); !maps.Equal(got, erased) {
		t.Fatalf("backfilled counts %v, want the erasure's %v", got, erased)
	}
	if known := e.WsScalar(t, `SELECT erased_counts_known::text FROM privacy_request_proof WHERE request_id = $1`, request); known != "true" {
		t.Fatalf("the tombstoned request's counts are known = %s", known)
	}
}

func TestTheProofBackfillOfARequestClosedWithoutAnAuditRowNamesNoCloser(t *testing.T) {
	e := setupProofs(t)
	request := e.openRequest(t, "access", "unaudited@proof.test")
	e.WsExec(t, `UPDATE data_subject_request SET status = 'rejected', resolution = 'declined' WHERE id = $1`, request)
	e.backfill(t)

	if got, want := e.proofLine(t, request), "access|rejected|unknown|true|unaudited@proof.test|-"; got != want {
		t.Fatalf("backfilled proof = %s, want %s", got, want)
	}
	if e.WsScalar(t, `SELECT (p.answered_at = d.updated_at)::text FROM privacy_request_proof p
		JOIN data_subject_request d ON d.id = p.request_id WHERE p.request_id = $1`, request) != "true" {
		t.Fatal("the proof's answer date is not the request's last write")
	}
}
