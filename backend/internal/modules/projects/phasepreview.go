// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package projects

import (
	"context"
	"encoding/json"
	"strings"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// PhaseTransitionPreview exposes the fields the phase writer changes.
type PhaseTransitionPreview struct {
	Before json.RawMessage
	After  json.RawMessage
}

func (in *AdvanceProjectPhaseInput) normalize() error {
	if !crmcontracts.AdvanceProjectPhaseRequestToPhase(in.ToPhase).Valid() {
		return &ProjectPhaseError{}
	}
	if in.ToPhase == PhaseClosed {
		if in.Reason == nil || strings.TrimSpace(*in.Reason) == "" {
			return &ClosedReasonRequiredError{}
		}
		in.Reason = new(strings.TrimSpace(*in.Reason))
	}
	return nil
}

func phaseTransitionPatch(current crmcontracts.Project, in AdvanceProjectPhaseInput) *storekit.Patch {
	p := storekit.NewPatch()
	p.Set("phase", string(*current.Phase), in.ToPhase)
	// A reopened project no longer carries the reason for its former closed state.
	switch {
	case in.ToPhase == PhaseClosed:
		p.Set("closed_reason", current.ClosedReason, *in.Reason)
	case current.ClosedReason != nil:
		p.Set("closed_reason", current.ClosedReason, nil)
	}
	return p
}

// PreviewAdvanceProjectPhase reads through the same field and row scope as the record.
func (s *Store) PreviewAdvanceProjectPhase(ctx context.Context, id ids.ProjectID, in AdvanceProjectPhaseInput) (PhaseTransitionPreview, error) {
	var out PhaseTransitionPreview
	current, err := s.GetProject(ctx, id, storekit.LiveOnly)
	if err != nil {
		return out, err
	}
	if err := in.normalize(); err != nil {
		return out, err
	}
	if current.Phase == nil || current.Version == nil {
		return out, apperrors.ErrPermissionDenied
	}
	if current.MaskedFields != nil {
		for _, field := range *current.MaskedFields {
			if field == "phase" || field == "closed_reason" {
				return out, apperrors.ErrPermissionDenied
			}
		}
	}
	if in.IfVersion != nil && int64(*current.Version) != *in.IfVersion {
		return out, apperrors.ErrVersionSkew
	}
	if string(*current.Phase) == in.ToPhase {
		return PhaseTransitionPreview{Before: json.RawMessage("{}"), After: json.RawMessage("{}")}, nil
	}
	patch := phaseTransitionPatch(current, in)
	out.Before, err = json.Marshal(patch.Before())
	if err != nil {
		return out, err
	}
	out.After, err = json.Marshal(patch.After())
	return out, err
}
