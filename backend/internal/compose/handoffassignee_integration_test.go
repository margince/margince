// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/compose/memberhandoff"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestPrivateProspectOwnerCanRepairAHandoffWithoutMemberAdministration(t *testing.T) {
	e := integration.Setup(t)
	memberManagerFor(t, e)
	ctx := e.Admin()
	if err := e.DB().Tx(ctx, func(tx pgx.Tx) error {
		members, err := identity.MemberPrincipalsTx(ctx, tx, []ids.UUID{e.Rep2})
		if err != nil {
			return err
		}
		actor, ok := members[e.Rep2]
		if !ok {
			t.Fatal("fixture member is inactive")
		}
		ctx = principal.WithActor(ctx, actor)
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	contact := e.SeedContact(t, "Private prospect", &e.Rep2)
	visibility := "owner"
	if _, err := e.Contacts.UpdateContact(ctx, ids.From[ids.ContactKind](contact), contacts.UpdateContactInput{Visibility: &visibility}); err != nil {
		t.Fatal(err)
	}
	handoff, err := e.Contacts.SubmitHandoff(ctx, contacts.NewSDRHandoff{ContactID: &contact, AssignedTo: &e.Rep1})
	if err != nil {
		t.Fatal(err)
	}
	server := Server{handoffHandlers: memberhandoff.NewHandlers(e.DB())}
	read := func(ctx context.Context) *httptest.ResponseRecorder {
		response := httptest.NewRecorder()
		server.GetHandoffAssignee(response, httptest.NewRequestWithContext(ctx, http.MethodGet, "/", nil), crmcontracts.Id(handoff))
		return response
	}
	if response := read(e.Admin()); response.Code != http.StatusNotFound {
		t.Fatalf("administrator read private prospect: %d %s", response.Code, response.Body.String())
	}
	response := read(ctx)
	if response.Code != http.StatusOK {
		t.Fatalf("owner read: %d %s", response.Code, response.Body.String())
	}
	var current crmcontracts.HandoffAssignee
	if err := json.Unmarshal(response.Body.Bytes(), &current); err != nil {
		t.Fatal(err)
	}
	if !current.CanReassign || current.Version != 1 {
		t.Fatalf("current assignment = %+v", current)
	}
	patch := func(receiver ids.UUID, version int64) *httptest.ResponseRecorder {
		response := httptest.NewRecorder()
		body := fmt.Sprintf(`{"assigned_to":%q,"version":%d}`, receiver.String(), version)
		server.ReassignHandoff(response, httptest.NewRequestWithContext(ctx, http.MethodPatch, "/", strings.NewReader(body)), crmcontracts.Id(handoff))
		return response
	}
	if response := patch(e.Rep1, 1); response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("receiver without private access: %d %s", response.Code, response.Body.String())
	}
	if response := patch(e.Rep2, 1); response.Code != http.StatusOK {
		t.Fatalf("owner repair: %d %s", response.Code, response.Body.String())
	}
	if response := patch(e.Rep1, 1); response.Code != http.StatusConflict {
		t.Fatalf("stale version: %d %s", response.Code, response.Body.String())
	}
	if got := e.WsScalar(t, `SELECT assigned_to::text FROM sdr_handoff WHERE id=$1`, handoff); got != e.Rep2.String() {
		t.Fatal("handoff was not repaired")
	}
	if response := read(e.Admin()); response.Code != http.StatusNotFound {
		t.Fatal("reassignment disclosed the private prospect")
	}
}
