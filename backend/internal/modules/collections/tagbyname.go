// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package collections

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// Finding a word by the name a person typed, and saying which of the three
// answers it is: a live tag, a retired one, or none at all. Kept apart from the
// tag writes because a caller asking "is there a word for this?" is asking
// something else than a caller applying one.

// FindTag answers the id of the LIVE tag with this name, or ok=false.
//
// Case-insensitive, matching the uq_tag_name index, and live-only: an archived
// word was retired on purpose and is not what a caller naming it means.
func (s *Store) FindTag(ctx context.Context, name string) (ids.UUID, bool, error) {
	id, state, err := s.lookupTagByName(ctx, name)
	return id, state == tagNameLive, err
}

// TagNameState says which of the three answers a name lookup gave, because a
// caller that has to explain a refusal cannot tell them apart from an id and a
// bool: an unknown word and a retired one need different sentences, and
// collapsing them tells somebody their tag does not exist when an admin can
// restore it.
type TagNameState int

const (
	tagNameMissing TagNameState = iota
	tagNameLive
	tagNameArchived
)

// LookupTagName resolves a name to its id and which state it is in.
func (s *Store) LookupTagName(ctx context.Context, name string) (ids.UUID, TagNameState, error) {
	return s.lookupTagByName(ctx, name)
}

// Archived returns whether the name resolved to a retired tag.
func (st TagNameState) Archived() bool { return st == tagNameArchived }

// Live returns whether the name resolved to a tag that may be applied.
func (st TagNameState) Live() bool { return st == tagNameLive }

func (s *Store) lookupTagByName(ctx context.Context, name string) (ids.UUID, TagNameState, error) {
	if err := auth.Require(ctx, "tag", principal.ActionRead); err != nil {
		return ids.UUID{}, tagNameMissing, err
	}
	var id ids.TagID
	var archived *time.Time
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		// Live first, then the most recently retired. uq_tag_name binds live
		// rows only, so a name can be held by one live tag AND any number of
		// archived ones — without the ordering this picks whichever row the
		// planner reaches, and a caller naming a word they can see would be
		// told it is archived.
		return tx.QueryRow(ctx, `
			SELECT id, archived_at FROM tag
			 WHERE lower(name) = lower($1)
			 ORDER BY archived_at IS NOT NULL, archived_at DESC
			 LIMIT 1`, NormalizeTagName(name)).Scan(&id, &archived)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return ids.UUID{}, tagNameMissing, nil
	}
	if err != nil {
		return ids.UUID{}, tagNameMissing, fmt.Errorf("collections: finding tag by name: %w", err)
	}
	if archived != nil {
		return id.UUID, tagNameArchived, nil
	}
	return id.UUID, tagNameLive, nil
}

// TagSummary is the tag as another module sees it. tagRow is unexported
// because its shape is this store's business; the seam that carries a tag
// across a module boundary carries this instead.
type TagSummary struct {
	TagID    ids.UUID
	Name     string
	Color    string
	Archived bool
}
