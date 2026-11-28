// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose/installseam"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/customfields"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/projects"
	"github.com/margince/margince/backend/internal/platform/database"
)

// InstallationDB is the pool bound to the installation's own workspace
// (ADR-0091 §9 step 3): the handle a store uses instead of asking every caller
// to have put a workspace in ctx first.
//
// The handle resolves the company itself (database.New) and keeps the first
// answer, so the lookup happens once per handle. A second handle never sees a
// stale answer from the first.
func InstallationDB(pool *pgxpool.Pool) *database.DB {
	return database.New(pool)
}

// ProjectsStore builds this module's store over the installation's workspace,
// with the custom-field catalog wired exactly as the HTTP surface wires it.
// Spelled once here because five composition sites need the same store and a
// second spelling is a second answer to one question.
func ProjectsStore(pool *pgxpool.Pool) *projects.Store {
	return ProjectsStoreOver(InstallationDB(pool))
}

// ProjectsStoreOver is ProjectsStore over a handle whose workspace is already
// decided — the datasource provider's path, and a suite that names its own
// workspace rather than resolving the installation's singleton.
func ProjectsStoreOver(db *database.DB) *projects.Store {
	return projects.NewStore(db).
		WithFieldCatalog(customfields.NewService(db.Pool(), nil)).
		WithCompanyEdges(contacts.AttachCompanyToProjectTx, projects.CompaniesFrom(contacts.CompaniesOnProjectTx), contacts.FirstCustomersOnProjectsTx).
		WithDealLink(installseam.ProjectDealLink(deals.NewStore(db, DealsInstallation())))
}
