// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package httperr

// Whose fault a constraint is, when the SQLSTATE cannot say.
//
// Every mutation writes three rows in one transaction — the record, its audit
// entry, its outbox event — and only the first carries anything the caller
// sent. Told apart by SQLSTATE alone they are the same event, and the net under
// the per-path validations answered all three as the caller's input to fix.
//
// For the two the caller never wrote, that answer is wrong in every clause it
// makes: not their value, nothing in their request to check, and "do not retry
// unchanged" said to a client whose next call would succeed the moment somebody
// fixes our code. The table is what separates them, and Postgres sends it.

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
)

func TestAConstraintOnARowTheCallerNeverWroteIsOurFault(t *testing.T) {
	for _, tc := range []struct {
		name       string
		table      string
		constraint string
	}{
		{
			// The case that surfaced this: connecting a data provider wrote an
			// audit row whose verb was missing from the CHECK. The admin was
			// told to check values in a request that was entirely valid.
			name:  "an audit verb the CHECK does not admit",
			table: storekit.TableAudit, constraint: "audit_log_action_check",
		},
		{
			name:  "an actor type the CHECK does not admit",
			table: storekit.TableAudit, constraint: "audit_log_actor_type_check",
		},
		{
			name:  "an outbox envelope the schema refuses",
			table: storekit.TableOutbox, constraint: "event_outbox_stream_check",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cause := &pgconn.PgError{
				Code: "23514", TableName: tc.table, ConstraintName: tc.constraint,
				Message: `new row for relation "` + tc.table + `" violates check constraint`,
			}
			fault, ok := Classify(fmt.Errorf("recording the write: %w", cause))
			if !ok {
				t.Fatal("the fault reached the unhandled path — which answers the same 500, but logs it as unknown rather than as the constraint it is")
			}
			if fault.Status != http.StatusInternalServerError {
				t.Errorf("status = %d, want 500 — the caller wrote no part of this row, so there is nothing in their request to fix", fault.Status)
			}
			if fault.Code != "internal" {
				t.Errorf("code = %q, want internal", fault.Code)
			}
			// No sentence at all. Naming the constraint would leak the schema,
			// and any sentence about "a value in this request" would be the
			// same falsehood the 422 was telling, one line shorter.
			if fault.Detail != "" {
				t.Errorf("detail = %q, want none — every true thing about this fault is already in the status", fault.Detail)
			}
			// The operator still gets the constraint: httperr.Write logs
			// InfraCause, and without it this defect is a bare 500 with nothing
			// to chase, which is worse than the wrong 422 it replaces.
			if !errors.Is(fault.InfraCause, cause) {
				t.Errorf("InfraCause = %v, want the Postgres error — the caller learns nothing, so the log has to learn everything", fault.InfraCause)
			}
		})
	}
}

// The control, one field apart. Without it the case above would pass against a
// classifier that answered 500 for every constraint — which would take every
// per-path validation's fallback with it.
func TestAConstraintOnTheCallersOwnRecordIsStillTheirs(t *testing.T) {
	fault, ok := Classify(fmt.Errorf("writing the row: %w", &pgconn.PgError{
		Code: "23514", TableName: "company", ConstraintName: "company_size_band_check",
		Message: `new row for relation "company" violates check constraint`,
	}))
	if !ok {
		t.Fatal("the constraint reached the unhandled path")
	}
	if fault.Status != http.StatusUnprocessableEntity || fault.Code != "value_not_allowed" {
		t.Fatalf("status/code = %d/%q, want 422/value_not_allowed — a CHECK on the record the request names is the caller's to fix",
			fault.Status, fault.Code)
	}
}

// A constraint failure that names no table is not evidence of anything: it must
// not be read as a side effect, and it must not be read as the caller's record
// either — it falls through to whatever the SQLSTATE says on its own.
func TestAViolationThatNamesNoTableIsNotClaimedByEitherSide(t *testing.T) {
	if table, ok := storekit.ViolatedTable(&pgconn.PgError{Code: "23514"}); ok {
		t.Fatalf("ViolatedTable answered %q for an error naming none", table)
	}
	fault, ok := Classify(&pgconn.PgError{Code: "23514", ConstraintName: "some_check"})
	if !ok || fault.Status != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d (classified %v), want the 422 a nameless CHECK has always answered", fault.Status, ok)
	}
}
