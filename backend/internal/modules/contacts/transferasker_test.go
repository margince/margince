// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"errors"
	"testing"

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
