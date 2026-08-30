// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package runner

// Whether a rep has said yes to an agent working overnight on their behalf.
//
// The table RECORDS a decision; it never confers authority. What lets an agent
// act is the passport, and the single production mint binds on_behalf_of and
// granted_by to the same session user — so the credential named here is always
// one the rep minted themselves. An admin turning the feature on for the
// workspace is a separate fact stored as a setting, deliberately not merged
// with this one: merging them is how an admin ends up granting on somebody's
// behalf.
//
// WHY A DECLINED ROW EXISTS. The product asks once. A rep who declined has no
// passport, and neither has a rep who was never asked — from the passport table
// alone the two are the same, and a product that cannot tell them apart asks
// the declining rep again every night.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The two answers a rep can give. A row exists only once they have answered.
//
// There is deliberately no stored "lapsed" state. Whether the credential is
// still usable is the PASSPORT's fact, and it changes without anything writing
// to this table — a passport expires at a moment nobody observes. A stored
// state would have to be kept in step with that, and the two would disagree the
// first time an expiry passed with no sweep running; every reader is then
// holding a state that says "granted" about a credential that stopped working
// hours ago. So the state records the ANSWER, which only the rep changes, and
// liveness is read from the passport at the moment it matters.
const (
	GrantStateGranted  = "granted"
	GrantStateDeclined = "declined"
)

// The field names an audit image of this row carries.
const (
	auditFieldAgentSpec  = "agent_spec"
	auditFieldState      = "state"
	auditFieldPassportID = "passport_id"
)

// StandingGrant is one rep's answer for one agent.
type StandingGrant struct {
	UserID ids.UserID
	Spec   string
	State  string
	// PassportID is the rep's own credential, present exactly when granted —
	// the schema pairs the two, so a nil here means the rep declined.
	PassportID *ids.PassportID
	// CredentialUsable is whether that passport is live RIGHT NOW: not revoked,
	// not expired. Read from the passport rather than stored here, because it
	// changes at a moment nothing writes to this table.
	CredentialUsable bool
	// PassportScopes is what the credential was minted with. Reported rather
	// than judged: this package knows what a passport HOLDS, and the module
	// that mints them knows what an agent NEEDS. A passport minted before an
	// agent gained a tool is short, and neither fact alone can say so.
	PassportScopes []string
	DecidedAt      time.Time
}

// Live reports whether this grant can authorize a run right now: the rep said
// yes, and the credential they minted is still usable.
func (g StandingGrant) Live() bool {
	return g.State == GrantStateGranted && g.CredentialUsable
}

// NeedsRenewal reports whether the rep already agreed and has nothing to act
// with — their passport was revoked or has expired.
//
// It is the difference between asking somebody to renew and asking them for the
// first time, and getting it wrong reads as the product having forgotten a
// decision they made.
func (g StandingGrant) NeedsRenewal() bool {
	return g.State == GrantStateGranted && !g.CredentialUsable
}

// MyGrant reads the ACTING rep's own answer for one agent, or reports that
// they have not been asked.
//
// It takes no user id, and that is the authorization rather than a convenience:
// the row is keyed on the caller's own identity, so there is no argument a
// handler could pass to read a colleague's decision. Standing authority is a
// personal fact — who agreed to be acted for, and with which credential — and
// the surface that shows it to a rep needs exactly one row, theirs.
//
// A missing row is not an error: "never asked" is the state the confirmation
// flow exists to act on, and turning it into a failure would make the ordinary
// first visit look like a fault.
func (s *Store) MyGrant(ctx context.Context, spec string) (StandingGrant, bool, error) {
	actor, ok := principal.Actor(ctx)
	if !ok || actor.UserID.IsZero() {
		// A principal with no human behind it has no standing grant of its own,
		// and answering "never asked" would invite a caller to offer it the
		// confirmation. A refusal is the honest answer.
		return StandingGrant{}, false, apperrors.ErrPermissionDenied
	}
	return s.grantFor(ctx, ids.From[ids.UserKind](actor.UserID), spec)
}

