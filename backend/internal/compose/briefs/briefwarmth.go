// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package briefs

// The warmth factor: where its input comes from, who may read that input, and
// what the run says when nobody may.
//
// Split from briefrank.go because it is the one factor whose input is behind a
// SEPARATE grant. Every other ranking fact rides the deal read the queue is
// already bounded by; warmth reads the deal's stakeholders, which is the
// `relationship` edge, and a caller can hold one and not the other. That makes
// "could we read it" a question the other factors never have to ask — and the
// answer part of the run's own record.

import (
	"context"
	"errors"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// omittedFactors names the ranking factors this run had no input for, as
// opposed to the ones that scored low.
//
// Only warmth can be withheld today, and it is all-or-nothing: every seat on a
// deal is a `deal_stakeholder` edge, so a caller without that grant reads no
// stakeholders for ANY deal. That is why it is a property of the run rather
// than of an item, and why one refused read omits the factor outright instead
// of leaving a queue where some deals were scored with it and some without.
// Empty rather than nil: "nothing was withheld" is an ANSWER, and the column
// storing it is NOT NULL for the same reason the wire field is always present.
func omittedFactors(gathered briefFacts) []string {
	if gathered.seatsReadable {
		return []string{}
	}
	return []string{string(crmcontracts.Warmth)}
}

// seatEvidenceBound resolves the seat edge's admission for the stakeholder
// evidence read: the arguments its clause binds, the clause itself, and whether
// the caller may run the read at all.
//
// The registrar returns positions offset by one because the statement it feeds
// already spends $1 on the deal id. Getting that wrong would bind the deal id
// to a scope predicate, which is why the offset lives here with the statement
// it belongs to rather than at the call site.
// A refusal is reported through `admitted`, never swallowed: what the caller
// does with it is the whole question this read used to leave open, and
// briefEvidenceRows carries the answer up to the run.
func seatEvidenceBound(ctx context.Context) (args []any, clause string, admitted bool, err error) {
	clause, err = auth.EdgeReadScope(ctx, "r", func(v any) int {
		args = append(args, v)
		return len(args) + 1
	})
	if errors.Is(err, apperrors.ErrPermissionDenied) {
		return nil, "", false, nil
	}
	if err != nil {
		return nil, "", false, err
	}
	if clause == "" {
		clause = "TRUE"
	}
	return args, clause, true, nil
}

// resolveWarmth fills each deal's warmth from its strongest visible
// stakeholder through the injected §4 seam.
//
// A stakeholder outside the caller's ROW scope contributes nothing: the factor
// floors for that deal instead of out-seeing the people list, which is right —
// the queue is still ranked on what this reader may know, deal by deal, the way
// every other factor is.
//
// A caller refused the seat edge OUTRIGHT is the different case, and it is not
// answered here: the factor has no input for ANY deal, so the whole queue
// reorders. That one is named in BriefRanking.FactorsOmitted rather than
// floored in silence — see omittedFactors.
func (e *BriefEngine) resolveWarmth(ctx context.Context, now time.Time, facts map[ids.UUID]briefDealFacts, stakeholders map[ids.UUID][]ids.UUID) error {
	cache := map[ids.UUID]people.RelationshipStrength{}
	for dealID, persons := range stakeholders {
		f := facts[dealID]
		for _, personID := range persons {
			st, ok := cache[personID]
			if !ok {
				var err error
				st, err = e.strength.PersonStrength(ctx, ids.From[ids.PersonKind](personID), now)
				switch {
				case errors.Is(err, apperrors.ErrNotFound), errors.Is(err, apperrors.ErrPermissionDenied):
					// Invisible to this caller: no strength to disclose.
					st = people.RelationshipStrength{}
				case err != nil:
					return err
				}
				cache[personID] = st
			}
			if st.Strength > f.warmthStrength {
				f.warmthStrength = st.Strength
				f.warmthEvidence = make([]ids.UUID, len(st.ContributingIDs))
				for i, activityID := range st.ContributingIDs {
					f.warmthEvidence[i] = activityID.UUID
				}
			}
		}
		facts[dealID] = f
	}
	return nil
}
