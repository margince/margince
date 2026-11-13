// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// An anonymous booker who knows the address of a colleague's private contact
// changes nothing on it. The host cannot see that contact, so the booking is
// refused, and no owner or employment is written before the refusal.
func TestABookingOnAPrivateContactTheHostCannotSeeIsRefusedAndChangesNothing(t *testing.T) {
	e := setupBookingApp(t)
	e.BootstrapWorkspace(t)
	apptest.EnableGoogleBookingPage(t, e)
	const address = "secret@private-works.example"
	createdID(t, e, "/v1/companies", AnyMap{"display_name": "Private Works", "domains": []AnyMap{{"domain": "private-works.example", "is_primary": true}}, "source": "manual"})
	colleague := SeedIDRow(t, e.Owner, `INSERT INTO app_user (id, email, display_name) VALUES ($1, 'colleague@private-works.test', 'Colleague')`)
	contact := SeedIDRow(t, e.Owner, `INSERT INTO contact (id, full_name, owner_id, visibility, source, captured_by) VALUES ($1, 'Kept Private', $2, 'owner', 'capture', 'human:x')`, colleague)
	if _, err := e.Owner.Exec(context.Background(), `INSERT INTO contact_email (contact_id, email, is_primary, source, captured_by) VALUES ($1, $2, true, 'capture', 'human:x')`, contact, address); err != nil {
		t.Fatal(err)
	}

	monday := nextMonday()
	body := AnyMap{"start": monday.Add(time.Hour), "end": monday.Add(90 * time.Minute), "booker": AnyMap{"name": "Kept Private", "email": address}, "consent": AnyMap{"policy_version": "pp-2026-01", "wording": "Contact me about this meeting."}}
	if status := publicCall(t, e, "POST", "/v1/public/booking/"+bookingSlug(t, e), body, nil, nil); status < 400 || status >= 500 {
		t.Fatalf("booking a private contact the host cannot see → %d, want a refusal", status)
	}

	var owner ids.UUID
	var employments int
	if err := e.Owner.QueryRow(context.Background(), `
		SELECT owner_id, (SELECT count(*) FROM relationship WHERE kind = 'employment' AND contact_id = $1)
		  FROM contact WHERE id = $1`, contact).Scan(&owner, &employments); err != nil {
		t.Fatal(err)
	}
	if owner != colleague || employments != 0 {
		t.Errorf("the refused booking left owner %s and %d employments, want the colleague %s and none", owner, employments, colleague)
	}
	var meetings int
	if err := e.Owner.QueryRow(context.Background(), `SELECT count(*) FROM activity WHERE kind = 'meeting'`).Scan(&meetings); err != nil {
		t.Fatal(err)
	}
	if meetings != 0 {
		t.Errorf("the refused booking filed %d meetings, want none", meetings)
	}
}
