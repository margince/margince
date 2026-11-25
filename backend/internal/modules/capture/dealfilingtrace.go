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

	"github.com/margince/margince/backend/internal/platform/database/storekit"
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

// hookDeal names the deal hook in a recorded change.
const hookDeal = "deal"

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
	case ignored != nil && ignored.Hook == hookDeal:
		// A refused deal answer shows even after a parties change. A unit's author
		// looks for it when the mail is not filed where they expected.
		return &Filing{Actor: ignored.Unit, Hook: ignored.Hook, Result: string(HookIgnored), Reason: ignored.Reason}
	case changed != nil:
		return &Filing{Actor: changed.Unit, Hook: changed.Hook, Result: changedResult(*changed, verdict, result), Reason: changed.Reason}
	case ignored != nil:
		return &Filing{Actor: ignored.Unit, Hook: ignored.Hook, Result: string(HookIgnored), Reason: ignored.Reason}
	case verdict.Outcome == DealLinked, result != "":
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
	if err := logUnauditedHookAnswers(ctx, tx, mail, verdict, result); err != nil {
		return err
	}
	filing := filingOf(verdict, result)
	if filing == nil {
		return nil
	}
	if _, err := tx.Exec(ctx, `UPDATE capture_trace SET filing = $2 WHERE activity_id = $1`, mail, filing); err != nil {
		return fmt.Errorf("capture: recording who filed a mail on its deal: %w", err)
	}
	return nil
}

// actionExtensionMailHook is the system log action of a hook answer that no
// audit row names. It is kept for good, unlike the swept capture trace.
const actionExtensionMailHook = "extension_mail_hook"

// undeclaredReason stands in for a reason the unit did not declare. That text
// is the hook's own and may hold the mail, so the lasting row never keeps it.
const undeclaredReason = "undeclared"

// logUnauditedHookAnswers keeps a lasting record of each hook answer that no
// audit row names. A written link or unlink names the changes that took effect
// (withHookEvidence), so only a failed or ignored answer is logged then. When
// nothing was written, as for a `none` answer, every answer is logged. result is
// set only by a repair, and a repair sets it only after it wrote.
func logUnauditedHookAnswers(ctx context.Context, tx pgx.Tx, mail ids.ActivityID, verdict DealVerdict, result string) error {
	written := result != "" || verdict.Outcome == DealLinked
	for _, c := range verdict.Hooks {
		if written && c.Result == HookChanged {
			continue
		}
		reason := c.Reason
		if !c.Declared && reason != "" {
			reason = undeclaredReason
		}
		detail := map[string]any{
			"activity_id": mail, "unit": c.Unit, "hook": c.Hook, fieldReason: reason,
			"result": string(c.Result), "outcome": string(verdict.Outcome),
		}
		if err := logHookAnswerOnce(ctx, tx, detail); err != nil {
			return err
		}
	}
	return nil
}

// logHookAnswerOnce writes the row unless this transaction already wrote it. A
// repair decides one mail once per mailbox in one transaction. now() is the
// transaction's start, which LogSystem stamps on its row.
func logHookAnswerOnce(ctx context.Context, tx pgx.Tx, detail map[string]any) error {
	var logged bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM system_log
		WHERE action = $1 AND occurred_at = now() AND detail @> $2::jsonb)`,
		actionExtensionMailHook, storekit.JSONArg(detail)).Scan(&logged); err != nil {
		return fmt.Errorf("capture: reading the mail hook answers already recorded: %w", err)
	}
	if logged {
		return nil
	}
	if _, err := storekit.LogSystem(ctx, tx, actionExtensionMailHook, detail); err != nil {
		return fmt.Errorf("capture: recording a mail hook answer: %w", err)
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
