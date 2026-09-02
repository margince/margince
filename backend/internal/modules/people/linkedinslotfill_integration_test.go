// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package people

// A landed linkedin evidence row must also fill the empty person_social slot,
// because that slot is what the rest of the product reads: the person rail's
// LinkedIn row, the provider identifier resolver that decides whether an
// enrichment vendor can look the contact up, the SAR export. Before the fill
// existed, a contact whose LinkedIn arrived through enrichment displayed the
// URL in its research section while every other reader answered "none" — and
// the provider lookup refused the contact as having nothing to match on.
//
// Over real Postgres because the fill's restraint is a SQL conflict clause
// (an existing handle wins) and its liveness guard is the subject lock, and
// neither exists anywhere a unit test could see.

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// storedLinkedinHandle reads the slot every non-evidence reader consults.
// Empty string means the slot is empty — the table allows one row per
// platform, so there is no second row to miss.
func storedLinkedinHandle(ctx context.Context, t *testing.T, e *dedupeEnv, personID ids.PersonID) string {
	t.Helper()
	var handle string
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx,
			`SELECT handle FROM person_social WHERE person_id = $1 AND platform = 'linkedin'`,
			personID).Scan(&handle)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return err
	}); err != nil {
		t.Fatalf("read back the linkedin slot: %v", err)
	}
	return handle
}

func TestALandedLinkedinFillReachesTheSocialSlot(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	personID, _ := e.seedEmployedPerson(ctx, t,
		"Nora Vik", "nora@slotfill.test", "Vik AS", "slotfill.test")

	const url = "https://www.linkedin.com/in/nora-vik/"
	applied, err := e.store.ApplyDiscoveredFields(ctx, personID, []DiscoveredField{{
		Field:           "linkedin",
		Value:           url,
		EvidenceSnippet: "Nora Vik — Vik AS",
		SourceRef:       "search:slotfill",
	}})
	if err != nil {
		t.Fatalf("ApplyDiscoveredFields: %v", err)
	}
	if len(applied) != 1 {
		t.Fatalf("applied = %v, want the linkedin fill to land", applied)
	}
	if got := storedLinkedinHandle(ctx, t, e, personID); got != url {
		t.Errorf("linkedin slot = %q, want %q: the evidence row landed without reaching the record", got, url)
	}
}

func TestAFillNeverReplacesAHandleAlreadyOnTheRecord(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	const typed = "https://www.linkedin.com/in/the-one-somebody-typed"
	person, err := e.store.CreatePerson(ctx, CreatePersonInput{
		FullName: "Ida Holm",
		Source:   "manual",
		Social:   map[string]any{"linkedin": typed},
	})
	if err != nil {
		t.Fatalf("seed person: %v", err)
	}
	personID := ids.From[ids.PersonKind](ids.UUID(person.Id))

	applied, err := e.store.ApplyDiscoveredFields(ctx, personID, []DiscoveredField{{
		Field:           "linkedin",
		Value:           "https://www.linkedin.com/in/somebody-a-search-found",
		EvidenceSnippet: "Ida Holm — profile",
		SourceRef:       "search:slotfill",
	}})
	if err != nil {
		t.Fatalf("ApplyDiscoveredFields: %v", err)
	}
	// The evidence row may land — the sidecar was unanswered — but the slot
	// carries somebody's statement and the fill has no grounds to replace it.
	if len(applied) != 1 {
		t.Fatalf("applied = %v, want the evidence row to land", applied)
	}
	if got := storedLinkedinHandle(ctx, t, e, personID); got != typed {
		t.Errorf("linkedin slot = %q, want the typed handle %q kept", got, typed)
	}
}

func TestAValueThatIsNotAProfileLinkStaysEvidenceOnly(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	personID, _ := e.seedEmployedPerson(ctx, t,
		"Sofie Dahl", "sofie@slotfill.test", "Dahl AS", "slotfill.test")

	applied, err := e.store.ApplyDiscoveredFields(ctx, personID, []DiscoveredField{{
		Field:           "linkedin",
		Value:           "https://dahl.example/team/sofie",
		EvidenceSnippet: "Sofie Dahl — Dahl AS",
		SourceRef:       "search:slotfill",
	}})
	if err != nil {
		t.Fatalf("ApplyDiscoveredFields: %v", err)
	}
	if len(applied) != 1 {
		t.Fatalf("applied = %v, want the evidence row to land", applied)
	}
	if got := storedLinkedinHandle(ctx, t, e, personID); got != "" {
		t.Errorf("linkedin slot = %q, want empty: a URL off LinkedIn's host is not a profile link", got)
	}
}
