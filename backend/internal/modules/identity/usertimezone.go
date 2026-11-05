// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// A user's own time zone.
//
// One zone per user. Booking reads working hours on it, and mail sent to the
// user is timed by it. The app stores the browser's zone on first sign-in as
// a guess. A zone the user picks themselves is marked as chosen, and a later
// browser guess does not replace it.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// TimezoneSource says who picked a zone.
type TimezoneSource string

const (
	// TimezoneFromBrowser is the app's guess from the browser. It is stored
	// only while no zone is stored.
	TimezoneFromBrowser TimezoneSource = "browser"
	// TimezoneChosen is the user's own pick. It is always stored.
	TimezoneChosen TimezoneSource = "chosen"
	// TimezoneFromAgent is an agent doing what its user asked. It is always stored
	// and holds against a browser's guess. The row names the agent and does not read
	// as chosen, because the user did not pick it themselves.
	TimezoneFromAgent TimezoneSource = "agent"

	fieldTimezoneChosen = "timezone_chosen"
	fieldTimezoneSetBy  = "timezone_set_by"
	fieldTimezoneSource = "source"
)

// SaveMyTimezone stores the CALLER's own time zone.
//
// Self-scoped from the authenticated principal. An agent sets the zone its user
// asked for as its own source, and never claims the user chose it.
func (s *Service) SaveMyTimezone(ctx context.Context, zone string, source TimezoneSource) (Seat, error) {
	human, err := ownSettingsSeat(ctx)
	if err != nil {
		return Seat{}, err
	}
	if source != TimezoneFromBrowser && source != TimezoneChosen && source != TimezoneFromAgent {
		return Seat{}, &WorkingHoursError{Field: fieldTimezoneSource, Message: "a source is browser, chosen or agent"}
	}
	_, isAgent := callingAgent(ctx)
	if isAgent && source != TimezoneFromAgent {
		return Seat{}, agentLimit("an agent does not choose a time zone for its user or pass as the browser; send source agent once they have asked for it")
	}
	if !isAgent && source == TimezoneFromAgent {
		return Seat{}, &WorkingHoursError{Field: fieldTimezoneSource, Message: "only an agent sends the source agent"}
	}
	if values.ZoneName(zone) != nil {
		return Seat{}, &WorkingHoursError{
			Field:   fieldWorkTimezone,
			Message: "a timezone is an IANA name, such as Europe/Berlin",
		}
	}
	var seat Seat
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		var werr error
		seat, werr = writeTimezoneTx(ctx, tx, ids.From[ids.UserKind](human), zone, source)
		return werr
	})
	if err != nil {
		return Seat{}, fmt.Errorf("identity: saving the caller's time zone: %w", err)
	}
	return seat, nil
}

// writeTimezoneTx is the one writer of a user's zone after their row exists.
// A browser guess over a stored zone, or the value already stored, writes
// nothing.
//
// Held by: TestOneWriterSetsAUsersTimezone (backend/gates/usertimezonewriters_test.go)
func writeTimezoneTx(ctx context.Context, tx pgx.Tx, user ids.UserID, zone string, source TimezoneSource) (Seat, error) {
	seat := Seat{UserID: user}
	var locale, before, setByBefore *string
	var chosenBefore bool
	if err := tx.QueryRow(ctx, `
		SELECT email, display_name, locale, greeting_name, timezone, timezone_chosen, timezone_set_by
		  FROM app_user WHERE id = $1 AND `+LiveMemberSQL("")+` FOR UPDATE`,
		user).Scan(&seat.Email, &seat.DisplayName, &locale, &seat.GreetingName, &before, &chosenBefore, &setByBefore); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Seat{}, apperrors.ErrNotFound
		}
		return Seat{}, err
	}
	if locale != nil {
		seat.Locale = *locale
	}
	seat.Timezone, seat.TimezoneChosen, seat.TimezoneSetBy = optionalValue(before), chosenBefore, setByBefore
	stored := before != nil && *before != ""
	if source == TimezoneFromBrowser && stored {
		return seat, nil
	}
	chosen := source == TimezoneChosen || (source == TimezoneFromBrowser && chosenBefore)
	setBy := timezoneSetBy(ctx)
	if stored && *before == zone && repeatsTheStoredZone(source, chosenBefore, setByBefore, setBy) {
		return seat, nil
	}
	if _, err := tx.Exec(ctx,
		`UPDATE app_user SET timezone = $2, timezone_chosen = $3, timezone_set_by = $4 WHERE id = $1`,
		user, zone, chosen, setBy); err != nil {
		return Seat{}, fmt.Errorf("identity: writing the time zone: %w", err)
	}
	beforeImage, afterImage := timezoneImage(before, chosenBefore), timezoneImage(&zone, chosen)
	beforeImage[fieldTimezoneSetBy], afterImage[fieldTimezoneSetBy] = setByBefore, setBy
	auditID, err := storekit.Audit(ctx, tx, "update", "user", user.UUID, beforeImage, afterImage)
	if err != nil {
		return Seat{}, err
	}
	if err := storekit.EmitEvent(ctx, tx, auditID, user.UUID,
		crmcontracts.PublicEventUserTimezoneChanged{Timezone: zone, Chosen: chosen}); err != nil {
		return Seat{}, err
	}
	seat.Timezone, seat.TimezoneChosen, seat.TimezoneSetBy = zone, chosen, setBy
	return seat, nil
}

// repeatsTheStoredZone reports a save of the zone already stored that says nothing
// new. An agent repeating a zone the user chose leaves it theirs. An agent repeating
// a browser's guess becomes its setter, because a guess is nobody's choice.
func repeatsTheStoredZone(source TimezoneSource, chosenBefore bool, setByBefore, setBy *string) bool {
	if source == TimezoneFromAgent {
		return chosenBefore || (setByBefore != nil && setBy != nil && *setBy == *setByBefore)
	}
	return chosenBefore && setByBefore == nil
}

// readSeatZone fills a self-service answer with the zone the seat holds, and who
// set it, for writers whose own statement is about something else.
func readSeatZone(ctx context.Context, tx pgx.Tx, seat *Seat) error {
	var zone *string
	if err := tx.QueryRow(ctx, `SELECT timezone, timezone_chosen, timezone_set_by FROM app_user WHERE id = $1`,
		seat.UserID).Scan(&zone, &seat.TimezoneChosen, &seat.TimezoneSetBy); err != nil {
		return fmt.Errorf("identity: reading the seat's time zone: %w", err)
	}
	seat.Timezone = optionalValue(zone)
	return nil
}

// timezoneImage renders one side of the audit's before/after pair. A user
// with no stored zone carries JSON null, not "UTC".
func timezoneImage(zone *string, chosen bool) map[string]any {
	return map[string]any{fieldWorkTimezone: zone, fieldTimezoneChosen: chosen}
}

func optionalValue(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// timezoneSetBy names the agent that set a zone, or nobody for a user's choice
// or a guess a browser made.
func timezoneSetBy(ctx context.Context) *string {
	if actor, isAgent := callingAgent(ctx); isAgent {
		return &actor.ID
	}
	return nil
}
