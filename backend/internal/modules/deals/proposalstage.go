// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// A proposal that went out on an open deal asks its owner whether the deal
// should move to the proposal stage. Nothing moves until the owner accepts.
//
// This file owns what the question is, which stage it names, and the write an
// accept performs. Compose owns what raises it: an offer sent, or an outbound
// mail with a proposal attached.

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/namematch"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ProposalStageKind is the approvals kind of the question. The approvals
// module spells it again for its grant and pin tables, and a compose test holds
// the two spellings together.
const ProposalStageKind = "deal_proposal_stage"

// Where a proposal stage question came from.
const (
	ProposalSourceOffer    = "offer"
	ProposalSourceActivity = "activity"
)

// How the target stage was found.
const (
	ProposalMatchedByStageName     = "stage_name"
	ProposalMatchedByExitCriterion = "exit_criterion"
)

// ProposalStageTTLDays is how long an unanswered question stays open.
const ProposalStageTTLDays = 14

// ProposalStageMove is the staged proposed_change. The ids decide the move;
// the names are what the card shows, read when the question was raised.
type ProposalStageMove struct {
	DealID            ids.UUID   `json:"deal_id"`
	DealName          string     `json:"deal_name"`
	FromStageID       ids.UUID   `json:"from_stage_id"`
	ToStageID         ids.UUID   `json:"to_stage_id"`
	ToStageName       string     `json:"to_stage_name"`
	SkippedStageIDs   []ids.UUID `json:"skipped_stage_ids"`
	SkippedStageNames []string   `json:"skipped_stage_names"`
	SourceType        string     `json:"source_type"`
	SourceID          ids.UUID   `json:"source_id"`
	MatchedBy         string     `json:"matched_by"`
}

// proposalWords name a proposal in German, English and Vietnamese, folded to
// unaccented lower-case words. A stage called "Contract" is not the proposal
// stage, so contract words are not here.
var proposalWords = []string{"proposal", "quote", "quotation", "offer", "angebot", "bao gia", "de xuat"}

// laterWords say the proposal is already answered, so a stage carrying one is
// a later stage than the proposal stage. They outrank proposalWords in a name.
var laterWords = []string{"accepted", "signed", "won", "angenommen", "unterschrieben", "chap nhan", "da ky"}

// sentWords say a proposal left. A criterion needs one beside a proposal word.
var sentWords = []string{"sent", "send", "verschickt", "gesendet", "versendet", "da gui"}

// proposalStageCandidate is one live stage of the deal's pipeline.
type proposalStageCandidate struct {
	id       ids.UUID
	position int
	open     bool
	names    []string
	criteria []string
}

// proposalPick is the stage chosen and how it was found.
type proposalPick struct {
	stage     proposalStageCandidate
	matchedBy string
}

// foldWords is the text as unaccented lower-case words framed by spaces, so a
// vocabulary entry matches whole words only. The Vietnamese đ is a letter of
// its own that folding keeps, so it is mapped by hand.
func foldWords(text string) string {
	folded := strings.NewReplacer("đ", "d", "Đ", "d").Replace(namematch.Fold(text))
	var out strings.Builder
	out.WriteByte(' ')
	space := true
	for _, r := range folded {
		if r == ' ' || !isWordRune(r) {
			if !space {
				out.WriteByte(' ')
			}
			space = true
			continue
		}
		out.WriteRune(r)
		space = false
	}
	if !space {
		out.WriteByte(' ')
	}
	return out.String()
}

func isWordRune(r rune) bool {
	return r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r > 127
}

func hasAny(words string, vocabulary []string) bool {
	return slices.ContainsFunc(vocabulary, func(word string) bool {
		return strings.Contains(words, " "+word+" ")
	})
}

func nameIsProposalStage(names []string) bool {
	for _, name := range names {
		words := foldWords(name)
		if hasAny(words, proposalWords) && !hasAny(words, laterWords) {
			return true
		}
	}
	return false
}

func criterionSaysProposalSent(texts []string) bool {
	for _, text := range texts {
		words := foldWords(text)
		if hasAny(words, proposalWords) && hasAny(words, sentWords) {
			return true
		}
	}
	return false
}

