// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// What the route does with the two answers the reading can give.
//
// The handler is four lines, and both of them matter: a reader without the
// tier must reach a refusal status rather than a 200 carrying an empty list,
// because an empty list is what a clear queue looks like and the two must not
// render the same.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// A reader admitted to the reading gets the rows, as JSON the contract names.
func TestTheRouteAnswersTheRowsAsTheContractShape(t *testing.T) {
	t.Parallel()

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/v1/worklist/hidden/set_aside", nil).WithContext(aLead())

	NewHandlers(unboundService()).GetHiddenBacklogRows(w, r, "set_aside")

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var got crmcontracts.HiddenBacklogRows
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("the body is not the contract shape: %v", err)
	}
	if string(got.Rule) != "set_aside" {
		t.Errorf("rule = %q, want the one the route named", got.Rule)
	}
}

// A reader without the tier is REFUSED rather than handed an empty list. The
// two are indistinguishable to a reader otherwise, and the empty one reads as
// good news.
func TestTheRouteRefusesAReaderWithoutTheTier(t *testing.T) {
	t.Parallel()

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/v1/worklist/hidden/set_aside", nil).
		WithContext(boardReaderAt(principal.RowScopeOwn))

	NewHandlers(unboundService()).GetHiddenBacklogRows(w, r, "set_aside")

	if w.Code == http.StatusOK {
		t.Fatalf("status = 200 for a reader who may not ask; body = %s", w.Body.String())
	}
}
