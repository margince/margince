// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// The decision taken as a message is written down, in the same transaction that
// writes it.
//
// The transmit decision beside this one answers "may this go out now" and is
// the last word. It cannot be the only word: it runs in a worker, minutes or
// days later, and its refusal reaches a parked row and an operator's lane
// rather than the person who typed the message. A staging decision is what
// makes a refusal answerable — the rep is still there, and the message has not
// yet been promised to anybody.
//
// It commits with the activity, the delivery row, the audit entry, the outbox
// event and the job. All of them or none: a decision recorded for a delivery
// that rolled back would describe a message nobody sent, and a delivery staged
// with no decision is the gap the transmit ticket exists to catch.

import (
	"context"
	"crypto/sha256"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// AuthorizeStagingTx records why this message was allowed to be queued, one row
// per recipient, on the caller's transaction.
//
// It does NOT open its own: the whole point is to commit with the delivery it
// describes. That also means it must not acquire a connection of its own, which
// is why every read below runs on the passed tx.
func (g *Gate) AuthorizeStagingTx(ctx context.Context, tx pgx.Tx, deliveryID ids.UUID, req commsauthz.Request) (commsauthz.DecisionSet, error) {
	for _, r := range req.Recipients {
		if err := r.Validate(); err != nil {
			return commsauthz.DecisionSet{}, fmt.Errorf(
				"consent: this recipient cannot be put to the engine: %w", err)
		}
	}
	if len(req.Recipients) == 0 {
		return commsauthz.DecisionSet{}, fmt.Errorf(
			"consent: a staging decision needs at least one recipient: %w", apperrors.ErrInvalidArgument)
	}

	setID := ids.NewV7()
	set := commsauthz.DecisionSet{}
	for _, r := range req.Recipients {
		d, err := g.decideOne(ctx, tx, r, req.LegacyPurposeKey)
		if err != nil {
			return commsauthz.DecisionSet{}, err
		}
		d.Phase = commsauthz.PhaseStaging
		d.Mode = commsauthz.ModeObserve
		d.Requested = req.Context
		set.Decisions = append(set.Decisions, d)
	}
	if err := g.recordStagingDecisions(ctx, tx, deliveryID, setID, req, set); err != nil {
		return commsauthz.DecisionSet{}, err
	}
	return set, nil
}

// recordStagingDecisions writes the rows.
//
// attempt is 0 and means "before any attempt": the delivery has not been picked
// up yet, and every transmit row that follows carries the attempt it belonged
// to. The fingerprint is of the message as staged, so a later reader can tell
// whether what went out is what was authorized.
func (g *Gate) recordStagingDecisions(ctx context.Context, tx pgx.Tx, deliveryID, setID ids.UUID, req commsauthz.Request, set commsauthz.DecisionSet) error {
	sum := sha256.Sum256([]byte(req.Subject + "\x00" + req.Body))
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return err
	}
	for _, d := range set.Decisions {
		subjectKind := nullableText(d.SubjectKind)
		var subjectID *ids.UUID
		if d.SubjectKind != "" {
			id := d.SubjectID
			subjectID = &id
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO communication_decision
			  (delivery_id, attempt, decision_set_id, recipient_address, subject_kind, subject_id,
			   phase, requested_category, resolved_category, verdict, reason_code, basis, suppression,
			   content_fingerprint, mode, actor)
			VALUES ($1,0,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
			ON CONFLICT (decision_set_id, recipient_address, phase) DO NOTHING`,
			deliveryID, setID, decisionRecipientKey(d.Recipient),
			subjectKind, subjectID, string(d.Phase), nullableCategory(d.Requested),
			string(d.Resolved), string(d.Verdict), d.ReasonCode,
			nullableBasis(d.Basis), nullableText(d.Suppression),
			sum[:], string(d.Mode), by); err != nil {
			return fmt.Errorf("consent: record the staging decision: %w", err)
		}
	}
	return nil
}

// nullableCategory keeps the requested category NULL when the caller claimed
// none, which is the honest record: a door that said nothing is different from
// one that named a category the engine then disagreed with.
func nullableCategory(c commsauthz.Category) *string {
	if c == "" {
		return nil
	}
	v := string(c)
	return &v
}
