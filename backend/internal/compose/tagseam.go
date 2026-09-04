// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The tag seam: the agents module asks, collections answers. Declared in
// agents and implemented here, like every cross-module edge (ADR-0054).

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/modules/collections"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type tagAdapter struct{ store *collections.Store }

// tagSeam binds the tag verbs to the one store the HTTP transport uses, so a
// tag applied over MCP and one applied in the web app pass the same gates and
// write the same audit row.
func tagSeam(pool *pgxpool.Pool) agents.Tags {
	return tagAdapter{store: collections.NewStore(InstallationDB(pool))}
}

// ResolveTag answers the id of an EXISTING workspace tag with this name, and
// refuses when there is none.
//
// It does not create. The vocabulary is governed — only Admin and Ops coin a
// word — so a tool that minted one on a name it did not recognise would hand
// every agent, and through it every rep, the authority the governance exists
// to withhold. A misspelling would become a permanent second tag nobody
// chose, which is exactly the drift a shared vocabulary is for preventing.
//
// Case-insensitive, because "K5 Conference" and "k5 conference" are the same
// tag to everyone except a database. An ARCHIVED tag of that name is refused
// with a distinct message: the word exists, it was retired on purpose, and
// the caller needs to know that rather than being told it is unknown.
func (a tagAdapter) ResolveTag(ctx context.Context, name string) (ids.UUID, error) {
	if found, ok, err := a.store.FindTag(ctx, name); err != nil || ok {
		return found, err
	}
	// A miss is either a word nobody has coined or one somebody retired. The
	// two need different answers: the second is not "no such tag".
	_, state, err := a.store.LookupTagName(ctx, name)
	if err != nil {
		return ids.UUID{}, err
	}
	if state.Archived() {
		return ids.UUID{}, fmt.Errorf("the tag %q was archived and cannot be applied; an admin can restore it: %w",
			name, apperrors.ErrConflict)
	}
	return ids.UUID{}, fmt.Errorf("no tag named %q exists, and this tool does not create one; an admin or ops seat can add it to the vocabulary: %w",
		name, apperrors.ErrNotFound)
}

// GetTag hands one word and its weight across. The counts come from the store
// so the tool surface and the web surface report the same number — a tool that
// counted for itself would be a second answer to the question the admin screen
// already asks.
func (a tagAdapter) GetTag(ctx context.Context, tagID ids.UUID) (agents.TagDetail, error) {
	row, usage, err := a.store.GetTag(ctx, ids.From[ids.TagKind](tagID))
	if err != nil {
		return agents.TagDetail{}, err
	}
	out := agents.TagDetail{
		Tag: agents.Tag{
			TagID:    row.ID.UUID,
			Name:     row.Name,
			Archived: row.ArchivedAt != nil,
		},
		People:    usage.People,
		Companies: usage.Companies,
		Deals:     usage.Deals,
	}
	if row.Color != nil {
		out.Color = *row.Color
	}
	return out, nil
}

// RecordTags hands the record-tag read across. The tool and the record page
// read the SAME store method, so a model and a person looking at one company
// cannot be told different things about who tagged it.
func (a tagAdapter) RecordTags(ctx context.Context, entityType string, entityID ids.UUID) (agents.RecordTagsResult, error) {
	read, err := a.store.RecordTagsFor(ctx, entityType, entityID)
	if err != nil {
		return agents.RecordTagsResult{}, err
	}
	out := agents.RecordTagsResult{
		Tags:     make([]agents.RecordTagOnRecord, 0, len(read.Data)),
		Withheld: read.Withheld,
	}
	for _, t := range read.Data {
		out.Tags = append(out.Tags, agents.RecordTagOnRecord{
			TagID:          t.TagID,
			Name:           t.Name,
			Archived:       t.Archived,
			AssignedBy:     t.AssignedByName,
			AssignedByKind: t.AssignedByKind,
			AssignedAt:     t.AssignedAt.Format(time.RFC3339),
		})
	}
	return out, nil
}

// RecordTagTypes answers the store's own list of served types.
func (a tagAdapter) RecordTagTypes() []string { return collections.RecordTagTypesServed() }

