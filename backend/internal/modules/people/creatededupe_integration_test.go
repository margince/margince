// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package people

// Manual creates meet the PO-F chokepoint under the per-lane policy
// creatededupe.go states: a claimed ADDRESS still refuses with the 409
// contract (existing id disclosed under visibility); a shared PHONE does
// not refuse but records the pair, because an exact hit routes past the
// fuzzy tier and would otherwise leave no trail at all; and a fuzzy
// near-match creates the record anyway and leaves both the queue row and
// one dedupe_near_match system_log line, committed in the same
// transaction as the create.

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
)

// nearMatchLines returns every dedupe_near_match ledger line for one
// created entity, so a test can assert both presence and count.
func nearMatchLines(ctx context.Context, t *testing.T, e *dedupeEnv, entityType string, createdID ids.UUID) []struct {
	MatchedID  string
	Confidence float64
} {
	t.Helper()
	var out []struct {
		MatchedID  string
		Confidence float64
	}
	err := e.store.tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT detail->>'matched_id', (detail->>'confidence')::float8
			  FROM system_log
			 WHERE action = 'dedupe_near_match'
			   AND detail->>'entity_type' = $1
			   AND detail->>'created_id' = $2`,
			entityType, createdID.String())
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var line struct {
				MatchedID  string
				Confidence float64
			}
			if err := rows.Scan(&line.MatchedID, &line.Confidence); err != nil {
				return err
			}
			out = append(out, line)
		}
		return rows.Err()
	})
	if err != nil {
		t.Fatalf("read dedupe_near_match ledger: %v", err)
	}
	return out
}

// openCandidatePairs returns every OPEN review-queue row naming one created
// entity, with the other side and the score. It reads dedupe_candidate itself
// rather than the near-match ledger: the ledger records the fuzzy tier's own
// working, while the queue is what a human is actually shown, and a pair can
// reach the queue from an exact lane that writes no ledger line at all.
func openCandidatePairs(ctx context.Context, t *testing.T, e *dedupeEnv, entityType string, createdID ids.UUID) []struct {
	OtherID    string
	Confidence float64
} {
	t.Helper()
	var out []struct {
		OtherID    string
		Confidence float64
	}
	err := e.store.tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT CASE WHEN left_person_id = $2 THEN right_person_id ELSE left_person_id END,
			       confidence
			  FROM dedupe_candidate
			 WHERE entity_type = $1 AND disposition = 'open'
			   AND $2 IN (left_person_id, right_person_id)`,
			entityType, createdID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var pair struct {
				OtherID    string
				Confidence float64
			}
			if err := rows.Scan(&pair.OtherID, &pair.Confidence); err != nil {
				return err
			}
			out = append(out, pair)
		}
		return rows.Err()
	})
	if err != nil {
		t.Fatalf("read the dedupe review queue: %v", err)
	}
	return out
}

func TestCreatePersonFuzzyNearMatchCreatesAndRecords(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	johnID, _ := e.seedEmployedPerson(ctx, t, "John Doe", "john.doe@acme.test", "Acme GmbH", "acme.test")

	// Similar name + an address on the employer's domain: 0.55·0.9667 +
	// 0.45·0.8 = 0.8917 ≥ 0.72 → fuzzy review. The manual policy creates
	// anyway — a probability never blocks a human — and records the pair.
	created, err := e.store.CreatePerson(ctx, CreatePersonInput{
		FullName: "Jon Doe", Source: "manual",
		Emails: []PersonEmailInput{{Email: "jon@acme.test", EmailType: "work", IsPrimary: true}},
	})
	if err != nil {
		t.Fatalf("a fuzzy near-match must create, not block: %v", err)
	}

	lines := nearMatchLines(ctx, t, e, "person", ids.UUID(created.Id))
	if len(lines) != 1 {
		t.Fatalf("got %d dedupe_near_match lines for the created person, want exactly 1", len(lines))
	}
	if lines[0].MatchedID != johnID.String() {
		t.Fatalf("recorded matched_id = %s, want the incumbent %s", lines[0].MatchedID, johnID)
	}
	if lines[0].Confidence < dedupeReviewThreshold {
		t.Fatalf("recorded confidence %.4f below the review threshold %.2f", lines[0].Confidence, dedupeReviewThreshold)
	}

	// The incumbent himself was created through the same path with no
	// near neighbour — a clean create must not leave a ledger line.
	if clean := nearMatchLines(ctx, t, e, "person", johnID.UUID); len(clean) != 0 {
		t.Fatalf("a no-match create left %d dedupe_near_match lines, want 0", len(clean))
	}
}

