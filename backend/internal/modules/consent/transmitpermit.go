// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

type transmitPermit struct {
	lease  *database.SessionLease
	ticket commsauthz.TransmitTicket
}

func (p *transmitPermit) Ticket() commsauthz.TransmitTicket { return p.ticket }
func (p *transmitPermit) Ready(ctx context.Context) error   { return p.lease.Ping(ctx) }
func (p *transmitPermit) Close(ctx context.Context)         { p.lease.Close(ctx) }

// AcquireTransmitPermit returns nil when a conflicting write or send holds the
// keys. The shared session locks survive the recheck's short transaction.
//
//nolint:ireturn // The comms seam owns the permit interface; its lease stays private to consent.
func (g *Gate) AcquireTransmitPermit(ctx context.Context, req commsauthz.TransmitRequest, issued commsauthz.TransmitTicket) (commsauthz.TransmitPermit, error) {
	if err := transmittable(req); err != nil {
		return nil, err
	}
	if !issued.Current(req.DeliveryID, req.Attempt) {
		return nil, fmt.Errorf("consent: no current decision covers this attempt: %w", apperrors.ErrInvalidArgument)
	}
	fresh, err := g.RecheckTransmit(ctx, req, issued)
	if err != nil {
		return nil, err
	}
	keys, err := g.transmitStopKeys(ctx, req)
	if err != nil {
		return nil, err
	}
	lease, held, err := g.store.db.TryWorkLease(ctx, "consent_transmit", req.DeliveryID, slog.Default())
	if errors.Is(err, database.ErrSessionBudgetBusy) {
		return nil, nil //nolint:nilnil // A nil permit means capacity is busy, not a failed send.
	}
	if err != nil || !held {
		return nil, err
	}
	handedOver := false
	defer func() {
		if !handedOver {
			lease.Close(ctx)
		}
	}()
	for _, key := range keys {
		held, err := lease.TryLock(ctx, key, true)
		if err != nil || !held {
			return nil, err
		}
	}
	matches, err := g.transmitSnapshotMatches(ctx, req, fresh, keys)
	if err != nil || !matches {
		return nil, err
	}

	handedOver = true
	return &transmitPermit{lease: lease, ticket: fresh}, nil
}

func (g *Gate) transmitSnapshotMatches(ctx context.Context, req commsauthz.TransmitRequest, fresh commsauthz.TransmitTicket, keys []string) (bool, error) {
	checked, err := g.readTransmitTicket(ctx, req, fresh)
	if err != nil {
		return false, err
	}
	if checked.Fingerprint == "" || checked.Fingerprint != fresh.Fingerprint {
		return false, nil
	}
	current, err := g.transmitStopKeys(ctx, req)
	if err != nil {
		return false, err
	}
	for _, key := range current {
		if !slices.Contains(keys, key) {
			return false, nil
		}
	}
	return true, nil
}

func (g *Gate) transmitStopKeys(ctx context.Context, req commsauthz.TransmitRequest) ([]string, error) {
	var keys []string
	err := g.readTransmitTx(ctx, func(tx pgx.Tx) error {
		claims, err := stagedClaims(ctx, tx, req.DeliveryID)
		if err != nil {
			return err
		}
		var pinned []ids.UUID
		for _, claim := range claims {
			if !claim.evidence.LeadID.IsZero() {
				pinned = append(pinned, claim.evidence.LeadID)
			}
		}
		keys, err = communicationStopKeysTx(ctx, tx, req.Recipients, pinned)
		return err
	})
	return keys, err
}

func communicationStopKeysTx(ctx context.Context, tx pgx.Tx, recipients []connector.Recipient, pinned []ids.UUID) ([]string, error) {
	var keys, emails []string
	var channels []storekit.ChannelIdentityKey
	for _, id := range pinned {
		if !id.IsZero() {
			keys = append(keys, id.String())
		}
	}
	for _, recipient := range recipients {
		if recipient.Channel != nil {
			channels = append(channels, storekit.ChannelIdentityKey{Provider: recipient.Channel.Provider, ChannelUserID: recipient.Channel.ChannelUserID})
		} else {
			keys = append(keys, normalizeAddress(recipient.Email))
			emails = append(emails, recipient.Email)
		}
		subject, found, err := resolveContact(ctx, tx, recipient)
		if err != nil && !errors.Is(err, apperrors.ErrConsentNotGranted) {
			return nil, err
		}
		if err == nil && !found {
			subject, found, err = resolveLead(ctx, tx, recipient)
			if err != nil {
				return nil, err
			}
		}
		if found {
			keys = append(keys, subject)
		}
	}
	keys = append(keys, storekit.SubjectLockKeys(channels, emails)...)
	keys = append(keys, storekit.StopAddressLockKeys(emails, nil)...)
	slices.Sort(keys)
	return slices.Compact(keys), nil
}

// A final send check must not acquire domain-row locks while its session holds
// stop keys. PostgreSQL enforces that property for every query it reaches.
func (g *Gate) readTransmitTx(ctx context.Context, read func(pgx.Tx) error) error {
	return g.store.db.Tx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SET TRANSACTION READ ONLY"); err != nil {
			return fmt.Errorf("consent: opening a read-only send check: %w", err)
		}
		return read(tx)
	})
}
