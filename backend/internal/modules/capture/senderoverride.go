// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// What a human decided about a sender, and the rule that the machine never
// takes it back.
//
// The verdict engine judges every new sender and is sometimes wrong — it calls
// a customer's private address personal, or an old friend business. An owner
// correcting that has to be corrected permanently: a decision the next message
// overwrites is not a decision, it is a suggestion, and the owner would find
// the same sender wrong again next week with no way to tell why.
//
// Per SEAT. A sender is personal to the colleague who knows them, so one rep's
// family member is another rep's customer, and a shared list would let either
// overrule the other about their own correspondence.

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/platform/mailidentity"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/relstrength"
)

// fieldDecision is the request field a sender-decision refusal points at, and
// the key the decision is recorded under.
const fieldDecision = "decision"

// The decisions a contact may record about a sender.
const (
	// OverrideBusiness readmits a sender the machine judged noise: they are a
	// counterparty of this business after all, and their mail belongs in the
	// CRM like anyone else's.
	OverrideBusiness = "business"
	// OverrideKeepOut ends it: no record, and the mail this sender already
	// brought in is destroyed.
	OverrideKeepOut = "keep_out"
	// OverridePersonal is the standing decision a `personal` verdict leaves
	// behind. The address is the seat's private life, so no contact is made
	// for it again, whoever writes, until the seat lifts it. Only the verdict
	// engine writes it (RecordPersonalStandingTx). Unlike keep_out it destroys
	// no mail.
	OverridePersonal = "personal"
)

// SenderOverride is one seat's decision about one address.
type SenderOverride struct {
	ID            ids.UUID
	Address       string
	Decision      string
	OverruledKind string
}

// SenderOverrideStore holds what contacts decided.
type SenderOverrideStore struct {
	db             *database.DB
	withdrawReview ReviewWithdrawer
}

// NewSenderOverrideStore builds the store.
func NewSenderOverrideStore(db *database.DB) *SenderOverrideStore {
	return &SenderOverrideStore{db: db}
}