func TestCreatePersonExactEmailStillRefusesWith409(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	johnID, _ := e.seedEmployedPerson(ctx, t, "John Doe", "john.exact@acme.test", "Acme GmbH", "acme.test")

	_, err := e.store.CreatePerson(ctx, CreatePersonInput{
		FullName: "Johnny Doe", Source: "manual",
		Emails: []PersonEmailInput{{Email: "JOHN.EXACT@ACME.TEST", EmailType: "work", IsPrimary: true}},
	})
	var dup *DuplicateEmailError
	if !errors.As(err, &dup) {
		t.Fatalf("exact email collision must stay the typed 409, got %v", err)
	}
	if dup.ExistingID != johnID {
		t.Fatalf("409 discloses %s, want the incumbent %s", dup.ExistingID, johnID)
	}
}

// A shared phone number is the exact lane a manual create CAN hit, and the
// one whose policy is neither of the other two: a household line and a
// switchboard belong to several real people, so refusing would be wrong, and
// staying silent would be worse than the fuzzy tier — an exact hit returns
// before scoring, so nothing else would record this pair.
//
// The names here are deliberately unalike and the addresses unrelated, so the
// fuzzy tier cannot reach the review threshold. If this test passes, the phone
// lane is what put the pair on the queue.
func TestCreatePersonSharingAPhoneCreatesAndRecordsThePair(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	const household = "+4915100000501"
	incumbent := e.seedPerson(ctx, t, "Wilhelmina Braunschweiger", []string{"w@household.test"}, []string{household})

	// The second number arrives in a different notation of the same line; the
	// lane compares the stored E.164 form, so it still matches.
	created, err := e.store.CreatePerson(ctx, CreatePersonInput{
		FullName: "Tim Ho", Source: "manual",
		Emails: []PersonEmailInput{{Email: "tim@elsewhere.test", EmailType: "work", IsPrimary: true}},
		Phones: []PersonPhoneInput{{Phone: "0049 151 0000 0501", PhoneType: "home", IsPrimary: true}},
	})
	if err != nil {
		t.Fatalf("a shared phone must not refuse a manual create: %v", err)
	}
	createdID := ids.From[ids.PersonKind](ids.UUID(created.Id))

	rows := openCandidates(ctx, t, e, entityPerson)
	if len(rows) != 1 {
		t.Fatalf("open queue holds %d candidates after a create on a shared number, want exactly 1", len(rows))
	}
	pair := map[ids.UUID]bool{rows[0].LeftID: true, rows[0].RightID: true}
	if !pair[incumbent.UUID] || !pair[createdID.UUID] {
		t.Fatalf("candidate pair = {%s, %s}, want it to name the incumbent %s and the create %s",
			rows[0].LeftID, rows[0].RightID, incumbent, createdID)
	}
	if rows[0].Confidence != identityConflictConfidence {
		t.Fatalf("confidence = %v, want the exact-key ceiling %v — an established key on both records outranks any similarity score",
			rows[0].Confidence, identityConflictConfidence)
	}

	// The evidence must name the number the lane matched on, in the stored
	// form, on BOTH sides: that equality is the whole finding.
	var evidence []evidenceEntry
	if err := json.Unmarshal(rows[0].Evidence, &evidence); err != nil {
		t.Fatalf("unmarshal evidence: %v", err)
	}
	found := false
	for _, ev := range evidence {
		if ev.Field != fieldPhone {
			continue
		}
		if ev.LeftValue == nil || ev.RightValue == nil {
			t.Fatalf("phone evidence entry %+v names only one side", ev)
		}
		if *ev.LeftValue != household || *ev.RightValue != household {
			t.Fatalf("phone evidence = %q / %q, want the shared number %q on both sides",
				*ev.LeftValue, *ev.RightValue, household)
		}
		found = true
	}
	if !found {
		t.Fatalf("evidence %+v does not name the shared phone that routed this decision", evidence)
	}

	// The ledger line belongs to the fuzzy arm — it records a SCORE that was
	// acted on. An exact key hit has no score to justify, and the queue row is
	// the trail; a line here would claim a near-match judgment nobody made.
	if lines := nearMatchLines(ctx, t, e, entityPerson, ids.UUID(created.Id)); len(lines) != 0 {
		t.Fatalf("an exact phone collision left %d dedupe_near_match lines, want 0", len(lines))
	}
}

