// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/database/textbounds"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Nobody can shorten a card's text before it lands. So a value past the column
// limit is cut, and the card still applies.
func TestACardValuePastTheLimitLandsCut(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	incumbent, err := e.store.CreateContact(ctx, CreateContactInput{
		FullName: "Dana Known", Source: "manual",
		Emails: []ContactEmailInput{{Email: "dana@known.example", EmailType: emailTypeWork, IsPrimary: true}},
	})
	if err != nil {
		t.Fatalf("seeding the incumbent: %v", err)
	}
	limit := textbounds.LimitOf("public.contact_profile_field.value")
	results := importCards(ctx, t, e, "BEGIN:VCARD\nFN:Dana Known\nURL:https://example.com/"+
		strings.Repeat("a", limit)+"\nEMAIL;TYPE=WORK:dana@known.example\nEND:VCARD\n")
	if len(results) != 1 || results[0].Outcome != VCardUpdated {
		t.Fatalf("outcomes = %+v, want one updated", results)
	}
	if got := profileFieldLength(ctx, t, e, ids.From[ids.ContactKind](ids.UUID(incumbent.Id)), fieldWebsite); got != limit {
		t.Fatalf("stored a website of %d characters, want the limit of %d", got, limit)
	}
}

// A researcher can shorten their own claim, so one past the limit is refused
// with the column and its limit rather than cut.
func TestAResearchClaimPastTheLimitIsRefused(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	contactID, _ := e.seedEmployedContact(ctx, t,
		"Mira Halvorsen", "mira@voltaq.test", "Voltaq Systems GmbH", "voltaq.test")
	limit := textbounds.LimitOf("public.contact_profile_field.value")

	_, err := e.store.SaveResearchClaims(ctx, contactID, []ResearchClaimInput{{
		Field: "role", Value: strings.Repeat("ế", limit+1),
		Quote: "Mira leads the platform team.", SourceURL: "https://voltaq.test/team",
	}})
	column, refusedAt, ok := storekit.TextBoundViolation(err)
	if !ok || column != "value" || refusedAt != limit {
		t.Fatalf("SaveResearchClaims answered %v, want the value column refused at %d", err, limit)
	}
}

func profileFieldLength(ctx context.Context, t *testing.T, e *dedupeEnv, contactID ids.ContactID, field string) int {
	t.Helper()
	var n int
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT char_length(value) FROM contact_profile_field WHERE contact_id = $1 AND field = $2`,
			contactID, field).Scan(&n)
	}); err != nil {
		t.Fatalf("reading the %s row: %v", field, err)
	}
	return n
}
