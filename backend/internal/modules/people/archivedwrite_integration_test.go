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
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/gradionhq/margince/backend/internal/shared/apperrors"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
)

// archiveRow retires a row by column rather than through the module's Archive
// entry point: the rep these suites run as holds no delete grant, and widening
// it would change what every other test in the package proves. The state under
// test is the archived row, not how it came to be one.
//
// table is a test-local literal, never caller input.
func archiveRow(ctx context.Context, t *testing.T, e *dedupeEnv, table string, id ids.UUID) {
	t.Helper()
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE `+table+` SET archived_at = now() WHERE id = $1`, id)
		return err
	}); err != nil {
		t.Fatalf("archive %s: %v", table, err)
	}
}

func TestAnArchivedRecordTakesNoStagedApply(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	personID, orgID := e.seedEmployedPerson(ctx, t,
		"Mira Halvorsen", "mira@voltaq.test", "Voltaq Systems GmbH", "voltaq.test")
	archiveRow(ctx, t, e, "organization", orgID.UUID)
	archiveRow(ctx, t, e, "person", personID.UUID)

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
				return e.store.ApplyDeepRead(ctx, DeepReadProposal{
					OrganizationID: orgID,
					SourceURL:      "https://voltaq.test/about",
					SiteReadID:     ids.NewV7(),
					Fields: []DeepReadField{{
						Field: "industry", Value: "Energietechnik",
						EvidenceSnippet: `"Energietechnik"`,
						SourceURL:       "https://voltaq.test/about", Confidence: 0.8,
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

	// Nothing landed. The columns these applies write are the ones the census
	// on #2145 found five paths competing over, so the assertion names them
	// rather than trusting that a refused call wrote nothing.
	for column, want := range map[string]string{
		"legal_name": "", "industry": "", "size_band": "",
	} {
		if got := archivedOrgColumn(ctx, t, e, orgID, column); got != want {
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

// archivedOrgColumn reads a nullable organization column as its empty string,
// which orgColumn cannot do — that helper scans into a string and a NULL
// legal_name on a company nobody has named would fail the scan rather than the
// assertion.
func archivedOrgColumn(
	ctx context.Context, t *testing.T, e *dedupeEnv, orgID ids.OrganizationID, column string,
) string {
	t.Helper()
	var v string
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT coalesce(`+column+`, '') FROM organization WHERE id = $1`, orgID).Scan(&v)
	}); err != nil {
		t.Fatalf("read organization.%s: %v", column, err)
	}
	return v
}
