// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// An address answers the company the plant would employ it at, on its own domain or
// the narrowest parent. Consumer mail, a refused domain and a hidden company never answer.
func TestAnAddressAnswersTheCompanyOnItsDomain(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	parent := e.createNamedCompany(ctx, t, "Addrco", "addrco.test")
	regional := e.createNamedCompany(ctx, t, "Addrco EU", "eu.addrco.test")
	e.createNamedCompany(ctx, t, "Mailbox Co", "gmail.com")
	e.createNamedCompany(ctx, t, "Refusedco", "refusedco.test")
	if _, err := e.store.SetDomainAdmission(ctx, "refusedco.test", DomainSuppressed, "a tool we use, not a customer"); err != nil {
		t.Fatalf("refusing refusedco.test a company: %v", err)
	}
	hidden := e.createNamedCompany(e.asOther(), t, "Hiddenco", "hiddenco.test")
	if _, err := e.store.UpdateCompany(e.asOther(), hidden, UpdateCompanyInput{Visibility: new("owner")}); err != nil {
		t.Fatalf("making hiddenco private: %v", err)
	}

	cases := []struct {
		email string
		want  ids.CompanyID
	}{
		{"dana@addrco.test", parent},
		{"Dana@US.Addrco.test", parent},
		{"eve@eu.addrco.test", regional},
		{"ed@team.eu.addrco.test", regional},
		{"carol@gmail.com", ids.CompanyID{}},
		{"rita@refusedco.test", ids.CompanyID{}},
		{"hal@hiddenco.test", ids.CompanyID{}},
		{"lou@notaddrco.test", ids.CompanyID{}},
		{"dana@addrco", ids.CompanyID{}},
		{"dana@", ids.CompanyID{}},
	}
	for _, c := range cases {
		t.Run(c.email, func(t *testing.T) {
			got, found, err := e.store.CompanyForAddress(ctx, c.email)
			if err != nil {
				t.Fatalf("asking for %s: %v", c.email, err)
			}
			if c.want.IsZero() {
				if found {
					t.Fatalf("%s answered %s (%s), want no company", c.email, got.CompanyName, got.CompanyId)
				}
				return
			}
			if !found || ids.UUID(got.CompanyId) != c.want.UUID {
				t.Fatalf("%s answered %+v found=%v, want %s", c.email, got, found, c.want)
			}
		})
	}
}
