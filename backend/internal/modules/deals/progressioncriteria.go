// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ValidateProgressionCriteriaTx holds the checklist against its writers until the move commits.
func (s *Store) ValidateProgressionCriteriaTx(ctx context.Context, tx pgx.Tx, change StageProgressionChange) error {
	if err := auth.Require(ctx, "pipeline", principal.ActionRead); err != nil {
		return err
	}
	var stageID ids.StageID
	// Criteria writers take this stage FOR UPDATE; concurrent decisions may share the read lock.
	err := tx.QueryRow(ctx, `SELECT id FROM stage WHERE id = $1 AND archived_at IS NULL FOR SHARE`, change.FromStageID).Scan(&stageID)
	if errors.Is(err, pgx.ErrNoRows) {
		return apperrors.ErrNotFound
	}
	if err != nil {
		return err
	}
	current, err := readCriteria(ctx, tx, stageID, storekit.LiveOnly)
	if err != nil {
		return err
	}
	if err := matchProgressionCriteria(change.Criteria, current); err != nil {
		return err
	}
	return s.refuseUnsettledProgressionCriteria(ctx, tx, change)
}

// refuseUnsettledProgressionCriteria holds every criterion the card calls met
// to the ledger as it stands now. A claim refuted since leaves the card stale.
// So does a buyer milestone resting on activity evidence our own side wrote.
// With the own domains bound, the author is judged against them now. A domain
// change the evidence pass has not reached yet then cannot move the deal.
func (s *Store) refuseUnsettledProgressionCriteria(ctx context.Context, tx pgx.Tx, change StageProgressionChange) error {
	domains := &lazyOwnDomains{own: s.ownDomains}
	for _, criterion := range change.Criteria {
		if !criterion.Met {
			continue
		}
		settled, err := s.criterionSettledNow(ctx, tx, change.DealID, criterion.CriterionID, domains)
		if err != nil {
			return err
		}
		if !settled {
			return apperrors.ErrVersionSkew
		}
	}
	return nil
}

type settlingCandidate struct {
	Kind        string
	SourceType  string
	SourceID    ids.UUID
	Side        string
	ExtractedBy string
}

func (s *Store) criterionSettledNow(
	ctx context.Context, tx pgx.Tx, dealID ids.DealID, criterionID ids.ExitCriterionID, domains *lazyOwnDomains,
) (bool, error) {
	rows, err := tx.Query(ctx, `SELECT c.kind, e.source_type, e.source_id, e.author_side, e.extracted_by
		  FROM deal_stage_evidence e JOIN stage_exit_criterion c ON c.id = e.criterion_id
		 WHERE e.deal_id = $1 AND e.criterion_id = $2 AND e.met AND e.refuted_at IS NULL`,
		dealID.UUID, criterionID.UUID)
	if err != nil {
		return false, fmt.Errorf("deals: rereading a met criterion's evidence: %w", err)
	}
	candidates, err := pgx.CollectRows(rows, pgx.RowToStructByPos[settlingCandidate])
	if err != nil {
		return false, fmt.Errorf("deals: rereading a met criterion's evidence: %w", err)
	}
	for _, c := range candidates {
		if domains.own != nil && c.SourceType == SourceActivity && c.ExtractedBy == ExtractedByDeterministic &&
			CriterionKind(c.Kind) == CriterionEventHeld {
			held, found, err := meetingStillProvesHeld(principal.SystemActing(ctx, stageMoveCheckActor), tx, c.SourceID, domains.own)
			if err != nil {
				return false, err
			}
			if found && !held {
				continue
			}
		}
		side := AuthorSide(c.Side)
		if domains.own != nil && c.SourceType == SourceActivity && CriterionKind(c.Kind).BuyerMilestone() {
			own, err := domains.read(ctx, tx)
			if err != nil {
				return false, err
			}
			judged, found, err := judgeActivityAuthor(ctx, tx, c.SourceID, own)
			if err != nil {
				return false, err
			}
			if found {
				side = judged
			}
		}
		if SettlesBuyerMilestone(CriterionKind(c.Kind), side) {
			return true, nil
		}
	}
	return false, nil
}

func matchProgressionCriteria(proposed []ProposedCriterion, current []crmcontracts.StageExitCriterion) error {
	if len(proposed) == 0 {
		return apperrors.ErrInvalidArgument
	}
	if len(proposed) != len(current) {
		return apperrors.ErrVersionSkew
	}
	byID := make(map[ids.UUID]crmcontracts.StageExitCriterion, len(current))
	for _, criterion := range current {
		byID[ids.UUID(criterion.Id)] = criterion
	}
	for _, criterion := range proposed {
		if criterion.Version <= 0 {
			return apperrors.ErrInvalidArgument
		}
		live, ok := byID[criterion.CriterionID.UUID]
		if !ok || live.Version == nil || *live.Version != criterion.Version || live.Key != criterion.Key || live.Label != criterion.Label || live.Required != criterion.Required {
			return apperrors.ErrVersionSkew
		}
		delete(byID, criterion.CriterionID.UUID)
	}
	return nil
}

// stageMoveCheckActor reads the own domains for the stage-move check. It is
// the principal of the evidence authorship pass, which judges the same thing.
const stageMoveCheckActor = "system:evidence_authorship"

// lazyOwnDomains reads the own domains once, and only when a buyer milestone
// rests on activity evidence. A card resting on a contract reads none.
type lazyOwnDomains struct {
	own     OwnDomainReader
	domains []string
	done    bool
}

func (l *lazyOwnDomains) read(ctx context.Context, tx pgx.Tx) ([]string, error) {
	if !l.done {
		// The own domains are installation settings, not anybody's records,
		// so an approver without activity read is still held to them.
		domains, err := l.own.Domains(principal.SystemActing(ctx, stageMoveCheckActor), tx)
		if err != nil {
			return nil, err
		}
		l.domains, l.done = domains, true
	}
	return l.domains, nil
}
