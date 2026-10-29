// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// clientRegistrationReadActor names the switch read in the audit trail: no
// human asked for it, a tool asked to sign someone in.
const clientRegistrationReadActor = identity.ClientRegistrationReadActor

// WithClientRegistrationLimit carries the deployment file's
// mcp.client_registration: which automatic methods admins may turn on at all.
func WithClientRegistrationLimit(limit identity.RegistrationMethods) Option {
	return func(s *Server, _ *pgxpool.Pool) { s.clientRegistrationLimit = &limit }
}

// publishClientRegistration hands the limit and the admin's switches to the
// authorization server. It runs after the option loop, so a server built
// without the option reads both methods as allowed, as before the switches.
func (s *Server) publishClientRegistration(pool *pgxpool.Pool, svc *identity.Service) {
	limit := identity.RegistrationMethods{Dynamic: true, MetadataDocument: true}
	if s.clientRegistrationLimit != nil {
		limit = *s.clientRegistrationLimit
	}
	if pool == nil {
		return
	}
	s.authHandlers = s.WithClientRegistration(limit, clientRegistrationSwitches(pool, svc))
}

// clientRegistrationSwitches reads the admin's two switches as the system, as
// enabledOidcProviders reads the sign-in providers. The user a tool is signing
// in holds no agent_client grant, and must not need one.
func clientRegistrationSwitches(pool *pgxpool.Pool, svc *identity.Service) identity.AdminRegistrationSwitches {
	cfg := StoredConfig(pool)
	return func(ctx context.Context) (identity.RegistrationMethods, error) {
		wsID, err := svc.InstallationWorkspace(ctx)
		if err != nil {
			return identity.RegistrationMethods{}, fmt.Errorf("resolving the installation for the client registration switches: %w", err)
		}
		readCtx := principal.SystemActing(principal.WithWorkspaceID(ctx, wsID.UUID), clientRegistrationReadActor)
		dynamic, err := config.StoredVal(readCtx, cfg, identity.DynamicClientRegistration)
		if err != nil {
			return identity.RegistrationMethods{}, fmt.Errorf("reading the self-registration switch: %w", err)
		}
		documents, err := config.StoredVal(readCtx, cfg, identity.MetadataDocumentClients)
		if err != nil {
			return identity.RegistrationMethods{}, fmt.Errorf("reading the metadata-document switch: %w", err)
		}
		return identity.RegistrationMethods{Dynamic: dynamic, MetadataDocument: documents}, nil
	}
}
