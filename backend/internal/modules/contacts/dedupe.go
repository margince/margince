// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/freemail"
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

// DedupeDecision is the closed outcome set of PO-F-1/PO-F-2. Fuzzy never
// resolves itself: DEDUPE_FUZZY_AUTOMERGE is pinned *(never)*, so the
// only automatic resolutions are exact-key ones.
type DedupeDecision string

const (
	// DecisionExactCollision is a unique-key hit: same email, or same
	// company domain. Deterministic, no score. The caller's policy decides
	// whether that blocks (API) or lands on the incumbent (capture).
	DecisionExactCollision DedupeDecision = "exact_collision"
	// DecisionFuzzyReview is a near-match at or above the threshold: a
	// human compares the two records side by side. Never a merge.
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
	// DecisionNoMatch means create.
	DecisionNoMatch DedupeDecision = "no_match"
)

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
}

// LaneConflict names both sides of an exact-lane disagreement and which
// lane spoke for each, so the caller's policy has the evidence it needs
// without re-running the ladder.
type LaneConflict struct {
	RoutedTo, Rival       ids.ContactID
	RoutedLane, RivalLane string
}

// The exact lanes, named for LaneConflict's evidence. Ladder order is
// routing precedence: an established channel binding outranks a shared
// address, which outranks a phone number households and switchboards
// share.
//
// LaneEmail alone is exported, and only because the published
// extension.MergeKeyEmail must equal it: a source declares that key to have an
// address reach this lane, and a fitness test outside this package reads both to
// hold them equal. The other two name no vocabulary beyond this module.
const (
	laneChannelIdentity = "channel_identity"
	LaneEmail           = "email"
	lanePhone           = "phone"
)

// exactLane is one lane's answer, in ladder order.
type exactLane struct {
	name      string
	contactID ids.ContactID
	found     bool
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
	// A nameless captured contact never fuzzy-matches: with no name there
	// is nothing to score, and company_match alone would collide every
	// colleague onto one record.
	if NormalizeContactName(c.FullName) == "" {
		return ContactResolution{Decision: DecisionNoMatch}, nil
	}
	return fuzzyContact(ctx, tx, c)
}

// exactLanes runs every exact lane, in ladder order. All of them run even
// once one has hit: a disagreement between two lanes is itself an answer
// the caller needs, and only the rival lanes can report it. A lane whose
// candidate keys are empty costs no query.
func exactLanes(ctx context.Context, tx pgx.Tx, c ContactCandidate) ([]exactLane, error) {
	channelHit, channelFound, err := exactContactByChannelIdentity(ctx, tx, c.ChannelIdentities)
	if err != nil {
		return nil, err
	}
	emailHit, emailFound, err := exactContactByEmail(ctx, tx, c.Emails)
	if err != nil {
		return nil, err
	}
	phoneHit, phoneFound, err := exactContactByPhone(ctx, tx, c.Phones)
	if err != nil {
		return nil, err
	}
	return []exactLane{
		{laneChannelIdentity, channelHit, channelFound},
		{LaneEmail, emailHit, emailFound},
		{lanePhone, phoneHit, phoneFound},
	}, nil
}

// routeExact picks the routed contact deterministically — the first lane
// that hit — and reports the first later lane that named someone else.
// Routing is immediate and never deferred to a human: a message with
// nowhere to land is worse than a message on the record whose binding was
// established first.
func routeExact(lanes []exactLane) (ContactResolution, bool) {
	for i, lane := range lanes {
		if !lane.found {
			continue
		}
		res := ContactResolution{Decision: DecisionExactCollision, ContactID: lane.contactID, MatchedLane: lane.name}
		for _, rival := range lanes[i+1:] {
			if rival.found && rival.contactID != lane.contactID {
				res.Conflict = &LaneConflict{
					RoutedTo: lane.contactID, Rival: rival.contactID,
					RoutedLane: lane.name, RivalLane: rival.name,
				}
				break
			}
		}
		return res, true
	}
	return ContactResolution{}, false
}

