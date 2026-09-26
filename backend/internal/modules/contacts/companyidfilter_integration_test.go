// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

import (
	"errors"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestTheCompanyListNarrowsToTheIDsNamedAndHidesWhatTheCallerCannotSee(t *testing.T) {
	e := setupDedupe(t)
	owner := e.as()
	named := seedLogoCompany(owner, t, e, "Voltaq Systems GmbH", "voltaq.test")
	private := seedLogoCompany(owner, t, e, "Fremdfirma GmbH", "fremd.test")
	unnamed := seedLogoCompany(owner, t, e, "Andere AG", "andere.test")
	if err := e.store.tx(owner, func(tx pgx.Tx) error {
		_, err := tx.Exec(owner, `UPDATE company SET owner_id = $2, visibility = 'owner' WHERE id = $1`, private, e.rep)
		return err
	}); err != nil {
		t.Fatalf("make the company capture-private: %v", err)
	}

	stranger := e.asOwnScoped(ids.NewV7())
	listed, _, err := e.store.ListCompanies(stranger, ListCompaniesInput{IDs: []ids.UUID{named.UUID, private.UUID}})
	if err != nil {
		t.Fatalf("ListCompanies(id): %v", err)
	}
	if len(listed) != 1 || listed[0].Id != openapiUUID(named) {
		t.Fatalf("listed %d companies, want only %s: the private one hidden, %s not named", len(listed), named, unnamed)
	}
}

func TestTheCompanyListRefusesMoreIDsThanOneBoardNames(t *testing.T) {
	e := setupDedupe(t)
	tooMany := make([]ids.UUID, maxCompanyIDFilter+1)
	for i := range tooMany {
		tooMany[i] = ids.NewV7()
	}
	_, _, err := e.store.ListCompanies(e.as(), ListCompaniesInput{IDs: tooMany})
	var refusal *httperr.DetailedError
	if !errors.As(err, &refusal) || refusal.Status != http.StatusUnprocessableEntity {
		t.Fatalf("ListCompanies with %d ids = %v, want a validation error", len(tooMany), err)
	}
}
