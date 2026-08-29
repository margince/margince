// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The dedupe card's evidence decoding — split from attentionseam.go on the
// file-length cap.

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/margince/margince/backend/internal/compose/attention"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// dedupeEvidenceRow is the detection-time snapshot as the queue stores it.
type dedupeEvidenceRow struct {
	Field      string  `json:"field"`
	LeftValue  *string `json:"left_value"`
	RightValue *string `json:"right_value"`
	Signal     string  `json:"signal"`
}

// comparisons decodes the stored snapshot.
//
// A snapshot that will not decode yields NO evidence rather than an error: the
// pair is still a real decision, and the two records beside each other are the
// larger part of the answer. Losing the field table degrades the card; refusing
// the whole lane over one malformed row would hide every other decision behind
// it.
func comparisons(ctx context.Context, candidate ids.UUID, raw json.RawMessage) []attention.FieldComparison {
	if len(raw) == 0 {
		return nil
	}
	var rows []dedupeEvidenceRow
	if err := json.Unmarshal(raw, &rows); err != nil {
		// Degrade for the reader, but say so. A snapshot that will not parse
		// means a detector wrote something nothing can read, and this is the
		// only place that would ever notice: an empty comparison and a corrupt
		// one look identical on screen, and forever.
		slog.WarnContext(ctx, "attention: dedupe evidence snapshot will not parse",
			"candidate_id", candidate.String(), "error", err)
		return nil
	}
	out := make([]attention.FieldComparison, 0, len(rows))
	for _, row := range rows {
		out = append(out, attention.FieldComparison{
			Field:  row.Field,
			Left:   row.LeftValue,
			Right:  row.RightValue,
			Signal: row.Signal,
		})
	}
	return out
}
