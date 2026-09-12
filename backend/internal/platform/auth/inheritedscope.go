// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

// Row scope for the records that own no owner_id. An activity and a
// signal carry free text about OTHER records, so their visibility is
// inherited rather than held: an activity from the records its links
// point at, a signal from the record it is about. Both rules live here,
// next to the activity_link disjunction they share (linkscope.go),
// because scope policy has exactly one spelling (ADR-0054 §8).

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ActivityAvailableClause is the predicate under which an activity is in the
// readable world at all: not held under a statutory retention obligation
// (A165/ADR-0114 §2). A restricted row is unavailable in EVERY ordinary read
// path — lists, timelines, search, exports, embeddings, agent grounding — for
// every principal, the unbounded admin included, because the obligation
// justifies storage and nothing else. It is composed into ActivityDiscoverClause
// so the ~30 readers that carry the scope get it for free, and it is exported
// for the readers that legitimately bypass the scope (an engine-internal
// aggregate, a capture-side gate) and must still exclude what is held.
// alias names the activity table in the outer query.
func ActivityAvailableClause(alias string) string {
	return alias + ".restricted_at IS NULL"
}

// ActivityDiscoverClause is the activity analogue of ScopeClause, and the
// weaker of the two activity gates: it answers whether the caller may learn
// that an activity EXISTS — its occurred_at, direction, kind, who owns it —
// without its content. Activities have no owner, but their free-text
// inherits the sensitivity of the records they attach to. An activity is
// discoverable when ANY linked contact/company/deal/lead/project is
// visible under the caller's row scope, or when it has no links at all (a
// workspace-shared note). It lives here, not in a module: it is the one
// scope rule that spans the record tables and activity_link rows, and scope
// policy has exactly one spelling (ADR-0054 §8). alias names the activity
// table in the outer query.
//
// A reader that shows subject, body, participants, thread, attachments or
// anything derived from them composes ActivityContentClause instead; this
// clause is for the safe markers alone (a last-touch date, an open-task
// count, an evidence probe), and a caller that picks it for content is the
// defect restrictedreaders_test.go and the module census exist to catch.
//
// It is never empty: an unbounded principal is spared the link-walk, not the
// availability test (ActivityAvailableClause). Callers written for the older
// "empty means unscoped" contract keep working — an always-present clause is
// what they append when it is present.
func ActivityDiscoverClause(ctx context.Context, alias string, arg func(any) int) (string, error) {
	p, err := rbacActor(ctx)
	if err != nil {
		return "", err
	}
	return activityDiscoverClause(p, alias, arg), nil
}

func activityDiscoverClause(p principal.Principal, alias string, arg func(any) int) string {
	available := ActivityAvailableClause(alias)
	if UnboundedFor(p, linkTargetTables...) {
		return available
	}
	// One correlated pass over the activity's links, not two. bool_or is
	// exactly the ANY-link rule, and its empty-set answer — NULL, coalesced
	// to true — is exactly the link-less note rule, so the shape carries
	// both halves without a second subquery.
	//
	// The two-subquery spelling this replaces read more plainly but cost a
	// hashed subplan: Postgres materialized the WHOLE activity_link table
	// up front to answer the NOT EXISTS arm, ~180ms on the smb bench tier,
	// paid before a single candidate row was examined. Every row-scoped
	// caller has been paying it; it only surfaced as a budget failure when
	// capture privacy stopped exempting the all-scope reader the perf
	// fixture happens to run as.
	//
	// The membership arm is the second way in, and it is what keeps a MEETING's
	// own attendees able to reach it. Discovery asks whether any LINKED record
	// is visible, so a meeting filed under a contact private to the seat that
	// captured it fails that test for a colleague who was IN the meeting — the
	// invitation is on their calendar and the row denies them. Membership is
	// the honest answer to "may this contact learn this exists": they were on it.
	//
	// It admits EXISTENCE only. Content still needs the audience arm, the linked
	// private contact stays unreadable with its own visibility check, and neither
	// the availability test nor object RBAC is relaxed. Nothing here reads
	// host_user_id: that column labels whose calendar a row came off, which is
	// ownership rather than membership, and a label is not evidence that anybody
	// was present.
	return fmt.Sprintf(`%[3]s AND (coalesce((SELECT bool_or(%[2]s)
	   FROM activity_link l WHERE l.activity_id = %[1]s.id), true)
	   OR %[4]s)`,
		alias, linkTargetVisible(p, "l", arg), available, activityAttendanceArm(p, alias, arg))
}

