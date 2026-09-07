// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// The name a member's colleagues see them by.
//
// It was written once — by the invite, or by the installation's cold start —
// and nothing could change it afterwards. `display_name` had exactly two
// writers, both INSERTs, so a person who married, was invited as "j.smith" or
// was simply typed wrong carried that name beside every record they touched
// with no way to correct it, and no admin could correct it for them either.
//
// Every surface reads the name from `app_user` at the moment it renders — the
// roster, the share and assignee pickers, the audit trail — so one UPDATE
// reaches all of them and there is no projection to keep in step.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// displayNameField is the name this answers to on every surface: the audit
// images, the outbox payload, and the field a 422 points the form at.
const displayNameField = "display_name"

// displayNameMaxRunes is the contract's own bound, counted in RUNES rather than
// bytes so the limit means the same thing in every script. `maxLength` in
// OpenAPI is a character count, and a 255-byte check would refuse a name of 90
// Vietnamese or Japanese characters that the contract admits.
const displayNameMaxRunes = 255

// SaveMyDisplayName records the name the CALLER's colleagues see them by.
//
// Self-scoped from the authenticated principal, never from a body field: an id
// on the request would be an admin's way to rename a colleague, which is a
// different act with a different audience and is not what this API offers.
// There is no object grant to check for the same reason — holding a seat is the
// whole authority needed to say what your own name is.
func (s *Service) SaveMyDisplayName(ctx context.Context, name string) (Seat, error) {
	// Deliberately NOT actingHuman: that resolves the human an agent is acting
	// UNDER, which is right for attributing work and wrong here. An agent
	// carrying its grantor's authority must not rename its grantor.
	actor, ok := principal.Actor(ctx)
	if !ok || actor.Type != principal.PrincipalHuman || actor.UserID.IsZero() {
		return Seat{}, apperrors.ErrPermissionDenied
	}
	human := actor.UserID

	// Trimmed before it is judged, so a name that is only spaces is refused
	// rather than stored as a blank a colleague cannot address.
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > displayNameMaxRunes {
		return Seat{}, &InvalidDisplayNameError{MaxRunes: displayNameMaxRunes}
	}

	seat := Seat{UserID: ids.From[ids.UserKind](human), DisplayName: name}
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		var before string
		// Both halves of liveness, through the one spelling: a DEACTIVATED
		// account is not archived, so `archived_at IS NULL` alone would let a
		// departed colleague go on renaming themselves in every picker their
		// old records still appear in.
		//
		// FOR UPDATE, the way every other write of this row does it
		// (`users.go`): without the lock a deactivation committing between this
		// read and the UPDATE below leaves the update matching nothing while
		// the audit and event rows are still written — a ledger entry for a
		// change that did not happen, and a before-image that was already
		// stale. Two renames racing would record the same thing.
		if err := tx.QueryRow(ctx,
			`SELECT email, display_name, COALESCE(locale, '')
			   FROM app_user WHERE id = $1 AND `+LiveMemberSQL("")+` FOR UPDATE`,
			human).Scan(&seat.Email, &before, &seat.Locale); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return apperrors.ErrNotFound
			}
			return err
		}
		if before == name {
			// Re-saving the name already stored writes nothing and publishes
			// nothing. A settings form that saves on every render would
			// otherwise fill the ledger with a change nobody made.
			return nil
		}
		// The row is locked above, so this cannot match zero — but the count is
		// checked rather than discarded, because an UPDATE that silently wrote
		// nothing while the audit and event rows went ahead is precisely the
		// failure the lock exists to prevent, and a guard nobody reads is not
		// a guard.
		tag, err := tx.Exec(ctx,
			`UPDATE app_user SET display_name = $2
			  WHERE id = $1 AND `+LiveMemberSQL("")+``, human, name)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return apperrors.ErrNotFound
		}
		// The name is IN the audit payload, where a signature's text is not: it
		// is what colleagues already see on every record this person touches,
		// so recording which name replaced which is the point of the entry
		// rather than a disclosure it makes.
		auditID, err := storekit.Audit(ctx, tx, "update", "user", human,
			displayNameImage(before), displayNameImage(name))
		if err != nil {
			return err
		}
		return storekit.EmitEvent(ctx, tx, auditID, human,
			crmcontracts.PublicEventUserDisplayNameChanged{DisplayName: name})
	})
	if err != nil {
		return Seat{}, fmt.Errorf("identity: saving the caller's display name: %w", err)
	}
	return seat, nil
}

// displayNameImage renders one side of the audit's before/after pair.
func displayNameImage(name string) map[string]string {
	return map[string]string{displayNameField: name}
}

// InvalidDisplayNameError names the field and the bound, so the refusal reaches
// the settings form against the control the reader used rather than as a
// sentence at the bottom of the page.
type InvalidDisplayNameError struct{ MaxRunes int }

func (e *InvalidDisplayNameError) Error() string {
	return fmt.Sprintf("a display name is 1 to %d characters", e.MaxRunes)
}

// FieldFault implements apperrors.FieldFault, which is what turns this into a
// 422 pointing at the control the reader used rather than a 500.
func (e *InvalidDisplayNameError) FieldFault() (field, code, message string) {
	return displayNameField, codeInvalid, fmt.Sprintf(
		"a display name is 1 to %d characters", e.MaxRunes)
}
