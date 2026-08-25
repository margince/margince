// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/gradionhq/margince/backend/internal/platform/freemail"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
	"github.com/gradionhq/margince/backend/internal/shared/ports/connector"
)

// The dedupe parameters (PO-F-1/PO-F-2). Source constants, not runtime
// config: the spec's registry pins them outside the runtime-config
// boundary because a workspace tuning its own match threshold would make
// "no duplicates" unauditable across installations.
const (
	dedupeReviewThreshold = 0.72
	dedupeNameWeight      = 0.55
	dedupeOrgDomainWeight = 0.45
)

// DedupeDecision is the closed outcome set of PO-F-1/PO-F-2. Fuzzy never
// resolves itself: DEDUPE_FUZZY_AUTOMERGE is pinned *(never)*, so the
// only automatic resolutions are exact-key ones.
type DedupeDecision string

const (
	// DecisionExactCollision is a unique-key hit: same email, or same
	// org domain. Deterministic, no score. The caller's policy decides
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
	// and the create path has no employer to agree with: CreatePersonInput
	// carries none. One person's second business card therefore landed as a new
	// record in silence — the case this lane was added for.
	DecisionNameCollisionReview DedupeDecision = "name_collision_review"
	// DecisionNoMatch means create.
	DecisionNoMatch DedupeDecision = "no_match"
)

// PersonCandidate is the input to PO-F-1 — the fields the formula reads,
// not a whole person: a resolver that took CreatePersonInput could not
// serve capture, promote, and the public booking surface alike.
type PersonCandidate struct {
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
	// CurrentPrimaryOrgID drives org_match = 1.0 when both sides share
	// an employer. Nil when the candidate has no known employer yet.
	CurrentPrimaryOrgID *ids.OrganizationID
	// QueueNameCollisions asks for the name-collision lane, and it is OPT-IN
	// because the answer it gives is only safe for one kind of caller.
	//
	// A caller that CREATES wants it: two records written the same way are a
	// question worth putting to a human, and the worst case is a queue row
	// somebody dismisses.
	//
	// A caller that ROUTES must not have it. Capture and the channel path ask
	// "which existing person does this message belong to", and for them a
	// PersonID is a delivery address. An unbound channel identity carrying a
	// common name would name somebody it has no business naming, and a message
	// would land on a stranger's timeline — a data leak dressed as a match.
	//
	// The distinction cannot be read off the candidate: both callers arrive with
	// a name and no key. So it is stated by whoever knows what they will do with
	// the answer.
	QueueNameCollisions bool
}

// PersonResolution is PO-F-1's output: the decision, the person it names,
// and — when two exact lanes named DIFFERENT people — the rival that lost
// the routing decision.
//
// It is a result type rather than a field bolted onto a plain match,
// because a conflict has to be impossible to receive without noticing:
// a new field compiles cleanly at every existing call site, a new return
// type does not.
type PersonResolution struct {
	Decision   DedupeDecision
	PersonID   ids.PersonID
	Confidence float64
	// MatchedLane names the exact lane that routed the decision, and is
	// empty for every decision other than DecisionExactCollision. WHICH key
	// matched is part of the answer, because a caller's exact policy differs
	// per lane: a claimed address is the API create's 409, while a phone
	// number households and switchboards share cannot refuse anything.
	MatchedLane string
	// Conflict is non-nil only when a later exact lane resolved to a
	// different person than the routed one. It is a REPORT: this resolver
	// writes nothing, and in particular never plants the routed lane's key
	// on the rival — preferring a lane for routing merges nobody, writing
	// keys across records is what merges people.
	Conflict *LaneConflict
}

// LaneConflict names both sides of an exact-lane disagreement and which
// lane spoke for each, so the caller's policy has the evidence it needs
// without re-running the ladder.
type LaneConflict struct {
	RoutedTo, Rival       ids.PersonID
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
	name     string
	personID ids.PersonID
	found    bool
}

