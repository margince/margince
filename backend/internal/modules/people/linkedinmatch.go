// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

// Matching LinkedIn ghosts to CRM records (ADR-0078 §2.1b).
//
// A ghost is a name, maybe a company, and — on CSV rows where the connection
// allowed it — an address. Turning that into "this colleague knows THIS
// contact" is a dedupe problem. Two evidences confirm automatically (DECISIONS
// A143); everything weaker is a suggestion a human confirms:
//
//	EXACT EMAIL              → confirmed. An address is identity here, the same
//	                           way it is on the capture path.
//	EXACT NAME + EMPLOYER    → confirmed, but only with no rival: no other
//	                           contact of that name at that employer, and no
//	                           second ghost of the owner's naming the same one.
//	                           The two strings are the same string and the
//	                           employer agrees; asking a human trains them to
//	                           click through the queue without reading.
//	FOLDED NAME + EMPLOYER   → suggested. "André" vs "Andre" is a judgement
//	                           about whether two spellings are one person.
//	AMBIGUOUS                → neither. Two Andreas Müllers at one firm is the
//	                           case that must not be resolved by a coin flip.
//
// A confirmation performs the WHOLE write, automatic or human: the connection
// linked, its profile URL on the contact, an audit row and an event for each.
// It therefore takes person:update — a read-only caller still sweeps, and its
// matches all land as suggestions. The write itself is in linkedinmatchapply.go
// (confirmMatchWriteTail); applying a pass's decisions is in
// linkedinautoconfirm.go.
//
// Nothing here ever CREATES a person. A ghost that matches nothing stays a
// ghost, and its only contribution is the org-level count — "someone here is
// connected to 3 people at this account" — which needs no identity at all.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/employment"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// LinkedInMatchResult reports what one matching pass decided.
type LinkedInMatchResult struct {
	Confirmed int
	Suggested int
}

// MatchLinkedInConnections runs the matcher over one OWNER's unmatched ghosts
// and reports what it decided.
//
// Scoped to the owner because the caller is an upload reporting its own
// result: a workspace-wide sweep would count a colleague's older unmatched
// ghosts as this upload's confirmations, so the number on the screen would
// describe work the person did not just do. A zero owner means every ghost —
// the shape a scheduled sweep wants, and the only caller allowed to say
// "workspace-wide".
//
// It is safe to re-run: a ghost a human has already confirmed or rejected is
// never revisited, so a nightly pass cannot overturn a person's decision, and
// a rejection is permanent rather than something the next import forgets.
func (s *Store) MatchLinkedInConnections(ctx context.Context, owner ids.UUID) (LinkedInMatchResult, error) {
	return runLinkedInMatch(ctx, s, owner, ids.Nil)
}

// runLinkedInMatch is the shared body of the two public entry points: it gates
// the caller, resolves employers, and runs the two tiers.
//
// Employers resolve in their OWN transaction, ahead of the match. That step
// writes linkedin_connection.matched_org_id, and the match transaction holds
// CONTACTS — a connection lock carried into it would cross the Art. 17 eraser,
// which locks the contact and then deletes the connection. A separate commit
// releases the connection locks before any contact is held, and the resolution
// is idempotent so its own transaction costs nothing. (locksBeforeTheSubject
// ratifies this: the static gate reads the two transactions as one body.)
func runLinkedInMatch(ctx context.Context, s *Store, owner, onlyPerson ids.UUID) (LinkedInMatchResult, error) {
	// Read is the floor: the matcher has to see which contacts this member may
	// be shown, and demanding update would silently strand the network of any
	// member whose role reads people without editing them. An automatic CONFIRM
	// is more — it stamps the connection's profile URL onto the contact — so it
	// takes the update grant on top. A read-only caller still sweeps; its
	// matches land as suggestions a human confirms, so it can never cause a
	// contact edit it is not itself allowed to make.
	if err := auth.Require(ctx, "person", principal.ActionRead); err != nil {
		return LinkedInMatchResult{}, err
	}
	canConfirm := auth.Allows(ctx, "person", principal.ActionUpdate)
	if err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		return matchGhostOrganizations(ctx, tx)
	}); err != nil {
		return LinkedInMatchResult{}, err
	}
	var out LinkedInMatchResult
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		return matchInTx(ctx, tx, owner, onlyPerson, canConfirm, &out)
	})
	return out, err
}

