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

// SetLegalEntityLogo points an entity at uploaded bytes and hands back the key
// they replaced.
func (s *Store) SetLegalEntityLogo(ctx context.Context, id ids.LegalEntityID, objectKey string) (*string, error) {
	return s.writeLegalEntityLogo(ctx, id, &objectKey)
}

// ClearLegalEntityLogo takes an entity's logo off and hands back its key. An
// entity with no logo is not an error.
func (s *Store) ClearLegalEntityLogo(ctx context.Context, id ids.LegalEntityID) (*string, error) {
	return s.writeLegalEntityLogo(ctx, id, nil)
}

func (s *Store) writeLegalEntityLogo(ctx context.Context, id ids.LegalEntityID, objectKey *string) (superseded *string, err error) {
	if err := auth.Require(ctx, legalEntityObject, principal.ActionUpdate); err != nil {
		return nil, err
	}
	err = s.tx(ctx, func(tx pgx.Tx) error {
		if _, err := storekit.LockRow(ctx, tx, legalEntityTable, id.UUID, storekit.LiveOnly); err != nil {
			return err
		}
		var isDefault bool
		if err := tx.QueryRow(ctx, `SELECT is_default, logo_object_key FROM legal_entity WHERE id = $1`, id).
			Scan(&isDefault, &superseded); err != nil {
			return fmt.Errorf("read the legal entity logo: %w", err)
		}
		if isDefault {
			return errDefaultEntityLogo
		}
		if derefString(superseded) == derefString(objectKey) {
			return nil
		}
		if _, err := tx.Exec(ctx, `UPDATE legal_entity SET logo_object_key = $2 WHERE id = $1`, id, objectKey); err != nil {
			return fmt.Errorf("write the legal entity logo: %w", err)
		}
		// Cleared on this transaction, so the ledger stops treating the key as
		// an orphan exactly when the column that names it commits.
		if objectKey != nil {
			if err := storedobjects.Clear(ctx, tx, *objectKey); err != nil {
				return err
			}
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

// LegalEntityLogoKey is the key an entity's logo bytes live at. An entity with
// no logo of its own, the default included, is ErrNotFound.
func (s *Store) LegalEntityLogoKey(ctx context.Context, id ids.LegalEntityID) (string, error) {
	if err := auth.Require(ctx, legalEntityObject, principal.ActionRead); err != nil {
		return "", err
	}
	var key *string
	err := s.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT logo_object_key FROM legal_entity WHERE id = $1`, id).Scan(&key)
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
