// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// A message every rung of a sweep's ladder declined — a safety filter
// withholding the answer, a validator refusing every reply — is recorded as
// declined, one stamp per question, and that sweep's backlog leaves it out. Left
// unstamped it would be the oldest row in the backlog forever, so every tick
// would re-send it and split the batch it rode in into single calls.
//
// A DECLINE IS FINAL ONLY FOR THE PROMPT THAT WAS DECLINED. The stamp names the
// digest of that prompt, and a sweep asking under any other digest offers the
// row again: a reworded prompt may get an answer from the same models, and a
// decline recorded against wording that no longer ships says nothing about the
// wording that does. The price is one more call per declined row each time the
// prompt moves, which is the same price a stale verdict pays in OwedRestale.
// The digest names the prompt alone: a loosened validator or response schema
// does not re-offer what the old one refused (see capturelabel.Ruleset).
// While two builds with different prompts run side by side in a rolling deploy,
// each re-offers what the other declined; that lasts as long as the rollout.
//
// The stamp is bookkeeping about the sweep, not a judgement of the message: it
// states no verdict, so it mints no audit entry and no event, the posture
// SetCaptureLabel takes for the label beside it.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// declineStamp is one question's pair of columns: when every rung declined it,
// and which prompt they declined. Paired in one value so no write can stamp the
// one without the other.
type declineStamp struct {
	at, ruleset, question string
}

var (
	captureLabelDecline = declineStamp{"capture_label_declined_at", "capture_label_declined_ruleset", "capture_label"}
	owedVerdictDecline  = declineStamp{"owed_verdict_declined_at", "owed_verdict_declined_ruleset", "owed_verdict"}
)

// errRulesetUnnamed refuses a read or write that names no prompt digest. Every
// stamp is stale against an empty digest: a backlog read for none would hand
// back every row already judged or declined, and a stamp naming none would be
// re-offered on the very next tick.
var errRulesetUnnamed = errors.New("no prompt digest was named")

// offeredSQL is "this question is still to be asked of the row under the prompt
// named by param": never declined, or declined under another prompt. Shared by
// every backlog that leaves declined rows out and by the write that stamps
// them, because the two must agree on what "declined" means — a backlog wider
// than the write's CAS re-sends a row the write then refuses to stamp, which
// is the per-tick loop the stamp exists to end.
func (d declineStamp) offeredSQL(qualifier, param string) string {
	return "(" + qualifier + d.at + " IS NULL OR " + rulesetStaleSQL(qualifier+d.ruleset, param) + ")"
}

// MarkCaptureLabelDeclined takes one message out of the classify backlog because
// the models declined to label it under ruleset, reporting whether this write
// stamped it.
func (s *Store) MarkCaptureLabelDeclined(ctx context.Context, id ids.UUID, ruleset string) (bool, error) {
	return s.markDeclined(ctx, id, captureLabelDecline, ruleset)
}

// MarkOwedVerdictDeclined takes one message out of the owed backlog and the
// re-judge sweep because the models declined to judge it under ruleset. A
// verdict already on the row stands: declining to re-judge it is not a
// judgement that it was wrong.
func (s *Store) MarkOwedVerdictDeclined(ctx context.Context, id ids.UUID, ruleset string) (bool, error) {
	if err := auth.Require(ctx, "activity", principal.ActionUpdate); err != nil {
		return false, err
	}
	return s.markDeclined(ctx, id, owedVerdictDecline, ruleset)
}

// markDeclined stamps the question as declined under ruleset. offeredSQL is the
// CAS: a second pass that reached the same message under the same prompt first
// leaves its stamp standing, and a stamp naming an older prompt is replaced. The
// hold and archive clauses are what the backlogs test: activity_refuse_restricted_mutation
// rejects any write to a held row, and an archived row is out of every backlog
// already, so neither may fail or be touched by the sweep.
func (s *Store) markDeclined(ctx context.Context, id ids.UUID, stamp declineStamp, ruleset string) (applied bool, err error) {
	if ruleset == "" {
		return false, fmt.Errorf("activities: recording a declined %s needs the prompt that was declined: %w", stamp.question, errRulesetUnnamed)
	}
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		at, rulesetCol := pgx.Identifier{stamp.at}.Sanitize(), pgx.Identifier{stamp.ruleset}.Sanitize()
		tag, err := tx.Exec(ctx, `
			UPDATE activity SET `+at+` = now(), `+rulesetCol+` = $2
			WHERE id = $1 AND `+stamp.offeredSQL("", "$2")+`
			  AND restricted_at IS NULL AND archived_at IS NULL`, id, ruleset)
		if err != nil {
			return fmt.Errorf("activities: recording that the models declined the %s: %w", stamp.question, err)
		}
		applied = tag.RowsAffected() > 0
		return nil
	})
	return applied, err
}
