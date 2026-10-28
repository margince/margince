// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/freemail"
	"github.com/margince/margince/backend/internal/platform/owndomain"
	"github.com/margince/margince/backend/internal/shared/kernel/correspondence"
	"github.com/margince/margince/backend/internal/shared/kernel/employment"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// The dedupe parameters (PO-F-1/PO-F-2). Source constants, not runtime
// config: the spec's registry pins them outside the runtime-config
// boundary because a workspace tuning its own match threshold would make
// "no duplicates" unauditable across installations.
const (
	dedupeReviewThreshold     = 0.72
	dedupeNameWeight          = 0.55
	dedupeCompanyDomainWeight = 0.45
)

// The bounds on one record's check. Without them a name many records share
// files a pair against each of them, under the company-name lock. A scan of N
// such records would also read N² rows. dedupeRivalLimit is how many rivals the
// ladder names, best first. dedupeCandidateRows is how many rows one candidate
// query reads for scoring, most similar name first.
const (
	dedupeRivalLimit    = 5
	dedupeCandidateRows = 50
)

// DedupeDecision is the closed outcome set of PO-F-1/PO-F-2. Fuzzy never
// resolves itself: DEDUPE_FUZZY_AUTOMERGE is pinned *(never)*, so the
// only automatic resolutions are exact-key ones.
type DedupeDecision string

const (
	// DecisionExactCollision is a unique-key hit: same email, or same
	// company domain. Deterministic, no score. The caller's policy decides
	// whether that blocks (API) or lands on the incumbent (capture).
	DecisionExactCollision DedupeDecision = "exact_collision"
	// DecisionFuzzyReview is a near-match at or above the threshold, or a
	// company named after a domain another company claims: a human compares
	// the two records side by side. Never a merge.
	DecisionFuzzyReview DedupeDecision = "fuzzy_review"
	// DecisionNameCollisionReview is two records written with exactly the same
	// name and no key in common. Like the fuzzy tier it is a question for a
	// human and never a merge; unlike it, there is no probability involved —
	// the names either match or they do not.
	//
	// It exists because the fuzzy tier cannot reach this pair. A perfect name
	// contributes 0.55 of a 0.72 bar, so it clears only on employer agreement,
	// and the create path has no employer to agree with: CreateContactInput
	// carries none. One contact's second business card therefore landed as a new
	// record in silence — the case this lane was added for.
	DecisionNameCollisionReview DedupeDecision = "name_collision_review"
	// DecisionAddressVariantReview is two records whose addresses differ only
	// by a plus tag (dedupeaddressvariant.go). Like the name-collision lane it
	// creates anyway and files a pair; it never routes, because a tag can be
	// shared.
	DecisionAddressVariantReview DedupeDecision = "address_variant_review"
	// DecisionNoMatch means create.
	DecisionNoMatch DedupeDecision = "no_match"
)

// queuesReview reports whether a decision means "a second record, and a human
// compares the two". Every writer that files or names a review pair asks this,
// so a new review lane cannot be dropped by one writer's switch.
func (d DedupeDecision) queuesReview() bool {
	switch d {
	case DecisionFuzzyReview, DecisionNameCollisionReview, DecisionAddressVariantReview:
		return true
	default:
		return false
	}
}

// ContactCandidate is the input to PO-F-1 — the fields the formula reads,
// not a whole contact: a resolver that took CreateContactInput could not
// serve capture, promote, and the public booking surface alike.
type ContactCandidate struct {
	FullName string
	// Emails are checked in full against the exact tier; every email on
	// the candidate counts, not just the primary.
	Emails []string
	// Phones are E.164 keys for the phone exact lane; a number that does
	// not normalize is no key at all and is dropped by the lane.
	Phones []string
	// ChannelIdentities are the messaging-channel keys (provider +
	// channel user id) an inbound channel message arrives with.
	ChannelIdentities []connector.ChannelIdentity
	// ConsumerMail decides whether a shared mail domain says anything about a
	// shared EMPLOYER. Nil falls back to the shipped baseline, which is right
	// for every caller that has no transaction-scoped matcher to hand.
	//
	// The workspace's own list has to be able to reach here, and specifically
	// its carve-outs: a `never` entry says "this IS a company's domain,
	// whatever the shipped list claims", and judging it by the baseline alone
	// would keep dropping the employer agreement an admin has explicitly
	// asserted.
	ConsumerMail *freemail.Matcher
	// CurrentPrimaryCompanyID drives company_match = 1.0 when both sides share
	// an employer. Nil when the candidate has no known employer yet.
	CurrentPrimaryCompanyID *ids.CompanyID
	// QueueNameCollisions asks for the name-collision lane, and it is OPT-IN
	// because the answer it gives is only safe for one kind of caller.
	//
	// A caller that CREATES wants it: two records written the same way are a
	// question worth putting to a human, and the worst case is a queue row
	// somebody dismisses.
	//
	// A caller that ROUTES must not have it. Capture and the channel path ask
	// "which existing contact does this message belong to", and for them a
	// ContactID is a delivery address. An unbound channel identity carrying a
	// common name would name somebody it has no business naming, and a message
	// would land on a stranger's timeline — a data leak dressed as a match.
	//
	// The distinction cannot be read off the candidate: both callers arrive with
	// a name and no key. So it is stated by whoever knows what they will do with
	// the answer.
	QueueNameCollisions bool
	// ExcludeID leaves one contact out of the candidate set: the contact being
	// re-checked after a rename, which would otherwise match itself perfectly
	// and hide every real twin behind the self-score.
	ExcludeID *ids.ContactID
}

