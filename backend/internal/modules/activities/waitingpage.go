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

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// noKeyset is the first page: every read but the refill asks for one page and
// stops, so the continuation slot is empty and the scan is bounded by the cap
// alone.
const noKeyset = ""

// WaitingPosition resumes a deterministic walk through messages sharing a timestamp.
type WaitingPosition struct {
	At time.Time
	ID ids.UUID
}

func olderThan(before WaitingPosition, arg func(any) int) string {
	if before.At.IsZero() {
		return ""
	}
	return fmt.Sprintf(" AND (a.occurred_at, a.id) < ($%d, $%d)", arg(before.At), arg(before.ID))
}
