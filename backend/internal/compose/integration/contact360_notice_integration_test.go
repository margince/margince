// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestContactPrivacyWorkUsesTheSameVisibleAgendaAsHome(t *testing.T) {
	e := Setup(t)
	contact := e.SeedContact(t, "Referred Contact", &e.Rep1)
	other := e.SeedContact(t, "Other Contact", &e.Rep1)
	for _, id := range []ids.UUID{contact, other} {
		if err := e.DB().Tx(e.Admin(), func(tx pgx.Tx) error {
			var acquisition ids.UUID
			if err := tx.QueryRow(e.Admin(), `INSERT INTO contact_acquisition_evidence (contact_id, kind, occurred_at, captured_by) VALUES (@contact, 'referral', @at, 'human:seed') RETURNING id`, pgx.NamedArgs{"contact": id, "at": roomFixedNow}).Scan(&acquisition); err != nil {
				return err
			}
			return consent.OpenNoticeCaseTx(e.Admin(), tx, consent.NoticeCaseInput{ContactID: ids.From[ids.ContactKind](id), AcquisitionID: acquisition, Rule: consent.RuleArt14, DueAt: roomFixedNow, State: consent.NoticeOpen})
		}); err != nil {
			t.Fatal(err)
		}
	}
	svc := contactRoomService(e)
	page, err := svc.Assemble(e.Admin(), ids.From[ids.ContactKind](contact))
	if err != nil {
		t.Fatal(err)
	}
	if page.PrivacyNoticeDuties == nil || len(*page.PrivacyNoticeDuties) != 1 || page.Moment == nil || page.Moment.Rule != crmcontracts.ContactMomentRulePrivacyNoticeDue {
		t.Fatalf("open privacy work is absent from contact: %+v", page)
	}
	// Contact authority alone must not expose the compliance case.
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, AccountRepPerms)
	hidden, err := svc.Assemble(rep, ids.From[ids.ContactKind](contact))
	if err != nil {
		t.Fatal(err)
	}
	if hidden.PrivacyNoticeDuties != nil || !slices.Contains(hidden.SectionsOmitted, crmcontracts.Contact360SectionsOmittedContact360SectionsOmittedPrivacyNoticeDuties) {
		t.Fatal("privacy work leaked without privacy-request authority")
	}
	caseID := ids.UUID((*page.PrivacyNoticeDuties)[0].Id)
	if _, err := consent.NewStore(e.DB()).ExcuseNoticeCase(e.Admin(), caseID, consent.ExcuseInput{State: consent.NoticeExemptWithReason, Note: "Already disclosed", Now: roomFixedNow}); err != nil {
		t.Fatal(err)
	}

	resolved, err := svc.Assemble(e.Admin(), ids.From[ids.ContactKind](contact))
	if err != nil {
		t.Fatal(err)
	}
	if resolved.PrivacyNoticeDuties == nil || len(*resolved.PrivacyNoticeDuties) != 0 || resolved.Moment.Rule == crmcontracts.ContactMomentRulePrivacyNoticeDue {
		t.Fatal("closed privacy work remains on the contact")
	}
}

func TestContactSourceReviewRequiresPrivacyAndContactVisibility(t *testing.T) {
	e := Setup(t)
	contact := e.SeedContact(t, "Unconfirmed Source Example", &e.Rep1)
	if err := e.DB().Tx(e.Admin(), func(tx pgx.Tx) error {
		var acquisition ids.UUID
		if err := tx.QueryRow(e.Admin(), `INSERT INTO contact_acquisition_evidence (contact_id,kind,captured_by) VALUES (@contact,'unknown_legacy','human:fixture') RETURNING id`, pgx.NamedArgs{"contact": contact}).Scan(&acquisition); err != nil {
			return err
		}
		return consent.OpenNoticeCaseTx(e.Admin(), tx, consent.NoticeCaseInput{ContactID: ids.From[ids.ContactKind](contact), AcquisitionID: acquisition, Rule: consent.RuleArt14, DueAt: roomFixedNow})
	}); err != nil {
		t.Fatal(err)
	}
	svc := contactRoomService(e)
	page, err := svc.Assemble(e.Admin(), ids.From[ids.ContactKind](contact))
	if err != nil || page.PrivacySourceReviews == nil || len(*page.PrivacySourceReviews) != 1 {
		t.Fatalf("source review missing: %+v err=%v", page, err)
	}
	if page.Moment.Rule == crmcontracts.ContactMomentRulePrivacyNoticeDue || len(*page.PrivacyNoticeDuties) != 0 {
		t.Fatal("unconfirmed source claimed a legal duty")
	}
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, AccountRepPerms)
	hidden, err := svc.Assemble(rep, ids.From[ids.ContactKind](contact))
	if err != nil || hidden.PrivacySourceReviews != nil {
		t.Fatalf("review leaked without privacy authority: %+v err=%v", hidden, err)
	}
	e.MakeCapturePrivate(t, "contact", contact, e.Rep1)
	other := e.As(e.Rep3, []ids.UUID{e.Team2}, AccountRepPerms)
	if _, err := svc.Assemble(other, ids.From[ids.ContactKind](contact)); err == nil {
		t.Fatal("contact outside row scope was exposed")
	}
}
