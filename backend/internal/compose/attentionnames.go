// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The attention feed's display-name resolver (attention.Names): one gated
// single-row read per distinct subject, each through the owning module's own
// get — so a label is exactly as visible as the record, under the READER's
// grants, and this seam holds no authority of its own. A refusal
// (permission denied, or the row-scope not-found that hides existence)
// costs the label and nothing else: the id still travels, the contract's
// "Absent when the caller may not read it".

import (
	"context"
	"errors"

	"github.com/margince/margince/backend/internal/compose/attention"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/modules/projects"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// attentionNames resolves each subject type through the store that owns it.
type attentionNames struct {
	people     *people.Store
	deals      *deals.Store
	activities *activities.Store
	projects   *projects.Store
}

var _ attention.Names = attentionNames{}

// Label answers one record's display name under the caller's scope.
//
// The person/organization/lead arms reuse the SAME face builders the merge
// cards use (personFace and siblings below in this package), so the two
// surfaces cannot come to spell one record's name two ways. A type outside
// this vocabulary answers absent rather than guessing — the subject enum
// and this switch are held together by the wiring test that labels every
// lane.
func (n attentionNames) Label(ctx context.Context, entityType string, id ids.UUID) (string, bool, error) {
	label, err := n.read(ctx, entityType, id)
	switch {
	case errors.Is(err, apperrors.ErrPermissionDenied), errors.Is(err, apperrors.ErrNotFound):
		return "", false, nil
	case err != nil:
		return "", false, err
	case label == "":
		// A record can honestly have no name (a lead captured with none); an
		// empty label is absent, not a blank string on a card.
		return "", false, nil
	default:
		return label, true, nil
	}
}

func (n attentionNames) read(ctx context.Context, entityType string, id ids.UUID) (string, error) {
	switch entityType {
	case flipObjectPerson:
		row, err := n.people.GetPerson(ctx, ids.From[ids.PersonKind](id), storekit.LiveOnly)
		if err != nil {
			return "", err
		}
		return personFace(row).Label, nil
	case flipObjectOrganization:
		row, err := n.people.GetOrganization(ctx, ids.From[ids.OrganizationKind](id), storekit.LiveOnly)
		if err != nil {
			return "", err
		}
		return organizationFace(row).Label, nil
	case flipObjectLead:
		row, err := n.people.GetLead(ctx, ids.From[ids.LeadKind](id), storekit.LiveOnly)
		if err != nil {
			return "", err
		}
		return leadFace(row).Label, nil
	case "deal":
		row, err := n.deals.GetDeal(ctx, ids.From[ids.DealKind](id), storekit.LiveOnly)
		if err != nil {
			return "", err
		}
		return row.Name, nil
	case "activity":
		row, err := n.activities.GetActivity(ctx, ids.From[ids.ActivityKind](id), storekit.LiveOnly)
		if err != nil {
			return "", err
		}
		if row.Subject == nil {
			return "", nil
		}
		return *row.Subject, nil
	case "project":
		row, err := n.projects.GetProject(ctx, ids.From[ids.ProjectKind](id), storekit.LiveOnly)
		if err != nil {
			return "", err
		}
		return row.Name, nil
	default:
		return "", nil
	}
}
