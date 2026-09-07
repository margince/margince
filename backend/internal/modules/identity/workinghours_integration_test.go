// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package identity

// A person choosing when they are bookable. The properties worth a real
// database: the fallback a person who has chosen nothing is answered with, that
// a choice is read back as their own, and that the refusals reach the control
// the reader used.

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestAPersonWhoHasChosenNothingIsBookableOnTheInstallationsClock(t *testing.T) {
	svc, ctx, _ := seatChoosingALanguage(t)

	hours, chosen, err := svc.MyWorkingHours(ctx)
	if err != nil {
		t.Fatalf("reading the fallback: %v", err)
	}
	if chosen {
		t.Error("a seat that has chosen nothing reports a choice, so a screen would show the " +
			"fallback as a decision somebody made")
	}
	// Not UTC. The bootstrap put this installation in Europe/Berlin, and a
	// person who has never chosen is read on the installation's clock — which
	// is what makes the UTC defect disappear before anybody touches a setting.
	if hours.Timezone != "Europe/Berlin" {
		t.Errorf("the fallback zone is %q, want the installation's", hours.Timezone)
	}
	if hours.StartMinute != 9*60 || hours.EndMinute != 17*60 || len(hours.Days) != 5 {
		t.Errorf("the fallback is %+v, want 09:00-17:00 Monday to Friday", hours)
	}
}

func TestChoosingWorkingHoursIsReadBackAsTheirOwn(t *testing.T) {
	svc, ctx, userID := seatChoosingALanguage(t)

	// A four-day week starting early, on their own clock — one of the two
	// shapes an installation-wide pair of numbers cannot express.
	saved, err := svc.SaveMyWorkingHours(ctx, WorkingHours{
		StartMinute: 8 * 60, EndMinute: 13 * 60,
		Days: []int{4, 1, 2, 3}, Timezone: "Asia/Ho_Chi_Minh",
	})
	if err != nil {
		t.Fatalf("saving working hours: %v", err)
	}
	// Sorted and deduplicated on the way in: the days are a set, and a reader
	// of the row should not have to sort it to see the week.
	if got := saved.Days; len(got) != 4 || got[0] != 1 || got[3] != 4 {
		t.Errorf("the days were stored as %v, want them ascending", got)
	}

	read, chosen, err := svc.MyWorkingHours(ctx)
	if err != nil {
		t.Fatalf("reading them back: %v", err)
	}
	if !chosen {
		t.Error("a person who chose their hours is reported as having chosen nothing")
	}
	if read.StartMinute != 8*60 || read.EndMinute != 13*60 || read.Timezone != "Asia/Ho_Chi_Minh" {
		t.Errorf("read back %+v, want what was saved", read)
	}

	// And the scheduler's own reader answers the same thing for that host,
	// because it is the one the availability seam calls.
	var direct WorkingHours
	if err := svc.db.Tx(context.Background(), func(tx pgx.Tx) error {
		var err error
		direct, _, err = WorkingHoursOf(context.Background(), tx, ids.From[ids.UserKind](userID))
		return err
	}); err != nil {
		t.Fatalf("reading through the scheduler's own door: %v", err)
	}
	if direct.StartMinute != read.StartMinute || direct.Timezone != read.Timezone {
		t.Errorf("the scheduler reads %+v where the settings page reads %+v", direct, read)
	}
}

func TestTheAuditRowCarriesBothWindows(t *testing.T) {
	svc, ctx, userID := seatChoosingALanguage(t)
	if _, err := svc.SaveMyWorkingHours(ctx, WorkingHours{
		StartMinute: 8 * 60, EndMinute: 13 * 60, Days: []int{1, 2, 3, 4}, Timezone: "Europe/Berlin",
	}); err != nil {
		t.Fatalf("saving: %v", err)
	}
	var before, after string
	if err := svc.db.Tx(context.Background(), func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `
			SELECT before ->> 'start_time', after ->> 'start_time'
			  FROM audit_log
			 WHERE entity_type = 'user' AND entity_id = $1 AND after ? 'start_time'
			 ORDER BY id DESC LIMIT 1`, userID).Scan(&before, &after)
	}); err != nil {
		t.Fatalf("reading the audit row: %v", err)
	}
	// The question the row exists to answer is "why did my bookings halve",
	// which needs the window that was replaced and not only the new one.
	if before != "09:00" || after != "08:00" {
		t.Errorf("the images say %q → %q, want the fallback replaced by the choice", before, after)
	}
}

func TestARefusalNamesTheControlTheReaderUsed(t *testing.T) {
	svc, ctx, _ := seatChoosingALanguage(t)
	for _, refused := range []struct {
		name  string
		in    WorkingHours
		field string
	}{
		{"a day that ends before it starts", WorkingHours{
			StartMinute: 17 * 60, EndMinute: 9 * 60, Days: []int{1}, Timezone: "Europe/Berlin",
		}, "end_time"},
		{"a week with no days in it", WorkingHours{
			StartMinute: 9 * 60, EndMinute: 17 * 60, Days: nil, Timezone: "Europe/Berlin",
		}, "days"},
		{"a zone the database does not carry", WorkingHours{
			StartMinute: 9 * 60, EndMinute: 17 * 60, Days: []int{1}, Timezone: "Mars/Olympus",
		}, "timezone"},
	} {
		t.Run(refused.name, func(t *testing.T) {
			_, err := svc.SaveMyWorkingHours(ctx, refused.in)
			var fault apperrors.FieldFault
			if !errors.As(err, &fault) {
				t.Fatalf("err = %v, want a refusal naming a field", err)
			}
			field, _, _ := fault.FieldFault()
			if field != refused.field {
				t.Errorf("the refusal names %q, want %q — a form puts the message against the "+
					"control the reader used", field, refused.field)
			}
		})
	}
}
