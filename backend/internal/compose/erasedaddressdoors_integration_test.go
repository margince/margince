// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// Every door an address is typed into refuses an erased subject's
// address and accepts a clean one. The subject is erased through the real
// eraser, so the suppression list holds what production writes.

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const (
	erasedAddress = "geloescht@erased-doors.example"
	cleanAddress  = "neu@erased-doors.example"
)

// erasedSubject creates a contact holding erasedAddress and erases it.
func erasedSubject(t *testing.T) *integration.Env {
	t.Helper()
	e := integration.Setup(t)
	contact, err := e.Contacts.CreateContact(e.Admin(), contacts.CreateContactInput{
		FullName: "Die Betroffene",
		Emails:   []contacts.ContactEmailInput{{Email: erasedAddress, EmailType: "work", IsPrimary: true}},
		Source:   "manual",
	})
	if err != nil {
		t.Fatalf("creating the subject: %v", err)
	}
	if err := privacy.NewEraser(e.DB()).EraseContact(e.Admin(), ids.UUID(contact.Id), "Art. 17 request"); err != nil {
		t.Fatalf("erasing the subject: %v", err)
	}
	return e
}

// createContactOverHTTP posts a manual create through the handlers the server
// mounts and answers the status and body.
func createContactOverHTTP(e *integration.Env, address string) (int, string) {
	body := `{"full_name":"Wieder Da","source":"manual","emails":[{"email":"` + address + `","email_type":"work","is_primary":true}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/contacts", strings.NewReader(body)).WithContext(e.Admin())
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	newContactsHandlers(e.Pool).CreateContact(rec, req, crmcontracts.CreateContactParams{})
	return rec.Code, rec.Body.String()
}

func TestTheManualCreateRefusesAnErasedAddressWithAStableCode(t *testing.T) {
	e := erasedSubject(t)
	status, body := createContactOverHTTP(e, erasedAddress)
	if status != http.StatusUnprocessableEntity || !strings.Contains(body, `"`+storekit.ErasedIdentifierCode+`"`) {
		t.Fatalf("the create answered %d %s, want 422 with code %s", status, body, storekit.ErasedIdentifierCode)
	}
	if strings.Contains(body, "Die Betroffene") {
		t.Errorf("the refusal names the erased record: %s", body)
	}
	if status, body := createContactOverHTTP(e, cleanAddress); status != http.StatusCreated {
		t.Fatalf("a clean address answered %d %s, want 201", status, body)
	}
}

func TestAHandMadeLeadRefusesAnErasedAddress(t *testing.T) {
	e := erasedSubject(t)
	name := "Wieder Da"
	for address, wantRefused := range map[string]bool{erasedAddress: true, cleanAddress: false} {
		_, _, err := e.Contacts.CreateLead(e.Admin(), contacts.CreateLeadInput{FullName: &name, Email: &address})
		_, refused := errors.AsType[*storekit.ErasedIdentifierError](err)
		if refused != wantRefused || (!wantRefused && err != nil) {
			t.Errorf("creating a lead with %s answered %v, want refused=%v", address, err, wantRefused)
		}
	}
}

func TestAnUpdateCannotGiveALeadAnErasedAddress(t *testing.T) {
	e := erasedSubject(t)
	name, address := "Wieder Da", cleanAddress
	lead, _, err := e.Contacts.CreateLead(e.Admin(), contacts.CreateLeadInput{FullName: &name, Email: &address})
	if err != nil {
		t.Fatalf("creating the lead: %v", err)
	}
	erased := erasedAddress
	_, err = e.Contacts.UpdateLead(e.Admin(), ids.From[ids.LeadKind](ids.UUID(lead.Id)), contacts.UpdateLeadInput{Email: &erased})
	if _, refused := errors.AsType[*storekit.ErasedIdentifierError](err); !refused {
		t.Fatalf("moving the lead onto the erased address answered %v, want the erasure refusal", err)
	}
}

func TestACSVImportSkipsAnErasedAddressAndLandsTheRest(t *testing.T) {
	for _, object := range []string{migration.ObjectContact, migration.ObjectLead} {
		t.Run(object, func(t *testing.T) {
			e := erasedSubject(t)
			ctx := importingAdmin(e)
			run, err := migration.NewRunStore(e.DB()).CreateDraftRun(ctx, migration.CreateDraftRunInput{
				SourceRef: "ws/import/" + ids.NewV7().String(), Source: "import_api", FileName: object + ".csv",
				Profile: json.RawMessage(`{"files":[{"name":"` + object + `.csv","row_count":2}]}`),
				Mapping: migration.RunMapping{Object: object, Fields: map[string]string{"Email": "email"}, SourceKey: "Email"},
			})
			if err != nil {
				t.Fatalf("opening the import run: %v", err)
			}
			w := newCSVWriters(e.DB(), run.ID, migration.PartMapping{Object: object},
				runOptions{}, &linkResolver{sourceSystem: "csv:erased"}, importColumns{})
			create := w.createContact
			if object == migration.ObjectLead {
				create = w.createLead
			}
			for i, address := range []string{erasedAddress, cleanAddress} {
				row := migration.Row{
					ExternalID: object + "-row-" + string(rune('a'+i)),
					Fields:     map[string]any{"full_name": "Wieder Da", fieldEmail: address},
				}
				result, err := create(ctx, row, nil)
				if err != nil {
					t.Fatalf("row %s failed the import: %v", address, err)
				}
				erased := result.Skipped && result.SkipWhy.Code == migration.ReasonAddressErased
				if erased != (address == erasedAddress) || (address == cleanAddress && !result.Created) {
					t.Errorf("row %s landed %+v", address, result)
				}
			}
		})
	}
}

// importingAdmin is an admin who may also run an import.
func importingAdmin(e *integration.Env) context.Context {
	perms := integration.AdminPerms
	perms.Objects = maps.Clone(perms.Objects)
	perms.Objects["import_run"] = principal.ObjectGrant{Create: true, Read: true, Update: true}
	return e.As(e.AdminUser, nil, perms)
}

func TestAVCardImportSkipsAnErasedAddressAndLandsTheRest(t *testing.T) {
	e := erasedSubject(t)
	results, err := e.Contacts.ImportVCards(e.Admin(), []contacts.VCardEntry{
		{FullName: "Wieder Da", Emails: []contacts.VCardChannel{{Value: erasedAddress, Kind: "work"}}},
		{FullName: "Ganz Neu", Emails: []contacts.VCardChannel{{Value: cleanAddress, Kind: "work"}}},
	})
	if err != nil {
		t.Fatalf("importing the cards: %v", err)
	}
	if len(results) != 2 || results[0].Outcome != contacts.VCardSkipped || results[1].Outcome != contacts.VCardCreated {
		t.Fatalf("the cards landed %+v, want the erased one skipped and the clean one created", results)
	}
	if !strings.Contains(results[0].Reason, "erasure list") {
		t.Errorf("the skip reason %q does not say why", results[0].Reason)
	}
}

func TestAContactUpdateCannotAddAnErasedAddress(t *testing.T) {
	e := erasedSubject(t)
	contact, err := e.Contacts.CreateContact(e.Admin(), contacts.CreateContactInput{
		FullName: "Wieder Da",
		Emails:   []contacts.ContactEmailInput{{Email: cleanAddress, EmailType: "work", IsPrimary: true}},
		Source:   "manual",
	})
	if err != nil {
		t.Fatalf("creating the contact: %v", err)
	}
	_, err = e.Contacts.UpdateContact(e.Admin(), ids.From[ids.ContactKind](ids.UUID(contact.Id)), contacts.UpdateContactInput{
		Emails: []contacts.ContactEmailInput{
			{Email: cleanAddress, EmailType: "work", IsPrimary: true},
			{Email: erasedAddress, EmailType: "home", Position: 1},
		},
	})
	if _, refused := errors.AsType[*storekit.ErasedIdentifierError](err); !refused {
		t.Fatalf("adding the erased address answered %v, want the erasure refusal", err)
	}
}

// The erased address second in the list: a check that reads only the first
// address would let it through.
func TestACreateRefusesAnErasedAddressThatIsNotTheFirst(t *testing.T) {
	e := erasedSubject(t)
	_, err := e.Contacts.CreateContact(e.Admin(), contacts.CreateContactInput{
		FullName: "Wieder Da",
		Emails: []contacts.ContactEmailInput{
			{Email: cleanAddress, EmailType: "work", IsPrimary: true},
			{Email: erasedAddress, EmailType: "home", Position: 1},
		},
		Source: "manual",
	})
	if _, refused := errors.AsType[*storekit.ErasedIdentifierError](err); !refused {
		t.Fatalf("a create carrying the erased address second answered %v, want the erasure refusal", err)
	}
}