// Set records the caller's decision about a sender, replacing their previous
// one.
//
// Own decision only: the seat is taken from the authenticated principal and
// never from the request, so there is no shape of this call that records one
// contact's answer under another's name.
func (s *SenderOverrideStore) Set(ctx context.Context, address, decision string) (SenderOverride, error) {
	if err := auth.RequireHumanOrItsAgent(ctx); err != nil {
		return SenderOverride{}, err
	}
	actor, ok := principal.Actor(ctx)
	if !ok || actor.UserID == ids.Nil {
		return SenderOverride{}, apperrors.ErrPermissionDenied
	}
	// A read seat is licensed to look. RequireHumanOrItsAgent reads no grant at
	// all, and a `business` decision is how a contact gets created.
	if !actor.SeatType.CanMutate() {
		return SenderOverride{}, apperrors.ErrPermissionDenied
	}
	if decision != OverrideBusiness && decision != OverrideKeepOut {
		return SenderOverride{}, &InvalidOverrideError{Field: fieldDecision, Reason: "decision is business or keep_out"}
	}
	// keep_out destroys mail for good, which only the user's own session does.
	if decision == OverrideKeepOut && actor.Type == principal.PrincipalAgent {
		return SenderOverride{}, fmt.Errorf("keep_out destroys this sender's mail, so the user decides it in the app: %w", apperrors.ErrPermissionDenied)
	}
	// The grant follows the DECISION, because the two are different acts.
	// `business` is how a contact comes to exist, so it takes create;
	// `keep_out` is how one is refused and their mail destroyed, so it takes
	// delete. Asking for create on a keep_out would refuse the seat who is
	// allowed to remove contacts but not add them, which is backwards.
	grant := principal.ActionCreate
	if decision == OverrideKeepOut {
		grant = principal.ActionDelete
	}
	if err := auth.Require(ctx, "contact", grant); err != nil {
		return SenderOverride{}, err
	}

	folded, ok := parseSingleAddress(address)
	if !ok {
		return SenderOverride{}, &InvalidOverrideError{Field: fieldAddress, Reason: fmt.Sprintf("give one email address of at most %d characters", maxIndexedAddressChars)}
	}
	var out SenderOverride
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		// The same key every reader of this decision takes, so a seat recording
		// one cannot land between a reader's look and the act it drives.
		if err := lockSenderDecisionTx(ctx, tx, actor.UserID, folded); err != nil {
			return err
		}
		if err := refuseAgentOverPrivateTx(ctx, tx, actor, folded); err != nil {
			return err
		}
		if err := refuseAgentOverAUsersDecisionTx(ctx, tx, actor, folded); err != nil {
			return err
		}
		capturedBy, err := storekit.CapturedBy(ctx)
		if err != nil {
			return err
		}
		// The kind the machine had reached, read BEFORE the write so the page
		// can say what was overruled. A contact who sees only their own answer
		// cannot tell a correction from a preference they set months ago.
		kind, err := machineKindForTx(ctx, tx, actor.UserID, folded)
		if err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `
			INSERT INTO capture_sender_override (user_id, address, decision, overruled_kind, captured_by)
			VALUES ($1, $2, $3, NULLIF($4, ''), $5)
			ON CONFLICT (user_id, address) DO UPDATE
			   SET decision = EXCLUDED.decision,
			       overruled_kind = COALESCE(EXCLUDED.overruled_kind, capture_sender_override.overruled_kind),
			       captured_by = EXCLUDED.captured_by
			RETURNING id, address, decision, coalesce(overruled_kind, '')`,
			actor.UserID, folded, decision, kind, capturedBy).
			Scan(&out.ID, &out.Address, &out.Decision, &out.OverruledKind); err != nil {
			return fmt.Errorf("capture: recording a sender decision: %w", err)
		}
		// Audit-only, like the exclusion list beside it: this is capture
		// configuration, and the closed event catalog carries no type for it.
		// The object spelled as a literal, not through the constant: the audit
		// gate reads call sites to tell an entity named at runtime from one
		// named in the source, and a constant is something it cannot resolve.
		if _, err := storekit.AuditEvent(ctx, tx, "update", "capture_settings", storekit.MustWorkspace(ctx),
			map[string]any{fieldDecision: decision, "overruled_kind": kind}); err != nil {
			return err
		}
		return s.handOverToVerdictTx(ctx, tx, actor.UserID, folded)
	})
	return out, err
}

// Remove withdraws a decision, handing the sender back to the machine.
func (s *SenderOverrideStore) Remove(ctx context.Context, address string) error {
	if err := auth.RequireHumanOrItsAgent(ctx); err != nil {
		return err
	}
	actor, ok := principal.Actor(ctx)
	if !ok || actor.UserID == ids.Nil {
		return apperrors.ErrPermissionDenied
	}
	if !actor.SeatType.CanMutate() {
		return apperrors.ErrPermissionDenied
	}
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		folded := normalizeEmail(address)
		if err := lockSenderDecisionTx(ctx, tx, actor.UserID, folded); err != nil {
			return err
		}
		if err := refuseAgentOverPrivateTx(ctx, tx, actor, folded); err != nil {
			return err
		}
		if err := refuseAgentOverAUsersDecisionTx(ctx, tx, actor, folded); err != nil {
			return err
		}
		lifted, err := liftPrivateStandingTx(ctx, tx, actor.UserID, folded)
		if err != nil {
			return err
		}
		restored := false
		if !lifted {
			if restored, err = restorePrivateStandingTx(ctx, tx, actor.UserID, folded); err != nil {
				return err
			}
		}
		if !lifted && !restored {
			tag, err := tx.Exec(ctx, `
				DELETE FROM capture_sender_override WHERE user_id = $1 AND address = $2`,
				actor.UserID, folded)
			if err != nil {
				return fmt.Errorf("capture: withdrawing a sender decision: %w", err)
			}
			if tag.RowsAffected() == 0 {
				// Not there and not yours are one answer, so an address probe
				// learns nothing about whose decisions exist.
				return apperrors.ErrNotFound
			}
		}
		// Spelled as a literal for the audit gate; see Set.
		_, err = storekit.AuditEvent(ctx, tx, "update", "capture_settings", storekit.MustWorkspace(ctx),
			map[string]any{fieldDecision: "withdrawn"})
		return err
	})
}

