// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type invitationIntent struct {
	ProposalID ids.UUID
	Public     *publicBookingIntent
}
type publicBookingIntent struct {
	KeyHash   string           `json:"key_hash"`
	Digest    string           `json:"digest"`
	Marketing MarketingOutcome `json:"marketing"`
}

// The browser's random request key is a recovery capability; only its hash is stored.
//
//nolint:nilnil // Old callers without a key request no recovery capability.
func publicBookingRequestIntent(r *http.Request, slug string, body crmcontracts.BookPublicMeetingJSONRequestBody) (*publicBookingIntent, error) {
	key := r.Header.Get("Idempotency-Key")
	if key == "" {
		return nil, nil
	}
	id, err := ids.Parse(key)
	if err != nil || id[6]>>4 != 4 || id[8]>>6 != 2 {
		return nil, &publicBookingKeyError{}
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	sum, digest := sha256.Sum256([]byte(slug+"\x00"+key)), sha256.Sum256(encoded)
	return &publicBookingIntent{KeyHash: hex.EncodeToString(sum[:]), Digest: hex.EncodeToString(digest[:])}, nil
}

//nolint:nilnil // A missing intent is a fresh booking, not a missing public resource.
func (s *Store) recoverPublicBooking(ctx context.Context, host ids.UserID, intent *publicBookingIntent) (*crmcontracts.MeetingInvitation, error) {
	if intent == nil {
		return nil, nil
	}
	var row invitationRow
	err := s.tx(ctx, func(tx pgx.Tx) error {
		args := schedulingArgs{}
		var id ids.UUID
		var created time.Time
		err := tx.QueryRow(ctx, `SELECT activity_id,created_at FROM meeting_invitation WHERE host_user_id=`+args.add(host)+` AND public_intent->>'key_hash'=`+args.add(intent.KeyHash), args...).Scan(&id, &created)
		if err != nil {
			return err
		}
		if !created.Add(24 * time.Hour).After(s.now()) {
			return apperrors.ErrConflict
		}
		row, err = readInvitation(ctx, tx, id, false)
		return err
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if row.PublicIntent == nil || row.PublicIntent.Digest != intent.Digest {
		return nil, apperrors.ErrConflict
	}
	out, err := s.recoverProposalInvitation(ctx, proposalRow{Host: host, Invitation: &row.ID})
	if err != nil {
		return nil, err
	}
	intent.Marketing = row.PublicIntent.Marketing
	return &out, nil
}

func admitPublicBookingIntent(ctx context.Context, tx pgx.Tx, intent *publicBookingIntent) error {
	if intent == nil {
		return nil
	}
	args := schedulingArgs{}
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM meeting_invitation WHERE public_intent->>'key_hash'=`+args.add(intent.KeyHash)+`)`, args...).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return apperrors.ErrConflict
	}
	return nil
}

type publicBookingKeyError struct{}

func (*publicBookingKeyError) Error() string {
	return "Use a fresh random UUID in Idempotency-Key for each booking request"
}

func (e *publicBookingKeyError) MessageFault() (string, string) {
	return "invalid_idempotency_key", e.Error()
}
