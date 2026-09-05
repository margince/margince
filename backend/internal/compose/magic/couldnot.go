// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package magic

// The could-not-complete lane: work that was agreed and did not land.
//
// ITS OWN LANE, and never folded into done. A failure reported as an achievement
// is the one thing this surface must not do — a rep who reads "drafted the recap"
// and acts as though it was sent has been misled by the product, not by a
// system that broke. Two sources, one meaning:
//
//   - an APPROVAL whose effect failed. Somebody said yes, the write did not
//     happen, and nothing else on the morning says so: the approval reads as
//     decided everywhere it appears.
//   - an AUTOMATION run that failed or was blocked. The rule is live and enabled
//     and its last firing did nothing.

import (
	"context"
	"errors"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/automation"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// TroubledRuns answers the automations that failed or were blocked.
//
// A seam because automation is a sibling module: compose injects the edge rather
// than this package importing it.
type TroubledRuns interface {
	TroubledRuns(ctx context.Context, since time.Time, limit int) ([]automation.TroubledAutomationRun, error)
}

// couldNotComplete reads the work that did not land.
//
// A REFUSED read is named to the caller rather than folded into an empty lane:
// "you may not see this" and "nothing failed" are different answers, and a
// receipt that conflated them would tell an administrator their machinery is
// healthy on the day they lost the grant to check.
func (s *Service) couldNotComplete(
	ctx context.Context, since time.Time, limit int,
) ([]crmcontracts.MagicLine, *crmcontracts.WorklistSourceUnavailable, error) {
	if s.troubled == nil {
		return []crmcontracts.MagicLine{}, nil, nil
	}
	runs, err := s.troubled.TroubledRuns(ctx, since, limit)
	if err != nil {
		if errors.Is(err, apperrors.ErrPermissionDenied) {
			return []crmcontracts.MagicLine{}, &crmcontracts.WorklistSourceUnavailable{
				Source: sourceAutomationHealth,
				Reason: crmcontracts.WorklistSourceUnavailableReasonWithheld,
			}, nil
		}
		return nil, nil, err
	}
	lines := make([]crmcontracts.MagicLine, 0, len(runs))
	for _, run := range runs {
		lines = append(lines, troubledLine(run))
	}
	return lines, nil, nil
}

// sourceAutomationHealth names this lane where a refusal is reported, in the
// worklist's own source vocabulary so a client that already draws those names
// needs no second one.
const sourceAutomationHealth = "automation_run"

// troubledLine dresses one failed firing.
//
// The rule's own id is the entity, not the run's: a reader who wants to act goes
// to the rule, and two failures of one rule name one thing to fix rather than
// two things to look at.
func troubledLine(run automation.TroubledAutomationRun) crmcontracts.MagicLine {
	values := map[string]string{"name": run.Name, "outcome": run.Outcome}
	if run.Reason != nil {
		values["reason"] = *run.Reason
	}
	consequence := "magic.consequence.automation_did_nothing"
	return crmcontracts.MagicLine{
		Id:         openapi_types.UUID(run.ID),
		OccurredAt: run.CreatedAt,
		Lane:       crmcontracts.MagicLaneCouldNotComplete,
		Summary: crmcontracts.MagicSentence{
			Key:    "magic.action.automation_troubled",
			Values: &values,
		},
		Entity: &crmcontracts.MagicEntityRef{
			Type: "automation",
			Id:   openapi_types.UUID(run.AutomationID.UUID),
		},
		Consequence: &consequence,
		Actor: crmcontracts.MagicActor{
			Type: crmcontracts.MagicActorSystem,
			Id:   "system:automation",
		},
		// A firing that did not happen has nothing to take back. Saying so
		// explicitly beats leaving the field absent, which a client would have
		// to guess about.
		Undo: &crmcontracts.MagicUndo{Undoable: false},
	}
}
