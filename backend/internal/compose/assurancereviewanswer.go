// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Answering a customer review. "Revived" is confirmed against the deal's own
// evidence, read by the same query and judged by the same assessment the
// nightly check uses. "Wait" and "closed lost" also write the deal, so the
// deal and the answer commit in one transaction or neither does.

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/assurance"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/viability"
)

// AssuranceReviewCheck confirms an answer to a review against its deal.
// "Not relevant" means the deal is closed; "revived" means the deal, judged
// as if no review were open, counts again.
func AssuranceReviewCheck(ctx context.Context, tx pgx.Tx, deal ids.UUID, outcome string, now time.Time) error {
	var status string
	if err := tx.QueryRow(ctx, `SELECT status FROM deal WHERE id = $1`, deal).Scan(&status); err != nil {
		return fmt.Errorf("compose: reading the reviewed deal: %w", err)
	}
	switch {
	case outcome == assurance.OutcomeNotRelevant && status == string(deals.DealOpen):
		return httperr.Validation("outcome", "deal_open", "Close the deal as lost to answer the review this way.")
	case outcome != assurance.OutcomeFixedRecord:
		return nil
	case status != string(deals.DealOpen):
		return httperr.Validation("outcome", "deal_closed", "The deal is closed, so there is nothing to revive.")
	}
	subjects, err := assuranceSubjects(ctx, tx, now, AssuranceCoverage(ctx, tx, now), []ids.UUID{deal})
	if err != nil {
		return err
	}
	if len(subjects) == 0 || subjects[0].Viability == nil {
		return errors.New("compose: the open deal under review was not read back")
	}
	judged := subjects[0].Viability
	if judged.Outcome == viability.OutcomeEligible || judged.Outcome == viability.OutcomeWaiting {
		return nil
	}
	return httperr.Validation("outcome", "not_revived",
		"The deal cannot count again yet: "+missingSupport(judged.Reasons)+".")
}

// missingSupport says in words what keeps a deal from counting.
func missingSupport(reasons []viability.Reason) string {
	words := map[viability.Reason]string{
		viability.ReasonCustomerSilent:       "the customer has not written or met with you for over 60 days",
		viability.ReasonNeverContacted:       "nobody has heard from the customer since the deal was created",
		viability.ReasonCustomerDeclined:     "the customer's last message turned it down",
		viability.ReasonCloseDateUnsupported: "the close date is not confirmed",
		viability.ReasonNoNextStep:           "no next step is planned",
		viability.ReasonCoverageUnknown:      "the mailbox could not be read",
	}
	var said []string
	for _, reason := range reasons {
		if word, ok := words[reason]; ok {
			said = append(said, word)
		}
	}
	if len(said) == 0 {
		return "the check could not confirm it"
	}
	return strings.Join(said, "; ")
}

// reviewAnswerHandlers serve the two review answers that also write the deal.
type reviewAnswerHandlers struct {
	pool      *pgxpool.Pool
	deals     *deals.Store
	assurance *assurance.Store
	now       func() time.Time
}

// answeringAssuranceStore is the assurance store every answer goes through,
// with the signal and review checks the answers need.
func answeringAssuranceStore(pool *pgxpool.Pool) *assurance.Store {
	return assurance.NewStore(InstallationDB(pool)).WithSignalDecision(DecideAssuranceSignal).
		WithReviewCheck(AssuranceReviewCheck)
}

func newReviewAnswerHandlers(pool *pgxpool.Pool) reviewAnswerHandlers {
	return reviewAnswerHandlers{
		pool: pool, deals: deals.NewStore(InstallationDB(pool), DealsInstallation()),
		assurance: answeringAssuranceStore(pool),
		now:       func() time.Time { return time.Now().UTC() },
	}
}

