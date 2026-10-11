// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Saying at boot which correspondence floor this installation applies.
//
// The floor decides what may be destroyed, and an operator reads the boot
// output. So it tells four cases apart. No jurisdiction is selected; one is,
// with no floor; one is, with a floor; a carried floor binds older mail.

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/database"
)

// announceStatutoryFloorFrom resolves the floor policy and reports it. A
// failed read is reported, not fatal: the destructive paths resolve the policy
// again and refuse on the same failure.
func announceStatutoryFloorFrom(pool *pgxpool.Pool, log *slog.Logger) {
	if log == nil || pool == nil {
		return
	}
	var policy privacy.FloorPolicy
	err := database.WithInfraTx(context.Background(), pool, func(tx pgx.Tx) error {
		var err error
		policy, err = privacy.ResolveFloorPolicy(context.Background(), tx, identity.CountryOf)
		return err
	})
	if err != nil {
		log.Error("privacy: the statutory retention floor could not be resolved at boot; destructive paths refuse until it can", "err", err)
		return
	}
	announceStatutoryFloor(log, privacy.StatutoryFloorPosture(policy))
}

// announceStatutoryFloor reports one posture. It warns when no floor binds new
// mail. Otherwise it logs at info, which records the floor in force.
func announceStatutoryFloor(log *slog.Logger, posture privacy.FloorPosture) {
	if log == nil {
		return
	}
	if c := posture.Carried; c != nil {
		log.Info("privacy: a carried retention floor from an earlier jurisdiction binds mail that occurred before the change",
			"class", c.Class, "keep", c.Keep.String(), "from_year_end", c.FromYearEnd,
			"before", c.Before, "binds_until", c.BindsUntil)
	}
	switch {
	case posture.Selected == "" && posture.Known:
		log.Info("privacy: no jurisdiction selected — the strictest compiled-in retention floor is in force",
			"class", posture.Class, "keep", posture.Keep.String(), "from_year_end", posture.FromYearEnd, "packs", posture.Packs)
	case posture.Selected == "" && len(posture.Packs) == 0:
		log.Warn("privacy: no jurisdiction selected and no jurisdiction packs compiled in — " +
			"no statutory retention floor applies to captured mail")
	case posture.Selected == "":
		log.Warn("privacy: no jurisdiction selected and no compiled-in pack declares a floor — "+
			"no statutory retention floor applies to captured mail", "packs", posture.Packs)
	case posture.Known:
		log.Info("privacy: statutory retention floor in force",
			"jurisdiction", posture.Selected, "class", posture.Class, "keep", posture.Keep.String(), "from_year_end", posture.FromYearEnd)
	default:
		log.Warn("privacy: no statutory retention floor — the selected jurisdiction's pack declares none",
			"jurisdiction", posture.Selected)
	}
}
