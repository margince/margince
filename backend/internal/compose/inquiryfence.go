// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/automation"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/webforms"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
	"github.com/margince/margince/backend/internal/shared/ports/workflow"
)

type inquiryFiringFence struct {
	forms  *webforms.Store
	leads  *contacts.Store
	notes  *activities.Store
	fence  *webFormFence
	legacy map[string]bool
}

func newInquiryFiringFence(db *database.DB) *inquiryFiringFence {
	legacy := map[string]bool{}
	for _, name := range automation.LegacyLeadHandlerNames() {
		legacy[name] = true
	}
	return &inquiryFiringFence{forms: webforms.NewStore(db, slog.Default()), leads: contacts.NewStore(db), notes: activities.NewStore(db), fence: newWebFormFence(db), legacy: legacy}
}

func (f *inquiryFiringFence) Within(ctx context.Context, spec workflow.Spec, ev workflow.Event, run func(context.Context) error) error {
	if spec.Name == provenance.WebsiteInquirySource {
		policy, err := automation.ParseInquiryPolicy(ev.Params)
		if err != nil {
			return err
		}
		return f.fence.Within(ctx, ids.UUID(policy.FormId), run)
	}
	if !f.legacy[spec.Name] {
		return run(ctx)
	}
	lead, err := f.leads.GetLead(ctx, ids.From[ids.LeadKind](ev.Entity.ID), storekit.LiveOnly)
	if errors.Is(err, apperrors.ErrNotFound) {
		return run(ctx)
	}
	if err != nil {
		return err
	}
	if lead.SourceSystem == nil || *lead.SourceSystem != provenance.WebFormSource || lead.SourceId == nil {
		return run(ctx)
	}
	form, _, ok := strings.Cut(*lead.SourceId, ":")
	id, err := ids.Parse(form)
	if !ok || err != nil {
		return errors.New("compose: website lead has no valid form identity")
	}
	return f.fence.Within(ctx, id, func(bound context.Context) error {
		note, err := f.notes.ActivityBySource(bound, provenance.WebFormSource, *lead.SourceId, storekit.IncludeArchived)
		if err == nil && note.Raw != nil {
			owner, err := automation.AcceptedInquiryOwner(note.Raw)
			if err != nil {
				return err
			}
			if owner != nil {
				return &workflow.DeclinedError{Reason: "the accepted website inquiry belongs to its reviewed handling policy"}
			}
		} else if err != nil && !errors.Is(err, apperrors.ErrNotFound) {
			return err
		}
		if errors.Is(err, apperrors.ErrNotFound) {
			form, err := f.forms.GetForm(bound, id)
			if errors.Is(err, apperrors.ErrNotFound) {
				return &workflow.DeclinedError{Reason: "the website form is no longer available"}
			}
			if err != nil {
				return err
			}
			if form.HandlingAutomationID != nil && form.HandlingCutoverAt != nil && !lead.CreatedAt.Before(*form.HandlingCutoverAt) {
				return &workflow.DeclinedError{Reason: "the website inquiry is waiting for its accepted note"}
			}
		}
		return run(bound)
	})
}
