// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

import (
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A signature's title becomes the role of the current job, so the company's
// contact list names the same role the contact page shows. A role a user set
// is never replaced.
func TestASignatureTitleFillsTheCurrentJobsRole(t *testing.T) {
	for name, humanRole := range map[string]string{"empty role": "", "role a user set": "Head of Buying", "no relationship grant": ""} {
		t.Run(name, func(t *testing.T) {
			e := setupDedupe(t)
			ctx := e.as()
			e.seedSignatureCandidate(ctx, t, "Role "+name, "connector:gmail")
			got, err := e.store.SignatureCandidates(ctx, 50, true)
			if err != nil {
				t.Fatal(err)
			}
			var picked SignatureCandidate
			for _, c := range got {
				if c.FullName == "Role "+name {
					picked = c
				}
			}
			company, err := e.store.CreateCompany(ctx, CreateCompanyInput{DisplayName: "Employer " + name, Source: "manual"})
			if err != nil {
				t.Fatal(err)
			}
			companyID, current, primary := ids.From[ids.CompanyKind](ids.UUID(company.Id)), "current", true
			job, err := e.store.CreateRelationship(ctx, CreateRelationshipInput{
				Kind: "employment", ContactID: &picked.ContactID, CompanyID: &companyID,
				EmploymentStatus: &current, IsCurrentPrimary: &primary, Source: "manual",
			})
			if err != nil {
				t.Fatal(err)
			}
			want := "VP Finance"
			if humanRole != "" {
				if _, err := e.store.UpdateRelationship(e.asEditor(), job.ID, UpdateRelationshipInput{Role: &humanRole}); err != nil {
					t.Fatal(err)
				}
				want = humanRole
			}
			applier := e.asEditor()
			if name == "no relationship grant" {
				// The title still lands; the role is a relationship edit this reader may not make.
				applier, want = ctx, ""
			}
			if _, err := e.store.ApplySignatureFields(applier, picked.ContactID, picked.ActivityID, []SignatureField{
				{Name: "title", Value: "VP Finance", Evidence: "Regards, Dana | VP Finance", Confidence: 0.95},
			}, true); err != nil {
				t.Fatal(err)
			}
			var role *string
			if err := e.store.tx(ctx, func(tx pgx.Tx) error {
				return tx.QueryRow(ctx, `SELECT role FROM relationship WHERE id = $1`, job.ID).Scan(&role)
			}); err != nil {
				t.Fatal(err)
			}
			stored := ""
			if role != nil {
				stored = *role
			}
			if stored != want {
				t.Errorf("role = %q, want %q", stored, want)
			}
		})
	}
}
