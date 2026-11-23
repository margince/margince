// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import "time"

// stampInterval is how stale a "last used" stamp must be before an
// authentication writes it again. Session and passport share it, passed to SQL as
// an interval and judged on the database clock like the expiries it feeds.
const stampInterval = time.Minute
