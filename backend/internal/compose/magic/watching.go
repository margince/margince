// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package magic

// The watching lane: the sources whose health an administrator must restore.
//
// SOURCE HEALTH ONLY, and the boundary is the whole of the file's scope. A live
// rule whose last firing did nothing is could-not-complete's subject, and
// carrying it here as well would be two answers to one question. This lane
// answers the STANDING CONDITION of a source; could-not-complete answers a
// firing that did not land.

import (
	"context"
	"errors"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// SourceHealth answers the reader's own capture connections needing a hand.
//
// A seam because capture is a sibling module: compose injects the edge, and
// this package declares the row it wants rather than importing the module's.
type SourceHealth interface {
	CaptureConcerns(ctx context.Context) ([]CaptureConcern, error)
}

// CaptureConcern is one connection's worst standing condition.
type CaptureConcern struct {
	ConnectionID ids.UUID
	Kind         string
	Provider     string
	// AccountLabel is the display-only mailbox address when one was recorded.
	AccountLabel string
	// FailingSince is when the failure streak began, nil for a condition that
	// is a state rather than a streak.
	FailingSince *time.Time
}

// WithSourceHealth binds the watching lane. An option for the reason the
// could-not-complete lane's automation half is one.
func (s *Service) WithSourceHealth(h SourceHealth) *Service {
	s.sources = h
	return s
}

// sourceCaptureHealth names this lane where a refusal is reported, in the
// worklist's own source vocabulary so a client that already draws those names
// needs no second one.
const sourceCaptureHealth = "capture_health"

// watching reads the sources standing in a condition somebody must clear.
//
// A REFUSED read is named to the caller rather than folded into an empty lane:
// "you may not see this" and "every source is healthy" are different answers,
// and conflating them would tell an administrator their capture is fine on the
// day they lost the grant to check.
func (s *Service) watching(
	ctx context.Context, asOf time.Time,
) ([]crmcontracts.MagicLine, *crmcontracts.WorklistSourceUnavailable, error) {
	if s.sources == nil {
		return []crmcontracts.MagicLine{}, nil, nil
	}
	concerns, err := s.sources.CaptureConcerns(ctx)
	if err != nil {
		if errors.Is(err, apperrors.ErrPermissionDenied) {
			return []crmcontracts.MagicLine{}, &crmcontracts.WorklistSourceUnavailable{
				Source: sourceCaptureHealth,
				Reason: crmcontracts.WorklistSourceUnavailableReasonWithheld,
			}, nil
		}
		return nil, nil, err
	}
	lines := make([]crmcontracts.MagicLine, 0, len(concerns))
	for _, concern := range concerns {
		line, ok := concernLine(concern, asOf)
		if !ok {
			continue
		}
		lines = append(lines, line)
	}
	return lines, nil, nil
}

// watchedCondition is what one standing condition says and what it costs.
type watchedCondition struct {
	// sentence and consequence are i18n keys rather than prose: the product
	// ships three languages, and a sentence composed here reaches a German
	// reader in English.
	sentence    string
	consequence string
}

// watchedConcerns is the closed set of conditions this lane can name, keyed by
// the capture module's own concern vocabulary.
var watchedConcerns = map[string]watchedCondition{
	"reauth_required": {
		sentence:    "magic.action.capture_reauth_required",
		consequence: "magic.consequence.capture_not_collecting",
	},
	"connection_error": {
		sentence:    "magic.action.capture_connection_error",
		consequence: "magic.consequence.capture_not_collecting",
	},
	"sync_failing": {
		sentence:    "magic.action.capture_sync_failing",
		consequence: "magic.consequence.capture_may_be_incomplete",
	},
	"backfill_failed": {
		sentence:    "magic.action.capture_backfill_failed",
		consequence: "magic.consequence.capture_history_incomplete",
	},
}

// concernLine dresses one standing condition, or refuses it.
//
// A kind this build cannot name is DROPPED, as lineOf drops an unknown action:
// a condition with no sentence is not a call to action, and a blank line in a
// lane that only ever asks for a hand would read as one.
func concernLine(c CaptureConcern, asOf time.Time) (crmcontracts.MagicLine, bool) {
	condition, ok := watchedConcerns[c.Kind]
	if !ok {
		return crmcontracts.MagicLine{}, false
	}
	values := map[string]string{"provider": c.Provider}
	if c.AccountLabel != "" {
		values["account"] = c.AccountLabel
	}
	// A condition that is a state rather than a streak has no beginning this
	// read can name, and this lane reports the standing condition rather than
	// its history — so the read's own instant is the honest answer for one.
	occurredAt := asOf
	if c.FailingSince != nil {
		occurredAt = *c.FailingSince
	}
	return crmcontracts.MagicLine{
		Id:         openapi_types.UUID(c.ConnectionID),
		OccurredAt: occurredAt,
		Lane:       crmcontracts.MagicLineLaneMagicLaneWatching,
		Summary: crmcontracts.MagicSentence{
			Key:    condition.sentence,
			Values: &values,
		},
		Entity: &crmcontracts.MagicEntityRef{
			Type: "capture_connection",
			Id:   openapi_types.UUID(c.ConnectionID),
		},
		Consequence: &condition.consequence,
		Actor: crmcontracts.MagicActor{
			Type: crmcontracts.MagicActorTypeMagicActorSystem,
			Id:   "system:capture",
		},
		// A condition is not a change, so there is nothing to put back.
		Undo: &crmcontracts.MagicUndo{Undoable: false, Reason: &nothingToUndo},
	}, true
}
