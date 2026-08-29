// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The feed's accountability-and-operations seams: the compliance clock and
// the sync's health, each bound to the module that owns what it shows —
// split from attentionseam.go on the one-concept-per-file length cap.

import (
	"context"

	"github.com/margince/margince/backend/internal/compose/attention"
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
