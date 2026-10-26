// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// The score a corrected origin implies, folded into the edit's own UPDATE.
//
// The row's version trigger fires on every UPDATE. A rescore written as a
// second statement after the edit therefore raises the version twice.
// Scored before the edit lands, the new number rides the same statement. It
// also shares the audit row and the event of the field that caused it.

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// stagedLeadRescore is a rescore staged on a patch. Its history entry follows
// the lead's UPDATE, because the entry's row refers to the lead. The zero value
// is "nothing moved".
type stagedLeadRescore struct {
	moved     bool
	displayed int
	scored    LeadScoring
	reason    *string
}

// stageLeadRescore scores the lead as the patch will leave it. A corrected
// origin, or a withdrawn override (resume), stages the new number. It goes to
// score_computed under an override, and to score otherwise.
//
// The number is a stamp. It is derived from the origin, so it stays out of the
// audit images, and a restore of the origin edit re-derives it.
//
// History records the number moving, except for a withdrawn override. The clear
// has already set score to the retained value, yet the newest entry still names
// the override. Left out, the series would say the override is in force.
func stageLeadRescore(
	ctx context.Context, tx pgx.Tx, id ids.LeadID, current crmcontracts.Lead, p *storekit.Patch, resume bool,
) (stagedLeadRescore, error) {
	if _, originMoved := p.Moved()[leadOriginField]; !originMoved && !resume {
		return stagedLeadRescore{}, nil
	}
	after := p.After()
	if !LeadStatus(stagedString(after, leadStatusColumn, string(current.Status))).Open() {
		return stagedLeadRescore{}, nil
	}
	title := stagedOptionalString(after, "title", current.Title)
	origin := stagedOptionalString(after, leadOriginField, current.Origin)
	reason := stagedOptionalString(after, "score_override_reason", current.ScoreOverrideReason)
	displayed := stagedInt(after, "score", current.Score)

	scored, err := scoreLeadFor(ctx, tx, id, time.Now().UTC(), title, origin)
	if err != nil {
		return stagedLeadRescore{}, err
	}
	machine := scored.Score
	if reason != nil {
		computed, held := stagedOptionalInt(after, fieldKeyScoreComputed, current.ScoreComputed)
		if held && computed == machine {
			return stagedLeadRescore{}, nil
		}
		p.SetStamp(fieldKeyScoreComputed, machine)
		return stagedLeadRescore{moved: true, displayed: displayed, scored: scored, reason: reason}, nil
	}
	if machine == displayed && !resume {
		return stagedLeadRescore{}, nil
	}
	if machine != displayed {
		p.SetStamp("score", machine)
	}
	return stagedLeadRescore{moved: true, displayed: machine, scored: scored}, nil
}

// record writes the history entry for the rescore, after the lead's UPDATE.
func (r stagedLeadRescore) record(ctx context.Context, tx pgx.Tx, id ids.LeadID) error {
	return appendLeadScoreHistory(ctx, tx, id, r.displayed, r.scored, r.reason)
}

// stagedString is the value the patch will write to a text column, or the
// current one when the patch leaves it alone.
func stagedString(after map[string]any, column, current string) string {
	if v, ok := after[column].(string); ok {
		return v
	}
	return current
}

// stagedOptionalString is stagedString for a nullable column. A patch that
// clears the column stages a nil.
func stagedOptionalString(after map[string]any, column string, current *string) *string {
	v, staged := after[column]
	if !staged {
		return current
	}
	if s, ok := v.(string); ok {
		return &s
	}
	return nil
}

func stagedInt(after map[string]any, column string, current int) int {
	if v, ok := after[column].(int); ok {
		return v
	}
	return current
}

// stagedOptionalInt reports the value the patch leaves in a nullable integer
// column, and whether that is a value at all rather than NULL.
func stagedOptionalInt(after map[string]any, column string, current *int) (int, bool) {
	v, staged := after[column]
	if !staged {
		if current == nil {
			return 0, false
		}
		return *current, true
	}
	n, ok := v.(int)
	return n, ok
}
