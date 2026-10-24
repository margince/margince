// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// The industry column through its real writers: create, patch, merge and the
// stored-data repair. Legacy rows are made by the writer first and given text
// in place afterwards, because the writer no longer stores any.

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func companyIndustryCreate(t *testing.T, e *dedupeEnv, name string, industry, description *string) (ids.CompanyID, string, string) {
	t.Helper()
	made, err := e.store.CreateCompany(e.as(), CreateCompanyInput{
		DisplayName: name, Source: "manual", Industry: industry, Description: description,
	})
	if err != nil {
		t.Fatalf("create %s: %v", name, err)
	}
	return ids.From[ids.CompanyKind](ids.UUID(made.Id)), describedAs(made.Description), industryOf(made.Industry)
}

func describedAs(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func industryOf[T ~string](v *T) string {
	if v == nil {
		return ""
	}
	return string(*v)
}

func TestACreateStoresTheCodeAnyMappableSpellingNames(t *testing.T) {
	e := setupDedupe(t)
	for name, text := range map[string]string{"Alpha GmbH": "Healthcare", "Beta GmbH": "health care", "Gamma GmbH": "Gesundheitswesen"} {
		_, description, code := companyIndustryCreate(t, e, name, &text, nil)
		if code != "healthcare" || description != "" {
			t.Errorf("%s stored %q with description %q, want healthcare and no note", text, code, description)
		}
	}
}

func TestACreateWithTextThatNamesNoIndustryStoresNoCodeAndKeepsTheText(t *testing.T) {
	e := setupDedupe(t)
	text := "pumps, valves and seals"
	written := "Industrial fluid handling."
	_, description, code := companyIndustryCreate(t, e, "Delta GmbH", &text, &written)
	if code != "" {
		t.Errorf("stored %q, want no code", code)
	}
	if description != written+" Industry as written: pumps, valves and seals" {
		t.Errorf("description = %q", description)
	}
}

func TestAPatchWithUnmappableTextLeavesTheCodeAndAddsTheNoteOnce(t *testing.T) {
	e := setupDedupe(t)
	known := "software"
	id, _, _ := companyIndustryCreate(t, e, "Epsilon GmbH", &known, nil)
	text := "cloud plumbing"
	for range 2 {
		if _, err := e.store.UpdateCompany(e.as(), id, UpdateCompanyInput{Industry: &text}); err != nil {
			t.Fatalf("patch: %v", err)
		}
	}
	got, err := e.store.GetCompany(e.as(), id, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if industryOf(got.Industry) != "software" {
		t.Errorf("industry = %q, want the standing code", industryOf(got.Industry))
	}
	if n := strings.Count(describedAs(got.Description), "Industry as written: cloud plumbing"); n != 1 {
		t.Errorf("note appears %d times in %q, want once", n, describedAs(got.Description))
	}
	var held bool
	if err := e.store.tx(e.as(), func(tx pgx.Tx) error {
		var err error
		held, err = descriptionHeldByHuman(e.as(), tx, id)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if held {
		t.Error("a source note claimed the description for a contact")
	}
}

func TestAPatchStoresTheCodeForMappableText(t *testing.T) {
	e := setupDedupe(t)
	id, _, _ := companyIndustryCreate(t, e, "Zeta GmbH", nil, nil)
	text := "Marketing And Advertising"
	got, err := e.store.UpdateCompany(e.as(), id, UpdateCompanyInput{Industry: &text})
	if err != nil {
		t.Fatal(err)
	}
	if industryOf(got.Industry) != "marketing_advertising" {
		t.Errorf("industry = %q", industryOf(got.Industry))
	}
}

func TestAMergeGivesTheSurvivorTheCodeOfALegacyText(t *testing.T) {
	e := setupDedupe(t)
	survivor, _, _ := companyIndustryCreate(t, e, "Eta GmbH", nil, nil)
	retired, _, _ := companyIndustryCreate(t, e, "Theta GmbH", nil, nil)
	setLegacyIndustry(t, e, retired, "health care")
	out, err := e.store.MergeCompany(e.as(), retired, survivor, nil)
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	if industryOf(out.Industry) != "healthcare" {
		t.Errorf("survivor industry = %q", industryOf(out.Industry))
	}
}

func setLegacyIndustry(t *testing.T, e *dedupeEnv, id ids.CompanyID, text string) {
	t.Helper()
	ctx := e.as()
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE company SET industry = $2 WHERE id = $1`, id, text)
		return err
	}); err != nil {
		t.Fatalf("giving the company legacy industry text (test setup): %v", err)
	}
}

func remapAll(t *testing.T, e *dedupeEnv, limit int) (scanned, changed int) {
	t.Helper()
	system := principal.SystemActing(e.as(), "system:test")
	after := ids.Nil
	for {
		var page IndustryRemapPage
		if err := e.store.tx(system, func(tx pgx.Tx) error {
			var err error
			page, err = RemapCompanyIndustriesTx(system, tx, after, limit)
			return err
		}); err != nil {
			t.Fatalf("remap: %v", err)
		}
		if len(page.Scanned) == 0 {
			return scanned, changed
		}
		scanned += len(page.Scanned)
		changed += page.Changed
		after = page.Scanned[len(page.Scanned)-1]
	}
}

func TestTheRepairMapsLegacyTextOncePagedAndLeavesHumanWork(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	human := "Written by a contact."
	mapped, _, _ := companyIndustryCreate(t, e, "Iota GmbH", nil, &human)
	unmapped, _, _ := companyIndustryCreate(t, e, "Kappa GmbH", nil, nil)
	coded := "software"
	already, _, _ := companyIndustryCreate(t, e, "Lambda GmbH", &coded, nil)
	setLegacyIndustry(t, e, mapped, "Maschinenbau")
	setLegacyIndustry(t, e, unmapped, "pumps and valves")

	var estimate int64
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		var err error
		estimate, err = CountCompaniesWithLegacyIndustry(ctx, tx, 100)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	scanned, changed := remapAll(t, e, 1)
	if estimate != 2 || scanned != 2 || changed != 2 {
		t.Errorf("estimate %d, scanned %d, changed %d, want 2 each (the coded row is excluded)", estimate, scanned, changed)
	}

	read := func(id ids.CompanyID) (string, string) {
		got, err := e.store.GetCompany(ctx, id, storekit.LiveOnly)
		if err != nil {
			t.Fatal(err)
		}
		return industryOf(got.Industry), describedAs(got.Description)
	}
	if code, description := read(mapped); code != "manufacturing" || description != human {
		t.Errorf("mapped row = %q, %q", code, description)
	}
	if code, description := read(unmapped); code != "" || description != "Industry as written: pumps and valves" {
		t.Errorf("unmapped row = %q, %q", code, description)
	}
	if code, _ := read(already); code != "software" {
		t.Errorf("coded row = %q", code)
	}
	if again, _ := remapAll(t, e, 10); again != 0 {
		t.Errorf("a second run inspected %d rows, want 0", again)
	}
	var audits int
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE entity_type = 'company' AND entity_id = ANY($1)`,
			[]ids.UUID{mapped.UUID, unmapped.UUID}).Scan(&audits)
	}); err != nil {
		t.Fatal(err)
	}
	if audits < 4 {
		t.Errorf("audit rows = %d, want a create and a remap row for each company", audits)
	}
	_ = context.Background
}