// activityAttendanceArm is the discovery half of membership: the caller was on
// this row on evidence they could not have written themselves.
//
// It is deliberately NARROWER than activityMembershipArm, which decides content.
// The difference is one column and it is the whole security of this clause.
//
// Capture stamps the ACTING SEAT as an activity_participant of every activity it
// writes (sinkactivity.go stampCaptureParticipants), with no attestation behind
// it. So "I have a participant row" means only "my connector landed this", and
// admitting that would let a seat discover every row its own connector ever
// captured — including one later filed under a record it may not read, which is
// exactly the state capture refuses to replay onto. Restricting the arm to
// kind='meeting' does NOT close that: the extension ingress copies a unit's
// chosen Kind straight through with no vocabulary check (compose/extingress.go),
// so "meeting" is a word a caller picks.
//
// The two evidenced sources, both of which a caller controls neither half of:
//
//   - capture_import, written only after mailboxWasARecipientTx found one of the
//     seat's OWN exact addresses on the message — the provider delivered it;
//   - a participant row carrying an address, which is written from a party list
//     only when the provider itself enumerated it (capture's
//     ParticipantListAttested). The seat-stamp above carries no address, so the
//     `address IS NOT NULL` test is what tells the two apart.
//
// Content is unaffected: activityAudienceArm keeps the wider membership test, so
// a seat that may already read a row still reads it. This arm only decides who
// may learn a row EXISTS through attendance rather than through its links.
func activityAttendanceArm(p principal.Principal, alias string, arg func(any) int) string {
	me := arg(p.UserID)
	return fmt.Sprintf(`(EXISTS (SELECT 1 FROM capture_import ci WHERE ci.activity_id = %[1]s.id AND ci.user_id = $%[2]d)
	   OR EXISTS (SELECT 1 FROM activity_participant ap
	               WHERE ap.activity_id = %[1]s.id AND ap.user_id = $%[2]d AND ap.address IS NOT NULL))`,
		alias, me)
}

// activityMembershipArm is the "I was on this" test for CONTENT: the caller's
// own seat imported the row, or they are stamped as one of its participants.
//
// Its discovery counterpart is activityAttendanceArm above, which is narrower by
// one column and says why. The two are allowed to differ in exactly that
// direction — content may be granted to a seat that discovery would not admit,
// because a seat reading a row it already captured discloses nothing new, while
// discovery decides whether an UNRELATED row becomes visible at all. They must
// never differ the other way: a row a caller can read must always be one they
// can discover, which holds because the content gate composes discovery whole
// and then ANDs this.
//
// Its existential twin, which asks whether ANYBODY matches before a write
// narrows a row, is ActivityHasAReaderTx in audienceorphan.go; change an arm
// here and change it there.
//
// Deliberately NOT included: the captured_by suffix match and the
// audience='workspace' arm that the audience test also carries. The first names
// one seat's provenance and the second is a statement about the audience rather
// than about who was present.
func activityMembershipArm(p principal.Principal, alias string, arg func(any) int) string {
	me := arg(p.UserID)
	return fmt.Sprintf(`(EXISTS (SELECT 1 FROM capture_import ci WHERE ci.activity_id = %[1]s.id AND ci.user_id = $%[2]d)
	   OR EXISTS (SELECT 1 FROM activity_participant ap WHERE ap.activity_id = %[1]s.id AND ap.user_id = $%[2]d))`,
		alias, me)
}

// ActivityContentClause is the stronger activity gate: discoverable AND the
// caller is in the activity's AUDIENCE. An activity's audience is
// `workspace` (the default — everyone who can discover it reads it),
// `participants` (the humans on it: the capturing mailbox owner, anyone
// stamped as a participant by seat) or `selected` (the participants plus
// the users and teams named in activity_audience_member). Every reader that
// serves content — subject, body, attachments, participants, thread, and
// everything derived from them: search, embeddings, briefs, exports,
// webhooks — composes this one.
//
// The audience is a property of the ROW a human set, so it does not yield
// to row_scope=all: an admin reading a colleague's limited mail is the
// disclosure the limit exists to prevent. Only the system principal — the
// relay, the privacy engines, the indexers writing on behalf of nobody —
// reads the audience arm away. The arms sit behind the cheap audience
// column test, so the EXISTS probes run for limited rows alone.
func ActivityContentClause(ctx context.Context, alias string, arg func(any) int) (string, error) {
	p, err := rbacActor(ctx)
	if err != nil {
		return "", err
	}
	discover := activityDiscoverClause(p, alias, arg)
	if p.Type == principal.PrincipalSystem {
		return discover, nil
	}
	return discover + " AND " + activityAudienceArm(p, alias, arg), nil
}

// ActivityAudienceArm renders the audience membership test alone — the
// predicate a DISCOVER-gated reader projects as a column to say, per row,
// whether the caller may read its content (content_state). It is TRUE for the
// system principal, which reads every audience away. alias names the activity
// table in the outer query.
func ActivityAudienceArm(ctx context.Context, alias string, arg func(any) int) (string, error) {
	p, err := rbacActor(ctx)
	if err != nil {
		return "", err
	}
	if p.Type == principal.PrincipalSystem {
		return "TRUE", nil
	}
	return activityAudienceArm(p, alias, arg), nil
}

