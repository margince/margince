// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// EntityNameReader keeps the default company's name with the owner of its profile.
type EntityNameReader func(context.Context, pgx.Tx, ids.UUID) (string, error)

// WithEntityNames uses the company profile owner for names that consent does not store.
func (s *Store) WithEntityNames(reader EntityNameReader) *Store {
	s.entityNames = reader
	return s
}

// WithEntityNames shares company identity resolution across reviews and exception lists.
func (h Handlers) WithEntityNames(reader EntityNameReader) Handlers {
	h.store = h.store.WithEntityNames(reader)
	return h
}

func (s *Store) nameReviewEntities(ctx context.Context, tx pgx.Tx, refusals []RefusedRecipient) error {
	entities := make([]ids.UUID, 0, len(refusals))
	for _, refusal := range refusals {
		if refusal.SendingEntity != nil {
			entities = append(entities, *refusal.SendingEntity)
		}
	}
	names, err := s.readEntityNames(ctx, tx, entities)
	if err != nil {
		return err
	}
	for i := range refusals {
		if refusals[i].SendingEntity != nil {
			refusals[i].SendingEntityName = names[*refusals[i].SendingEntity]
		}
	}
	return nil
}

func (s *Store) readEntityNames(ctx context.Context, tx pgx.Tx, entities []ids.UUID) (map[ids.UUID]string, error) {
	names := make(map[ids.UUID]string, len(entities))
	if s.entityNames == nil {
		return names, nil
	}
	for _, entity := range entities {
		if _, known := names[entity]; known {
			continue
		}
		name, err := s.entityNames(ctx, tx, entity)
		if err != nil {
			return nil, fmt.Errorf("consent: reading a sending company's name: %w", err)
		}
		names[entity] = name
	}
	return names, nil
}

func (s *Store) nameListedEntities(ctx context.Context, tx pgx.Tx, entries []listedOverride) error {
	entities := make([]ids.UUID, 0, len(entries))
	for _, entry := range entries {
		if entry.value.SendingEntityId != nil {
			entities = append(entities, ids.UUID(*entry.value.SendingEntityId))
		}
	}
	names, err := s.readEntityNames(ctx, tx, entities)
	if err != nil {
		return err
	}
	for i := range entries {
		id := entries[i].value.SendingEntityId
		if id == nil {
			continue
		}
		if name := names[ids.UUID(*id)]; name != "" {
			entries[i].value.SendingEntityName = &name
		}
	}
	return nil
}
