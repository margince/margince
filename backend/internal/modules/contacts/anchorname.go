// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// errInstallationRenameUnwired refuses an anchor rename the installation name
// could not follow; a store wired without the renamer fails here, never drifts.
var errInstallationRenameUnwired = errors.New(
	"contacts: this store renames the anchor company but has no installation renamer; wire WithInstallationRenamer")

// InstallationRenamer writes the installation's name on the caller's
// transaction. Identity owns that setting, so compose binds it.
type InstallationRenamer func(ctx context.Context, tx pgx.Tx, name string) error

// WithInstallationRenamer wires the rename an administrator renaming the anchor company owes.
func (s *Store) WithInstallationRenamer(rename InstallationRenamer) *Store {
	s.renameInstallation = rename
	return s
}

// WithInstallationRenamer wires the rename into the transport's own store.
func (h Handlers) WithInstallationRenamer(rename InstallationRenamer) Handlers {
	h.store = h.store.WithInstallationRenamer(rename)
	return h
}

// WithInstallationRenamer wires the rename into the provider's store.
func (p *Provider) WithInstallationRenamer(rename InstallationRenamer) *Provider {
	p.store = p.store.WithInstallationRenamer(rename)
	return p
}

// followAnchorNameTx makes the installation's name the anchor company's
// display name, after a write that moved companyID's display name.
func (s *Store) followAnchorNameTx(ctx context.Context, tx pgx.Tx, companyID ids.CompanyID) error {
	var anchor bool
	var name string
	err := tx.QueryRow(ctx,
		`SELECT is_anchor, display_name FROM company WHERE id = $1`, companyID).Scan(&anchor, &name)
	if errors.Is(err, pgx.ErrNoRows) {
		return apperrors.ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("contacts: reading company %s for the installation name: %w", companyID, err)
	}
	if !anchor {
		return nil
	}
	if err := requireAnchorRenamer(ctx); err != nil {
		return err
	}
	if code, why := values.CompanyNameFault(name); code != "" {
		return httperr.Validation(fieldDisplayName, code, why)
	}
	if s.renameInstallation == nil {
		return errInstallationRenameUnwired
	}
	return s.renameInstallation(ctx, tx, name)
}

// followAnchorSaveTx follows a save of the anchor that wrote its name.
func (s *Store) followAnchorSaveTx(ctx context.Context, tx pgx.Tx, target anchorTarget) error {
	if !target.named {
		return nil
	}
	return s.followAnchorNameTx(ctx, tx, target.id)
}

// requireAnchorRenamer admits only a human administrator, because the anchor's
// name is the installation's and its settings are theirs to change.
func requireAnchorRenamer(ctx context.Context) error {
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return err
	}
	if _, human := principal.HumanUserID(by); !human {
		return &AnchorProtectedError{
			Field:  fieldDisplayName,
			Action: "its name is the installation's name, so only an administrator renames it, on the company profile",
		}
	}
	err = requireAnchorAdministrator(ctx)
	if errors.Is(err, apperrors.ErrPermissionDenied) {
		return fmt.Errorf("only an administrator renames the company, because its name is the installation's name: %w",
			apperrors.ErrPermissionDenied)
	}
	return err
}