// activityAudienceArm renders the audience membership test for one human (or
// the human behind an agent). The participant arms hold for every audience:
// a contact on a conversation always reads it, limited or not.
//
// Its existential twin — does ANYBODY match, which is the question an audience
// write owes before it narrows a row — is ActivityHasAReaderTx in
// audienceorphan.go. Change an arm here and change it there; the pair is held by
// a test named in that file's doc comment.
func activityAudienceArm(p principal.Principal, alias string, arg func(any) int) string {
	me := arg(p.UserID)
	// captured_by is `human:<uuid>` for a hand-logged row and
	// `connector:<name>:<uuid>` for a captured one; both end in the user's
	// id, so one suffix match names the author and the mailbox owner alike.
	//
	// It names ONE seat, though, and a message can arrive in several mailboxes
	// while the row records only whichever sync landed first. capture_import is
	// the per-mailbox record, so the arm beside it admits every seat whose own
	// mailbox delivered the message rather than only the first — the colleague
	// cc'd on a held thread whose own sync ran second used to fall through to
	// the participant arm alone, which is a different question (were you on the
	// mail) from this one (was this your mail).
	author := arg("%:" + p.UserID.String())
	teams := arg(p.TeamIDs)
	// The two membership arms are activityMembershipArm's, composed rather than
	// repeated: discovery asks the same question of the same two tables, and a
	// second copy is how one of them gains an arm the other lacks.
	return fmt.Sprintf(`(%[1]s.audience = 'workspace'
	   OR %[1]s.captured_by LIKE $%[3]d
	   OR %[5]s
	   OR (%[1]s.audience = 'selected' AND EXISTS (
	      SELECT 1 FROM activity_audience_member am WHERE am.activity_id = %[1]s.id
	        AND ((am.subject_type = 'user' AND am.subject_id = $%[2]d)
	          OR (am.subject_type = 'team' AND am.subject_id = ANY($%[4]d))))))`,
		alias, me, author, teams, activityMembershipArm(p, alias, arg))
}

// EnsureActivityVisible is EnsureVisible for activities under the DISCOVER
// gate: the caller may learn the row exists. A caller about to hand back
// content uses EnsureActivityContentVisible. Out of scope reads as
// ErrNotFound.
func EnsureActivityVisible(ctx context.Context, tx pgx.Tx, id ids.UUID) error {
	return ensureActivity(ctx, tx, id, ActivityDiscoverClause, false)
}

// EnsureActivityVisibleLive is EnsureActivityVisible with the two
// strictnesses a caller serving STORED data needs — the row must still be
// live, and an unbounded actor does not skip the probe. See EnsureVisibleLive.
func EnsureActivityVisibleLive(ctx context.Context, tx pgx.Tx, id ids.UUID) error {
	return ensureActivity(ctx, tx, id, ActivityDiscoverClause, true)
}

// EnsureActivityContentVisible is the single-row CONTENT gate: discoverable
// and in the caller's audience. A limited activity the caller may discover
// but not read answers ErrNotFound here exactly like an invisible one — the
// existence-hiding probe has one answer, and a reader that wants to render
// a withheld marker lists with ActivityDiscoverClause and projects
// content_state instead of probing twice.
func EnsureActivityContentVisible(ctx context.Context, tx pgx.Tx, id ids.UUID) error {
	return ensureActivity(ctx, tx, id, ActivityContentClause, false)
}

// EnsureActivityContentVisibleLive is EnsureActivityContentVisible with the
// live-row strictness of EnsureVisibleLive.
func EnsureActivityContentVisibleLive(ctx context.Context, tx pgx.Tx, id ids.UUID) error {
	return ensureActivity(ctx, tx, id, ActivityContentClause, true)
}

type activityClause func(ctx context.Context, alias string, arg func(any) int) (string, error)

// ensureActivity is the one probe behind the four spellings above. It is
// never skipped, even for an unbounded principal: the clause always carries
// the availability test, and a restricted row must read as gone to everyone.
func ensureActivity(ctx context.Context, tx pgx.Tx, id ids.UUID, clauseFor activityClause, live bool) error {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	idPos := arg(id)

	clause, err := clauseFor(ctx, "a", arg)
	if err != nil {
		return err
	}
	if live {
		return probeExistsLive(ctx, tx, "activity a", "a", idPos, clause, args)
	}
	var visible bool
	err = tx.QueryRow(ctx,
		fmt.Sprintf(`SELECT EXISTS (SELECT 1 FROM activity a WHERE a.id = $%d AND %s)`, idPos, clause),
		args...).Scan(&visible)
	if err != nil {
		return err
	}
	if !visible {
		return apperrors.ErrNotFound
	}
	return nil
}

