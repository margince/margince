// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The inbox read side: the store row shape and the List/Get queries.
// Every read here runs through decidable (authority.go), so triage
// visibility and the decision gate can never drift apart.

package approvals

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// row is the store shape of one approval.
type row struct {
	ID         ids.ApprovalID
	Kind       string
	Status     string
	ProposedBy string
	OnBehalfOf *ids.UserID
	PassportID *ids.PassportID
	// TargetType + TargetID are the polymorphic pointer to the entity the
	// staging acts on (deal, org, person, lead, activity, …); the id stays
	// untyped because the pair IS the discriminated reference.
	TargetType    *string
	TargetID      *ids.UUID
	TargetVersion *int64
	// TargetLabel is what the target was CALLED when the proposal was staged.
	// Nil where the type has no name, or where the row had gone by then — the
	// card says nothing rather than saying unknown.
	TargetLabel    *string
	Summary        *string
	ProposedChange json.RawMessage
	DiffHash       string
	ExpiresAt      time.Time
	DecidedBy      *ids.UserID
	DecidedAt      *time.Time
	ConsumedAt     *time.Time
	CreatedAt      time.Time
	// BundleID is the act that staged this row together with its siblings, nil
	// for one staged alone (bundle.go).
	BundleID *ids.UUID
	// Evidence is what each claim was read out of, nil for a staging that read
	// nothing (evidence.go).
	Evidence json.RawMessage
	// EffectFailedAt + EffectFailure are the mark decide.go leaves on an
	// approved row whose released work did not run: when, and the sentence a
	// reader is shown. Nil on every row whose effect ran.
	EffectFailedAt *time.Time
	EffectFailure  *string
}

const columns = `id, kind, status, proposed_by, on_behalf_of, passport_id,
	target_entity_type, target_entity_id, target_version, target_label, summary,
	proposed_change, diff_hash, expires_at, decided_by, decided_at, consumed_at, created_at,
	bundle_id, evidence, effect_failed_at, effect_failure`

func scan(r pgx.Row) (row, error) {
	var a row
	err := r.Scan(&a.ID, &a.Kind, &a.Status, &a.ProposedBy, &a.OnBehalfOf, &a.PassportID,
		&a.TargetType, &a.TargetID, &a.TargetVersion, &a.TargetLabel, &a.Summary,
		&a.ProposedChange, &a.DiffHash, &a.ExpiresAt, &a.DecidedBy, &a.DecidedAt, &a.ConsumedAt, &a.CreatedAt,
		&a.BundleID, &a.Evidence, &a.EffectFailedAt, &a.EffectFailure)
	return a, err
}

// effectiveStatus folds lazy expiry in: a pending row past its expiry
// reads as expired everywhere without a sweeper process.
//
// Except for the kinds that do not expire at all. Expiry answers "has this
// intention gone stale against the state it was formed on" — a real question
// about a PROPOSAL, and a meaningless one about a staging whose subject is
// simply still waiting. Those kinds are excluded from the comparison rather than
// given a distant expires_at, because a date is a cliff however far away it is
// put, and the invariant here is that there is no cliff (ExpiresNever).
func (a row) effectiveStatus(now time.Time) string {
	if a.Status == statusPending && !ExpiresNever(a.Kind) && now.After(a.ExpiresAt) {
		return StatusExpired
	}
	return a.Status
}

// inboxBatch is the scan window List filters per round trip; List keeps
// paging until the display limit is met or the table is exhausted, so a
// burst of undecidable stagings can never starve older visible rows out
// of a caller's inbox.
const inboxBatch = 200

// PendingScanCap bounds how many staged rows PendingForTarget scans for one
// record. Decidability is a per-row probe, so an unbounded target would make
// a single record page pay for the whole backlog. A caller that counts must
// read a full result as "this many or more" rather than as an exact total.
const PendingScanCap = inboxBatch

// statusPending is the status column value a staged, undecided row carries.
const statusPending = "pending"

