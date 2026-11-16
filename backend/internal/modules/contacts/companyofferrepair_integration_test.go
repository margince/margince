// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

import (
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestSiteOfferLivesOnlyInItsProfileField(t *testing.T) {
	t.Parallel()
	e := setupDedupe(t)
	made, err := e.store.CreateCompany(e.as(), CreateCompanyInput{DisplayName: "Profile Only", Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	id := ids.From[ids.CompanyKind](ids.UUID(made.Id))
	applySiteReadDescription(e.as(), t, e, id, "Sells industrial bearings.")
	got, err := e.store.GetCompany(e.as(), id, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if got.Description != nil {
		t.Errorf("offer duplicated in description: %v", got.Description)
	}
	fields, err := e.store.ListCompanyProfileFields(e.as(), id)
	if err != nil {
		t.Fatal(err)
	}
	if len(fields) != 1 || fields[0].Value != "Sells industrial bearings." {
		t.Errorf("profile fields = %v", fields)
	}
}

func TestOfferDuplicateRepairPreservesHumanEditsAndReplays(t *testing.T) {
	t.Parallel()
	e := setupDedupe(t)
	system := principal.SystemActing(e.as(), "system:test")
	summary := "Supplies industrial bearings."
	var targets []ids.CompanyID
	for _, name := range []string{"Duplicate Offer", "Human Description", "Human Offer"} {
		made, err := e.store.CreateCompany(e.as(), CreateCompanyInput{DisplayName: name, Source: "manual"})
		if err != nil {
			t.Fatal(err)
		}
		id := ids.From[ids.CompanyKind](ids.UUID(made.Id))
		targets = append(targets, id)
		applySiteReadDescription(e.asAgent(), t, e, id, summary)
		if err := e.store.tx(system, func(tx pgx.Tx) error {
			if _, err := tx.Exec(system, `UPDATE company SET description = $2 WHERE id = $1`, id, summary); err != nil {
				return err
			}
			return storekit.StampFields(system, tx, "company", id.UUID, "site_read", "agent:site_read", []storekit.FieldStamp{{Field: "description"}})
		}); err != nil {
			t.Fatal(err)
		}
	}
	changed := summary + " Edited."
	if _, err := e.store.UpdateCompany(e.as(), targets[1], UpdateCompanyInput{Description: &changed}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.store.UpdateCompanyProfileField(e.as(), targets[2], fieldOfferSummary, ProfileFieldWriteInput{Value: &changed}); err != nil {
		t.Fatal(err)
	}
	run := func() int {
		t.Helper()
		count := 0
		after := ids.Nil
		for {
			var page OfferDuplicatePage
			if err := e.store.tx(system, func(tx pgx.Tx) error {
				var err error
				page, err = ClearSiteOfferDuplicatesTx(system, tx, after, 1)
				return err
			}); err != nil {
				t.Fatal(err)
			}
			count += page.Changed
			if len(page.Scanned) == 0 {
				break
			}
			after = page.Scanned[len(page.Scanned)-1]
		}
		return count
	}
	if got := run(); got != 1 {
		t.Fatalf("changed %d companies, want 1", got)
	}
	if got := run(); got != 0 {
		t.Fatalf("replay changed %d companies", got)
	}
	for i, id := range targets {
		got, err := e.store.GetCompany(e.as(), id, storekit.LiveOnly)
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 && got.Description != nil {
			t.Error("duplicate description remains")
		}
		if i > 0 && got.Description == nil {
			t.Error("human-edited record cleared")
		}
	}
}

func TestCompanyOfferSaveKeepsTheDescriptionSeparate(t *testing.T) {
	t.Parallel()
	e := newAnchorEnv(t)
	offer := "Supplies precision bearings."
	if _, err := e.store.SaveCompany(e.ctx, SaveCompanyInput{DisplayName: "Our Company", Fields: map[string]*string{fieldOfferSummary: &offer}}); err != nil {
		t.Fatal(err)
	}
	got, err := e.store.GetCompany(e.ctx, e.anchorID, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if got.Description != nil {
		t.Errorf("offer duplicated into description: %v", got.Description)
	}
	description := "An independently authored company description."
	if _, err := e.store.UpdateCompany(e.ctx, e.anchorID, UpdateCompanyInput{Description: &description}); err != nil {
		t.Fatal(err)
	}
	offer = "Supplies bearings and drive components."
	if _, err := e.store.SaveCompany(e.ctx, SaveCompanyInput{DisplayName: "Our Company", Fields: map[string]*string{fieldOfferSummary: &offer}}); err != nil {
		t.Fatal(err)
	}
	got, err = e.store.GetCompany(e.ctx, e.anchorID, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if got.Description == nil || *got.Description != description {
		t.Errorf("offer edit changed independent description: %v", got.Description)
	}
	fields, err := e.store.ListCompanyProfileFields(e.ctx, e.anchorID)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range fields {
		if string(field.Field) == fieldOfferSummary {
			if field.Value != offer {
				t.Errorf("offer = %q, want %q", field.Value, offer)
			}
			return
		}
	}
	t.Fatal("saved offer is absent from the profile")
}
