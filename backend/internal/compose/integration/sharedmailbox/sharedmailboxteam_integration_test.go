// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package sharedmailbox

// A team that reads a shared mailbox. An agent may not grow it. An admin who
// grows it is audited with the mailbox, and it cannot be archived.

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const opsMailbox = "ops@fable.test"

// readerTeamService is identity with capture's seam, Team1 reading ops@.
func readerTeamService(t *testing.T, e *integration.Env) *identity.Service {
	t.Helper()
	integration.ConnectSharedMailbox(t, integration.SharedMailboxRegistry(t, e.Pool, "imap"), e.WS, integration.SharedMailbox{
		Provider: "imap", Address: opsMailbox, ReaderTeam: e.Team1, ConnectedBy: e.AdminUser, Posture: "held",
	})
	return compose.TeamServiceForTest(e.Pool)
}

// agentOfAdmin is the admin's own agent, acting on their passport.
func agentOfAdmin(e *integration.Env) context.Context {
	agent, ok := principal.Actor(e.Admin())
	if !ok {
		return e.Admin()
	}
	agent.Type, agent.OnBehalfOf, agent.PassportID = principal.PrincipalAgent, e.AdminUser, ids.NewV7()
	return principal.WithActor(e.Admin(), agent)
}

func TestAnAgentCannotAddAReaderToATeamThatReadsASharedMailbox(t *testing.T) {
	e := integration.Setup(t)
	service := readerTeamService(t, e)
	before := teamMembersOf(t, e.Team1)

	err := service.SetTeamMember(agentOfAdmin(e), adminIdentity(e), e.Team1, e.Rep3, true)
	if !errors.Is(err, apperrors.ErrPermissionDenied) || !strings.Contains(err.Error(), opsMailbox) {
		t.Fatalf("an agent adding a reader got %v, want a refusal naming %s", err, opsMailbox)
	}
	if after := teamMembersOf(t, e.Team1); after != before {
		t.Fatalf("the refused add still changed the team: %d members, was %d", after, before)
	}
}

// agentOfAdmin is the admin's own agent, acting on their passport.

func TestAnAgentIsRefusedEvenWhenTheTeamsSharedMailboxIsDisconnected(t *testing.T) {
	e := integration.Setup(t)
	service := readerTeamService(t, e)
	if _, err := integration.OwnerConn(t).Exec(context.Background(),
		`UPDATE capture_connection SET status = 'disconnected', archived_at = now() WHERE account_label = $1`, opsMailbox); err != nil {
		t.Fatal(err)
	}
	err := service.SetTeamMember(agentOfAdmin(e), adminIdentity(e), e.Team1, e.Rep3, true)
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("an agent adding a reader of a disconnected mailbox's history got %v, want a refusal", err)
	}
}

func TestAnAdminAddingAReaderIsAuditedWithTheMailboxTheyCanNowRead(t *testing.T) {
	e := integration.Setup(t)
	service := readerTeamService(t, e)
	if err := service.SetTeamMember(e.Admin(), adminIdentity(e), e.Team1, e.Rep3, true); err != nil {
		t.Fatalf("an admin adding a reader: %v", err)
	}
	var named bool
	if err := integration.OwnerConn(t).QueryRow(context.Background(), `
		SELECT EXISTS (SELECT 1 FROM audit_log WHERE entity_type = 'team' AND entity_id = $1
		                AND after->'shared_mailboxes' ? $2)`, e.Team1, opsMailbox).Scan(&named); err != nil {
		t.Fatal(err)
	}
	if !named {
		t.Fatal("the audit row of a member added to a reader team does not name the mailbox they can now read")
	}
}

func TestATeamThatReadsASharedMailboxCannotBeArchived(t *testing.T) {
	e := integration.Setup(t)
	service := readerTeamService(t, e)
	archived := true
	_, err := service.UpdateTeam(e.Admin(), adminIdentity(e), e.Team1, identity.UpdateTeamInput{Archived: &archived})
	if !errors.Is(err, apperrors.ErrConflict) || !strings.Contains(err.Error(), opsMailbox) {
		t.Fatalf("archiving a reader team got %v, want a conflict naming %s", err, opsMailbox)
	}
}

func teamMembersOf(t *testing.T, team ids.UUID) int {
	t.Helper()
	var n int
	if err := integration.OwnerConn(t).QueryRow(context.Background(),
		`SELECT count(*) FROM team_membership WHERE team_id = $1`, team).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}
