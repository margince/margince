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
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// omittedFactors names the ranking factors this run had no input for, as
// opposed to the ones that scored low.
//
// Only warmth can be withheld today, and it is all-or-nothing whichever of its
// two reads is refused. Every seat on a deal is a `deal_stakeholder` edge, so a
// caller without that grant reads no stakeholders for ANY deal; and a caller
// without the contact grant gets the same answer for every stakeholder they do
// reach. Either way the factor has no input anywhere, which is why this is a
// property of the run rather than of an item, and why it omits the factor
// outright instead of leaving a queue where some deals were scored with it and
// some without.
// Empty rather than nil: "nothing was withheld" is an ANSWER, and the column
// storing it is NOT NULL for the same reason the wire field is always present.
func omittedFactors(gathered briefFacts, warmthReadable bool) []string {
	if gathered.seatsReadable && warmthReadable {
		return []string{}
	}
	return []string{string(crmcontracts.MorningBriefFactorsOmittedWarmth)}
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
// floors for that deal instead of out-seeing the contacts list, which is right —
// the queue is still ranked on what this reader may know, deal by deal, the way
// every other factor is.
//
// An OUTRIGHT refusal is the different case: the factor then has no input for
// ANY deal, so the whole queue reorders and the reader is owed the fact. There
// are two ways to be refused outright and both are reported — the seat edge
// (seatEvidenceBound, above) and the contact grant this read needs, which is
// what `readable` answers.
//
// The two are told apart by their sentinel, which is the contract the whole
// tree holds: a row-scope miss answers ErrNotFound so existence stays hidden,
// and an object denial answers ErrPermissionDenied. Reading them as one thing
// is what let a caller with no contact grant get a silently cold queue.
func (e *BriefEngine) resolveWarmth(
	ctx context.Context, now time.Time,
	facts map[ids.UUID]briefDealFacts, stakeholders map[ids.UUID][]ids.UUID,
) (readable bool, err error) {
	// Asked UP FRONT, the way the seat edge is, and not inferred from the first
	// refusal. A caller with no contact grant whose candidate deals happen to
	// carry no stakeholders never reaches the read at all — so a run that
	// inferred the answer would report nothing withheld for a queue whose warmth
	// factor could not have been read even if there had been someone to score.
	// The grant is a property of the caller; the seats are a property of the
	// deals, and only one of those decides whether the factor is readable.
	if err := auth.Require(ctx, "contact", principal.ActionRead); err != nil {
		if errors.Is(err, apperrors.ErrPermissionDenied) {
			return false, nil
		}
		return false, err
	}
	readable = true
	cache := map[ids.UUID]contacts.RelationshipStrength{}
	for dealID, stakeholderIDs := range stakeholders {
		f := facts[dealID]
		for _, contactID := range stakeholderIDs {
			st, ok := cache[contactID]
			if !ok {
				var err error
				st, err = e.strength.ContactStrength(ctx, ids.From[ids.ContactKind](contactID), now)
				switch {
				case errors.Is(err, apperrors.ErrNotFound):
					// Outside this caller's row scope: no strength to disclose,
					// and the queue is still ranked on what they may know.
					st = contacts.RelationshipStrength{}
				case errors.Is(err, apperrors.ErrPermissionDenied):
					// The grant was checked above, so this is the seam refusing
					// for a reason of its own. Still not one deal scoring low:
					// every contact answers the same way, so the factor has
					// nothing to read for the whole run.
					readable = false
					st = contacts.RelationshipStrength{}
				case err != nil:
					return false, err
				}
				cache[contactID] = st
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
	return readable, nil
}
