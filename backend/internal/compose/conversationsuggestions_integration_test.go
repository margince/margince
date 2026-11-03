// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/capture/mailmap"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

func TestConversationSuggestionsUseTheRealContactAndLeadRoutes(t *testing.T) {
	for _, kind := range []datasource.EntityType{datasource.EntityContact, datasource.EntityLead} {
		t.Run(string(kind), func(t *testing.T) {
			e := integration.Setup(t)
			var anchor ids.UUID
			var leadCreated time.Time
			if kind == datasource.EntityContact {
				contact, err := e.Contacts.CreateContact(e.Admin(), contacts.CreateContactInput{FullName: "Alice Buyer", Source: "manual", Emails: []contacts.ContactEmailInput{{Email: "alice@buyer.test", EmailType: "work", IsPrimary: true}}})
				if err != nil {
					t.Fatal(err)
				}
				anchor = ids.UUID(contact.Id)
			} else {
				lead, _, err := e.Contacts.CreateLead(e.Admin(), contacts.CreateLeadInput{FullName: new("Alice Buyer"), Email: new("alice@buyer.test"), Source: "manual"})
				if err != nil {
					t.Fatal(err)
				}
				anchor = ids.UUID(lead.Id)
				leadCreated = lead.CreatedAt
			}
			rec := partiesMailRecord(t, e, partyMailProof{outbound: false, sentByOwner: false}, "Bob Buyer <bob@buyer.test>, Casey Buyer <casey@buyer.test>", "", "Could we discuss the product requirements?")
			owner := seatAddress(t, e, e.AdminUser)
			raw := []byte(strings.Replace(string(rec.Raw), "To: "+owner, "To: "+owner+", Taylor Buyer <taylor@buyer.test>", 1))
			if kind == datasource.EntityLead {
				raw = []byte(strings.ReplaceAll(string(raw), "Mon, 5 Oct 2026 10:00:00 +0000", leadCreated.Add(time.Minute).Format(time.RFC1123Z)))
			}
			parsed, err := mailmap.Parse(raw, owner)
			if err != nil {
				t.Fatal(err)
			}
			rec = parsed.ToRecord("gmail", raw)
			actor, ok := principal.Actor(partyOwnerContext(t, e))
			if !ok {
				t.Fatal("missing fixture actor")
			}
			actor.Type = principal.PrincipalConnector
			actor.ID = "connector:gmail"
			actor.OnBehalfOf = e.AdminUser
			ref, err := newCaptureSink(e.Pool, CaptureConfig{}).Upsert(principal.WithActor(e.Admin(), actor), rec)
			if err != nil {
				t.Fatal(err)
			}
			if e.WsCount(t, `SELECT count(*) FROM contact_email WHERE lower(email)=ANY($1::text[])`, []string{"bob@buyer.test", "casey@buyer.test", "taylor@buyer.test"}) != 0 {
				t.Fatal("inbound copies were automatically admitted")
			}
			server := newServer(e.Pool, quietLog(), authHandlers{}, dealsHandlers{})
			handler := crmcontracts.Handler(server)
			read := func() crmcontracts.ConversationSuggestionListResponse {
				t.Helper()
				response := httptest.NewRecorder()
				request := httptest.NewRequest(http.MethodGet, "/"+string(kind)+"s/"+anchor.String()+"/conversation-suggestions", nil).WithContext(e.Admin())
				handler.ServeHTTP(response, request)
				if response.Code != http.StatusOK {
					t.Fatalf("suggestion read: %d %s", response.Code, response.Body.String())
				}
				var out crmcontracts.ConversationSuggestionListResponse
				if err := json.Unmarshal(response.Body.Bytes(), &out); err != nil {
					t.Fatal(err)
				}
				return out
			}
			before := e.WsCount(t, `SELECT count(*) FROM audit_log`)
			out := read()
			var addresses []string
			for _, suggestion := range out.Data {
				addresses = append(addresses, suggestion.Address)
				if ids.UUID(suggestion.ActivityId) != ref.ID {
					t.Fatal("suggestion lost its source")
				}
			}
			slices.Sort(addresses)
			if !slices.Equal(addresses, []string{"bob@buyer.test", "casey@buyer.test", "taylor@buyer.test"}) {
				t.Fatalf("suggestions=%v", addresses)
			}
			if e.WsCount(t, `SELECT count(*) FROM audit_log`) != before {
				t.Fatal("suggestion read mutated the record")
			}
			if _, err := capture.NewExclusionStore(InstallationDB(e.Pool)).Add(e.Admin(), capture.ExclusionScopeUser, capture.ExclusionKindAddress, "casey@buyer.test"); err != nil {
				t.Fatal(err)
			}
			if out := read(); len(out.Data) != 0 {
				t.Fatalf("a newly excluded source still offered recipients: %+v", out)
			}
		})
	}
}
