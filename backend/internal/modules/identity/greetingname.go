// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// The name a member's colleagues greet them by.
//
// The first word of the display name is the wrong word often enough to matter:
// "Dr. Sofia Meier" was greeted "Hi Dr.," and "Nguyễn Thị Lan" "Chào Nguyễn".
// So a member may say which word it is. NULL means nobody has said, and every
// greeting falls back to that first word through draftfloor.GreetingName.
//
// Three writers, one rule between them: the member themselves, the invite form,
// and the first federated sign-in, which fills an empty value from the
// provider's given name and never replaces one somebody typed.

import (
	"context"
	"errors"
	"fmt"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/draftfloor"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// greetingNameField is the name this answers to in the audit images and the
// field a 422 points the form at.
const greetingNameField = "greeting_name"

// greetingNameMaxRunes is the contract's bound and the column's CHECK, counted
// in runes for the reason displayNameMaxRunes is.
const greetingNameMaxRunes = 100

// greetingNameOf flattens a typed value to one trimmed line and refuses one
// past the bound. Nil or blank comes back "", which clears. draftfloor.OneLine
// because the value is rendered into a greeting line, and a line break in it
// would open a paragraph the template never wrote.
func greetingNameOf(raw *string) (string, error) {
	if raw == nil {
		return "", nil
	}
	name := draftfloor.OneLine(*raw)
	if !fitsGreetingBound(name) {
		return "", &InvalidGreetingNameError{MaxRunes: greetingNameMaxRunes}
	}
	return name, nil
}

func fitsGreetingBound(name string) bool {
	return utf8.RuneCountInString(name) <= greetingNameMaxRunes
}

// greetingNameColumn is the column's value for a normalized name: NULL for
// none, so "never said" is one state rather than two.
func greetingNameColumn(name string) *string {
	if name == "" {
		return nil
	}
	return &name
}

// SaveMyGreetingName records the name the CALLER's colleagues greet them by.
// Nil or blank clears it.
//
// Self-scoped from the authenticated principal on the terms SaveMyDisplayName
// states: no id on the request, no object grant, and an agent may not set it
// for its grantor.
func (s *Service) SaveMyGreetingName(ctx context.Context, raw *string) (Seat, error) {
	actor, ok := principal.Actor(ctx)
	if !ok || actor.Type != principal.PrincipalHuman || actor.UserID.IsZero() {
		return Seat{}, apperrors.ErrPermissionDenied
	}
	human := ids.From[ids.UserKind](actor.UserID)
	typed, err := greetingNameOf(raw)
	if err != nil {
		return Seat{}, err
	}
	name := greetingNameColumn(typed)
	seat := Seat{UserID: human, GreetingName: name}
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		var before *string
		// FOR UPDATE and LiveMemberSQL for the reasons SaveMyDisplayName gives:
		// a deactivation committing between this read and the UPDATE would
		// leave an audit row for a change that did not happen.
		if err := tx.QueryRow(ctx,
			`SELECT email, display_name, COALESCE(locale, ''), greeting_name
			   FROM app_user WHERE id = $1 AND `+LiveMemberSQL("")+` FOR UPDATE`,
			human).Scan(&seat.Email, &seat.DisplayName, &seat.Locale, &before); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return apperrors.ErrNotFound
			}
			return err
		}
		if sameGreetingName(before, name) {
			return nil
		}
		return writeGreetingName(ctx, tx, human, before, name)
	})
	if err != nil {
		return Seat{}, fmt.Errorf("identity: saving the caller's greeting name: %w", err)
	}
	return seat, nil
}

// fillGreetingNameFromProvider stores the login provider's given name, but
// only while the member's greeting name is empty.
//
// The `greeting_name IS NULL` clause is the whole promise: a value the member
// typed, or one an earlier sign-in filled and they kept, is never replaced by
// what a provider sends. A given name that does not fit the bound is dropped
// rather than cut, since half a name greets nobody.
func fillGreetingNameFromProvider(ctx context.Context, tx pgx.Tx, userID ids.UserID, givenName string) error {
	name := draftfloor.OneLine(givenName)
	if name == "" || !fitsGreetingBound(name) {
		return nil
	}
	tag, err := tx.Exec(ctx,
		`UPDATE app_user SET greeting_name = $2
		  WHERE id = $1 AND greeting_name IS NULL AND `+LiveMemberSQL("")+``, userID, name)
	if err != nil {
		return fmt.Errorf("identity: filling the greeting name from the provider: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return nil
	}
	return auditGreetingName(selfActorCtx(ctx, userID), tx, userID, nil, &name)
}

// writeGreetingName updates the locked row, then audits and publishes the change.
func writeGreetingName(ctx context.Context, tx pgx.Tx, userID ids.UserID, before, after *string) error {
	tag, err := tx.Exec(ctx,
		`UPDATE app_user SET greeting_name = $2
		  WHERE id = $1 AND `+LiveMemberSQL("")+``, userID, after)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return apperrors.ErrNotFound
	}
	return auditGreetingName(ctx, tx, userID, before, after)
}

// auditGreetingName writes the audit and outbox rows for one change. The value
// rides both, as the display name does: it is what colleagues already read in
// every greeting addressed to this member.
func auditGreetingName(ctx context.Context, tx pgx.Tx, userID ids.UserID, before, after *string) error {
	auditID, err := storekit.Audit(ctx, tx, "update", "user", userID.UUID,
		greetingNameImage(before), greetingNameImage(after))
	if err != nil {
		return err
	}
	return storekit.EmitEvent(ctx, tx, auditID, userID.UUID,
		crmcontracts.PublicEventUserGreetingNameChanged{GreetingName: after})
}

// greetingNameImage renders one side of the audit pair; nil marshals to JSON
// null, so "never set" stays apart from any value.
func greetingNameImage(name *string) map[string]*string {
	return map[string]*string{greetingNameField: name}
}

// sameGreetingName reports whether a save would move nothing, so it writes
// nothing and publishes nothing.
func sameGreetingName(a, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// InvalidGreetingNameError names the field and the bound, so the refusal lands
// on the control the reader used.
type InvalidGreetingNameError struct{ MaxRunes int }

func (e *InvalidGreetingNameError) Error() string {
	return fmt.Sprintf("a greeting name is at most %d characters", e.MaxRunes)
}

// FieldFault implements apperrors.FieldFault, which turns this into a 422.
func (e *InvalidGreetingNameError) FieldFault() (field, code, message string) {
	return greetingNameField, codeInvalid, e.Error()
}