// ListInput narrows an inbox read. Status and Kind filter the staged rows
// themselves; TargetType + TargetID scope the question to ONE record and are
// a pair — a type alone would match every record of that type and an id alone
// every type of that id, so the transport refuses a half-reference and one
// never reaches here.
type ListInput struct {
	Status     *string
	Kind       *string
	TargetType *string
	TargetID   *ids.UUID
	// BundleID narrows the read to the proposals ONE act staged together. It
	// composes with the other filters rather than replacing them, and it does
	// not relax the per-row decidability probe: a member the caller could not
	// decide is as absent here as it is from the unfiltered inbox.
	BundleID *ids.UUID
	// DecidedBySystem narrows to what ran with nobody asked — the receipts a
	// "done for you" surface may claim.
	//
	// It reads the decision's own marker rather than inferring the answer from
	// an empty decided_by. Two reasons the inference was wrong: no writer
	// produces approved-with-no-decider, so the test matched nothing; and
	// decided_by is emptied by ON DELETE SET NULL when an app_user is deleted,
	// which would relabel that person's decisions as the system's own.
	//
	// Asked in SQL because the caller bounds its read: filtering after a page
	// puts the limit on the wrong set, and a page full of ordinary decisions
	// narrows to nothing while real receipts sit behind it.
	DecidedBySystem *bool
	// DecidedAfter keeps decisions made since an instant.
	//
	// It travels with DecidedBySystem for one reason: the page is ordered by
	// created_at, and a receipt's window is about decided_at. Those disagree —
	// an approval staged last week and decided this morning sorts BELOW one
	// staged this morning and decided days ago — so a window applied after the
	// page can discard everything the page held and hide the recent decision
	// underneath. In SQL, the limit only ever counts rows inside the window.
	DecidedAfter *time.Time
	// FailedForDecider narrows to the approved rows THIS caller decided
	// whose released work then failed (decide.go's mark). A flag rather
	// than a caller-supplied decider id — List binds the acting user
	// itself, so nobody reads another person's failures by naming them.
	FailedForDecider bool
	Limit            int
	// Cursor continues a previous page: the opaque keyset token that page
	// reported as next_cursor. Empty starts at the newest row.
	Cursor string
	// failedDecider is FailedForDecider resolved to the acting user;
	// set by List from the principal, never by a caller.
	failedDecider *ids.UUID
}

// targeted reports whether the read is scoped to one record.
func (in ListInput) targeted() bool { return in.TargetType != nil && in.TargetID != nil }

// List returns the inbox, newest first — but only the approvals the caller
// could themselves decide. Deciding is human work, and so is triage: an
// agent cannot browse the queue of withheld authority, and neither can a
// human who lacks the grant the staged effect needs or could not read the
// staged target itself. Without this filter the inbox is
// a workspace-wide side channel that leaks proposed_change, target ids,
// and diffs to any low-privilege user (C3/ADR-0036).
//
// The Page is has_more and the token to continue with. A record page can carry
// dozens of stagings — one deep site read stages a proposal per person it found
// — so a client that filtered to one record has to be able to tell a full page
// from a complete answer, AND to ask for the rest. has_more without a cursor
// would only tell them rows are missing.
func (s *Service) List(ctx context.Context, in ListInput) ([]row, storekit.Page, error) {
	if err := actingForAHuman(ctx); err != nil {
		return nil, storekit.Page{}, err
	}
	p, _ := principal.Actor(ctx)
	if in.FailedForDecider {
		if p.UserID.IsZero() {
			return nil, storekit.Page{}, apperrors.ErrPermissionDenied
		}
		in.failedDecider = &p.UserID
	}
	if in.Limit <= 0 || in.Limit > inboxBatch {
		in.Limit = 50
	}
	start, err := startOf(in.Cursor)
	if err != nil {
		return nil, storekit.Page{}, err
	}
	var out []row
	var page storekit.Page
	err = s.db.Tx(ctx, func(tx pgx.Tx) (err error) {
		if in.targeted() {
			out, page, err = listForTarget(ctx, tx, p, in, start)
			return err
		}
		// The caller's own failed decisions skip the per-row decidability
		// probe: it asks "could this caller decide this NOW", and a failure
		// must not vanish when that answer changes — target archived, grant
		// narrowed — because the row exists to tell its decider that work
		// THEY released did not run. Scope is decided_by = the acting user.
		if in.failedDecider != nil {
			out, page, err = scanOwnDecisions(ctx, tx, in, start)
			return err
		}
		out, page, err = scanInbox(ctx, tx, p, in, start)
		return err
	})
	if err != nil {
		return nil, storekit.Page{}, err
	}
	return out, page, nil
}

