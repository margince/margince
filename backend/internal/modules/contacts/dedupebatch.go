// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// The ladder's answer about records a batch will create but has not written.
// A batch writer lands its rows one at a time, so its commit meets each earlier
// row as an incumbent. A dry run writes nothing and carries those rows itself.
// A member is a rival only when the ladder would both admit and score it. The
// scorers run in Go first, so a row with no scored member costs no query. The
// query's admission predicate then runs in Postgres, over the members kept.

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
)

// CompanyBatch is the companies a batch will create.
type CompanyBatch struct {
	members []batchCompany
	// The gate scores a pairing only when it shares a squashed name or a word
	// sameCompanyToken matches. A member is found by one or the other.
	bySquashed map[string][]int
	byToken    map[string][]int
	// tokensOfLength holds each distinct word under its rune length, since
	// sameCompanyToken matches only words within companyFuzzyTokenLengthSlack.
	tokensOfLength map[int][]string
	domains        map[string]bool
}

type batchCompany struct {
	display, legal string
	names          companyNamePair
}

// Add records a company the batch will create.
func (b *CompanyBatch) Add(c CompanyCandidate) {
	if b.domains == nil {
		b.bySquashed, b.byToken, b.tokensOfLength, b.domains = map[string][]int{}, map[string][]int{}, map[int][]string{}, map[string]bool{}
	}
	at := len(b.members)
	member := batchCompany{display: c.DisplayName, legal: c.LegalName, names: companyNamePairOf(c.DisplayName, c.LegalName)}
	for _, form := range member.names.forms() {
		if form.squashed != "" {
			b.bySquashed[form.squashed] = append(b.bySquashed[form.squashed], at)
		}
		for _, token := range form.tokens {
			if _, filed := b.byToken[token]; !filed {
				length := len([]rune(token))
				b.tokensOfLength[length] = append(b.tokensOfLength[length], token)
			}
			b.byToken[token] = append(b.byToken[token], at)
		}
	}
	b.members = append(b.members, member)
	for _, d := range c.Domains {
		b.domains[normalizeDomain(d)] = true
	}
}

func (p companyNamePair) forms() []companyNameForm {
	return []companyNameForm{p.display, p.legal}
}

// gateMatches finds the members filed under a key formsShareADistinctiveWord
// could match the candidate on.
func (b *CompanyBatch) gateMatches(candidate companyNamePair) []int {
	var matches []int
	for _, form := range candidate.forms() {
		if form.squashed != "" {
			matches = append(matches, b.bySquashed[form.squashed]...)
		}
		for _, token := range form.tokens {
			length := len([]rune(token))
			for near := length - companyFuzzyTokenLengthSlack; near <= length+companyFuzzyTokenLengthSlack; near++ {
				for _, filed := range b.tokensOfLength[near] {
					if sameCompanyToken(token, filed) {
						matches = append(matches, b.byToken[filed]...)
					}
				}
			}
		}
	}
	return matches
}

// CompanyRivals is the Go half of DedupeCompany's verdict on one candidate.
// It says whether the display name spells a member's domain, and which members
// the fuzzy scorer puts over the threshold.
type CompanyRivals struct {
	candidate CompanyCandidate
	spelled   bool
	scored    []batchCompany
}

// Rivals scores c against the members the gate may pair it with.
func (b *CompanyBatch) Rivals(c CompanyCandidate) CompanyRivals {
	if NormalizeCompanyName(c.DisplayName) == "" && NormalizeCompanyName(c.LegalName) == "" {
		return CompanyRivals{}
	}
	if spelled, ok := displayNameDomain(c.DisplayName); ok && b.domains[spelled] {
		return CompanyRivals{spelled: true}
	}
	candidate := companyNamePairOf(c.DisplayName, c.LegalName)
	rivals := CompanyRivals{candidate: c}
	seen := map[int]bool{}
	for _, at := range b.gateMatches(candidate) {
		if seen[at] {
			continue
		}
		seen[at] = true
		if _, rival := companyPairRival(candidate, b.members[at].names); rival {
			rivals.scored = append(rivals.scored, b.members[at])
		}
	}
	return rivals
}

// Settled reports whether Go alone decided, and then whether it found a rival.
// An unsettled verdict needs Admitted.
func (r CompanyRivals) Settled() (settled, rival bool) {
	return r.spelled || len(r.scored) == 0, r.spelled
}

