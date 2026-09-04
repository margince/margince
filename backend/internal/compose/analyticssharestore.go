// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Issuing, resolving and revoking a share link.
//
// A share is the one object in this tree that outlives the request that made
// it and answers to somebody who is not signed in as its author. Three rules
// follow from that and none of them is optional:
//
//   - The raw token is returned ONCE and never stored. What the table holds is
//     a digest, so a database dump opens nothing.
//   - The expiry is capped by the server, not by the caller. A caller-chosen
//     expiry is a caller-chosen "forever".
//   - Opening re-checks the ISSUER's standing. A link is not a grant that
//     survives its author's seat.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/forecasting"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/bearer"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ShareLifetimeCeiling is the longest a share may live.
//
// Thirty days, and the number is the point rather than the value: a share the
// caller dates is a share that never expires, because the caller who wants it
// open indefinitely will type indefinitely. Somebody who needs longer issues a
// second link, which is a decision that leaves a record.
const ShareLifetimeCeiling = 30 * 24 * time.Hour

// The two kinds a share can be, as strings. Derived from the contract's own
// enum rather than spelled again here: the wire and the column then cannot
// come to disagree about what "snapshot" is called.
var (
	shareKindLive     = string(crmcontracts.ShareKindLive)
	shareKindSnapshot = string(crmcontracts.ShareKindSnapshot)
)

// tableAnalyticsShare is the audited entity type for a share.
const tableAnalyticsShare = "analytics_share"

// objectForecast is the RBAC object a share reads and writes under. A share
// grants no more than the forecast itself does.
const objectForecast = "forecast"

// NewShare is a request to issue one.
type NewShare struct {
	Kind       string
	Target     string
	Scope      forecasting.Scope
	SnapshotID *ids.UUID
	ExpiresAt  time.Time
}

// Share is one issued link as the table holds it, without its token.
type Share struct {
	ID         ids.UUID
	Kind       string
	Target     string
	Scope      forecasting.Scope
	SnapshotID *ids.UUID
	CreatedBy  ids.UserID
	ExpiresAt  time.Time
	RevokedAt  *time.Time
	CreatedAt  time.Time
}

// AnalyticsShareStore issues and resolves share links.
type AnalyticsShareStore struct {
	now func() time.Time
}

// NewAnalyticsShareStore binds the clock the ceiling is measured against.
func NewAnalyticsShareStore(now func() time.Time) *AnalyticsShareStore {
	return &AnalyticsShareStore{now: now}
}

// Issue mints a share and returns the raw token ONCE.
//
// The token is the second return value and never reaches the row. A caller
// that loses it issues another share rather than recovering this one, which is
// the property that makes the digest worth storing.
func (s *AnalyticsShareStore) Issue(
	ctx context.Context, tx pgx.Tx, in NewShare,
) (Share, string, error) {
	if err := auth.Require(ctx, objectForecast, principal.ActionCreate); err != nil {
		return Share{}, "", err
	}
	actor, ok := principal.Actor(ctx)
	if !ok {
		return Share{}, "", fmt.Errorf("compose: issuing a share without an actor")
	}
	expires, err := s.cappedExpiry(in.ExpiresAt)
	if err != nil {
		return Share{}, "", err
	}
	if err := checkShareKind(in); err != nil {
		return Share{}, "", err
	}
	capturedBy, err := storekit.CapturedBy(ctx)
	if err != nil {
		return Share{}, "", err
	}
	raw, digest, err := bearer.Mint()
	if err != nil {
		return Share{}, "", err
	}

	out := Share{
		Kind: in.Kind, Target: in.Target, Scope: in.Scope,
		SnapshotID: in.SnapshotID, CreatedBy: ids.From[ids.UserKind](actor.UserID), ExpiresAt: expires,
	}
	if err := tx.QueryRow(ctx, `
		INSERT INTO analytics_share
		    (kind, target, scope_kind, scope_id, snapshot_id, token_hash,
		     created_by, expires_at, captured_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, created_at`,
		in.Kind, in.Target, in.Scope.Kind, in.Scope.ID, in.SnapshotID,
		digest, actor.UserID, expires, capturedBy,
	).Scan(&out.ID, &out.CreatedAt); err != nil {
		return Share{}, "", fmt.Errorf("compose: issuing a share: %w", err)
	}

	// The audit records that a link exists and what it opens. It does not
	// record the token or its digest: an audit row a support engineer reads is
	// not a place to keep the thing the link is protected by.
	auditID, err := storekit.AuditEvent(ctx, tx, "create", tableAnalyticsShare, out.ID,
		map[string]any{
			paramKind:    in.Kind,
			"target":     in.Target,
			"scope_kind": in.Scope.Kind,
			"expires_at": expires,
		})
	if err != nil {
		return Share{}, "", err
	}
	if err := storekit.EmitEvent(ctx, tx, auditID, out.ID,
		crmcontracts.PublicEventForecastShareIssued{
			ShareId:   openapi_types.UUID(out.ID),
			Kind:      crmcontracts.PublicEventForecastShareIssuedKind(in.Kind),
			Target:    in.Target,
			ExpiresAt: expires,
		}); err != nil {
		return Share{}, "", err
	}
	return out, raw, nil
}