// ListTags hands the collections module's own cross-module shape straight
// through: TagVocabulary was written for a caller outside that module and
// this is the first one, so there is nothing here to translate beyond the
// field names the wire uses.
func (a tagAdapter) ListTags(ctx context.Context, includeArchived bool) ([]agents.Tag, bool, error) {
	rows, truncated, err := a.store.TagVocabulary(ctx, includeArchived)
	if err != nil {
		return nil, false, err
	}
	out := make([]agents.Tag, 0, len(rows))
	for _, r := range rows {
		out = append(out, agents.Tag{TagID: r.TagID, Name: r.Name, Color: r.Color, Archived: r.Archived})
	}
	return out, truncated, nil
}

func (a tagAdapter) EnsureTaggable(ctx context.Context, entityType string, entityID ids.UUID) error {
	return a.store.EnsureTaggable(ctx, entityType, entityID)
}

func (a tagAdapter) FindTag(ctx context.Context, name string) (ids.UUID, bool, error) {
	return a.store.FindTag(ctx, name)
}

// TaggableTypes hands through the collections module's own list, so the tool
// schemas' record_type enum and the store's CHECK cannot drift apart.
func (a tagAdapter) TaggableTypes() []string {
	return collections.TaggableEntityTypes()
}

func (a tagAdapter) ApplyTag(ctx context.Context, tagID ids.UUID, entityType string, entityID ids.UUID) error {
	_, err := a.store.ApplyTag(ctx, ids.From[ids.TagKind](tagID), entityType, entityID)
	return err
}

func (a tagAdapter) RemoveTag(ctx context.Context, tagID ids.UUID, entityType string, entityID ids.UUID) error {
	return a.store.RemoveTag(ctx, ids.From[ids.TagKind](tagID), entityType, entityID)
}

// --- the vocabulary verbs ---
//
// Each hands straight to the store method the HTTP handler and the admin card
// already call, so an agent coining or renaming a word takes exactly the gates
// a person does — `tag.create` and `tag.update`, which the seeded roles give
// Admin and Ops alone. Re-deriving any of that here would be a second write
// gate to keep in step with the first.

// tagOf renders a written row as the WORD it now is. Not TagDetail: that type
// carries usage counts, and a write has not counted anything — filling them
// with zeroes would report a word on fifty records as carried by none.
//
// It takes the row's fields rather than the row, because the store's row type
// is unexported: compose may hold one of those values but cannot name it, and
// exporting a type so one helper can spell it would widen the store's surface
// for this file's convenience.
func tagOf(id ids.TagID, name string, color *string, archived bool) agents.Tag {
	out := agents.Tag{TagID: id.UUID, Name: name, Archived: archived}
	if color != nil {
		out.Color = *color
	}
	return out
}

// The description is nil because `create_tag`'s schema does not offer one: a
// tool that advertised the field would have to explain a word in a sentence
// the model invented, and the HTTP door is where a curator writes that.
func (a tagAdapter) CreateTag(ctx context.Context, name string, color *string) (agents.Tag, error) {
	row, err := a.store.CreateTag(ctx, name, color, nil)
	if err != nil {
		return agents.Tag{}, err
	}
	return tagOf(row.ID, row.Name, row.Color, row.ArchivedAt != nil), nil
}

// MergeTags hands the store the two ids and nothing else: which tag survives is
// the caller's decision, already made and already approved by a human, and the
// store owns every refusal that remains (a self-merge, an archived target, a
// word the caller may not read).
func (a tagAdapter) MergeTags(ctx context.Context, source, target ids.UUID) (agents.TagMergeResult, error) {
	// Unpinned on the agent path. The staged row carries ONE pinned version and
	// it is the source's, so there is nothing here to check the survivor
	// against yet — closing that needs approvals to carry a second pin.
	out, err := a.store.MergeTags(ctx, ids.From[ids.TagKind](source), ids.From[ids.TagKind](target), 0)
	if err != nil {
		return agents.TagMergeResult{}, err
	}
	return agents.TagMergeResult{Moved: out.Moved, Collapsed: out.Collapsed}, nil
}

func (a tagAdapter) UpdateTag(ctx context.Context, tagID ids.UUID, in agents.TagEdit) (agents.Tag, error) {
	// expectedVersion 0: the tool takes no If-Match, and zero is the store's
	// own spelling for "the caller sent none and accepts last-write-wins" —
	// the same answer the HTTP door gives a request without the header.
	row, err := a.store.UpdateTag(ctx, ids.From[ids.TagKind](tagID), collections.TagUpdate{
		Name:        in.Name,
		Color:       in.Color,
		Description: in.Description,
	}, 0)
	if err != nil {
		return agents.Tag{}, err
	}
	return tagOf(row.ID, row.Name, row.Color, row.ArchivedAt != nil), nil
}
