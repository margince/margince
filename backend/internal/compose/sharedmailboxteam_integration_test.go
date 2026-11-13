// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const opsMailbox = "ops@fable.test"

// readerTeamService is identity with capture's seam, Team1 reading ops@.
func readerTeamService(t *testing.T, e *integration.Env) *identity.Service {
	t.Helper()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		integration.ConnectSharedMailbox(t, tx, integration.SharedMailbox{
			Provider: "imap", Address: opsMailbox, ReaderTeam: e.Team1, ConnectedBy: e.AdminUser, Posture: "held",
		})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return identity.NewService(e.Pool).WithMailCapture(mailCaptureSeam{})
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

func TestAnAgentCannotAddAReaderToATeamThatReadsASharedMailbox(t *testing.T) {
	e := integration.Setup(t)
	service := readerTeamService(t, e)
	before := teamMembersOf(t, e.Team1)

	err := service.SetTeamMember(agentOfAdmin(e), recoveryAdminIdentity(e), e.Team1, e.Rep3, true)
	if !errors.Is(err, apperrors.ErrPermissionDenied) || !strings.Contains(err.Error(), opsMailbox) {
		t.Fatalf("an agent adding a reader got %v, want a refusal naming %s", err, opsMailbox)
	}
	if after := teamMembersOf(t, e.Team1); after != before {
		t.Fatalf("the refused add still changed the team: %d members, was %d", after, before)
	}
}

// agentOfAdmin is the admin's own agent, acting on their passport.
func agentOfAdmin(e *integration.Env) context.Context {
	agent, _ := principal.Actor(e.Admin())
	agent.Type, agent.OnBehalfOf, agent.PassportID = principal.PrincipalAgent, e.AdminUser, ids.NewV7()
	return principal.WithActor(e.Admin(), agent)
}

func TestAnAgentCannotInviteAMemberOntoATeamThatReadsASharedMailbox(t *testing.T) {
	e := integration.Setup(t)
	service := readerTeamService(t, e)
	company := contacts.NewStore(e.DB()).WithInstallationRenamer(installationRenamer(e.Pool))
	if _, err := company.SaveCompany(e.Admin(), contacts.SaveCompanyInput{DisplayName: "Example company"}); err != nil {
		t.Fatal(err)
	}
	identity.NewHandlers(service).WithInstallationDescribed(installationDescribed(company))
	_, _, err := service.InviteUser(agentOfAdmin(e), recoveryAdminIdentity(e), identity.InviteUserInput{
		Email: "newcomer@fable.test", DisplayName: "Newcomer", Role: "rep", TeamIDs: []ids.UUID{e.Team1},
	})
	if !errors.Is(err, apperrors.ErrPermissionDenied) || !strings.Contains(err.Error(), opsMailbox) {
		t.Fatalf("an agent inviting onto a reader team got %v, want a refusal naming %s", err, opsMailbox)
	}
}

func TestAnAgentIsRefusedEvenWhenTheTeamsSharedMailboxIsDisconnected(t *testing.T) {
	e := integration.Setup(t)
	service := readerTeamService(t, e)
	if _, err := integration.OwnerConn(t).Exec(context.Background(),
		`UPDATE capture_connection SET status = 'disconnected', archived_at = now() WHERE account_label = $1`, opsMailbox); err != nil {
		t.Fatal(err)
	}
	err := service.SetTeamMember(agentOfAdmin(e), recoveryAdminIdentity(e), e.Team1, e.Rep3, true)
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("an agent adding a reader of a disconnected mailbox's history got %v, want a refusal", err)
	}
}

func TestAnAdminAddingAReaderIsAuditedWithTheMailboxTheyCanNowRead(t *testing.T) {
	e := integration.Setup(t)
	service := readerTeamService(t, e)
	if err := service.SetTeamMember(e.Admin(), recoveryAdminIdentity(e), e.Team1, e.Rep3, true); err != nil {
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
	_, err := service.UpdateTeam(e.Admin(), recoveryAdminIdentity(e), e.Team1, identity.UpdateTeamInput{Archived: &archived})
	if !errors.Is(err, apperrors.ErrConflict) || !strings.Contains(err.Error(), opsMailbox) {
		t.Fatalf("archiving a reader team got %v, want a conflict naming %s", err, opsMailbox)
	}
}

func TestFoldingAMailCopyCarriesTheSharedMailboxsImportRow(t *testing.T) {
	e := integration.Setup(t)
	kept, duplicate := ids.NewV7(), ids.NewV7()
	for _, id := range []ids.UUID{kept, duplicate} {
		e.WsExec(t, `INSERT INTO activity(id,kind,subject,occurred_at,source,captured_by) VALUES($1,'email','Order','2026-10-01T09:00:00Z','imap','connector:imap')`, id)
	}
	var folded bool
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		box := integration.ConnectSharedMailbox(t, tx, integration.SharedMailbox{
			Provider: "imap", Address: opsMailbox, ReaderTeam: e.Team1, ConnectedBy: e.AdminUser, Posture: "held",
		})
		if _, err := tx.Exec(context.Background(), `INSERT INTO capture_import(activity_id,connection_id,posture_at_import) VALUES($1,$2,'held')`, duplicate, box); err != nil {
			return err
		}
		var err error
		folded, err = carryMailImportsTx(context.Background(), tx, ids.From[ids.ActivityKind](duplicate), ids.From[ids.ActivityKind](kept))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if !folded {
		t.Fatal("a copy only a shared mailbox imported was refused the fold")
	}
	if got := e.WsCount(t, `SELECT count(*) FROM capture_import WHERE activity_id=$1 AND user_id IS NULL AND posture_at_import='held'`, kept); got != 1 {
		t.Fatalf("the surviving message carries %d shared import rows, want 1: its reader team would lose it", got)
	}
}
