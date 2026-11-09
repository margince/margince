// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/authz"
	"github.com/margince/margince/backend/internal/shared/ports/authz/authztest"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

const fixtureOwnerEmail = "fixture-owner@example.test"

type internalMailAuthority struct{}

func (internalMailAuthority) EffectiveRBAC(context.Context, ids.UUID, ids.UUID) (authz.RBAC, error) {
	return authz.RBAC{Permissions: principal.Permissions{Objects: map[string]principal.ObjectGrant{
		"activity": {Create: true}, "contact": {Create: true}, "lead": {Create: true},
	}}}, nil
}

func (internalMailAuthority) SeatType(context.Context, ids.UUID, ids.UUID) (principal.SeatType, error) {
	return principal.SeatFull, nil
}

type internalMailConnector struct{}

func (internalMailConnector) Descriptor() connector.Descriptor {
	return connector.Descriptor{Name: "gmail", Version: "fixture"}
}

func (internalMailConnector) Authenticate(context.Context, connector.AuthRequest) (connector.Auth, error) {
	return connector.Auth("fixture-token"), nil
}

func (internalMailConnector) Sync(_ context.Context, _ connector.Auth, cursor connector.Cursor, _ connector.Sink) (connector.Cursor, error) {
	return cursor, nil
}

func (internalMailConnector) Normalize(context.Context, connector.RawRecord) ([]connector.NormalizedRecord, error) {
	return nil, nil
}
func (internalMailConnector) HealthCheck(context.Context, connector.Auth) error { return nil }
func (internalMailConnector) AccountLabel(connector.Auth) (string, error) {
	return fixtureOwnerEmail, nil
}

type internalMailLiveProvider struct {
	internalMailConnector
	act func(context.Context, connector.Sink) error
}

func (p internalMailLiveProvider) Sync(ctx context.Context, _ connector.Auth, cursor connector.Cursor, sink connector.Sink) (connector.Cursor, error) {
	return cursor, p.act(ctx, sink)
}

func newInternalMailRegistryFixture(t *testing.T) (context.Context, *capture.Registry, keyvault.Vault, ids.WorkspaceID, *pgxpool.Pool) {
	t.Helper()
	e := SetupSearch(t)
	ctx := principal.WithCorrelationID(e.AsTeamRep(e.Rep1, e.Team1), ids.NewV7())
	vault := keyvault.NewMemory()
	ws := ids.From[ids.WorkspaceKind](e.WS)
	registry := capture.NewRegistry(database.BindTo(e.Pool, ws), nil, internalMailAuthority{}, vault)
	registry.Register(internalMailConnector{})
	return ctx, registry, vault, ws, e.Pool
}

func operationalMailRecord(sourceID, counterparty string, addresses ...string) connector.NormalizedRecord {
	return connector.NormalizedRecord{
		EntityType: "activity", NaturalKey: connector.NaturalKey{SourceSystem: connector.EmailSourceSystem, SourceID: sourceID},
		Fields: capture.ActivityFields{Kind: "email", Subject: "Salary review for the team", Body: "Confidential: the numbers we agreed for next quarter.", Direction: connector.DirectionInbound},
		Source: "imap:" + sourceID, CapturedBy: "connector:imap",
		Raw:          []byte("From: " + counterparty + "\r\nSubject: Salary review\r\n\r\nBody."),
		Counterparty: connector.Counterparty{Email: counterparty, Domain: "example.test", Direction: connector.DirectionInbound},
		Addresses:    addresses,
	}
}

func (r internalMailAuthority) AdmittedAuthority(ctx context.Context, ws, human, _ ids.UUID) (authz.RBAC, principal.SeatType, error) {
	return authztest.AdmittedFromPair(ctx, ws, human, r.EffectiveRBAC, r.SeatType)
}
