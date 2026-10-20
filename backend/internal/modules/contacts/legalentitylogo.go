// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// A legal entity's logo. The default entity wears the anchor company's, so
// only a further entity stores a key of its own. The store is blob-free, as the
// company logo's is: the caller puts the bytes first and collects the key this
// write superseded.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/storedobjects"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

var errDefaultEntityLogo = &LegalEntityRefusal{
	codeFieldOnCompanyForm,
	"the default legal entity wears the company logo; change it on the company form",
}

// EnsureLegalEntityLogoWritable answers whether a logo write on this entity
// would be refused, before the caller decodes or stores any bytes. The write
// itself asks again under its lock, so this is the early refusal and not the
// gate.
func (s *Store) EnsureLegalEntityLogoWritable(ctx context.Context, id ids.LegalEntityID, ifVersion *int64) error {
	if err := auth.Require(ctx, legalEntityObject, principal.ActionUpdate); err != nil {
		return err
	}
	return s.tx(ctx, func(tx pgx.Tx) error {
		_, err := lockedLogo(ctx, tx, id, ifVersion)
		return err
	})
}

// SetLegalEntityLogo points an entity at uploaded bytes and hands back the key
// they replaced.
func (s *Store) SetLegalEntityLogo(ctx context.Context, id ids.LegalEntityID, objectKey string, ifVersion *int64) (*string, error) {
	return s.writeLegalEntityLogo(ctx, id, &objectKey, ifVersion)
}

// ClearLegalEntityLogo takes an entity's logo off and hands back its key. An
// entity with no logo is not an error.
func (s *Store) ClearLegalEntityLogo(ctx context.Context, id ids.LegalEntityID, ifVersion *int64) (*string, error) {
	return s.writeLegalEntityLogo(ctx, id, nil, ifVersion)
}

// lockedLogo locks a live entity and answers the key its logo is stored at. The
// default entity and a stale version are refused.
func lockedLogo(ctx context.Context, tx pgx.Tx, id ids.LegalEntityID, ifVersion *int64) (*string, error) {
	if _, err := storekit.LockRow(ctx, tx, legalEntityTable, id.UUID, storekit.LiveOnly); err != nil {
		return nil, err
	}
	if err := storekit.EnsureIfMatch(ctx, tx, legalEntityTable, id.UUID, ifVersion); err != nil {
		return nil, err
	}
	var isDefault bool
	var key *string
	var args legalEntityArgs
	if err := tx.QueryRow(ctx, `SELECT is_default, logo_object_key FROM legal_entity WHERE id = `+args.add(id), args...).
		Scan(&isDefault, &key); err != nil {
		return nil, fmt.Errorf("read the legal entity logo: %w", err)
	}
	if isDefault {
		return nil, errDefaultEntityLogo
	}
	return key, nil
}

func (s *Store) writeLegalEntityLogo(ctx context.Context, id ids.LegalEntityID, objectKey *string, ifVersion *int64) (superseded *string, err error) {
	if err := auth.Require(ctx, legalEntityObject, principal.ActionUpdate); err != nil {
		return nil, err
	}
	err = s.tx(ctx, func(tx pgx.Tx) error {
		superseded, err = lockedLogo(ctx, tx, id, ifVersion)
		if err != nil || derefString(superseded) == derefString(objectKey) {
			return err
		}
		var args legalEntityArgs
		if _, err := tx.Exec(ctx, `UPDATE legal_entity SET logo_object_key = `+args.add(objectKey)+
			` WHERE id = `+args.add(id), args...); err != nil {
			return fmt.Errorf("write the legal entity logo: %w", err)
		}
		if err := handOverLogoKeys(ctx, tx, superseded, objectKey); err != nil {
			return err
		}
		auditID, err := storekit.Audit(ctx, tx, actionUpdate, legalEntityObject, id.UUID,
			map[string]any{columnLogoKey: superseded}, map[string]any{columnLogoKey: objectKey})
		if err != nil {
			return err
		}
		return storekit.EmitEvent(ctx, tx, auditID, id.UUID,
			crmcontracts.InternalEventLegalEntityUpdated{ChangedFields: []string{"logo_url"}})
	})
	if err != nil {
		return nil, err
	}
	return supersededObject(superseded, derefString(objectKey)), nil
}

// handOverLogoKeys moves the ledger along with the column, on the write's own
// transaction. The new key stops being provisional. The key this write drops
// becomes provisional, so the sweep collects its bytes when the caller's delete
// fails, and a rolled-back write collects nothing.
func handOverLogoKeys(ctx context.Context, tx pgx.Tx, dropped, adopted *string) error {
	if adopted != nil {
		if err := storedobjects.Clear(ctx, tx, *adopted); err != nil {
			return err
		}
	}
	if dropped == nil || *dropped == "" {
		return nil
	}
	return storedobjects.RecordTx(ctx, tx, storedobjects.KindLogo, *dropped)
}

// LegalEntityLogoKey is the key an entity's logo bytes live at. An entity with
// no logo of its own, the default included, is ErrNotFound.
func (s *Store) LegalEntityLogoKey(ctx context.Context, id ids.LegalEntityID) (string, error) {
	if err := auth.Require(ctx, legalEntityObject, principal.ActionRead); err != nil {
		return "", err
	}
	var key *string
	err := s.tx(ctx, func(tx pgx.Tx) error {
		var args legalEntityArgs
		return tx.QueryRow(ctx, `SELECT logo_object_key FROM legal_entity WHERE id = `+args.add(id), args...).Scan(&key)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", apperrors.ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("read the legal entity logo key: %w", err)
	}
	if key == nil || *key == "" {
		return "", apperrors.ErrNotFound
	}
	return *key, nil
}

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