// Admitted answers whether the fuzzy tier's query admits a scored member.
func (r CompanyRivals) Admitted(ctx context.Context, tx pgx.Tx) (bool, error) {
	if settled, rival := r.Settled(); settled {
		return rival, nil
	}
	displays := make([]string, 0, len(r.scored))
	legals := make([]string, 0, len(r.scored))
	for _, m := range r.scored {
		displays = append(displays, m.display)
		legals = append(legals, m.legal)
	}
	var args []any
	displaysAt, legalsAt := bindParam(&args, displays), bindParam(&args, legals)
	arms := companyAdmissionArms(&args, r.candidate)
	if len(arms) == 0 {
		return false, nil
	}
	// The columns carry fuzzyCompany's names, so its arms read these rows
	// unchanged. An empty legal name is NULL, as the stored column is.
	var admitted bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
		  SELECT 1 FROM (SELECT d AS display_name, NULLIF(l, '') AS legal_name
		                   FROM unnest(`+displaysAt+`::text[], `+legalsAt+`::text[]) AS m(d, l)) AS company
		   WHERE `+strings.Join(arms, " OR ")+`)`, args...).Scan(&admitted); err != nil {
		return false, fmt.Errorf("dedupe company batch admission: %w", err)
	}
	return admitted, nil
}

// bindParam appends value to args and answers its placeholder.
func bindParam[T any](args *[]any, value T) string {
	*args = append(*args, value)
	return "$" + strconv.Itoa(len(*args))
}

// ContactBatch is the contacts a batch will create, filed under the mail
// domains of their addresses.
//
// Filed by domain because a batch row carries no employer. A shared mail
// domain is then the only employer evidence between two rows, and a name alone
// cannot clear the threshold.
type ContactBatch struct {
	byDomain map[string][]batchContact
}

// batchContact is a member's name as given, and as the scorer compares it.
type batchContact struct {
	name, normalized string
}

// Add records a contact the batch will create. A consumer mail domain is not
// filed: the ladder never reads one as a shared employer.
func (b *ContactBatch) Add(c ContactCandidate) {
	if b.byDomain == nil {
		b.byDomain = map[string][]batchContact{}
	}
	member := batchContact{name: c.FullName, normalized: normalizeName(c.FullName)}
	for _, e := range c.Emails {
		if domain := emailDomain(e); domain != "" && sharedEmployerDomain(c, domain) {
			b.byDomain[domain] = append(b.byDomain[domain], member)
		}
	}
}

// ContactRivals is the Go half of the fuzzy tier's verdict on one candidate.
// It holds the members on a shared employer domain scored over the threshold.
type ContactRivals struct {
	candidate   ContactCandidate
	names       []string
	confidences []float64
}

// Rivals scores c against the members filed under its own mail domains.
func (b *ContactBatch) Rivals(c ContactCandidate) ContactRivals {
	rivals := ContactRivals{candidate: c}
	if NormalizeContactName(c.FullName) == "" {
		return rivals
	}
	name := normalizeName(c.FullName)
	for _, e := range c.Emails {
		domain := emailDomain(e)
		// Every member filed here shares only this domain with c, so the
		// employer term is one number for all of them.
		employer := companyMatch(c, contactCandidateRow{mailDomains: []string{domain}})
		for _, member := range b.byDomain[domain] {
			similarity := normalizedNameSimilarity(name, member.normalized)
			// Corroboration is the costlier test, and only a pair the employer
			// lifts over the bar needs it.
			if contactScore(similarity, employer, true) < dedupeReviewThreshold {
				continue
			}
			confidence := contactScore(similarity, employer, namesCorroborate(c.FullName, member.name))
			if confidence >= dedupeReviewThreshold {
				rivals.names = append(rivals.names, member.name)
				rivals.confidences = append(rivals.confidences, confidence)
			}
		}
	}
	return rivals
}

// Any reports whether a member was scored, so Best has a query to run.
func (r ContactRivals) Any() bool {
	return len(r.names) > 0
}

// Best is the fuzzy tier's confidence in the best scored member it admits,
// had that member been written. It is 0 when it admits none. An address two
// rows share is the exact tier's question, which the batch asks first.
func (r ContactRivals) Best(ctx context.Context, tx pgx.Tx) (float64, error) {
	if !r.Any() {
		return 0, nil
	}
	var args []any
	namesAt, candidateAt := bindParam(&args, r.names), bindParam(&args, r.candidate.FullName)
	rows, err := tx.Query(ctx, `
		SELECT ord FROM unnest(`+namesAt+`::text[]) WITH ORDINALITY AS p(full_name, ord)
		 WHERE `+contactNameAdmitsSQL("p.full_name", candidateAt), args...)
	if err != nil {
		return 0, fmt.Errorf("dedupe contact batch admission: %w", err)
	}
	admitted, err := pgx.CollectRows(rows, pgx.RowTo[int64])
	if err != nil {
		return 0, fmt.Errorf("dedupe contact batch admission: %w", err)
	}
	best := 0.0
	for _, ord := range admitted {
		best = max(best, r.confidences[ord-1])
	}
	return best, nil
}