// matchInTx reads both tiers' candidates, then applies them, folding the pass's
// decision into out. canConfirm is the caller's person:update authority: with it
// an exact identity auto-confirms, without it every match is a suggestion.
// onlyPerson is ids.Nil for a whole sweep, or the one contact a person-scoped
// call names.
//
// Both candidate sets are read BEFORE either is applied, so every contact a
// confirm will touch can be locked once, in one ascending order — the order two
// concurrent passes must share or deadlock. Reading the name tier before
// applying the address tier is what lets the two sets be locked together;
// applying the address tier first would take its contact locks out of that one
// order.
func matchInTx(ctx context.Context, tx pgx.Tx, owner, onlyPerson ids.UUID, canConfirm bool, out *LinkedInMatchResult) error {
	emailCands, err := emailMatchCandidates(ctx, tx, owner, onlyPerson)
	if err != nil {
		return err
	}
	// A contact the address tier will confirm this pass is withheld from the
	// name tier's proposals: one contact is not two of a colleague's
	// connections, the in-pass form of the name SELECT's NOT EXISTS, which sees
	// only confirms from earlier passes. Only under canConfirm — without it the
	// address tier suggests rather than confirms, and the guard does not apply.
	var confirmedByEmail []ids.UUID
	if canConfirm {
		confirmedByEmail = confirmableContacts(emailCands)
	}
	nameCands, err := nameMatchCandidates(ctx, tx, owner, onlyPerson, confirmedByEmail)
	if err != nil {
		return err
	}
	if canConfirm {
		if err := holdConfirmContacts(ctx, tx, emailCands, nameCands); err != nil {
			return err
		}
	}
	ec, es, err := applyMatchCandidates(ctx, tx, emailCands, canConfirm)
	if err != nil {
		return err
	}
	nc, ns, err := applyMatchCandidates(ctx, tx, nameCands, canConfirm)
	if err != nil {
		return err
	}
	out.Confirmed += ec + nc
	out.Suggested += es + ns
	return nil
}

// MatchLinkedInConnectionsForPerson matches the unmatched ghosts against ONE
// contact, and it is the path that matters most in practice.
//
// A workspace does not learn its contacts all at once. An export is uploaded
// during onboarding, and the people it could match are created over the
// following hours and weeks — by mail capture, by a site read, by a rep typing
// a name in. Every one of those is a chance to attach a ghost that the upload
// could not have attached, and asking each writer to remember to call the
// matcher would guarantee that one of them forgets.
//
// So the trigger is the EVENT every writer already emits. person.created and
// person.updated flow through the outbox because the write shape puts them
// there, and the cg:graph-edge consumer turns them into this call. Manual
// entry, capture, site read, merge and import all reach it without any of them
// knowing this function exists.
//
// Scoped to the one person so the cost is proportional to the change: a
// workspace-wide pass per person event would re-scan every unmatched ghost
// thousands of times during a capture backfill.
//
// owner narrows the email/name-and-employer arms to that ONE member's
// ghosts — the same narrowing MatchLinkedInConnections applies via its own
// owner parameter. The caller (compose/linkedinmatchgen.go's matchPerson)
// already runs this once per ghost owner under that owner's real principal
// precisely so a match is decided by the ghost owner's authority (see
// ghostOwnerCapturePrivacy's doc above); passing ids.Nil here — SQL NULL,
// "every owner" — would match every member's ghosts under whichever
// member's row scope the loop currently binds, turning a guessed address
// uploaded as an owner-private ghost into a contact-existence oracle: wait
// for a broader-scoped colleague's pass to reach it, then read
// match_status to learn a contact you cannot see exists.
func (s *Store) MatchLinkedInConnectionsForPerson(ctx context.Context, owner, person ids.UUID) (LinkedInMatchResult, error) {
	return runLinkedInMatch(ctx, s, owner, person)
}

// ghostOwnerCapturePrivacy is the capture-privacy arm of the boundary, carried
// on the match itself.
//
// It is a property of the ROW — visibility='owner' means the importing member
// alone, not even an admin — so it cannot come from the caller's scope clause
// and has to be rendered against the GHOST's owner. Row scope is the other arm
// and is a property of the READER; it arrives through auth.ScopeClauseFor,
// which is why the background sweep must run under each owner's real principal
// (compose/linkedinmatchgen.go, compose/linkedinrematch.go). A system principal
// is unbounded by design, so a sweep that ran as one would have neither arm:
// upload a guessed address, wait, and read match_status to learn whether a
// contact you cannot see exists.
const ghostOwnerCapturePrivacy = `(p.visibility <> 'owner' OR p.owner_id = g.owner_user_id)`

