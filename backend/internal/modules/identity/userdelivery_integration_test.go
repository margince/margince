// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package identity

// A member choosing what the product may send them.
//
// The properties worth a real database: never-chosen is a different answer from
// chosen-none, an omitted field is left alone rather than cleared, a save that
// moves nothing writes nothing, and the change is auditable.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The distinction the nullable columns exist for.
//
// A member who never chose follows the installation's default and moves if that
// default moves; one who chose "none" has made a decision that stays. A shape
// that collapsed them would silently freeze today's default for everybody who
// ever opened the settings page.
func TestNeverChosenIsNotTheSameAsChoosingNone(t *testing.T) {
	svc, ctx, _ := seatChoosingALanguage(t)

	fresh, err := svc.MyDelivery(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if fresh.Weekly != nil {
		t.Errorf("a seat that never chose reads as %q, wanted absent", *fresh.Weekly)
	}

	none := DeliveryNone
	after, err := svc.SaveMyDelivery(ctx, Delivery{Weekly: &none})
	if err != nil {
		t.Fatal(err)
	}
	if after.Weekly == nil || *after.Weekly != DeliveryNone {
		t.Errorf("after choosing none the setting reads as %v, wanted none", after.Weekly)
	}
}

// An omitted field is left alone, never cleared.
//
// A client that renders three controls and sends two must not reset the third —
// the settings page is exactly where that mistake costs somebody their mail
// without anybody choosing it.
func TestAnOmittedFieldIsLeftAlone(t *testing.T) {
	svc, ctx, _ := seatChoosingALanguage(t)

	email, none := DeliveryEmail, DeliveryNone
	if _, err := svc.SaveMyDelivery(ctx, Delivery{MorningBrief: &email, Weekly: &none}); err != nil {
		t.Fatal(err)
	}

	// A second save that mentions only the weekly.
	quiet := true
	after, err := svc.SaveMyDelivery(ctx, Delivery{QuietDay: &quiet})
	if err != nil {
		t.Fatal(err)
	}

	if after.MorningBrief == nil || *after.MorningBrief != DeliveryEmail {
		t.Errorf("the untouched morning setting came back as %v, wanted the email it held",
			after.MorningBrief)
	}
	if after.Weekly == nil || *after.Weekly != DeliveryNone {
		t.Errorf("the untouched weekly setting came back as %v, wanted the none it held",
			after.Weekly)
	}
}

// A save that moves nothing writes nothing.
//
// A settings page saves on every render, and a ledger full of changes nobody
// made is a ledger nobody can read.
func TestASaveThatChangesNothingWritesNothing(t *testing.T) {
	svc, ctx, userID := seatChoosingALanguage(t)

	email := DeliveryEmail
	if _, err := svc.SaveMyDelivery(ctx, Delivery{MorningBrief: &email}); err != nil {
		t.Fatal(err)
	}
	before := deliveryAudits(t, svc, userID)
	if before != 1 {
		t.Fatalf("the first save wrote %d audit rows, wanted 1 — without this the "+
			"no-op check below proves nothing", before)
	}

	if _, err := svc.SaveMyDelivery(ctx, Delivery{MorningBrief: &email}); err != nil {
		t.Fatal(err)
	}

	if after := deliveryAudits(t, svc, userID); after != before {
		t.Errorf("re-choosing the same setting wrote %d further audit rows, wanted none",
			after-before)
	}
}

// A choice outside the vocabulary is refused rather than stored.
func TestAnUnknownDeliveryChoiceIsRefused(t *testing.T) {
	svc, ctx, _ := seatChoosingALanguage(t)

	pigeon := "carrier_pigeon"
	if _, err := svc.SaveMyDelivery(ctx, Delivery{Weekly: &pigeon}); err == nil {
		t.Error("an unknown delivery choice was accepted")
	}

	hour := 47
	if _, err := svc.SaveMyDelivery(ctx, Delivery{HourLocal: &hour}); err == nil {
		t.Error("an hour outside a day was accepted")
	}
}

// deliveryAudits counts the audit rows a delivery write leaves behind.
func deliveryAudits(t *testing.T, svc *Service, userID ids.UUID) int {
	t.Helper()
	ctx := context.Background()
	var n int
	if err := svc.db.Tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT count(*) FROM audit_log
			  WHERE entity_type = 'user' AND entity_id = $1
			    AND after ? 'morning_brief_delivery'`, userID).Scan(&n)
	}); err != nil {
		t.Fatal(err)
	}
	return n
}
