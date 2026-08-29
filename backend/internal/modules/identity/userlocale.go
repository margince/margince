// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// A member's own display language.
//
// Distinct from the installation's base language in both what it governs and
// who may set it. `installation.base_language` is what AI writes in when the
// whole team reads the result, and it is admin/ops. This is what ONE person's
// interface is rendered in, it is theirs alone, and no seat — including admin —
// sets it for somebody else through this API.
//
// The column has existed since the baseline and nothing ever wrote to it: the
// frontend kept the choice in localStorage, so it was lost on the next browser
// and invisible to the server. Reading it back is what lets a brief written for
// this reader come out in the language they actually read.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// localeField is the name this preference answers to on every surface: the
// audit images, the outbox payload, and the field a 422 points the form at.
// One spelling, so a refusal names the control the reader actually used.
const localeField = "locale"

// SaveMyLocale records the language the CALLER's own interface is rendered in.
//
// Self-scoped from the authenticated principal, never from a body field: an id
// on the request would be an admin's way to set a colleague's display language,
// which is not a thing this API offers. There is no object grant to check for
// the same reason — holding a seat is the whole authority needed to choose the
// language you read in.
func (s *Service) SaveMyLocale(ctx context.Context, locale string) (Seat, error) {
	// Deliberately NOT actingHuman: that resolves the human an agent is acting
	// UNDER, which is right for attributing work and wrong here. A display
	// language is a preference about a person's own screen, and an agent
	// carrying its grantor's authority must not change what its grantor reads.
	actor, ok := principal.Actor(ctx)
	if !ok || actor.Type != principal.PrincipalHuman || actor.UserID.IsZero() {
		return Seat{}, apperrors.ErrPermissionDenied
	}
	human := actor.UserID
	if !textlang.Known(locale) {
		// The catalogs are the limit, not a policy: a locale the product ships
		// no messages for renders as raw message keys, which is worse than
		// refusing the choice.
		return Seat{}, &UnknownLocaleError{Locale: locale}
	}
	seat := Seat{UserID: ids.From[ids.UserKind](human), Locale: locale}
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		var before *string
		if err := tx.QueryRow(ctx,
			`SELECT email, display_name, locale
			   FROM app_user WHERE id = $1 AND archived_at IS NULL`,
			human).Scan(&seat.Email, &seat.DisplayName, &before); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return apperrors.ErrNotFound
			}
			return err
		}
		if before != nil && *before == locale {
			// Re-choosing the language already stored writes nothing and
			// publishes nothing. A settings page that saves on every render
			// would otherwise fill the ledger with a change nobody made.
			return nil
		}
		if _, err := tx.Exec(ctx,
			`UPDATE app_user SET locale = $2
			  WHERE id = $1 AND archived_at IS NULL`, human, locale); err != nil {
			return err
		}
		// The locale is in the audit payload where a signature's text is not:
		// which language somebody reads in is not private the way the words
		// they sign their mail with are, and a subscriber deciding which
		// catalog to render needs the value rather than the fact of a change.
		auditID, err := storekit.Audit(ctx, tx, "update", "user", human,
			localeImage(before), localeImage(&locale))
		if err != nil {
			return err
		}
		return storekit.EmitEvent(ctx, tx, auditID, human,
			crmcontracts.PublicEventUserLocaleChanged{Locale: locale})
	})
	if err != nil {
		return Seat{}, fmt.Errorf("identity: saving the caller's display language: %w", err)
	}
	return seat, nil
}

// Seat is who the caller is, after a write to their own row — enough to answer
// with the seat as it now stands without a second read.
type Seat struct {
	UserID      ids.UserID
	Email       string
	DisplayName string
	Locale      string
}

// localeImage renders one side of the audit's before/after pair.
//
// A member who never chose has no locale, and that is a different fact from
// having chosen English — so the key carries JSON null rather than "". The
// pointer is what encodes that: `*string(nil)` marshals to null, where an empty
// string would read as a choice somebody made.
func localeImage(locale *string) map[string]*string {
	return map[string]*string{localeField: locale}
}

// contractLocale renders a member's chosen language on the wire.
//
// Nil when they never chose. The contract says absent means "follows their
// browser", so sending "" instead would tell a client that somebody made a
// choice whose value is the empty string.
func contractLocale(locale string) *crmcontracts.UserLocale {
	if locale == "" {
		return nil
	}
	out := crmcontracts.UserLocale(locale)
	return &out
}

// UnknownLocaleError names the field and the languages that would work, so the
// refusal reaches the settings form against the control the reader used rather
// than as a sentence at the bottom of the page.
type UnknownLocaleError struct{ Locale string }

func (e *UnknownLocaleError) Error() string {
	return fmt.Sprintf("locale %q is not a language this build ships a catalog for", e.Locale)
}

// FieldFault carries it to a 422 naming `locale` on every surface.
func (e *UnknownLocaleError) FieldFault() (field, code, message string) {
	return localeField, "invalid", "a display language is one of en, de, vi"
}
