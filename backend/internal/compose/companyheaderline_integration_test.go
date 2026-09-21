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

// A summary longer than the header column still fills the header.
//
// THE DEFECT. The contract accepts 2000 characters of `offer_summary`; the
// column behind the header holds 500. The write used to carry a length test, so
// an in-contract summary over 500 saved as a profile field and wrote NOTHING to
// the header — no error, no audit entry, no signal. The company header stayed
// blank and there was no way to learn why.
//
// 1500 characters is the case the issue names, and it is fully in contract.
func TestALongOfferSummaryStillFillsTheCompanyHeader(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.As(e.Rep1, nil, integration.AdminPerms)
	summary := strings.TrimSpace(strings.Repeat("We build warehouse robotics for mid-market logistics. ", 29))
	if len([]rune(summary)) < 1400 {
		t.Fatalf("the fixture summary is %d characters — too short to be over the column's bound, so "+
			"this would pass without exercising anything", len([]rune(summary)))
	}

	if _, err := e.Contacts.SaveCompany(ctx, contacts.SaveCompanyInput{
		DisplayName: "Acme Robotics",
		Fields:      map[string]*string{"offer_summary": &summary},
	}); err != nil {
		t.Fatalf("saving the company profile: %v", err)
	}

	header := anchorDescription(t, e)
	if header == "" {
		t.Fatal("the header is blank after an in-contract summary was saved — the write was skipped and " +
			"nothing said so, which is the whole defect: a reader sees an empty header and cannot find out why")
	}
	if n := len([]rune(header)); n > 500 {
		t.Errorf("the header holds %d characters, over the column's 500 — the CHECK would have refused it", n)
	}
	if !strings.HasPrefix(summary, header) {
		t.Errorf("the header is not a prefix of the summary, so it says something the profile does not: %q", header)
	}
}

// The boundary: exactly 500 writes whole, 501 writes a prefix. 501 is the case
// that was silently broken.
func TestTheHeaderBoundaryWritesAtFiveHundredAndAtFiveHundredAndOne(t *testing.T) {
	for _, tc := range []struct {
		name   string
		length int
	}{
		{"exactly the column's bound", 500},
		{"one character over", 501},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := integration.Setup(t)
			ctx := e.As(e.Rep1, nil, integration.AdminPerms)
			// Words, so the 501 case has a boundary to cut at — a single long
			// token would exercise the hard-cut arm instead, which
			// headerline_test.go covers on its own.
			summary := strings.TrimSpace(strings.Repeat("robotics ", tc.length/9+1))[:tc.length]

			if _, err := e.Contacts.SaveCompany(ctx, contacts.SaveCompanyInput{
				DisplayName: "Acme Robotics",
				Fields:      map[string]*string{"offer_summary": &summary},
			}); err != nil {
				t.Fatalf("saving a %d-character summary: %v", tc.length, err)
			}

			header := anchorDescription(t, e)
			if header == "" {
				t.Fatalf("a %d-character summary left the header blank", tc.length)
			}
			if !strings.HasPrefix(summary, header) {
				t.Errorf("the header is not a prefix of the summary: %q", header)
			}
			if tc.length <= 500 && header != summary {
				t.Errorf("a summary the column accepts whole was shortened to %d characters", len([]rune(header)))
			}
		})
	}
}
