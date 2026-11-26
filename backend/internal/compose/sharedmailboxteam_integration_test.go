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
	connectOpsMailbox(t, e)
	return identity.NewService(e.Pool).WithMailCapture(mailCaptureSeam{})
}

// connectOpsMailbox connects ops@, read by Team1.
func connectOpsMailbox(t *testing.T, e *integration.Env) ids.UUID {
	t.Helper()
	return integration.ConnectSharedMailbox(t, integration.SharedMailboxRegistry(t, e.Pool, "imap"), e.WS, integration.SharedMailbox{
		Provider: "imap", Address: opsMailbox, ReaderTeam: e.Team1, ConnectedBy: e.AdminUser, Posture: "held",
	})
}

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

func TestFoldingAMailCopyCarriesTheSharedMailboxsImportRow(t *testing.T) {
	e := integration.Setup(t)
	kept, duplicate := ids.NewV7(), ids.NewV7()
	for _, id := range []ids.UUID{kept, duplicate} {
		e.WsExec(t, `INSERT INTO activity(id,kind,subject,occurred_at,source,captured_by) VALUES($1,'email','Order','2026-10-01T09:00:00Z','imap','connector:imap')`, id)
	}
	box := connectOpsMailbox(t, e)
	var folded bool
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
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