// exactContactByEmail is PO-F-1 tier 1. Every candidate email is checked;
// the lowest contact id wins so a candidate colliding on two emails
// against two contacts resolves the same way on every run.
func exactContactByEmail(ctx context.Context, tx pgx.Tx, emails []string) (ids.ContactID, bool, error) {
	if len(emails) == 0 {
		return ids.ContactID{}, false, nil
	}
	lowered := make([]string, 0, len(emails))
	for _, e := range emails {
		lowered = append(lowered, normalizeEmail(e))
	}
	var id ids.ContactID
	err := tx.QueryRow(ctx, `
		SELECT contact_id FROM contact_email
		WHERE email = ANY($1) AND archived_at IS NULL
		ORDER BY contact_id
		LIMIT 1`, lowered).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return ids.ContactID{}, false, nil
	}
	if err != nil {
		return ids.ContactID{}, false, fmt.Errorf("dedupe contact exact tier: %w", err)
	}
	return id, true, nil
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
// superset of the other. It is contactNameKeySQL, which spells the two
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
	rows, err := tx.Query(ctx, `
		SELECT p.id, p.full_name, r.company_id, od.domain,
		       -- EVERY live address, not an unordered LIMIT 1: a contact with a
		       -- work and a personal address has two primaries as far as this
		       -- query is concerned, and picking either at random would make
		       -- the employer term depend on the plan Postgres chose. Archived
		       -- addresses are excluded — an address somebody stopped using is
		       -- not evidence of where they work now.
		       (SELECT array_agg(DISTINCT split_part(pe.email, '@', 2))
		          FROM contact_email pe
		         WHERE pe.contact_id = p.id AND pe.archived_at IS NULL)
		  FROM contact p
		  LEFT JOIN relationship r
		    ON r.contact_id = p.id AND r.kind = 'employment'
		   AND `+employment.CurrentPrimarySQL("r")+` AND r.archived_at IS NULL
		  LEFT JOIN company_domain od
		    ON od.company_id = r.company_id AND od.archived_at IS NULL
		 WHERE p.archived_at IS NULL
		   AND (f_fold_apostrophes(lower(p.full_name)) % f_fold_apostrophes(lower($1))
		        OR ($2::uuid IS NOT NULL AND r.company_id = $2)
		        OR `+contactNameKeySQL("p.full_name")+` = `+contactNameKeySQL("$1")+`)`,
		c.FullName, c.CurrentPrimaryCompanyID)
	if err != nil {
		return ContactResolution{}, fmt.Errorf("dedupe contact candidate set: %w", err)
	}
	defer rows.Close()

	best := ContactResolution{Decision: DecisionNoMatch}
	// The best name-IDENTICAL row, tracked apart from the best-scoring one
	// because they are not the same question and the winner of one is often not
	// the winner of the other: a near-name at a matching employer outscores an
	// exact name with no employer known, so reading the name lane off `best`
	// would lose exactly the pair it exists to catch.
	sameName := ContactResolution{Decision: DecisionNoMatch}
	candidateKey := NormalizeContactName(c.FullName)
	for rows.Next() {
		var row contactCandidateRow
		if err := rows.Scan(&row.id, &row.fullName, &row.companyID, &row.companyDomain, &row.mailDomains); err != nil {
			return ContactResolution{}, fmt.Errorf("scan contact candidate: %w", err)
		}
		confidence := contactConfidence(c, row)
		// Equal confidence resolves to the lowest contact id — a total
		// order, so the queue does not shuffle between runs.
		if confidence > best.Confidence ||
			(confidence == best.Confidence && best.ContactID != (ids.ContactID{}) && row.id.String() < best.ContactID.String()) {
			best.Confidence, best.ContactID = confidence, row.id
		}
		if NormalizeContactName(row.fullName) == candidateKey {
			// Same tie-break as above, and for the same reason: two incumbents
			// spelled identically must not shuffle the queue between runs.
			if confidence > sameName.Confidence ||
				(sameName.ContactID == (ids.ContactID{})) ||
				(confidence == sameName.Confidence && row.id.String() < sameName.ContactID.String()) {
				sameName.Confidence, sameName.ContactID = confidence, row.id
			}
		}
	}
	if err := rows.Err(); err != nil {
		return ContactResolution{}, fmt.Errorf("drain contact candidates: %w", err)
	}
	if best.Confidence >= dedupeReviewThreshold {
		best.Decision = DecisionFuzzyReview
		return best, nil
	}
	// THE NAME-COLLISION LANE. Two contacts in one workspace written exactly the
	// same way are worth a human's glance even when nothing else agrees.
	//
	// It is a lane of its own rather than a lower threshold, and the difference
	// is not cosmetic: the weights are shared with company matching, so
	// moving the bar to admit this pair would drag every company comparison down
	// with it. An exact name is also not a probability — it either is the same
	// string or it is not — so scoring it and comparing against a fuzzy bar was
	// always the wrong instrument.
	//
	// WHY IT WAS UNREACHABLE. A perfect name scores 0.55·1.0 and the bar is
	// 0.72, so the pair could only clear it on employer agreement. But
	// CreateContactInput carries no employer at all — the employment edge is a
	// separate call made after the contact exists — so at create time the company
	// term is structurally 0, and a second business card for someone already in
	// the workspace was created in silence every time.
	//
	// IT FLAGS, IT NEVER MERGES AND NEVER REFUSES. A father and son at one firm
	// are a real pair of records that share a name and an employer, and an
	// automatic rule that merged them would destroy data no undo restores. So
	// this is a question put to a human, which is the same shape the phone lane
	// already takes for a shared switchboard.
	if c.QueueNameCollisions && sameName.ContactID != (ids.ContactID{}) {
		sameName.Decision = DecisionNameCollisionReview
		return sameName, nil
	}
	return ContactResolution{Decision: DecisionNoMatch}, nil
}

// contactConfidence is the PO-F-1 score: weights sum to 1.0, so the
// result is in [0,1] and comparable against the threshold directly.
func contactConfidence(c ContactCandidate, row contactCandidateRow) float64 {
	return dedupeNameWeight*nameSimilarity(c.FullName, row.fullName) +
		dedupeCompanyDomainWeight*companyMatch(c, row)
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

// candidateSharesDomain reports whether any candidate email sits on an
// company domain the incumbent is mapped to.
func candidateSharesDomain(c ContactCandidate, domain string) bool {
	for _, e := range c.Emails {
		if emailDomain(e) == normalizeDomain(domain) {
			return true
		}
	}
	return false
}

// normalizeEmail matches how contact_email stores the address: the insert
// path lowercases on write, so the exact tier compares like for like.
func normalizeEmail(e string) string { return strings.ToLower(strings.TrimSpace(e)) }

// normalizeDomain matches company_domain's storage contract:
// lowercase only — never unaccent, or münich.example would collide with
// a different company's munich.example.
func normalizeDomain(d string) string { return strings.ToLower(strings.TrimSpace(d)) }

// emailDomain returns the lowercased host of an address, or "" when the
// input carries no host to compare.
func emailDomain(e string) string {
	at := strings.LastIndex(normalizeEmail(e), "@")
	if at < 0 {
		return ""
	}
	return normalizeEmail(e)[at+1:]
}