// cappedExpiry refuses an expiry beyond the ceiling rather than quietly
// shortening it.
//
// Silently clamping would hand back a link the caller believes lasts a year,
// and they would find out when it stopped working in front of whoever they
// sent it to. A refusal is a worse first minute and a better week.
func (s *AnalyticsShareStore) cappedExpiry(want time.Time) (time.Time, error) {
	now := s.now()
	if want.IsZero() {
		return now.Add(ShareLifetimeCeiling), nil
	}
	if !want.After(now) {
		return time.Time{}, fmt.Errorf(
			"%w: a share expiring in the past opens nothing", apperrors.ErrInvalidArgument)
	}
	if want.Sub(now) > ShareLifetimeCeiling {
		return time.Time{}, fmt.Errorf(
			"%w: a share may not outlive %d days; issue another when this one lapses",
			apperrors.ErrInvalidArgument, int(ShareLifetimeCeiling/(24*time.Hour)))
	}
	return want, nil
}

// checkShareKind holds the kind-and-snapshot pairing in Go as well as in the
// CHECK constraint.
//
// Both, deliberately: the constraint is what makes it true of the data, and
// this is what makes the caller's mistake an argument error rather than a
// database error surfacing as a 500.
func checkShareKind(in NewShare) error {
	switch in.Kind {
	case shareKindLive:
		if in.SnapshotID != nil {
			return fmt.Errorf(
				"%w: a live share re-runs the reading and names no frozen state",
				apperrors.ErrInvalidArgument)
		}
	case shareKindSnapshot:
		if in.SnapshotID == nil {
			return fmt.Errorf(
				"%w: a snapshot share must name the state it serves",
				apperrors.ErrInvalidArgument)
		}
	default:
		return fmt.Errorf("%w: a share is live or snapshot, not %q",
			apperrors.ErrInvalidArgument, in.Kind)
	}
	return nil
}

// Resolve turns a raw token into the share it opens, or answers not-found.
//
// Every refusal is the SAME answer — expired, revoked, unknown digest, an
// issuer who has lost the permission — because distinguishing them tells
// somebody holding a guessed token which guesses were closer.
func (s *AnalyticsShareStore) Resolve(
	ctx context.Context, tx pgx.Tx, rawToken string,
) (Share, error) {
	var out Share
	var scopeKind string
	err := tx.QueryRow(ctx, `
		SELECT id, kind, target, scope_kind, scope_id, snapshot_id,
		       created_by, expires_at, revoked_at, created_at
		FROM analytics_share
		WHERE token_hash = $1`, bearer.Digest(rawToken),
	).Scan(&out.ID, &out.Kind, &out.Target, &scopeKind, &out.Scope.ID,
		&out.SnapshotID, &out.CreatedBy, &out.ExpiresAt, &out.RevokedAt, &out.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Share{}, apperrors.ErrNotFound
	}
	if err != nil {
		return Share{}, fmt.Errorf("compose: resolving a share: %w", err)
	}
	out.Scope.Kind = scopeKind

	if out.RevokedAt != nil || !out.ExpiresAt.After(s.now()) {
		return Share{}, apperrors.ErrNotFound
	}
	// The issuer's standing, as it stands now rather than as it stood when
	// they issued it. A link is not a grant that outlives the seat behind it.
	holds, err := identity.IssuerStillHolds(ctx, tx, out.CreatedBy, objectForecast, principal.ActionRead)
	if err != nil {
		return Share{}, err
	}
	if !holds {
		return Share{}, apperrors.ErrNotFound
	}
	return out, nil
}

// Revoke closes a share before its expiry. Idempotent: revoking a revoked
// share is the outcome the caller asked for.
func (s *AnalyticsShareStore) Revoke(ctx context.Context, tx pgx.Tx, id ids.UUID) error {
	// CREATE, not delete. No role holds forecast:delete — a forecast reading is
	// derived and a call supersedes rather than being rewritten, so the object
	// is seeded create+read and nothing grants the verb (policy/defaults.go).
	// Gating revocation on it made every share permanent: the issuer could mint
	// a link and no seat in the installation could close it.
	//
	// Create is the right verb rather than a workaround. Revoking is not
	// deleting a forecast; it is withdrawing a link the same seat was allowed
	// to issue, and whoever may issue one may close one.
	if err := auth.Require(ctx, objectForecast, principal.ActionCreate); err != nil {
		return err
	}
	// The row is locked before it is read-modified-written. Idempotence here
	// is COALESCE over a value the statement itself reads, so two concurrent
	// revocations without the lock would each see NULL and each write their own
	// instant — and the audit rows would then disagree about when the link
	// stopped serving, which is the one fact an access review asks this table
	// for.
	if _, err := storekit.LockRow(ctx, tx, tableAnalyticsShare, id, storekit.NoArchiveColumn); err != nil {
		return err
	}
	var revoked time.Time
	err := tx.QueryRow(ctx, `
		UPDATE analytics_share
		SET revoked_at = COALESCE(revoked_at, $2), version = version + 1, updated_at = now()
		WHERE id = $1
		RETURNING revoked_at`, id, s.now()).Scan(&revoked)
	if errors.Is(err, pgx.ErrNoRows) {
		return apperrors.ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("compose: revoking a share: %w", err)
	}
	auditID, err := storekit.AuditEvent(ctx, tx, "delete", tableAnalyticsShare, id,
		map[string]any{"revoked_at": revoked})
	if err != nil {
		return err
	}
	return storekit.EmitEvent(ctx, tx, auditID, id,
		crmcontracts.PublicEventForecastShareRevoked{
			ShareId:   openapi_types.UUID(id),
			RevokedAt: revoked,
		})
}
