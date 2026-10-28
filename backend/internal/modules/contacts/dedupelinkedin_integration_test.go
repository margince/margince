// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// Two contacts holding one LinkedIn profile are a review pair, whatever their
// addresses and names say. The write stands, and the pair retires once the
// profiles differ.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The Go key and the SQL expression must give one answer. Otherwise the lane
// compares a key no handle is indexed under.
func TestTheLinkedInProfileKeyAgreesInGoAndSQL(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	for _, handle := range []string{
		"https://www.linkedin.com/in/sam-example", "https://de.linkedin.com/in/Sam-Example/",
		"https://www.linkedin.com/in/sam-example/details/experience", "http://LinkedIn.com/in/sam?trk=x",
		"https://www.linkedin.com/company/acme", "https://example.com/in/sam",
		"https://linkedin.com.example.com/in/sam",
	} {
		want, ok := linkedInProfileKey(handle)
		var got *string
		if err := e.store.tx(ctx, func(tx pgx.Tx) error {
			return tx.QueryRow(ctx, `SELECT `+linkedInProfileKeySQL("$1::text"), handle).Scan(&got)
		}); err != nil {
			t.Fatalf("ask SQL about %q: %v", handle, err)
		}
		if (got != nil) != ok || (ok && *got != want) {
			t.Errorf("%q: SQL says %v, Go says %q (a profile: %v)", handle, got, want, ok)
		}
	}
}

// createWithLinkedIn creates a contact through the real writer.
func (e *dedupeEnv) createWithLinkedIn(ctx context.Context, t *testing.T, name, email, profile string) ids.ContactID {
	t.Helper()
	in := CreateContactInput{
		FullName: name, Source: "manual",
		Emails: []ContactEmailInput{{Email: email, EmailType: "work", IsPrimary: true}},
	}
	if profile != "" {
		in.Social = map[string]any{SocialLinkedIn: profile}
	}
	created, err := e.store.CreateContact(ctx, in)
	if err != nil {
		t.Fatalf("creating %s: %v", name, err)
	}
	return ids.From[ids.ContactKind](ids.UUID(created.Id))
}

// openLinkedInPairs counts the open pairs this lane filed between the two.
func (e *dedupeEnv) openLinkedInPairs(ctx context.Context, t *testing.T, a, b ids.ContactID) int {
	t.Helper()
	var n int
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT count(*) FROM dedupe_candidate
			 WHERE entity_type = 'contact' AND disposition = 'open' AND archived_at IS NULL
			   AND source = $3 AND $1 IN (left_contact_id, right_contact_id)
			   AND $2 IN (left_contact_id, right_contact_id)`, a, b, linkedInLaneSource).Scan(&n)
	}); err != nil {
		t.Fatalf("count the pair: %v", err)
	}
	return n
}

func TestTwoContactsHoldingOneLinkedInProfileAreAReviewPair(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	sam := e.createWithLinkedIn(ctx, t, "Sam Example", "sam@example.com", "https://www.linkedin.com/in/sam-example")
	samuel := e.createWithLinkedIn(ctx, t, "Samuel Example", "sam@example.org", "https://de.linkedin.com/in/sam-example/")
	if n := e.openLinkedInPairs(ctx, t, sam, samuel); n != 1 {
		t.Fatalf("open LinkedIn pairs after the second create = %d, want 1", n)
	}
	evidence := pairEvidence(ctx, t, e, samuel.UUID)
	if len(evidence) != 1 || evidence[0][evidenceFieldKey] != fieldLinkedin {
		t.Fatalf("the pair's evidence is %v, want the shared LinkedIn profile", evidence)
	}

	// An edit that sets the profile files the same pair.
	later := e.createWithLinkedIn(ctx, t, "S. Example", "s@example.net", "")
	if _, err := e.store.UpdateContact(ctx, later, UpdateContactInput{
		Social: map[string]any{SocialLinkedIn: "https://www.linkedin.com/in/Sam-Example"},
	}); err != nil {
		t.Fatalf("setting the profile by an edit: %v", err)
	}
	if n := e.openLinkedInPairs(ctx, t, sam, later); n != 1 {
		t.Errorf("open LinkedIn pairs after the edit = %d, want 1", n)
	}

	// Once the profiles differ, the next re-check retires the pair.
	if _, err := e.store.UpdateContact(ctx, samuel, UpdateContactInput{
		Social: map[string]any{SocialLinkedIn: "https://www.linkedin.com/in/samuel-other"},
	}); err != nil {
		t.Fatalf("changing the profile: %v", err)
	}
	e.recheck(t)
	if n := e.openLinkedInPairs(ctx, t, sam, samuel); n != 0 {
		t.Errorf("open LinkedIn pairs after the profile changed and the re-check ran = %d, want 0", n)
	}
	if n := e.openLinkedInPairs(ctx, t, sam, later); n != 1 {
		t.Errorf("the re-check retired a pair whose profiles still match: %d open", n)
	}
}
