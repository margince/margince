// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// Two contacts holding one LinkedIn profile are a review pair, whatever their
// addresses and names say. The write stands, and the pair retires once the
// profiles differ.

import (
	"context"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The Go key and the SQL expression must give one answer. Otherwise the lane
// compares a key no handle is indexed under.
func TestTheLinkedInProfileKeyAgreesInGoAndSQL(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	for _, raw := range []string{
		"https://www.linkedin.com/in/sam-example", "https://de.linkedin.com/in/Sam-Example/",
		"https://www.linkedin.com/in/sam-example/details/experience", "http://LinkedIn.com/in/sam?trk=x",
		"https://www.linkedin.com/in/jörg-müller", "https://www.linkedin.com/company/acme",
		"https://example.com/in/sam", "https://linkedin.com.example.com/in/sam",
	} {
		// The SQL reads handles as the slot stores them. A value the slot
		// refuses is asked as written, and must name no profile either way.
		handle := raw
		if stored, err := NormalizeContactLinkedInURL(raw); err == nil {
			handle = stored
		}
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

// openLinkedInPairs counts the open pairs between the two that name the
// LinkedIn profile as their evidence, whichever detector filed them.
func (e *dedupeEnv) openLinkedInPairs(ctx context.Context, t *testing.T, a, b ids.ContactID) int {
	t.Helper()
	var n int
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT count(*) FROM dedupe_candidate
			 WHERE entity_type = 'contact' AND disposition = 'open' AND archived_at IS NULL
			   AND evidence @> jsonb_build_array(jsonb_build_object($3::text, $4::text))
			   AND $1 IN (left_contact_id, right_contact_id)
			   AND $2 IN (left_contact_id, right_contact_id)`, a, b, evidenceFieldKey, fieldLinkedin).Scan(&n)
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

// Two contacts that already shared a profile before the lane existed reach the
// queue through the duplicate scan, which writes no handle of its own.
func TestTheDuplicateScanPairsContactsAlreadySharingALinkedInProfile(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	first := e.createWithLinkedIn(ctx, t, "Kim Example", "kim@example.com", "")
	second := e.createWithLinkedIn(ctx, t, "Kimberly Example", "kim@example.org", "")
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO contact_social (contact_id, platform, handle)
			VALUES ($1, 'linkedin', 'https://www.linkedin.com/in/kim-example'),
			       ($2, 'linkedin', 'https://www.linkedin.com/in/Kim-Example')`, first, second)
		return err
	}); err != nil {
		t.Fatalf("seeding the handles a writer filed before the lane existed: %v", err)
	}
	if n := e.openLinkedInPairs(ctx, t, first, second); n != 0 {
		t.Fatalf("the seed filed %d pairs, so the scan would prove nothing", n)
	}
	e.scanEverything(t, 10)
	if n := e.openLinkedInPairs(ctx, t, first, second); n != 1 {
		t.Errorf("open LinkedIn pairs after the scan = %d, want 1", n)
	}
}

// One profile has more holders than one pass files. Each later pass reaches
// the pairs the first left out, until every pair is on the queue.
func TestTheScanReachesEveryPairOfAProfileHeldPastTheCap(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	holders := make([]ids.ContactID, 7)
	for i := range holders {
		holders[i] = e.createWithLinkedIn(ctx, t, fmt.Sprintf("Holder %d", i), fmt.Sprintf("holder%d@example.com", i), "")
	}
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		for _, h := range holders {
			if _, err := tx.Exec(ctx, `INSERT INTO contact_social (contact_id, platform, handle)
				VALUES ($1, 'linkedin', 'https://www.linkedin.com/in/shared-profile')`, h); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("seeding the shared profile: %v", err)
	}
	for pass := range 6 {
		if filed, _ := e.scanEverything(t, 10); filed == 0 {
			break
		} else if pass == 5 {
			t.Fatalf("the scan still filed %d pairs on its sixth pass", filed)
		}
	}
	want := len(holders) * (len(holders) - 1) / 2
	got := 0
	for i := range holders {
		for j := i + 1; j < len(holders); j++ {
			got += e.openLinkedInPairs(ctx, t, holders[i], holders[j])
		}
	}
	if got != want {
		t.Errorf("open LinkedIn pairs among the seven holders = %d, want %d", got, want)
	}
}

// A pair filed on a shared phone that now holds only on LinkedIn stays. The
// queue shows the shared profile as its evidence, not an empty address pair.
func TestAPhonePairThatHoldsOnLinkedInShowsTheProfile(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	left := e.seedContact(ctx, t, "Ola Berg", []string{"ola@phone.example"}, []string{"+49305550111"})
	right := e.seedContact(ctx, t, "Kim Lund", []string{"kim@phone.example"}, []string{"+49305550111"})
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO contact_social (contact_id, platform, handle)
			VALUES ($1, 'linkedin', 'https://www.linkedin.com/in/one-human'),
			       ($2, 'linkedin', 'https://www.linkedin.com/in/one-human')`, left, right)
		return err
	}); err != nil {
		t.Fatalf("seeding the shared profile: %v", err)
	}
	if _, err := e.store.UpdateContact(ctx, right, UpdateContactInput{Phones: []ContactPhoneInput{}}); err != nil {
		t.Fatal(err)
	}
	if got := e.recheck(t); got.Closed != 0 {
		t.Fatalf("the re-check closed a pair the shared profile still holds: %+v", got)
	}
	evidence := pairEvidence(ctx, t, e, right.UUID)
	if len(evidence) != 1 || evidence[0][evidenceFieldKey] != fieldLinkedin {
		t.Errorf("the pair's evidence is %v, want the shared LinkedIn profile", evidence)
	}
}
