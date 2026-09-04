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
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// attentionNames resolves each subject type through the store that owns it.
type attentionNames struct {
	people     *people.Store
	deals      *deals.Store
	activities *activities.Store
	projects   *projects.Store
}

var _ attention.Names = attentionNames{}

// newAttentionNames assembles the resolver over one installation's stores.
//
// Two surfaces read display names through it: the attention feed's cards and
// the analytics drill-through's source rows. They share this constructor
// because a name must resolve identically on both — a second assembly could
// bind a different store set, and then the same record would be named on one
// surface and withheld on the other for no reason a reader could see.
func newAttentionNames(db *database.DB) attentionNames {
	return attentionNames{
		people:     people.NewStore(db),
		deals:      deals.NewStore(db, DealsInstallation()),
		activities: activities.NewStore(db),
		projects:   projects.NewStore(db),
	}
}

// Labels answers a set of one type's display names under the caller's scope.
//
// One store read per TYPE, which is the whole point of the seam's shape: a
// page carrying a hundred people asks about people once. Each store's read
// carries that record's own object grant and row-scope clause, so a label is
// exactly as visible as the record, and this seam holds no authority of its
// own.
//
// A whole-read refusal — this caller may not read PEOPLE at all — costs the
// type's labels and nothing else, because the contract's "absent when the
// caller may not read it" is the same answer for one record or a hundred.
// Any other error propagates: a database that will not answer must not read
// as a page of records the reader lacks grants for.
//
// A type outside this vocabulary answers nothing rather than guessing — the
// subject enum and this switch are held together by the wiring test that
// labels every lane.
func (n attentionNames) Labels(ctx context.Context, entityType string, want []ids.UUID) (map[ids.UUID]string, error) {
	labels, err := n.read(ctx, entityType, want)
	switch {
	case errors.Is(err, apperrors.ErrPermissionDenied), errors.Is(err, apperrors.ErrNotFound):
		// No labels, and no error: "the caller may not read it" is the same
		// answer for one record or a hundred. An empty map rather than nil,
		// so a caller reading it back cannot tell a refusal from an empty
		// page — there is nothing here they are meant to tell apart.
		return map[ids.UUID]string{}, nil
	case err != nil:
		return nil, err
	default:
		return labels, nil
	}
}

func (n attentionNames) read(ctx context.Context, entityType string, want []ids.UUID) (map[ids.UUID]string, error) {
	switch entityType {
	case flipObjectPerson:
		return n.people.PersonLabels(ctx, want)
	case flipObjectOrganization:
		return n.people.OrganizationLabels(ctx, want)
	case flipObjectLead:
		return n.people.LeadLabels(ctx, want)
	case string(datasource.RecordDeal):
		return n.deals.DealLabels(ctx, want)
	case string(datasource.EntityActivity):
		return n.activities.ActivityLabels(ctx, want)
	case string(datasource.RecordProject):
		return n.projects.ProjectLabels(ctx, want)
	default:
		// A type outside the vocabulary names nothing rather than guessing.
		return map[ids.UUID]string{}, nil
	}
}
