// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The tool door onto lists. read_lists and change_lists answer through the
// same collections store the /v1/lists routes do, in the same contract shapes,
// so an agent and the user behind it read one list the same way.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/modules/collections"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type listSeam struct {
	on      bool
	store   *collections.Store
	preview filterPreviewHandlers
}

// newListSeam builds the tool door. While lists are switched off every call
// answers not found, the same as the routes.
func newListSeam(pool *pgxpool.Pool, on bool) listSeam {
	store := NewCollectionsStore(pool)
	return listSeam{on: on, store: store, preview: filterPreviewHandlers{pool: pool, collections: store}}
}

func (s listSeam) ReadLists(ctx context.Context, q agents.ListRead) (json.RawMessage, error) {
	if !s.on {
		return nil, apperrors.ErrNotFound
	}
	switch q.Mode {
	case agents.ListModeFind:
		filter := collections.ListFilter{Query: &q.Query, Sharing: q.Sharing, Archived: storekit.LiveOnly}
		if q.EntityType != "" {
			filter.EntityType = &q.EntityType
		}
		return encodedListAnswer(s.store.ListsPage(ctx, filter))
	case agents.ListModeGet:
		return encodedListAnswer(s.store.ListView(ctx, listID(*q.ListID)))
	case agents.ListModeMembers:
		return encodedListAnswer(s.store.MembersPage(ctx, listID(*q.ListID), q.Limit, q.Cursor))
	case agents.ListModeWhy:
		return encodedListAnswer(s.store.ExplainView(ctx, listID(*q.ListID), *q.RecordID))
	case agents.ListModeHistory:
		return encodedListAnswer(s.store.HistoryPage(ctx, listID(*q.ListID), q.Limit, q.Cursor))
	default:
		return s.previewDefinition(ctx, q)
	}
}

// previewDefinition answers what a filter would select, through the engine
// and projection the filter builder previews with, logged as a read.
func (s listSeam) previewDefinition(ctx context.Context, q agents.ListRead) (json.RawMessage, error) {
	engine, ok, err := s.store.SegmentEngine(ctx, q.EntityType)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, &agents.BadArgsError{Cause: fmt.Errorf("%q is not a filterable record type", q.EntityType)}
	}
	pred, err := collections.PredicateFromDefinition(q.Definition)
	if err != nil {
		return nil, &agents.BadArgsError{Cause: err}
	}
	limit := q.Limit
	if limit == 0 {
		limit = filterPreviewDefaultRows
	}
	out, err := s.preview.preview(ctx, engine, pred, limit, true)
	if err != nil {
		return nil, err
	}
	out.Resource = crmcontracts.FilterPreviewResource(q.EntityType)
	return json.Marshal(out)
}

func (s listSeam) ChangeLists(ctx context.Context, c agents.ListChange) (json.RawMessage, error) {
	if !s.on {
		return nil, apperrors.ErrNotFound
	}
	switch c.Mode {
	case agents.ListModeCreate:
		in := collections.CreateListInput{
			Name: *c.Name, EntityType: c.EntityType, ListType: c.ListType, Definition: c.Definition,
			Purpose: c.Purpose, TeamID: typed[ids.TeamKind](c.TeamID), StewardID: typed[ids.UserKind](c.StewardID),
		}
		if c.Sharing != nil {
			in.Sharing = *c.Sharing
		}
		return encodedListAnswer(s.store.CreateListView(ctx, in))
	case agents.ListModeUpdate:
		return encodedListAnswer(s.store.UpdateListView(ctx, listID(*c.ListID), collections.UpdateListInput{
			Name: c.Name, Purpose: c.Purpose, Definition: c.Definition, Sharing: c.Sharing,
			TeamID: typed[ids.TeamKind](c.TeamID), StewardID: typed[ids.UserKind](c.StewardID), IfVersion: c.Version,
		}))
	case agents.ListModeArchive, agents.ListModeRestore:
		return encodedListAnswer(s.store.SetArchivedView(ctx, listID(*c.ListID), c.Mode == agents.ListModeArchive))
	case agents.ListModeAdd:
		return encodedListAnswer(s.store.AddMemberView(ctx, listID(*c.ListID), memberChangeOf(c)))
	default:
		if err := s.store.RemoveMember(ctx, listID(*c.ListID), memberChangeOf(c)); err != nil {
			return nil, err
		}
		return json.Marshal(map[string]bool{"removed": true})
	}
}

func memberChangeOf(c agents.ListChange) collections.MemberChange {
	return collections.MemberChange{
		EntityType: c.EntityType, EntityID: *c.RecordID, Note: c.Note, Reason: collections.ReasonChosen,
	}
}

func listID(id ids.UUID) ids.ListID { return ids.From[ids.ListKind](id) }

func typed[K ids.EntityKind](id *ids.UUID) *ids.ID[K] {
	if id == nil {
		return nil
	}
	v := ids.From[K](*id)
	return &v
}

// encoded marshals a store answer, or passes its error on.
func encodedListAnswer[T any](v T, err error) (json.RawMessage, error) {
	if err != nil {
		return nil, err
	}
	return json.Marshal(v)
}

// opCreateList names no list in its route: it makes one.
const opCreateList = "createList"

// listChangeCommand decodes the six list-changing routes for the REST door's
// governance seam. The approval binds the routed list; a new list has none.
//
//nolint:ireturn // a decoder's whole product is the erased command-and-resolver pair restCommands is typed by
func listChangeCommand(pol agentPolicy, deps restCommandDeps, r *http.Request, _ []byte) (agents.GovernedCall, error) {
	var id ids.UUID
	if pol.Op != opCreateList {
		routed, err := routedID(r)
		if err != nil {
			return nil, err
		}
		id = routed
	}
	return agents.NewListChangeCall(deps.records, deps.language, agents.ListChangeCommand{ID: id}), nil
}
