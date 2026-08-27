// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

// Reading the link a restore recorded — the audit row it reversed — back onto
// both history projections.
//
// The two projections meet the link in different shapes, because they already
// read the evidence column differently: the record spine projects the single key
// in SQL and never decodes the blob, while field history decodes the whole image
// for an agent actor's evidence anyway. Both funnel through one parse here, so
// "what counts as a link" has one answer whichever projection a reader is on.

import (
	"fmt"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// reversalLinkColumn is the link as a SELECT expression, for the projection both
// readers of the audit spine share.
//
// The key is inlined as a compile-time literal rather than bound: the two
// queries number their arguments differently — one at a fixed position, one
// varying with its boundary and cursor predicates — so a placeholder here would
// have to be right for both. It is a constant identifier and never a value off a
// request, which is the only shape of identifier this tree formats into SQL.
const reversalLinkColumn = `a.evidence ->> '` + UndidAuditLogID + `'`

// reversalLinkFromEvidence reads the link off a decoded evidence image.
func reversalLinkFromEvidence(evidence map[string]any) (*ids.UUID, error) {
	raw, present := evidence[UndidAuditLogID]
	if !present || raw == nil {
		return nil, nil //nolint:nilnil // almost every row reversed nothing: "no link" IS the answer, not a fault
	}
	text, isString := raw.(string)
	if !isString {
		return nil, fmt.Errorf("reversal link %q is %T, want a uuid string", UndidAuditLogID, raw)
	}
	return parseReversalLink(text)
}

// reversalLinkFromColumn reads the link off the projected reversalLinkColumn.
// `->>` yields SQL NULL for an absent key AND for a key whose value is JSON
// null, so one nil pointer covers both shapes of "this row reversed nothing".
func reversalLinkFromColumn(projected *string) (*ids.UUID, error) {
	if projected == nil {
		return nil, nil //nolint:nilnil // SQL NULL here means the row reversed nothing, which is the common case
	}
	return parseReversalLink(*projected)
}

// parseReversalLink refuses anything that is not a uuid rather than reporting no
// link: a corrupt link would pair a reversal with the wrong change, and a
// history line that names the wrong reversed row is worse than a failed read.
func parseReversalLink(raw string) (*ids.UUID, error) {
	parsed, err := ids.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("reversal link %q: %w", raw, err)
	}
	return &parsed, nil
}
