// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The feed's accountability-and-operations seams: the compliance clock and
// the sync's health, each bound to the module that owns what it shows —
// split from attentionseam.go on the one-concept-per-file length cap.

import (
	"context"
	"time"

	"github.com/margince/margince/backend/internal/compose/attention"
	"github.com/margince/margince/backend/internal/modules/aiactivity"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/modules/overlay"
)

// attentionDSRs binds the compliance lane to the consent module's own thin
// read; the DSR-admin gate lives in the store, not here.
type attentionDSRs struct{ store *consent.Store }

func (d attentionDSRs) OpenDueSoonest(ctx context.Context, limit int) ([]attention.DSRCase, error) {
	owed, err := d.store.OpenDSRsDueSoonest(ctx, limit)
	if err != nil {
		return nil, err
	}
	out := make([]attention.DSRCase, 0, len(owed))
	for _, request := range owed {
		out = append(out, attention.DSRCase{ID: request.ID, Kind: request.Kind, DueAt: request.DueAt})
	}
	return out, nil
}

// attentionSyncHealth binds the sync-health lane to the overlay module's own
// aggregated read; the mode gate and the every-role read posture live there.
type attentionSyncHealth struct{ svc *overlay.Service }

func (h attentionSyncHealth) Concerns(ctx context.Context) ([]attention.SyncConcern, error) {
	concerns, err := h.svc.SyncHealth(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]attention.SyncConcern, 0, len(concerns))
	for _, concern := range concerns {
		out = append(out, attention.SyncConcern{
			Kind:        concern.Kind,
			ErrorClass:  concern.ErrorClass,
			Failures:    concern.Failures,
			NextSweepAt: concern.NextSweepAt,
			Band:        concern.Band,
			Objects:     concern.Objects,
		})
	}
	return out, nil
}

// attentionCaptureHealth binds the capture-health lane to the capture
// module's own per-user read; the human-only arm lives there.
type attentionCaptureHealth struct{ registry *capture.Registry }

func (c attentionCaptureHealth) CaptureConcerns(ctx context.Context) ([]attention.CaptureConcern, error) {
	concerns, err := c.registry.HealthConcerns(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]attention.CaptureConcern, 0, len(concerns))
	for _, concern := range concerns {
		out = append(out, attention.CaptureConcern{
			ConnectionID: concern.ConnectionID,
			Kind:         concern.Kind,
			Provider:     concern.Provider,
			AccountLabel: concern.AccountLabel,
		})
	}
	return out, nil
}

// attentionAIWork binds the AI-work-health lane to the same projection the
// activity rail reads; the person-only refusal lives in the store's read.
type attentionAIWork struct{ store *aiactivity.Store }

func (a attentionAIWork) Troubled(ctx context.Context, since time.Time, limit int) ([]attention.TroubledRun, error) {
	troubled, err := a.store.Troubled(ctx, since, limit)
	if err != nil {
		return nil, err
	}
	out := make([]attention.TroubledRun, 0, len(troubled))
	for _, run := range troubled {
		item := attention.TroubledRun{
			ID:         run.ID,
			State:      run.State,
			OccurredAt: run.OccurredAt,
		}
		if run.Summary != nil {
			item.Summary = *run.Summary
		}
		if run.SubjectLabel != nil {
			item.SubjectLabel = *run.SubjectLabel
		}
		out = append(out, item)
	}
	return out, nil
}
