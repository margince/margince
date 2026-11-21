// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

import (
	"context"
	"fmt"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A company created again under its legal form lands on the review queue
// through the create path. The two share no domain or fact, and neither is
// merged. Each form is one some market writes after the name.
func TestACompanyNamedAgainWithItsLegalFormIsSuggestedForReview(t *testing.T) {
	for i, form := range []string{
		"Inc", "OHG", "KGaA", "e.V.", "UG (haftungsbeschränkt)", "GmbH & Co. KGaA",
		"S.p.A.", "SARL", "S.L.", "AB", "A/S", "Pte. Ltd.", "Sdn Bhd",
	} {
		t.Run(form, func(t *testing.T) {
			e := setupDedupe(t)
			ctx := e.as()
			plain := createNamedCompany(ctx, t, e, "Example Telecom", fmt.Sprintf("plain%d.test", i))
			formal := createNamedCompany(ctx, t, e, "Example Telecom "+form, fmt.Sprintf("formal%d.test", i))
			rows := openCandidates(ctx, t, e, "company")
			if len(rows) != 1 {
				t.Fatalf("open company queue holds %d candidates, want 1", len(rows))
			}
			if got := map[ids.UUID]bool{rows[0].LeftID: true, rows[0].RightID: true}; !got[plain] || !got[formal] {
				t.Fatalf("pair {%s,%s} does not name both companies", rows[0].LeftID, rows[0].RightID)
			}
			if live := e.count(ctx, t, `SELECT count(*) FROM company WHERE archived_at IS NULL AND id = ANY($1)`,
				[]ids.UUID{plain, formal}); live != 2 {
				t.Fatalf("%d of the two companies are live, want both: a suggestion merges nothing", live)
			}
		})
	}
}

// An everyday last word is not a legal form. A different brand under the same
// legal form is a different company. Neither files a pair on its name alone.
func TestASimilarNameWithoutTheSameBrandIsNotSuggested(t *testing.T) {
	for i, pair := range [][2]string{
		{"Example Day", "Example Day Spa"},
		{"Example Telecom", "Example Media OHG"},
		{"Example Telecom AB", "Example Telekom AB"},
	} {
		t.Run(pair[1], func(t *testing.T) {
			e := setupDedupe(t)
			ctx := e.as()
			createNamedCompany(ctx, t, e, pair[0], fmt.Sprintf("first%d.test", i))
			createNamedCompany(ctx, t, e, pair[1], fmt.Sprintf("second%d.test", i))
			if rows := openCandidates(ctx, t, e, "company"); len(rows) != 0 {
				t.Fatalf("%q and %q filed %d pairs on their names alone, want none", pair[0], pair[1], len(rows))
			}
		})
	}
}

func createNamedCompany(ctx context.Context, t *testing.T, e *dedupeEnv, name, domain string) ids.UUID {
	t.Helper()
	company, err := e.store.CreateCompany(ctx, CreateCompanyInput{
		DisplayName: name, Source: "manual",
		Domains: []CompanyDomainInput{{Domain: domain, IsPrimary: true}},
	})
	if err != nil {
		t.Fatalf("create %q: %v", name, err)
	}
	return ids.UUID(company.Id)
}
