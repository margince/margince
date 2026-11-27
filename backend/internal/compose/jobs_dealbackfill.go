// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The stakeholder mail backfill: new phases of the hourly deal refile pass.
// They file old mail of a deal's stakeholders once, then offer back the silent
// closes that missing mail caused. It is not a data task because it asks the
// deal topic check, and a data task may not call a model.
// Its progress lives in installation_state, so a pass that ran out of attempts
// resumes where it stopped and asks the model nothing twice.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
	"github.com/margince/margince/backend/internal/shared/kernel/relstrength"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
)

// backfill runs one bounded step of the backfill when the pass asks for it,
// and reports whether work remains, so the worker snoozes.
func (w *capturedDealRefileWorker) backfill(ctx context.Context, args CapturedDealRefileArgs) (bool, error) {
	if !args.Scope.Backfill {
		return false, nil
	}
	return w.backfillStep(ctx, args)
}

// backfillStep is the state one step reads and writes back, with the model
// calls the pass had made when the step began.
type backfillStep struct {
	state      capture.StakeholderBackfillState
	startCalls int
}

// backfillStep does at most w.calls topic checks of the backfill and reports
// whether more work remains.
func (w *capturedDealRefileWorker) backfillStep(ctx context.Context, args CapturedDealRefileArgs) (bool, error) {
	state, err := w.backfillState(ctx, args)
	if err != nil || state.Finished() {
		return false, err
	}
	step := &backfillStep{state: state, startCalls: state.ModelCalls}
	var more bool
	if !step.state.ClosesDone {
		more, err = w.backfillClosePhase(ctx, step)
	} else {
		more, err = w.backfillOpenPhase(ctx, step)
	}
	if err != nil {
		return false, err
	}
	if !more {
		return false, w.finishBackfill(ctx, step.state)
	}
	return true, w.storeBackfill(ctx, step.state)
}

// backfillState reads the stored state, or starts the pass. The window is the
// stored deals.silent_close_days. The ceiling and instant are the pass's own,
// frozen so a resumed pass keeps the range it began with.
func (w *capturedDealRefileWorker) backfillState(ctx context.Context, args CapturedDealRefileArgs) (capture.StakeholderBackfillState, error) {
	var state capture.StakeholderBackfillState
	err := w.db.Tx(ctx, func(tx pgx.Tx) error {
		stored, found, err := storekit.InstallationStateTx[capture.StakeholderBackfillState](ctx, tx, storekit.StateStakeholderMailBackfill)
		if err != nil {
			return err
		}
		if found && stored.Version == capture.StakeholderBackfillVersion {
			state = stored
			return nil
		}
		window, err := config.StoredApplyTx(ctx, tx, config.Bound(), deals.SilentCloseDays)
		if err != nil {
			return fmt.Errorf("compose: reading the silent close threshold: %w", err)
		}
		state = capture.StakeholderBackfillState{
			Version: capture.StakeholderBackfillVersion, Window: window, AsOf: args.AsOf, Ceiling: args.Ceiling,
		}
		return storekit.StoreInstallationStateTx(ctx, tx, storekit.StateStakeholderMailBackfill, state)
	})
	return state, err
}

func (w *capturedDealRefileWorker) storeBackfill(ctx context.Context, state capture.StakeholderBackfillState) error {
	return w.db.Tx(ctx, func(tx pgx.Tx) error {
		return storekit.StoreInstallationStateTx(ctx, tx, storekit.StateStakeholderMailBackfill, state)
	})
}

// finishBackfill stamps the pass finished and logs what it did, in one transaction.
func (w *capturedDealRefileWorker) finishBackfill(ctx context.Context, state capture.StakeholderBackfillState) error {
	finished := w.now()
	state.FinishedAt = &finished
	return w.db.Tx(ctx, func(tx pgx.Tx) error {
		if err := storekit.StoreInstallationStateTx(ctx, tx, storekit.StateStakeholderMailBackfill, state); err != nil {
			return err
		}
		_, err := storekit.LogSystem(ctx, tx, "capture_stakeholder_backfill_done", map[string]any{
			"window_days": state.Window, "inspected": state.Inspected, "filed": state.Filed,
			"model_calls": state.ModelCalls, "offered_back": state.Offered,
		})
		return err
	})
}

// backfillOpenPhase files the unfiled mail of seated contacts on open deals.
func (w *capturedDealRefileWorker) backfillOpenPhase(ctx context.Context, step *backfillStep) (bool, error) {
	state := &step.state
	var scanned []ids.UUID
	err := w.db.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		scanned, err = w.repair.StakeholderBackfillCandidatesTx(ctx, tx, state.AsOf, state.Window, state.OpenAfter, state.Ceiling, w.page)
		return err
	})
	if err != nil {
		return false, err
	}
	repair := w.repairAt(state.AsOf)
	for _, mail := range scanned {
		result, err := repair.BackfillStakeholderMail(ctx, w.db, ids.From[ids.ActivityKind](mail), state.AsOf, state.Window)
		if err := w.backfillFault(ctx, mail, deferredAsFault(result, err)); err != nil {
			return false, errors.Join(err, w.storeBackfill(ctx, *state))
		}
		state.OpenAfter = mail
		state.Count(result)
		if state.ModelCalls-step.startCalls >= w.calls {
			return true, nil
		}
	}
	return len(scanned) == w.page, nil
}

