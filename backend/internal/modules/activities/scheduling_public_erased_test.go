// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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

func TestThePublicBookingDoorRefusesAnErasedAddressWithoutSayingSo(t *testing.T) {
	h := Handlers{}.WithPublicBooking(erasedBookerContacts{}, nil)
	var req crmcontracts.BookPublicMeetingJSONRequestBody
	req.Booker.Name = "Wieder Da"
	req.Booker.Email = "geloescht@betroffene.example"
	rec := httptest.NewRecorder()
	_, ok := h.ensureBooker(rec, httptest.NewRequest(http.MethodPost, "/public/booking/x", nil), req)
	body := rec.Body.String()
	if ok || rec.Code != http.StatusUnprocessableEntity || !strings.Contains(body, "booker.email") {
		t.Fatalf("the door answered %d %s, want 422 naming booker.email", rec.Code, body)
	}
	if strings.Contains(body, "erase") || strings.Contains(body, storekit.ErasedIdentifierCode) {
		t.Errorf("an anonymous caller learns the address was erased: %s", body)
	}
}