// DedupePerson is PO-F-1, the single person-matching implementation —
// "one dedupe implementation, not two". It reads; it never writes and
// never merges. Callers map the decision onto their own policy.
func DedupePerson(ctx context.Context, tx pgx.Tx, c PersonCandidate) (PersonResolution, error) {
	lanes, err := exactLanes(ctx, tx, c)
	if err != nil {
		return PersonResolution{}, err
	}
	if res, routed := routeExact(lanes); routed {
		return res, nil
	}
	// A nameless captured contact never fuzzy-matches: with no name there
	// is nothing to score, and org_match alone would collide every
	// colleague onto one record.
	if NormalizePersonName(c.FullName) == "" {
		return PersonResolution{Decision: DecisionNoMatch}, nil
	}
	return fuzzyPerson(ctx, tx, c)
}

// exactLanes runs every exact lane, in ladder order. All of them run even
// once one has hit: a disagreement between two lanes is itself an answer
// the caller needs, and only the rival lanes can report it. A lane whose
// candidate keys are empty costs no query.
func exactLanes(ctx context.Context, tx pgx.Tx, c PersonCandidate) ([]exactLane, error) {
	channelHit, channelFound, err := exactPersonByChannelIdentity(ctx, tx, c.ChannelIdentities)
	if err != nil {
		return nil, err
	}
	emailHit, emailFound, err := exactPersonByEmail(ctx, tx, c.Emails)
	if err != nil {
		return nil, err
	}
	phoneHit, phoneFound, err := exactPersonByPhone(ctx, tx, c.Phones)
	if err != nil {
		return nil, err
	}
	return []exactLane{
		{laneChannelIdentity, channelHit, channelFound},
		{LaneEmail, emailHit, emailFound},
		{lanePhone, phoneHit, phoneFound},
	}, nil
}

// routeExact picks the routed person deterministically — the first lane
// that hit — and reports the first later lane that named someone else.
// Routing is immediate and never deferred to a human: a message with
// nowhere to land is worse than a message on the record whose binding was
// established first.
func routeExact(lanes []exactLane) (PersonResolution, bool) {
	for i, lane := range lanes {
		if !lane.found {
			continue
		}
		res := PersonResolution{Decision: DecisionExactCollision, PersonID: lane.personID, MatchedLane: lane.name}
		for _, rival := range lanes[i+1:] {
			if rival.found && rival.personID != lane.personID {
				res.Conflict = &LaneConflict{
					RoutedTo: lane.personID, Rival: rival.personID,
					RoutedLane: lane.name, RivalLane: rival.name,
				}
				break
			}
		}
		return res, true
	}
	return PersonResolution{}, false
}

// exactPersonByEmail is PO-F-1 tier 1. Every candidate email is checked;
// the lowest person id wins so a candidate colliding on two emails
// against two people resolves the same way on every run.
func exactPersonByEmail(ctx context.Context, tx pgx.Tx, emails []string) (ids.PersonID, bool, error) {
	if len(emails) == 0 {
		return ids.PersonID{}, false, nil
	}
	lowered := make([]string, 0, len(emails))
	for _, e := range emails {
		lowered = append(lowered, normalizeEmail(e))
	}
	var id ids.PersonID
	err := tx.QueryRow(ctx, `
		SELECT person_id FROM person_email
		WHERE email = ANY($1) AND archived_at IS NULL
		ORDER BY person_id
		LIMIT 1`, lowered).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return ids.PersonID{}, false, nil
	}
	if err != nil {
		return ids.PersonID{}, false, fmt.Errorf("dedupe person exact tier: %w", err)
	}
	return id, true, nil
}

// personCandidateRow is one row of the restricted candidate set.
type personCandidateRow struct {
	id       ids.PersonID
	fullName string
	orgID    *ids.OrganizationID
	// orgDomain is a domain the incumbent's EMPLOYER is registered under;
	// mailDomains are the ones their own live addresses sit on. Both say "these
	// two work at the same place", and the second says it while the employer is
	// still an open question — which is where a captured counterparty starts.
	orgDomain   *string
	mailDomains []string
}

