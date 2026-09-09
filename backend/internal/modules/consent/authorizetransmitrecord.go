// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// The transmit decision's own persistence, and the suppression facts it reads
// independently of any consent grant.
//
// Split from authorizetransmit.go for the file-length ceiling, the same
// reason authorizedecide.go was split from it before: that file answers
// "may this go out now", and this one answers "what stops it regardless of
// consent" and "how the answer is written down".

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// recordDecisions writes one immutable row per recipient.
//
// The content fingerprint is a hash of subject and body, never the text: it
// exists so a later reader can tell whether the message that went is the
// message that was authorized, and storing the words themselves would make the
// decision a second copy of the mail.
func (g *Gate) recordDecisions(ctx context.Context, tx pgx.Tx, req commsauthz.TransmitRequest, setID ids.UUID, set commsauthz.DecisionSet) ([]commsauthz.Decision, error) {
	var written []commsauthz.Decision
	sum := SendingDigest(req.Subject, req.Body, req.HTMLBody)
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return nil, err
	}
	for _, d := range set.Decisions {
		// Both or neither, which the table's own CHECK also demands: a
		// subject_kind naming a row with no id describes nothing.
		subjectKind := nullableText(d.SubjectKind)
		var subjectID *ids.UUID
		if d.SubjectKind != "" {
			id := d.SubjectID
			subjectID = &id
		}
		tag, err := tx.Exec(ctx, `
			INSERT INTO communication_decision
			  (delivery_id, attempt, decision_set_id, recipient_address, subject_kind, subject_id,
			   phase, resolved_category, verdict, reason_code, basis, suppression,
			   content_fingerprint, legacy_verdict, mode, actor)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)
			ON CONFLICT (decision_set_id, recipient_address, phase) DO NOTHING`,
			req.DeliveryID, req.Attempt, setID, decisionRecipientKey(d.Recipient),
			subjectKind, subjectID, string(d.Phase), string(d.Resolved), string(d.Verdict),
			d.ReasonCode, nullableBasis(d.Basis), nullableText(d.Suppression),
			sum[:], d.LegacyVerdict, string(d.Mode), by)
		if err != nil {
			return nil, fmt.Errorf("consent: record the transmit decision: %w", err)
		}
		if tag.RowsAffected() > 0 {
			written = append(written, d)
		}
	}
	return written, nil
}

// nullableBasis and nullableText carry the difference between "no value" and
// "the empty string" to Postgres. A *string is what pgx reads as NULL, and the
// distinction matters on both columns: a decision with no basis recorded is not
// the same fact as one whose basis is blank.
func nullableBasis(b commsauthz.Basis) *string {
	if b == "" {
		return nil
	}
	v := string(b)
	return &v
}

func nullableText(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// liveSuppression reads what stops a message reaching this recipient
// independently of any consent grant.
//
// Two shapes, and the address arm matters as much as the person arm: a hard
// bounce is a fact about a MAILBOX, so it is recorded against the address and
// keeps applying when the same address later appears on a different record.
// The person arm carries objections and restrictions, which follow the human.
func liveSuppression(ctx context.Context, tx pgx.Tx, personID string, r connector.Recipient) ([]string, error) {
	// EVERY live kind, not the strongest one.
	//
	// An earlier version took one row ordered by a fixed strength, which was
	// sound while every kind refused everything: whichever won, the answer was
	// the same. It stopped being sound when reach became category-dependent —
	// a marketing objection sorts first and binds the LEAST, so a person
	// carrying both an objection and a hard bounce had the bounce masked and
	// their invoice sent to a dead mailbox. Strength is no longer a total
	// order, so the caller is given all of them and applies each.
	//
	// Reading the row is not applying it: what a suppression BINDS depends on
	// the category, which is not known here. applySuppression decides that,
	// after resolution.
	rows, err := tx.Query(ctx, `
		SELECT DISTINCT kind FROM communication_suppression
		 WHERE revoked_at IS NULL
		   AND (person_id = $1
		        OR lead_id = $1
		        OR (address IS NOT NULL AND $2 <> '' AND lower(address) = lower($2)))`,
		personID, r.Email)
	if err != nil {
		return nil, fmt.Errorf("consent: read the recipient's suppressions: %w", err)
	}
	defer rows.Close()
	var kinds []string
	for rows.Next() {
		var kind string
		if err := rows.Scan(&kind); err != nil {
			return nil, fmt.Errorf("consent: read the recipient's suppressions: %w", err)
		}
		kinds = append(kinds, reasonForSuppressionKind(kind))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("consent: read the recipient's suppressions: %w", err)
	}
	return kinds, nil
}

// reasonForSuppressionKind maps a stored kind onto the reason code a decision
// row carries.
//
// A kind this code does not recognise gets its OWN code rather than being
// folded onto a known one: suppressionBinds refuses an unrecognised code
// outright, and folding it onto a recognised one would hand it that code's
// narrower reach. That is exactly how subject_request came to permit five
// categories of mail while wearing the statutory restriction's name.
func reasonForSuppressionKind(kind string) string {
	switch kind {
	case "marketing_objection":
		return commsauthz.ReasonObjection
	case "processing_restriction":
		return commsauthz.ReasonRestricted
	case "subject_request":
		return commsauthz.ReasonSubjectRequest
	case "hard_bounce":
		return commsauthz.ReasonHardBounce
	default:
		return "unrecognised_suppression:" + kind
	}
}

// decisionRecipientKey is the stored identity of one recipient, and it is
// deliberately NOT recipientLabel.
//
// recipientLabel exists to name a refused recipient in an operator's error
// message, where a channel account id is withheld on purpose — the caller never
// supplied it, so a refusal must not hand it back. That is right for a sentence
// and wrong for a key: every channel recipient would store the same words, so
// two recipients on one delivery would collide on the uniqueness index and the
// second decision — possibly the refusal — would be dropped.
//
// A channel identity is therefore stored structurally. It stays inside the
// installation, where the timeline already holds the same id.
func decisionRecipientKey(r connector.Recipient) string {
	if r.Channel != nil {
		return r.Channel.Provider + ":" + r.Channel.ChannelUserID
	}
	return r.Email
}
