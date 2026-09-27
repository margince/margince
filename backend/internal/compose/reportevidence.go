// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The record set a saved run names, and how much of it this reader reaches.
//
// A run fixes a question, and the question names records. The reader's share
// of them is the drill-through the report drawer opens — ExplainAnalyticsCell
// under the reader's live grants. The WHOLE set is the same drill-through under
// full authority, with the population the reader was answered about. The first
// is what may be searched; the second is only compared against it, so a claim
// about "every record" can be refused when the reader holds part of them.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/analyticsquery"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// reportRunCohort is one saved run's record set as this reader reaches it.
type reportRunCohort struct {
	Entity    datasource.EntityType
	Reached   []ids.UUID
	Withheld  bool
	Truncated bool
	// Unreached: records of the set are outside the reader's access today.
	Unreached bool
	// Unestablished: the whole set could not be determined, so nothing is
	// known about what the reader missed.
	Unestablished bool
}

// cohortAuthorityID names the full-authority read in anything that logs it.
const cohortAuthorityID = "system:report-evidence"

// readReportRunCohort resolves a run — one cell, or with a nil cell the whole
// population it measured — to the records this reader reaches, through the
// same drill-through POST /analytics/runs/{id}/cells/explain serves.
func readReportRunCohort(
	ctx context.Context, tx pgx.Tx, runID ids.UUID, cell *[]any, floor analyticsquery.Floor,
) (reportRunCohort, error) {
	explain, err := reportRunExplain(ctx, tx, runID, cell)
	if err != nil {
		return reportRunCohort{}, err
	}
	reached, err := ExplainAnalyticsCell(ctx, tx, explain, floor)
	if err != nil {
		return reportRunCohort{}, err
	}
	reachedIDs, err := explainedIDs(reached)
	if err != nil {
		return reportRunCohort{}, err
	}
	out := reportRunCohort{
		Entity: reached.Entity, Withheld: reached.Withheld, Truncated: reached.Truncated,
		Reached: reachedIDs,
	}
	if reached.Withheld {
		return out, nil
	}
	whole, established, err := wholeCohort(ctx, tx, explain, floor)
	if err != nil {
		return reportRunCohort{}, err
	}
	if !established {
		out.Unestablished = true
		return out, nil
	}
	reachedSet := make(map[ids.UUID]bool, len(out.Reached))
	for _, id := range out.Reached {
		reachedSet[id] = true
		if !whole[id] {
			// The reader holds a record the whole set lacks, so the whole set
			// is not whole: say nothing about it rather than something wrong.
			out.Unestablished = true
		}
	}
	for id := range whole {
		if !reachedSet[id] {
			out.Unreached = true
		}
	}
	return out, nil
}

// wholeCohort is the run's record set under full authority, narrowed to the
// population the reader was answered about. false means it could not be
// determined, which the caller reports rather than guesses past.
func wholeCohort(
	ctx context.Context, tx pgx.Tx, explain analyticsquery.Explain, floor analyticsquery.Floor,
) (map[ids.UUID]bool, bool, error) {
	reader, ok := principal.Actor(ctx)
	if !ok {
		return nil, false, errors.New("compose: no actor bound to context")
	}
	spec := prebuiltReports[explain.Query.Entity]
	q := explain.Query
	// An unscoped question is answered about the READER's default population;
	// full authority would resolve it to the workspace. Naming the reader's
	// resolution keeps the whole set about the same records.
	if q.ScopeKind == "" && populationApplies(spec, RequestedScope{}) {
		resolved, err := ResolveAnalyticsScope(ctx, tx, RequestedScope{})
		if err != nil {
			return nil, false, err
		}
		switch resolved.Kind {
		case ScopeKindWorkspace:
			q.ScopeKind = ScopeKindWorkspace
		case ScopeKindOwner, ScopeKindTeam:
			q.ScopeKind, q.ScopeID = resolved.Kind, resolved.ID.String()
		default:
			// Several teams and the reader: no one requestable scope names it.
			return nil, false, nil
		}
	}
	// UserID stays the reader's so "my own records" keeps meaning theirs,
	// including the unowned rows their default population admits.
	full := principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalSystem, ID: cohortAuthorityID, UserID: reader.UserID,
	})
	known, err := ExplainAnalyticsCell(full, tx, analyticsquery.Explain{Query: q, Group: explain.Group}, floor)
	if err != nil {
		if isAnalyticsRefusal(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	if known.Withheld || known.Truncated {
		return nil, false, nil
	}
	knownIDs, err := explainedIDs(known)
	if err != nil {
		return nil, false, err
	}
	whole := make(map[ids.UUID]bool, len(knownIDs))
	for _, id := range knownIDs {
		whole[id] = true
	}
	return whole, true, nil
}

// isAnalyticsRefusal says the question was refused rather than failed.
func isAnalyticsRefusal(err error) bool {
	var refusal *analyticsquery.RefusalError
	return errors.As(err, &refusal) ||
		errors.Is(err, apperrors.ErrPermissionDenied) ||
		errors.Is(err, apperrors.ErrNotFound) ||
		errors.Is(err, apperrors.ErrInvalidArgument)
}

// explainedIDs reads the record id off each explained row: the drill-through's
// first column, rendered as a uuid string.
func explainedIDs(out AnalyticsExplanation) ([]ids.UUID, error) {
	found := make([]ids.UUID, 0, len(out.Rows))
	for _, row := range out.Rows {
		raw, ok := row["id"].(string)
		if !ok {
			return nil, fmt.Errorf("compose: an explained row carries no record id")
		}
		id, err := ids.Parse(raw)
		if err != nil {
			return nil, fmt.Errorf("compose: an explained row's record id: %w", err)
		}
		found = append(found, id)
	}
	return found, nil
}
