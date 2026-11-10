// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// An invite may name the legal entity the new member works for. It is written
// in the invite's transaction, through the assignment route's own writer, so
// a refused entity leaves no seat behind.

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

type entityMemberWire struct {
	UserID string `json:"user_id"`
}

func inviteEnv(t *testing.T) (*apptest.AppEnv, versionedWire) {
	t.Helper()
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	e.DescribeCompany(t)
	var entity versionedWire
	body := map[string]any{"display_name": "Acme Wien", "legal_name": "Acme Wien GmbH", "country": "at"}
	if status := e.Call(t, http.MethodPost, "/v1/legal-entities", body, nil, &entity); status != http.StatusCreated {
		t.Fatalf("creating the legal entity -> %d", status)
	}
	return e, entity
}

// inviteInto is an invite body; an empty entity names none.
func inviteInto(email, entity string) map[string]any {
	body := seatBody(email)
	if entity != "" {
		body["legal_entity_id"] = entity
	}
	return body
}

func entityMembers(t *testing.T, e *apptest.AppEnv, entityID string) []entityMemberWire {
	t.Helper()
	var page struct {
		Data []entityMemberWire `json:"data"`
	}
	if status := e.Call(t, http.MethodGet, "/v1/legal-entities/"+entityID+"/members", nil, nil, &page); status != http.StatusOK {
		t.Fatalf("listing the members of %s -> %d", entityID, status)
	}
	return page.Data
}

func TestAnInviteNamingAnEntityPlacesTheMemberThere(t *testing.T) {
	e, entity := inviteEnv(t)
	var invited struct {
		ID string `json:"id"`
	}
	if status := e.Call(t, http.MethodPost, "/v1/users", inviteInto("wien@invite.test", entity.ID), nil, &invited); status != http.StatusCreated {
		t.Fatalf("inviting into Acme Wien -> %d, want 201", status)
	}
	members := entityMembers(t, e, entity.ID)
	if len(members) != 1 || members[0].UserID != invited.ID {
		t.Fatalf("Acme Wien lists %+v, want only the invited member %s", members, invited.ID)
	}
	var audited int
	if err := e.Owner.QueryRow(context.Background(),
		`SELECT count(*) FROM audit_log WHERE action = 'assign' AND entity_type = 'legal_entity' AND entity_id = $1`,
		entity.ID).Scan(&audited); err != nil {
		t.Fatal(err)
	}
	if audited != 1 {
		t.Errorf("the placement wrote %d assignment audit rows, want 1", audited)
	}
	var announced int
	if err := e.Owner.QueryRow(context.Background(),
		`SELECT count(*) FROM event_outbox WHERE envelope->>'type' = 'legal_entity.member_added'
		    AND envelope->'entity'->>'id' = $1 AND envelope::text LIKE '%' || $2::text || '%'`,
		entity.ID, invited.ID).Scan(&announced); err != nil {
		t.Fatal(err)
	}
	if announced != 1 {
		t.Errorf("the placement queued %d legal_entity.member_added events naming the member, want 1", announced)
	}
}

func TestAnInviteNamingNoEntityLeavesTheMemberWithTheDefault(t *testing.T) {
	e, entity := inviteEnv(t)
	if status := e.Call(t, http.MethodPost, "/v1/users", inviteInto("default@invite.test", ""), nil, nil); status != http.StatusCreated {
		t.Fatalf("inviting with no entity -> %d, want 201", status)
	}
	if members := entityMembers(t, e, entity.ID); len(members) != 0 {
		t.Errorf("Acme Wien lists %+v, want nobody", members)
	}
}

