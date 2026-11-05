// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// An upload wins over the Google picture, and a delete pins initials until the next upload.
// The store is blob-free: callers put the bytes first and collect the key a write hands back.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/storedobjects"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/assetrev"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// AvatarOrigin is who put a seat's picture there, as app_user.avatar_origin holds it.
type AvatarOrigin string

const (
	AvatarUploaded   AvatarOrigin = "upload"
	AvatarFromGoogle AvatarOrigin = "google"
)

// avatarFillableSQL is the whole promise a provider fill keeps: it writes only
// a seat whose member neither uploaded a picture nor deleted one.
const avatarFillableSQL = `avatar_removed_at IS NULL AND (avatar_origin IS NULL OR avatar_origin = 'google')`

// avatarRevision versions how a stored picture is rendered, inside every URL.
const avatarRevision = "avatar-v1"

const (
	avatarOriginField    = "avatar_origin"
	avatarRemovedAtField = "avatar_removed_at"
)

// AvatarURL is where a client fetches a seat's picture, nil when it has none.
// The key never reaches the wire; its digest is the cache-busting token.
func AvatarURL(userID ids.UUID, objectKey *string) *string {
	if objectKey == nil || *objectKey == "" {
		return nil
	}
	path := fmt.Sprintf("/v1/users/%s/avatar?v=%s", userID.String(), AvatarETag(*objectKey))
	return &path
}

// AvatarETag names the bytes at key with the same token AvatarURL carries.
func AvatarETag(objectKey string) string {
	return assetrev.Digest(avatarRevision, objectKey)
}

type avatarState struct {
	key       *string
	origin    *string
	removedAt *time.Time
}

// pinned reports a seat its member took the picture off.
func (a avatarState) pinned() bool { return a.key == nil && a.removedAt != nil }

// SetMyAvatar points the CALLER's own seat at uploaded bytes and hands back the
// key they replaced. Human-only, like the other self-service saves.
func (s *Service) SetMyAvatar(ctx context.Context, objectKey string) (Seat, *string, error) {
	human, err := selfScoped(ctx)
	if err != nil {
		return Seat{}, nil, err
	}
	if objectKey == "" {
		return Seat{}, nil, errors.New("identity: an uploaded picture needs the key its bytes were stored at")
	}
	origin := string(AvatarUploaded)
	return s.writeMyAvatar(ctx, ids.From[ids.UserKind](human), avatarState{key: &objectKey, origin: &origin})
}

// RemoveMyAvatar takes the CALLER's picture off and hands back its key. A seat with
// none is pinned all the same, so a later Google sign-in cannot give it one.
func (s *Service) RemoveMyAvatar(ctx context.Context) (Seat, *string, error) {
	human, err := selfScoped(ctx)
	if err != nil {
		return Seat{}, nil, err
	}
	removedAt := s.now()
	return s.writeMyAvatar(ctx, ids.From[ids.UserKind](human), avatarState{removedAt: &removedAt})
}

func (s *Service) writeMyAvatar(ctx context.Context, user ids.UserID, after avatarState) (Seat, *string, error) {
	seat := Seat{UserID: user}
	var dropped *string
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		before, err := lockOwnAvatar(ctx, tx, &seat)
		if err != nil {
			return err
		}
		if before.pinned() && after.pinned() {
			return nil
		}
		tag, err := tx.Exec(ctx,
			`UPDATE app_user SET avatar_object_key = $2, avatar_origin = $3, avatar_removed_at = $4
			  WHERE id = $1 AND `+LiveMemberSQL(""), user, after.key, after.origin, after.removedAt)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return apperrors.ErrNotFound
		}
		dropped = before.key
		return recordAvatarChange(ctx, tx, user, before, after)
	})
	if err != nil {
		return Seat{}, nil, fmt.Errorf("identity: saving the caller's picture: %w", err)
	}
	seat.AvatarKey = after.key
	return seat, dropped, nil
}

func lockOwnAvatar(ctx context.Context, tx pgx.Tx, seat *Seat) (avatarState, error) {
	var before avatarState
	err := tx.QueryRow(ctx,
		`SELECT email, display_name, COALESCE(locale, ''), greeting_name,
		        avatar_object_key, avatar_origin, avatar_removed_at
		   FROM app_user WHERE id = $1 AND `+LiveMemberSQL("")+` FOR UPDATE`, seat.UserID).
		Scan(&seat.Email, &seat.DisplayName, &seat.Locale, &seat.GreetingName,
			&before.key, &before.origin, &before.removedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return avatarState{}, apperrors.ErrNotFound
	}
	return before, err
}

