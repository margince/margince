// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// Relationship strength (formulas-and-rules §4, B-E13.16): one
// deterministic recency × frequency × reciprocity function over captured
// interactions — never predictive ML. The score decomposes exactly to
// its three named factors (P6 "no mystery number") and reads contact +
// activity ONLY: leads never contribute (ADR-0008 — a lead-linked
// activity carries lead_id, not contact_id, so exclusion is structural
// and the tests pin it).

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/employment"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/relstrength"
)

// The §4 vocabulary this package uses, aliased from the leaf that owns the
// arithmetic (shared/kernel/relstrength) rather than restated. A second
// definition of a tunable is a second thing to keep in step.
const (
	bucketNone            = relstrength.BucketNone
	relStrengthWindowDays = relstrength.WindowDays
	// relStrengthEvidenceCap bounds the contributing-ids payload; the
	// factors are computed over the FULL window regardless. It is this
	// package's own concern — a display cap, not part of the formula.
	relStrengthEvidenceCap = 200
)

// RelationshipStrength is the explainable §4 output: the 0–100 score,
// its display bucket, the three factors it reconciles to, and the
// contributing activity ids (clickable, "no mystery number").
type RelationshipStrength struct {
	Strength int
	Bucket   string // weak | moderate | strong | none (no interactions yet)

	Recency     float64
	Frequency   float64
	Reciprocity float64

	LastInteraction     *time.Time
	InteractionCount90d int
	Inbound90d          int
	Outbound90d         int
	ContributingIDs     []ids.ActivityID

	// The two directions dated separately, because the counts above cannot say
	// who wrote LAST once both are non-zero: a contact who replied in March and
	// was chased in August has inbound and outbound alike, and only the dates
	// say which way the conversation is owed. EngagementOf reads the counts for
	// the window and these dates for the direction that separates answered from
	// waiting.
	LastInbound  *time.Time
	LastOutbound *time.Time
	// LastInboundActivity is the message a follow-up would answer — the anchor
	// the composer needs to open a reply rather than a fresh thread.
	//
	// Bounded by the SAME 90-day window as the direction counts, unlike the two
	// dates above. Those are history: "they last wrote in March" is true and
	// worth showing however old it is. An anchor is an ACTION, and it has to
	// agree with the state the counts report — a contact whose only message is
	// eighteen months old reads untried, and offering to "follow up" on that
	// thread would open a reply to a conversation the same page just said had
	// never happened. Nil is therefore the common case on a cold account, and
	// it is the honest one.
	//
	// Read as a correlated LIMIT 1 rather than an aggregate over the history:
	// array_agg(...)[1] materialised every inbound id a contact had ever sent
	// in order to use one of them, which is the payload growing with history
	// that this fold exists to avoid.
	LastInboundActivity *ids.ActivityID
}

// strengthKinds are the activity kinds that count as contact, from the one
// shared definition — see relstrength.InteractionKindSQLGroup.
var strengthKinds = relstrength.InteractionKindSQLGroup()

// strengthInteractionUnit is what ONE interaction is when these folds count
// them, from the same shared definition — see relstrength.InteractionUnitSQL.
// Every fold below aliases activity as `a`, so one rendering serves them all.
var strengthInteractionUnit = relstrength.InteractionUnitSQL("a")

// ContactStrength computes the §4 baseline for one contact. The contact
// read is row-scoped exactly like GetContact: a contact the caller cannot
// see has no strength to disclose.
func (s *Store) ContactStrength(ctx context.Context, contactID ids.ContactID, now time.Time) (RelationshipStrength, error) {
	if err := auth.Require(ctx, "contact", principal.ActionRead); err != nil {
		return RelationshipStrength{}, err
	}
	var out RelationshipStrength
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		if err := auth.EnsureVisible(ctx, tx, "contact", contactID.UUID); err != nil {
			return err
		}
		return strengthInputs(ctx, tx, contactID, now, nil, &out)
	})
	if err != nil {
		return RelationshipStrength{}, err
	}
	out.finish(now)
	return out, nil
}

