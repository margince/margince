// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"context"
	"log/slog"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// ownerAnswers is the set of owner kinds one source may state.
//
// Personal automatic work needs an evidenced responsible seat. Each kind in
// api/worklist-kinds.yaml declares which answers its lane may give. A row of
// an unknown source, or with an answer its kind does not allow, reaches no
// page. Its lane is reported failed and the fault is logged.
type ownerAnswers map[ownerKind]bool

func answers(kinds ...ownerKind) ownerAnswers {
	out := make(ownerAnswers, len(kinds))
	for _, kind := range kinds {
		out[kind] = true
	}
	return out
}

// admitBySource applies each kind's owner answers to every classified row, before
// scope, counts, Focus and paging read them, so all of them see one answer. It
// returns the kept rows and one failed-source entry per lane that lost a row.
func admitBySource(ctx context.Context, rows []ranked) ([]ranked, []crmcontracts.WorklistSourceUnavailable) {
	kept := rows[:0:0]
	var refused []crmcontracts.WorklistSourceUnavailable
	failed := map[crmcontracts.WorklistItemSource]bool{}
	for _, row := range rows {
		allowed, known := ownerAnswersOf(row.item.Source)
		if known && allowed[row.ownerRef.kind] {
			kept = append(kept, row)
			continue
		}
		slog.ErrorContext(ctx, "a worklist row has no admission for its owner; it creates no work",
			"source", row.item.Source, "lane", row.lane, "owner_kind", int(row.ownerRef.kind), "policy_known", known)
		lane := reportedLane(row)
		if !failed[lane] {
			failed[lane] = true
			refused = append(refused, crmcontracts.WorklistSourceUnavailable{Source: string(lane), Reason: "failed"})
		}
	}
	return kept, refused
}

// reportedLane names the lane a refused row is reported under. That is the
// lane that read it, or its own source for a row read outside the day's lanes.
func reportedLane(row ranked) crmcontracts.WorklistItemSource {
	if row.lane != "" {
		return row.lane
	}
	return row.item.Source
}