// WaitInputCheck records the customer's wait on the deal and defers the review
// to the same day.
func (h reviewAnswerHandlers) WaitInputCheck(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	var body crmcontracts.WaitInputCheckJSONRequestBody
	if !httperr.Decode(w, r, &body) {
		return
	}
	until := body.Until.Time
	h.answer(w, r, ids.UUID(id), func(ctx context.Context, tx pgx.Tx, deal ids.DealID, active deals.CustomColumns) error {
		if _, err := h.deals.UpdateDealTx(ctx, tx, deal, deals.UpdateDealInput{WaitUntil: &until}, active); err != nil {
			return err
		}
		return h.assurance.ResolveTx(ctx, tx, ids.UUID(id), assurance.Resolution{
			Outcome: assurance.OutcomeRemindLater, RemindAt: &until,
		}, h.now())
	})
}

// CloseLostInputCheck closes the deal as lost and answers the review with
// the same reason.
func (h reviewAnswerHandlers) CloseLostInputCheck(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	var body crmcontracts.CloseLostInputCheckJSONRequestBody
	if !httperr.Decode(w, r, &body) {
		return
	}
	reason := strings.TrimSpace(body.LostReason)
	h.answer(w, r, ids.UUID(id), func(ctx context.Context, tx pgx.Tx, deal ids.DealID, active deals.CustomColumns) error {
		var lost ids.StageID
		err := tx.QueryRow(ctx, `
			SELECT st.id FROM deal d JOIN stage st ON st.pipeline_id = d.pipeline_id
			 WHERE d.id = $1 AND st.semantic = $2 AND st.archived_at IS NULL
			 ORDER BY st.position LIMIT 1`, deal, string(deals.SemanticLost)).Scan(&lost)
		if errors.Is(err, pgx.ErrNoRows) {
			return httperr.Validation("lost_reason", "no_lost_stage", "This deal's pipeline has no lost stage to close it in.")
		}
		if err != nil {
			return fmt.Errorf("compose: finding the lost stage: %w", err)
		}
		// The target is a lost stage, so the move claims no win and carries no
		// win-evidence reason.
		if _, err := h.deals.AdvanceDealTx(ctx, tx, deal, deals.AdvanceDealInput{
			ToStageID: lost, LostReason: &reason, LostReasonCode: body.LostReasonCode, WonWithoutContractReason: nil,
		}, active); err != nil {
			return err
		}
		ends := h.now().Add(assurance.MaxSuppression - time.Minute)
		return h.assurance.ResolveTx(ctx, tx, ids.UUID(id), assurance.Resolution{
			Outcome: assurance.OutcomeNotRelevant, Reason: reason, ExpiresAt: &ends,
		}, h.now())
	})
}

// answer runs one deal-writing answer to an open customer review.
func (h reviewAnswerHandlers) answer(
	w http.ResponseWriter, r *http.Request, review ids.UUID,
	write func(ctx context.Context, tx pgx.Tx, deal ids.DealID, active deals.CustomColumns) error,
) {
	ctx := r.Context()
	// Gated before anything is written: both answers move the deal, so the
	// answerer needs the review's right and the deal's.
	for _, object := range []string{"forecast", "deal"} {
		if err := auth.Require(ctx, object, principal.ActionUpdate); err != nil {
			httperr.Write(w, r, err)
			return
		}
	}
	active, err := h.deals.ActiveDealColumns(ctx)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	err = database.WithWorkspaceTx(ctx, h.pool, func(tx pgx.Tx) error {
		var deal ids.UUID
		err := tx.QueryRow(ctx, `SELECT subject_id FROM assurance_exception
			WHERE id = $1 AND type = $2 AND status = 'open'`, review, assurance.TypeDealViability).Scan(&deal)
		if errors.Is(err, pgx.ErrNoRows) {
			return apperrors.ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("compose: reading the review: %w", err)
		}
		if err := auth.EnsureWritable(ctx, tx, "deal", deal); err != nil {
			return err
		}
		// The deal may have been won or lost since the review was shown; neither
		// answer may move a closed deal.
		var status string
		if err := tx.QueryRow(ctx, `SELECT status FROM deal WHERE id = $1 FOR UPDATE`, deal).Scan(&status); err != nil {
			return fmt.Errorf("compose: reading the reviewed deal: %w", err)
		}
		if status != string(deals.DealOpen) {
			return httperr.Validation("outcome", "deal_closed", "The deal was closed since the review was shown.")
		}
		return write(ctx, tx, ids.From[ids.DealKind](deal), active)
	})
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
