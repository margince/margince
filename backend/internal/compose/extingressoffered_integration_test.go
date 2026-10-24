// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The value a feed offered for a company field is kept beside the value on the
// record, whether or not the record took it.

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/pkg/extension"
)

func TestAFeedValueIsKeptBesideTheTypedOneAndReplacedByTheNextOffer(t *testing.T) {
	e := setupCRMIngress(t)
	first := e.companyEditedByAUser(t)
	id := ids.From[ids.CompanyKind](ids.MustParse(first.Ref.ID))

	e.mustIngest(t, companyRecord("acct-1", extension.CompanyFields{DisplayName: "Acme", Industry: "Feed value"}))
	e.mustIngest(t, companyRecord("acct-1", extension.CompanyFields{DisplayName: "Acme", Industry: "Feed again"}))

	offered, err := e.Contacts.ListOfferedValues(e.Admin(), id)
	if err != nil {
		t.Fatalf("listing offered values: %v", err)
	}
	if len(offered) != 1 || offered[0].Field != "industry" || offered[0].Value != "Feed again" || offered[0].SourceSystem == "" {
		t.Errorf("offered = %+v, want the one latest industry offer", offered)
	}
	if industry, _, _, _ := e.companyColumns(t, first.Ref.ID); industry == nil || *industry != "Hand typed" {
		t.Errorf("industry = %v, want the typed value beside the offer", industry)
	}
}

func TestAnArchivedCompanyServesNoOfferedValues(t *testing.T) {
	e := setupCRMIngress(t)
	made := e.mustIngest(t, companyRecord("acct-1", extension.CompanyFields{DisplayName: "Acme", Industry: "Feed value"}))
	id := ids.From[ids.CompanyKind](ids.MustParse(made.Ref.ID))
	if _, err := e.Contacts.ArchiveCompany(e.Admin(), id, nil); err != nil {
		t.Fatalf("archiving: %v", err)
	}
	if _, err := e.Contacts.ListOfferedValues(e.Admin(), id); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("listing for an archived company: %v, want not found", err)
	}
}

func TestErasingAnEmployeeDeletesTheOfferedValuesOfTheirCompany(t *testing.T) {
	e := setupCRMIngress(t)
	made := e.mustIngest(t, companyRecord("acct-1", extension.CompanyFields{DisplayName: "Acme", Description: "run by Erika"}))
	other := e.mustIngest(t, companyRecord("acct-2", extension.CompanyFields{DisplayName: "Other", Description: "unrelated"}))
	contact := e.SeedContact(t, "Erika Mustermann", nil)
	e.WsExec(t, `INSERT INTO relationship (kind,contact_id,company_id,source,captured_by) VALUES ('employment',$1,$2,'manual','human:x')`,
		contact, ids.MustParse(made.Ref.ID))

	if err := privacy.NewEraser(InstallationDB(e.Pool)).EraseContact(e.Admin(), contact, "subject request"); err != nil {
		t.Fatalf("erasing: %v", err)
	}
	left := e.countAsWorkspace(t, `SELECT count(*) FROM company_offered_value WHERE company_id = '`+made.Ref.ID+`'`)
	kept := e.countAsWorkspace(t, `SELECT count(*) FROM company_offered_value WHERE company_id = '`+other.Ref.ID+`'`)
	if left != 0 || kept != 1 {
		t.Errorf("offered rows after erasure: employer %d, other company %d, want 0 and 1", left, kept)
	}
}

func TestOfferedValuesGoWithTheCompanyTheyDescribe(t *testing.T) {
	e := setupCRMIngress(t)
	made := e.mustIngest(t, companyRecord("acct-1", extension.CompanyFields{DisplayName: "Acme", Industry: "Feed value"}))
	count := `SELECT count(*) FROM company_offered_value`
	if got := e.countAsWorkspace(t, count); got != 1 {
		t.Fatalf("offered rows = %d, want 1", got)
	}
	e.readAsWorkspace(t, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `DELETE FROM company WHERE id = $1`, ids.MustParse(made.Ref.ID))
		return err
	})
	if got := e.countAsWorkspace(t, count); got != 0 {
		t.Errorf("offered rows after the company is deleted = %d, want 0", got)
	}
}
