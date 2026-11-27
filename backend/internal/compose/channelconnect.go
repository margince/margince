// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/margince/margince/backend/internal/compose/messengeraccounts"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/capture/telegram"
)

// WithChannelSurface must precede WithKeyvault so mutations receive its vault.
func WithChannelSurface() Option {
	return func(s *Server, pool *pgxpool.Pool) {
		s.channelHandlers = capture.NewChannelHandlers(
			capture.NewChannelStore(InstallationDB(pool), nil, telegram.NewAPI(nil, ""), s.log).WithAuthority(messengeraccounts.MemberAuthority))
	}
}
