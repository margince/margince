// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// The legal entity a member works for, placed in the transaction of the invite
// or edit that names it. contacts owns the assignment and identity never imports it, so
// the composition root injects the writer.

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// EntityMembership assigns user to entity inside the caller's transaction,
// asking the same grants the assignment route asks. An unknown or archived
// entity answers apperrors.ErrNotFound.
type EntityMembership func(ctx context.Context, tx pgx.Tx, entity ids.LegalEntityID, user ids.UserID) error

// WithEntityMembership binds the writer on the SERVICE, so every surface that
// invites or edits a member can place them.
func (h Handlers) WithEntityMembership(assign EntityMembership) Handlers {
	if h.svc == nil {
		return h
	}
	h.svc.entityMembership = assign
	return h
}

// fieldLegalEntityID is the request field that names the member's entity.
const fieldLegalEntityID = "legal_entity_id"

var errEntityMembershipUnwired = errors.New("identity: nothing places a member in a legal entity")

// placeMember assigns the member to the entity the request named. No entity
// changes nothing: a new member stays with the default one. The entity is a
// field of the request, so an unknown or archived one is a 422 on that field,
// not a 404.
func (s *Service) placeMember(ctx context.Context, tx pgx.Tx, entity *ids.LegalEntityID, user ids.UserID) error {
	if entity == nil {
		return nil
	}
	if s.entityMembership == nil {
		return errEntityMembershipUnwired
	}
	err := s.entityMembership(ctx, tx, *entity, user)
	if errors.Is(err, apperrors.ErrNotFound) {
		return &values.ParseError{
			Field: fieldLegalEntityID, Code: "unknown_legal_entity",
			Message: "legal_entity_id names no live legal entity; choose one from listLegalEntities",
		}
	}
	return err
}

// legalEntityOf reads the optional legal_entity_id a request sends.
func legalEntityOf(field *openapi_types.UUID) *ids.LegalEntityID {
	if field == nil {
		return nil
	}
	named := ids.From[ids.LegalEntityKind](ids.UUID(*field))
	return &named
}
