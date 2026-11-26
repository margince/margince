// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package strengthdates

import (
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
)

// DirectionlessSendersTask refolds the stored dates of each contact who wrote
// a message stored with no direction. Those dates were folded before such a
// message counted as theirs, and no row changes to fire a trigger. A refold
// writes only dates that differ, so a second run changes no row.
func DirectionlessSendersTask() datatask.Task {
	return datatask.Task{
		ID: "core/1795675601_directionless_sender_strength_dates", Version: 1,
		Title:    systemtext.Literal("datatask.title.core.1795675601_directionless_sender_strength_dates"),
		Requires: []string{"core/1795675601"},
		Steps:    []datatask.Step{refoldStep(contacts.DirectionlessSendersPageTx)},
	}
}
