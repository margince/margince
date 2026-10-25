// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// erasedBookerContacts answers every ensure as the contacts store answers an
// address on the erasure list.
type erasedBookerContacts struct{}

func (erasedBookerContacts) EnsureContactByEmail(context.Context, string, string, string) (ids.UUID, error) {
	return ids.Nil, &storekit.ErasedIdentifierError{Field: "emails"}
}

func TestThePublicBookingDoorAnswersAnErasedAddressAsATakenSlot(t *testing.T) {
	start := time.Date(2026, 3, 2, 10, 0, 0, 0, time.UTC)
	var req crmcontracts.BookPublicMeetingJSONRequestBody
	req.Booker.Name = "Wieder Da"
	req.Booker.Email = "geloescht@betroffene.example"
	req.Start, req.End = start, start.Add(30*time.Minute)
	httpReq := httptest.NewRequest(http.MethodPost, "/public/booking/x", nil)

	erased := httptest.NewRecorder()
	h := Handlers{}.WithPublicBooking(erasedBookerContacts{}, nil)
	if _, ok := h.ensureBooker(erased, httpReq, req); ok {
		t.Fatal("the door admitted an erased booker")
	}
	taken := httptest.NewRecorder()
	writeStoreErr(taken, httpReq, &SlotTakenError{Start: start})

	if erased.Code != taken.Code || erased.Body.String() != taken.Body.String() {
		t.Fatalf("the erased answer %d %s differs from a taken slot's %d %s",
			erased.Code, erased.Body.String(), taken.Code, taken.Body.String())
	}
	for _, header := range []string{"Content-Type", "Content-Length", "Cache-Control"} {
		if erased.Header().Get(header) != taken.Header().Get(header) {
			t.Errorf("header %s: erased %q, taken slot %q", header, erased.Header().Get(header), taken.Header().Get(header))
		}
	}
}
