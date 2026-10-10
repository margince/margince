// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealrooms

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The contract makes expires_at required so that clearing an expiry is
// something a caller asked for. A body without the key is refused before the
// store, which a zero Handlers has none of, is reached.
func TestAnExpiryBodyThatOmitsExpiresAtIsRefusedBeforeTheStore(t *testing.T) {
	r := httptest.NewRequest(http.MethodPut, "/v1/deal-rooms/x/expiry", strings.NewReader(`{}`))
	r.Header.Set("If-Match", "3")
	w := httptest.NewRecorder()

	Handlers{}.SetDealRoomExpiry(w, r, crmcontracts.Id(ids.NewV7()), crmcontracts.SetDealRoomExpiryParams{})

	if w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), "expires_at") {
		t.Errorf("got %d %s, want a 422 naming expires_at", w.Code, w.Body.String())
	}
}
