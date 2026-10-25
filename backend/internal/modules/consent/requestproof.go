// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// The proof a closed privacy request keeps: one privacy_request_proof row per
// request, written in the transaction that closes it. It holds what an answer
// must show and no mail or other contact data, so it can outlive the contact.

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

const (
	// proofEntity is the audit subject of a proof row.
	proofEntity        = "privacy_request_proof"
	fieldIdentityCheck = "identity_check"
	fieldRequester     = "requester_email"

	// identityConfirmLink is the check a confirm-link case carries without
	// being told: the subject used a link mailed to their own address.
	identityConfirmLink = "confirm_link"
	// identityUnknown is only for a proof backfilled from a request closed
	// before proofs existed, when nothing recorded how the identity was checked.
	identityUnknown = "unknown"

	// proofRetentionYears is how long a proof is kept after the end of the
	// calendar year its request was answered in.
	proofRetentionYears = 3
)

// dateLineEast is the zone where a new year starts first. An answer's year is
// read there, so a proof is kept a year longer at most, never shorter.
var dateLineEast = time.FixedZone("UTC+14", 14*60*60)

// ProofKeepUntil is when a proof answered at answered may be deleted: the
// start of the fourth year after the year it was answered in. Exported for
// the tests that set a proof's age, so they apply this rule rather than a copy.
func ProofKeepUntil(answered time.Time) time.Time {
	year := answered.In(dateLineEast).Year()
	return time.Date(year+proofRetentionYears+1, time.January, 1, 0, 0, 0, 0, time.UTC)
}

// ProofStatement is what the officer closing a request says for its proof.
// Both fields are nil when the patch does not state them.
type ProofStatement struct {
	IdentityCheck  *string
	RequesterEmail *string
}

func proofStatementOf(req crmcontracts.UpdateDataSubjectRequest) ProofStatement {
	var out ProofStatement
	if req.IdentityCheck != nil {
		check := string(*req.IdentityCheck)
		out.IdentityCheck = &check
	}
	if req.RequesterEmail != nil {
		address := string(*req.RequesterEmail)
		out.RequesterEmail = &address
	}
	return out
}

// closesAs reports whether the patch moves an open request to status.
func closesAs(current dsrRow, in UpdateDSRInput, status string) bool {
	return in.Status != nil && *in.Status == status && !closedDSRStatus(current.Status)
}

// closes reports whether the patch moves an open request to either closed status.
func closes(current dsrRow, in UpdateDSRInput) bool {
	return in.Status != nil && closedDSRStatus(*in.Status) && !closedDSRStatus(current.Status)
}

// linkMailedToSQL is where the confirm link behind request d was mailed. It is
// NULL for a request filed by hand, or one whose link is gone.
const linkMailedToSQL = `(SELECT t.delivered_to FROM contact_confirm_submission s
		JOIN confirm_token t ON t.id = s.token_id WHERE s.id = d.source_submission_id)`

// proofSource is what the proof copies from the request before it closes.
// The zero value is a patch that closes nothing, and writes no proof.
type proofSource struct {
	closes        bool
	requestNumber string
	// requesterEmail is empty when no address is known; the column is NULL.
	requesterEmail string
	receivedAt     time.Time
	identityCheck  string
}

// readProofSource reads what the proof needs from the request, inside the
// closing transaction.
//
// It runs before an erasure, because the erasure deletes the confirm link the
// requester's address is read from. It refuses a close that states no identity
// check, so a request that cannot close is never erased.
func readProofSource(ctx context.Context, tx pgx.Tx, current dsrRow, in UpdateDSRInput) (proofSource, error) {
	if !closes(current, in) {
		return proofSource{}, nil
	}
	var (
		source   = proofSource{closes: true}
		channel  *string
		mailedTo *string
	)
	err := tx.QueryRow(ctx, `
		SELECT coalesce(d.receipt_reference, d.id::text), coalesce(d.received_at, d.created_at), d.channel,
		       `+linkMailedToSQL+`
		  FROM data_subject_request d WHERE d.id = $1`, current.ID,
	).Scan(&source.requestNumber, &source.receivedAt, &channel, &mailedTo)
	if err != nil {
		return proofSource{}, err
	}
	switch {
	case in.Proof.IdentityCheck != nil:
		if !crmcontracts.UpdateDataSubjectRequestIdentityCheck(*in.Proof.IdentityCheck).Valid() {
			return proofSource{}, &ValidationError{Field: fieldIdentityCheck, Reason: "not an identity check"}
		}
		source.identityCheck = *in.Proof.IdentityCheck
	case channel != nil && *channel == channelConfirmLink:
		source.identityCheck = identityConfirmLink
	default:
		return proofSource{}, &ValidationError{Field: fieldIdentityCheck, Reason: "closing a request needs how the requester's identity was checked"}
	}
	source.requesterEmail, err = requesterAddress(in.Proof.RequesterEmail, mailedTo, current.SubjectRef)
	if err != nil {
		return proofSource{}, err
	}
	return source, nil
}

