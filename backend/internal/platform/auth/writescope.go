// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

// Write authority over one row (A52/ADR-0039, UC-E11-08 E2). rowscope.go
// answers "which rows may this caller SEE"; this file answers the narrower
// question a mutation asks: "is this caller's own authority over the row
// write-level".
//
// The two differ in exactly one arm. A manual grant widens visibility whatever
// its access level — write satisfies read, so any live grant should let its
// holder open the record — but only `write` widens the authority to CHANGE it.
// The visibility predicate counts every live grant by design, so a mutation
// that gates on visibility alone accepts a read share as a licence to write,
// which is what the sharing screen tells the user it is not.
//
// Everything here is the second half of a pair. A write-authority probe is a
// NARROWING of a visibility probe, never a substitute for one: on its own it
// would answer "yes" for a row the caller cannot see at all (a non-shareable
// table, or a system principal), and it would answer ErrPermissionDenied where
// existence-hiding owes ErrNotFound. So the exported spellings below run the
// visibility probe first and keep its 404, then narrow with the write arm and
// answer ErrPermissionDenied — the caller has already been told the row is
// theirs to read, so there is nothing left for a 404 to hide.

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// grantAccessWrite is the wider of record_grant's two access levels. Spelled
// once here because this file both compares against it and embeds it in SQL.
const grantAccessWrite = "write"

// EnsureWritable is EnsureVisible for a path that CHANGES the row: the caller
// must see it, and their authority over it must be write-level. It is the
// spelling every record mutation's row gate uses — update, archive, advance,
// promote, disqualify — so a `read` share cannot pass one.
//
// The two probes run as separate statements in the caller's transaction, and
// that is safe in the direction it has to be: both must pass, and each is a
// narrowing, so a share revoked between them can only turn an admission into a
// refusal. There is no interleaving that admits a caller neither statement
// alone would.
func EnsureWritable(ctx context.Context, tx pgx.Tx, table string, id ids.UUID) error {
	if err := EnsureVisible(ctx, tx, table, id); err != nil {
		return err
	}
	return ensureWriteAuthority(ctx, tx, table, id)
}

// EnsureWritableLive is EnsureVisibleLive's write-authority twin — the row must
// exist, be live, and be the caller's to change. Everything EnsureVisibleLive's
// own comment says about why the live filter is load-bearing applies here
// unchanged; this adds only the grant-access arm.
//
// THIS IS THE SPELLING A WRITE THAT ADDS OWES, and the rule is stated here once
// rather than re-argued at each gate: archived means frozen, so a write that
// puts something NEW on a record needs the record to still be live and not
// merely visible.
//
// A path that STAGES a proposal and applies it later needs it most: the archive
// lands inside that window, and the window is the ordinary case rather than a
// race. Art. 17 erasure is the sharpest form — erasure stamps archived_at and
// leaves the row standing, so a probe without the live filter still answers
// "yours" for a subject every live read path now refuses, and an apply arriving
// afterwards refills a declared-PII table the erasure had cleared.
//
// It is deliberately NOT stated as "any child of an archived anchor is frozen",
// which is the wider rule it looks like. A gate shared by writes that GRANT and
// writes that REVOKE cannot take this probe wholesale: freezing a revocation
// because its anchor was retired is the opposite of what retiring it meant. The
// half this probe does not cover is EnsureRetractable, and the pair is chosen by
// what is being written rather than by the table alone.
//
// Where a caller must reach an archived row ON PURPOSE — Art. 17 erasure, the
// retention sweep, the archive transition itself, a merge retiring its source,
// or a refusal path that writes nothing — it uses EnsureWritable and says why
// at the call site.
//
// It NARROWS the window rather than closing it, and a caller whose write would
// be harmful on the far side of it owes LockSubjectLive as well. This reads a
// snapshot; the write happens in a later statement of the same transaction, and
// under READ COMMITTED an archive or an erasure committing in between lands
// anyway. For most callers the
// residue is a stale write. Where it is a live capability or a PII row an
// erasure had just cleared, it is not, and the lock is what makes the two take
// turns. Which writers owe it is derived in backend/liveprobelock_test.go.
func EnsureWritableLive(ctx context.Context, tx pgx.Tx, table string, id ids.UUID) error {
	if err := EnsureVisibleLive(ctx, tx, table, id); err != nil {
		return err
	}
	return ensureWriteAuthority(ctx, tx, table, id)
}

