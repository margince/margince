// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package people

// A write refuses an archived row, on the five paths in this module that used
// to reach one.
//
// The window these share is not a race. Something decides, time passes, and the
// write arrives afterwards — a scrape or deep-read proposal sitting in the inbox
// until somebody approves it, a deep-read auto-apply with no human in the loop
// at all, or a capture sweep that picks a live candidate and then waits on a
// model call. Whichever it is, the archive lands inside that gap, which makes it
// the ordinary case rather than a contention problem.
//
// They failed it in two different ways, and both are here. Most asked
// auth.EnsureWritable, which omits the liveness filter and, for an actor with
// unbounded row scope, skips the existence check outright. The signature
// enricher asked nothing at all. So the apply landed on the retired record,
// shipped an organization.updated event for a row PATCH /organizations/{id}
// refuses, and (for the person paths) wrote declared-PII rows back onto a
// subject Art. 17 erasure had just cleared.

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
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
		{
			name: "signature fields read out of the person's own mail",
			why: "ApplySignatureFields (enrichsignature.go) — the same declared-PII table as the row above, " +
				"and the path that asked for nothing at all rather than for too little",
			call: func() error {
				// Both column-backed fields, because they land in different
				// places and only one of them is a column: title fills
				// person.title, phone INSERTs a person_phone row. The phone is
				// the arm whose own emptiness predicate argues the wrong way
				// round after an erasure — erasure deletes person_phone, so
				// "no live phone row" is exactly what an erased subject
				// answers.
				_, err := e.store.ApplySignatureFields(ctx, personID, e.openSignatureSource(ctx, t), []SignatureField{
					{
						Name: "title", Value: "Head of Procurement",
						Evidence: "Mira Halvorsen | Head of Procurement", Confidence: 0.9,
					},
					{
						Name: "phone", Value: "+49 30 1234567",
						Evidence: "T +49 30 1234567", Confidence: 0.9,
					},
				})
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
	// five refusals, because a gate that returns the right error while still
	// committing the write is the failure this exists to notice — and these are
	// the columns the applies above would have moved.
	for column, want := range map[string]string{
		"legal_name": "", "industry": "", "size_band": "",
	} {
		if got := orgColumn(ctx, t, e, orgID, column); got != want {
			t.Errorf("organization.%s = %q after refused applies, want %q", column, got, want)
		}
	}
	// The person's own three, counted separately because they are three
	// different writes with three different predicates: the sidecar row carries
	// its liveness on the INSERT, the title on its UPDATE, and the phone on
	// neither until this change. A single count would have been earned by
	// whichever of them still refused.
	var profileFields, phones int
	var title *string
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx,
			`SELECT count(*) FROM person_profile_field WHERE person_id = $1`, personID).Scan(&profileFields); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx,
			`SELECT count(*) FROM person_phone WHERE person_id = $1`, personID).Scan(&phones); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `SELECT title FROM person WHERE id = $1`, personID).Scan(&title)
	}); err != nil {
		t.Fatalf("read the person after refused applies: %v", err)
	}
	if profileFields != 0 {
		t.Errorf("person_profile_field rows = %d, want 0 — a refused apply wrote PII onto an archived subject", profileFields)
	}
	if phones != 0 {
		t.Errorf("person_phone rows = %d, want 0 — a refused apply gave an erased subject a phone number back", phones)
	}
	if title != nil {
		t.Errorf("person.title = %q after refused applies, want unset", *title)
	}
}
