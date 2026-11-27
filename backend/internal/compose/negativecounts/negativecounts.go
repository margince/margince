// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package negativecounts corrects the counts stored before their floors.
package negativecounts

import (
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
)

// DataTask corrects the counts stored before their floors: a negative duration
// becomes unknown, a negative token figure zero. Once every supported upgrade
// has run it, the floors can be validated.
func DataTask() datatask.Task {
	return datatask.Task{
		ID: "core/1795744558_negative_counts", Version: 1,
		Title:    systemtext.Literal("datatask.title.core.1795744558_negative_counts"),
		Requires: []string{"core/1795744556", "core/1795744557"},
		Steps: []datatask.Step{{
			Name: "clear_negative_durations", Limit: 200, MinLimit: 1,
			Audit:         datatask.AuditPolicy{PerRecord: true},
			EmptyWhenDone: true,
			Estimate:      activities.CountNegativeDurations, Batch: activities.ClearNegativeDurationsTx,
		}, {
			Name: "zero_negative_token_counts", Limit: 500, MinLimit: 1,
			Audit: datatask.AuditPolicy{Reason: "An ai_call row is the meter's record of a served call, " +
				"written unaudited by ai.CallMeter, and the correction stores the figure that meter stores now."},
			EmptyWhenDone: true,
			Estimate:      ai.CountNegativeTokenCalls, Batch: ai.ZeroNegativeTokenCountsTx,
		}},
	}
}