// EnsureRetractable is EnsureWritableLive's twin for the writes that RELEASE
// rather than add — revoke, void, cancel, retract, delist. The row must be
// visible and the caller's to change, and it deliberately does NOT have to be
// live.
//
// It runs exactly the probes EnsureWritable runs, and the difference is that it
// says so. A gate reading EnsureWritable cannot tell a deliberate reach for an
// archived row from a forgotten live filter, and every archived-write defect
// this pair was written for was individually defensible where it sat. Naming
// the choice is what makes a new operation pick a side at its own call site,
// the way consent.Record picks one on the state it is recording.
//
// The rule it holds: an archived anchor freezes what its children GRANT,
// PUBLISH or ACCRUE, and never freezes what they REVOKE, VOID, CANCEL or
// RETRACT. A retraction against a retired record is not a change to the record
// — it is the cleanup the retirement implies, so freezing one protects nobody.
// The decisive case is a deal room: archiving the deal must not take away the
// ability to cut off a buyer's access to it, which is the moment somebody
// reaches for it.
//
// This is the same asymmetry consent already applies one level down — refuse
// the claim, allow the withdrawal — so a caller weighing which spelling it owes
// asks what the write DOES, never which table it lands on.
func EnsureRetractable(ctx context.Context, tx pgx.Tx, table string, id ids.UUID) error {
	return EnsureWritable(ctx, tx, table, id)
}

// HoldWritableLive is EnsureWritableLive and LockSubjectLive as one decision,
// which is how every entry point that owes both should ask: probe, then hold,
// with nothing in between that could take another row lock first.
//
// The order inside is the deadlock rule. The eraser is subject-first, so the
// subject must be the first row this transaction holds; a caller that locked
// something else on the way here has already lost that guarantee, which is why
// this is one call rather than two the caller sequences.
func HoldWritableLive(ctx context.Context, tx pgx.Tx, table string, id ids.UUID) error {
	if err := EnsureWritableLive(ctx, tx, table, id); err != nil {
		return err
	}
	return LockSubjectLive(ctx, tx, table, id)
}

// LockSubjectLive holds the subject's row for the rest of the transaction and
// refuses if it is not live, so a write that follows cannot be overtaken by an
// archive or an Art. 17 erasure the probe could not see.
//
// It is NOT storekit.LockRow, which runs the same SELECT … FOR UPDATE and
// answers the same sentinel, and the reason is the layer rather than the SQL.
// LockRow is a STORE helper: tableownership_test.go counts its table argument as
// a write by that module and writeauthorityreach_test.go counts it as a mutation
// owing a row probe. Both are right for the 58 sites where a module locks its
// own row before patching it. Every call here locks somebody ELSE'S subject —
// consent locking person, activities locking a polymorphic parent — purely to
// refuse a race, writing nothing. Routed through LockRow, five reads that mutate
// nothing would each need a cross-store write ratification and a probe waiver.
// Row-level authority over another module's subject is what this package is for,
// so the lock lives beside the probe it completes.
//
// The lock is taken at the WRITE rather than inside EnsureWritableLive, where
// every live-probed path would take it. That probe runs at the top of two dozen
// transactions, several in `people`, where a documented order already exists and
// renamerecheck.go records a deadlock found only by review when a row lock was
// taken out of turn. Locking in the primitive adds an edge to every one of those
// orders at once; locking at the write adds it only where the residue is
// harmful. Call sites take it BEFORE any other row lock in their transaction,
// because the eraser is subject-first and the opposite order closes a cycle.
//
// FOR NO KEY UPDATE rather than FOR UPDATE: the subject is a parent row other
// tables reference, and this must not block a child insert taking a foreign key
// to it (FOR KEY SHARE). It conflicts with everything else — the archive and the
// erasure, which UPDATE the row, and any concurrent lock on the same subject.
//
// Held by: TestALockedSubjectMakesTheEraserWait and
// TestLockSubjectLiveRefusesWhatItCannotHold
// (backend/internal/modules/people/subjectlock_integration_test.go) for what the
// lock does, and TestALiveProbedWriteOfAHeldRowLocksItsSubject
// (backend/liveprobelock_test.go) for which writers owe it.
func LockSubjectLive(ctx context.Context, tx pgx.Tx, table string, id ids.UUID) error {
	// The same closed set every sibling in this file gates on. One call site
	// passes a request-body value (ai.RecordInput.SubjectType), so this is the
	// check rather than a formality: Sanitize below stops an injection, but a
	// table outside this set has no archived_at and would reach the caller as a
	// raw SQL error where a sentinel is owed.
	if !ownerScopedTables[table] {
		return fmt.Errorf("auth: %q is not a row-scoped subject table: %w", table, apperrors.ErrNotFound)
	}
	q := fmt.Sprintf(
		`SELECT 1 FROM %s WHERE id = $1 AND archived_at IS NULL FOR NO KEY UPDATE`,
		pgx.Identifier{table}.Sanitize())
	var held int
	if err := tx.QueryRow(ctx, q, id).Scan(&held); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return apperrors.ErrNotFound
		}
		return err
	}
	return nil
}

