// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// A request names its asker as requester and recipient, so an agent with no user
// id cannot ask: it would stage a request against nobody.
func TestAnAgentWithNoUserCannotAskForAHandover(t *testing.T) {
	agent := principal.Principal{
		Type: principal.PrincipalAgent, PassportID: ids.NewV7(), OnBehalfOf: ids.NewV7(), SeatType: principal.SeatFull,
		Permissions: principal.Permissions{Objects: map[string]principal.ObjectGrant{
			entityContact: {Read: true, Update: true},
		}},
	}
	err := requireTransferAsker(principal.WithActor(context.Background(), agent), entityContact)
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("an agent with no user asked: %v, want permission denied", err)
	}
	agent.UserID = agent.OnBehalfOf
	if err := requireTransferAsker(principal.WithActor(context.Background(), agent), entityContact); err != nil {
		t.Errorf("an agent acting for its user: %v, want admitted", err)
	}
}

// noRowTx answers every EXISTS probe "no row".
type noRowTx struct{ pgx.Tx }

func (noRowTx) QueryRow(context.Context, string, ...any) pgx.Row { return noRow{} }

type noRow struct{}

func (noRow) Scan(dest ...any) error {
	answer, ok := dest[0].(*bool)
	if !ok {
		return errors.New("noRow: the probe scans a bool")
	}
	*answer = false
	return nil
}

// An all-scope seat is told it may already change a record only after the
// other checks. A missing grant is 403 and a missing record 404, as for every
// other asker.
func TestAnAllScopeAskerMeetsTheGrantAndTheRecordFirst(t *testing.T) {
	asker := func(grant principal.ObjectGrant) context.Context {
		user := ids.NewV7()
		return principal.WithActor(context.Background(), principal.Principal{
			Type: principal.PrincipalHuman, UserID: user, ID: "user:" + user.String(), SeatType: principal.SeatFull,
			Permissions: principal.Permissions{RowScope: principal.RowScopeAll, Objects: map[string]principal.ObjectGrant{entityContact: grant}},
		})
	}
	_, err := TransferRequestableTx(asker(principal.ObjectGrant{Read: true}), noRowTx{}, entityContact, ids.NewV7())
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("an all-scope asker without contact.update: %v, want permission denied", err)
	}
	_, err = TransferRequestableTx(asker(principal.ObjectGrant{Read: true, Update: true}), noRowTx{}, entityContact, ids.NewV7())
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("an all-scope asker naming no record: %v, want not found", err)
	}
}