// noConfirmedRivalConnection excludes a contact the same member already has a
// confirmed LinkedIn connection to. One contact is not two different
// connections of the same colleague, so a second one is not matched onto them.
// The address tier and the name tier both apply it, over the same g (connection)
// and p (person) aliases.
const noConfirmedRivalConnection = `NOT EXISTS (
			           SELECT 1 FROM linkedin_connection other
			            WHERE other.matched_person_id = p.id
			              AND other.owner_user_id = g.owner_user_id
			              AND other.match_status = 'confirmed')`

// emailMatchCandidates reads the ghosts whose address is already a known
// contact's address. An address identifies a person, so every such pair is
// confirmable: the applier auto-confirms it for a caller that may edit a contact
// — the same rule capture's dedupe uses — and suggests it for a read-only one.
func emailMatchCandidates(ctx context.Context, tx pgx.Tx, owner, onlyPerson ids.UUID) ([]matchCandidate, error) {
	// The person row scope, on the MATCH itself. Without it the matcher links
	// a ghost to a contact the uploader cannot see — and then reports a
	// confirmed count, which turns a one-row CSV into an oracle: upload a
	// guessed address, read the number, learn whether an owner-private
	// captured contact with that address exists.
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	ownerPos := arg(nullableOwner(owner))
	// The same nullable-parameter shape as the owner filter: a zero id means
	// "every candidate", so one query serves both the sweep and the per-person
	// call rather than two spellings of the same match drifting apart.
	personPos := arg(nullableOwner(onlyPerson))
	visible, err := auth.ScopeClauseFor(ctx, "person", "p", arg)
	if err != nil {
		return nil, err
	}
	if visible == "" {
		visible = sqlAlwaysVisible
	}
	rows, err := tx.Query(ctx, fmt.Sprintf(`
		SELECT g.id, pe.person_id, g.owner_user_id, true
		  FROM linkedin_connection g
		  JOIN person_email pe ON g.email IS NOT NULL AND lower(pe.email) = g.email
		  JOIN person p ON p.id = pe.person_id AND p.archived_at IS NULL
		 WHERE g.tombstoned_at IS NULL
		   -- Only an undecided ghost. A human's confirm or reject stands.
		   AND g.match_status = 'unmatched'
		   AND ($%[1]d::uuid IS NULL OR g.owner_user_id = $%[1]d)
		   AND ($%[3]d::uuid IS NULL OR p.id = $%[3]d)
		   AND `+ghostOwnerCapturePrivacy+`
		   AND `+noConfirmedRivalConnection+`
		   AND (%[2]s)`, ownerPos, visible, personPos), args...)
	if err != nil {
		return nil, fmt.Errorf("people: reading LinkedIn address matches: %w", err)
	}
	cands, err := scanCandidates(rows)
	if err != nil {
		return nil, fmt.Errorf("people: reading LinkedIn address matches: %w", err)
	}
	return cands, nil
}

