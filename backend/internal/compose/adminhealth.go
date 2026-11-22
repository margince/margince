// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import "time"

// utcNow is the wall clock the health reports stamp and measure their windows by.
func utcNow() time.Time { return time.Now().UTC() }
