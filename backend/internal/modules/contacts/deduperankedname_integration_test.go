// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestSharedTrigramsKeepTheCandidateLimitAndItsIdOrder(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	e.plainContact(ctx, t, "Someone Else", "other@ranked.test")
	contacts := seedRankedContacts(t, e, "Mara Example", "Example Mara")
	analyzeAsOwner(t, "contact", "contact_email")
	candidate := ContactCandidate{FullName: "Mara Example"}
	check := func(want []ids.UUID) {
		t.Helper()
		var got []ids.UUID
		if err := e.store.tx(ctx, func(tx pgx.Tx) error {
			rows, err := tx.Query(ctx, contactCandidateQuery, contactCandidateArgs(candidate))
			if err != nil {
				return err
			}
			got, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (ids.UUID, error) {
				var id ids.UUID
				var name string
				var company *ids.UUID
				var domain *string
				var addresses []string
				err := row.Scan(&id, &name, &company, &domain, &addresses)
				return id, err
			})
			return err
		}); err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(got, want) {
			t.Fatalf("candidate ids=%v want=%v", got, want)
		}
	}
	check(contacts[:dedupeCandidateRows])
	assertRankedNamePlans(t, e, candidate)
	excluded := ids.From[ids.ContactKind](contacts[0])
	candidate.ExcludeID = &excluded
	check(contacts[1 : dedupeCandidateRows+1])
	assertRankedNamePlans(t, e, candidate)
	candidate.ExcludeID = nil
	for _, id := range contacts[:10] {
		if _, err := e.store.ArchiveContact(ctx, ids.From[ids.ContactKind](id), nil); err != nil {
			t.Fatal(err)
		}
	}
	check(contacts[10:])
	excluded = ids.From[ids.ContactKind](contacts[10])
	candidate.ExcludeID = &excluded
	check(contacts[11:])
	near := e.plainContact(ctx, t, "Marie Example", "near@ranked.test")
	check(append(slices.Clone(contacts[11:]), near))
}

func assertRankedNamePlans(t *testing.T, e *dedupeEnv, candidate ContactCandidate) {
	t.Helper()
	ctx := e.as()
	for _, generic := range []bool{false, true} {
		var raw []byte
		err := e.store.tx(ctx, func(tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, `SET LOCAL enable_seqscan = off`); err != nil {
				return err
			}
			args := contactCandidateArgs(candidate)
			if !generic {
				return tx.QueryRow(ctx, `EXPLAIN (ANALYZE, FORMAT JSON) `+contactCandidateQuery, args).Scan(&raw)
			}
			query, values, err := args.RewriteQuery(ctx, tx.Conn(), contactCandidateQuery, nil)
			if err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `SET LOCAL plan_cache_mode=force_generic_plan`); err != nil {
				return err
			}
			statement := pgx.Identifier{"ranked_" + ids.NewV7().String()}.Sanitize()
			if _, err := tx.Exec(ctx, `PREPARE `+statement+` AS `+query); err != nil {
				return err
			}
			planQuery := `EXPLAIN (ANALYZE,FORMAT JSON) EXECUTE ` + statement + `(` + storekit.Placeholders(values) + `)`
			planErr := tx.QueryRow(ctx, planQuery, append([]any{pgx.QueryExecModeSimpleProtocol}, values...)...).Scan(&raw)
			_, closeErr := tx.Exec(ctx, `DEALLOCATE `+statement)
			return errors.Join(planErr, closeErr)
		})
		if err != nil {
			t.Fatalf("planning name candidates (generic=%v): %v", generic, err)
		}
		var plans []struct {
			Plan mailboxPlanNode `json:"Plan"` //nolint:tagliatelle // Postgres's EXPLAIN key.
		}
		if err := json.Unmarshal(raw, &plans); err != nil || len(plans) != 1 {
			t.Fatalf("reading candidate plan: %v", err)
		}
		if !plans[0].Plan.usesIndex("idx_contact_name_trigram_candidates") {
			t.Fatalf("name seek index missing (generic=%v): %s", generic, raw)
		}
		if plans[0].Plan.ActualRows != dedupeCandidateRows {
			t.Fatalf("name candidates returned %d rows (generic=%v), want %d", plans[0].Plan.ActualRows, generic, dedupeCandidateRows)
		}
		if !plans[0].Plan.usesIndex("idx_contact_email_contact") {
			t.Fatalf("candidate addresses did not seek their contact (generic=%v): %s", generic, raw)
		}
		if !plans[0].Plan.boundsRankedContacts() {
			t.Fatalf("name candidates read beyond the bounded prefix: %s", raw)
		}
	}
}

