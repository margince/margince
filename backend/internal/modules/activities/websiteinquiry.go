// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"maps"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
	"github.com/margince/margince/backend/internal/shared/ports/workflow"
)

type WebsiteInquiryAdoption struct {
	Activity crmcontracts.Activity
	Event    events.Envelope
}

func (s *Store) AdoptWebsiteInquiry(ctx context.Context, id ids.ActivityID, version int64, form, policy ids.UUID) (WebsiteInquiryAdoption, error) {
	if err := auth.Require(ctx, "activity", principal.ActionUpdate); err != nil {
		return WebsiteInquiryAdoption{}, err
	}
	if version < 1 || form == ids.Nil || policy == ids.Nil {
		return WebsiteInquiryAdoption{}, apperrors.ErrConflict
	}
	var out WebsiteInquiryAdoption
	err := s.txRetryingLockCycles(ctx, pgx.ReadCommitted, func(tx pgx.Tx) error {
		var err error
		out, err = adoptWebsiteInquiryInTx(ctx, tx, id, version, form, policy, s.now())
		return err
	})
	return out, err
}

func adoptWebsiteInquiryInTx(ctx context.Context, tx pgx.Tx, id ids.ActivityID, version int64, form, policy ids.UUID, at time.Time) (WebsiteInquiryAdoption, error) {
	lock, err := storekit.LockRow(ctx, tx, "activity", id.UUID, storekit.LiveOnly)
	if err != nil {
		return WebsiteInquiryAdoption{}, err
	}
	if err := auth.EnsureActivityWritableIn(ctx, tx, id.UUID, true); err != nil {
		return WebsiteInquiryAdoption{}, err
	}
	current, err := ReadActivityContentTx(ctx, tx, id, storekit.LiveOnly)
	if err != nil {
		return WebsiteInquiryAdoption{}, err
	}
	if current.Version == nil || int64(*current.Version) != version {
		return WebsiteInquiryAdoption{}, apperrors.ErrVersionSkew
	}
	if current.Kind != crmcontracts.ActivityKindNote || current.SourceSystem == nil || *current.SourceSystem != provenance.WebFormSource || current.SourceId == nil || current.Raw == nil || !strings.HasPrefix(*current.SourceId, form.String()+":") {
		return WebsiteInquiryAdoption{}, apperrors.ErrConflict
	}
	owner, err := workflow.AcceptedInquiryOwner(current.Raw)
	if err != nil {
		return WebsiteInquiryAdoption{}, err
	}
	if owner != nil && *owner != policy {
		return WebsiteInquiryAdoption{}, apperrors.ErrConflict
	}
	next := maps.Clone(*current.Raw)
	next["handling_automation_id"] = policy
	patch := storekit.NewPatch()
	// Each explicit review pins a new note version; the occurrence key still prevents duplicate work.
	patch.SetStamp("raw", next)
	if err := patch.ApplyLocked(ctx, tx, lock); err != nil {
		return WebsiteInquiryAdoption{}, err
	}
	updated, err := readActivityContentAt(ctx, tx, id, storekit.LiveOnly, at)
	if err != nil {
		return WebsiteInquiryAdoption{}, err
	}
	audit, err := storekit.Audit(ctx, tx, "update", "activity", id.UUID,
		map[string]any{"handling_automation_id": owner, "version": current.Version},
		map[string]any{"handling_automation_id": policy, "version": updated.Version})
	if err != nil {
		return WebsiteInquiryAdoption{}, err
	}
	kind, wirePolicy := "note", crmcontracts.Id(policy)
	event, err := storekit.EmitEventEnvelope(ctx, tx, audit, id.UUID, crmcontracts.PublicEventActivityUpdated{
		Kind: &kind, HandlingAutomationId: &wirePolicy, SourceSystem: current.SourceSystem, WebsiteSubmissionKey: current.SourceId,
		ChangedFields: crmcontracts.PublicEventActivityChangedFields{HandlingAutomationId: &wirePolicy},
	})
	if err != nil {
		return WebsiteInquiryAdoption{}, err
	}
	return WebsiteInquiryAdoption{Activity: updated, Event: event}, nil
}
