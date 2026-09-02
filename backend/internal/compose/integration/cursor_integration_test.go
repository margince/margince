// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The invariant: a keyset cursor is client input, so a token that fails to
// decode answers the contract's `422 code: malformed_cursor` — on EVERY
// cursor-paginated list operation the contract exposes.
//
// The CODE is the assertion, not the status class. A client acts on the code:
// malformed_cursor means re-issue without the token. `required` tells it to
// send a field it just sent, `invalid` names no remedy, and neither is
// reachable from the contract, so a client written against the contract cannot
// recognise them — yet all three are 4xx, and a test that asked only for a 4xx
// passed over every one of them.
//
// The endpoint set is a SAMPLE, not a census: it carries every implementation
// that decodes the token itself, plus several that delegate to
// storekit.DecodeCursor. The contract declares the Cursor parameter on far more
// operations than are listed here, and what covers those is the static gate in
// backend/gates/cursorrefusal_test.go — it reads every refusal in the tree, where this
// suite reads the wire for the ones that got it wrong.
//
// Saying which is which matters: a list that claimed to enumerate the contract
// would stop the next author looking when an endpoint left the list.

import (
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

func TestMalformedCursorAnswersMalformedCursorEverywhere(t *testing.T) {
	e := apptest.SetupApp(t)

	apptest.BootstrapWorkspaceSession(t, e, "Cursor Probe", "admin@cursor.test", "Admin")

	// Not valid base64url, so every decoder rejects it.
	const garbage = "cursor=%21%21garbage%21%21"
	endpoints := []string{
		"/v1/people?" + garbage,
		"/v1/organizations?" + garbage,
		"/v1/partners?" + garbage,
		"/v1/deals?" + garbage,
		"/v1/activities?" + garbage,
		"/v1/leads?" + garbage,
		"/v1/relationships?" + garbage,
		"/v1/search?q=probe&" + garbage,
		"/v1/dedupe/candidates?" + garbage,
		"/v1/data-subject-requests?" + garbage,
		"/v1/approvals?" + garbage,
	}
	// A token that is valid base64 of valid JSON but is not a cursor. It decodes
	// where garbage does not, so it reaches the layer that has to decide whether
	// the SHAPE is a cursor — a different refusal from "this is not a token".
	const notACursor = "cursor=bnVsbA"
	endpoints = append(endpoints, "/v1/dedupe/candidates?"+notACursor)

	for _, path := range endpoints {
		var problem cursorProblem
		status := e.Call(t, "GET", path, nil, nil, &problem)
		if status != http.StatusUnprocessableEntity {
			t.Errorf("GET %s with a malformed cursor = %d %+v, want 422", path, status, problem)
			continue
		}
		if problem.Code != "validation_error" {
			t.Errorf("GET %s problem code = %q, want validation_error", path, problem.Code)
			continue
		}
		if len(problem.Details.Errors) != 1 {
			t.Errorf("GET %s reported %+v, want exactly one field error", path, problem.Details.Errors)
			continue
		}
		if got := problem.Details.Errors[0]; got.Field != "cursor" || got.Code != "malformed_cursor" {
			t.Errorf("GET %s field error = %+v, want field=cursor code=malformed_cursor", path, got)
		}
	}
}

// cursorProblem is the RFC 7807 body with the field refusals a caller reads to
// learn which input to drop.
type cursorProblem struct {
	Code    string `json:"code"`
	Details struct {
		Errors []struct {
			Field string `json:"field"`
			Code  string `json:"code"`
		} `json:"errors"`
	} `json:"details"`
}
