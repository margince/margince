// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/teamchat"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// DisconnectTeamChatProvider retains the connection owner's retirement and audit behavior during a service switch.
func (h ChannelHandlers) DisconnectTeamChatProvider(ctx context.Context, provider string, expected teamchat.Selection) error {
	if !teamchat.Supported(provider) {
		return apperrors.ErrInvalidArgument
	}
	if err := auth.Require(ctx, channelConnectionObject, principal.ActionDelete); err != nil {
		return err
	}
	if h.store == nil {
		return ErrChannelWiringIncomplete
	}
	connections, err := h.store.List(ctx)
	if err != nil {
		return err
	}
	for _, connection := range connections {
		if connection.Provider == provider {
			if err := h.store.disconnectForSelection(ctx, connection.ID, &expected); err != nil {
				return err
			}
		}
	}
	return nil
}