func TestCreateOrganizationFuzzyNearMatchCreatesAndRecords(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	incumbent, err := e.store.CreateOrganization(ctx, CreateOrganizationInput{
		DisplayName: "Wayne Enterprises GmbH", Source: "manual",
		Domains: []OrgDomainInput{{Domain: "wayne.test", IsPrimary: true}},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Same stem, different legal suffix, no shared domain: suffix
	// normalization scores 1.0 → fuzzy review. Different legal entities
	// are a human's call, so the create proceeds and the pair records.
	created, err := e.store.CreateOrganization(ctx, CreateOrganizationInput{
		DisplayName: "Wayne Enterprises Inc", Source: "manual",
		Domains: []OrgDomainInput{{Domain: "wayne-us.test", IsPrimary: true}},
	})
	if err != nil {
		t.Fatalf("a fuzzy near-match must create, not block: %v", err)
	}

	lines := nearMatchLines(ctx, t, e, "organization", ids.UUID(created.Id))
	if len(lines) != 1 {
		t.Fatalf("got %d dedupe_near_match lines for the created org, want exactly 1", len(lines))
	}
	if lines[0].MatchedID != ids.UUID(incumbent.Id).String() {
		t.Fatalf("recorded matched_id = %s, want the incumbent %s", lines[0].MatchedID, incumbent.Id)
	}
	if lines[0].Confidence < dedupeReviewThreshold {
		t.Fatalf("recorded confidence %.4f below the review threshold %.2f", lines[0].Confidence, dedupeReviewThreshold)
	}

	if clean := nearMatchLines(ctx, t, e, "organization", ids.UUID(incumbent.Id)); len(clean) != 0 {
		t.Fatalf("a no-match create left %d dedupe_near_match lines, want 0", len(clean))
	}
}

func TestCreateOrganizationExactDomainStillRefusesWith409(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	incumbent, err := e.store.CreateOrganization(ctx, CreateOrganizationInput{
		DisplayName: "Stark Industries GmbH", Source: "manual",
		Domains: []OrgDomainInput{{Domain: "stark.test", IsPrimary: true}},
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = e.store.CreateOrganization(ctx, CreateOrganizationInput{
		DisplayName: "Totally Unrelated Name", Source: "manual",
		Domains: []OrgDomainInput{{Domain: "STARK.TEST", IsPrimary: true}},
	})
	var dup *DuplicateDomainError
	if !errors.As(err, &dup) {
		t.Fatalf("exact domain collision must stay the typed 409, got %v", err)
	}
	if dup.ExistingID.UUID != ids.UUID(incumbent.Id) {
		t.Fatalf("409 discloses %s, want the incumbent %s", dup.ExistingID, incumbent.Id)
	}
}

// LARS'S CASE, 2026-08-25: one person, a second business card, a different
// address. Same name, same employer, and no key in common — the second card
// carries the new address and nothing else that has been seen before.
//
// This is the scenario a rep actually hits. A card is typed in with a personal
// address, or with a typo, and nothing on it matches what is stored except who
// the person is and where they work. The email lane cannot fire, because the
// addresses differ by construction; the phone lane cannot fire, because a card
// hand-entered in a hurry often carries no number at all.
//
// It has to reach a human. Merging two people is not a decision an automatic
// rule should take — a father and son at one firm are a real pair of records
// that share everything this test shares — so the answer is the review queue,
// not a silent merge and not a refusal.
func TestCreatePersonWithASecondAddressAtTheSameEmployerIsQueuedForAHuman(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	lucyID, orgID := e.seedEmployedPerson(ctx, t,
		"Lucy Vo", "lucy.vo@terralogic.test", "Terralogic", "terralogic.test")

	// The second card: identical name, an address nobody has seen, no phone.
	// The employment edge is stated the way a rep filing a card would state it
	// — they know where she works, which is the whole reason the pair is worth
	// a human's glance.
	created, err := e.store.CreatePerson(ctx, CreatePersonInput{
		FullName: "Lucy Vo", Source: "manual",
		Emails: []PersonEmailInput{{Email: "lucy@personal.test", EmailType: "personal", IsPrimary: true}},
	})
	if err != nil {
		t.Fatalf("a second card must create, never block — a probability is not a conflict: %v", err)
	}
	createdID := ids.From[ids.PersonKind](ids.UUID(created.Id))
	primary := true
	if _, err := e.store.CreateRelationship(ctx, CreateRelationshipInput{
		Kind: "employment", PersonID: &createdID, OrganizationID: &orgID,
		IsCurrentPrimary: &primary, Source: "manual",
	}); err != nil {
		t.Fatalf("attach the second card's employer: %v", err)
	}

	pairs := openCandidatePairs(ctx, t, e, "person", ids.UUID(created.Id))
	if len(pairs) != 1 {
		t.Fatalf("got %d open dedupe candidates for the second card, want exactly 1 — "+
			"one person with two addresses at one employer is the case a human has to settle", len(pairs))
	}
	if pairs[0].OtherID != lucyID.String() {
		t.Fatalf("the candidate names %s, want the incumbent %s", pairs[0].OtherID, lucyID)
	}
}

// The other half of the same rule, and the reason the lane flags instead of
// merging: two DIFFERENT people can be written exactly the same way. A father
// and son at one firm share a name and an employer and are two real records.
//
// So the create must succeed and both records must survive. The pair still
// reaches the queue — a human is the only one who can tell these two cases
// apart — but nothing about the second record is merged, moved or refused.
func TestAnIdenticalNameCreatesASecondRecordAndNeverMergesIt(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	seniorID, _ := e.seedEmployedPerson(ctx, t,
		"Karl Fischer", "karl.fischer@fischerbau.test", "Fischer Bau", "fischerbau.test")

	// The junior's address is on a DIFFERENT domain, deliberately. A shared
	// employer domain scores 0.8 on the org term and carries the pair over the
	// fuzzy bar on its own — which would prove the old tier still works and say
	// nothing about the name lane. This pair has the name and nothing else.
	junior, err := e.store.CreatePerson(ctx, CreatePersonInput{
		FullName: "Karl Fischer", Source: "manual",
		Emails: []PersonEmailInput{{Email: "karl.junior@privat.test", EmailType: "personal", IsPrimary: true}},
	})
	if err != nil {
		t.Fatalf("two people may share a name: %v", err)
	}
	if ids.UUID(junior.Id) == seniorID.UUID {
		t.Fatal("the create returned the incumbent's id — the lane merged two records it was only allowed to question")
	}

	// Both rows are live. A lane that flags must leave the data exactly as a
	// lane that did nothing would.
	var live int
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT count(*) FROM person WHERE id IN ($1, $2) AND archived_at IS NULL AND merged_into_id IS NULL`,
			seniorID, ids.UUID(junior.Id)).Scan(&live)
	}); err != nil {
		t.Fatalf("count the two records: %v", err)
	}
	if live != 2 {
		t.Fatalf("got %d live unmerged records, want 2 — flagging a pair must not touch either row", live)
	}

	// And the question was asked, once.
	if pairs := openCandidatePairs(ctx, t, e, "person", ids.UUID(junior.Id)); len(pairs) != 1 {
		t.Fatalf("got %d open candidates, want exactly 1 — a name collision is a question, and it has to be put", len(pairs))
	}
}

// A create that resembles nobody must still write nothing. The lane is narrow
// by construction — it fires on an exact normalized name and on nothing else —
// and this is what holds it there: a near-name below the fuzzy bar has no key
// in common and no exact name either, so it is simply a new person.
func TestANameNobodySharesLeavesTheQueueEmpty(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	e.seedEmployedPerson(ctx, t, "Karl Fischer", "karl.fischer@fischerbau.test", "Fischer Bau", "fischerbau.test")

	created, err := e.store.CreatePerson(ctx, CreatePersonInput{
		FullName: "Karla Fischerova", Source: "manual",
		Emails: []PersonEmailInput{{Email: "karla@elsewhere.test", EmailType: "work", IsPrimary: true}},
	})
	if err != nil {
		t.Fatalf("create an unrelated person: %v", err)
	}
	if pairs := openCandidatePairs(ctx, t, e, "person", ids.UUID(created.Id)); len(pairs) != 0 {
		t.Fatalf("got %d open candidates for a name nobody shares, want 0 — "+
			"a lane that queues everything is a queue nobody works", len(pairs))
	}
}
