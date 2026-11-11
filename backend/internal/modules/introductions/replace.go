// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package introductions

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

func (s *Store) cancelTx(ctx context.Context, tx pgx.Tx, id ids.UUID, reason string, version int) error {
	return s.moveTx(ctx, tx, id, func() Status { return StatusCancelled }, version, func(cur *Request) error {
		return May(cur.Status, StatusCancelled, s.roleOf(ctx, cur))
	}, func(ctx context.Context, tx pgx.Tx, cur *Request) (pgconn.CommandTag, error) {
		return tx.Exec(ctx, `
			UPDATE intro_request
			   SET status = 'cancelled', decision_reason = @reason, closed_at = @closed_at,
			       version = version + 1, updated_at = now()
			 WHERE id = @id AND version = @version`,
			pgx.StrictNamedArgs{"id": id, "reason": nullIfEmpty(reason), "closed_at": s.now(), "version": version})
	}, func(cur *Request) events.Payload {
		return crmcontracts.PublicEventIntroRequestClosed{
			IntroRequestId: openapi_types.UUID(id),
			ContactId:      openapi_types.UUID(cur.ContactID),
			Reason:         crmcontracts.IntroRequestClosedCancelled,
		}
	})
}

// Replace makes a fresh ask without carrying the original colleague’s approval.
func (s *Store) Replace(ctx context.Context, id ids.UUID, reason string, version int, req NewRequest) (ids.UUID, error) {
	if err := requireReplacement(ctx); err != nil {
		return ids.UUID{}, err
	}
	if strings.TrimSpace(reason) == "" {
		return ids.UUID{}, &values.ParseError{Field: "reason", Code: codeRequired, Message: "explain why this ask is being replaced"}
	}
	if err := tooLong("reason", reason, reasonBound); err != nil {
		return ids.UUID{}, err
	}
	actor, err := requesterOf(ctx, req)
	if err != nil {
		return ids.UUID{}, err
	}
	req, err = admitAsk(ctx, req)
	if err != nil {
		return ids.UUID{}, err
	}
	capturedBy, err := storekit.CapturedBy(ctx)
	if err != nil {
		return ids.UUID{}, err
	}
	replacement := ids.NewV7()
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		current, err := s.load(ctx, tx, id)
		if err != nil {
			return err
		}
		if current.RequesterUserID != actor.UserID {
			return apperrors.ErrPermissionDenied
		}
		if sameAskRoute(current, req) {
			return &values.ParseError{Field: fieldIntroducerUserID, Code: "same_route", Message: "choose another colleague or a different route"}
		}
		req.ContactID = current.ContactID
		if err := prepareAskRouteTx(ctx, tx, req); err != nil {
			return err
		}
		if err := s.cancelTx(ctx, tx, id, reason, version); err != nil {
			return err
		}
		return s.createTx(ctx, tx, replacement, actor, req, capturedBy)
	})
	if err != nil {
		return ids.UUID{}, err
	}
	return replacement, nil
}

func requireReplacement(ctx context.Context) error {
	if err := auth.Require(ctx, "introduction", principal.ActionRead); err != nil {
		return err
	}
	if err := auth.Require(ctx, "introduction", principal.ActionCreate); err != nil {
		return err
	}
	if err := auth.Require(ctx, "introduction", principal.ActionUpdate); err != nil {
		return err
	}

	return auth.Require(ctx, "contact", principal.ActionRead)
}

func sameAskRoute(current *Request, req NewRequest) bool {
	if current.IntroducerUser != req.IntroducerUser || current.RouteType != req.RouteType {
		return false
	}
	if current.ThroughContactID == nil || req.ThroughContactID == nil {
		return current.ThroughContactID == nil && req.ThroughContactID == nil
	}
	return *current.ThroughContactID == *req.ThroughContactID
}