// backfillClosePhase works one close for silence at a time. It files the mail
// the deal missed, then offers the close back when that mail shows the customer
// was answering. The state keeps the position inside the close in hand.
func (w *capturedDealRefileWorker) backfillClosePhase(ctx context.Context, step *backfillStep) (bool, error) {
	state := &step.state
	since := state.AsOf.AddDate(0, 0, -state.Window)
	var closes []deals.SilentClose
	err := w.db.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		closes, err = w.closes.SilentClosesTx(ctx, tx, since, state.ClosesAfter, 1)
		return err
	})
	if err != nil {
		return false, err
	}
	if len(closes) == 0 {
		state.ClosesDone = true
		return true, nil
	}
	closed := closes[0]
	var scanned []ids.UUID
	err = w.db.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		scanned, err = w.repair.SilentCloseCandidatesTx(ctx, tx, closed.Deal.UUID, closed.ClosedAt, state.Window, state.MailAfter, w.page)
		return err
	})
	if err != nil {
		return false, err
	}
	repair := w.repairAt(state.AsOf)
	for _, mail := range scanned {
		result, err := repair.FileOnSilentClose(ctx, w.db, ids.From[ids.ActivityKind](mail), closed.Deal.UUID, closed.ClosedAt, state.Window)
		if err := w.backfillFault(ctx, mail, deferredAsFault(result, err)); err != nil {
			return false, errors.Join(err, w.storeBackfill(ctx, *state))
		}
		state.MailAfter = mail
		state.Count(result)
		if result.Filed {
			state.FiledOnClose++
		}
		if state.ModelCalls-step.startCalls >= w.calls {
			return true, nil
		}
	}
	if len(scanned) == w.page {
		return true, nil
	}
	if state.FiledOnClose > 0 {
		offered, err := w.offerBack(ctx, closed, state.Window)
		if err != nil {
			return false, err
		}
		if offered {
			state.Offered++
		}
	}
	state.ClosesAfter, state.MailAfter, state.FiledOnClose = closed.CorrectionID, ids.Nil, 0
	return true, nil
}

// errBackfillDeferred is a mail the model could not judge yet.
var errBackfillDeferred = errors.New("compose: the deal topic check is deferred, so the mail is tried again next pass")

// deferredAsFault turns a deferred answer into the transient fault it is.
func deferredAsFault(result capture.BackfillResult, err error) error {
	if err == nil && result.Deferred {
		return errBackfillDeferred
	}
	return err
}

// backfillFault logs a fault of one mail. A mail that is gone is permanent, so
// the pass skips it. Any other fault, a model outage included, is transient.
// It is returned, so the pass stops with its position kept. The next hourly
// pass retries the same mail, and the pass finishes only when none is left.
func (w *capturedDealRefileWorker) backfillFault(ctx context.Context, mail ids.UUID, cause error) error {
	if cause == nil {
		return nil
	}
	logErr := w.db.Tx(ctx, func(tx pgx.Tx) error {
		_, err := storekit.LogSystem(ctx, tx, "capture_stakeholder_backfill_fault", map[string]any{"activity_id": mail, "error": cause.Error()})
		return err
	})
	if errors.Is(cause, apperrors.ErrNotFound) {
		return logErr
	}
	return errors.Join(cause, logErr)
}

// offerBack puts a close back on its owner's Home when, with the mail now
// filed, the customer had written within the threshold before the close.
func (w *capturedDealRefileWorker) offerBack(ctx context.Context, closed deals.SilentClose, threshold int) (bool, error) {
	var premature bool
	err := w.db.Tx(ctx, func(tx pgx.Tx) error {
		zone, err := installationZoneTx(ctx, tx)
		if err != nil {
			return err
		}
		var progress *time.Time
		var precision *string
		keptOpen := relstrength.DealKeptOpenSQL("a", "al.deal_id")
		err = tx.QueryRow(ctx, `SELECT max(a.occurred_at), `+occurredday.LatestPrecisionSQL("a", "")+`
			 FROM activity a JOIN activity_link al ON al.activity_id = a.id
			 WHERE al.deal_id = $1 AND a.archived_at IS NULL AND `+customerProgressAnyAudienceSQL("a", keptOpen, "$2"),
			closed.Deal, closed.ClosedAt).Scan(&progress, &precision)
		if err != nil {
			return fmt.Errorf("compose: reading the customer's mail before a close: %w", err)
		}
		premature = progress != nil && daysSince(progress, occurredday.IsStatedDay(precision), closed.ClosedAt, zone) <= threshold
		return nil
	})
	if err != nil || !premature {
		return false, err
	}
	return w.closes.ReofferClose(ctx, closed, func(ctx context.Context, tx pgx.Tx) error {
		return w.closer.recordOfferedBack(ctx, tx, closed.Deal)
	})
}

// recordOfferedBack writes on the deal's timeline why its close is on Home again.
func (c silentDealCloser) recordOfferedBack(ctx context.Context, tx pgx.Tx, deal ids.DealID) error {
	subject := systemtext.Literal("deal.lost_reoffered")
	body := strings.Join([]string{subject + ".", systemtext.Literal("deal.lost_undo")}, " ")
	component := systemtext.Sequence(systemtext.Sentence("deal.lost_reoffered"), systemtext.Sentence("deal.lost_undo"))
	component.Layout = "sentences"
	_, _, err := c.activities.LogActivityTx(ctx, tx, activities.LogActivityInput{
		SystemText: &systemtext.Message{Subject: systemtext.Sentence("deal.lost_reoffered"), Body: component},
		Kind:       string(crmcontracts.ActivityKindNote), Subject: &subject, Body: &body, Origin: activities.OriginSystemRemediation,
		Links: []activities.ActivityLinkInput{{EntityType: entityTypeDeal, EntityID: deal.UUID}},
	})
	if err != nil {
		return fmt.Errorf("compose: writing the offered-back close on the deal's timeline: %w", err)
	}
	return nil
}
