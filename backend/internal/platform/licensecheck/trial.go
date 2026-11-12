// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package licensecheck

import "time"

// TrialLength is how long a production installation runs with no license,
// counted from the day its workspace was created.
const TrialLength = 14 * 24 * time.Hour

// Trial is the free period of a production installation that has no license.
// Nothing is signed: the start is the workspace's own creation time, and an
// operator who changes it or the code is outside the license terms, not
// outside a check this type could make.
type Trial struct {
	StartedAt time.Time
}

// EndsAt is the first instant the trial no longer covers.
func (t Trial) EndsAt() time.Time { return t.StartedAt.Add(TrialLength) }

// Ended reports whether the trial is over at now. There is no grace period:
// the installation is warned in the days before, and locks at EndsAt.
func (t Trial) Ended(now time.Time) bool { return !now.Before(t.EndsAt()) }