// ContactResolution is PO-F-1's output: the decision, the contact it names,
// and — when two exact lanes named DIFFERENT contacts — the rival that lost
// the routing decision.
//
// It is a result type rather than a field bolted onto a plain match,
// because a conflict has to be impossible to receive without noticing:
// a new field compiles cleanly at every existing call site, a new return
// type does not.
type ContactResolution struct {
	Decision   DedupeDecision
	ContactID  ids.ContactID
	Confidence float64
	// MatchedLane names the exact lane that routed the decision, and is
	// empty for every decision other than DecisionExactCollision. WHICH key
	// matched is part of the answer, because a caller's exact policy differs
	// per lane: a claimed address is the API create's 409, while a phone
	// number households and switchboards share cannot refuse anything.
	MatchedLane string
	// Conflict is non-nil only when a later exact lane resolved to a
	// different contact than the routed one. It is a REPORT: this resolver
	// writes nothing, and in particular never plants the routed lane's key
	// on the rival — preferring a lane for routing merges nobody, writing
	// keys across records is what merges contacts.
	Conflict *LaneConflict
	// Reviews holds the records the review lanes paired the candidate with, the
	// one named above first. Writers file each of them, and a reader that
	// shows one picks the first the caller may see (firstVisibleReview).
	Reviews []ReviewMatch
}

// DedupeContact is PO-F-1, the single contact-matching implementation —
// "one dedupe implementation, not two". It reads; it never writes and
// never merges. Callers map the decision onto their own policy.
func DedupeContact(ctx context.Context, tx pgx.Tx, c ContactCandidate) (ContactResolution, error) {
	lanes, err := exactLanes(ctx, tx, c)
	if err != nil {
		return ContactResolution{}, err
	}
	if res, routed := routeExact(lanes); routed {
		return res, nil
	}
	return reviewContact(ctx, tx, c)
}

// contactCandidateRow is one row of the restricted candidate set.
type contactCandidateRow struct {
	id        ids.ContactID
	fullName  string
	companyID *ids.CompanyID
	// companyDomain is a domain the incumbent's EMPLOYER is registered under;
	// mailDomains are the ones their own live addresses sit on. Both say "these
	// two work at the same place", and the second says it while the employer is
	// still an open question — which is where a captured counterparty starts.
	companyDomain *string
	mailDomains   []string
}

