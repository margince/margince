// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// Every door that names a seat refuses a name past displayNameMaxRunes before
// it reaches the service. The handlers hold no service, so one that reached it
// would panic rather than pass.
func TestEverySeatNameDoorRefusesPastTheSeatNameLimit(t *testing.T) {
	h := Handlers{}
	doors := map[string]struct {
		body  map[string]string
		serve func(http.ResponseWriter, *http.Request)
	}{
		"invite": {
			map[string]string{"email": "seat@acme.test", "role": "rep"},
			func(w http.ResponseWriter, r *http.Request) { h.InviteUser(w, r, crmcontracts.InviteUserParams{}) },
		},
		"former member":  {map[string]string{"email": "seat@acme.test"}, h.CreateFormerMember},
		"service member": {map[string]string{"extension": "feed"}, h.DeclareServiceMember},
	}
	admin := seatWithGrant(principal.ObjectGrant{Create: true, Read: true})
	for door, d := range doors {
		for _, runes := range []int{displayNameMaxRunes + 1, 255} {
			d.body["display_name"] = strings.Repeat("組", runes)
			body, err := json.Marshal(d.body)
			if err != nil {
				t.Fatal(err)
			}
			req := httptest.NewRequest(http.MethodPost, "/users", strings.NewReader(string(body)))
			rec := httptest.NewRecorder()
			d.serve(rec, req.WithContext(withIdentity(context.Background(), admin)))
			if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), `"field":"display_name"`) {
				t.Errorf("%s with a %d-character name = %d %s, want 422 on display_name", door, runes, rec.Code, rec.Body.String())
			}
		}
	}
}
