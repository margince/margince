// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/platform/owndomain"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func (e *SearchEnv) seedCompanyOnFoldedDomain(t *testing.T, name, domain string) ids.UUID {
	t.Helper()
	companyID := e.SeedID(t,
		`INSERT INTO company (id, display_name, owner_id, source, captured_by)
		 VALUES ($1, $2, $3, 'manual', 'human:x')`, name, e.Rep1)
	if _, err := e.Owner.Exec(context.Background(),
		`INSERT INTO company_domain (id, company_id, domain, source, captured_by, domain_folded)
		 VALUES ($1, $2, $3, 'manual', 'human:x', $4)`, ids.NewV7(), companyID, domain, owndomain.StoredOwn(domain)); err != nil {
		t.Fatal(err)
	}
	return companyID
}

// A signal naming a domain in Unicode, as a URL, an address or a bare host,
// reaches the company stored under the punycode form.
func TestASignalInEitherSpellingResolvesToTheCompanyOnThatDomain(t *testing.T) {
	t.Parallel()
	e := SetupSearch(t)
	store := signalStore(e)
	company := e.seedCompanyOnFoldedDomain(t, "Bücher Verlag", "xn--bcher-kva.example")
	admin := e.adminSignals()
	for _, ref := range []string{
		"web:https://www.Bücher.example/preise",
		"inbound:anna@bücher.example",
		"bücher.example",
		"inbound:anna@xn--bcher-kva.example",
	} {
		resolved, err := store.Resolve(admin, createRaw(t, store, admin, ref))
		if err != nil {
			t.Fatalf("resolving %q: %v", ref, err)
		}
		if resolved.ResolvedCompanyId == nil || ids.UUID(*resolved.ResolvedCompanyId) != company {
			t.Errorf("%q resolved to %v (%s), want %s", ref, resolved.ResolvedCompanyId, resolved.ResolutionState, company)
		}
	}
}

// Two companies holding one folded domain have no single owner, so the
// signal is left for a human instead of picking one by id.
func TestASignalOnADomainTwoCompaniesHoldIsNotAttributed(t *testing.T) {
	t.Parallel()
	e := SetupSearch(t)
	store := signalStore(e)
	e.seedCompanyOnFoldedDomain(t, "Bücher Verlag", "xn--bcher-kva.example")
	e.seedCompanyOnFoldedDomain(t, "Buecher Handel", "bücher.example")
	admin := e.adminSignals()
	resolved, err := store.Resolve(admin, createRaw(t, store, admin, "inbound:anna@bücher.example"))
	if err != nil {
		t.Fatal(err)
	}
	if resolved.ResolvedCompanyId != nil || string(resolved.ResolutionState) != "low_confidence" {
		t.Errorf("resolved to %v in state %s, want no company and low_confidence", resolved.ResolvedCompanyId, resolved.ResolutionState)
	}
}