// nameEmployerCandidatesSQL finds the ghosts whose normalized name and live
// employer agree with a contact's, and marks which of them an exact name
// releases for automatic confirmation. Four parameter positions: the owner
// filter, the person row scope, the single-person narrowing, and the contacts
// the address tier already confirmed this pass and so withholds from here.
//
// A var and not a const, because the employment-currency predicate is a
// function call: employment.IsCurrentSQL is the one definition of "this job is
// still theirs", and a const cannot reach it. This query used to hand-spell it
// and got the semantics right, which is what made the copy invisible.
var nameEmployerCandidatesSQL = `
		WITH pair AS (
		    -- DISTINCT pairs FIRST. A contact with two live employment rows at
		    -- one account (a role change recorded as a second row) joins twice
		    -- and is still one candidate; counting the join rows would read
		    -- that as an ambiguity and refuse a correct match.
		    SELECT DISTINCT g.id AS ghost_id, p.id AS person_id,
		           g.owner_user_id AS owner_user_id,
		           -- Whether the names agree EXACTLY, before folding. The fold
		           -- is what finds the candidate; this is what decides whether
		           -- a human still has to look at it.
		           g.full_name = p.full_name AS exact_name
		      FROM linkedin_connection g
		      JOIN person p
		        ON p.archived_at IS NULL
		       -- f_unaccent + lower is the DATABASE's approximation of the Go
		       -- normalizer that produced normalized_name. It narrows
		       -- candidates, and the outcome is confirmed only on an EXACT name,
		       -- so a near-miss costs a proposal rather than a wrong link.
		       AND lower(f_unaccent(p.full_name)) = g.normalized_name
		      JOIN relationship r
		        ON r.person_id = p.id AND r.kind = 'employment'
		       -- Still employed TODAY, the same test the coverage and intro
		       -- reads take: a future end date is still employment.
		       AND r.archived_at IS NULL
		       AND ` + employment.IsCurrentSQL("r.ended_at") + `
		     WHERE g.match_status = 'unmatched'
		       AND g.tombstoned_at IS NULL
		       -- The employer is matched through matched_org_id, which the
		       -- Go-side resolver set using the ONE org-name normalizer. Doing
		       -- it here in SQL would mean a second spelling of the
		       -- legal-suffix strip, and two spellings of a normalizer drift.
		       AND ($%[1]d::uuid IS NULL OR g.owner_user_id = $%[1]d)
		       -- Narrowing to ONE contact must not narrow the ambiguity checks:
		       -- the windows below still see every same-named candidate, so
		       -- a per-person call cannot confirm a link the sweep would have
		       -- refused as ambiguous. It filters the RESULT, not the pairs.
		       AND (%[2]s)
		       AND ` + ghostOwnerCapturePrivacy + `
		       AND g.matched_org_id IS NOT NULL
		       AND r.organization_id = g.matched_org_id
		       -- A contact the address tier confirmed EARLIER in this pass, held
		       -- out here the same way the NOT EXISTS below holds out one
		       -- confirmed in an earlier pass. COALESCE, because a nil parameter
		       -- arrives as SQL NULL and ` + "`<> ALL(NULL)`" + ` is NULL — which would
		       -- reject every contact; the empty array admits them all.
		       AND p.id <> ALL(COALESCE($%[4]d::uuid[], '{}'))
		       AND ` + noConfirmedRivalConnection + `
		),
		candidate AS (
		    -- The count is over distinct PEOPLE one ghost matches, which is what
		    -- ambiguity means for the ghost. (count(DISTINCT …) is not a window
		    -- function in Postgres, hence the two steps rather than one.)
		    SELECT ghost_id, person_id, owner_user_id, exact_name,
		           count(*) OVER (PARTITION BY ghost_id) AS matches
		      FROM pair
		),
		scored AS (
		    -- And the mirror count: distinct GHOSTS of one owner an exact name
		    -- would confirm onto one contact. Two unmatched ghosts with the same
		    -- exact name and employer pointing at the same person are as
		    -- ambiguous as one ghost pointing at two people — neither may
		    -- auto-confirm, though each stays a suggestion a human can judge.
		    SELECT ghost_id, person_id, owner_user_id, exact_name, matches,
		           count(*) FILTER (WHERE exact_name AND matches = 1)
		             OVER (PARTITION BY owner_user_id, person_id) AS exact_confirmers
		      FROM candidate
		)
		SELECT ghost_id, person_id, owner_user_id,
		       (exact_name AND matches = 1 AND exact_confirmers = 1) AS confirmable
		  FROM scored
		 -- Ambiguity is not even a suggestion: one ghost matching two contacts
		 -- of the same name is the case a human must resolve, and picking one
		 -- would be a guess wearing a confirmation's clothes. Such a ghost gets
		 -- no row here and stays unmatched.
		 WHERE matches = 1
		   -- The per-person narrowing, applied to the RESULT and not to the
		   -- windows above: both ambiguity counts must still see every
		   -- same-named candidate, or a per-person call would confirm a link the
		   -- workspace-wide sweep correctly refuses.
		   AND ($%[3]d::uuid IS NULL OR person_id = $%[3]d)
		 ORDER BY person_id, ghost_id`

