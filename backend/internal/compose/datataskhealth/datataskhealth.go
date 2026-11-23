// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package datataskhealth reads one correction step for System health and the
// data task report. It gives the finding a step raises and the action it left open.
package datataskhealth

import (
	"context"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// Finding is the System health code and condition of one step. A finished step
// raises none unless it left action needed.
func Finding(row datatask.Progress, now time.Time) (string, crmcontracts.HealthSourceCondition, bool) {
	if row.Version == 0 && row.BlockedReason == datatask.ReviewRequired {
		return "", "", false
	}
	switch {
	case row.Status == datatask.Done && row.Action != nil:
		return "updates_action", crmcontracts.HealthSourceConditionWaitingHuman, true
	case row.Status == datatask.Done:
		return "", "", false
	case row.Status == datatask.Failed:
		return "updates_failed", crmcontracts.HealthSourceConditionTechnical, true
	case row.Status == datatask.Paused || row.BlockedReason == datatask.ReviewRequired ||
		row.BlockedReason == "missing_prerequisite" || row.BlockedReason == "newer_release_required":
		return "updates_review", crmcontracts.HealthSourceConditionWaitingHuman, true
	case row.NextAttemptAt != nil && row.NextAttemptAt.After(now):
		return "updates_retrying", crmcontracts.HealthSourceConditionRetrying, true
	default:
		return "updates_processing", crmcontracts.HealthSourceConditionProcessing, true
	}
}

// fallbackReason words a reason this build has no copy for, so a new reason
// still reads as a to-do rather than as nothing.
const fallbackReason = "unknown"

// MaySeeRecords holds the record references to the bar a privacy reviewer
// meets: audit_log read over every row with no field masks. Others get the count.
func MaySeeRecords(ctx context.Context) bool {
	actor, ok := principal.Actor(ctx)
	return ok && auth.Allows(ctx, "audit_log", principal.ActionRead) && auth.Unbounded(actor) && len(actor.Permissions.FieldMasks) == 0
}

// Action is the step's open action in the reader's language, or nil. The
// records are left out unless withRecords is set.
func Action(action *datatask.ActionNeeded, language textlang.Lang, withRecords bool) *crmcontracts.DataTaskActionNeeded {
	if action == nil {
		return nil
	}
	code := "datatask.action." + action.Reason
	if !systemtext.Has(code + ".sentence") {
		code = "datatask.action." + fallbackReason
	}
	out := &crmcontracts.DataTaskActionNeeded{
		Reason: crmcontracts.DataTaskActionNeededReason(action.Reason), Count: action.Count,
		Sentence: systemtext.Copy(code+".sentence", language), Remedy: systemtext.Copy(code+".remedy", language),
		Records: make([]crmcontracts.DataTaskActionRecord, 0, len(action.Records)),
	}
	if !withRecords {
		return out
	}
	for _, record := range action.Records {
		out.Records = append(out.Records, crmcontracts.DataTaskActionRecord{Type: record.Type, Id: openapi_types.UUID(record.ID)})
	}
	return out
}