func (n mailboxPlanNode) boundsRankedContacts() bool {
	if n.RelationName == "contact" && n.ActualLoops > 0 {
		if n.IndexName != "idx_contact_name_trigram_candidates" && n.IndexName != "contact_pkey" {
			return false
		}
		if n.ActualRows*n.ActualLoops > dedupeCandidateRows+1 {
			return false
		}
	}
	for _, child := range n.Plans {
		if !child.boundsRankedContacts() {
			return false
		}
	}
	return true
}

func seedRankedContacts(t *testing.T, e *dedupeEnv, names ...string) []ids.UUID {
	t.Helper()
	var contacts []ids.UUID
	for i := range dedupeCandidateRows + 10 {
		contacts = append(contacts, e.plainContact(e.as(), t, names[i%len(names)], fmt.Sprintf("contact-%d@ranked.test", i)))
	}
	slices.SortFunc(contacts, func(a, b ids.UUID) int { return cmp.Compare(a.String(), b.String()) })
	return contacts
}

func TestRankedNamesPreserveReviewConfidenceAndTheRivalLimit(t *testing.T) {
	for _, name := range []string{"Björn Example", "O'Hara Example"} {
		t.Run(name, func(t *testing.T) {
			e := setupDedupe(t)
			if name == "Björn Example" {
				e.plainContact(e.as(), t, "Bjoern Example", "earlier@ranked.test")
			}
			contacts := seedRankedContacts(t, e, name, strings.ToUpper(name))
			candidate := ContactCandidate{FullName: name, Emails: []string{"new@ranked.test"}, QueueNameCollisions: true}
			if err := e.store.tx(e.as(), func(tx pgx.Tx) error {
				got, err := DedupeContact(e.as(), tx, candidate)
				if err != nil {
					return err
				}
				if len(got.Reviews) != dedupeRivalLimit {
					t.Fatalf("reviews=%d want=%d", len(got.Reviews), dedupeRivalLimit)
				}
				for i, review := range got.Reviews {
					if review.ContactID.UUID != contacts[i] || review.Decision != DecisionFuzzyReview || math.Abs(review.Confidence-0.91) > 1e-9 {
						t.Fatalf("review %d=%+v want contact=%s confidence=0.91", i, review, contacts[i])
					}
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestEmployerDomainsHaveAStableOrderAtTheCandidateCut(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	contacts := seedRankedContacts(t, e, "Mara Example")
	createEmployer := func(name string, domains ...string) ids.CompanyID {
		t.Helper()
		input := CreateCompanyInput{DisplayName: name, Source: "manual"}
		for _, domain := range domains {
			input.Domains = append(input.Domains, CompanyDomainInput{Domain: domain})
		}
		company, err := e.store.CreateCompany(ctx, input)
		if err != nil {
			t.Fatal(err)
		}
		return ids.From[ids.CompanyKind](ids.UUID(company.Id))
	}
	earlier := createEmployer("Earlier employer", "earlier-a.test", "earlier-b.test")
	cutEmployer := createEmployer("Cut employer", "cut-z.test", "cut-b.test", "cut-a.test")
	before := (dedupeCandidateRows - 2) / 2
	for _, contact := range contacts[:before] {
		employ(t, e, ids.From[ids.ContactKind](contact), earlier)
	}
	cut := ids.From[ids.ContactKind](contacts[before])
	employ(t, e, cut, cutEmployer)
	for _, test := range []struct {
		domain     string
		first      ids.UUID
		confidence float64
	}{
		{"cut-b.test", cut.UUID, 0.91},
		{"cut-z.test", contacts[0], 0.55},
	} {
		candidate := ContactCandidate{FullName: "Mara Example", Emails: []string{"new@" + test.domain}, QueueNameCollisions: true}
		if err := e.store.tx(ctx, func(tx pgx.Tx) error {
			got, err := DedupeContact(ctx, tx, candidate)
			if err != nil {
				return err
			}
			if len(got.Reviews) != dedupeRivalLimit || got.Reviews[0].ContactID.UUID != test.first || math.Abs(got.Reviews[0].Confidence-test.confidence) > 1e-9 {
				t.Fatalf("domain %s reviews=%+v want first=%s confidence=%f", test.domain, got.Reviews, test.first, test.confidence)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
}
