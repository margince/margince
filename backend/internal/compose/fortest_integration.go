// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// Wiring the integration lane drives, behind the integration tag so no
// shipped binary carries it.

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// ActAsImportRunForTest binds the principal an import attempt runs as, through
// the rebuild the import job calls, on a context already bound to the workspace.
func ActAsImportRunForTest(ctx context.Context, pool *pgxpool.Pool, run migration.Run) (context.Context, error) {
	return storedActor{authority: identity.NewService(pool)}.actAsRun(ctx, run)
}

// CaptureRegistryWithoutConnectorsForTest is the production registry with no connector.
func CaptureRegistryWithoutConnectorsForTest(pool *pgxpool.Pool, vault keyvault.Vault) *capture.Registry {
	return capture.NewRegistry(InstallationDB(pool), newCaptureSink(pool, CaptureConfig{}), identity.NewService(pool), vault)
}

// SharedMailboxRouterForTest is the /v1 router over the connector and timeline
// handlers on registry. login stands in for the IMAP server's answer.
func SharedMailboxRouterForTest(pool *pgxpool.Pool, registry *capture.Registry, login func(context.Context, connector.AuthRequest) (connector.Auth, error)) http.Handler {
	srv := Server{
		connectorHandlers:  connectorHandlers{registry: registry, imapAuthenticate: login},
		activitiesHandlers: newActivitiesHandlers(pool),
	}
	return crmcontracts.HandlerFromMuxWithBaseURL(srv, chi.NewRouter(), "/v1")
}

// TeamServiceForTest is identity with capture's team seam, as the API wires it.
func TeamServiceForTest(pool *pgxpool.Pool) *identity.Service {
	return identity.NewService(pool).WithMailCapture(mailCaptureSeam{})
}
