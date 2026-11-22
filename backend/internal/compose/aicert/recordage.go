// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert

import "time"

// recordMaxAge is how long a record stays current for STALE_ONLY. A vendor can
// change the weights behind a model name with no stamp moving, so a sweep
// measures a record this old again even when nothing in the build changed.
const recordMaxAge = 60 * 24 * time.Hour

// recordsWithin keeps the records run within maxAge of now. A record whose run
// date does not parse is dropped: an age nobody can read is not a young one.
func recordsWithin(records []Record, now time.Time, maxAge time.Duration) []Record {
	kept := make([]Record, 0, len(records))
	for _, rec := range records {
		ranAt, err := time.Parse(time.RFC3339, rec.RanAt)
		if err == nil && now.Sub(ranAt) <= maxAge {
			kept = append(kept, rec)
		}
	}
	return kept
}
