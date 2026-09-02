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
