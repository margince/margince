// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package people

// The VAT-check store's statements, run against a real database.
//
// A unit suite cannot see any of what matters here: the CHECK constraints that
// refuse a receipt for a lookup nobody made, the unique index that makes a
// re-check replace rather than accumulate, or the join that decides whether a
// number is worth consulting. Each of those is enforced by the schema and by
// nothing in Go, so a statement naming a column that is not there would pass
// every unit test and fail on the first real company.

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// consultedAt is fixed rather than time.Now(): a stored timestamp read back and
// compared is an assertion about the write, and a clock in the middle of it
// makes the test's own passing depend on how fast the database answered.
var consultedAt = time.Date(2026, 8, 20, 9, 30, 0, 0, time.UTC)

func TestEveryVatCheckStatementRunsAgainstTheRealSchema(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()

	org, err := e.store.CreateOrganization(ctx, CreateOrganizationInput{
		DisplayName: "Umsatzsteuer GmbH", Source: "manual",
	})
	if err != nil {
		t.Fatalf("seeding the organization to check: %v", err)
	}
	orgID := ids.From[ids.OrganizationKind](ids.UUID(org.Id))

	// 1. A company that has never been consulted says so, distinctly from one
	//    whose number came back invalid.
	if _, err := e.store.VatCheckFor(ctx, orgID); err == nil {
		t.Fatal("an unconsulted company answered a VAT check, so an absence of evidence reads as evidence")
	}

	// 2. The first check. Nothing is replaced, so the audit image has no before
	//    half — the path that would fail if the write demanded one.
	first := VatCheck{
		OrganizationID:     orgID,
		Number:             "DE123456789",
		Status:             VatCheckValid,
		ConsultationNumber: "WAPIAAAAXk3-stand-in",
		RegisteredName:     "Umsatzsteuer GmbH",
		RegisteredAddress:  "Musterstr. 1, Berlin",
		CheckedAt:          consultedAt,
	}
	if err := e.store.RecordVatCheck(ctx, first); err != nil {
		t.Fatalf("RecordVatCheck on a first consultation: %v", err)
	}

	stored, err := e.store.VatCheckFor(ctx, orgID)
	if err != nil {
		t.Fatalf("VatCheckFor after a consultation: %v", err)
	}
	if stored.ConsultationNumber != first.ConsultationNumber {
		t.Errorf("the receipt read back as %q, want %q — it is the evidence, and losing it in the round trip leaves a verdict that proves nothing",
			stored.ConsultationNumber, first.ConsultationNumber)
	}
	if stored.Status != VatCheckValid || stored.Number != first.Number {
		t.Errorf("read back %q/%q, want %q/%q", stored.Status, stored.Number, first.Status, first.Number)
	}
	if !stored.CheckedAt.Equal(consultedAt) {
		t.Errorf("checked_at read back as %v, want %v — the date is half of what a receipt proves", stored.CheckedAt, consultedAt)
	}

	// 3. A RE-CHECK replaces rather than accumulates, and this is the statement
	//    the unique index decides. A second row here would leave two answers
	//    about one company and no rule for which is current.
	recheck := first
	recheck.Status = VatCheckInvalid
	recheck.ConsultationNumber = "WAPIAAAAXk4-stand-in"
	recheck.CheckedAt = consultedAt.Add(24 * time.Hour)
	if err := e.store.RecordVatCheck(ctx, recheck); err != nil {
		t.Fatalf("RecordVatCheck on a re-consultation: %v", err)
	}
	after, err := e.store.VatCheckFor(ctx, orgID)
	if err != nil {
		t.Fatalf("VatCheckFor after a re-consultation: %v", err)
	}
	if after.Status != VatCheckInvalid || after.ConsultationNumber != recheck.ConsultationNumber {
		t.Errorf("the re-check read back as %q/%q, want %q/%q — a number that went bad is the finding this lane exists for",
			after.Status, after.ConsultationNumber, recheck.Status, recheck.ConsultationNumber)
	}
}

// The schema refuses a receipt beside a lookup that did not happen, and the
// store refuses it first with a sentence. Both matter: one is the guarantee,
// the other is what a caller reads.
func TestAReceiptCannotBeStoredForALookupNobodyMade(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()

	org, err := e.store.CreateOrganization(ctx, CreateOrganizationInput{
		DisplayName: "Unreachable Register GmbH", Source: "manual",
	})
	if err != nil {
		t.Fatalf("seeding the organization: %v", err)
	}
	orgID := ids.From[ids.OrganizationKind](ids.UUID(org.Id))

	err = e.store.RecordVatCheck(ctx, VatCheck{
		OrganizationID:     orgID,
		Number:             "DE123456789",
		Status:             VatCheckUnavailable,
		ConsultationNumber: "WAPIAAAAXk5-stand-in",
		CheckedAt:          consultedAt,
	})
	if err == nil {
		t.Fatal("a consultation number was stored beside an unavailable register, so a receipt exists for a check nobody made")
	}

	// The register being unreachable is itself worth recording — without the
	// receipt.
	if err := e.store.RecordVatCheck(ctx, VatCheck{
		OrganizationID: orgID,
		Number:         "DE123456789",
		Status:         VatCheckUnavailable,
		CheckedAt:      consultedAt,
	}); err != nil {
		t.Fatalf("recording an unavailable register: %v", err)
	}
}