// grantFor is the read MyGrant and the scheduler share. Unexported, because an
// exported form taking a user id is a cross-rep read waiting for a caller.
func (s *Store) grantFor(ctx context.Context, userID ids.UserID, spec string) (StandingGrant, bool, error) {
	var out StandingGrant
	found := false
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		out, found, err = grantForTx(ctx, tx, userID, spec)
		return err
	})
	return out, found, err
}

// grantForTx is the statement itself, inside a transaction the caller owns.
//
// It exists because answering a grant and minting the credential it names must
// commit together, so the read that reports the result has to join that same
// transaction — a read afterwards could be answered by a state a concurrent
// revoke had already changed.
func grantForTx(ctx context.Context, tx pgx.Tx, userID ids.UserID, spec string) (StandingGrant, bool, error) {
	var out StandingGrant
	// Liveness resolved in the SAME statement the fan-out resolves it in, so
	// the card a rep reads and the run the scheduler queues cannot disagree
	// about whether their authority still works.
	row := tx.QueryRow(ctx, `
		SELECT g.user_id, g.agent_spec, g.state, g.passport_id, g.decided_at,
		       coalesce(p.revoked_at IS NULL AND p.expires_at > now(), false),
		       coalesce(p.scopes, '{}')
		  FROM agent_standing_grant g
		  LEFT JOIN passport p ON p.id = g.passport_id
		 WHERE g.user_id = $1 AND g.agent_spec = $2`, userID, spec)
	switch err := row.Scan(&out.UserID, &out.Spec, &out.State,
		&out.PassportID, &out.DecidedAt, &out.CredentialUsable, &out.PassportScopes); {
	case errors.Is(err, pgx.ErrNoRows):
		return StandingGrant{}, false, nil
	case err != nil:
		return StandingGrant{}, false, fmt.Errorf("read the rep's standing grant: %w", err)
	}
	return out, true, nil
}

// MyGrantTx is MyGrant inside the caller's transaction — the same
// acting-principal rule, so it too cannot be pointed at a colleague.
//
// IT TAKES A LOCK, and the lock is the reason this exists rather than callers
// reusing the pool-opening read. Answering a grant is read-then-write across
// two tables: read the previous answer, revoke the passport it named, mint a
// new one, write the answer. Two concurrent answers from one rep would both
// read "no live credential", both mint, and the upsert would keep one — leaving
// the other live and referenced by nothing, which is standing authority nobody
// can find in order to end it.
//
// A row lock cannot serialize this: on the first answer there IS no row to
// lock. So the lock is on the (rep, agent) PAIR, taken before the read and held
// to commit, which is exactly the span the decision spans.
func (s *Store) MyGrantTx(ctx context.Context, tx pgx.Tx, spec string) (StandingGrant, bool, error) {
	actor, ok := principal.Actor(ctx)
	if !ok || actor.UserID.IsZero() {
		return StandingGrant{}, false, apperrors.ErrPermissionDenied
	}
	userID := ids.From[ids.UserKind](actor.UserID)
	if _, err := tx.Exec(ctx,
		`SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`,
		"agent_standing_grant:"+userID.String()+":"+spec); err != nil {
		return StandingGrant{}, false, fmt.Errorf("hold this rep's answer while it is decided: %w", err)
	}
	return grantForTx(ctx, tx, userID, spec)
}

