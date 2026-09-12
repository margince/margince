// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package httperr

// A deploy crossing a live connection, told apart from a defect in our own SQL.
//
// Both arrive as 0A000. One clears on the next attempt and the caller should be
// told to make it; the other is a statement no retry will ever make legal, and
// telling a client to repeat it spends their budget on a settled refusal. The
// routine and the message are what separate them, and Postgres sends both.

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

// The two ways the refusal identifies itself on the wire. A pooler relays the
// fields it chooses to, so neither may be the one the mapping depends on.
func TestASchemaChangeUnderALivePlanIsRetryable(t *testing.T) {
	for _, tc := range []struct {
		name  string
		cause *pgconn.PgError
	}{
		{
			name: "identified by its routine",
			cause: &pgconn.PgError{
				Code: "0A000", Routine: "RevalidateCachedQuery",
				Message: "cached plan must not change result type",
			},
		},
		{
			name: "identified by its message, the routine lost in a pooler",
			cause: &pgconn.PgError{
				Code: "0A000", Message: "cached plan must not change result type",
			},
		},
		{
			// A translated server keeps the routine and loses the sentence.
			name: "identified by its routine, the message in another language",
			cause: &pgconn.PgError{
				Code: "0A000", Routine: "RevalidateCachedQuery",
				Message: "der zwischengespeicherte Plan darf den Ergebnistyp nicht ändern",
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fault, ok := Classify(fmt.Errorf("listing deals: %w", tc.cause))
			if !ok {
				t.Fatal("the refusal reached the unhandled path, where it is logged as an unknown " +
					"server fault and reported to the caller as a settled one")
			}
			if fault.Status != http.StatusServiceUnavailable {
				t.Errorf("status %d, want %d — the installation could not serve this request, and the "+
					"caller did nothing wrong", fault.Status, http.StatusServiceUnavailable)
			}
			if !fault.Transient() {
				t.Errorf("code %q is not transient, so every surface tells the caller repeating the "+
					"call cannot help — the one thing that does help here", fault.Code)
			}
			if fault.InfraCause == nil {
				t.Error("the cause did not reach InfraCause, so the operator's half of the event is lost")
			}
			if strings.Contains(fault.Detail, "cached plan") || strings.Contains(fault.Detail, "0A000") {
				t.Errorf("the detail forwards database text to the caller: %q", fault.Detail)
			}
		})
	}
}

// The rest of 0A000 is a statement WE sent that the server will not run, and no
// number of retries makes it legal. Classifying the class rather than this one
// member would turn every such defect into advice to try again — the exact
// inversion the stale plan exists to correct.
func TestAnUnsupportedStatementStaysASettledServerFault(t *testing.T) {
	for _, cause := range []*pgconn.PgError{
		{Code: "0A000", Routine: "DefineIndex", Message: "access method does not support this feature"},
		{Code: "0A000", Routine: "transformInsertStmt", Message: "cannot insert into view"},
	} {
		if fault, ok := Classify(fmt.Errorf("running the report: %w", cause)); ok {
			t.Errorf("%q was classified as %+v, want the opaque 500 — it is our defect, and a caller "+
				"told to retry it will retry it forever", cause.Message, fault)
		}
	}
}

// A 0A000 is not a constraint, and the net that answers constraints must not
// claim it: "a value in this request is outside what its field accepts" is a
// false statement about a request whose values were all fine.
func TestTheStalePlanIsNotAnsweredAsTheCallersInput(t *testing.T) {
	cause := &pgconn.PgError{
		Code: "0A000", Routine: "RevalidateCachedQuery",
		Message: "cached plan must not change result type",
	}
	fault, ok := Classify(fmt.Errorf("updating the company: %w", cause))
	if !ok {
		t.Fatal("the refusal was not classified")
	}
	if fault.Status >= 400 && fault.Status < 500 {
		t.Errorf("status %d blames the caller for a deployment of ours", fault.Status)
	}
	if !errors.Is(fault.InfraCause, error(cause)) {
		t.Errorf("InfraCause = %v, want the pg error itself so the log names the routine", fault.InfraCause)
	}
}