// fuzzyPerson is PO-F-1 tier 2. The candidate set is restricted to
// people sharing a name trigram or the candidate's employer — the
// formula's own bound, so scoring stays inside the create budget instead
// of walking the workspace.
// WHY THE CANDIDATE QUERY HAS THREE ARMS. The trigram arm and the employer arm
// both narrow a set for a SCORE, and being approximate is fine there: a row they
// miss was never going to win. The name-collision lane is not a score — it
// decides on NormalizePersonName equality — so a prefilter that is merely
// approximate could hide a row that would have been exactly equal, and the
// employer arm cannot rescue it because a manual create carries no employer at
// all.
//
// The third arm folds BOTH sides with SQL's own functions, exactly as the
// trigram arm does. Computing one side in Go would move the divergence rather
// than close it: SQL's lower+unaccent and Go's Unicode full folding are two
// different normalizations, and the point is to stop relying on either being a
// superset of the other. It is personNameKeySQL, which spells the two
// properties NormalizePersonName has and a bare SQL comparison does not: the
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
func fuzzyPerson(ctx context.Context, tx pgx.Tx, c PersonCandidate) (PersonResolution, error) {
	rows, err := tx.Query(ctx, `
		SELECT p.id, p.full_name, r.organization_id, od.domain,
		       -- EVERY live address, not an unordered LIMIT 1: a contact with a
		       -- work and a personal address has two primaries as far as this
		       -- query is concerned, and picking either at random would make
		       -- the employer term depend on the plan Postgres chose. Archived
		       -- addresses are excluded — an address somebody stopped using is
		       -- not evidence of where they work now.
		       (SELECT array_agg(DISTINCT split_part(pe.email, '@', 2))
		          FROM person_email pe
		         WHERE pe.person_id = p.id AND pe.archived_at IS NULL)
		  FROM person p
		  LEFT JOIN relationship r
		    ON r.person_id = p.id AND r.kind = 'employment'
		   AND `+CurrentPrimaryEmploymentSQL("r")+` AND r.archived_at IS NULL
		  LEFT JOIN organization_domain od
		    ON od.organization_id = r.organization_id AND od.archived_at IS NULL
		 WHERE p.archived_at IS NULL
		   AND (f_fold_apostrophes(lower(p.full_name)) % f_fold_apostrophes(lower($1))
		        OR ($2::uuid IS NOT NULL AND r.organization_id = $2)
		        OR `+personNameKeySQL("p.full_name")+` = `+personNameKeySQL("$1")+`)`,
		c.FullName, c.CurrentPrimaryOrgID)
	if err != nil {
		return PersonResolution{}, fmt.Errorf("dedupe person candidate set: %w", err)
	}
	defer rows.Close()

	best := PersonResolution{Decision: DecisionNoMatch}
	// The best name-IDENTICAL row, tracked apart from the best-scoring one
	// because they are not the same question and the winner of one is often not
	// the winner of the other: a near-name at a matching employer outscores an
	// exact name with no employer known, so reading the name lane off `best`
	// would lose exactly the pair it exists to catch.
	sameName := PersonResolution{Decision: DecisionNoMatch}
	candidateKey := NormalizePersonName(c.FullName)
	for rows.Next() {
		var row personCandidateRow
		if err := rows.Scan(&row.id, &row.fullName, &row.orgID, &row.orgDomain, &row.mailDomains); err != nil {
			return PersonResolution{}, fmt.Errorf("scan person candidate: %w", err)
		}
		confidence := personConfidence(c, row)
		// Equal confidence resolves to the lowest person id — a total
		// order, so the queue does not shuffle between runs.
		if confidence > best.Confidence ||
			(confidence == best.Confidence && best.PersonID != (ids.PersonID{}) && row.id.String() < best.PersonID.String()) {
			best.Confidence, best.PersonID = confidence, row.id
		}
		if NormalizePersonName(row.fullName) == candidateKey {
			// Same tie-break as above, and for the same reason: two incumbents
			// spelled identically must not shuffle the queue between runs.
			if confidence > sameName.Confidence ||
				(sameName.PersonID == (ids.PersonID{})) ||
				(confidence == sameName.Confidence && row.id.String() < sameName.PersonID.String()) {
				sameName.Confidence, sameName.PersonID = confidence, row.id
			}
		}
	}
	if err := rows.Err(); err != nil {
		return PersonResolution{}, fmt.Errorf("drain person candidates: %w", err)
	}
	if best.Confidence >= dedupeReviewThreshold {
		best.Decision = DecisionFuzzyReview
		return best, nil
	}
	// THE NAME-COLLISION LANE. Two people in one workspace written exactly the
	// same way are worth a human's glance even when nothing else agrees.
	//
	// It is a lane of its own rather than a lower threshold, and the difference
	// is not cosmetic: the weights are shared with organization matching, so
	// moving the bar to admit this pair would drag every company comparison down
	// with it. An exact name is also not a probability — it either is the same
	// string or it is not — so scoring it and comparing against a fuzzy bar was
	// always the wrong instrument.
	//
	// WHY IT WAS UNREACHABLE. A perfect name scores 0.55·1.0 and the bar is
	// 0.72, so the pair could only clear it on employer agreement. But
	// CreatePersonInput carries no employer at all — the employment edge is a
	// separate call made after the person exists — so at create time the org
	// term is structurally 0, and a second business card for someone already in
	// the workspace was created in silence every time.
	//
	// IT FLAGS, IT NEVER MERGES AND NEVER REFUSES. A father and son at one firm
	// are a real pair of records that share a name and an employer, and an
	// automatic rule that merged them would destroy data no undo restores. So
	// this is a question put to a human, which is the same shape the phone lane
	// already takes for a shared switchboard.
	if c.QueueNameCollisions && sameName.PersonID != (ids.PersonID{}) {
		sameName.Decision = DecisionNameCollisionReview
		return sameName, nil
	}
	return PersonResolution{Decision: DecisionNoMatch}, nil
}