// scanInbox walks the whole table newest-first and filters each keyset batch
// through the per-row decidability probe.
//
// Decidability is role/target/row-scope-shaped, not expressible as one WHERE
// without joining every object grant — so it runs in memory, and the scan
// pages rather than taking one wide LIMIT: a burst of undecidable stagings
// must never starve older visible rows out of a caller's inbox.
//
// It fills one row PAST the display limit so has_more is a fact rather than a
// guess; that row is then dropped, and its existence is what the flag reports.
//
// start is the caller's cursor: the scan begins after the last row the previous
// page returned, so the batches walk forward through the table instead of
// re-filtering the newest rows on every request.
func scanInbox(ctx context.Context, tx pgx.Tx, p principal.Principal, in ListInput, start *keysetStart) ([]row, storekit.Page, error) {
	decide := func(a row) (bool, error) { return decidable(ctx, tx, p, a) }
	var out []row
	from := start
	for {
		q, args := approvalPageQuery(in, from)
		batch, err := collect(ctx, tx, q, args)
		if err != nil {
			return nil, storekit.Page{}, err
		}
		var full bool
		out, full, err = appendDecidable(batch, out, in.Limit+1, decide)
		if err != nil {
			return nil, storekit.Page{}, err
		}
		if full || len(batch) < inboxBatch {
			break // a row past the display limit is in hand, or the table is exhausted
		}
		from = after(batch[len(batch)-1])
	}
	return capPage(out, in.Limit, nil)
}

// listForTarget answers the inbox scoped to ONE record.
//
// Every row shares that target, so the target-visibility half of decidable is
// asked ONCE for the record rather than once per row — the inbox's per-row
// probe exists only because its rows point at different records. The per-kind
// grant check still varies by row and stays in the loop.
//
// A target the caller could not read — its type ungranted, or its row outside
// their scope — answers an EMPTY list, never a refusal: nothing staged against a
// record they cannot see is decidable, and saying so is the same
// existence-hiding answer the record's own read gives.
//
// The scan is bounded at PendingScanCap, so a full scan is also a reason to
// report has_more: past the cap this read cannot tell a client it has seen
// everything, and claiming otherwise is the lie the flag exists to prevent.
func listForTarget(ctx context.Context, tx pgx.Tx, p principal.Principal, in ListInput, start *keysetStart) ([]row, storekit.Page, error) {
	visible, err := targetVisible(ctx, tx, in.TargetType, in.TargetID)
	if err != nil {
		return nil, storekit.Page{}, err
	}
	if !visible {
		return []row{}, storekit.Page{}, nil
	}
	q, args := approvalPageQuery(in, start)
	batch, err := collect(ctx, tx, q, args)
	if err != nil {
		return nil, storekit.Page{}, err
	}
	var scanned *row
	if len(batch) == PendingScanCap {
		scanned = &batch[len(batch)-1]
	}
	granted := func(a row) (bool, error) { return requireDecisionGrants(p, a) == nil, nil }
	out, _, err := appendDecidable(batch, nil, in.Limit+1, granted)
	if err != nil {
		return nil, storekit.Page{}, err
	}
	return capPage(out, in.Limit, scanned)
}