// fuzzyContact is PO-F-1 tier 2. The candidate set is restricted to
// contacts sharing a name trigram or the candidate's employer — the
// formula's own bound, so scoring stays inside the create budget instead
// of walking the workspace.
// WHY THE CANDIDATE QUERY HAS THREE ARMS. The trigram arm and the employer arm
// both narrow a set for a SCORE, and being approximate is fine there: a row they
// miss was never going to win. The name-collision lane is not a score — it
// decides on NormalizeContactName equality — so a prefilter that is merely
// approximate could hide a row that would have been exactly equal, and the
// employer arm cannot rescue it because a manual create carries no employer at
// all.
//
// The third arm folds BOTH sides with SQL's own functions, exactly as the
// trigram arm does. Computing one side in Go would move the divergence rather
// than close it: SQL's lower+unaccent and Go's Unicode full folding are two
// different normalizations, and the point is to stop relying on either being a
// superset of the other. It is exactNameKeySQL, which spells the two
// properties NormalizeContactName has and a bare SQL comparison does not: the
// trim, and the internal-whitespace collapse. Both are real divergences —
// "  Lucy Vo  " and "Lucy Vo" are Go-equal, and so are "Éva  Ő" and "Éva Ő",
// and a name reflowed across a line break is how the second one arrives.
//
// NO KNOWN PAIR NEEDS THAT ARM. Every case that could be constructed — case,
// accents, ß, a trailing space — is already admitted by the trigram arm, and
// TestEveryNameGoCallsEqualReachesTheNameLane says so by passing without it. It
// is a GUARANTEE rather than a bug fix: the trigram operator is a similarity
// threshold, and this lane's correctness should not rest on one approximate
// predicate happening to cover another normalization's output.
//
//nolint:cyclop // the rename added no branch: this body is what it was under the old noun.
func fuzzyContact(ctx context.Context, tx pgx.Tx, c ContactCandidate) (ContactResolution, error) {
	// candidate gathers the ids each arm of the `WHERE` below can name, one
	// indexed read per arm. The arms are joined with `OR` across contact and
	// relationship, which no index answers. Without candidate, every read
	// scores every live contact. The `WHERE` still decides; candidate only
	// bounds what it reads.
	rows, err := tx.Query(ctx, `
		WITH candidate AS (
		  SELECT p.id FROM contact p
		   WHERE p.archived_at IS NULL AND f_fold_apostrophes(lower(p.full_name)) % f_fold_apostrophes(lower($1))
		  UNION
		  SELECT p.id FROM contact p
		   WHERE p.archived_at IS NULL AND `+exactNameKeySQL("p.full_name")+` = `+exactNameKeySQL("$1")+`
		  UNION
		  SELECT p.id FROM contact p
		   WHERE p.archived_at IS NULL AND `+exactDigraphNameKeySQL("p.full_name")+` = `+exactDigraphNameKeySQL("$1")+`
		  UNION
		  SELECT r.contact_id FROM relationship r
		   WHERE $2::uuid IS NOT NULL AND r.company_id = $2 AND r.kind = 'employment'
		     AND `+employment.CurrentPrimarySQL("r")+` AND r.archived_at IS NULL)
		SELECT p.id, p.full_name, r.company_id, od.domain,
		       -- EVERY live address, not an unordered LIMIT 1: a contact with a
		       -- work and a personal address has two primaries as far as this
		       -- query is concerned, and picking either at random would make
		       -- the employer term depend on the plan Postgres chose. Archived
		       -- addresses are excluded — an address somebody stopped using is
		       -- not evidence of where they work now.
		       (SELECT array_agg(pe.email)
		          FROM contact_email pe
		         WHERE pe.contact_id = p.id AND pe.archived_at IS NULL)
		  FROM contact p
		  LEFT JOIN relationship r
		    ON r.contact_id = p.id AND r.kind = 'employment'
		   AND `+employment.CurrentPrimarySQL("r")+` AND r.archived_at IS NULL
		  LEFT JOIN company_domain od
		    ON od.company_id = r.company_id AND od.archived_at IS NULL
		 WHERE p.archived_at IS NULL
		   AND p.id IN (SELECT id FROM candidate)
		   AND ($3::uuid IS NULL OR p.id <> $3)
		   AND (f_fold_apostrophes(lower(p.full_name)) % f_fold_apostrophes(lower($1))
		        OR ($2::uuid IS NOT NULL AND r.company_id = $2)
		        OR `+exactNameKeySQL("p.full_name")+` = `+exactNameKeySQL("$1")+`
		        OR `+exactDigraphNameKeySQL("p.full_name")+` = `+exactDigraphNameKeySQL("$1")+`)
		 ORDER BY similarity(f_fold_apostrophes(lower(p.full_name)), f_fold_apostrophes(lower($1))) DESC, p.id
		 LIMIT $4`,
		c.FullName, c.CurrentPrimaryCompanyID, c.ExcludeID, dedupeCandidateRows)
	if err != nil {
		return ContactResolution{}, fmt.Errorf("dedupe contact candidate set: %w", err)
	}
	defer rows.Close()

	// One contact can come back once per employer domain, so each keeps its
	// best score before it is judged.
	scored := map[ids.ContactID]ReviewMatch{}
	for rows.Next() {
		var row contactCandidateRow
		var addresses []string
		if err := rows.Scan(&row.id, &row.fullName, &row.companyID, &row.companyDomain, &addresses); err != nil {
			return ContactResolution{}, fmt.Errorf("scan contact candidate: %w", err)
		}
		row.mailDomains = mailDomainsOf(addresses)
		match := ReviewMatch{ContactID: row.id, Confidence: contactConfidence(c, row)}
		switch {
		case match.Confidence >= dedupeReviewThreshold:
			match.Decision = DecisionFuzzyReview
		case c.QueueNameCollisions && contactNamesCollide(c.FullName, row.fullName):
			// The name-collision lane: two contacts written the same way are
			// worth a human's glance even when nothing else agrees. A perfect
			// name scores 0.55 of a 0.72 bar, and a create carries no employer,
			// so the fuzzy tier cannot reach this pair. It flags and never
			// merges: a father and son at one firm are two real records.
			match.Decision = DecisionNameCollisionReview
		default:
			continue
		}
		if held, seen := scored[row.id]; !seen || match.Confidence > held.Confidence ||
			(held.Decision != DecisionFuzzyReview && match.Decision == DecisionFuzzyReview) {
			scored[row.id] = match
		}
	}
	if err := rows.Err(); err != nil {
		return ContactResolution{}, fmt.Errorf("drain contact candidates: %w", err)
	}
	return resolutionOf(rankedReviews(scored)), nil
}

