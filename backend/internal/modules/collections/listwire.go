// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package collections

// The list answers in the contract's own shapes. The HTTP handlers and the
// agent tools both answer through these, so a caller gets the same list, the
// same count and the same reasons whichever door it asked through.

import (
	"context"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ListsPage answers the list library read.
func (s *Store) ListsPage(ctx context.Context, filter ListFilter) (crmcontracts.ListListResponse, error) {
	lists, truncated, err := s.ListLists(ctx, filter)
	if err != nil {
		return crmcontracts.ListListResponse{}, err
	}
	summaries := make([]*listSummary, 0, len(lists))
	for _, l := range lists {
		summary, err := s.summarize(ctx, l)
		if err != nil {
			return crmcontracts.ListListResponse{}, err
		}
		summaries = append(summaries, &summary)
	}
	if err := s.observedFor(ctx, summaries); err != nil {
		return crmcontracts.ListListResponse{}, err
	}
	data := make([]crmcontracts.List, 0, len(summaries))
	for _, summary := range summaries {
		data = append(data, wireList(*summary))
	}
	return crmcontracts.ListListResponse{Data: data, Page: crmcontracts.PageInfo{HasMore: truncated}}, nil
}

// ListView answers one list with its count, health and dependencies.
func (s *Store) ListView(ctx context.Context, id ids.ListID) (crmcontracts.List, error) {
	l, err := s.GetList(ctx, id)
	if err != nil {
		return crmcontracts.List{}, err
	}
	return s.view(ctx, l)
}

// view summarizes a list row the caller just read or wrote.
func (s *Store) view(ctx context.Context, l listRow) (crmcontracts.List, error) {
	summary, err := s.summarize(ctx, l)
	if err != nil {
		return crmcontracts.List{}, err
	}
	if err := s.observedFor(ctx, []*listSummary{&summary}); err != nil {
		return crmcontracts.List{}, err
	}
	if summary.Joined, err = s.joinedSinceVisit(ctx, l); err != nil {
		return crmcontracts.List{}, err
	}
	changes, found, err := s.changesSinceVisit(ctx, l)
	if err != nil {
		return crmcontracts.List{}, err
	}
	if found {
		summary.Changes = &changes
	}
	if summary.Dependencies, err = s.Dependencies(ctx, l.ID); err != nil {
		return crmcontracts.List{}, err
	}
	return wireList(summary), nil
}

// CreateListView makes a list and answers it.
func (s *Store) CreateListView(ctx context.Context, in CreateListInput) (crmcontracts.List, error) {
	l, err := s.CreateList(ctx, in)
	if err != nil {
		return crmcontracts.List{}, err
	}
	return s.view(ctx, l)
}

// UpdateListView changes a list and answers it.
func (s *Store) UpdateListView(ctx context.Context, id ids.ListID, in UpdateListInput) (crmcontracts.List, error) {
	l, err := s.UpdateList(ctx, id, in)
	if err != nil {
		return crmcontracts.List{}, err
	}
	return s.view(ctx, l)
}

// SetArchivedView archives or restores a list and answers it.
func (s *Store) SetArchivedView(ctx context.Context, id ids.ListID, archive bool) (crmcontracts.List, error) {
	l, err := s.setArchived(ctx, id, archive)
	if err != nil {
		return crmcontracts.List{}, err
	}
	return s.view(ctx, l)
}

// MembersPage answers one page of a list's members.
func (s *Store) MembersPage(ctx context.Context, id ids.ListID, limit int, cursor string) (crmcontracts.ListMemberListResponse, error) {
	members, page, err := s.ListMembers(ctx, id, limit, cursor)
	if err != nil {
		return crmcontracts.ListMemberListResponse{}, err
	}
	data := make([]crmcontracts.ListMember, 0, len(members))
	for _, m := range members {
		data = append(data, wireMember(m))
	}
	return crmcontracts.ListMemberListResponse{Data: data, Page: wirePage(page)}, nil
}

// AddMemberView adds one Shortlist member and answers it.
func (s *Store) AddMemberView(ctx context.Context, id ids.ListID, change MemberChange) (crmcontracts.ListMember, error) {
	m, err := s.AddMember(ctx, id, change)
	if err != nil {
		return crmcontracts.ListMember{}, err
	}
	return wireMember(m), nil
}

// VisitView records the caller's visit and answers it.
func (s *Store) VisitView(ctx context.Context, id ids.ListID) (crmcontracts.ListVisit, error) {
	visit, err := s.VisitList(ctx, id)
	if err != nil {
		return crmcontracts.ListVisit{}, err
	}
	return crmcontracts.ListVisit{
		ListId: openapi_types.UUID(id.UUID), VisitedAt: visit.VisitedAt, PreviousVisitAt: visit.Previous,
	}, nil
}

// ExplainView answers why a record is or is not on a list.
func (s *Store) ExplainView(ctx context.Context, id ids.ListID, entityID ids.UUID) (crmcontracts.ListMemberExplanation, error) {
	why, err := s.ExplainMember(ctx, id, entityID)
	if err != nil {
		return crmcontracts.ListMemberExplanation{}, err
	}
	out := crmcontracts.ListMemberExplanation{
		ListId: openapi_types.UUID(id.UUID), EntityId: openapi_types.UUID(entityID),
		ListType: crmcontracts.ListMemberExplanationListType(why.ListType), Member: why.Member,
		AddedBy: why.AddedBy, AddedAt: why.AddedAt, Note: why.Note,
	}
	if why.Clauses != nil {
		clauses := wireVerdict(*why.Clauses)
		out.Clauses, out.Eligible = &clauses, &why.Eligible
	}
	if why.AddedBy != nil {
		names, err := s.actorNames(ctx, []string{*why.AddedBy})
		if err != nil {
			return crmcontracts.ListMemberExplanation{}, err
		}
		if name, ok := names[*why.AddedBy]; ok {
			out.AddedByName = &name
		}
	}
	return out, nil
}

// HistoryPage answers one page of a list's history.
func (s *Store) HistoryPage(ctx context.Context, id ids.ListID, limit int, cursor string) (crmcontracts.ListHistoryResponse, error) {
	entries, page, err := s.History(ctx, id, limit, cursor)
	if err != nil {
		return crmcontracts.ListHistoryResponse{}, err
	}
	actors := make([]string, 0, len(entries))
	for _, e := range entries {
		actors = append(actors, e.Actor)
	}
	names, err := s.actorNames(ctx, actors)
	if err != nil {
		return crmcontracts.ListHistoryResponse{}, err
	}
	data := make([]crmcontracts.ListHistoryEntry, 0, len(entries))
	for _, e := range entries {
		data = append(data, wireHistory(e, names))
	}
	return crmcontracts.ListHistoryResponse{Data: data, Page: wirePage(page)}, nil
}

func wireList(l listSummary) crmcontracts.List {
	out := crmcontracts.List{
		Id: openapi_types.UUID(l.ID.UUID), Name: l.Name, Purpose: l.Purpose,
		EntityType: crmcontracts.ListEntityType(l.EntityType), ListType: crmcontracts.ListListType(l.ListType),
		Sharing: crmcontracts.ListSharing(l.Sharing), Version: l.Version,
		OwnerId: userUUID(l.OwnerID), StewardId: userUUID(l.StewardID), StewardName: l.StewardName,
		VisibleCount: l.VisibleCount, Health: crmcontracts.ListHealth(l.Health), CanEdit: l.CanEdit,
		CreatedAt: &l.CreatedAt, UpdatedAt: &l.UpdatedAt, ArchivedAt: l.ArchivedAt,
	}
	if len(l.Definition) > 0 {
		out.Definition = &l.Definition
	}
	if len(l.RetiredFields) > 0 {
		out.RetiredFields = &l.RetiredFields
	}
	if l.TeamID != nil {
		team := openapi_types.UUID(l.TeamID.UUID)
		out.TeamId = &team
	}
	if l.LastCheck != nil {
		out.LastCheck = &crmcontracts.ListCheck{
			CheckedAt: l.LastCheck.CheckedAt, Outcome: crmcontracts.ListCheckOutcome(l.LastCheck.Outcome),
		}
	}
	if l.Pulse != nil {
		out.SinceLastVisit = &crmcontracts.ListPulse{Since: l.Pulse.Since, Entered: l.Pulse.Entered, Left: l.Pulse.Left}
	}
	if l.Joined != nil {
		joined := make([]openapi_types.UUID, 0, len(l.Joined))
		for _, id := range l.Joined {
			joined = append(joined, openapi_types.UUID(id))
		}
		out.JoinedSinceVisit = &joined
	}
	if l.Dependencies != nil {
		deps := make([]crmcontracts.ListDependency, 0, len(l.Dependencies))
		for _, d := range l.Dependencies {
			deps = append(deps, wireDependency(d))
		}
		out.Dependencies = &deps
	}
	if l.Changes != nil {
		out.ChangesSinceVisit = &crmcontracts.ListChangeSummary{
			Since: l.Changes.Since, FilterChanges: l.Changes.FilterChanges,
			Joined: wireChangeGroup(l.Changes.Joined), Left: wireChangeGroup(l.Changes.Left),
		}
	}
	return out
}

func wireDependency(d listDependency) crmcontracts.ListDependency {
	out := crmcontracts.ListDependency{
		Kind: crmcontracts.ListDependencyKind(d.Kind), OccurredAt: d.OccurredAt, Actor: d.Actor,
	}
	if d.Rule == nil {
		return out
	}
	role := crmcontracts.ListDependencyRole(d.Rule.Role)
	out.Role = &role
	if d.Rule.Name != "" {
		id := openapi_types.UUID(d.Rule.ID)
		out.AutomationId, out.AutomationName = &id, &d.Rule.Name
	}
	return out
}

func wireChangeGroup(g changeGroup) crmcontracts.ListChangeGroup {
	records := make([]crmcontracts.ListChangedRecord, 0, len(g.Records))
	for _, r := range g.Records {
		records = append(records, crmcontracts.ListChangedRecord{EntityId: openapi_types.UUID(r.ID), Name: r.Name})
	}
	return crmcontracts.ListChangeGroup{Count: g.Count, Records: records}
}

func userUUID(id *ids.UserID) *openapi_types.UUID {
	if id == nil {
		return nil
	}
	u := openapi_types.UUID(id.UUID)
	return &u
}

func wireMember(m memberRow) crmcontracts.ListMember {
	out := crmcontracts.ListMember{
		Id: openapi_types.UUID(m.ID), ListId: openapi_types.UUID(m.ListID.UUID),
		EntityType: crmcontracts.ListMemberEntityType(m.EntityType), EntityId: openapi_types.UUID(m.EntityID),
		AddedBy: &m.AddedBy, Note: m.Note,
	}
	// A computed Live List member carries no added-at instant; only a
	// Shortlist member row does.
	if !m.CreatedAt.IsZero() {
		out.CreatedAt = &m.CreatedAt
	}
	return out
}

func wireVerdict(n storekit.ExplainNode) crmcontracts.ListClauseVerdict {
	out := crmcontracts.ListClauseVerdict{Result: n.Result, Value: n.Value}
	if n.Join != "" {
		join := crmcontracts.ListClauseVerdictJoin(n.Join)
		children := make([]crmcontracts.ListClauseVerdict, 0, len(n.Children))
		for _, child := range n.Children {
			children = append(children, wireVerdict(child))
		}
		out.Join, out.Children = &join, &children
		return out
	}
	field, op := n.Field, n.Op
	out.Field, out.Op, out.Operand = &field, &op, n.Operand
	if n.Hidden {
		hidden := true
		out.Hidden = &hidden
	}
	return out
}

func wireHistory(e HistoryEntry, names map[string]string) crmcontracts.ListHistoryEntry {
	out := crmcontracts.ListHistoryEntry{
		Id: openapi_types.UUID(e.ID), Kind: crmcontracts.ListHistoryEntryKind(e.Kind),
		OccurredAt: e.OccurredAt, Actor: e.Actor, EntityType: e.EntityType, Note: e.Note,
		DefinitionVersion: e.DefinitionVersion, Version: e.Version, Name: e.Name, Sharing: e.Sharing,
	}
	if name, ok := names[e.Actor]; ok {
		out.ActorName = &name
	}
	if e.EntityID != nil {
		id := openapi_types.UUID(*e.EntityID)
		out.EntityId = &id
	}
	if e.Reason != nil {
		reason := crmcontracts.ListHistoryEntryReason(*e.Reason)
		out.Reason = &reason
	}
	if e.Definition != nil {
		out.Definition = &e.Definition
	}
	return out
}

func wirePage(page storekit.Page) crmcontracts.PageInfo {
	info := crmcontracts.PageInfo{HasMore: page.HasMore}
	if page.NextCursor != "" {
		info.NextCursor = &page.NextCursor
	}
	return info
}
