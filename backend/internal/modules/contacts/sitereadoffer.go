// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// The offer slot of a dossier conversation: the one (field, value) Margince's
// latest reply offered to apply, kept by the server so a bare yes can accept it.
//
// The conversation itself is not stored — the client replays it — so the slot
// is the only part of it the server vouches for. It is keyed to the human it
// was offered to, and every answered message replaces it, which is what makes
// an offer good for exactly the turn that follows it.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// SiteReadOffer is one offer as the server recorded it. TurnDigest names the
// assistant message that made it and DraftVersion the dossier it was grounded
// in, so a replayed conversation that differs in either is not the one the
// offer was made in.
type SiteReadOffer struct {
	Field        string   `json:"field"`
	Value        string   `json:"value"`
	SourceIDs    []string `json:"source_ids"`
	TurnDigest   string   `json:"turn_digest"`
	DraftVersion int      `json:"draft_version"`
	OfferedTo    string   `json:"offered_to"`
}

// requireSiteReadOfferAuthority admits whoever the onboarding read itself
// admits as a writer: an offer to anybody else would grant a change they
// cannot make.
func requireSiteReadOfferAuthority(ctx context.Context) error {
	return requireSiteReadGrant(ctx, TargetKindOnboarding)
}

// MayOfferSiteReadChange reports whether the caller may be offered a change to
// one company field: they hold the offer authority and no mask withholds the
// field from them. A yes can grant only what saving could write.
func MayOfferSiteReadChange(ctx context.Context, field string) bool {
	if requireSiteReadOfferAuthority(ctx) != nil {
		return false
	}
	p, ok := principal.Actor(ctx)
	return ok && !slices.Contains(auth.MaskedFields(p, "company", true), field)
}

// StandingSiteReadOffer answers the offer the caller's own conversation holds
// on an onboarding read, or nil. Another human's offer is nil too: a yes grants
// only what Margince offered to whoever is saying it.
func (s *Store) StandingSiteReadOffer(ctx context.Context, readID ids.UUID) (*SiteReadOffer, error) {
	if err := requireSiteReadOfferAuthority(ctx); err != nil {
		return nil, err
	}
	caller, err := storekit.CapturedBy(ctx)
	if err != nil {
		return nil, err
	}
	var offer *SiteReadOffer
	err = s.tx(ctx, func(tx pgx.Tx) error {
		var raw []byte
		err := tx.QueryRow(ctx, `SELECT conversation_offer FROM site_read
			WHERE id = $1 AND target_kind = 'onboarding'`, readID).Scan(&raw)
		if errors.Is(err, pgx.ErrNoRows) {
			return apperrors.ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("read the standing site-read offer: %w", err)
		}
		offer, err = decodeSiteReadOffer(raw)
		return err
	})
	if err != nil || offer == nil || offer.OfferedTo != caller || !MayOfferSiteReadChange(ctx, offer.Field) {
		return nil, err
	}
	return offer, nil
}

// ReplaceSiteReadOffer records the offer the caller's latest answered message
// earned, nil for none, in place of whatever the slot held. accepted names the
// standing offer that message accepted, if it did, so the audit row says which
// Margince offer a yes turned into a proposed change.
//
// Audited and not published: the closed catalog carries no site_read type, and
// nothing downstream reads the slot. The change an accepted offer leads to is
// published as company.* when the administrator saves it.
func (s *Store) ReplaceSiteReadOffer(ctx context.Context, readID ids.UUID, offer, accepted *SiteReadOffer) error {
	if err := requireSiteReadOfferAuthority(ctx); err != nil {
		return err
	}
	caller, err := storekit.CapturedBy(ctx)
	if err != nil {
		return err
	}
	if offer != nil {
		if !MayOfferSiteReadChange(ctx, offer.Field) {
			return fmt.Errorf("offer %s: %w", offer.Field, apperrors.ErrPermissionDenied)
		}
		stamped := *offer
		stamped.OfferedTo = caller
		offer = &stamped
	}
	return s.tx(ctx, func(tx pgx.Tx) error {
		return replaceSiteReadOfferTx(ctx, tx, readID, offer, accepted)
	})
}

func replaceSiteReadOfferTx(ctx context.Context, tx pgx.Tx, readID ids.UUID, offer, accepted *SiteReadOffer) error {
	var raw []byte
	err := tx.QueryRow(ctx, `SELECT conversation_offer FROM site_read
		WHERE id = $1 AND target_kind = 'onboarding' FOR UPDATE`, readID).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return apperrors.ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("lock the site-read offer slot: %w", err)
	}
	before, err := decodeSiteReadOffer(raw)
	if err != nil {
		return err
	}
	if before == nil && offer == nil {
		return nil
	}
	encoded, err := encodeSiteReadOffer(offer)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE site_read SET conversation_offer = $2, updated_at = now() WHERE id = $1`,
		readID, encoded); err != nil {
		return fmt.Errorf("replace the site-read offer: %w", err)
	}
	var evidence map[string]any
	if accepted != nil {
		evidence = map[string]any{"accepted_offer": accepted}
	}
	if _, err := storekit.AuditWithEvidence(ctx, tx, actionUpdate, "site_read", readID,
		map[string]any{"conversation_offer": before}, map[string]any{"conversation_offer": offer}, evidence); err != nil {
		return fmt.Errorf("audit the site-read offer: %w", err)
	}
	return nil
}

//nolint:nilnil // an empty slot is the ordinary answer, not an error
func decodeSiteReadOffer(raw []byte) (*SiteReadOffer, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var offer SiteReadOffer
	if err := json.Unmarshal(raw, &offer); err != nil {
		return nil, fmt.Errorf("the site-read offer slot is unreadable: %w", err)
	}
	return &offer, nil
}

// encodeSiteReadOffer spells an empty slot as SQL NULL, never as the JSON null
// a nil pointer marshals to, so "no offer" has one spelling in the column.
func encodeSiteReadOffer(offer *SiteReadOffer) ([]byte, error) {
	if offer == nil {
		return nil, nil
	}
	encoded, err := json.Marshal(offer)
	if err != nil {
		return nil, fmt.Errorf("encode the site-read offer: %w", err)
	}
	return encoded, nil
}
