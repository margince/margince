// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package introductions

// Recording an introduction that already happened outside Margince.
//
// There is no ask behind such a row, so it skips the lifecycle: it is born
// introduced and stays so. The lifecycle writers in this module leave it alone
// by asking the origin, and the unique index on the pair turns a double click
// into a conflict.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
	"github.com/margince/margince/backend/internal/shared/kernel/workforce"
)

// NewRecorded is an introduction that already happened. The introduced seat is
// stored as the row's requester, the seat the introduction was made for.
type NewRecorded struct {
	ContactID      ids.UUID
	IntroducerUser ids.UUID
	IntroducedUser ids.UUID
	On             time.Time
	Channel        string
}

// Record writes one recorded introduction and returns its id.
//
// The caller must be the introducer or the introduced colleague: either can
// vouch for it, nobody else can. The caller is never a field on the input.
func (s *Store) Record(ctx context.Context, in NewRecorded) (ids.UUID, error) {
	if err := auth.Require(ctx, "introduction", principal.ActionCreate); err != nil {
		return ids.UUID{}, err
	}
	if err := auth.Require(ctx, "contact", principal.ActionRead); err != nil {
		return ids.UUID{}, err
	}
	if err := auth.RequireHuman(ctx); err != nil {
		return ids.UUID{}, err
	}
	actor, ok := principal.Actor(ctx)
	if !ok || actor.UserID.IsZero() {
		return ids.UUID{}, apperrors.ErrPermissionDenied
	}
	if err := admitRecording(in, s.now()); err != nil {
		return ids.UUID{}, err
	}
	if err := MayRecord(recorderRole(actor.UserID, in)); err != nil {
		return ids.UUID{}, err
	}
	capturedBy, err := storekit.CapturedBy(ctx)
	if err != nil {
		return ids.UUID{}, err
	}

	id := ids.NewV7()
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		if err := auth.EnsureVisibleLive(ctx, tx, "contact", in.ContactID); err != nil {
			return err
		}
		if err := lockNamedContacts(ctx, tx, in.ContactID, nil); err != nil {
			return err
		}
		if err := requireLiveSeats(ctx, tx, in.IntroducerUser, in.IntroducedUser); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO intro_request (
				id, contact_id, requester_user_id, introducer_user_id, route_type,
				origin, recorded_channel, status, introduced_at, captured_by)
			VALUES ($1,$2,$3,$4,'direct','recorded',$5,'introduced',$6,$7)`,
			id, in.ContactID, in.IntroducedUser, in.IntroducerUser,
			in.Channel, in.On, capturedBy)
		if err != nil {
			if storekit.IsUniqueViolation(err) {
				return fmt.Errorf(
					"introductions: this introduction is already recorded for this contact: %w",
					apperrors.ErrConflict)
			}
			return fmt.Errorf("introductions: recording the introduction: %w", err)
		}
		auditID, err := storekit.AuditEvent(ctx, tx, "create", "intro_request", id,
			map[string]any{
				"contact_id":         in.ContactID.String(),
				"introducer_user_id": in.IntroducerUser.String(),
				"introduced_user_id": in.IntroducedUser.String(),
				"origin":             string(OriginRecorded),
				"recorded_channel":   in.Channel,
			})
		if err != nil {
			return err
		}
		return storekit.EmitEvent(ctx, tx, auditID, in.ContactID,
			crmcontracts.PublicEventIntroRequestRecorded{
				IntroRequestId:   openapi_types.UUID(id),
				ContactId:        openapi_types.UUID(in.ContactID),
				IntroducerUserId: openapi_types.UUID(in.IntroducerUser),
				IntroducedUserId: openapi_types.UUID(in.IntroducedUser),
			})
	})
	if err != nil {
		return ids.UUID{}, err
	}
	return id, nil
}

// recorderRole names which of the two colleagues the caller is, or "" for
// anybody else, in the terms MayRecord judges.
func recorderRole(caller ids.UUID, in NewRecorded) Actor {
	switch caller {
	case in.IntroducerUser:
		return ActorIntroducer
	case in.IntroducedUser:
		return ActorRequester
	default:
		return ""
	}
}

// admitRecording refuses what the contract does not admit before any query.
func admitRecording(in NewRecorded, now time.Time) error {
	if in.IntroducerUser == in.IntroducedUser {
		return &values.ParseError{
			Field: "introduced_user_id", Code: "invalid",
			Message: "an introduction is between two colleagues; name a different introduced_user_id",
		}
	}
	if !crmcontracts.IntroRecordedChannel(in.Channel).Valid() {
		return notAllowed("channel", "in_person, email, call or other")
	}
	today := now.UTC().Truncate(24 * time.Hour)
	if in.On.UTC().Truncate(24 * time.Hour).After(today) {
		return &values.ParseError{
			Field: "introduced_on", Code: "in_future",
			Message: "an introduction that already happened cannot be dated in the future",
		}
	}
	return nil
}

// requireLiveSeats refuses a seat that does not exist, has left, or is an
// agent: the record says two colleagues met, and neither a departed seat nor a
// credential can have.
func requireLiveSeats(ctx context.Context, tx pgx.Tx, introducer, introduced ids.UUID) error {
	var live int
	if err := tx.QueryRow(ctx, `
		SELECT count(*) FROM app_user
		 WHERE id = ANY($1) AND NOT is_agent AND `+workforce.LiveMemberSQL(""),
		[]ids.UUID{introducer, introduced}).Scan(&live); err != nil {
		return fmt.Errorf("introductions: checking the two colleagues: %w", err)
	}
	if live != 2 {
		return &values.ParseError{
			Field: "introducer_user_id", Code: "unknown_colleague",
			Message: "both colleagues must be current members of the workspace",
		}
	}
	return nil
}
