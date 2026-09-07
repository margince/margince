// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose/briefevidence"
	"github.com/margince/margince/backend/internal/modules/activities"
)

// emailRows is the reader that opens a cited message, for every service that
// grounds prose in records.
//
// One line per construction site, and there are two per service: the default
// assembly and the option that rebinds it once a model lane is wired. A
// service constructed without this reader draws its citations exactly as it did
// before the enrichment existed — no error, no empty section, just a chip that
// does nothing — so the omission is invisible except to the assembly test that
// asserts every one of them holds a reader.
//
//nolint:ireturn // the seam's whole job is to hand every construction site the one interface they take.
func emailRows(pool *pgxpool.Pool) briefevidence.Reader {
	return activities.NewStore(InstallationDB(pool))
}
