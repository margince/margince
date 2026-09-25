// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package signals

// How a human answered a signal, joined onto the signal's own read.
//
// The rows have been written on every human resolution since the table existed
// and read by nothing: the note explaining WHY a signal was dismissed was
// recorded and then invisible to the next reader of it.

import (
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// signalResolutionJoin attaches the LATEST human answer on a signal.
//
// LEFT JOIN LATERAL and not a join on the table: a signal reopened and resolved
// again has more than one row, and a plain join would return the signal once per
// answer. Every append is kept — the earlier ones are the audit's to show — and
// the read says how the signal stands now.
const signalResolutionJoin = `
	LEFT JOIN LATERAL (
		SELECT outcome, note, resolved_by, created_at
		  FROM signal_resolution
		 WHERE signal_id = s.id
		 ORDER BY created_at DESC, id DESC
		 LIMIT 1
	) r ON true`

// signalResolutionColumns are r's half of the projection, in scanSignal's order.
const signalResolutionColumns = `r.outcome, r.note, r.resolved_by, r.created_at`

// resolutionFrom assembles the contract object, or nothing.
//
// The four columns arrive NULL together — the lateral join found no answer —
// and an object of zero values would read as an answer somebody gave.
func resolutionFrom(outcome, note *string, by *ids.UUID, at *time.Time) *crmcontracts.SignalResolution {
	if outcome == nil || at == nil {
		return nil
	}
	resolution := &crmcontracts.SignalResolution{
		Outcome:    crmcontracts.SignalResolutionOutcome(*outcome),
		Note:       note,
		ResolvedAt: *at,
	}
	if by != nil {
		id := openapi_types.UUID(*by)
		resolution.ResolvedBy = &id
	}
	return resolution
}
