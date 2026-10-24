// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// One human on a second address: a plus tag, a subdomain of the employer's
// domain, or a name with "oe" for "ö". Each case is a review pair for a human,
// never a merge and never a route.

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/freemail"
	"github.com/margince/margince/backend/internal/shared/kernel/correspondence"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// pairEvidence reads the evidence of the open contact pair naming one record.
func pairEvidence(ctx context.Context, t *testing.T, e *dedupeEnv, created ids.UUID) []map[string]any {
	t.Helper()
	var raw []byte
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT evidence FROM dedupe_candidate
			 WHERE entity_type = 'contact' AND disposition = 'open'
			   AND $1 IN (left_contact_id, right_contact_id)`, created).Scan(&raw)
	}); err != nil {
		t.Fatalf("read the pair's evidence: %v", err)
	}
	var evidence []map[string]any
	if err := json.Unmarshal(raw, &evidence); err != nil {
		t.Fatalf("decode the pair's evidence: %v", err)
	}
	return evidence
}

func TestATaggedAddressIsAReviewPairWithItsBase(t *testing.T) {
	cases := []struct {
		name, incumbent, candidate string
	}{
		{"tagged candidate", "ab@example.com", "ab+accounting@example.com"},
		{"tagged incumbent", "ab+accounting@example.com", "ab@example.com"},
		{"two tags", "ab+sales@example.com", "AB+accounting@Example.com"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := setupDedupe(t)
			ctx := e.as()
			// "Ab" is the name capture gives a contact known only by its address;
			// the lane must pair it although the names have nothing in common.
			incumbent := e.plainContact(ctx, t, "Ab", tc.incumbent)
			got := e.dedupeInTx(ctx, t, ContactCandidate{
				FullName: "Anna Beispiel", Emails: []string{tc.candidate}, QueueNameCollisions: true,
			})
			if got.Decision != DecisionAddressVariantReview || got.ContactID.UUID != incumbent {
				t.Fatalf("got %s on %s, want %s on %s", got.Decision, got.ContactID, DecisionAddressVariantReview, incumbent)
			}
			if len(got.Reviews) != 1 || got.Reviews[0].IncumbentEmail != correspondence.Fold(tc.incumbent) ||
				got.Reviews[0].CandidateEmail != correspondence.Fold(tc.candidate) {
				t.Errorf("reviews %+v, want one pair carrying both addresses", got.Reviews)
			}
		})
	}
}

func TestATaggedAddressNeverAnswersARoutingCaller(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	e.plainContact(ctx, t, "Ab", "ab@example.com")
	got := e.dedupeInTx(ctx, t, ContactCandidate{
		FullName: "Anna Beispiel", Emails: []string{"ab+accounting@example.com"},
	})
	if got.Decision != DecisionNoMatch {
		t.Fatalf("a routing caller got %s for a tagged address, want %s: a shared tag must never route mail",
			got.Decision, DecisionNoMatch)
	}
}

func TestAnUnrelatedAddressOnTheSameDomainIsNoVariant(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	e.plainContact(ctx, t, "Ab", "ab@example.com")
	got := e.dedupeInTx(ctx, t, ContactCandidate{
		FullName: "Anna Beispiel", Emails: []string{"abc+accounting@example.com"}, QueueNameCollisions: true,
	})
	if got.Decision != DecisionNoMatch {
		t.Fatalf("got %s, want %s: abc@ is not ab@", got.Decision, DecisionNoMatch)
	}
}

func TestCreatingATaggedAddressFilesTheAddressPair(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	incumbent := e.plainContact(ctx, t, "Ab", "ab@example.com")
	created := e.plainContact(ctx, t, "Anna Beispiel", "ab+accounting@example.com")

	pairs := openCandidatePairs(ctx, t, e, "contact", created)
	if len(pairs) != 1 || pairs[0].OtherID != incumbent.String() {
		t.Fatalf("got pairs %+v, want one pair with %s", pairs, incumbent)
	}
	if pairs[0].Confidence != 0 {
		t.Errorf("confidence %v, want 0: the lane compared strings and scored nothing", pairs[0].Confidence)
	}
	evidence := pairEvidence(ctx, t, e, created)
	if len(evidence) != 1 || evidence[0][evidenceFieldKey] != fieldEmail ||
		evidence[0][evidenceLeftKey] != "ab+accounting@example.com" || evidence[0][evidenceRightKey] != "ab@example.com" {
		t.Fatalf("evidence %v, want the two addresses side by side", evidence)
	}
}

func TestATaggedVCardIsReportedAsAPossibleDuplicate(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	incumbent := e.plainContact(ctx, t, "Ab", "ab@example.com")
	results, err := e.store.ImportVCards(ctx, []VCardEntry{{
		FullName: "Anna Beispiel", Emails: []VCardChannel{{Value: "ab+accounting@example.com"}},
	}})
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if len(results) != 1 || results[0].Outcome != VCardNeedsReview ||
		results[0].ContactID == nil || results[0].ContactID.UUID != incumbent {
		t.Fatalf("got %+v, want a possible duplicate of %s", results, incumbent)
	}
}

func TestCapturingATaggedAddressFilesOnePair(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	incumbent := e.plainContact(ctx, t, "Ab", "ab@example.com")
	res, err := e.store.EnsureCounterparty(ctx,
		e.datedEnsureInput(ctx, t, "ab+accounting@example.com", "example.com", nowInTx(ctx, t, e)))
	if err != nil || !res.ContactCreated {
		t.Fatalf("ensure = %+v (err %v), want a new contact: the base address must not route the mail", res, err)
	}
	if res.ContactID.UUID == incumbent {
		t.Fatal("capture landed the tagged address on the base address's contact")
	}
	pairs := openCandidatePairs(ctx, t, e, "contact", res.ContactID.UUID)
	if len(pairs) != 1 || pairs[0].OtherID != incumbent.String() {
		t.Fatalf("got pairs %+v, want one pair with %s", pairs, incumbent)
	}
}

// The Go rule and the SQL expression must give one answer. Otherwise the lane
// compares a key no address is stored under.
func TestTheAddressBaseAgreesInGoAndSQL(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	for _, address := range []string{
		"ab@example.com", "ab+x@example.com", "+x@example.com", "ab+x+y@example.com",
		"AB+X@Example.COM", "a.b+tag@sub.example.com", "ab@x+y.example.com",
	} {
		want, _ := correspondence.BaseAddress(address)
		var got string
		if err := e.store.tx(ctx, func(tx pgx.Tx) error {
			return tx.QueryRow(ctx, `SELECT `+addressBaseSQL("$1::text"), address).Scan(&got)
		}); err != nil {
			t.Fatalf("ask SQL about %q: %v", address, err)
		}
		if got != want {
			t.Errorf("%q: SQL says %q, Go says %q", address, got, want)
		}
	}
}

func TestASubdomainOfTheEmployersDomainCountsAsTheSameEmployer(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	incumbent, _ := e.seedEmployedContact(ctx, t, "Bernd Beispiel", "bb@example.com", "Company A", "example.com")
	got := e.dedupeInTx(ctx, t, ContactCandidate{FullName: "Bernd Beispiel", Emails: []string{"bb@cloud.example.com"}})
	if got.Decision != DecisionFuzzyReview || got.ContactID != incumbent {
		t.Fatalf("got %s on %s, want %s on %s", got.Decision, got.ContactID, DecisionFuzzyReview, incumbent)
	}
}

func TestASubdomainOfAConsumerMailHostIsNoEmployer(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	e.plainContact(ctx, t, "Bernd Beispiel", "bernd@gmail.com")
	candidate := ContactCandidate{FullName: "Bernd Beispiel", Emails: []string{"b.beispiel@mail.gmail.com"}}
	if got := e.dedupeInTx(ctx, t, candidate); got.Decision != DecisionNoMatch {
		t.Fatalf("got %s, want %s: two addresses at one mailbox provider say nothing about an employer",
			got.Decision, DecisionNoMatch)
	}
	// A workspace that asserts the domain is a company's gets the employer term back.
	candidate.ConsumerMail = freemail.New(nil, []string{"gmail.com"})
	if got := e.dedupeInTx(ctx, t, candidate); got.Decision != DecisionFuzzyReview {
		t.Fatalf("got %s with the carve-out, want %s", got.Decision, DecisionFuzzyReview)
	}
}

func TestATransliteratedUmlautIsTheSameName(t *testing.T) {
	cases := []struct {
		incumbent, candidate string
		want                 DedupeDecision
	}{
		{"Björn Beispiel", "Bjoern Beispiel", DecisionNameCollisionReview},
		{"Bjoern Beispiel", "Björn Beispiel", DecisionNameCollisionReview},
		{"Jürgen Müller", "JUERGEN MUELLER", DecisionNameCollisionReview},
		{"Björn Beispiel", "Bjorn Beispiel", DecisionNameCollisionReview},
		{"Bjoern Beispiel", "Bjorn Beispiel", DecisionNoMatch},
		// A short decomposed name at the ordinary trigram threshold.
		{"Ba\u0308r", "Baer", DecisionNameCollisionReview},
		{"Baer", "Ba\u0308r", DecisionNameCollisionReview},
	}
	for _, tc := range cases {
		t.Run(tc.incumbent+"/"+tc.candidate, func(t *testing.T) {
			e := setupDedupe(t)
			ctx := e.as()
			e.plainContact(ctx, t, tc.incumbent, "one@first.test")
			got := e.dedupeInTx(ctx, t, ContactCandidate{
				FullName: tc.candidate, Emails: []string{"two@second.test"}, QueueNameCollisions: true,
			})
			if got.Decision != tc.want {
				t.Fatalf("got %s, want %s", got.Decision, tc.want)
			}
		})
	}
}

// digraphEqualNamePairs are pairs digraphNameKey calls equal, most of them
// pairs NormalizeContactName does not. Kept apart from goEqualNamePairs, whose
// guard is the plain key.
var digraphEqualNamePairs = []goEqualNamePair{
	{"oe for ö", "Björn Beispiel", "Bjoern Beispiel"},
	{"ue for ü, upper case", "Jürgen Müller", "JUERGEN MUELLER"},
	{"ae for ä", "Märta Bäcker", "Maerta Baecker"},
	// Decomposed input: an "o" or "a" followed by a combining diaeresis, in
	// both directions.
	{"decomposed incumbent", "Bjo\u0308rn Beispiel", "Bjoern Beispiel"},
	{"decomposed candidate", "Bjoern Beispiel", "Bjo\u0308rn Beispiel"},
	{"decomposed against composed", "Ba\u0308r Beispiel", "Bär Beispiel"},
}

func TestTheDigraphArmAdmitsEveryDigraphEqualPair(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	for _, tc := range digraphEqualNamePairs {
		t.Run(tc.name, func(t *testing.T) {
			if digraphNameKey(tc.incumbent) != digraphNameKey(tc.second) {
				t.Fatalf("digraphNameKey does not call %q and %q equal, so this row asserts nothing", tc.incumbent, tc.second)
			}
			var admitted bool
			if err := e.store.tx(ctx, func(tx pgx.Tx) error {
				return tx.QueryRow(ctx, `SELECT `+exactDigraphNameKeySQL("$1::text")+` = `+exactDigraphNameKeySQL("$2::text"),
					tc.incumbent, tc.second).Scan(&admitted)
			}); err != nil {
				t.Fatalf("ask the arm: %v", err)
			}
			if !admitted {
				t.Errorf("the digraph arm does not admit %q against %q", tc.second, tc.incumbent)
			}
		})
	}
}

// With the trigram arm switched off, the pair still reaches the name lane, so
// the digraph arm is present in the real candidate query.
func TestTheDigraphArmIsInTheCandidateQuery(t *testing.T) {
	for _, tc := range digraphEqualNamePairs {
		t.Run(tc.name, func(t *testing.T) {
			e := setupDedupe(t)
			ctx := e.as()
			incumbent := e.plainContact(ctx, t, tc.incumbent, "one@first.test")
			var got ContactResolution
			if err := e.store.tx(ctx, func(tx pgx.Tx) error {
				if _, err := tx.Exec(ctx, `SET LOCAL pg_trgm.similarity_threshold = 1`); err != nil {
					return err
				}
				var err error
				got, err = fuzzyContact(ctx, tx, ContactCandidate{
					FullName: tc.second, Emails: []string{"two@second.test"}, QueueNameCollisions: true,
				})
				return err
			}); err != nil {
				t.Fatalf("run the candidate query: %v", err)
			}
			if got.Decision != DecisionNameCollisionReview || got.ContactID.UUID != incumbent {
				t.Fatalf("got %s on %s, want %s on %s", got.Decision, got.ContactID, DecisionNameCollisionReview, incumbent)
			}
		})
	}
}

func TestACompanyOnASubdomainOfAHeldDomainIsAReviewPair(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	holder := e.createNamedCompany(ctx, t, "Example Group", "example.com")
	got := e.dedupeCompanyInTx(ctx, t, CompanyCandidate{DisplayName: "Cloudy", Domains: []string{"cloud.example.com"}})
	if got.Decision != DecisionFuzzyReview || got.CompanyID != holder || got.Ranked[0].MatchedField != laneDomain {
		t.Fatalf("got %+v, want a domain review pair with %s", got, holder)
	}

	created := e.createNamedCompany(ctx, t, "Cloudy", "cloud.example.com")
	if field := e.filedPairField(ctx, t, created, holder); field != laneDomain {
		t.Fatalf("the create filed field %q, want %q", field, laneDomain)
	}
}

func TestACompanyOnSubdomainsOfTwoHeldDomainsListsBoth(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	first := e.createNamedCompany(ctx, t, "Example Group", "example.com")
	second := e.createNamedCompany(ctx, t, "Other Group", "other.test")
	got := e.dedupeCompanyInTx(ctx, t, CompanyCandidate{
		DisplayName: "Cloudy", Domains: []string{"a.example.com", "b.other.test"},
	})
	listed := map[ids.CompanyID]bool{}
	for _, scored := range got.Ranked {
		listed[scored.CompanyID] = true
	}
	if got.Decision != DecisionFuzzyReview || !listed[first] || !listed[second] {
		t.Fatalf("got %+v, want both holders listed", got)
	}
}

func TestACompanyOnItsOwnRegistrableDomainHasNoSubdomainPair(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	e.createNamedCompany(ctx, t, "Example Group", "example.com")
	got := e.dedupeCompanyInTx(ctx, t, CompanyCandidate{DisplayName: "Cloudy", Domains: []string{"other.example"}})
	if got.Decision != DecisionNoMatch {
		t.Fatalf("got %+v, want no match", got)
	}
}

// beginHeldAddressCreate is a transaction that has taken the base-address lock
// for one address and written a contact holding it, handed back uncommitted.
func (e *dedupeEnv) beginHeldAddressCreate(t *testing.T, name, email string) (pgx.Tx, ids.ContactID, int) {
	t.Helper()
	ctx := e.as()
	tx, err := e.store.db.Pool().Begin(ctx)
	if err != nil {
		t.Fatalf("open the in-flight create: %v", err)
	}
	t.Cleanup(func() {
		if err := tx.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			t.Errorf("release the in-flight create: %v", err)
		}
	})
	var pid int
	if err := tx.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&pid); err != nil {
		t.Fatalf("read the in-flight create's pid: %v", err)
	}
	if err := lockAddressBaseLane(ctx, tx, []string{email}); err != nil {
		t.Fatalf("take the base-address lock: %v", err)
	}
	id := ids.New[ids.ContactKind]()
	if _, err := tx.Exec(ctx,
		`INSERT INTO contact (id, full_name, source, captured_by) VALUES ($1, $2, 'manual', 'human:x')`,
		id, name); err != nil {
		t.Fatalf("insert the in-flight contact: %v", err)
	}
	if err := insertContactEmails(ctx, tx, ids.From[ids.WorkspaceKind](e.ws), id, "manual", "human:x",
		[]ContactEmailInput{{Email: email, EmailType: "work", IsPrimary: true}}); err != nil {
		t.Fatalf("insert the in-flight address: %v", err)
	}
	return tx, id, pid
}

// Two creates of ab@ and ab+x@ at once: the second waits for the first and
// files the pair, rather than both reading no incumbent.
func TestATaggedCreateRacingItsBaseStillFilesThePair(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	first, firstID, pid := e.beginHeldAddressCreate(t, "Ab", "ab@example.com")
	second := e.createInBackground(ctx, CreateContactInput{
		FullName: "Anna Beispiel", Source: "manual",
		Emails: []ContactEmailInput{{Email: "ab+x@example.com", EmailType: "work", IsPrimary: true}},
	})
	res, finished := waitUntilBlocked(t, first, pid, second)
	if err := first.Commit(ctx); err != nil {
		t.Fatalf("commit the first create: %v", err)
	}
	if !finished {
		res = <-second
	}
	if res.err != nil {
		t.Fatalf("the second create: %v", res.err)
	}
	pairs := openCandidatePairs(ctx, t, e, "contact", ids.UUID(res.contact.Id))
	if len(pairs) != 1 || pairs[0].OtherID != firstID.String() {
		t.Fatalf("got pairs %+v, want one pair with %s", pairs, firstID)
	}
}
