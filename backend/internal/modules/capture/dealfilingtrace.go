// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// Who filed a mail on its deal, kept on the mail's capture trace so Settings >
// Capture can show it. A hook may file mail by a rule of the installation, such
// as a purchase order number looked up in a customer table. That rule has to be
// visible to the colleagues whose mail it files.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// FilingActorCore is the actor of a filing that no hook touched.
const FilingActorCore = "core"

// The results a filing can record, besides a deal outcome.
const (
	filingPartiesChanged = "parties_changed"
	filingMoved          = "moved"
	filingRetracted      = "retracted"
)

// Filing is the latest decision about a mail's deal. Actor is "core" or the
// name of the extension. Hook is empty for core. Reason is the rung core used,
// or the reason key the extension declared.
type Filing struct {
	Actor  string `json:"actor"`
	Hook   string `json:"hook,omitempty"`
	Result string `json:"result"`
	Reason string `json:"reason,omitempty"`
}

// filingOf is the filing a verdict amounts to, or nil when there is nothing to
// show. A failed hook outranks a change, a change outranks an ignored answer,
// and core's own link is shown last.
func filingOf(verdict DealVerdict, result string) *Filing {
	var failed, changed, ignored *HookChange
	for i := range verdict.Hooks {
		c := &verdict.Hooks[i]
		switch c.Result {
		case HookFailed:
			failed = c
		case HookIgnored:
			ignored = c
		default:
			changed = c
		}
	}
	switch {
	case failed != nil:
		return &Filing{Actor: failed.Unit, Hook: failed.Hook, Result: string(HookFailed), Reason: failed.Reason}
	case changed != nil:
		return &Filing{Actor: changed.Unit, Hook: changed.Hook, Result: changedResult(*changed, verdict, result), Reason: changed.Reason}
	case ignored != nil:
		return &Filing{Actor: ignored.Unit, Hook: ignored.Hook, Result: string(HookIgnored), Reason: ignored.Reason}
	case verdict.Outcome == DealLinked:
		return &Filing{Actor: FilingActorCore, Result: orElse(result, string(DealLinked)), Reason: string(verdict.Basis)}
	}
	return nil
}

func changedResult(change HookChange, verdict DealVerdict, result string) string {
	switch {
	case result != "":
		return result
	case change.Hook == "parties":
		return filingPartiesChanged
	}
	return string(verdict.Outcome)
}

func orElse(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

// recordFiling stores the filing on the mail's trace rows. A mail with no trace
// row, because the day's window passed, has nothing to update.
func recordFiling(ctx context.Context, tx pgx.Tx, mail ids.ActivityID, verdict DealVerdict, result string) error {
	filing := filingOf(verdict, result)
	if filing == nil {
		return nil
	}
	if _, err := tx.Exec(ctx, `UPDATE capture_trace SET filing = $2 WHERE activity_id = $1`, mail, filing); err != nil {
		return fmt.Errorf("capture: recording who filed a mail on its deal: %w", err)
	}
	return nil
}

// recordRepairFiling records a repair that moved or took back a link.
func recordRepairFiling(ctx context.Context, tx pgx.Tx, mail ids.ActivityID, verdict DealVerdict, outcome DealOutcome) error {
	switch outcome {
	case DealMachineMoved:
		return recordFiling(ctx, tx, mail, verdict, filingMoved)
	case DealMachineRetracted, DealMachineRetractedRefused:
		return recordFiling(ctx, tx, mail, verdict, filingRetracted)
	}
	return nil
}