// probeExistsLive is the one spelling of "this row exists, is not archived,
// and the caller may see it" for the aliased scope probes. An empty clause
// narrows nothing but never SKIPS the probe: that skip is what would let an
// unbounded actor be handed a row that is gone.
func probeExistsLive(ctx context.Context, tx pgx.Tx, from, alias string, idPos int, clause string, args []any) error {
	q := fmt.Sprintf(`SELECT EXISTS (SELECT 1 FROM %s WHERE %s.id = $%d AND %s.archived_at IS NULL`,
		from, alias, idPos, alias)
	if clause != "" {
		q += " AND " + clause
	}
	q += ")"

	var visible bool
	if err := tx.QueryRow(ctx, q, args...).Scan(&visible); err != nil {
		return err
	}
	if !visible {
		return apperrors.ErrNotFound
	}
	return nil
}

// RelationshipEndpointScope is the edge analogue: a relationship owns no
// owner_id, and its sensitivity is the CONJUNCTION of its endpoints' — an edge
// names two records, so one readable by someone who cannot read either end would
// disclose that record's existence and its link to the other.
//
// Every non-null endpoint must be visible under the caller's row scope, on read
// exactly as on write. Only a caller unbounded over EVERY endpoint table carries
// no clause; contact and company hold capture privacy, so that is the system
// principal alone.
//
// It lives here rather than in contacts for the reason the two rules above do:
// scope policy has exactly one spelling (ADR-0054 §8), and this rule now has two
// readers in different modules — contacts's own list and read SQL, and the
// approvals inbox, which must decide whether a staged archive of an edge is
// visible to the human being asked to approve it. A second copy of a conjunction
// is a second place for one of its five arms to be forgotten.
//
// alias names the relationship table in the outer query.
func RelationshipEndpointScope(ctx context.Context, alias string, arg func(any) int) (string, error) {
	p, err := rbacActor(ctx)
	if err != nil {
		return "", err
	}
	if UnboundedFor(p, relationshipEndpoints()...) {
		return "", nil
	}
	clauses := make([]string, 0, len(relationshipEndpointColumns))
	for _, endpoint := range relationshipEndpointColumns {
		predicate := VisiblePredicate(p, endpoint.table, arg)
		clauses = append(clauses, fmt.Sprintf(
			`(%[1]s.%[2]s IS NULL OR EXISTS (
			   SELECT 1 FROM %[3]s ep WHERE ep.id = %[1]s.%[2]s AND ep.archived_at IS NULL AND %[4]s))`,
			alias, endpoint.column, endpoint.table, predicate("ep"),
		))
	}
	return "(" + strings.Join(clauses, " AND ") + ")", nil
}

// relationshipEndpointColumns is every endpoint an edge can carry, paired with
// the table it points at. Two columns point at `company`, which is why this
// is a slice and not a map.
var relationshipEndpointColumns = []struct{ column, table string }{
	{contactIDColumn, tableContact},
	{"counterparty_contact_id", tableContact},
	{companyIDColumn, tableCompany},
	{"counterparty_company_id", tableCompany},
	{dealIDColumn, tableDeal},
	{projectIDColumn, tableProject},
}

// relationshipEndpoints is the distinct endpoint TABLES, for the unbounded
// short-circuit. Derived from the column list so the two cannot disagree about
// which tables an edge can reach.
func relationshipEndpoints() []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(relationshipEndpointColumns))
	for _, endpoint := range relationshipEndpointColumns {
		if seen[endpoint.table] {
			continue
		}
		seen[endpoint.table] = true
		out = append(out, endpoint.table)
	}
	return out
}

// EnsureRelationshipVisible probes one edge under the endpoint-conjunction rule.
// Absence and out-of-scope answer identically (existence-hiding), which is what
// lets the approvals inbox ask "may this human see the edge this archive targets"
// without disclosing that the edge exists.
//
// Archived edges still answer visible, matching the clause the owning store uses:
// an approval staged against an edge that was archived in the meantime stays
// DECIDABLE, so a human can reject it rather than find an undecidable row.
func EnsureRelationshipVisible(ctx context.Context, tx pgx.Tx, id ids.UUID) error {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	idPos := arg(id)

	clause, err := RelationshipEndpointScope(ctx, "r", arg)
	if err != nil {
		return err
	}
	q := fmt.Sprintf(`SELECT EXISTS (SELECT 1 FROM relationship r WHERE r.id = $%d`, idPos)
	if clause != "" {
		q += " AND " + clause
	}
	q += ")"

	var visible bool
	if err := tx.QueryRow(ctx, q, args...).Scan(&visible); err != nil {
		return err
	}
	if !visible {
		return apperrors.ErrNotFound
	}
	return nil
}
