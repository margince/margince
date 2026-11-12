// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Sources lists the messages a receipt rests on: those its change cites, or
// the one its approval cites.
func (r Receipt) Sources() []ids.UUID {
	if len(r.CitedActivities) > 0 {
		return r.CitedActivities
	}
	if r.SourceActivityID.IsZero() {
		return nil
	}
	return []ids.UUID{r.SourceActivityID}
}

// NewestSource is the latest source the reader may open, the one a change most
// rests on; zero when the reader may open none. Only a source in happened is
// named. It is dated as the change recorded it, else as happened reads it now.
func (r Receipt) NewestSource(happened map[ids.UUID]activities.ActivityMoment) (ids.UUID, activities.ActivityMoment) {
	var newest ids.UUID
	var at activities.ActivityMoment
	for _, source := range r.Sources() {
		when, open := happened[source]
		if !open {
			continue
		}
		if recorded, ok := r.CitedMoments[source]; ok {
			when = activities.ActivityMoment{At: recorded.At, Precision: recorded.Precision}
		}
		if newest.IsZero() || when.At.After(at.At) {
			newest, at = source, when
		}
	}
	return newest, at
}
