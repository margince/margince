// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// errInstallationRenameUnwired refuses an anchor rename the installation name
// could not follow; a store wired without the renamer fails here, never drifts.
var errInstallationRenameUnwired = errors.New(
	"contacts: this store renames the anchor company but has no installation renamer; wire WithInstallationRenamer")

// InstallationRenamer writes the installation's name on the caller's
// transaction. Identity owns that setting, so compose binds it.
type InstallationRenamer func(ctx context.Context, tx pgx.Tx, name string) error

// WithInstallationRenamer wires the rename a human renaming the anchor company owes.
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
// display name, after a write that may have renamed companyID.
//
// Only a human's write moves it. An agent or a site read only proposes a name.
func (s *Store) followAnchorNameTx(ctx context.Context, tx pgx.Tx, companyID ids.CompanyID) error {
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return err
	}
	if _, human := principal.HumanUserID(by); !human {
		return nil
	}
	var anchor bool
	var name string
	err = tx.QueryRow(ctx,
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
	if s.renameInstallation == nil {
		return errInstallationRenameUnwired
	}
	return s.renameInstallation(ctx, tx, name)
}
