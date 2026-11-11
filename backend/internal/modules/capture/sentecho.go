// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// The capture half of recovering a sent message's provider-stamped identity
// after its read-back failed. comms owns the deliveries, so both seams cross
// through compose. The echo's capture asks comms to re-key the send. The
// send's receipt asks capture whether the echo is already stored.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// SentEchoAdoption re-keys the seat's sent message onto the identity its echo
// carries, in the capture transaction, and answers whether it did.
// comms.Store.AdoptEchoIdentityTx is the implementation.
// accountSince is when the connection was bound to the account it holds now,
// nil when it never changed account.
type SentEchoAdoption func(ctx context.Context, tx pgx.Tx, seat ids.UUID, provider, providerMessageID, stamped string, accountSince *time.Time) (bool, error)

// WithSentEchoAdoption returns a copy that folds a seat's own sent echo onto
// the send it copies. Without one, an echo of a send whose identity is unknown
// lands as a second row.
func (s *Sink) WithSentEchoAdoption(adopt SentEchoAdoption) *Sink {
	out := *s
	out.adoptSentEcho = adopt
	return &out
}

// adoptSentEchoTx runs before the capture's merge lock, so the natural key the
// echo is then written under already belongs to the send. A fault in the
// re-key is a duplicate row, not a lost message: it is logged and the capture
// goes on. A lock cycle is the exception, because retrying the whole capture
// is what resolves it.
func (s *Sink) adoptSentEchoTx(ctx context.Context, tx pgx.Tx, rec connector.NormalizedRecord) error {
	fields, ok := rec.Fields.(ActivityFields)
	seat, provider := actorUserID(ctx), deliveringConnector(ctx)
	if s.adoptSentEcho == nil || !ok || rec.NaturalKey.SourceSystem != connector.EmailSourceSystem ||
		rec.ProviderMessageID == "" || seat.IsZero() || provider == "" {
		return nil
	}
	// Mail sent from another of the seat's addresses arrives inbound until this
	// re-reading. It runs before the alias sighting the landing makes, so a
	// message that itself first proves an alias is not adopted: one extra row.
	_, fields, err := s.asSentFromOwnAddressTx(ctx, tx, rec, fields)
	if err != nil || fields.Direction != connector.DirectionOutbound {
		return err
	}
	since, live, err := connectionAccountSinceTx(ctx, tx, seat, provider)
	if err != nil || !live {
		return err
	}
	if err := storekit.LockSentEchoKey(ctx, tx, seat, provider, rec.ProviderMessageID); err != nil {
		return err
	}
	if providerFiledTransport(rec.CapturedBy) && fields.BodyCompleteness == BodyComplete && !fields.CredentialsMasked {
		ctx = connector.WithSentEcho(ctx, connector.SentEcho{Identity: rec.NaturalKey.SourceID, Subject: fields.Subject, Body: fields.Body})
	}
	return s.adoptInSavepointTx(ctx, tx, seat, provider, rec, since)
}

// adoptInSavepointTx re-keys inside a savepoint, so a failed re-key rolls back
// alone and the capture keeps its own writes.
func (s *Sink) adoptInSavepointTx(ctx context.Context, tx pgx.Tx, seat ids.UUID, provider string, rec connector.NormalizedRecord, since *time.Time) error {
	sp, err := tx.Begin(ctx)
	if err != nil {
		return fmt.Errorf("capture: opening the sent-echo adoption savepoint: %w", err)
	}
	if _, err := s.adoptSentEcho(ctx, sp, seat, provider, rec.ProviderMessageID, rec.NaturalKey.SourceID, since); err != nil {
		if storekit.IsLockCycle(err) {
			return err
		}
		if rbErr := sp.Rollback(ctx); rbErr != nil {
			return errors.Join(err, rbErr)
		}
		slog.WarnContext(ctx, "capture: the sent echo could not adopt its send's identity",
			"err", err, "provider_message_id", rec.ProviderMessageID)
		return nil
	}
	return sp.Commit(ctx)
}

// connectionAccountSinceTx answers when the seat's live connection was bound
// to the account it holds now (nil: never rebound), and whether one is live.
func connectionAccountSinceTx(ctx context.Context, tx pgx.Tx, seat ids.UUID, provider string) (*time.Time, bool, error) {
	var since *time.Time
	err := tx.QueryRow(ctx, `
		SELECT account_bound_at FROM capture_connection
		 WHERE user_id = $1 AND provider = $2 AND archived_at IS NULL AND status IN ('connected','error')`,
		seat, provider).Scan(&since)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("capture: reading the account the connection holds: %w", err)
	}
	return since, true, nil
}

// EchoIdentityTx answers the identity of the seat's captured echo of one sent
// message, found by the provider's message id under the connection that
// delivered it. comms reads it through compose when a read-back failed. The
// connection must have held its current account when the message was sent.
func EchoIdentityTx(ctx context.Context, tx pgx.Tx, seat ids.UUID, provider, providerMessageID string, sentAt time.Time) (string, bool, error) {
	var stamped string
	err := tx.QueryRow(ctx, `
		SELECT a.source_id FROM capture_import ci JOIN activity a ON a.id = ci.activity_id
		 WHERE ci.user_id = $1 AND ci.provider_message_ids && `+storekit.ProviderMessageKeysSQL("$1", "$2", "$3")+`
		   AND a.source_system = $4 AND a.direction = 'outbound' AND a.archived_at IS NULL
		   AND EXISTS (SELECT 1 FROM capture_connection c WHERE c.user_id = $1 AND c.provider = $2
		               AND c.archived_at IS NULL AND (c.account_bound_at IS NULL OR c.account_bound_at <= $5))
		 ORDER BY a.created_at, a.id LIMIT 1`,
		seat, provider, providerMessageID, connector.EmailSourceSystem, sentAt).Scan(&stamped)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("capture: reading the sent message's echo: %w", err)
	}
	return stamped, true, nil
}