// ContactStrengthTx is ContactStrength inside a caller-opened transaction —
// the composite record read. Same gates in the same order.
func (s *Store) ContactStrengthTx(
	ctx context.Context, tx pgx.Tx, contactID ids.ContactID,
	now time.Time, within *ids.ProjectID,
) (RelationshipStrength, error) {
	if err := auth.Require(ctx, "contact", principal.ActionRead); err != nil {
		return RelationshipStrength{}, err
	}
	if err := auth.EnsureVisible(ctx, tx, "contact", contactID.UUID); err != nil {
		return RelationshipStrength{}, err
	}
	var out RelationshipStrength
	if err := strengthInputs(ctx, tx, contactID, now, within, &out); err != nil {
		return RelationshipStrength{}, err
	}
	out.finish(now)
	return out, nil
}

// StrengthToWire renders a §4 result onto the contract's shared
// RelationshipStrength. It lives with the computation, not beside one of
// its transports: the per-contact route, the per-company route and the
// company view all answer this shape, and a bucket rename made in only one
// of three places is the drift this prevents.
//
// factors.direction has no dedicated domain field — the §4 computation
// derives it internally on the way to reciprocity — so it is recomputed
// here from the same two counts rather than invented.
func StrengthToWire(rs RelationshipStrength, now time.Time) crmcontracts.RelationshipStrength {
	inbound, outbound := rs.Inbound90d, rs.Outbound90d
	direction := 0.0
	if directed := inbound + outbound; directed > 0 {
		direction = 1 - math.Abs(float64(inbound-outbound))/float64(directed)
	}
	contributing := make([]openapi_types.UUID, len(rs.ContributingIDs))
	for i, activityID := range rs.ContributingIDs {
		contributing[i] = openapi_types.UUID(activityID.UUID)
	}
	computedAt := now
	wire := crmcontracts.RelationshipStrength{
		Score:                   rs.Strength,
		Bucket:                  StrengthBucketToWire(rs.Bucket),
		LastInteraction:         rs.LastInteraction,
		ComputedAt:              &computedAt,
		Inbound90d:              &inbound,
		Outbound90d:             &outbound,
		ContributingActivityIds: &contributing,
	}
	wire.Factors.Recency = float32(rs.Recency)
	wire.Factors.Frequency = float32(rs.Frequency)
	wire.Factors.Reciprocity = float32(rs.Reciprocity)
	wire.Factors.Direction = float32(direction)
	return wire
}

// StrengthBucketToWire types the domain's display bucket for the contract.
// The two vocabularies are the same words on purpose — the wire enum was
// aligned to the §4 kernel's — so this switch renames nothing. The input is
// still a plain string, so a value the kernel never emits reads as none
// rather than as a wire value the enum never declared.
func StrengthBucketToWire(bucket string) crmcontracts.RelationshipStrengthBucket {
	switch bucket {
	case relstrength.BucketWeak:
		return crmcontracts.RelationshipStrengthBucketWeak
	case relstrength.BucketModerate:
		return crmcontracts.RelationshipStrengthBucketModerate
	case relstrength.BucketStrong:
		return crmcontracts.RelationshipStrengthBucketStrong
	default:
		return crmcontracts.RelationshipStrengthBucketNone
	}
}

// ContactStrength pairs one of a company's current contacts with
// that contact's §4 score.
type ContactStrength struct {
	ContactID ids.ContactID
	Strength  RelationshipStrength
}