func TestAnInviteNamingAnEntityThatCannotTakeMembersAddsNoSeat(t *testing.T) {
	e, entity := inviteEnv(t)
	var archived versionedWire
	if status := e.Call(t, http.MethodPost, "/v1/legal-entities/"+entity.ID+"/archive", nil,
		map[string]string{"If-Match": fmt.Sprint(entity.Version)}, &archived); status != http.StatusOK {
		t.Fatalf("archiving Acme Wien -> %d", status)
	}
	for name, id := range map[string]string{
		"an unknown entity":  "0190a000-0000-7000-8000-000000000001",
		"an archived entity": entity.ID,
	} {
		email := "refused-" + id + "@invite.test"
		var refusal problemBody
		if status := e.Call(t, http.MethodPost, "/v1/users", inviteInto(email, id), nil, &refusal); status != http.StatusUnprocessableEntity {
			t.Errorf("inviting into %s -> %d, want 422", name, status)
		}
		if faults := refusal.Details.Errors; len(faults) != 1 ||
			faults[0].Field != "legal_entity_id" || faults[0].Code != "unknown_legal_entity" {
			t.Errorf("inviting into %s answered %+v, want one legal_entity_id/unknown_legal_entity", name, faults)
		}
		if n := seatRowsFor(t, e, email); n != 0 {
			t.Errorf("inviting into %s was refused yet wrote %d seat(s)", name, n)
		}
	}
}

// Placing a member is assigning them, so an inviter who may add seats but
// lacks either assignment grant is refused the whole invite.
func TestAnInviteNamingAnEntityAsksBothAssignmentGrants(t *testing.T) {
	for _, revoked := range []string{"legal_entity", "user_admin"} {
		t.Run(revoked, func(t *testing.T) {
			e, entity := inviteEnv(t)
			if _, err := e.Owner.Exec(context.Background(),
				`UPDATE role SET permissions = jsonb_set(permissions, ARRAY['objects', $1, 'update'], 'false') WHERE key = 'admin'`,
				revoked); err != nil {
				t.Fatal(err)
			}
			email := "ungranted-" + revoked + "@invite.test"
			if status := e.Call(t, http.MethodPost, "/v1/users", inviteInto(email, entity.ID), nil, nil); status != http.StatusForbidden {
				t.Errorf("inviting into an entity without %s.update -> %d, want 403", revoked, status)
			}
			if n := seatRowsFor(t, e, email); n != 0 {
				t.Errorf("the refused invite wrote %d seat(s)", n)
			}
		})
	}
}

// The member edit takes the entity too, so the edit dialog saves in one
// request; an unknown entity refuses the whole edit.
func TestAMemberEditMovesTheMemberToTheEntityNamed(t *testing.T) {
	e, entity := inviteEnv(t)
	var invited struct {
		ID          string `json:"id"`
		DisplayName string `json:"display_name"`
		Email       string `json:"email"`
		UpdatedAt   string `json:"updated_at"`
	}
	if status := e.Call(t, http.MethodPost, "/v1/users", inviteInto("edited@invite.test", ""), nil, &invited); status != http.StatusCreated {
		t.Fatalf("inviting -> %d", status)
	}
	edit := func(name, entityID string) int {
		return e.Call(t, http.MethodPatch, "/v1/users/"+invited.ID, map[string]any{
			"display_name": name, "email": invited.Email,
			"expected_updated_at": invited.UpdatedAt, "legal_entity_id": entityID,
		}, nil, nil)
	}
	if status := edit("Renamed", "0190a000-0000-7000-8000-000000000001"); status != http.StatusUnprocessableEntity {
		t.Errorf("an edit naming an unknown entity -> %d, want 422", status)
	}
	var name string
	if err := e.Owner.QueryRow(context.Background(), `SELECT display_name FROM app_user WHERE id = $1`, invited.ID).Scan(&name); err != nil {
		t.Fatal(err)
	}
	if name != invited.DisplayName {
		t.Errorf("the refused edit renamed the member to %q", name)
	}
	if status := edit(invited.DisplayName, entity.ID); status != http.StatusNoContent {
		t.Fatalf("an edit naming Acme Wien -> %d, want 204", status)
	}
	if members := entityMembers(t, e, entity.ID); len(members) != 1 || members[0].UserID != invited.ID {
		t.Errorf("Acme Wien lists %+v after the edit, want the member", members)
	}
}
