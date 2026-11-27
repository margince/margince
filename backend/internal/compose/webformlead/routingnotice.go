// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package webformlead

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/notices"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
	"github.com/margince/margince/backend/internal/shared/ports/workflow"
)

// websiteLeadRouting commits the first assignment and its arrival notice together.
// The wrapped handler declines already-owned leads, so reassignment cannot
// announce an old post again.
type websiteLeadRouting struct {
	workflow.Handler
	db      *database.DB
	arrival func(context.Context, *database.DB, ids.UUID, ids.UUID, notices.Target) error
}

// RoutingWithArrival adds the accepted post's owner notice to lead assignment.
func RoutingWithArrival(db *database.DB, handler workflow.Handler, arrival func(context.Context, *database.DB, ids.UUID, ids.UUID, notices.Target) error) workflow.Handler {
	return websiteLeadRouting{Handler: handler, db: db, arrival: arrival}
}

func (w websiteLeadRouting) Apply(ctx context.Context, ev workflow.Event, eff workflow.Effect, token *workflow.ApprovalToken) (workflow.RunResult, error) {
	var out workflow.RunResult
	err := database.WithWorkspaceWriteTx(ctx, w.db.Pool(), func(bound context.Context) error {
		var err error
		out, err = w.Handler.Apply(bound, ev, eff, token)
		if err != nil {
			return err
		}
		return w.tellRoutedWebsiteLeadOwner(bound, ev.Entity.ID)
	})
	return out, err
}

func (w websiteLeadRouting) tellRoutedWebsiteLeadOwner(ctx context.Context, leadID ids.UUID) error {
	db := w.db
	// Metadata is server-owned; tellWebsiteLeadOwner rechecks the recipient's
	// own record and note visibility before it creates or delivers a notice.
	lookup := principal.SystemActing(ctx, "system:website-lead-owner")
	lead, err := contacts.NewStore(db).GetLead(lookup, ids.From[ids.LeadKind](leadID), storekit.LiveOnly)
	if err != nil {
		return err
	}
	if lead.SourceSystem == nil || *lead.SourceSystem != provenance.WebFormSource || lead.SourceId == nil {
		return nil
	}
	notes, _, err := activities.NewStore(db).ListActivities(lookup, activities.ListActivitiesInput{
		Kind: new("note"), SourceSystem: new(provenance.WebFormSource), SourceIDs: []string{*lead.SourceId},
		EntityType: new("lead"), EntityID: &leadID, Limit: new(1),
	})
	if err != nil {
		return err
	}
	if len(notes) == 0 || notes[0].WebForm == nil || notes[0].WebForm.HandlingAutomationId != nil {
		return nil
	}
	formText, submission, found := strings.Cut(*lead.SourceId, ":")
	if !found || submission == "" {
		return fmt.Errorf("compose: website lead has no submission identity")
	}
	formID, err := ids.Parse(formText)
	if err != nil {
		return fmt.Errorf("compose: reading a routed website form identity: %w", err)
	}
	err = w.arrival(lookup, db, formID, ids.UUID(notes[0].Id), notices.Target{Type: "lead", ID: leadID})
	if errors.Is(err, apperrors.ErrNotFound) {
		return nil
	}
	return err
}
