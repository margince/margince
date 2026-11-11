// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// RunPendingWorkspace reads only the durable, due contact cohort.
func (e *CaptureEnricher) RunPendingWorkspace(ctx context.Context) (bool, error) {
	held, release, err := e.holdThePass(ctx)
	if err != nil || !held {
		return false, err
	}
	defer release()
	ctx = principal.SystemActing(ctx, "agent:enrich")
	enabled, err := config.StoredVal(ctx, StoredConfig(e.pool), capture.SignatureEnrich)
	if err != nil {
		return false, err
	}
	work, byContact, err := e.pendingSignatureCandidates(ctx, enabled)
	if err != nil {
		return false, err
	}

	advanced := 0
	for _, request := range work {
		var readErr error
		if candidate, ok := byContact[request.ContactID]; ok {
			_, readErr = e.enrichOne(ctx, candidate, enabled)
			request.Snapshot = candidate.Snapshot
		}
		retryAfter := time.Duration(0)
		if readErr != nil {
			retryAfter = 15 * time.Minute
			if until, ok := ai.DeferredUntil(readErr); ok {
				retryAfter = min(retryAfter, max(time.Second, until.Sub(e.now())))
			}
		}
		if err := e.store.SettleSignatureWork(ctx, request, retryAfter); err != nil {
			return false, errors.Join(readErr, fmt.Errorf("settling signature work: %w", err))
		}
		if readErr != nil {
			if ai.IsDeferral(readErr) {
				e.log.InfoContext(ctx, "signature enrich: shared model lane deferred", "retry_in", retryAfter, "err", readErr)
				return false, river.JobSnooze(retryAfter)
			}
			e.log.WarnContext(ctx, "signature enrich: contact read deferred", "contact", request.ContactID.String(), "err", readErr)
		} else {
			advanced++
		}
	}
	return advanced > 0 && len(work) == e.limit, nil
}

func (e *CaptureEnricher) pendingSignatureCandidates(ctx context.Context, enabled bool) ([]contacts.SignatureWork, map[ids.ContactID]contacts.SignatureCandidate, error) {
	work, err := e.store.PendingSignatureWork(ctx, e.limit)
	if err != nil || len(work) == 0 {
		return nil, nil, err
	}
	contactIDs := make([]ids.ContactID, 0, len(work))
	for _, request := range work {
		contactIDs = append(contactIDs, request.ContactID)
	}
	candidates, err := e.store.SignatureCandidatesForContacts(ctx, contactIDs, e.limit, enabled)
	if err != nil {
		return nil, nil, err
	}
	byContact := make(map[ids.ContactID]contacts.SignatureCandidate, len(candidates))
	for _, candidate := range candidates {
		byContact[candidate.ContactID] = candidate
	}
	return work, byContact, nil
}
