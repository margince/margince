// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package messengeraccounts_test

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/compose/messengeraccounts"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/assignments"
	"github.com/margince/margince/backend/internal/modules/chatlinks"
	"github.com/margince/margince/backend/internal/modules/chatlinks/messaging"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

type rosterDirectory struct{ messaging.Client }

func (rosterDirectory) Identity(context.Context) (messaging.Identity, error) {
	return messaging.Identity{Workspace: "TEAM", Member: messaging.Member{ID: "ADMIN", Name: "Sender", Active: true}}, nil
}

func (rosterDirectory) ResolvePerson(_ context.Context, email string) (messaging.Member, error) {
	return messaging.Member{ID: email, Email: email, Active: true}, nil
}

func TestMessengerRecordTeamUsesOwnerAndActualAssignmentsRatherThanEveryReader(t *testing.T) {
	e := integration.Setup(t)
	e.GrantRole(t, e.AdminUser, "admin")
	ctx := integration.OnSeat(e.Admin(), principal.SeatFull)
	company := e.SeedCompany(t, "Pilot account", &e.Rep1)
	role, err := ids.Parse(e.WsScalar(t, `SELECT id::text FROM record_role WHERE key='account_manager'`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := assignments.NewStore(e.DB()).CreateAssignment(e.Admin(), assignments.CreateAssignmentInput{RecordType: crmcontracts.AssignmentRecordTypeCompany, RecordID: company, SubjectKind: crmcontracts.AssignmentSubjectKindUser, SubjectID: e.Rep2, RoleID: role}); err != nil {
		t.Fatal(err)
	}
	vault := keyvault.NewMemory()
	store := chatlinks.NewStore(e.DB(), nil, nil, nil).WithMessenger(func() keyvault.Vault { return vault }, func(context.Context, chatlinks.MemberGrant, []byte) (messaging.Client, error) {
		return rosterDirectory{}, nil
	}).WithMessengerRoster(messengeraccounts.RecordRoster).WithMemberAuthority(messengeraccounts.MemberAuthority)
	if _, err := store.ConnectMember(ctx, chatlinks.MemberGrant{Provider: "slack", Workspace: "TEAM", Member: "ADMIN"}, []byte("synthetic-member-token")); err != nil {
		t.Fatal(err)
	}
	roster, err := store.MessengerTeam(ctx, "company", company, "slack")
	if err != nil {
		t.Fatal(err)
	}
	if len(roster) != 2 {
		t.Fatalf("actual owner plus assigned person became %d recipients", len(roster))
	}
	seen := map[ids.UUID]bool{}
	for _, person := range roster {
		seen[person.Person.ID] = true
		if person.State != "matched" || person.Member == "" {
			t.Fatalf("expected matched actual team: %+v", person)
		}
	}
	if !seen[e.Rep1] || !seen[e.Rep2] || seen[e.AdminUser] {
		t.Fatal("record readers were substituted for the actual team")
	}
	if n := e.WsScalar(t, "SELECT count(*)::text FROM chat_link"); n != "0" {
		t.Fatal("resolving the team created a channel link")
	}
}