// refuseAgentOverPrivateTx keeps a private sender's standing from an agent,
// whatever stamp the row carries. The classifier writes the row as an agent.
// Changing it can put a private sender back in the CRM. Only the user's own
// session does. The caller holds the sender's write lock.
func refuseAgentOverPrivateTx(ctx context.Context, tx pgx.Tx, actor principal.Principal, folded string) error {
	if actor.Type != principal.PrincipalAgent {
		return nil
	}
	standing, err := PersonalStandingTx(ctx, tx, actor.UserID, folded)
	if err != nil {
		return err
	}
	if standing {
		return fmt.Errorf("a private sender's decision is the user's, so the user changes it in the app: %w", apperrors.ErrPermissionDenied)
	}
	return nil
}

// liftPrivateStandingTx turns a private standing into the seat's own `business`
// answer instead of deleting it. A deleted row would leave a settled `personal`
// verdict and no decision, and readers cannot tell that from a verdict nobody
// has lifted. The seat's answer is the record that they chose it.
func liftPrivateStandingTx(ctx context.Context, tx pgx.Tx, user ids.UUID, folded string) (bool, error) {
	capturedBy, err := storekit.CapturedBy(ctx)
	if err != nil {
		return false, err
	}
	tag, err := tx.Exec(ctx, `
		UPDATE capture_sender_override
		   SET decision = $3, overruled_kind = $4, captured_by = $5
		 WHERE user_id = $1 AND address = $2 AND decision = $6`,
		user, folded, OverrideBusiness, KindPersonal, capturedBy, OverridePersonal)
	if err != nil {
		return false, fmt.Errorf("capture: lifting a private sender's decision: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

// restorePrivateStandingTx answers a withdrawn correction of a private sender
// with the private standing again, when the settled verdict still stands for
// it. Without this, withdrawing a `business` answer that overruled a standing
// would leave the sender with no decision at all, and correspondence would
// make the contact. It reports whether it restored one.
func restorePrivateStandingTx(ctx context.Context, tx pgx.Tx, user ids.UUID, folded string) (bool, error) {
	capturedBy, err := storekit.CapturedBy(ctx)
	if err != nil {
		return false, err
	}
	tag, err := tx.Exec(ctx, `
		UPDATE capture_sender_override o
		   SET decision = $3, overruled_kind = NULL, captured_by = $4
		 WHERE o.user_id = $1 AND o.address = $2 AND o.decision <> $3
		   AND EXISTS (SELECT 1 `+settledPrivateVerdictSQL+` AND p.owner_id = o.user_id AND p.email = o.address)`,
		user, folded, OverridePersonal, capturedBy)
	if err != nil {
		return false, fmt.Errorf("capture: restoring a private sender's decision: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

// standingAddress is the spelling a standing decision is stored and read under.
// It is the one the contact identity layer uses, so two spellings of one
// address are one sender.
func standingAddress(address string) string { return mailidentity.FoldAddress(address) }

// refuseAgentOverAUsersDecisionTx keeps a decision the user made about a sender
// from their agent. The caller holds the sender's write lock.
func refuseAgentOverAUsersDecisionTx(ctx context.Context, tx pgx.Tx, actor principal.Principal, folded string) error {
	if actor.Type != principal.PrincipalAgent {
		return nil
	}
	var decidedBy *string
	err := tx.QueryRow(ctx, `SELECT captured_by FROM capture_sender_override
		 WHERE user_id = $1 AND address = $2`, actor.UserID, folded).Scan(&decidedBy)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("capture: reading who decided about a sender: %w", err)
	}
	return refuseAgentOverAUsersDecision(actor, decidedBy, "this sender")
}

// refuseAgentOverAUsersDecision is the one answer to whether an agent may change a
// capture decision, from the stamp of whoever made it. A row with no stamp
// predates agents, so a user made it.
func refuseAgentOverAUsersDecision(actor principal.Principal, decidedBy *string, what string) error {
	if actor.Type != principal.PrincipalAgent || (decidedBy != nil && strings.HasPrefix(*decidedBy, agentStampPrefix)) {
		return nil
	}
	return &httperr.DetailedError{
		Status: http.StatusConflict, Code: "user_decided",
		Detail: "the user already decided about " + what + ", so the user changes it",
	}
}

// agentDecisionTakeoverSQL keeps a rule's stamp on a repeat add, unless an agent
// made it: a user adding it again makes the decision theirs. %[1]s is the table.
const agentDecisionTakeoverSQL = `CASE WHEN %[1]s.created_by LIKE 'agent:%%' THEN EXCLUDED.created_by ELSE %[1]s.created_by END`

const agentStampPrefix = "agent:"

// OverrideForTx answers what this seat decided about a sender, or "" when they
// have not.
//
// THE VERDICT ENGINE CONSULTS THIS FIRST and never writes over it. A machine
// that could overturn a contact would make every correction temporary, and the
// owner would have no way to tell a fresh mistake from one they already fixed.
// senderOverrideEntity and senderOverrideIdentity spell the serialization key
// ONCE. Two writers of one lock key that disagree by a character take two
// different locks and serialize nothing, while looking exactly like code that
// does — which is the failure this key exists to prevent, in its own mechanism.
const senderOverrideEntity = "capture_sender_override"

func senderOverrideIdentity(user ids.UUID, foldedAddress string) string {
	return user.String() + ":" + foldedAddress
}

func OverrideForTx(ctx context.Context, tx pgx.Tx, user ids.UUID, address string) (string, error) {
	folded := normalizeEmail(address)
	if user == ids.Nil || folded == "" {
		return "", nil
	}
	// Locked, not merely read. A caller that acts on this answer — the verdict
	// engine does, and its effects reach the whole workspace — needs the
	// decision to hold until it commits. Under READ COMMITTED a Set() in its
	// own transaction commits between this read and that write, and the answer
	// acted on was already stale. Reentrant, so Set may take it and then write
	// through here without deadlocking.
	if err := lockSenderDecisionTx(ctx, tx, user, folded); err != nil {
		return "", err
	}
	var decision string
	err := tx.QueryRow(ctx, `
		SELECT decision FROM capture_sender_override
		 WHERE user_id = $1 AND address = $2`, user, folded).Scan(&decision)
	if err != nil {
		if err == pgx.ErrNoRows {
			return "", nil
		}
		return "", fmt.Errorf("capture: reading a seat's decision about a sender: %w", err)
	}
	return decision, nil
}

// verdictMayStandSQL is the one rule for when a machine verdict about an address
// may leave a lasting decision. The From of an inbound mail is forged by
// anyone, so an inbound trigger must be authenticated. An outbound trigger
// cannot be forged, since the seat wrote to the address. alias names the
// triggering activity and email the ledger row's address.
func verdictMayStandSQL(alias, email string) string {
	return `(` + alias + `.counterparty_email = ` + email + ` AND (` + alias + `.inbound_sender_authenticated
	          OR (` + relstrength.WeWroteSQL(alias) + ` AND ` + alias + `.counterparty_outbound_attested)))`
}

const personalStandingSQL = `SELECT EXISTS (SELECT 1 FROM capture_sender_override
	WHERE user_id = $1 AND address = $2 AND decision = $3)`

// PersonalStandingTx answers whether this seat holds a standing private
// decision for the address. One plain read with no lock. The capture ladder
// reads the same fact inside priorDispositionTx, and the two share
// OverridePersonal.
func PersonalStandingTx(ctx context.Context, tx pgx.Tx, user ids.UUID, address string) (bool, error) {
	folded := standingAddress(address)
	if user == ids.Nil || folded == "" {
		return false, nil
	}
	var standing bool
	if err := tx.QueryRow(ctx, personalStandingSQL, user, folded, OverridePersonal).Scan(&standing); err != nil {
		return false, fmt.Errorf("capture: reading a seat's private sender: %w", err)
	}
	return standing, nil
}

// PersonalStandingLockedTx is PersonalStandingTx under the sender-decision lock,
// which the caller then holds until its transaction ends. The lock and the read
// go in one exchange, and the read takes its own snapshot after the lock.
func PersonalStandingLockedTx(ctx context.Context, tx pgx.Tx, user ids.UUID, address string) (bool, error) {
	folded := standingAddress(address)
	if user == ids.Nil || folded == "" {
		return false, nil
	}
	batch := &pgx.Batch{}
	storekit.QueueWriteIdentityLock(batch, senderOverrideEntity, senderOverrideIdentity(user, folded))
	batch.Queue(personalStandingSQL, user, folded, OverridePersonal)
	results := tx.SendBatch(ctx, batch)
	var standing bool
	_, lockErr := results.Exec()
	if lockErr == nil {
		lockErr = results.QueryRow().Scan(&standing)
	}
	if closeErr := results.Close(); lockErr == nil {
		lockErr = closeErr
	}
	if lockErr != nil {
		return false, fmt.Errorf("capture: reading a seat's private sender under its lock: %w", lockErr)
	}
	return standing, nil
}

// RecordPersonalStandingTx makes a `personal` verdict a standing decision for
// the seat's address, in the verdict's own transaction. A decision the user
// typed wins: DO NOTHING leaves their business or keep_out in place.
func RecordPersonalStandingTx(ctx context.Context, tx pgx.Tx, owner ids.UUID, address string) error {
	folded := standingAddress(address)
	if owner == ids.Nil || folded == "" {
		return nil
	}
	if err := lockSenderDecisionTx(ctx, tx, owner, folded); err != nil {
		return err
	}
	capturedBy, err := storekit.CapturedBy(ctx)
	if err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `
		INSERT INTO capture_sender_override (user_id, address, decision, captured_by)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (user_id, address) DO NOTHING`,
		owner, folded, OverridePersonal, capturedBy)
	if err != nil {
		return fmt.Errorf("capture: recording a private sender: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return nil
	}
	// Spelled as a literal for the audit gate; see Set.
	_, err = storekit.AuditEvent(ctx, tx, "update", "capture_settings", storekit.MustWorkspace(ctx),
		map[string]any{fieldDecision: OverridePersonal})
	return err
}

// machineKindForTx reads the kind the verdict engine last reached for this
// sender, so a recorded override can say what it overruled.
func machineKindForTx(ctx context.Context, tx pgx.Tx, user ids.UUID, address string) (string, error) {
	var kind string
	err := tx.QueryRow(ctx, `
		SELECT coalesce(kind, '') FROM capture_pending_counterparty
		 WHERE email = $1 AND owner_id = $2 AND kind IS NOT NULL
		 ORDER BY updated_at DESC LIMIT 1`, address, user).Scan(&kind)
	if err != nil {
		if err == pgx.ErrNoRows {
			// The engine never reached this sender. A human may still decide
			// about them — an address they know is coming, say — and the page
			// then shows a decision that overruled nothing.
			return "", nil
		}
		return "", fmt.Errorf("capture: reading what the machine decided about a sender: %w", err)
	}
	return kind, nil
}

// InvalidOverrideError is a malformed decision; it answers 422 naming the field.
type InvalidOverrideError struct{ Field, Reason string }

func (e *InvalidOverrideError) Error() string { return "capture sender decision: " + e.Reason }

// FieldFault maps the refusal onto the wire.
func (e *InvalidOverrideError) FieldFault() (field, code, message string) {
	return e.Field, "invalid_sender_decision", e.Reason
}

func lockSenderDecisionTx(ctx context.Context, tx pgx.Tx, user ids.UUID, address string) error {
	return storekit.LockWriteIdentity(ctx, tx, senderOverrideEntity,
		senderOverrideIdentity(user, normalizeEmail(address)))
}
