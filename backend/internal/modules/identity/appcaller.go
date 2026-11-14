// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/owndomain"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// AppCaller is one caller an installed chat app vouches for on one call. It
// holds the app's issuer, the app's own id for the caller, and the address the
// app has verified. The caller has checked the app's signature already.
type AppCaller struct {
	Issuer  string
	Subject string
	Email   string
	// Label names the app on the passport the member sees in their settings.
	Label string
}

// ErrNoSeat is an app caller whose address belongs to no live member.
var ErrNoSeat = errors.New("identity: no live member has this address")

// appPassportScopes are the verbs an app passport carries. The member's own
// grants still bound every call, as they bound any passport.
var appPassportScopes = []string{string(principal.ScopeRead), string(principal.ScopeWrite), string(principal.ScopeSend)}

// columns is the pair the passport row stores, nil for a passport no app holds.
func (c *AppCaller) columns() (issuer, subject *string) {
	if c == nil {
		return nil, nil
	}
	return &c.Issuer, &c.Subject
}

// appBinding is the newest passport an app caller holds.
type appBinding struct {
	id         ids.PassportID
	onBehalfOf ids.UserID
	revoked    bool
	expired    bool
}

// agentAppCallerPredicate finds the passport by its id, and only while the
// address the app vouches for is still the member's login.
var agentAppCallerPredicate = `p.id = $1 AND ` + storekit.SeatLoginIsSQL("u", "$2", "$3")

// AuthenticateAppCaller resolves an app caller to the passport it acts
// through. The first call mints that passport for the member whose login is
// the vouched address; later calls find it by issuer and subject. A revoked
// passport stays refused, and so does a passport whose member no longer has
// the vouched address. An address no live member holds answers ErrNoSeat.
func (s *Service) AuthenticateAppCaller(ctx context.Context, c AppCaller) (AgentIdentity, error) {
	var a AgentIdentity
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		b, found, err := newestAppBinding(ctx, tx, c)
		if err != nil {
			return err
		}
		if !found || b.expired {
			// Taken before the second read, so two first calls mint one passport.
			if err := lockAuthorization(ctx, tx); err != nil {
				return err
			}
			if b, found, err = newestAppBinding(ctx, tx, c); err != nil {
				return err
			}
		}
		if found && b.revoked {
			return apperrors.ErrNotFound
		}
		if !found || b.expired {
			if b.id, err = mintAppPassport(ctx, tx, c, b, found); err != nil {
				return err
			}
		}
		if a, err = s.authenticateAgentWhere(ctx, tx, agentAppCallerPredicate, b.id, typedLoginAddress(c.Email), owndomain.OfAddress(c.Email)); err != nil {
			return err
		}
		return touchPassport(ctx, tx, a.PassportID)
	})
	if err != nil {
		return AgentIdentity{}, err
	}
	return a, nil
}

// newestAppBinding reads the newest passport this caller holds, revoked or not.
func newestAppBinding(ctx context.Context, tx pgx.Tx, c AppCaller) (appBinding, bool, error) {
	var b appBinding
	var revokedAt *time.Time
	err := tx.QueryRow(ctx, `
		SELECT id, on_behalf_of, revoked_at, expires_at <= now() FROM passport
		 WHERE app_issuer = $1 AND app_subject = $2
		 ORDER BY created_at DESC, id DESC LIMIT 1`, c.Issuer, c.Subject).
		Scan(&b.id, &b.onBehalfOf, &revokedAt, &b.expired)
	if errors.Is(err, pgx.ErrNoRows) {
		return appBinding{}, false, nil
	}
	if err != nil {
		return appBinding{}, false, fmt.Errorf("identity: reading an app caller's passport: %w", err)
	}
	b.revoked = revokedAt != nil
	return b, true, nil
}

// mintAppPassport mints the passport for the member whose login is the vouched
// address. Renewing an expired passport keeps the member it was minted for, so
// an address that moved to someone else is refused rather than rebound.
func mintAppPassport(ctx context.Context, tx pgx.Tx, c AppCaller, expired appBinding, renewing bool) (ids.PassportID, error) {
	var member Identity
	err := tx.QueryRow(ctx,
		`SELECT u.id, u.seat_type FROM app_user u
		 WHERE `+LiveMemberSQL("u")+` AND NOT u.is_agent AND `+storekit.SeatLoginIsSQL("u", "$1", "$2")+`
		 `+storekit.SeatLoginPrefersTypedSQL("u", "$1"),
		typedLoginAddress(c.Email), owndomain.OfAddress(c.Email)).Scan(&member.UserID, &member.SeatType)
	if errors.Is(err, pgx.ErrNoRows) {
		return ids.PassportID{}, ErrNoSeat
	}
	if err != nil {
		return ids.PassportID{}, fmt.Errorf("identity: finding the member an app vouches for: %w", err)
	}
	if renewing && member.UserID != expired.onBehalfOf {
		return ids.PassportID{}, apperrors.ErrNotFound
	}
	ttl := maxPassportTTL
	issued, err := mintPassport(ctx, tx, member, IssuePassportInput{
		Label: &c.Label, Scopes: appPassportScopes, TTL: &ttl, App: &c,
	}, nil)
	if err != nil {
		return ids.PassportID{}, err
	}
	return issued.ID, nil
}

// ErrAppBindingLive refuses to release a passport that is still live: the
// binding is released only after the passport was revoked.
var ErrAppBindingLive = errors.New("identity: revoke the passport before allowing its chat user again")

// ReleaseAppBinding lets a chat app's user be bound again after their passport
// was revoked. The passport stays revoked; it only stops naming the user, so
// their next call mints a fresh passport under the same checks as a first one.
func (s *Service) ReleaseAppBinding(ctx context.Context, id Identity, passportID ids.PassportID) error {
	ctx = actorCtx(ctx, id)
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		var onBehalfOf ids.UserID
		var revokedAt *time.Time
		var issuer, subject *string
		err := tx.QueryRow(ctx, `SELECT on_behalf_of, revoked_at, app_issuer, app_subject FROM passport
			WHERE id = $1 FOR UPDATE`, passportID).Scan(&onBehalfOf, &revokedAt, &issuer, &subject)
		if errors.Is(err, pgx.ErrNoRows) {
			return apperrors.ErrNotFound
		}
		if err != nil {
			return err
		}
		if err := mayManagePassport(ctx, tx, id, onBehalfOf); err != nil {
			return err
		}
		if issuer == nil {
			return apperrors.ErrNotFound
		}
		if revokedAt == nil {
			return ErrAppBindingLive
		}
		if _, err := tx.Exec(ctx, `UPDATE passport SET app_issuer = NULL, app_subject = NULL WHERE id = $1`, passportID); err != nil {
			return err
		}
		_, err = storekit.Audit(ctx, tx, "update", "passport", passportID.UUID,
			map[string]any{"app_issuer": *issuer, "app_subject": *subject}, map[string]any{"app_issuer": nil, "app_subject": nil})
		return err
	})
}