// requesterAddress picks the officer's stated address, else the link's, else a
// subject reference that is an address. Empty when none is one.
func requesterAddress(stated, mailedTo *string, subjectRef string) (string, error) {
	if stated != nil {
		parsed, err := values.ParseEmail(*stated)
		if err != nil {
			return "", &ValidationError{Field: fieldRequester, Reason: "not a plain email address"}
		}
		return parsed.String(), nil
	}
	for _, candidate := range []*string{mailedTo, &subjectRef} {
		if candidate == nil {
			continue
		}
		if parsed, err := values.ParseEmail(*candidate); err == nil {
			return parsed.String(), nil
		}
	}
	return "", nil
}

// requestProof is one proof row as it is written.
type requestProof struct {
	requestID    ids.UUID
	source       proofSource
	kind         string
	result       string
	erasedCounts map[string]int64
	countsKnown  bool
	// completedByType and completedByID name the closer as audit_log names an
	// actor; both nil when no record names one.
	completedByType, completedByID *string
	answeredAt                     time.Time
	backfillSource                 string
}

// writeRequestProof writes the proof of a request this transaction closed.
//
// erased is what the erasure removed. nil means it erased nothing, which the
// proof records as an empty map, not an unknown one.
func writeRequestProof(ctx context.Context, tx pgx.Tx, source proofSource, closed dsrRow, erased map[string]int64) error {
	if !source.closes {
		return nil
	}
	actor, err := storekit.Actor(ctx)
	if err != nil {
		return err
	}
	if erased == nil {
		erased = map[string]int64{}
	}
	// updated_at is the closing write's own transaction time, stamped by the
	// row's trigger.
	var answeredAt time.Time
	if err := tx.QueryRow(ctx, `SELECT updated_at FROM data_subject_request WHERE id = $1`, closed.ID).Scan(&answeredAt); err != nil {
		return err
	}
	_, err = insertRequestProof(ctx, tx, requestProof{
		requestID: closed.ID, source: source, kind: closed.Kind, result: closed.Status,
		erasedCounts: erased, countsKnown: true, answeredAt: answeredAt,
		completedByType: new(string(actor.Type)), completedByID: new(actor.ID),
	})
	return err
}

// insertRequestProof writes one proof and its audit row, and reports whether it
// wrote one. A request that already has a proof keeps it.
//
// The audit image carries no address, so deleting the proof deletes the
// address it recorded.
func insertRequestProof(ctx context.Context, tx pgx.Tx, p requestProof) (bool, error) {
	keepUntil := ProofKeepUntil(p.answeredAt)
	var proofID ids.UUID
	err := tx.QueryRow(ctx, `
		INSERT INTO privacy_request_proof
		  (request_id, request_number, requester_email, received_at, request_kind, identity_check, result,
		   erased_counts, erased_counts_known, completed_by_type, completed_by_id, answered_at, keep_until)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		ON CONFLICT (request_id) DO NOTHING
		RETURNING id`,
		p.requestID, p.source.requestNumber, pgtype.Text{String: p.source.requesterEmail, Valid: p.source.requesterEmail != ""},
		p.source.receivedAt, p.kind,
		p.source.identityCheck, p.result, p.erasedCounts, p.countsKnown, p.completedByType, p.completedByID,
		p.answeredAt, keepUntil,
	).Scan(&proofID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	image := map[string]any{
		"request_id": p.requestID, fieldKind: p.kind, fieldIdentityCheck: p.source.identityCheck,
		"result": p.result, "erased_counts": p.erasedCounts, "erased_counts_known": p.countsKnown,
		"keep_until": keepUntil,
	}
	if p.backfillSource != "" {
		image["backfilled_from"] = p.backfillSource
	}
	_, err = storekit.Audit(ctx, tx, "create", proofEntity, proofID, nil, image)
	return err == nil, err
}
