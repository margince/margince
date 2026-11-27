// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration_test

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// A provider's name, company and title past their columns' limit land cut to
// it, so a capture never fails on text nobody can shorten.
func TestACapturedLeadsTextPastItsLimitLandsCut(t *testing.T) {
	t.Parallel()
	e := integration.Setup(t)
	ctx := principal.WithCorrelationID(principal.WithWorkspaceID(context.Background(), e.WS), ids.NewV7())
	ctx = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalConnector, ID: "connector:test", OnBehalfOf: e.AdminUser,
		Permissions: principal.Permissions{
			Objects:  map[string]principal.ObjectGrant{"lead": {Create: true, Read: true}},
			RowScope: principal.RowScopeAll,
		},
	})
	long := strings.Repeat("x", values.TextLabelCharacters+5)
	ref, err := compose.NewCaptureSink(e.Pool, compose.CaptureConfig{}).Upsert(ctx, connector.NormalizedRecord{
		EntityType: "lead",
		NaturalKey: connector.NaturalKey{SourceSystem: "test", SourceID: "long-name-1"},
		Fields:     capture.LeadFields{FullName: long, CompanyName: long, Title: long},
		Source:     "test:long-name-1", CapturedBy: "connector:test",
	})
	if err != nil {
		t.Fatalf("capturing a lead with long text: %v", err)
	}
	var name, company, title string
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT full_name, company_name, title FROM lead WHERE id = $1`, ref.ID).
			Scan(&name, &company, &title)
	}); err != nil {
		t.Fatal(err)
	}
	for column, got := range map[string]string{"full_name": name, "company_name": company, "title": title} {
		if got != long[:values.TextLabelCharacters] {
			t.Errorf("%s holds %d characters, want the first %d", column, len(got), values.TextLabelCharacters)
		}
	}
}