// pickProposalStage answers the stage a sent proposal moves the deal to. The
// stages are in pipeline order. A stage whose name is a proposal name is the
// target itself. A criterion "proposal sent" on stage X means the proposal
// leaves X, so the target is the open stage after it. The earliest match wins;
// a match at or before the deal's own stage means the deal is already there.
func pickProposalStage(stages []proposalStageCandidate, current ids.UUID) (proposalPick, bool) {
	at := slices.IndexFunc(stages, func(s proposalStageCandidate) bool { return s.id == current })
	if at < 0 {
		return proposalPick{}, false
	}
	var best *proposalPick
	consider := func(stage proposalStageCandidate, matchedBy string) {
		if best == nil || stage.position < best.stage.position {
			best = &proposalPick{stage: stage, matchedBy: matchedBy}
		}
	}
	for i, stage := range stages {
		if nameIsProposalStage(stage.names) {
			consider(stage, ProposalMatchedByStageName)
		}
		if criterionSaysProposalSent(stage.criteria) && i+1 < len(stages) {
			consider(stages[i+1], ProposalMatchedByExitCriterion)
		}
	}
	if best == nil || best.stage.position <= stages[at].position || !best.stage.open {
		return proposalPick{}, false
	}
	return *best, true
}

// ProposalStageFor answers the question a sent proposal puts on a deal. It
// answers false when there is nothing to ask. That is so when the deal is
// closed, archived or on legal hold, its pipeline is retired, or no later open
// stage matches.
func ProposalStageFor(ctx context.Context, tx pgx.Tx, dealID ids.DealID) (ProposalStageMove, bool, error) {
	var name string
	var status string
	var hold, retired bool
	var stage, pipeline ids.UUID
	err := tx.QueryRow(ctx, `
		SELECT d.name, d.status, d.legal_hold, d.stage_id, d.pipeline_id, p.archived_at IS NOT NULL
		  FROM deal d JOIN pipeline p ON p.id = d.pipeline_id
		 WHERE d.id = $1 AND d.archived_at IS NULL`, dealID).Scan(&name, &status, &hold, &stage, &pipeline, &retired)
	if errors.Is(err, pgx.ErrNoRows) {
		return ProposalStageMove{}, false, nil
	}
	if err != nil {
		return ProposalStageMove{}, false, fmt.Errorf("deals: reading the deal a proposal went out on: %w", err)
	}
	if DealStatus(status) != DealOpen || hold || retired {
		return ProposalStageMove{}, false, nil
	}
	stages, err := readProposalStageCandidates(ctx, tx, pipeline)
	if err != nil {
		return ProposalStageMove{}, false, err
	}
	pick, ok := pickProposalStage(stages, stage)
	if !ok {
		return ProposalStageMove{}, false, nil
	}
	move := ProposalStageMove{
		DealID: dealID.UUID, DealName: name, FromStageID: stage, ToStageID: pick.stage.id,
		ToStageName: pick.stage.names[0], MatchedBy: pick.matchedBy,
	}
	from := stages[slices.IndexFunc(stages, func(s proposalStageCandidate) bool { return s.id == stage })]
	for _, between := range stages {
		if between.position > from.position && between.position < pick.stage.position {
			move.SkippedStageIDs = append(move.SkippedStageIDs, between.id)
			move.SkippedStageNames = append(move.SkippedStageNames, between.names[0])
		}
	}
	return move, true, nil
}

// readProposalStageCandidates reads the live stages of a pipeline in order.
// Each carries every name it is known by and the text of its live criteria.
func readProposalStageCandidates(ctx context.Context, tx pgx.Tx, pipeline ids.UUID) ([]proposalStageCandidate, error) {
	rows, err := tx.Query(ctx, `
		SELECT s.id, s.position, s.semantic, s.name, s.name_translations,
		       coalesce((SELECT array_agg(c.label || ' ' || coalesce(c.hint, '') ORDER BY c.position)
		                   FROM stage_exit_criterion c
		                  WHERE c.stage_id = s.id AND c.archived_at IS NULL), '{}')
		  FROM stage s
		 WHERE s.pipeline_id = $1 AND s.archived_at IS NULL
		 ORDER BY s.position`, pipeline)
	if err != nil {
		return nil, fmt.Errorf("deals: reading the stages a proposal may move a deal to: %w", err)
	}
	stages, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (proposalStageCandidate, error) {
		var s proposalStageCandidate
		var semantic, name string
		var translations map[string]string
		if err := row.Scan(&s.id, &s.position, &semantic, &name, &translations, &s.criteria); err != nil {
			return s, err
		}
		s.open = StageSemantic(semantic) == SemanticOpen
		s.names = []string{name}
		for _, lang := range slices.Sorted(maps.Keys(translations)) {
			s.names = append(s.names, translations[lang])
		}
		return s, nil
	})
	if err != nil {
		return nil, fmt.Errorf("deals: reading the stages a proposal may move a deal to: %w", err)
	}
	return stages, nil
}

