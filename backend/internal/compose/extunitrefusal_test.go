// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
	"github.com/margince/margince/backend/pkg/extension"
)

// Every published refusal class reaches the wire as the refusal it IS.
//
// The defect this pins is the one a status assertion catches and an error-text
// assertion never does: all four are the extension surface's own sentinels, so
// httperr — whose table is the core's fixed §0 registry — classified none of
// them and answered `500 internal` to a caller who had simply mistyped an
// input.
func TestAUnitsRefusalReachesTheWireAsItsOwnClass(t *testing.T) {
	for _, tc := range []struct {
		class  error
		status int
		code   string
	}{
		{extension.ErrInvalid, http.StatusUnprocessableEntity, "validation_error"},
		{extension.ErrForbidden, http.StatusForbidden, "permission_denied"},
		{extension.ErrNotFound, http.StatusNotFound, "not_found"},
		{extension.ErrConflict, http.StatusConflict, "conflict"},
	} {
		t.Run(tc.code, func(t *testing.T) {
			raised := fmt.Errorf("%w: paste the token from your profile", tc.class)
			rec := httptest.NewRecorder()
			httperr.Write(rec, httptest.NewRequest(http.MethodPost, "/v1/ext/thing", nil), unitRefusal(raised))

			if rec.Code != tc.status {
				t.Errorf("status = %d, want %d — body %s", rec.Code, tc.status, rec.Body)
			}
			if !strings.Contains(rec.Body.String(), `"code":"`+tc.code+`"`) {
				t.Errorf("body carries no %q code: %s", tc.code, rec.Body)
			}
			// The unit's own sentence is the only part of the answer that says
			// what to DO, so it has to survive the mapping.
			if !strings.Contains(rec.Body.String(), "paste the token from your profile") {
				t.Errorf("the unit's sentence did not reach the caller: %s", rec.Body)
			}
			// And it says it ONCE: the class is already on the wire as the
			// status and the code, so repeating its text in the detail would
			// be the third copy of a thing the caller can already read.
			if strings.Contains(rec.Body.String(), tc.class.Error()) {
				t.Errorf("the class text is repeated inside the detail: %s", rec.Body)
			}
		})
	}
}

// The agent reads the SAME class the caller does, off the same input.
//
// One invariant on two sides of a wire is one item, so the two transports are
// asserted from one table rather than from two lists that can drift. The route
// half is above; this half is what the MCP dispatcher actually asks
// (httperr.Classify, through explainClassified), and a class it cannot
// recognise is the whole defect: an agent that mistyped an argument was told
// the tool "failed for an internal reason" and to RETRY, which re-issues the
// same rejected call until a scheduled run's step budget is gone.
//
// Recognition is also the "no unhandled error log line" half of both claims.
// httperr.Write logs one only on its unclassified branch, and explainClassified
// logs an Error only when Classify answers false; a class both recognise is a
// refusal neither files as a server fault.
func TestTheAgentPathReadsAUnitsRefusalAsTheSameClass(t *testing.T) {
	for _, tc := range []struct {
		class error
		code  string
	}{
		{extension.ErrInvalid, "validation_error"},
		{extension.ErrForbidden, "permission_denied"},
		{extension.ErrNotFound, "not_found"},
		{extension.ErrConflict, "conflict"},
	} {
		t.Run(tc.code, func(t *testing.T) {
			raised := fmt.Errorf("%w: paste the token from your profile", tc.class)

			// Unmapped first, so the assertion below is about the mapping
			// rather than about a taxonomy that would have recognised the
			// sentinel anyway.
			if _, recognised := httperr.Classify(raised); recognised {
				t.Fatalf("the core taxonomy already recognises %v, so this test no longer proves the mapping does anything", tc.class)
			}

			fault, recognised := httperr.Classify(unitRefusal(raised))
			if !recognised {
				t.Fatalf("the agent path cannot classify a unit's %v, so it reports an internal fault and tells the agent to retry", tc.class)
			}
			if fault.Code != tc.code {
				t.Errorf("the agent reads code %q where the caller reads %q — the two transports disagree about one refusal", fault.Code, tc.code)
			}
		})
	}
}

// And the classification is applied where the agent path actually runs a unit's
// handler. The table above proves the mapping; this proves Handle uses it,
// which is the line that was missing — unitRefusal existed and only the route
// called it.
func TestTheToolPathClassifiesWhatTheUnitsHandlerReturned(t *testing.T) {
	tool := extensionTool{
		unit:    "refusaldemo",
		version: "1.0.0",
		spec:    mcp.ToolSpec{Name: "refusaldemo_thing"},
		handle: func(context.Context, extension.Runtime, json.RawMessage) (json.RawMessage, error) {
			return nil, fmt.Errorf("%w: the body is empty", extension.ErrInvalid)
		},
	}

	_, err := tool.Handle(context.Background(), json.RawMessage(`{}`))

	fault, recognised := httperr.Classify(err)
	if !recognised {
		t.Fatalf("Handle returned the unit's raw sentinel (%v) — the agent path answers an internal fault for a caller's own mistake", err)
	}
	if fault.Code != "validation_error" {
		t.Errorf("code = %q, want validation_error", fault.Code)
	}
}

// A refusal the CORE raised passes through untouched — the extension route's
// standing rule that a unit's refusal reads like the core route's beside it
// cuts both ways, and re-mapping something already in the product's vocabulary
// is how the two shapes drift apart.
func TestARefusalTheCoreRaisedIsNotRemapped(t *testing.T) {
	core := errors.New("compose: something the core said")
	if got := unitRefusal(core); !errors.Is(got, core) {
		t.Fatalf("unitRefusal rewrote a core error: %v", got)
	}
	if unitRefusal(nil) != nil {
		t.Fatal("unitRefusal invented an error out of nil")
	}
}
