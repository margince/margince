// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package inquiries

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
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

// FiringFence runs a rule under the fence of the web form its inquiry came from.
// It declines an earlier lead rule once a reviewed policy owns the inquiry.
type FiringFence struct {
	forms  *webforms.Store
	leads  *contacts.Store
	notes  *activities.Store
	fence  webforms.HandlingFence
	legacy map[string]bool
}

// NewFiringFence fences forms with fence and knows the earlier lead rules by name.
func NewFiringFence(db *database.DB, fence webforms.HandlingFence) *FiringFence {
	legacy := map[string]bool{}
	for _, name := range automation.LegacyLeadHandlerNames() {
		legacy[name] = true
	}
	return &FiringFence{forms: webforms.NewStore(db, slog.Default()), leads: contacts.NewStore(db), notes: activities.NewStore(db), fence: fence, legacy: legacy}
}

// Within runs a rule's work under its form's fence. Rules with no form behind
// them run unfenced.
func (f *FiringFence) Within(ctx context.Context, spec workflow.Spec, ev workflow.Event, run func(context.Context) error) error {
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
		if err := f.legacyMayFire(bound, lead, id); err != nil {
			return err
		}
		return run(bound)
	})
}

// legacyMayFire declines an earlier lead rule for an inquiry a reviewed policy
// accepted, or one that waits for its accepted note.
func (f *FiringFence) legacyMayFire(ctx context.Context, lead crmcontracts.Lead, formID ids.UUID) error {
	note, err := f.notes.ActivityBySource(ctx, provenance.WebFormSource, *lead.SourceId, storekit.IncludeArchived)
	if errors.Is(err, apperrors.ErrNotFound) {
		form, err := f.forms.GetForm(ctx, formID)
		if errors.Is(err, apperrors.ErrNotFound) {
			return &workflow.DeclinedError{Reason: "the website form is no longer available"}
		}
		if err != nil {
			return err
		}
		if form.HandlingAutomationID != nil && form.HandlingCutoverAt != nil && !lead.CreatedAt.Before(*form.HandlingCutoverAt) {
			return &workflow.DeclinedError{Reason: "the website inquiry is waiting for its accepted note"}
		}
		return nil
	}
	if err != nil || note.Raw == nil {
		return err
	}
	owner, err := automation.AcceptedInquiryOwner(note.Raw)
	if err != nil {
		return err
	}
	if owner != nil {
		return &workflow.DeclinedError{Reason: "the accepted website inquiry belongs to its reviewed handling policy"}
	}
	return nil
}
