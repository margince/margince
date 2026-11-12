// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"net/http"
	"net/http/httptest"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/config/configtest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

var caseAbsenceRepPerms = principal.Permissions{
	RoleKeys: []string{"rep"},
	Objects: map[string]principal.ObjectGrant{
		"contact":               {Read: true},
		"installation_settings": {Read: true},
	},
	RowScope: principal.RowScopeOwn,
}

// A contact screen asks for the case of every contact it opens, and reads a
// 404 as "no duplicates". A case the reader may not see must answer that same
// 404, byte for byte, or the answer tells them a hidden duplicate exists.
func TestAHiddenContactCaseAnswersLikeNoCase(t *testing.T) {
	e := Setup(t)
	settings := configtest.StoredOver(t, e.Pool, contacts.ConfigSettings())
	store := contacts.NewStore(e.DB()).WithSettings(settings)
	handlers := contacts.NewHandlers(e.DB()).WithSettings(settings)
	seed := func(owner ids.UUID, name, phone string) ids.UUID {
		t.Helper()
		user := ids.From[ids.UserKind](owner)
		created, err := store.CreateContact(e.Admin(), contacts.CreateContactInput{
			FullName: name, Source: "manual", OwnerID: &user,
			Phones: []contacts.ContactPhoneInput{{Phone: phone, PhoneType: "work", IsPrimary: true, Position: 1}},
		})
		if err != nil {
			t.Fatalf("creating a contact: %v", err)
		}
		return ids.UUID(created.Id)
	}
	alone := seed(e.Rep1, "Ada Lovelace", "+4930555901")
	visible := seed(e.Rep1, "Grace Hopper", "+4930555902")
	seed(e.Rep1, "Grace Hopper", "+4930555902")
	hidden := seed(e.Rep2, "Edsger Dijkstra", "+4930555903")
	seed(e.Rep2, "Edsger Dijkstra", "+4930555903")
	if got, err := store.GetContactCase(e.Admin(), hidden); err != nil || len(got.Members) != 2 {
		t.Fatalf("the admin reads the hidden case as %d members (err %v), want the seeded pair", len(got.Members), err)
	}
	e.WsExec(t, `UPDATE contact SET visibility = 'owner' WHERE owner_id = $1`, e.Rep2)

	rep := e.As(e.Rep1, nil, caseAbsenceRepPerms)
	get := func(contact ids.UUID) *httptest.ResponseRecorder {
		t.Helper()
		w := httptest.NewRecorder()
		r := httptest.NewRequestWithContext(rep, http.MethodGet, "/v1/dedupe/contact-cases/"+contact.String(), nil)
		handlers.GetContactCase(w, r, openapi_types.UUID(contact))
		return w
	}
	if w := get(visible); w.Code != http.StatusOK {
		t.Fatalf("the rep's own case answers %d, want 200: %s", w.Code, w.Body)
	}
	none, withheld := get(alone), get(hidden)
	if none.Code != http.StatusNotFound {
		t.Fatalf("a contact in no case answers %d, want 404: %s", none.Code, none.Body)
	}
	if withheld.Code != none.Code || withheld.Body.String() != none.Body.String() {
		t.Errorf("a hidden case answers %d %s, want what no case answers: %d %s",
			withheld.Code, withheld.Body, none.Code, none.Body)
	}
}
