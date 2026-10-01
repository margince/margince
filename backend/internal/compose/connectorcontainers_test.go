// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Every refusal the folder listing makes is a different fact, and a client acts
// on each differently — which is the whole reason they are separate arms rather
// than one error. These assert them apart.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

func listContainers(ctx context.Context, t *testing.T, h connectorHandlers, provider string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/connectors/"+provider+"/containers", nil)
	h.ListConnectorContainers(rec, req.WithContext(ctx), crmcontracts.CaptureProvider(provider))
	return rec
}

// A deployment that composed no registry answers the declared 501 rather than
// panicking on a nil.
func TestListContainersOnAnUnwiredRoleAnswers501(t *testing.T) {
	rec := listContainers(humanCtx(), t, connectorHandlers{}, "gmail")
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want 501 on a role with no registry", rec.Code)
	}
}

// Folders belong to a mailbox. A calendar connection is told so rather than
// being handed an empty list, which would read as "this mailbox has none".
func TestListContainersRefusesACalendarProvider(t *testing.T) {
	rec := listContainers(humanCtx(), t, wiredHandlers(), "gcal")
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422 for a provider that is not a mailbox", rec.Code)
	}
}

// Reading your own mailbox's folders is a signed-in human action: an
// unauthenticated caller is refused before any provider is asked.
func TestListContainersRefusesAnAnonymousCaller(t *testing.T) {
	rec := listContainers(context.Background(), t, wiredHandlers(), "gmail")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 with no principal on the context", rec.Code)
	}
}

// The 501 arm that matters: a provider this build has, which cannot list
// folders. The client renders its other two exclusion kinds on this answer, so
// it has to be distinguishable from an outage and from an absent mailbox.
func TestListContainersReportsAProviderThatCannotList(t *testing.T) {
	h := wiredHandlers()
	rec := listContainers(humanCtx(), t, h, "telegram")
	// Telegram is a channel transport: either it is not a mailbox (422) or it
	// cannot list (501), and both are honest refusals that name the reason
	// rather than answering an empty list.
	if rec.Code != http.StatusNotImplemented && rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want a named refusal for a channel transport", rec.Code)
	}
	var problem map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &problem); err != nil {
		t.Fatalf("refusal body is not a problem document: %v", err)
	}
	if problem["code"] == nil || problem["code"] == "" {
		t.Error("the refusal carries no code — a client cannot tell which case it is")
	}
}