// FillAvatarFromProvider points a seat at a provider picture's key while avatarFillableSQL
// holds. It reports whether the row names the key afterwards, and the key it replaced.
func (s *Service) FillAvatarFromProvider(ctx context.Context, user ids.UserID, objectKey string) (bool, *string, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return false, nil, err
	}
	var written bool
	var dropped *string
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		var before avatarState
		err := tx.QueryRow(ctx,
			`SELECT avatar_object_key, avatar_origin, avatar_removed_at
			   FROM app_user WHERE id = $1 AND `+LiveMemberSQL("")+` FOR UPDATE`, user).
			Scan(&before.key, &before.origin, &before.removedAt)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		if before.key != nil && *before.key == objectKey {
			written = true
			return storedobjects.Clear(ctx, tx, objectKey)
		}
		origin := string(AvatarFromGoogle)
		tag, err := tx.Exec(ctx,
			`UPDATE app_user SET avatar_object_key = $2, avatar_origin = $3
			  WHERE id = $1 AND `+avatarFillableSQL+` AND `+LiveMemberSQL(""), user, objectKey, origin)
		if err != nil || tag.RowsAffected() != 1 {
			return err
		}
		written, dropped = true, before.key
		return recordAvatarChange(selfActorCtx(ctx, user), tx, user, before,
			avatarState{key: &objectKey, origin: &origin})
	})
	if err != nil {
		return false, nil, fmt.Errorf("identity: filling a seat's picture from its provider: %w", err)
	}
	return written, dropped, nil
}

// The dropped key turns provisional, so the sweep collects it if the caller's delete fails.
// Every writer is the member's own act, so the event names them as who changed it.
func recordAvatarChange(ctx context.Context, tx pgx.Tx, user ids.UserID, before, after avatarState) error {
	if after.key != nil {
		if err := storedobjects.Clear(ctx, tx, *after.key); err != nil {
			return err
		}
	}
	if before.key != nil && *before.key != "" {
		if err := storedobjects.RecordTx(ctx, tx, storedobjects.KindAvatar, *before.key); err != nil {
			return err
		}
	}
	auditID, err := storekit.Audit(ctx, tx, "update", "user", user.UUID, avatarImage(before), avatarImage(after))
	if err != nil {
		return err
	}
	return storekit.EmitEvent(ctx, tx, auditID, user.UUID, crmcontracts.PublicEventUserUpdated{
		UserId: openapi_types.UUID(user.UUID), By: openapi_types.UUID(user.UUID),
	})
}

// avatarImage is one side of the audit pair. The key stays out: it names a
// bucket path, and who chose the picture is what the trail is for.
func avatarImage(a avatarState) map[string]any {
	return map[string]any{avatarOriginField: a.origin, avatarRemovedAtField: a.removedAt}
}

// SeatAvatarKey is where a colleague's picture lives. No picture, an archived seat and an
// unknown id are all ErrNotFound: the client draws initials for each.
func (s *Service) SeatAvatarKey(ctx context.Context, user ids.UserID) (string, error) {
	if err := auth.RequireMember(ctx); err != nil {
		return "", err
	}
	var key *string
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT avatar_object_key FROM app_user WHERE id = $1 AND archived_at IS NULL`, user).Scan(&key)
	})
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && (key == nil || *key == "")) {
		return "", apperrors.ErrNotFound
	}
	if err != nil {
		return "", err
	}
	return *key, nil
}

// UnreferencedAvatarKeys answers which keys no seat names, for the stored-object sweep.
func (s *Service) UnreferencedAvatarKeys(ctx context.Context, keys []string) ([]string, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return nil, err
	}
	if len(keys) == 0 {
		return nil, nil
	}
	var out []string
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			`SELECT k FROM unnest($1::text[]) AS k
			  WHERE NOT EXISTS (SELECT 1 FROM app_user u WHERE u.avatar_object_key = k)`, keys)
		if err != nil {
			return err
		}
		out, err = pgx.CollectRows(rows, pgx.RowTo[string])
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("identity: checking which picture keys are unreferenced: %w", err)
	}
	return out, nil
}
