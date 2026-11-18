// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
)

// privateMailDealClockDataTask refolds the stored deal clocks written before a
// deal counted the mail only its participants may read.
func privateMailDealClockDataTask() datatask.Task {
	audit := datatask.AuditPolicy{Reason: clockRefoldAuditReason}
	return datatask.Task{
		ID: "core/1795014819_private_mail_moves_the_deal_clock", Version: 1,
		Title: systemtext.Literal("datatask.title.core.1795014819_private_mail_moves_the_deal_clock"), Requires: []string{"core/1795014819"},
		Steps: []datatask.Step{
			clockRefoldStep("deals", storekit.ClockDeals, storekit.ClockNotFolded, audit),
		},
	}
}
