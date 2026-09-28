// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// The budget this module's detached claim writes carry, kept the way
// platform/keyvault keeps CleanupTimeout: beside the rule it answers.

import "time"

// bookkeepingTimeout bounds a detached settle or release: one statement against
// one row, so it is a ceiling on a stuck database rather than a budget anything
// approaches. Long rather than tight — an unbooked settle leaves the key in
// flight for the whole window, which makes the result the retry exists to fetch
// permanently unreachable.
const bookkeepingTimeout = 30 * time.Second