// ApplyProposalStageTx writes the accepted move in the caller's transaction,
// through the advance every stage move takes. It answers why nothing moved, or
// an empty reason once the deal has moved.
//
// The deal must still be open and off legal hold, and the target must still be
// a later open stage. The move's own gates may refuse it, legal review
// included. That is a reason too: the move runs in a savepoint.
func (s *Store) ApplyProposalStageTx(
	ctx context.Context, tx pgx.Tx, move ProposalStageMove, approvalID ids.ApprovalID, active CustomColumns,
) (string, error) {
	deal := ids.From[ids.DealKind](move.DealID)
	if err := lockThenEnsureWritable(ctx, tx, deal); err != nil {
		return "", err
	}
	reason, err := proposalStillApplies(ctx, tx, move)
	if err != nil || reason != "" {
		return reason, err
	}
	approval := approvalID.UUID
	_, refused, err := s.advanceInSavepoint(ctx, tx, deal, AdvanceDealInput{
		ToStageID: ids.From[ids.StageKind](move.ToStageID), ApprovalID: &approval,
	}, active.cols)
	if err != nil {
		return "", err
	}
	if refused {
		return ProposalStageRefusedReason, nil
	}
	return "", nil
}

// ProposalStageStillApplies is the read-only half of the accept: why the move
// would write nothing now, or an empty reason when it would go ahead.
func ProposalStageStillApplies(ctx context.Context, tx pgx.Tx, move ProposalStageMove) (string, error) {
	return proposalStillApplies(ctx, tx, move)
}

// The reasons an accept writes nothing.
const (
	ProposalStageGoneReason    = "This deal is closed, archived or on legal hold, so nothing was moved."
	ProposalStageTargetReason  = "The stages of this pipeline changed since the question was asked, so nothing was moved."
	ProposalStageRefusedReason = "The deal's stage rules do not let it move to that stage yet, such as a pending legal review, so nothing was moved."
)

func proposalStillApplies(ctx context.Context, tx pgx.Tx, move ProposalStageMove) (string, error) {
	var status string
	var hold bool
	var stage ids.UUID
	err := tx.QueryRow(ctx, `SELECT status, legal_hold, stage_id FROM deal WHERE id = $1 AND archived_at IS NULL`,
		move.DealID).Scan(&status, &hold, &stage)
	if errors.Is(err, pgx.ErrNoRows) {
		return ProposalStageGoneReason, nil
	}
	if err != nil {
		return "", fmt.Errorf("deals: rereading the deal a proposal question is about: %w", err)
	}
	if DealStatus(status) != DealOpen || hold {
		return ProposalStageGoneReason, nil
	}
	_, ok, err := lockStagesMovedPast(ctx, tx, ids.From[ids.DealKind](move.DealID), stage, ids.From[ids.StageKind](move.ToStageID))
	if err != nil {
		return "", err
	}
	if !ok {
		return ProposalStageTargetReason, nil
	}
	return "", nil
}

// ProposalStageOutdated reports whether a waiting question has nothing left to
// ask. It has none once the deal reached or passed the stage, closed, was
// archived, or the stage is gone.
func ProposalStageOutdated(ctx context.Context, tx pgx.Tx, move ProposalStageMove) (bool, error) {
	var outdated bool
	err := tx.QueryRow(ctx, `
		SELECT d.archived_at IS NOT NULL OR d.status <> 'open' OR target.id IS NULL OR cur.position >= target.position
		  FROM deal d
		  JOIN stage cur ON cur.id = d.stage_id
		  LEFT JOIN stage target ON target.id = $2 AND target.archived_at IS NULL
		 WHERE d.id = $1`, move.DealID, move.ToStageID).Scan(&outdated)
	if errors.Is(err, pgx.ErrNoRows) {
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("deals: reading whether a proposal question is outdated: %w", err)
	}
	return outdated, nil
}