// EnsureWritableForSubjectRights is EnsureVisibleForSubjectRights' write-authority
// twin, for the one subject-rights path that DESTROYS rather than reads: Art. 17
// erasure. The capture-privacy arm stays lifted for the reason that function
// gives — an unpromoted captured record is still held, and an erasure that
// silently spared it would be the defect — and the write arm is added on top,
// because a colleague handed a `read` share of a person is not thereby handed
// the authority to erase them.
//
// Its sibling, SAR assembly, deliberately does NOT use this: an export is a
// read, and read authority is the whole of what a read needs.
func EnsureWritableForSubjectRights(ctx context.Context, tx pgx.Tx, table string, id ids.UUID) error {
	if err := EnsureVisibleForSubjectRights(ctx, tx, table, id); err != nil {
		return err
	}
	return ensureWriteAuthority(ctx, tx, table, id)
}

// WritableBy is VisibleTo's write-authority twin: it answers whether the
// caller could CHANGE the row, without erroring. The dedupe and merge paths
// ride it where the answer decides between absorbing an existing record and
// refusing with a bare conflict — a match the caller may read but not rewrite
// is not a match they may merge into, and the conflict must still be reported
// without disclosing the id.
func WritableBy(ctx context.Context, tx pgx.Tx, table string, id ids.UUID) (bool, error) {
	err := EnsureWritable(ctx, tx, table, id)
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, apperrors.ErrNotFound), errors.Is(err, apperrors.ErrPermissionDenied):
		return false, nil
	default:
		return false, err
	}
}

// EnsureCanGrant holds ADR-0039's scope-intersection rule — "a granter can never
// share wider than they hold" — and asks it at EVERY access level, because a
// grant is not only a width. It is also a TERM.
//
// A grant assertion is an upsert on
// (record_type, record_id, subject_type, subject_id) and it restates the whole
// grant: access, expiry, reason and granted_by all take the new request's
// values, which is deliberate and is what the contract documents. So a caller
// admitted at `read` does not merely pass on sight they hold — on an existing
// grant they rewrite its terms, including terms somebody else set. The width of
// the access column is the smaller half of what an assertion decides.
//
// The question is answered by the primitive every mutation rides: a caller who
// could not change the row themselves cannot hand that authority to somebody
// else, nor restate the terms on which somebody else handed it on.
//
// This is deliberately NOT "the caller has SOME claim on the row". On the tables
// every seat reads whole a read claim is universal and would admit everyone;
// write authority is owner- or grant-scoped even there, which is what makes the
// rule bite uniformly across the shareable set rather than only on the tables
// that happen to be row-scoped for reads.
func EnsureCanGrant(ctx context.Context, tx pgx.Tx, table string, id ids.UUID) error {
	// The primitive rejects an unknown name itself, like every sibling here:
	// the record type reaching this comes from a request body, and its caller's
	// own allowlist is a second line rather than the only one.
	if !shareableTables[table] {
		return fmt.Errorf("auth: %q is not a shareable table", table)
	}
	return ensureWriteAuthority(ctx, tx, table, id)
}

