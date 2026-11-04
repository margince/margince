// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package identity

// A person's own time zone. A chosen zone is stored with its flag and one
// audit row. A browser guess never replaces a stored zone. An unchanged value
// writes nothing. The scheduler reads the new zone.

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func timezoneRow(t *testing.T, svc *Service, user ids.UUID) (zone *string, chosen bool, audits int) {
	t.Helper()
	if err := svc.db.Tx(context.Background(), func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `
			SELECT timezone, timezone_chosen,
			       (SELECT count(*) FROM audit_log WHERE entity_type = 'user' AND entity_id = $1
			           AND after ? 'timezone_chosen')
			  FROM app_user WHERE id = $1`, user).Scan(&zone, &chosen, &audits)
	}); err != nil {
		t.Fatalf("reading the zone: %v", err)
	}
	return zone, chosen, audits
}

func TestAChosenTimezoneIsStoredWithItsFlagAndOneAuditRow(t *testing.T) {
	svc, ctx, user := seatChoosingALanguage(t)
	seat, err := svc.SaveMyTimezone(ctx, "America/New_York", TimezoneChosen)
	if err != nil {
		t.Fatal(err)
	}
	if seat.Timezone != "America/New_York" || !seat.TimezoneChosen {
		t.Errorf("the answer is %q chosen=%v, want the chosen zone", seat.Timezone, seat.TimezoneChosen)
	}
	zone, chosen, audits := timezoneRow(t, svc, user)
	if zone == nil || *zone != "America/New_York" || !chosen || audits != 1 {
		t.Errorf("row = %v chosen=%v audits=%d, want the zone, chosen, one audit row", zone, chosen, audits)
	}

	// The same choice again writes nothing.
	if _, err := svc.SaveMyTimezone(ctx, "America/New_York", TimezoneChosen); err != nil {
		t.Fatal(err)
	}
	if _, _, audits := timezoneRow(t, svc, user); audits != 1 {
		t.Errorf("an unchanged zone wrote %d audit rows, want still 1", audits)
	}

	// A browser guess does not replace it.
	seat, err = svc.SaveMyTimezone(ctx, "Asia/Tokyo", TimezoneFromBrowser)
	if err != nil {
		t.Fatal(err)
	}
	if seat.Timezone != "America/New_York" {
		t.Errorf("the answer after a browser guess carries %q, want the stored zone", seat.Timezone)
	}
	if zone, _, audits := timezoneRow(t, svc, user); *zone != "America/New_York" || audits != 1 {
		t.Errorf("a browser guess replaced the zone: %q, %d audit rows", *zone, audits)
	}

	// The scheduler reads the new zone.
	var hours WorkingHours
	if err := svc.db.Tx(context.Background(), func(tx pgx.Tx) error {
		var err error
		hours, _, err = WorkingHoursOf(context.Background(), tx, ids.From[ids.UserKind](user))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if hours.Timezone != "America/New_York" {
		t.Errorf("WorkingHoursOf reads %q, want the chosen zone", hours.Timezone)
	}
}

func TestABrowserGuessFillsOnlyAnEmptyZone(t *testing.T) {
	svc, ctx, user := seatChoosingALanguage(t)
	if err := svc.db.Tx(context.Background(), func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `UPDATE app_user SET timezone = NULL WHERE id = $1`, user)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	seat, err := svc.SaveMyTimezone(ctx, "Europe/Lisbon", TimezoneFromBrowser)
	if err != nil {
		t.Fatal(err)
	}
	if seat.Timezone != "Europe/Lisbon" || seat.TimezoneChosen {
		t.Errorf("the answer is %q chosen=%v, want the guess, not chosen", seat.Timezone, seat.TimezoneChosen)
	}
	if zone, chosen, audits := timezoneRow(t, svc, user); zone == nil || *zone != "Europe/Lisbon" || chosen || audits != 1 {
		t.Errorf("row = %v chosen=%v audits=%d, want the guess stored once, not chosen", zone, chosen, audits)
	}
	// The person's own pick still replaces the guess.
	if _, err := svc.SaveMyTimezone(ctx, "Europe/Berlin", TimezoneChosen); err != nil {
		t.Fatal(err)
	}
	if zone, chosen, _ := timezoneRow(t, svc, user); *zone != "Europe/Berlin" || !chosen {
		t.Errorf("row = %q chosen=%v, want the pick", *zone, chosen)
	}
}

func TestTheTimezoneWriterRefusesWhatItMayNotStore(t *testing.T) {
	svc, ctx, user := seatChoosingALanguage(t)
	var fault apperrors.FieldFault
	if _, err := svc.SaveMyTimezone(ctx, "Etc/Nowhere", TimezoneChosen); !errors.As(err, &fault) {
		t.Errorf("an unknown zone: err = %v, want a field fault", err)
	} else if field, _, _ := fault.FieldFault(); field != "timezone" {
		t.Errorf("the refusal names %q, want timezone", field)
	}
	agent := principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalAgent, ID: "agent:test", UserID: user,
	})
	if _, err := svc.SaveMyTimezone(agent, "Europe/Berlin", TimezoneChosen); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("an agent: err = %v, want permission denied", err)
	}
	if err := svc.db.Tx(context.Background(), func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `UPDATE app_user SET archived_at = now() WHERE id = $1`, user)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SaveMyTimezone(ctx, "Europe/Berlin", TimezoneChosen); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("an archived seat: err = %v, want not found", err)
	}
}

// A stored zone is announced like any other write: one outbox row per change,
// carrying the zone and whether it was chosen. An unchanged save announces
// nothing.
func TestAStoredTimezoneIsAnnouncedOnce(t *testing.T) {
	svc, ctx, user := seatChoosingALanguage(t)
	events := func() (n int, zone string, chosen bool) {
		t.Helper()
		if err := svc.db.Tx(context.Background(), func(tx pgx.Tx) error {
			return tx.QueryRow(context.Background(), `
				SELECT count(*), coalesce(max(envelope -> 'payload' ->> 'timezone'), ''),
				       coalesce(bool_or((envelope -> 'payload' ->> 'chosen')::boolean), false)
				  FROM event_outbox
				 WHERE envelope ->> 'type' = 'user_timezone.changed'
				   AND envelope -> 'entity' ->> 'id' = $1`, user.String()).Scan(&n, &zone, &chosen)
		}); err != nil {
			t.Fatalf("reading the outbox: %v", err)
		}
		return n, zone, chosen
	}
	if _, err := svc.SaveMyTimezone(ctx, "America/New_York", TimezoneChosen); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SaveMyTimezone(ctx, "America/New_York", TimezoneChosen); err != nil {
		t.Fatal(err)
	}
	if n, zone, chosen := events(); n != 1 || zone != "America/New_York" || !chosen {
		t.Errorf("outbox holds %d user_timezone.changed rows (%q, chosen=%v), want one for the chosen zone", n, zone, chosen)
	}
}
