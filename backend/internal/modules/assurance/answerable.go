// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package assurance

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// AnswerableFindings answers which findings the caller may answer now, by the
// rules Resolve admits an answer under. Deals are asked in one statement; a
// signal or a call is asked by its own rule, and a list holds few of those.
func AnswerableFindings(ctx context.Context, tx pgx.Tx, found []Exception) (map[ids.UUID]bool, error) {
	out := map[ids.UUID]bool{}
	actor, refusal := answeringActor(ctx)
	allowed, err := admitted(refusal)
	if err != nil {
		return nil, err
	}
	// The answer routes take a human session only, so an agent reading the
	// list is shown each finding to read.
	if !allowed || actor.Type != principal.PrincipalHuman {
		return out, nil
	}
	deals, err := answerableBesideDeals(ctx, tx, found, out)
	if err != nil {
		return nil, err
	}
	updates, err := admitted(auth.Require(ctx, subjectKindDeal, principal.ActionUpdate))
	if err != nil {
		return nil, err
	}
	if len(deals) == 0 || !updates {
		return out, nil
	}
	writable, err := auth.WritableSubset(ctx, tx, subjectKindDeal, deals)
	if err != nil {
		return nil, err
	}
	for _, e := range found {
		if e.SubjectKind == subjectKindDeal {
			out[e.ID] = writable[e.SubjectID]
		}
	}
	return out, nil
}

// answerableBesideDeals asks each signal and call by its own rule, filling
// out, and answers the deals left for one statement.
func answerableBesideDeals(ctx context.Context, tx pgx.Tx, found []Exception, out map[ids.UUID]bool) ([]ids.UUID, error) {
	var deals []ids.UUID
	var err error
	for _, e := range found {
		switch e.SubjectKind {
		case subjectKindDeal:
			deals = append(deals, e.SubjectID)
		case subjectKindSignal:
			if out[e.ID], err = admitted(ensureSignalAnswerable(ctx, tx, e.SubjectID)); err != nil {
				return nil, err
			}
		case SubjectKindForecastCall:
			if auth.Require(ctx, "forecast", principal.ActionCreate) != nil {
				continue
			}
			if out[e.ID], err = admitted(ensureCallAnswerable(ctx, tx, e.SubjectID)); err != nil {
				return nil, err
			}
		}
	}
	return deals, nil
}

// admitted reads a refusal as "no" and passes any other failure on.
func admitted(err error) (bool, error) {
	if errors.Is(err, apperrors.ErrNotFound) || errors.Is(err, apperrors.ErrPermissionDenied) {
		return false, nil
	}
	return err == nil, err
}