// ensureWriteAuthority refuses a caller whose only authority over the row is a
// `read` share. It is unexported because it is not a probe on its own: it never
// asks whether the row is visible, or even whether it exists, so a caller that
// reached for it alone would be gating a mutation on a question with no row in
// it. Every exported spelling above pairs it with a visibility probe first.
//
// A non-shareable row-scoped table (a list, a saved view) answers nil: no grant
// can name one, so the visibility probe that ran first applied the owner scope
// and nothing else, and that scope IS the write authority. Saying so here keeps
// the callers uniform — a mutation asks for write authority whatever its table,
// rather than each call site deciding whether its table can be shared.
func ensureWriteAuthority(ctx context.Context, tx pgx.Tx, table string, id ids.UUID) error {
	if !ownerScopedTables[table] {
		return fmt.Errorf("auth: %q is not a row-scoped table", table)
	}
	p, err := rbacActor(ctx)
	if err != nil {
		return err
	}
	if Unbounded(p) || !shareableTables[table] {
		return nil
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	idPos := arg(id)
	clause := writeAuthorityPredicate(p, table, arg)

	var permitted bool
	if err := tx.QueryRow(ctx,
		fmt.Sprintf(`SELECT EXISTS (SELECT 1 FROM %s WHERE id = $%d AND %s)`, table, idPos, clause),
		args...).Scan(&permitted); err != nil {
		return err
	}
	if !permitted {
		return apperrors.ErrPermissionDenied
	}
	return nil
}

// writeAuthorityPredicate renders the write arm: own/team scope, or a live
// grant that says `write`.
//
// Capture privacy is absent on purpose, and its absence is not a widening. The
// visibility probe this always follows has already applied it, so re-applying
// it here could only refuse a row that probe admitted — which is precisely what
// the subject-rights path must not do, since it lifts capture privacy
// deliberately and for a reason Art. 17 states.
func writeAuthorityPredicate(p principal.Principal, table string, arg func(any) int) string {
	return writeAuthorityPredicateAs(p, table, table, arg)
}

// writeAuthorityPredicateAs renders the write arm against an aliased row —
// the spelling the activity link walk needs, where the record sits under a
// probe alias rather than its own table name.
func writeAuthorityPredicateAs(p principal.Principal, table, alias string, arg func(any) int) string {
	// An ownerless row is nobody's to change: it is claimed first
	// (EnsureClaimable), then written under the owner scope like any other.
	owner := ownerPredicate(p, arg, unownedIsNobodys)(alias)
	me, teams := arg(p.UserID), arg(p.TeamIDs)
	return fmt.Sprintf(`(%s OR EXISTS (
		   SELECT 1 FROM record_grant rg
		   WHERE rg.record_type = '%s' AND rg.record_id = %s.id
		     AND rg.access = '%s'
		     AND (rg.expires_at IS NULL OR rg.expires_at > now())
		     AND ((rg.subject_type = 'user' AND rg.subject_id = $%d)
		       OR (rg.subject_type = 'team' AND rg.subject_id = ANY($%d)))))`,
		owner, table, alias, grantAccessWrite, me, teams)
}

// EnsureActivityWritable is EnsureWritable for an activity, which has no
// owner_id of its own. The caller must READ it (the content gate — a limited
// conversation is nobody else's to edit), and their authority to CHANGE it is
// any of:
//
//   - they authored or captured it (captured_by names their user id);
//   - it is their task or their meeting (assignee_id / host_user_id);
//   - it is a link-less, workspace-shared note;
//   - at least one linked record is theirs to change — the same own/team
//     scope or `write` grant EnsureWritable takes on that record.
//
// Reads of customer identity are shared across the workspace, so the read
// gate alone would let every seat rewrite every colleague's correspondence;
// this is the arm that keeps activity writes team-shaped. An unbounded human
// edits every activity they can read, as they edit every record.
func EnsureActivityWritable(ctx context.Context, tx pgx.Tx, id ids.UUID) error {
	return EnsureActivityWritableIn(ctx, tx, id, true)
}

// EnsureActivityWritableIn is EnsureActivityWritable against a chosen row
// liveness. live=false serves a caller that already resolved the row past
// its own LiveOnly lock and confirmed it is held under a statutory
// retention obligation (activities.lockActivityForWrite).
//
// It skips the content-visible gate's LIVENESS half rather than passing live
// through to it: ActivityAvailableClause is `restricted_at IS NULL`
// UNCONDITIONALLY — by design, a restricted row reads as gone to everyone
// through that gate, live argument or not (ensureActivity's own doc). A
// caller reaching this function with live=false already proved the row
// exists by another means (the row lock, taken directly against the table),
// so re-asking the liveness half would only reproduce the same false 404
// this exists to remove. What it does NOT earn a skip from is the OTHER
// half ActivityContentClause folds in for every non-system caller —
// ActivityAudienceArm, the row's own participants/selected narrowing —
// which the ownership check below cannot stand in for: ownership answers
// "is this the caller's team's record", audience answers "did a human limit
// who reads this ONE message", and a caller who owns a record is not
// thereby a participant on every limited message under it. An unbounded
// human is bound by this too — ActivityContentClause's own doc says only
// the system principal reads the audience arm away, so Unbounded below must
// not become a bypass a held row's write-authority check does not have to
// answer for.
func EnsureActivityWritableIn(ctx context.Context, tx pgx.Tx, id ids.UUID, live bool) error {
	if live {
		if err := ensureActivity(ctx, tx, id, ActivityContentClause, true); err != nil {
			return err
		}
	} else {
		included, err := activityAudienceIncludes(ctx, tx, id)
		if err != nil {
			return err
		}
		if !included {
			return apperrors.ErrNotFound
		}
	}
	p, err := rbacActor(ctx)
	if err != nil {
		return err
	}
	if Unbounded(p) {
		return nil
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	idPos, me, author := arg(id), arg(p.UserID), arg("%:"+p.UserID.String())

	var permitted bool
	if err := tx.QueryRow(ctx, fmt.Sprintf(`
		SELECT EXISTS (SELECT 1 FROM activity a WHERE a.id = $%[1]d AND (
		   a.captured_by LIKE $%[3]d
		   OR a.assignee_id = $%[2]d
		   OR a.host_user_id = $%[2]d
		   OR NOT EXISTS (SELECT 1 FROM activity_link l WHERE l.activity_id = a.id)
		   OR EXISTS (SELECT 1 FROM activity_link l WHERE l.activity_id = a.id AND %[4]s)))`,
		idPos, me, author, linkTargetWritable(p, "l", arg)), args...).Scan(&permitted); err != nil {
		return err
	}
	if !permitted {
		if !live {
			return apperrors.ErrNotFound
		}
		return apperrors.ErrPermissionDenied
	}
	return nil
}

// activityAudienceIncludes probes ONLY ActivityAudienceArm — no liveness, no
// discoverability — for a caller in EnsureActivityWritableIn's live=false
// branch, whose row existence and archived state were already settled by
// its own lock. A row the caller cannot find at all answers false, not an
// error: the same not-found the audience arm itself would give inside the
// ordinary content-visible probe.
func activityAudienceIncludes(ctx context.Context, tx pgx.Tx, id ids.UUID) (bool, error) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	idPos := arg(id)
	audience, err := ActivityAudienceArm(ctx, "a", arg)
	if err != nil {
		return false, err
	}
	var included bool
	err = tx.QueryRow(ctx, fmt.Sprintf(
		`SELECT EXISTS (SELECT 1 FROM activity a WHERE a.id = $%d AND (%s))`, idPos, audience),
		args...).Scan(&included)
	return included, err
}

// linkTargetWritable is linkTargetVisible's write twin: one arm per
// activity_link column, each asking whether the record it points at is the
// caller's to change.
func linkTargetWritable(p principal.Principal, alias string, arg func(any) int) string {
	arms := make([]string, 0, len(linkTargetTables))
	for _, t := range []struct{ column, table, probe string }{
		{"person_id", tablePerson, "wp"},
		{"organization_id", tableOrganization, "wo"},
		{"deal_id", tableDeal, "wd"},
		{"lead_id", tableLead, "wl"},
		{"project_id", tableProject, "wpr"},
	} {
		arms = append(arms, fmt.Sprintf(
			`(%[1]s.%[2]s IS NOT NULL AND EXISTS (SELECT 1 FROM %[3]s %[4]s WHERE %[4]s.id = %[1]s.%[2]s AND %[5]s))`,
			alias, t.column, t.table, t.probe, writeAuthorityPredicateAs(p, t.table, t.probe, arg)))
	}
	return "(" + strings.Join(arms, " OR ") + ")"
}

// EnsureClaimable is the gate in front of taking ownership of a row. A claim
// is permitted on a row the caller can see that is UNOWNED — the case the
// write arm refuses on purpose, so that somebody has to put their name on a
// record before changing it — or that is already theirs to change (a
// reassignment to oneself under existing write authority). A row owned by
// somebody else, with no write grant, answers ErrPermissionDenied: taking it
// over is what a share or an admin's reassignment is for.
func EnsureClaimable(ctx context.Context, tx pgx.Tx, table string, id ids.UUID) error {
	if err := EnsureVisibleLive(ctx, tx, table, id); err != nil {
		return err
	}
	p, err := rbacActor(ctx)
	if err != nil {
		return err
	}
	if Unbounded(p) {
		return nil
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	idPos := arg(id)
	clause := writeAuthorityPredicate(p, table, arg)

	var permitted bool
	if err := tx.QueryRow(ctx,
		fmt.Sprintf(`SELECT EXISTS (SELECT 1 FROM %[1]s WHERE id = $%[2]d AND (%[1]s.owner_id IS NULL OR %[3]s))`, table, idPos, clause),
		args...).Scan(&permitted); err != nil {
		return err
	}
	if !permitted {
		return apperrors.ErrPermissionDenied
	}
	return nil
}