// personConfidence is the PO-F-1 score: weights sum to 1.0, so the
// result is in [0,1] and comparable against the threshold directly.
func personConfidence(c PersonCandidate, row personCandidateRow) float64 {
	return dedupeNameWeight*nameSimilarity(c.FullName, row.fullName) +
		dedupeOrgDomainWeight*orgMatch(c, row)
}

// orgMatch is PO-F-1's employer agreement term, most-specific first: a shared
// employer row beats a shared company domain, which beats two addresses simply
// sitting on the same domain.
//
// That last rung is what keeps the term alive for a captured counterparty.
// Capture creates the person and withholds the company until a site read judges
// the domain, so for the whole time that question is open there is no employer
// row and no organization_domain to agree about — and two colleagues at a new
// customer would stop meeting at the fuzzy tier just when their records are
// newest and most likely to be twins.
func orgMatch(c PersonCandidate, row personCandidateRow) float64 {
	if c.CurrentPrimaryOrgID != nil && row.orgID != nil && *c.CurrentPrimaryOrgID == *row.orgID {
		return 1.0
	}
	if row.orgDomain != nil && candidateSharesDomain(c, *row.orgDomain) {
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
// it says nothing at all — two people at gmail.com share a mail host, not a
// job — and scoring it would put every same-named pair of private addresses in
// the review queue, which is exactly where "same domain" carries least signal.
//
// The candidate's own matcher decides when it carries one, so a workspace that
// has corrected the shipped list is obeyed here as well as in capture — its
// carve-outs are assertions that a domain IS an employer's, and honouring them
// only on the capture side would leave dedupe dropping evidence the admin
// deliberately restored.
func sharedEmployerDomain(c PersonCandidate, domain string) bool {
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
// organization domain the incumbent is mapped to.
func candidateSharesDomain(c PersonCandidate, domain string) bool {
	for _, e := range c.Emails {
		if emailDomain(e) == normalizeDomain(domain) {
			return true
		}
	}
	return false
}

// normalizeEmail matches how person_email stores the address: the insert
// path lowercases on write, so the exact tier compares like for like.
func normalizeEmail(e string) string { return strings.ToLower(strings.TrimSpace(e)) }

// normalizeDomain matches organization_domain's storage contract:
// lowercase only — never unaccent, or münich.example would collide with
// a different organization's munich.example.
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
