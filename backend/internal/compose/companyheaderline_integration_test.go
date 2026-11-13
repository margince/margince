// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// An in-contract offer summary always reaches the company header.

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
)

// anchorDescription reads the header line the company view renders, straight
// off the column — what was WRITTEN is the question, not what an assembler
// hands back.
func anchorDescription(t *testing.T, e *integration.Env) string {
	t.Helper()
	var description *string
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT description FROM company WHERE is_anchor AND archived_at IS NULL`).Scan(&description)
	}); err != nil {
		t.Fatalf("reading the anchor's header line: %v", err)
	}
	if description == nil {
		return ""
	}
	return *description
}

// An in-contract summary fills the header whole: the header column admits the
// document limit, so no in-contract summary is cut or dropped.
func TestALongOfferSummaryStillFillsTheCompanyHeader(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.As(e.Rep1, nil, integration.AdminPerms)
	summary := strings.TrimSpace(strings.Repeat("We build warehouse robotics for mid-market logistics. ", 29))

	if _, err := e.Contacts.SaveCompany(ctx, contacts.SaveCompanyInput{
		DisplayName: "Acme Robotics",
		Fields:      map[string]*string{"offer_summary": &summary},
	}); err != nil {
		t.Fatalf("saving the company profile: %v", err)
	}

	if header := anchorDescription(t, e); header != summary {
		t.Errorf("the header holds %d of the summary's %d characters; an in-contract summary must reach it whole",
			len([]rune(header)), len([]rune(summary)))
	}
}