// StrengthForCompanyContacts computes §4 for every current employee of one
// company that the caller can read, inside the caller's OWN
// transaction and in a fixed number of queries — two, regardless of how
// many contacts the account has.
//
// It exists because the per-contact path opens a transaction each: the
// company view needs a score beside every contact, and doing that through
// ContactStrength would open one transaction per row and read a different
// instant for each of them. The caller has already gated the company;
// what this adds is the contact row scope, applied here as a predicate so a
// contact the caller may not read contributes nothing and is not named.
//
// The results come back in the order the contacts sort by id, so a page
// built from them is deterministic.
func StrengthForCompanyContacts(
	ctx context.Context, tx pgx.Tx, companyID ids.CompanyID,
	now time.Time, within *ids.ProjectID,
) ([]ContactStrength, error) {
	if err := auth.Require(ctx, "contact", principal.ActionRead); err != nil {
		return nil, err
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	companyPos := arg(companyID)
	// The roster is drawn from employment EDGES: "who works at this account" is
	// the fact relationship.read governs, and this read is the one company360's
	// contacts section is built from. A refusal reaches the assembler, which
	// names `contacts` in sections_omitted — so the page says "you may not see
	// this" rather than showing an account with nobody at it.
	edgeBound, err := auth.EdgeReadScope(ctx, "r", arg)
	if err != nil {
		return nil, err
	}
	if edgeBound == "" {
		edgeBound = "TRUE"
	}
	scope, err := contactScopePredicate(ctx, arg)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, fmt.Sprintf(`
		SELECT p.id FROM contact p
		JOIN relationship r ON r.contact_id = p.id
		WHERE r.kind = 'employment' AND r.company_id = $%d
		  AND `+employment.IsCurrentSQL("r.ended_at")+` AND r.archived_at IS NULL
		  AND p.archived_at IS NULL AND (%s) AND (%s)
		ORDER BY p.id`, companyPos, edgeBound, scope), args...)
	if err != nil {
		return nil, err
	}
	contacts, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (ids.ContactID, error) {
		var id ids.ContactID
		err := row.Scan(&id)
		return id, err
	})
	if err != nil {
		return nil, err
	}
	if len(contacts) == 0 {
		return nil, nil
	}
	return contactStrengths(ctx, tx, contacts, now, nil, within)
}

// contactScopePredicate is the caller's contact row-scope predicate for a query
// that aliases contact as p, spelled once for the two batch reads below.
func contactScopePredicate(ctx context.Context, arg func(any) int) (string, error) {
	return scopeOrAllRows(ctx, "contact", "p", arg)
}

// StrengthForContacts computes §4 for an ARBITRARY contact set inside the
// caller's own transaction, pruned to their contact row scope and to live
// rows. A contact the caller may not read, or one that is archived, is
// absent from the result rather than carried with a zero — the caller
// learns nothing about a record they cannot open.
//
// StrengthForCompanyContacts answers "everyone employed here"; this answers
// "these contacts", which is what a reader that assembled its own contact
// set needs — the company view's connection graph scores employees and
// deal stakeholders together, and asking per contact would open one
// transaction each and read a different instant for every node.
func StrengthForContacts(ctx context.Context, tx pgx.Tx, contacts []ids.ContactID, now time.Time) ([]ContactStrength, error) {
	return strengthForContactsAt(ctx, tx, contacts, now, nil)
}

// StrengthForContactsAsOf answers what §4 WOULD have said at a past instant:
// the same fold, with the window ending at asOf and every later interaction
// excluded. It is what "this relationship is going cold" compares against —
// today's score against the same contact's score a month ago — and it needs
// no snapshot table to do it.
//
// It is a COUNTERFACTUAL over today's corpus, not exact history, and the
// difference is not pedantic. An activity archived or erased since asOf is
// absent from this answer even though it was present then. That is the
// behaviour we want, not a defect to route around: an erasure must not be
// able to resurrect the strength it removed, or the score becomes a way to
// read deleted data. What the caller is entitled to say is "strength then,
// judged by what we may still count today" — and no more than that.
func StrengthForContactsAsOf(ctx context.Context, tx pgx.Tx, contacts []ids.ContactID, asOf time.Time) ([]ContactStrength, error) {
	return strengthForContactsAt(ctx, tx, contacts, asOf, &asOf)
}

// strengthForContactsAt is the shared body. A nil upper bound means "no upper
// bound", which is deliberately NOT the same as passing now: an interaction
// timestamped slightly ahead of the app's clock is ordinary, and the live
// score has always counted it.
func strengthForContactsAt(ctx context.Context, tx pgx.Tx, contacts []ids.ContactID, now time.Time, until *time.Time) ([]ContactStrength, error) {
	if err := auth.Require(ctx, "contact", principal.ActionRead); err != nil {
		return nil, err
	}
	if len(contacts) == 0 {
		return nil, nil
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	contactsPos := arg(contacts)
	scope, err := contactScopePredicate(ctx, arg)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, fmt.Sprintf(`
		SELECT p.id FROM contact p
		WHERE p.id = ANY($%d) AND p.archived_at IS NULL AND (%s)
		ORDER BY p.id`, contactsPos, scope), args...)
	if err != nil {
		return nil, err
	}
	visible, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (ids.ContactID, error) {
		var id ids.ContactID
		err := row.Scan(&id)
		return id, err
	})
	if err != nil {
		return nil, err
	}
	if len(visible) == 0 {
		return nil, nil
	}
	return contactStrengths(ctx, tx, visible, now, until, nil)
}