// approvalWhere is the ONE spelling of "which staged rows this read wants":
// the caller's filters, plus the keyset cursor of the previous batch when the
// scan is paging. Every read of the approval table renders its predicate here,
// so a filter added to the surface reaches the inbox, the target-scoped list
// and the record page together instead of drifting between copies.
func approvalWhere(in ListInput, from *keysetStart, arg func(any) int) string {
	var terms []string
	if in.Status != nil {
		terms = append(terms, fmt.Sprintf("status = $%d", arg(*in.Status)))
	}
	if in.Kind != nil {
		terms = append(terms, fmt.Sprintf("kind = $%d", arg(*in.Kind)))
	}
	if in.TargetType != nil {
		terms = append(terms, fmt.Sprintf("target_entity_type = $%d", arg(*in.TargetType)))
	}
	if in.TargetID != nil {
		terms = append(terms, fmt.Sprintf("target_entity_id = $%d", arg(*in.TargetID)))
	}
	if in.BundleID != nil {
		terms = append(terms, fmt.Sprintf("bundle_id = $%d", arg(*in.BundleID)))
	}
	if in.DecidedBySystem != nil {
		terms = append(terms, fmt.Sprintf("decided_by_system = $%d", arg(*in.DecidedBySystem)))
	}
	if in.DecidedAfter != nil {
		terms = append(terms, fmt.Sprintf("decided_at > $%d", arg(*in.DecidedAfter)))
	}
	if in.failedDecider != nil {
		terms = append(terms, fmt.Sprintf("effect_failed_at IS NOT NULL AND decided_by = $%d", arg(*in.failedDecider)))
	}
	if from != nil {
		terms = append(terms, fmt.Sprintf("(created_at, id) < ($%d, $%d)", arg(from.createdAt), arg(from.id)))
	}
	if len(terms) == 0 {
		return ""
	}
	return " WHERE " + strings.Join(terms, " AND ")
}

// approvalPageQuery is one newest-first page of the scan under those filters.
//
// Every reader scans the same window: the inbox pages through it until the
// display limit is met, and a target-scoped read takes one window as its cap
// (PendingScanCap). One bound, so the two can never drift into disagreeing
// about how deep "we looked" goes.
func approvalPageQuery(in ListInput, from *keysetStart) (string, []any) {
	args := []any{}
	arg := func(v any) int { args = append(args, v); return len(args) }
	where := approvalWhere(in, from, arg)
	return fmt.Sprintf(`SELECT %s FROM approval%s ORDER BY created_at DESC, id DESC LIMIT %d`,
		columns, where, inboxBatch), args
}

// appendDecidable filters one scanned batch through a visibility probe and
// appends the rows that pass, stopping the moment limit is met (full = true)
// so a burst of undecidable stagings cannot starve older visible rows out of
// the caller's inbox.
//
// The probe is a parameter because the two readers differ in exactly one half:
// the inbox asks the whole decidable predicate per row, while a target-scoped
// read has already established that one target's visibility for every row and
// asks only the per-kind grants.
func appendDecidable(batch, out []row, limit int, visible func(row) (bool, error)) ([]row, bool, error) {
	for i := range batch {
		a := batch[i]
		ok, err := visible(a)
		if err != nil {
			return out, false, err
		}
		if !ok {
			continue
		}
		out = append(out, a)
		if len(out) >= limit {
			return out, true, nil
		}
	}
	return out, false, nil
}

// collect materializes one query's rows (the row-scope probes inside the
// filter loop need the connection, so the cursor cannot stay open).
func collect(ctx context.Context, tx pgx.Tx, q string, args []any) ([]row, error) {
	rows, err := tx.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []row
	for rows.Next() {
		a, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Service) Get(ctx context.Context, id ids.ApprovalID) (row, error) {
	if err := actingForAHuman(ctx); err != nil {
		return row{}, err
	}
	p, _ := principal.Actor(ctx)
	var a row
	err := s.db.Tx(ctx, func(tx pgx.Tx) (err error) {
		a, err = get(ctx, tx, id)
		if err != nil {
			return err
		}
		// An approval the caller could not decide reads as absent — the
		// same existence-hiding the row-scope convention uses, so Get never
		// becomes a lookup oracle for out-of-scope proposed changes (C3),
		// whether the gap is a missing grant or a target row outside the
		// caller's row scope.
		visible, err := decidable(ctx, tx, p, a)
		if err != nil {
			return err
		}
		if !visible {
			return apperrors.ErrNotFound
		}
		return nil
	})
	if err != nil {
		return row{}, err
	}
	return a, nil
}

func get(ctx context.Context, tx pgx.Tx, id ids.ApprovalID) (row, error) {
	a, err := scan(tx.QueryRow(ctx, `SELECT `+columns+` FROM approval WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return row{}, apperrors.ErrNotFound
	}
	return a, err
}
