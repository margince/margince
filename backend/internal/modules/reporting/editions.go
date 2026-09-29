// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package reporting

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func (s *Service) edition(ctx context.Context, tx pgx.Tx, id ids.UUID) (crmcontracts.ReportingEdition, []Fact, error) {
	var out crmcontracts.ReportingEdition
	var b bindings
	audience, err := audienceClause(ctx, &b)
	if err != nil {
		return out, nil, err
	}
	// A later audience expansion cannot make an originally private edition public.
	published := "(SELECT owner_id,publication_audience AS audience,publication_team_id AS audience_team_id FROM report_edition WHERE id=" + b.add(id) + ") d"
	var permitted bool
	if err := tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM "+published+" WHERE "+audience+")", b.values...).Scan(&permitted); err != nil {
		return out, nil, err
	}
	if !permitted {
		return out, nil, storedError(pgx.ErrNoRows)
	}
	b = bindings{}
	var redacted *time.Time
	err = tx.QueryRow(ctx, "SELECT manifest,redacted_at FROM report_edition WHERE id="+b.add(id), b.values...).Scan(&out, &redacted)
	if err != nil {
		return out, nil, storedError(err)
	}
	if _, err := s.report(ctx, tx, ids.UUID(out.ReportId), false); err != nil {
		return out, nil, err
	}
	b = bindings{}
	rows, err := tx.Query(ctx, "SELECT fact FROM report_edition_contribution WHERE edition_id="+b.add(id)+" ORDER BY metric,context_id,contribution_key", b.values...)
	if err != nil {
		return out, nil, err
	}
	facts, err := pgx.CollectRows(rows, pgx.RowTo[Fact])
	if err != nil {
		return out, nil, err
	}
	visible, withheld, err := s.authority.Visible(ctx, tx, facts)
	if err != nil {
		return out, nil, err
	}
	contextWithheld, err := s.editionContextWithheld(ctx, tx, out.Evaluation.Context.Scope)
	if err != nil {
		return out, nil, err
	}
	withheld = withheld || contextWithheld
	out.Redacted = redacted != nil
	out.Withheld = withheld || out.Redacted
	if out.Withheld {
		out.Evaluation, err = s.evaluator.ProjectFrozen(out.Evaluation, visible)
		if err != nil {
			return out, nil, err
		}
	}
	return out, visible, nil
}

// GetEdition rechecks current disclosure authority before returning frozen data.
func (s *Service) GetEdition(ctx context.Context, id ids.UUID) (crmcontracts.ReportingEdition, error) {
	var out crmcontracts.ReportingEdition
	err := s.store.db.TxIsolated(ctx, pgx.RepeatableRead, func(tx pgx.Tx) error { var err error; out, err = s.GetEditionTx(ctx, tx, id); return err })
	return out, err
}

// GetEditionTx shares the caller’s snapshot while applying edition read authority.
func (s *Service) GetEditionTx(ctx context.Context, tx pgx.Tx, id ids.UUID) (crmcontracts.ReportingEdition, error) {
	if err := auth.Require(ctx, "report_edition", principal.ActionRead); err != nil {
		return crmcontracts.ReportingEdition{}, err
	}
	out, _, err := s.edition(ctx, tx, id)
	return out, err
}

// ListEditions projects retained editions through the current reader’s permissions.
func (s *Service) ListEditions(ctx context.Context, reportID ids.UUID, after *ids.UUID, limit int) (crmcontracts.ReportingEditionList, error) {
	out := crmcontracts.ReportingEditionList{Data: []crmcontracts.ReportingEdition{}}
	if err := auth.Require(ctx, "report_edition", principal.ActionRead); err != nil {
		return out, err
	}
	// Every entry re-authorizes its evidence; bound the multiplied read cost.
	limit = max(1, min(5, limit))
	err := s.store.db.TxIsolated(ctx, pgx.RepeatableRead, func(tx pgx.Tx) error {
		if _, err := s.report(ctx, tx, reportID, false); err != nil {
			return err
		}
		var b bindings
		audience, err := audienceClause(ctx, &b)
		if err != nil {
			return err
		}
		where := "report_id=" + b.add(reportID)
		if after != nil {
			where += " AND id<" + b.add(*after)
		}
		query := "SELECT d.id FROM (SELECT id,owner_id,publication_audience AS audience,publication_team_id AS audience_team_id FROM report_edition WHERE " + where + ") d WHERE " + audience + " ORDER BY d.id DESC LIMIT " + b.add(limit+1)
		rows, err := tx.Query(ctx, query, b.values...)
		if err != nil {
			return err
		}
		editionIDs, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
		if err != nil {
			return err
		}
		if len(editionIDs) > limit {
			cursor := editionIDs[limit-1].String()
			out.NextCursor = &cursor
			editionIDs = editionIDs[:limit]
		}
		for _, id := range editionIDs {
			edition, _, err := s.edition(ctx, tx, id)
			if err != nil {
				return err
			}
			out.Data = append(out.Data, edition)
		}
		return nil
	})
	return out, err
}

// EditionEvidence returns only currently authorized contributions from the saved capture.
func (s *Service) EditionEvidence(ctx context.Context, id ids.UUID, metric crmcontracts.ReportingMetricID, contextID, group string, through *time.Time, cursor *string, limit int) (crmcontracts.ReportingEvidence, error) {
	if err := auth.Require(ctx, "report_edition", principal.ActionRead); err != nil {
		return crmcontracts.ReportingEvidence{}, err
	}
	var out crmcontracts.ReportingEvidence
	err := s.store.db.TxIsolated(ctx, pgx.RepeatableRead, func(tx pgx.Tx) error {
		edition, facts, err := s.edition(ctx, tx, id)
		if err != nil {
			return err
		}
		out, err = evidencePage(edition.Evaluation.Context, facts, metric, contextID, group, through, cursor, limit)
		return err
	})
	return out, err
}

// Compare withholds deltas when edition contexts cannot be compared honestly.
func (s *Service) Compare(ctx context.Context, left, right ids.UUID) (crmcontracts.ReportingComparison, error) {
	out := crmcontracts.ReportingComparison{Deltas: []crmcontracts.ReportingDelta{}}
	if err := auth.Require(ctx, "report_edition", principal.ActionRead); err != nil {
		return out, err
	}
	err := s.store.db.TxIsolated(ctx, pgx.RepeatableRead, func(tx pgx.Tx) error {
		a, _, err := s.edition(ctx, tx, left)
		if err != nil {
			return err
		}
		b, _, err := s.edition(ctx, tx, right)
		if err != nil {
			return err
		}
		out.Left, out.Right = a, b
		reason := comparisonReason(a, b)
		out.Compatible = reason == ""
		if reason != "" {
			out.Reason = &reason
		} else {
			out.Deltas = comparisonDeltas(a, b)
		}
		return nil
	})
	return out, err
}

func (s *Service) editionContextWithheld(ctx context.Context, tx pgx.Tx, scope crmcontracts.ReportingScope) (bool, error) {
	if _, scopeErr := s.authority.Scope(ctx, tx, scope, false); scopeErr != nil {
		if !errors.Is(scopeErr, apperrors.ErrNotFound) && !errors.Is(scopeErr, apperrors.ErrPermissionDenied) {
			return false, scopeErr
		}
		return true, nil
	}
	if !auth.Allows(ctx, "sales_target", principal.ActionRead) {
		return true, nil
	}
	return false, nil
}