// contactConfidence is the PO-F-1 score: weights sum to 1.0, so the
// result is in [0,1] and comparable against the threshold directly.
func contactConfidence(c ContactCandidate, row contactCandidateRow) float64 {
	name := dedupeNameWeight * contactNameSimilarity(c.FullName, row.fullName)
	if !namesCorroborate(c.FullName, row.fullName) {
		return name
	}
	return name + dedupeCompanyDomainWeight*companyMatch(c, row)
}

// companyMatch is PO-F-1's employer agreement term, most-specific first: a shared
// employer row beats a shared company domain, which beats two addresses simply
// sitting on the same domain.
//
// That last rung is what keeps the term alive for a captured counterparty.
// Capture creates the contact and withholds the company until a site read judges
// the domain, so for the whole time that question is open there is no employer
// row and no company_domain to agree about — and two colleagues at a new
// customer would stop meeting at the fuzzy tier just when their records are
// newest and most likely to be twins.
func companyMatch(c ContactCandidate, row contactCandidateRow) float64 {
	if c.CurrentPrimaryCompanyID != nil && row.companyID != nil && *c.CurrentPrimaryCompanyID == *row.companyID {
		return 1.0
	}
	if row.companyDomain != nil && candidateSharesDomain(c, *row.companyDomain) {
		return 0.8
	}
	for _, domain := range row.mailDomains {
		if sharedEmployerDomain(c, domain) {
			return 0.8
		}
	}
	return 0.0
}

// sharedEmployerDomain reports whether two addresses sitting on the same mail
// domain says anything about a shared EMPLOYER. On a consumer mailbox provider
// it says nothing at all — two contacts at gmail.com share a mail host, not a
// job — and scoring it would put every same-named pair of private addresses in
// the review queue, which is exactly where "same domain" carries least signal.
//
// The candidate's own matcher decides when it carries one, so a workspace that
// has corrected the shipped list is obeyed here as well as in capture — its
// carve-outs are assertions that a domain IS an employer's, and honouring them
// only on the capture side would leave dedupe dropping evidence the admin
// deliberately restored.
func sharedEmployerDomain(c ContactCandidate, domain string) bool {
	matcher := c.ConsumerMail
	if matcher == nil {
		matcher = consumerMailBaseline
	}
	return candidateSharesDomain(c, domain) && !matcher.IsConsumer(domain)
}

// consumerMailBaseline is the shipped list with no workspace overlay — the
// answer for a caller that built no matcher.
var consumerMailBaseline = freemail.New(nil, nil)

// candidateSharesDomain reports whether any candidate email sits under the
// same registrable domain as the incumbent's: `cloud.example.com` and
// `example.com` are one employer's. freemail.Registrable keeps hosting
// suffixes such as github.io distinct per tenant. The consumer-mail check in
// sharedEmployerDomain walks a subdomain up to its registrable domain, so a
// subdomain of gmail.com still says nothing about an employer.
func candidateSharesDomain(c ContactCandidate, domain string) bool {
	want := freemail.Registrable(normalizeDomain(domain))
	if want == "" {
		return false
	}
	for _, e := range c.Emails {
		if freemail.Registrable(emailDomain(e)) == want {
			return true
		}
	}
	return false
}

// normalizeEmail matches how contact_email stores the address: the insert
// path lowercases on write, so the exact tier compares like for like.
func normalizeEmail(e string) string { return correspondence.Fold(e) }

// normalizeDomain matches company_domain's storage contract:
// lowercase only — never unaccent, or münich.example would collide with
// a different company's munich.example.
func normalizeDomain(d string) string { return strings.ToLower(strings.TrimSpace(d)) }

// emailDomain returns the lowercased host of an address, or "" when the
// input carries no host to compare.
func emailDomain(e string) string { return owndomain.OfAddress(normalizeEmail(e)) }

// mailDomainsOf is the distinct folded domains of a contact's addresses, in
// the order they were read.
func mailDomainsOf(addresses []string) []string {
	var domains []string
	for _, address := range addresses {
		if domain := emailDomain(address); domain != "" && !slices.Contains(domains, domain) {
			domains = append(domains, domain)
		}
	}
	return domains
}
