// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// Gmail ignores the dots of a local part and answers at gmail.com and
// googlemail.com alike. Two such spellings are one mailbox and a review pair.
// Mail still routes on the address as written.

import (
	"encoding/json"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/correspondence"
)

func TestAGmailSpellingOfOneMailboxIsAReviewPair(t *testing.T) {
	cases := []struct{ name, incumbent, candidate string }{
		{"gmail beside gmail", "ab@gmail.com", "a.b@gmail.com"},
		{"gmail beside googlemail", "a.b@gmail.com", "ab@googlemail.com"},
		{"googlemail beside gmail", "a.b@googlemail.com", "A.B.@Gmail.com"},
		{"googlemail beside googlemail", "ab@googlemail.com", "a.b+crm@googlemail.com"},
		{"only the domain differs", "ab@gmail.com", "ab@googlemail.com"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := setupDedupe(t)
			ctx := e.as()
			incumbent := e.plainContact(ctx, t, "Ab", tc.incumbent)
			got := e.dedupeInTx(ctx, t, ContactCandidate{
				FullName: "Anna Beispiel", Emails: []string{tc.candidate}, QueueNameCollisions: true,
			})
			if got.Decision != DecisionAddressVariantReview || got.ContactID.UUID != incumbent {
				t.Fatalf("got %s on %s, want %s on %s", got.Decision, got.ContactID, DecisionAddressVariantReview, incumbent)
			}
			if len(got.Reviews) != 1 || got.Reviews[0].IncumbentEmail != tc.incumbent ||
				got.Reviews[0].CandidateEmail != correspondence.Fold(tc.candidate) {
				t.Errorf("reviews %+v, want one pair carrying both addresses as written", got.Reviews)
			}
		})
	}
}

func TestADottedAddressAtAnotherHostIsNoVariant(t *testing.T) {
	for incumbent, candidate := range map[string]string{
		"ab@example.com": "a.b@example.com", "ab@outlook.com": "a.b@outlook.com", "ab@mail.gmail.com": "a.b@mail.gmail.com",
	} {
		t.Run(incumbent, func(t *testing.T) {
			e := setupDedupe(t)
			ctx := e.as()
			e.plainContact(ctx, t, "Ab", incumbent)
			got := e.dedupeInTx(ctx, t, ContactCandidate{
				FullName: "Anna Beispiel", Emails: []string{candidate}, QueueNameCollisions: true,
			})
			if got.Decision != DecisionNoMatch {
				t.Fatalf("got %s, want %s: only Gmail ignores the dots of a local part", got.Decision, DecisionNoMatch)
			}
		})
	}
}

func TestAGmailSpellingNeverAnswersARoutingCaller(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	e.plainContact(ctx, t, "Ab", "ab@gmail.com")
	got := e.dedupeInTx(ctx, t, ContactCandidate{FullName: "Anna Beispiel", Emails: []string{"a.b@googlemail.com"}})
	if got.Decision != DecisionNoMatch {
		t.Fatalf("a routing caller got %s for a Gmail spelling, want %s", got.Decision, DecisionNoMatch)
	}
}

func TestCapturingAGmailSpellingCreatesAContactAndFilesOnePair(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	incumbent := e.plainContact(ctx, t, "Ab", "ab@gmail.com")
	res, err := e.store.EnsureCounterparty(ctx,
		e.datedEnsureInput(ctx, t, "a.b@googlemail.com", "googlemail.com", nowInTx(ctx, t, e)))
	if err != nil || !res.ContactCreated {
		t.Fatalf("ensure = %+v (err %v), want a new contact: the base address must not route the mail", res, err)
	}
	if res.ContactID.UUID == incumbent {
		t.Fatal("capture landed the googlemail address on the gmail address's contact")
	}
	pairs := openCandidatePairs(ctx, t, e, "contact", res.ContactID.UUID)
	if len(pairs) != 1 || pairs[0].OtherID != incumbent.String() {
		t.Fatalf("got pairs %+v, want one pair with %s", pairs, incumbent)
	}
}

// Postgres pairs an expression index with a query by syntax, so a migration
// that spells the expression differently builds an index the lane never reads.
// Sequential scans are off, so a small table cannot make one look cheap.
func TestTheAddressVariantLaneReadsTheMailboxBaseIndex(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	e.execAs(ctx, t, `
		WITH seeded AS (
		  INSERT INTO contact (full_name, source, captured_by)
		  SELECT 'Contact ' || g, 'manual', 'human:test' FROM generate_series(1, 2000) g RETURNING id, full_name)
		INSERT INTO contact_email (contact_id, email, source, captured_by)
		SELECT id, 'p.' || replace(full_name, 'Contact ', '') || '@gmail.com', 'manual', 'human:test' FROM seeded`)
	analyzeAsOwner(t, "contact", "contact_email")

	var raw []byte
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SET LOCAL enable_seqscan = off`); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `EXPLAIN (FORMAT JSON) `+addressVariantQuery,
			[]string{"p7@gmail.com"}, []string{"p.7@googlemail.com"}, nil, dedupeRivalLimit).Scan(&raw)
	}); err != nil {
		t.Fatalf("planning the lane query: %v", err)
	}
	var plans []struct {
		Plan mailboxPlanNode `json:"Plan"` //nolint:tagliatelle // Postgres's EXPLAIN key, not ours to rename
	}
	if err := json.Unmarshal(raw, &plans); err != nil || len(plans) != 1 {
		t.Fatalf("reading the plan: %v (%d plans)", err, len(plans))
	}
	if !plans[0].Plan.usesIndex("idx_contact_email_mailbox_base") {
		t.Errorf("the lane query does not read idx_contact_email_mailbox_base:\n%s", raw)
	}
}

// mailboxPlanNode is the part of an EXPLAIN (FORMAT JSON) node this test reads.
type mailboxPlanNode struct {
	IndexName string            `json:"Index Name"` //nolint:tagliatelle // Postgres's EXPLAIN key, not ours to rename
	Plans     []mailboxPlanNode `json:"Plans"`      //nolint:tagliatelle // Postgres's EXPLAIN key, not ours to rename
}

func (n mailboxPlanNode) usesIndex(name string) bool {
	if n.IndexName == name {
		return true
	}
	for _, child := range n.Plans {
		if child.usesIndex(name) {
			return true
		}
	}
	return false
}