// nameMatchCandidates reads the ghosts whose normalized name and live employer
// agree with a contact's, marking which an exact name releases for confirmation.
//
// It requires BOTH, and the employment must be live. The employer is what turns
// a common name into a plausible identification, and it is still only plausible:
// only an EXACT name at a matched employer, with no rival ghost or contact, is
// confirmable — and the applier confirms it only for a caller that may edit a
// contact. A folded-only name ("André" vs "Andre") and everything ambiguous is
// a suggestion a human confirms.
//
// excludeContacts are the contacts the address tier confirmed this pass; a
// contact already confirmed against is not proposed again, because one contact
// is not two different connections of the same colleague.
func nameMatchCandidates(ctx context.Context, tx pgx.Tx, owner, onlyPerson ids.UUID, excludeContacts []ids.UUID) ([]matchCandidate, error) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	ownerPos := arg(nullableOwner(owner))
	personPos := arg(nullableOwner(onlyPerson))
	excludePos := arg(excludeContacts)
	// Same reason as the address arm: a match against an invisible contact both
	// creates a link the uploader may not make and reports its existence.
	visible, err := auth.ScopeClauseFor(ctx, "person", "p", arg)
	if err != nil {
		return nil, err
	}
	if visible == "" {
		visible = sqlAlwaysVisible
	}
	rows, err := tx.Query(ctx, fmt.Sprintf(nameEmployerCandidatesSQL, ownerPos, visible, personPos, excludePos), args...)
	if err != nil {
		return nil, fmt.Errorf("people: reading LinkedIn name-and-employer matches: %w", err)
	}
	cands, err := scanCandidates(rows)
	if err != nil {
		return nil, fmt.Errorf("people: reading LinkedIn name-and-employer matches: %w", err)
	}
	return cands, nil
}

// matchGhostOrganizations attaches ghosts to an ACCOUNT by employer name even
// when the person never matches.
//
// This is where most of the value is, and it needs no identity at all. "Three
// people here are LinkedIn-connected to someone at Acme" is actionable on its
// own — it tells a rep the door is not cold — and it is true whether or not
// any of those three is a contact in the CRM.
// OrganizationLinkedInReach counts, per colleague, how many of their LinkedIn
// connections work at one account — the weaker, clearly-labelled evidence tier
// beside real interaction history.
//
// It is a COUNT and never a list of names, and that is a privacy decision
// rather than a payload-size one. The connections are third parties who never
// consented to appearing in this CRM; saying "Lars knows 3 people at Acme"
// discloses nothing about them, while naming them would publish a private
// address book to the colleague's whole team.
func OrganizationLinkedInReach(ctx context.Context, tx pgx.Tx, orgID ids.OrganizationID) (map[ids.UUID]int, error) {
	if err := auth.Require(ctx, "organization", principal.ActionRead); err != nil {
		return nil, err
	}
	// The row gate, not just the object grant. A reach count is a statement
	// ABOUT an account — answering it for an account the caller cannot open
	// discloses that the account exists, and does so through a side door that
	// the account's own read path closes. 404-hiding, like every other
	// single-record read.
	if err := auth.EnsureVisible(ctx, tx, "organization", orgID.UUID); err != nil {
		return nil, err
	}
	// Both halves of "still works here", not just archived_at. A reach count is
	// an offer to ask a colleague for an intro, and a deactivated account is as
	// unreachable as an archived one — filtering on archived_at alone answers
	// "ask Lars" about someone who cannot be asked. Spelled out rather than
	// taken from identity.LiveMemberSQL because a module never imports a
	// sibling (ADR-0054 §3); TestOnlyOneSpellingOfALiveMember holds the two
	// together.
	rows, err := tx.Query(ctx, `
		SELECT g.owner_user_id, count(*)
		  FROM linkedin_connection g
		  JOIN app_user u ON u.id = g.owner_user_id
		                 AND u.status = 'active' AND u.archived_at IS NULL
		 WHERE g.matched_org_id = $1
		   AND g.tombstoned_at IS NULL
		   AND g.match_status <> 'rejected'
		 GROUP BY g.owner_user_id`, orgID)
	if err != nil {
		return nil, fmt.Errorf("people: counting LinkedIn reach into an account: %w", err)
	}
	defer rows.Close()
	out := map[ids.UUID]int{}
	for rows.Next() {
		var user ids.UUID
		var n int
		if err := rows.Scan(&user, &n); err != nil {
			return nil, err
		}
		out[user] = n
	}
	return out, rows.Err()
}

// nullableOwner renders the zero id as SQL NULL, which the scoping clauses
// read as "every owner". A zero uuid would otherwise match nobody and turn a
// workspace-wide sweep into a silent no-op.
func nullableOwner(owner ids.UUID) *ids.UUID {
	if owner == ids.Nil {
		return nil
	}
	return &owner
}
