// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// A landed linkedin evidence row must also fill the empty contact_social slot,
// because that slot is what the rest of the product reads: the contact rail's
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
func storedLinkedinHandle(ctx context.Context, t *testing.T, e *dedupeEnv, contactID ids.ContactID) string {
	t.Helper()
	var handle string
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx,
			`SELECT handle FROM contact_social WHERE contact_id = $1 AND platform = 'linkedin'`,
			contactID).Scan(&handle)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return err
	}); err != nil {
		t.Fatalf("read back the linkedin slot: %v", err)
	}
	return handle
}

// fillLinkedinFromSite has the employer's site publish this contact's LinkedIn
// address, through the writer that fills it in production. The match itself is
// not under test here, so a site that fails to match the contact fails the test.
func fillLinkedinFromSite(ctx context.Context, t *testing.T, e *dedupeEnv, companyID ids.CompanyID, name, url string) {
	t.Helper()
	matched, err := e.store.ApplySiteContactFields(ctx, companyID, SiteContactFields{
		Name:            name,
		LinkedinURL:     url,
		EvidenceSnippet: name + " — team page",
		SourceURL:       "https://slotfill.test/team",
	})
	if err != nil {
		t.Fatalf("ApplySiteContactFields: %v", err)
	}
	if !matched {
		t.Fatalf("the site did not match %s, so this test proves nothing about the slot", name)
	}
}

func TestALandedLinkedinFillReachesTheSocialSlot(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	contactID, companyID := e.seedEmployedContact(ctx, t,
		"Nora Vik", "nora@slotfill.test", "Vik AS", "slotfill.test")

	// The trailing slash is deliberate: the slot stores the normalized
	// spelling (the dedupe key's), not the verbatim evidence value.
	const observed = "https://www.linkedin.com/in/nora-vik/"
	const normalized = "https://www.linkedin.com/in/nora-vik"
	fillLinkedinFromSite(ctx, t, e, companyID, "Nora Vik", observed)

	if got := readStoredClaim(ctx, t, e, contactID, "linkedin").value; got != observed {
		t.Fatalf("linkedin evidence = %q, want the fill to land %q", got, observed)
	}
	if got := storedLinkedinHandle(ctx, t, e, contactID); got != normalized {
		t.Errorf("linkedin slot = %q, want %q: the evidence row landed without reaching the record", got, normalized)
	}
}

func TestAFillNeverReplacesAHandleAlreadyOnTheRecord(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	contactID, companyID := e.seedEmployedContact(ctx, t,
		"Ida Holm", "ida@slotfill.test", "Holm AS", "slotfill.test")
	const typed = "https://www.linkedin.com/in/the-one-somebody-typed"
	if _, err := e.store.UpdateContact(ctx, contactID, UpdateContactInput{
		Social: map[string]any{"linkedin": typed},
		Source: "manual",
	}); err != nil {
		t.Fatalf("type the handle: %v", err)
	}

	const published = "https://www.linkedin.com/in/somebody-a-site-named"
	fillLinkedinFromSite(ctx, t, e, companyID, "Ida Holm", published)

	// The evidence row lands — the sidecar was unanswered — but the slot
	// carries somebody's statement and the fill has no grounds to replace it.
	if got := readStoredClaim(ctx, t, e, contactID, "linkedin").value; got != published {
		t.Fatalf("linkedin evidence = %q, want the evidence row to land %q", got, published)
	}
	if got := storedLinkedinHandle(ctx, t, e, contactID); got != typed {
		t.Errorf("linkedin slot = %q, want the typed handle %q kept", got, typed)
	}
}

func TestAValueThatIsNotAProfileLinkStaysEvidenceOnly(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	contactID, companyID := e.seedEmployedContact(ctx, t,
		"Sofie Dahl", "sofie@slotfill.test", "Dahl AS", "slotfill.test")

	const offHost = "https://dahl.example/team/sofie"
	fillLinkedinFromSite(ctx, t, e, companyID, "Sofie Dahl", offHost)

	if got := readStoredClaim(ctx, t, e, contactID, "linkedin").value; got != offHost {
		t.Fatalf("linkedin evidence = %q, want the evidence row to land %q", got, offHost)
	}
	if got := storedLinkedinHandle(ctx, t, e, contactID); got != "" {
		t.Errorf("linkedin slot = %q, want empty: a URL off LinkedIn's host is not a profile link", got)
	}
}

// A slot that was filled says so in a row of its own.
//
// The caller's audit row attests to the EVIDENCE write and reads the same
// whether the slot was empty or already held somebody's statement, so a reader
// asking when this contact gained its profile link had nothing to read.
func TestAFilledSlotIsNamedInAnAuditRowOfItsOwn(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	contactID, companyID := e.seedEmployedContact(ctx, t,
		"Tuva Lien", "tuva@slotfill.test", "Lien AS", "slotfill.test")

	fillLinkedinFromSite(ctx, t, e, companyID, "Tuva Lien", "https://www.linkedin.com/in/tuva-lien")
	if got := countAuditRowsHolding(ctx, t, e.store, "contact", contactID.UUID, "social"); got != 1 {
		t.Errorf("audit rows naming the social write = %d, want 1 — a fill nothing records is a "+
			"change to what the rail, the resolver and the SAR export answer, with no trace of when", got)
	}

	// The control: a SECOND fill against the now-occupied slot writes no
	// handle, and must not claim the social write a reader would then look for.
	fillLinkedinFromSite(ctx, t, e, companyID, "Tuva Lien", "https://www.linkedin.com/in/tuva-lien-2")
	if got := countAuditRowsHolding(ctx, t, e.store, "contact", contactID.UUID, "social"); got != 1 {
		t.Errorf("audit rows naming the social write = %d after a fill that found the slot occupied, "+
			"want the original 1 — the two writes must not read alike", got)
	}
}
