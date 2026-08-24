// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package people

// A write refuses an archived row, on the four paths in this module that used
// to reach one.
//
// The window these share is not a race. A scrape or deep-read proposal sits in
// the inbox while somebody archives the company, and the approval arrives
// afterwards — or, for deep-read auto-apply, arrives with no human in the loop
// at all. Each gate asked auth.EnsureWritable, which omits the liveness filter
// and, for an actor with unbounded row scope, skips the existence check
// outright; so the apply landed on the retired record, shipped an
// organization.updated event for a row PATCH /organizations/{id} refuses, and
// (for the person path) wrote a declared-PII row back onto a subject that
// Art. 17 erasure had just cleared.

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/gradionhq/margince/backend/internal/shared/apperrors"
)

func TestAnArchivedRecordTakesNoStagedApply(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	personID, orgID := e.seedEmployedPerson(ctx, t,
		"Mira Halvorsen", "mira@voltaq.test", "Voltaq Systems GmbH", "voltaq.test")

	// Commissioned while the company is still live, because a deep-read fact
	// carries a real site_read FK. Minted here rather than faked so the deep
	// read below fails on the GATE when the gate is broken, instead of dying on
	// a foreign key and reporting a refusal that never happened.
	dossier, _, err := e.store.StartSiteRead(ctx, orgID, "https://voltaq.test/", "rep")
	if err != nil {
		t.Fatalf("commission the dossier while the company is live: %v", err)
	}

	archiver := e.asArchiver()
	if _, err := e.store.ArchiveOrganization(archiver, orgID, nil); err != nil {
		t.Fatalf("archive organization: %v", err)
	}
	if _, err := e.store.ArchivePerson(archiver, personID, nil); err != nil {
		t.Fatalf("archive person: %v", err)
	}

	for _, tc := range []struct {
		name string
		// why names the surface a reader should go looking at when this fails,
		// because the refusal itself cannot say which apply produced it.
		why  string
		call func() error
	}{
		{
			name: "an accepted scrape enrichment",
			why:  "ApplyEnrichment (enrich.go) — compose/scrapeaccept.go approves this after the archive",
			call: func() error {
				return e.store.ApplyEnrichment(ctx, orgID, ApplyColdStartProfileInput{
					SourceURL: "https://voltaq.test/impressum",
					Fields: []ColdStartFieldInput{{
						Field: "legal_name", Value: "Voltaq Systems GmbH & Co. KG",
						EvidenceSnippet: `"Voltaq Systems GmbH & Co. KG"`,
						SourceURL:       "https://voltaq.test/impressum", Confidence: 0.9,
					}},
				})
			},
		},
		{
			name: "an accepted deep read",
			why:  "ApplyDeepReadTx (organizationfact.go) — reachable with no human in the loop via deepreadautoapply",
			call: func() error {
				// Carries a FACT as well as a field, and specifically an
				// employee_range one: size_band is filled from an applied fact
				// (fillSizeBandFromFacts), so a fields-only proposal would leave
				// that column empty however the gate behaved and the assertion
				// on it below would pass over a broken branch.
				return e.store.ApplyDeepRead(ctx, DeepReadProposal{
					OrganizationID: orgID,
					SourceURL:      "https://voltaq.test/about",
					SiteReadID:     dossier.ID,
					Fields: []DeepReadField{{
						Field: "industry", Value: "Energietechnik",
						EvidenceSnippet: `"Energietechnik"`,
						SourceURL:       "https://voltaq.test/about", Confidence: 0.8,
					}},
					Facts: []DeepReadFact{{
						Category: factCategoryCompany, Field: FactEmployeeRange,
						Value: "51-200", EvidenceSnippet: `"51-200 Mitarbeitende"`,
						SourceURL: "https://voltaq.test/about", Confidence: 0.8,
					}},
				})
			},
		},
		{
			name: "a dossier commissioned for the company",
			why:  "StartSiteRead (siteread.go) — an archived company has no dossier to commission",
			call: func() error {
				_, _, err := e.store.StartSiteRead(ctx, orgID, "https://voltaq.test/", "rep")
				return err
			},
		},
		{
			name: "approved discovered fields for the person",
			why:  "ApplyDiscoveredFields (searchpersonfields.go) — person_profile_field is declared PII and erasure had cleared it",
			call: func() error {
				_, err := e.store.ApplyDiscoveredFields(ctx, personID, []DiscoveredField{{
					Field: "linkedin", Value: "https://www.linkedin.com/in/mira-halvorsen",
					EvidenceSnippet: "Mira Halvorsen — Voltaq Systems GmbH",
				}})
				return err
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// ErrNotFound and not ErrPermissionDenied: an archived record is
			// gone as far as a write is concerned, and existence-hiding owes
			// the same 404 a row-scope miss owes.
			if err := tc.call(); !errors.Is(err, apperrors.ErrNotFound) {
				t.Fatalf("got %v, want not found — see %s", err, tc.why)
			}
		})
	}

	// Nothing landed. Asserted on the columns rather than inferred from the
	// four refusals, because a gate that returns the right error while still
	// committing the write is the failure this exists to notice — and these are
	// the columns the applies above would have moved.
	for column, want := range map[string]string{
		"legal_name": "", "industry": "", "size_band": "",
	} {
		if got := orgColumn(ctx, t, e, orgID, column); got != want {
			t.Errorf("organization.%s = %q after refused applies, want %q", column, got, want)
		}
	}
	var profileFields int
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT count(*) FROM person_profile_field WHERE person_id = $1`, personID).Scan(&profileFields)
	}); err != nil {
		t.Fatalf("count person_profile_field: %v", err)
	}
	if profileFields != 0 {
		t.Errorf("person_profile_field rows = %d, want 0 — a refused apply wrote PII onto an archived subject", profileFields)
	}
}
