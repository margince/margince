// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package search

// Which stopped statement is the READER'S news, and which is not.
//
// Postgres raises 57014 for three different things — a spent statement_timeout,
// an operator cancelling the backend, and the client going away — and only one
// of them is something the person typing can do anything about. Told apart
// wrongly, a caller whose own request was cancelled is sent to rewrite a query
// that was fine.
//
// Asserted here rather than by racing a real ceiling. A test that bounded a
// live statement to a millisecond and expected it to lose would be timing the
// machine it runs on: on a warm cache the query finishes first and the test
// passes for the wrong reason, or fails on a busy one. What can be wrong is the
// judgement, and the judgement is a pure function of an error and a context.

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

// queryCanceled is the error Postgres actually returns, by its SQLSTATE.
func queryCanceled() error {
	return &pgconn.PgError{Code: "57014", Message: "canceling statement due to statement timeout"}
}

func TestASpentCeilingIsReportedAsATooBroadQuery(t *testing.T) {
	err := rankingFault(context.Background(), queryCanceled())

	var tooBroad *QueryTooBroadError
	if !errors.As(err, &tooBroad) {
		t.Fatalf("err = %v, want QueryTooBroadError — a spent ceiling surfacing as a raw database "+
			"fault tells the reader nothing they can act on", err)
	}
	// 422 naming `q`, because `q` is what the reader can change. A 5xx would say
	// the server is broken, and retrying the same words would not help.
	field, code, message := tooBroad.FieldFault()
	if field != "q" || code != "query_too_broad" {
		t.Errorf("fault = (%q, %q), want (q, query_too_broad)", field, code)
	}
	// It still WRAPS the database error, so the query executor — which wants the
	// same stopped statement as a degraded plan rather than a fault — can still
	// recognise it.
	if !errors.Is(err, tooBroad.Err) {
		t.Error("the fault does not carry the database error, so the executor that reads the " +
			"SQLSTATE would see a plain error where it expects a cancellation")
	}
	if message == "" {
		t.Error("the fault carries no message, so the reader is told the query is wrong and not what to do")
	}
}

// THE CASE THAT MUST NOT BE MISREAD. The caller's own deadline expired, so the
// same 57014 comes back — and calling that a too-broad query describes their
// timeout as a property of the workspace, to nobody, since the reader is gone.
func TestACancelledRequestIsNotATooBroadQuery(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := rankingFault(ctx, queryCanceled())

	var tooBroad *QueryTooBroadError
	if errors.As(err, &tooBroad) {
		t.Error("a cancelled request was reported as a too-broad query — the reader would be sent " +
			"to rewrite a query that was fine, and there is nobody left to read the answer")
	}
}

// And every other failure stays what it was. A ceiling that swallowed unrelated
// database faults into "narrow your search" would hide them behind advice that
// cannot work.
func TestAnUnrelatedDatabaseFaultIsNotRewritten(t *testing.T) {
	err := rankingFault(context.Background(), fmt.Errorf("connection refused"))

	var tooBroad *QueryTooBroadError
	if errors.As(err, &tooBroad) {
		t.Error("an unrelated fault was reported as a too-broad query")
	}
	if err == nil {
		t.Fatal("an unrelated fault was swallowed entirely")
	}
}
