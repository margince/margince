// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contactdraft_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/contact360"
	"github.com/margince/margince/backend/internal/compose/contactdraft"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

type pathContact struct{ contact crmcontracts.Contact }

func (v pathContact) AssembleScoped(context.Context, ids.ContactID, contact360.AssembleOptions) (crmcontracts.Contact360, error) {
	return crmcontracts.Contact360{Contact: v.contact}, nil
}

// readableContacts answers the contacts it holds and not-found for any other,
// the answer a row-scope miss gives.
type readableContacts map[ids.ContactID]crmcontracts.Contact

func (r readableContacts) GetContact(_ context.Context, id ids.ContactID, _ storekit.ArchivedFilter) (crmcontracts.Contact, error) {
	contact, ok := r[id]
	if !ok {
		return crmcontracts.Contact{}, apperrors.ErrNotFound
	}
	return contact, nil
}

func contactAt(address string) (ids.ContactID, crmcontracts.Contact) {
	id := ids.From[ids.ContactKind](ids.NewV7())
	email := openapi_types.Email(address)
	first := "Ada"
	return id, crmcontracts.Contact{
		Id: openapi_types.UUID(id.UUID), FullName: "Ada " + address, FirstName: &first, PrimaryEmail: &email,
	}
}

func draftAlsoTo(t *testing.T, readable readableContacts, also ...ids.ContactID) *httptest.ResponseRecorder {
	t.Helper()
	pathID, path := contactAt("lead@customer.test")
	svc := contactdraft.NewService(pathContact{path}, nil).WithCoRecipients(readable)
	listed := make([]string, 0, len(also))
	for _, id := range also {
		listed = append(listed, `"`+id.String()+`"`)
	}
	seat := ids.NewV7()
	human := principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + seat.String(), UserID: seat,
	})
	req := httptest.NewRequestWithContext(human, http.MethodPost, "/contacts/x/draft-email",
		strings.NewReader(`{"also_to":[`+strings.Join(listed, ",")+`]}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	contactdraft.NewHandlers(svc).DraftContactEmail(rec, req, openapi_types.UUID(pathID.UUID))
	return rec
}

func TestADraftToSeveralContactsAddressesEachOfThem(t *testing.T) {
	t.Parallel()
	first, firstContact := contactAt("ana@customer.test")
	second, secondContact := contactAt("ben@customer.test")
	rec := draftAlsoTo(t, readableContacts{first: firstContact, second: secondContact}, first, second, first)
	if rec.Code != http.StatusOK {
		t.Fatalf("drafting to three contacts answered %d: %s", rec.Code, rec.Body.String())
	}
	var draft crmcontracts.CompanyEmailDraft
	if err := json.Unmarshal(rec.Body.Bytes(), &draft); err != nil {
		t.Fatalf("decoding the draft: %v", err)
	}
	want := []openapi_types.Email{"lead@customer.test", "ana@customer.test", "ben@customer.test"}
	if draft.To == nil || !slices.Equal(*draft.To, want) {
		t.Fatalf("to = %v, want every contact once, the path contact first: %v", draft.To, want)
	}
	if !strings.HasPrefix(draft.Body, "Hello all,") {
		t.Errorf("body opens %q, want the group greeting rather than one recipient's name", draft.Body)
	}
}

func TestADraftNamingAContactTheCallerCannotReadIsRefused(t *testing.T) {
	t.Parallel()
	readable, readableContact := contactAt("ana@customer.test")
	hidden, _ := contactAt("hidden@customer.test")
	rec := draftAlsoTo(t, readableContacts{readable: readableContact}, readable, hidden)
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "recipient_not_on_file") {
		t.Fatalf("drafting to an unreadable contact answered %d: %s, want 422 recipient_not_on_file",
			rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "hidden@customer.test") {
		t.Errorf("the refusal disclosed the unreadable contact's address: %s", rec.Body.String())
	}
}
