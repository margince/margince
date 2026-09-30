// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// What is this colleague called, asked by the notification centre.
//
// The centre names the colleague whose decision took a line back, and reads
// that name through identity's directory — the same SeatNames the Worklist
// lane resolves an origin's actor through, and the same one seatNamer binds
// for the agent surface. Three askers, one answer: a panel that derived a name
// of its own would print a second spelling of one colleague beside the first.
//
// It lives here because a module never imports a sibling.

import (
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/notices"
)

// noticeSeatNames narrows identity's whole service to the one question the
// centre asks of it, which is the compiler's copy of that promise.
func noticeSeatNames(pool *pgxpool.Pool) notices.SeatNamer {
	return identity.NewService(pool)
}
