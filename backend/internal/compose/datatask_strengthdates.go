// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
)

// strengthDatesDataTask folds the stored strength dates of every contact the
// migration that added them found, through the refold the clock job runs.
func strengthDatesDataTask() datatask.Task {
	return datatask.Task{
		ID: "core/1795258153_contact_strength_dates", Version: 1,
		Title: systemtext.Literal("datatask.title.core.1795258153_contact_strength_dates"), Requires: []string{"core/1795258153"},
		Steps: []datatask.Step{
			clockRefoldStep("contacts", storekit.ClockContacts, storekit.ClockStrengthUnfolded,
				datatask.AuditPolicy{Reason: clockRefoldAuditReason}),
		},
	}
}