// LiveGrantsFor reads every rep who granted this agent and still holds the
// credential, for the nightly fan-out.
//
// It filters on the passport being present rather than trusting the state
// alone: a revoked credential leaves a granted row behind on purpose, and a
// fan-out that enqueued for it would queue work no authority can perform.
func (s *Store) LiveGrantsFor(ctx context.Context, spec string) ([]StandingGrant, error) {
	var out []StandingGrant
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT g.user_id, g.agent_spec, g.state, g.passport_id, g.decided_at, true, p.scopes
			  FROM agent_standing_grant g
			  JOIN passport p ON p.id = g.passport_id
			 WHERE g.agent_spec = $1
			   AND g.state = $2
			   AND g.passport_id IS NOT NULL
			   -- The credential must still be usable TONIGHT. A revoked or
			   -- expired passport fails at claim time, and a job enqueued
			   -- against one is a run that can only ever fail — the rep sees
			   -- a broken night instead of the honest "your authority
			   -- lapsed, renew it" the grant state is there to give them.
			   AND p.revoked_at IS NULL
			   AND p.expires_at > now()
			 ORDER BY g.user_id`, spec, GrantStateGranted)
		if err != nil {
			return fmt.Errorf("read the live standing grants: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var g StandingGrant
			if err := rows.Scan(&g.UserID, &g.Spec, &g.State, &g.PassportID,
				&g.DecidedAt, &g.CredentialUsable, &g.PassportScopes); err != nil {
				return fmt.Errorf("scan a standing grant: %w", err)
			}
			out = append(out, g)
		}
		return rows.Err()
	})
	return out, err
}

// RecordDecisionTx writes one rep's answer inside the caller's transaction.
//
// It takes the transaction rather than opening its own because a GRANT is only
// half a fact: the other half is the passport mint, and the two must commit
// together. A mint that succeeded beside a grant that did not leaves a live
// credential nothing points at; a grant beside a failed mint claims an
// authority that does not exist.
//
// Re-answering overwrites. A rep who declined and later changes their mind is
// giving a new answer to the same question, not a second answer — the unique
// constraint says so, and this makes the write agree with it.
//
// WHOSE ANSWER. It takes no user id. The rep is read from the acting principal,
// so the only decision any caller can record is their own — an argument would
// let one person answer for another, and "the admin turned it on for you" is
// precisely what this feature exists not to be. A principal with no human
// behind it is refused: a machine has no standing decision to make.
func RecordDecisionTx(
	ctx context.Context, tx pgx.Tx, spec, state string, passportID *ids.PassportID,
) error {
	actor, ok := principal.Actor(ctx)
	if !ok || actor.UserID.IsZero() {
		return apperrors.ErrPermissionDenied
	}
	userID := ids.From[ids.UserKind](actor.UserID)
	if err := refuseUnanswerable(ctx, tx, userID, state, passportID); err != nil {
		return err
	}
	// The row as it stands, read INSIDE this transaction and before the write.
	//
	// It is what makes the audit row honest: an upsert is a first answer or a
	// changed one, and the ledger has to say which. A changed answer whose
	// before-image is empty records that a rep's authority moved without
	// recording what it moved from, and an audit row cannot be corrected later.
	before, existed, err := grantImageTx(ctx, tx, userID, spec)
	if err != nil {
		return err
	}
	// `xmax = 0` is how the upsert reports which branch it took: the system
	// column is zero on a row this statement INSERTED and non-zero on one it
	// updated. It is read here rather than trusted from the SELECT above,
	// because FOR UPDATE cannot lock a row that does not exist yet — two first
	// answers arriving together both read "absent", and without this the second
	// would audit a create for a row it actually replaced.
	var id ids.UUID
	var inserted bool
	if err := tx.QueryRow(ctx, `
		INSERT INTO agent_standing_grant (user_id, agent_spec, state, passport_id)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (user_id, agent_spec) DO UPDATE
		   SET state = EXCLUDED.state,
		       passport_id = EXCLUDED.passport_id,
		       decided_at = now(),
		       updated_at = now()
		RETURNING id, xmax = 0`, userID, spec, state, passportID).Scan(&id, &inserted); err != nil {
		return fmt.Errorf("record the rep's standing grant: %w", err)
	}
	// The statement's own answer wins over the read: a row that appeared
	// between them is one this write replaced, whatever the SELECT saw.
	if !inserted && !existed {
		before, existed = nil, true
	}
	if inserted {
		existed = false
	}
	// Standing authority is an audited fact for the same reason the passport
	// mint is one: it is the record of who agreed to be acted for, and by what.
	after := grantImage(spec, state, passportID)
	return auditDecision(ctx, tx, id, spec, before, after, existed)
}

// refuseUnanswerable rejects everything that is not one of the two answers a
// rep can give, paired with the credential that answer requires.
func refuseUnanswerable(
	ctx context.Context, tx pgx.Tx, userID ids.UserID, state string, passportID *ids.PassportID,
) error {
	switch state {
	case GrantStateGranted:
		if passportID == nil {
			// The shape the schema forbids, refused here too so the failure
			// names the cause rather than surfacing as a constraint violation.
			return errors.New("agents: a granted standing grant must carry the rep's passport")
		}
		// The passport must be THIS rep's. The composite foreign key refuses
		// any other pairing, so this check is not what makes it safe — it is
		// what makes the refusal readable, and it names the reason: a grant
		// pairing one rep with another's credential would have the fan-out run
		// as the passport's owner on somebody else's say-so.
		return refuseBorrowedCredential(ctx, tx, userID, *passportID)
	case GrantStateDeclined:
		if passportID != nil {
			return errors.New("agents: a declined standing grant carries no passport")
		}
		return nil
	default:
		return fmt.Errorf("agents: %q is not an answer a rep can give", state)
	}
}

// refuseBorrowedCredential rejects a grant naming a passport that acts as
// somebody other than the rep the grant is for.
func refuseBorrowedCredential(
	ctx context.Context, tx pgx.Tx, userID ids.UserID, passportID ids.PassportID,
) error {
	var actsAs ids.UserID
	switch err := tx.QueryRow(ctx,
		`SELECT on_behalf_of FROM passport WHERE id = $1`, passportID).Scan(&actsAs); {
	case errors.Is(err, pgx.ErrNoRows):
		return errors.New("agents: the standing grant names a passport that does not exist")
	case err != nil:
		return fmt.Errorf("read the passport a standing grant names: %w", err)
	}
	if actsAs != userID {
		return errors.New("agents: a standing grant may only name the rep's own passport — " +
			"a run under somebody else's credential acts as them, not as the rep who granted it")
	}
	return nil
}

// auditDecision files the ledger row for one recorded answer, saying whether it
// was a first answer or a changed one.
func auditDecision(
	ctx context.Context, tx pgx.Tx, id ids.UUID, spec string,
	before, after map[string]any, existed bool,
) error {
	if !existed {
		_, err := storekit.AuditEvent(ctx, tx, "create", "agent_standing_grant", id, after)
		return err
	}
	if before == nil {
		// The row was replaced by a concurrent first answer this transaction
		// could not see. What it held is unrecoverable now, and the audit
		// contract refuses an update with no before-image rather than letting
		// one claim a change it cannot describe — so the honest record is the
		// evidence that the prior image was lost, not a fabricated empty one.
		_, err := storekit.AuditWithEvidence(ctx, tx, "update", "agent_standing_grant", id,
			map[string]any{auditFieldAgentSpec: spec}, after,
			map[string]any{"before_image": "raced with a concurrent first answer"})
		return err
	}
	_, err := storekit.Audit(ctx, tx, "update", "agent_standing_grant", id, before, after)
	return err
}

// grantImageTx reads the row's own field image before a write touches it.
func grantImageTx(
	ctx context.Context, tx pgx.Tx, userID ids.UserID, spec string,
) (map[string]any, bool, error) {
	var state string
	var passportID *ids.PassportID
	// FOR UPDATE, so two answers arriving together are ordered rather than
	// racing. Without it both can read "absent", both audit a create, and the
	// ledger records one decision twice — or both read the same stale image and
	// each records the other's answer as what it replaced.
	switch err := tx.QueryRow(ctx, `
		SELECT state, passport_id FROM agent_standing_grant
		 WHERE user_id = $1 AND agent_spec = $2
		 FOR UPDATE`, userID, spec).Scan(&state, &passportID); {
	case errors.Is(err, pgx.ErrNoRows):
		return nil, false, nil
	case err != nil:
		return nil, false, fmt.Errorf("read the standing grant being replaced: %w", err)
	}
	return grantImage(spec, state, passportID), true, nil
}

// grantImage is one snapshot of the row's own fields.
//
// It carries the passport, because "who agreed to be acted for, and by WHAT" is
// the question standing authority raises — an image naming only the state says
// a rep's authority moved without saying which credential it moved to, and an
// audit row cannot be corrected later.
func grantImage(spec, state string, passportID *ids.PassportID) map[string]any {
	image := map[string]any{auditFieldAgentSpec: spec, auditFieldState: state}
	if passportID != nil {
		image[auditFieldPassportID] = passportID.String()
	}
	return image
}