// contactStrengths folds the §4 inputs for a whole contact set out of ONE
// grouped pass over their qualifying activities. The evidence ids are
// deliberately NOT collected here: they are the contact page's receipts, and
// carrying up to relStrengthEvidenceCap of them per contact would make an
// account list payload grow with its history rather than its contact count.
func contactStrengths(
	ctx context.Context, tx pgx.Tx, contacts []ids.ContactID,
	now time.Time, until *time.Time, within *ids.ProjectID,
) ([]ContactStrength, error) {
	args := []any{contacts, now.AddDate(0, 0, -relStrengthWindowDays), until}
	// BOTH aliases, or the score and the message it cites answer different
	// questions: `a` is what the counts are computed over and `i` is the
	// citation the card shows, so a page narrowed to one project could report a
	// number from that project beside a message from another.
	scoreWithin, citedWithin := "", ""
	if within != nil {
		args = append(args, *within)
		scoreWithin = " AND " + activityWithinProject("a", len(args))
		citedWithin = " AND " + activityWithinProject("i", len(args))
	}
	rows, err := tx.Query(ctx, `
		SELECT l.contact_id,
		       max(a.occurred_at),
		       count(DISTINCT `+strengthInteractionUnit+`) FILTER (WHERE a.occurred_at >= $2),
		       count(DISTINCT `+strengthInteractionUnit+`) FILTER (WHERE a.occurred_at >= $2 AND a.direction = 'inbound'),
		       count(DISTINCT `+strengthInteractionUnit+`) FILTER (WHERE a.occurred_at >= $2 AND a.direction = 'outbound'),
		       max(a.occurred_at) FILTER (WHERE a.direction = 'inbound'),
		       max(a.occurred_at) FILTER (WHERE a.direction = 'outbound'),
		       (SELECT i.id FROM activity i
		          JOIN activity_link il ON il.activity_id = i.id AND il.contact_id = l.contact_id
		         WHERE i.direction = 'inbound' AND i.kind IN `+strengthKinds+`
		           AND i.archived_at IS NULL AND i.occurred_at >= $2`+auth.AudienceWorkspaceOnly("i")+`
		           AND ($3::timestamptz IS NULL OR i.occurred_at <= $3)`+citedWithin+`
		         ORDER BY i.occurred_at DESC, i.id DESC
		         LIMIT 1)
		FROM activity a
		JOIN activity_link l ON l.activity_id = a.id
		WHERE l.contact_id = ANY($1) AND a.kind IN `+strengthKinds+` AND a.archived_at IS NULL`+auth.AudienceWorkspaceOnly("a")+`
		  -- NULL means no upper bound, so the live score is unchanged; an
		  -- as-of read passes the instant it is asking about.
		  AND ($3::timestamptz IS NULL OR a.occurred_at <= $3)`+scoreWithin+`
		GROUP BY l.contact_id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	byContact := make(map[ids.ContactID]*RelationshipStrength, len(contacts))
	for rows.Next() {
		var contactID ids.ContactID
		var rs RelationshipStrength
		if err := rows.Scan(&contactID, &rs.LastInteraction, &rs.InteractionCount90d,
			&rs.Inbound90d, &rs.Outbound90d, &rs.LastInbound, &rs.LastOutbound,
			&rs.LastInboundActivity); err != nil {
			return nil, err
		}
		byContact[contactID] = &rs
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := make([]ContactStrength, 0, len(contacts))
	for _, contactID := range contacts {
		// A contact with no qualifying interaction is still a contact: it
		// carries the honest "none" bucket, never a missing row.
		rs := RelationshipStrength{Bucket: bucketNone}
		if found, ok := byContact[contactID]; ok {
			rs = *found
		}
		rs.finish(now)
		out = append(out, ContactStrength{ContactID: contactID, Strength: rs})
	}
	return out, nil
}

func strengthInputs(
	ctx context.Context, tx pgx.Tx, contactID ids.ContactID,
	now time.Time, within *ids.ProjectID, out *RelationshipStrength,
) error {
	windowStart := now.AddDate(0, 0, -relStrengthWindowDays)
	foldArgs := []any{contactID, windowStart}
	// Every alias the fold reads, for the reason contactStrengths gives: the
	// counts, the cited message and the contributing ids are one reading, and
	// narrowing some of them would report a number over one body of work with
	// evidence from another.
	scoreWithin, citedWithin := "", ""
	if within != nil {
		foldArgs = append(foldArgs, *within)
		scoreWithin = " AND " + activityWithinProject("a", len(foldArgs))
		citedWithin = " AND " + activityWithinProject("i", len(foldArgs))
	}

	// One pass over the contact's qualifying interactions: overall last
	// touch, the 90-day direction counts, and the contributing ids.
	if err := tx.QueryRow(ctx, `
		SELECT max(a.occurred_at),
		       count(DISTINCT `+strengthInteractionUnit+`) FILTER (WHERE a.occurred_at >= $2),
		       count(DISTINCT `+strengthInteractionUnit+`) FILTER (WHERE a.occurred_at >= $2 AND a.direction = 'inbound'),
		       count(DISTINCT `+strengthInteractionUnit+`) FILTER (WHERE a.occurred_at >= $2 AND a.direction = 'outbound'),
		       max(a.occurred_at) FILTER (WHERE a.direction = 'inbound'),
		       max(a.occurred_at) FILTER (WHERE a.direction = 'outbound'),
		       (SELECT i.id FROM activity i
		          JOIN activity_link il ON il.activity_id = i.id AND il.contact_id = $1
		         WHERE i.direction = 'inbound' AND i.kind IN `+strengthKinds+`
		           AND i.archived_at IS NULL AND i.occurred_at >= $2`+auth.AudienceWorkspaceOnly("i")+citedWithin+`
		         ORDER BY i.occurred_at DESC, i.id DESC
		         LIMIT 1)
		FROM activity a
		JOIN activity_link l ON l.activity_id = a.id AND l.contact_id = $1
		WHERE a.kind IN `+strengthKinds+` AND a.archived_at IS NULL`+auth.AudienceWorkspaceOnly("a")+scoreWithin,
		foldArgs...).Scan(&out.LastInteraction, &out.InteractionCount90d,
		&out.Inbound90d, &out.Outbound90d, &out.LastInbound, &out.LastOutbound,
		&out.LastInboundActivity); err != nil {
		return err
	}

	citeArgs := []any{contactID, windowStart, relStrengthEvidenceCap}
	contributingWithin := ""
	if within != nil {
		citeArgs = append(citeArgs, *within)
		contributingWithin = " AND " + activityWithinProject("a", len(citeArgs))
	}
	rows, err := tx.Query(ctx, `
		SELECT a.id FROM activity a
		JOIN activity_link l ON l.activity_id = a.id AND l.contact_id = $1
		WHERE a.kind IN `+strengthKinds+` AND a.archived_at IS NULL AND a.occurred_at >= $2`+auth.AudienceWorkspaceOnly("a")+contributingWithin+`
		ORDER BY a.occurred_at DESC
		LIMIT $3`, citeArgs...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id ids.ActivityID
		if err := rows.Scan(&id); err != nil {
			return err
		}
		out.ContributingIDs = append(out.ContributingIDs, id)
	}
	return rows.Err()
}

// finish folds the gathered inputs through the §4 formula.
//
// The arithmetic itself lives in shared/kernel/relstrength, not here: the
// per-colleague score (PO-F-3b, ADR-0078) is the same curve over a narrower
// input set, and the two appear side by side on one screen. A constant tuned
// in one copy and not the other would read as a contradiction rather than a
// rounding difference, so there is one copy.
func (r *RelationshipStrength) finish(now time.Time) {
	score := relstrength.Compute(relstrength.Inputs{
		LastInteraction: r.LastInteraction,
		Count90d:        r.InteractionCount90d,
		Inbound90d:      r.Inbound90d,
		Outbound90d:     r.Outbound90d,
	}, now)
	r.Strength = score.Strength
	r.Bucket = score.Bucket
	r.Recency = score.Recency
	r.Frequency = score.Frequency
	r.Reciprocity = score.Reciprocity
}