// A number that has already been consulted is not worth consulting again; a
// CORRECTED one is. The join answering that is the whole trigger for the lane,
// and getting it backwards either re-asks forever or never asks at all.
func TestOnlyAnUncheckedNumberIsWorthConsulting(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()

	bare, err := e.store.CreateOrganization(ctx, CreateOrganizationInput{
		DisplayName: "Steuernummer AG", Source: "manual",
	})
	if err != nil {
		t.Fatalf("seeding the organization: %v", err)
	}
	// A company stating no number has nothing to ask about.
	if _, worth, err := e.store.VatNumberForCheck(ctx, ids.From[ids.OrganizationKind](ids.UUID(bare.Id))); err != nil {
		t.Fatalf("VatNumberForCheck on a company with no number: %v", err)
	} else if worth {
		t.Error("a company that states no VAT number is worth consulting, so the lane would ask the register about nothing")
	}

	// Seeded through the REAL writer — the cold-start apply a site read lands
	// through — because a hand-inserted row would prove nothing about the path
	// production takes.
	const stated = "DE123456789"
	orgID, err := e.store.ApplyColdStartProfile(ctx, ApplyColdStartProfileInput{
		SourceURL: "https://steuernummer.example/impressum",
		Fields: []ColdStartFieldInput{{
			Field: fieldRegisterVat, Value: stated,
			EvidenceSnippet: "USt-IdNr: " + stated,
			SourceURL:       "https://steuernummer.example/impressum",
			Confidence:      0.9,
		}},
	})
	if err != nil {
		t.Fatalf("applying the read-back VAT number: %v", err)
	}
	number, worth, err := e.store.VatNumberForCheck(ctx, orgID)
	if err != nil {
		t.Fatalf("VatNumberForCheck on a stated number: %v", err)
	}
	if !worth || number != stated {
		t.Fatalf("a stated, unconsulted number answered %q/%v, want %q/true", number, worth, stated)
	}

	if err := e.store.RecordVatCheck(ctx, VatCheck{
		OrganizationID: orgID, Number: stated, Status: VatCheckValid, CheckedAt: consultedAt,
	}); err != nil {
		t.Fatalf("recording the consultation: %v", err)
	}
	if _, worth, err := e.store.VatNumberForCheck(ctx, orgID); err != nil {
		t.Fatalf("VatNumberForCheck after a consultation: %v", err)
	} else if worth {
		t.Error("an already-consulted number is still worth consulting, so the lane would re-ask the register forever")
	}

	// A CORRECTION has never been checked, whatever the stored receipt says
	// about the number it replaced.
	const corrected = "DE987654321"
	if _, err := e.store.UpdateOrganizationProfileField(ctx, orgID, fieldRegisterVat,
		ProfileFieldWriteInput{Value: strPtr(corrected)}); err != nil {
		t.Fatalf("correcting the VAT number: %v", err)
	}
	number, worth, err = e.store.VatNumberForCheck(ctx, orgID)
	if err != nil {
		t.Fatalf("VatNumberForCheck after a correction: %v", err)
	}
	if !worth || number != corrected {
		t.Errorf("a corrected number answered %q/%v, want %q/true — the stored receipt names the number it was issued for, so a new one is unchecked",
			number, worth, corrected)
	}
}

// A register that declined is not a verdict, so it must not silence the number
// for good. Recording `unavailable` and then treating it like an answer is how
// one transient 502 costs a company its VAT check forever.
func TestAnUnansweredConsultationIsAskedAgain(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()

	const stated = "DE123456789"
	orgID, err := e.store.ApplyColdStartProfile(ctx, ApplyColdStartProfileInput{
		SourceURL: "https://transient.example/impressum",
		Fields: []ColdStartFieldInput{{
			Field: fieldRegisterVat, Value: stated,
			EvidenceSnippet: "USt-IdNr: " + stated,
			SourceURL:       "https://transient.example/impressum",
			Confidence:      0.9,
		}},
	})
	if err != nil {
		t.Fatalf("applying the read-back VAT number: %v", err)
	}

	if err := e.store.RecordVatCheck(ctx, VatCheck{
		OrganizationID: orgID, Number: stated,
		Status: VatCheckUnavailable, CheckedAt: consultedAt,
	}); err != nil {
		t.Fatalf("recording an unavailable register: %v", err)
	}

	number, worth, err := e.store.VatNumberForCheck(ctx, orgID)
	if err != nil {
		t.Fatalf("VatNumberForCheck after an unavailable register: %v", err)
	}
	if !worth || number != stated {
		t.Errorf("after an unanswered consultation the number answered %q/%v, want %q/true — a register that declined said nothing about this company",
			number, worth, stated)
	}
}

// The same number printed two ways is one number. An extracted field carrying
// surrounding whitespace must not consult as a different company from the one
// already checked, or every enqueue spends another consultation forever.
func TestWhitespaceDoesNotMakeANumberLookUnchecked(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()

	const spaced = " DE123456789 "
	orgID, err := e.store.ApplyColdStartProfile(ctx, ApplyColdStartProfileInput{
		SourceURL: "https://spaced.example/impressum",
		Fields: []ColdStartFieldInput{{
			Field: fieldRegisterVat, Value: spaced,
			EvidenceSnippet: "USt-IdNr:" + spaced,
			SourceURL:       "https://spaced.example/impressum",
			Confidence:      0.9,
		}},
	})
	if err != nil {
		t.Fatalf("applying the read-back VAT number: %v", err)
	}

	// The store consults, and stores, the trimmed number — which is what the
	// register was actually asked about.
	if err := e.store.RecordVatCheck(ctx, VatCheck{
		OrganizationID: orgID, Number: spaced,
		Status: VatCheckValid, CheckedAt: consultedAt,
	}); err != nil {
		t.Fatalf("recording the consultation: %v", err)
	}
	if _, worth, err := e.store.VatNumberForCheck(ctx, orgID); err != nil {
		t.Fatalf("VatNumberForCheck on a spaced number: %v", err)
	} else if worth {
		t.Error("a spaced field reads as unchecked against the trimmed number it was consulted under, so every enqueue would consult again")
	}
}
