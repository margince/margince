// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// A consumer-mail domain never names a company. A typed claim of one on a
// company's domain list is refused, on create and on edit alike.

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func wantConsumerDomainRefusal(t *testing.T, err error) {
	t.Helper()
	var refusal *httperr.DetailedError
	if !errors.As(err, &refusal) || refusal.Code != "validation_error" ||
		len(refusal.Fields) != 1 || refusal.Fields[0].Code != "consumer_mail_domain" {
		t.Fatalf("want a consumer_mail_domain field refusal, got %v", err)
	}
}

func TestCompanyDomainRefusesConsumerMail(t *testing.T) {
	t.Parallel()
	e := setupDedupe(t)
	ctx := e.as()

	_, err := e.store.CreateCompany(ctx, CreateCompanyInput{
		DisplayName: "Webmail GmbH", Source: "manual",
		Domains: []CompanyDomainInput{{Domain: "gmail.com", IsPrimary: true}},
	})
	wantConsumerDomainRefusal(t, err)

	company, err := e.store.CreateCompany(ctx, CreateCompanyInput{
		DisplayName: "Voltaq Systems GmbH", Source: "manual",
		Domains: []CompanyDomainInput{{Domain: "voltaq.test", IsPrimary: true}},
	})
	if err != nil {
		t.Fatal(err)
	}
	companyID := ids.From[ids.CompanyKind](ids.UUID(company.Id))

	_, err = e.store.UpdateCompany(ctx, companyID, UpdateCompanyInput{
		Domains: &[]CompanyDomainInput{
			{Domain: "voltaq.test", IsPrimary: true},
			{Domain: "https://www.GMAIL.com/", IsPrimary: false},
		},
	})
	wantConsumerDomainRefusal(t, err)
	if live := liveDomainsOf(ctx, t, e, companyID); len(live) != 1 || !live["voltaq.test"] {
		t.Fatalf("a refused edit changed the domains: %+v", live)
	}
}

// Onboarding and the site-read confirmation save the company's website through
// setCompanyDomain, so a consumer-mail website is refused there too.
func TestSavingACompanyWebsiteOnConsumerMailIsRefused(t *testing.T) {
	t.Parallel()
	e := setupDedupe(t)
	website := "https://www.gmail.com"
	_, err := e.store.SaveCompany(e.asAdmin(), SaveCompanyInput{DisplayName: "Our House GmbH", Website: &website})
	wantConsumerDomainRefusal(t, err)

	fine := "https://our-house.test"
	if _, err := e.store.SaveCompany(e.asAdmin(), SaveCompanyInput{DisplayName: "Our House GmbH", Website: &fine}); err != nil {
		t.Fatalf("a real website: %v", err)
	}
}

// A company-domain write queues behind a consumer-list write holding the same
// domain, so one of them sees the other's row.
func TestACompanyDomainWriteWaitsForTheConsumerListWriter(t *testing.T) {
	t.Parallel()
	e := setupDedupe(t)
	ctx := e.as()
	company := e.createNamedCompany(ctx, t, "Raceco", "raceco.test")
	finished := make(chan struct{})
	var updateErr error
	waited := false
	err := e.store.tx(ctx, func(tx pgx.Tx) error {
		if err := storekit.LockConsumerDomain(ctx, tx, "race-provider.example"); err != nil {
			return err
		}
		go func() {
			defer close(finished)
			_, updateErr = e.store.UpdateCompany(ctx, company, UpdateCompanyInput{Domains: &[]CompanyDomainInput{
				{Domain: "raceco.test", IsPrimary: true}, {Domain: "mail.race-provider.example"},
			}})
		}()
		waited = waitsOnAnAdvisoryLock(ctx, t, tx, finished)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	<-finished
	if !waited {
		t.Error("the company-domain write did not wait for the lock a consumer-list write holds")
	}
	if updateErr != nil {
		t.Errorf("the queued write failed once released: %v", updateErr)
	}
}

func waitsOnAnAdvisoryLock(ctx context.Context, t *testing.T, tx pgx.Tx, finished <-chan struct{}) bool {
	t.Helper()
	for range 20000 {
		select {
		case <-finished:
			return false
		default:
		}
		var waiting bool
		if err := tx.QueryRow(ctx, `
			SELECT EXISTS (SELECT 1 FROM pg_locks WHERE locktype = 'advisory' AND NOT granted)`).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			return true
		}
	}
	return false
}
