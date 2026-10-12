// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Names records for the attention feed, analytics drill-through, audit log and
// privacy queues: one batched read per type, through the store that owns it.

import (
	"context"
	"errors"

	"github.com/margince/margince/backend/internal/compose/attention"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/collections"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/modules/projects"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// attentionNames resolves each subject type through the store that owns it.
type attentionNames struct {
	contacts    *contacts.Store
	deals       *deals.Store
	activities  *activities.Store
	projects    *projects.Store
	identity    *identity.Service
	collections *collections.Store
}

var (
	_ attention.Names       = attentionNames{}
	_ privacy.RecordLabeler = attentionNames{}
)

// newAttentionNames is the one assembly every naming surface shares, so a record
// is named or withheld alike on each.
func newAttentionNames(db *database.DB) attentionNames {
	return attentionNames{
		contacts:    contacts.NewStore(db),
		deals:       deals.NewStore(db, DealsInstallation()),
		activities:  activities.NewStore(db),
		projects:    projects.NewStore(db),
		identity:    identity.NewServiceFor(db),
		collections: collections.NewStore(db),
	}
}

// Labels answers a set of one type's display names under the caller's scope.
//
// One store read per TYPE, which is the whole point of the seam's shape: a
// page carrying a hundred contacts asks about contacts once. Each store's read
// carries that record's own object grant and row-scope clause, so a label is
// exactly as visible as the record, and this seam holds no authority of its
// own.
//
// A whole-read refusal — this caller may not read CONTACTS at all — costs the
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
	case entityContact:
		return n.contacts.ContactLabels(ctx, want)
	case entityCompany:
		return n.contacts.CompanyLabels(ctx, want)
	case entityLead:
		return n.contacts.LeadLabels(ctx, want)
	case string(datasource.RecordDeal):
		return n.deals.DealLabels(ctx, want)
	case string(datasource.EntityActivity):
		return n.activities.ActivityLabels(ctx, want)
	case string(datasource.RecordProject):
		return n.projects.ProjectLabels(ctx, want)
	default:
		return n.catalogRead(ctx, entityType, want)
	}
}

const (
	entityUser    = "user"
	entityTeam    = "team"
	entityTag     = "tag"
	entityList    = "list"
	entityProduct = "product"
)

// catalogRead names the seats and the administered vocabulary a page can be
// about, each through its owning module's batched read.
func (n attentionNames) catalogRead(ctx context.Context, entityType string, want []ids.UUID) (map[ids.UUID]string, error) {
	switch entityType {
	case entityUser:
		return seatNamer(n.identity)(ctx, want)
	case entityTeam:
		return n.identity.TeamLabels(ctx, want)
	case "role":
		return n.identity.RoleLabels(ctx, want)
	case entityTag:
		return n.collections.TagLabels(ctx, want)
	case entityList:
		return n.collections.ListLabels(ctx, want)
	case "pipeline":
		return n.deals.PipelineLabels(ctx, want)
	case "stage":
		return n.deals.StageLabels(ctx, want)
	case entityProduct:
		return n.deals.ProductLabels(ctx, want)
	default:
		// A type outside the vocabulary names nothing rather than guessing.
		return map[ids.UUID]string{}, nil
	}
}
