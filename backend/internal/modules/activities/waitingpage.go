// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// The waiting scan's keyset continuation.
//
// Its own file because the rest of waiting.go is about WHICH messages are
// waiting, and this is about where one page of them stops. The two change for
// different reasons: an eligibility rule moves in the SQL template, a paging
// rule moves here.

import (
	"fmt"
	"time"
)

// noKeyset is the first page: every read but the refill asks for one page and
// stops, so the continuation slot is empty and the scan is bounded by the cap
// alone.
const noKeyset = ""

// olderThan renders the keyset continuation for a waiting page, or nothing for
// the first one.
//
// On a.occurred_at alone rather than the (occurred_at, id) pair the row
// comparisons elsewhere use, and that is deliberate: this is a GROUP BY over
// links, so `a.id` is in the grouping and `a.occurred_at` is what the ORDER BY
// reads. A strict `<` can therefore skip rows sharing the boundary instant.
// That is the safe direction — a skipped row is one the reader does not see on
// this pass, where a `<=` would return the boundary row again and the refill
// would make no progress at all.
func olderThan(before time.Time, arg func(any) int) string {
	if before.IsZero() {
		return ""
	}
	return fmt.Sprintf(" AND a.occurred_at < $%d", arg(before))
}
