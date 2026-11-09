// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package meetingmap

import (
	"time"

	"github.com/margince/margince/backend/internal/modules/capture"
)

func eventDuration(event Event) *int {
	if event.StartsAt.IsZero() || !event.EndsAt.After(event.StartsAt) {
		return nil
	}
	seconds := int(event.EndsAt.Sub(event.StartsAt) / time.Second)
	return capture.StorableDuration(&seconds)
}
