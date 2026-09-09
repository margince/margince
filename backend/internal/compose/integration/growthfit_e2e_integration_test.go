// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The growth-fit HTTP surface end to end (DOSS-WIRE-2): a company with nothing
// recorded abstains and says what to go and find; recording some of the required
// inputs moves the completeness figure the abstention rests on.
//
// The counting cannot be proven anywhere but here. Completeness is decided from
// columns the sidecar readers project and from rows the real writers shape, and
// a unit test supplies both itself. Only a real database can show that the
// reader actually fetches what the counting reads.

import (
	"context"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The admin BootstrapWorkspace signs in as. Named here because this suite has
// to look the acting reader up by identity to prove the cache row is keyed to
// them rather than shared.
const bootstrappedAdminEmail = "ada@example.com"

type growthFitResponse struct {
	CompanyID   string  `json:"company_id"`
	Band             string  `json:"band"`
	BandCappedReason *string `json:"band_capped_reason"`
	NextStep         *string `json:"next_step"`
	GeneratedBy      string  `json:"generated_by"`
	DataCompleteness struct {
		Present  int      `json:"present"`
		Expected int      `json:"expected"`
		Missing  []string `json:"missing"`
	} `json:"data_completeness"`
}

func TestGrowthFitAbstainsAndNamesWhatToGather(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	companyID := createBareCompany(t, e)

	// A company we hold nothing about. The panel must not read as "poor fit".
	var fresh growthFitResponse
	if status := e.Call(t, "GET", "/v1/companies/"+companyID+"/growth-fit", nil, nil, &fresh); status != http.StatusOK {
		t.Fatalf("GET growth-fit = %d, want 200", status)
	}
	if fresh.Band != "unknown" {
		t.Errorf("band = %q, want unknown — nothing is recorded about this company", fresh.Band)
	}
	if fresh.DataCompleteness.Expected == 0 {
		t.Error("expected = 0 — a completeness figure without its denominator is not one")
	}
	if fresh.DataCompleteness.Present != 0 {
		t.Errorf("present = %d, want 0 for a company with no profile fields and no facts", fresh.DataCompleteness.Present)
	}
	if len(fresh.DataCompleteness.Missing) != fresh.DataCompleteness.Expected {
		t.Errorf("missing named %d of %d absent inputs — the next step is built from this list",
			len(fresh.DataCompleteness.Missing), fresh.DataCompleteness.Expected)
	}
	if fresh.NextStep == nil || *fresh.NextStep == "" {
		t.Error("an abstention with no next step leaves the reader nothing to act on")
	}
	if fresh.GeneratedBy != "deterministic" {
		t.Errorf("generated_by = %q, want deterministic — no model lane is wired", fresh.GeneratedBy)
	}

	// Record three of the required inputs the way a site read would — two
	// written recently, one written long ago — then force a re-assembly. Only
	// the two fresh ones may count, and the figure must MOVE, because a reader
	// that is not seeing rows the database holds looks exactly like a company
	// with nothing recorded.
	seedRequiredProfileFields(t, e, companyID)

	var enriched growthFitResponse
	if status := e.Call(t, "POST", "/v1/companies/"+companyID+"/growth-fit", nil, nil, &enriched); status != http.StatusOK {
		t.Fatalf("POST growth-fit = %d, want 200", status)
	}
	if enriched.DataCompleteness.Present != 2 {
		t.Errorf("present = %d, want 2 — three fields were recorded and one is too old to count",
			enriched.DataCompleteness.Present)
	}
	// The stale one is still OUTSTANDING, so it must still be named as
	// something to go and get. A value too old to rest a judgment on is a gap,
	// not a fact we hold.
	if !namesMissingInput(enriched.DataCompleteness.Missing, "their industry") {
		t.Errorf("missing = %v, want it to still name the stale industry value",
			enriched.DataCompleteness.Missing)
	}
	if enriched.DataCompleteness.Expected != fresh.DataCompleteness.Expected {
		t.Errorf("expected moved from %d to %d — the denominator is the required set, which did not change",
			fresh.DataCompleteness.Expected, enriched.DataCompleteness.Expected)
	}
	// Still short of the floor, so still an abstention — and the two inputs we
	// now hold must have dropped out of what it asks for.
	if enriched.Band != "unknown" {
		t.Errorf("band = %q, want unknown — two of seven is below the abstention floor", enriched.Band)
	}
	for _, named := range enriched.DataCompleteness.Missing {
		if named == "what they offer" || named == "who they sell to" {
			t.Errorf("missing still names %q, which was just recorded", named)
		}
	}
}

// The assembly is cached PER READER, and that is a disclosure guarantee rather
// than an optimisation: a growth fit can rest on records scoped to one reader.
// The row must therefore be keyed to the human who asked for it.
func TestTheGrowthFitCacheRowIsKeyedToTheReaderWhoAskedForIt(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	companyID := createBareCompany(t, e)

	// One real read first, purely to MINT a row. Its fingerprint is the one
	// this company's facts compute to, and a hand-written one would not be:
	// the cache gate rejects a stale fingerprint before it ever consults the
	// reader, so a seeded row with an invented fingerprint is refused for the
	// wrong reason and an unkeyed read would pass this test too.
	var first growthFitResponse
	if status := e.Call(t, "GET", "/v1/companies/"+companyID+"/growth-fit", nil, nil, &first); status != http.StatusOK {
		t.Fatalf("GET growth-fit = %d, want 200", status)
	}
	if first.Band == otherReadersBand {
		t.Fatalf("a bare company assessed as %q, so that band cannot mark the other reader's row", first.Band)
	}

	// Now move that row — fingerprint and all — to a DIFFERENT reader, with a
	// band this company could not produce, and leave the acting reader with
	// nothing of their own. The row is live: it would be served on sight.
	acting := readerByEmail(t, e, bootstrappedAdminEmail)
	other := giveTheCachedRowToAnotherReader(t, e, companyID, acting)

	var second growthFitResponse
	if status := e.Call(t, "GET", "/v1/companies/"+companyID+"/growth-fit", nil, nil, &second); status != http.StatusOK {
		t.Fatalf("second GET growth-fit = %d, want 200", status)
	}
	if second.Band == otherReadersBand {
		t.Fatal("the read served another reader's cached assessment — the cache is keyed on write and not on read")
	}

	// And it wrote its own row rather than overwriting theirs — BOTH halves,
	// because either alone is satisfied by the defect. A write keyed on the
	// company and not the reader replaces the other reader's row and still
	// leaves the acting reader with one of their own, so "mine exists" passes
	// for exactly the keying this test exists to refuse. What separates them is
	// that theirs is still there.
	readers := cachedGrowthFitReaders(t, e, companyID)
	if !slices.Contains(readers, acting) {
		t.Errorf("the acting reader has no row of their own after their read; the cache holds %v", readers)
	}
	if !slices.Contains(readers, other) {
		t.Errorf("the other reader's cached row is gone after somebody else read the same company — the "+
			"cache write is keyed on the company rather than on (company, reader), so one "+
			"reader's assessment evicts another's; the cache holds %v", readers)
	}
}

// cachedGrowthFitReaders returns the readers holding a cached assessment of
// this company. Read as a SET rather than probed one id at a time: what the
// assertions above are about is which rows survived a write, and a per-id
// existence check reports the first missing one without saying what is there
// instead.
func cachedGrowthFitReaders(t *testing.T, e *apptest.AppEnv, companyID string) []ids.UUID {
	t.Helper()
	rows, err := e.Owner.Query(context.Background(),
		`SELECT user_id FROM company_growth_fit WHERE company_id = $1 ORDER BY user_id`, companyID)
	if err != nil {
		t.Fatalf("reading the cached growth-fit rows: %v", err)
	}
	defer rows.Close()
	var readers []ids.UUID
	for rows.Next() {
		var reader ids.UUID
		if scanErr := rows.Scan(&reader); scanErr != nil {
			t.Fatalf("scanning a cached growth-fit row: %v", scanErr)
		}
		readers = append(readers, reader)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("reading the cached growth-fit rows: %v", err)
	}
	return readers
}

func namesMissingInput(missing []string, want string) bool {
	for _, named := range missing {
		if named == want {
			return true
		}
	}
	return false
}

func createBareCompany(t *testing.T, e *apptest.AppEnv) string {
	t.Helper()
	var company struct {
		ID string `json:"id"`
	}
	if status := e.Call(t, "POST", "/v1/companies", AnyMap{
		"display_name": "Voltaq Systems GmbH", "source": "ui",
	}, nil, &company); status != http.StatusCreated {
		t.Fatalf("create company = %d, want 201", status)
	}
	return company.ID
}

// seedRequiredProfileFields writes three of the required inputs the way the
// site-read path actually leaves them: machine-sourced, carrying the evidence
// such a row must have, and with `retrieved_at` UNSET — migration 0194 adds
// that column and nothing in this tree writes it yet, so a row carrying one is
// a shape no writer produces.
//
// Freshness therefore runs off `updated_at`, which is what production measures
// today. Two rows are written now and count; the third is backdated past the
// window and must not, which is the boundary the served figure rests on.
func seedRequiredProfileFields(t *testing.T, e *apptest.AppEnv, companyID string) {
	t.Helper()
	// updated_at is written explicitly because the row's own trigger stamps
	// now() on every write, so a backdated value cannot be produced by an
	// UPDATE after the fact.
	const insert = `
		INSERT INTO company_profile_field (id, company_id, field, value, source, evidence_snippet, source_url, confidence, captured_by, updated_at)
		VALUES ($1, $2, $3, $4, 'site_read', $5, 'https://voltaq.example/about', 0.9,
		        'site_read:seed', $6)`
	now := time.Now().UTC()
	for field, seed := range map[string]struct {
		value   string
		written time.Time
	}{
		"offer_summary": {"Load-shifting software for industrial sites", now.Add(-24 * time.Hour)},
		"icp":           {"Energy-intensive manufacturers", now.Add(-24 * time.Hour)},
		"industry":      {"Industrial software", now.Add(-400 * 24 * time.Hour)},
	} {
		if _, err := e.Owner.Exec(context.Background(), insert,
			ids.NewV7(), companyID, field, seed.value, seed.value, seed.written); err != nil {
			t.Fatalf("seed profile field %s: %v", field, err)
		}
	}
}

// otherReadersBand is a band a company with nothing recorded can never be
// assessed as, so seeing it in a response can only mean the read crossed
// readers rather than reassessing.
const otherReadersBand = "strong"

func readerByEmail(t *testing.T, e *apptest.AppEnv, email string) ids.UUID {
	t.Helper()
	var id ids.UUID
	if err := e.Owner.QueryRow(context.Background(),
		`SELECT id FROM app_user WHERE email = $1`, email).Scan(&id); err != nil {
		t.Fatalf("reader lookup for %s: %v", email, err)
	}
	return id
}

// giveTheCachedRowToAnotherReader reassigns the acting reader's just-written
// cache row to a second reader and marks it with a band this company could not
// produce, leaving the acting reader with none of their own.
//
// Moving the REAL row rather than writing one is what makes this a probe. The
// cache gate checks the fingerprint before it checks anything about the reader,
// so an invented fingerprint is rejected for a reason that has nothing to do
// with keying — and the test would pass over an unkeyed read. This row carries
// the fingerprint the next read will compute, so it is live: an unkeyed read
// finds it, accepts it, and serves it.
// giveTheCachedRowToAnotherReader returns the reader it handed the row to, so
// the caller can assert that reader's row is still there afterwards. Without
// the id there is nothing to look for, and "the acting reader has a row" is
// satisfied by a write that replaced the other one.
func giveTheCachedRowToAnotherReader(t *testing.T, e *apptest.AppEnv, companyID string, acting ids.UUID) ids.UUID {
	t.Helper()
	other := ids.NewV7()
	if _, err := e.Owner.Exec(context.Background(),
		`INSERT INTO app_user (id, email, display_name) VALUES ($1, 'grace@example.com', 'Grace')`, other); err != nil {
		t.Fatalf("create the other reader: %v", err)
	}
	tag, err := e.Owner.Exec(context.Background(),
		`UPDATE company_growth_fit
		    SET user_id = $1,
		        payload = jsonb_set(payload, '{band}', to_jsonb($2::text))
		  WHERE company_id = $3 AND user_id = $4`,
		other, otherReadersBand, companyID, acting)
	if err != nil {
		t.Fatalf("hand the cached row to the other reader: %v", err)
	}
	if tag.RowsAffected() != 1 {
		t.Fatalf("moved %d cache rows, want exactly the one the first read wrote", tag.RowsAffected())
	}
	return other
}
