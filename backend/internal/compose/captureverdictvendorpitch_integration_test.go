// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
)

// markOf reads the relationship mark and its setter for the contact holding an
// address. found is false when no contact holds it.
func markOf(t *testing.T, e *integration.Env, email string) (mark, setBy string, found bool) {
	t.Helper()
	var m, by *string
	var rows int
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		err := tx.QueryRow(context.Background(), `
			SELECT p.relationship_mark, p.relationship_mark_set_by
			  FROM contact p JOIN contact_email pe ON pe.contact_id = p.id
			 WHERE pe.email = $1`, email).Scan(&m, &by)
		if err == pgx.ErrNoRows {
			return nil
		}
		rows++
		return err
	}); err != nil {
		t.Fatalf("reading the mark of %s: %v", email, err)
	}
	if rows == 0 {
		return "", "", false
	}
	if m != nil {
		mark = *m
	}
	if by != nil {
		setBy = *by
	}
	return mark, setBy, true
}

func listedAddresses(t *testing.T, e *integration.Env, in contacts.ListContactsInput) map[string]bool {
	t.Helper()
	rows, _, err := e.Contacts.ListContacts(e.Admin(), in)
	if err != nil {
		t.Fatalf("listing contacts: %v", err)
	}
	out := map[string]bool{}
	for _, row := range rows {
		if row.Emails == nil {
			continue
		}
		for _, mail := range *row.Emails {
			out[string(mail.Email)] = true
		}
	}
	return out
}

func TestAVendorPitchFromAStrangerBecomesAMarkedContactTheDefaultListsLeaveOut(t *testing.T) {
	e := integration.Setup(t)
	const vendor, prospect = "dana.keller@seo-growth.example", "buyer@realco.example"

	vendorMail := seedCapturedMail(t, e, vendor, "Free SEO audit")
	vendorRow := seedPendingDisposition(t, e, vendor, "seo-growth.example", vendorMail)
	prospectMail := seedCapturedMail(t, e, prospect, "Interested in your product")
	prospectRow := seedPendingDisposition(t, e, prospect, "realco.example", prospectMail)

	runVerdict(t, e, &scriptedVerdictBrain{verdicts: map[string]string{
		vendorRow.String():   capture.KindVendorPitch,
		prospectRow.String(): capture.KindContact,
	}})

	mark, setBy, found := markOf(t, e, vendor)
	if !found {
		t.Fatal("a vendor_pitch verdict created no contact, so the mail has nobody to hang on")
	}
	if mark != "vendor_marketing" || setBy != verdictActor {
		t.Errorf("vendor contact mark = %q set by %q, want vendor_marketing set by %q", mark, setBy, verdictActor)
	}
	if mark, _, _ := markOf(t, e, prospect); mark != "" {
		t.Errorf("an ordinary contact verdict marked the contact %q", mark)
	}
	if n := countIn(t, e, `SELECT count(*) FROM activity WHERE id = $1 AND archived_at IS NULL`, vendorMail); n != 1 {
		t.Error("a vendor_pitch verdict hid the message; the mail must stay visible")
	}

	defaults := listedAddresses(t, e, contacts.ListContactsInput{})
	if defaults[vendor] || !defaults[prospect] {
		t.Errorf("default list = %v, want the prospect and not the vendor", defaults)
	}
	query := vendor
	if found := listedAddresses(t, e, contacts.ListContactsInput{Query: &query}); !found[vendor] {
		t.Error("a text search did not find the marked contact")
	}
	only, anyMark := "vendor_marketing", "any"
	if got := listedAddresses(t, e, contacts.ListContactsInput{RelationshipMark: &only}); !got[vendor] || got[prospect] {
		t.Errorf("relationship_mark=vendor_marketing listed %v, want only the vendor", got)
	}
	if got := listedAddresses(t, e, contacts.ListContactsInput{RelationshipMark: &anyMark}); !got[vendor] || !got[prospect] {
		t.Errorf("relationship_mark=any listed %v, want both", got)
	}
}

func TestAVendorPitchFromAnAddressTheWorkspaceWroteToIsAnOrdinaryContact(t *testing.T) {
	e := integration.Setup(t)
	const sender = "partner@supplier.example"

	seedAttestedOutbound(t, e, "vp-out-1", sender, "vp-t1")
	mail := seedCapturedMail(t, e, sender, "Our new offer")
	row := seedPendingDisposition(t, e, sender, "supplier.example", mail)

	runVerdict(t, e, &scriptedVerdictBrain{verdicts: map[string]string{row.String(): capture.KindVendorPitch}})

	mark, _, found := markOf(t, e, sender)
	if !found {
		t.Fatal("the fallback to an ordinary contact created no contact")
	}
	if mark != "" {
		t.Errorf("an address the workspace wrote to was marked %q; the model's answer alone must never mark", mark)
	}
	if got := listedAddresses(t, e, contacts.ListContactsInput{}); !got[sender] {
		t.Error("the corresponded contact is missing from the default list")
	}
}

func TestAVendorPitchNeverMarksAContactThatAlreadyExists(t *testing.T) {
	e := integration.Setup(t)
	const sender = "known@vendorish.example"

	mail := seedCapturedMail(t, e, sender, "Quick question")
	row := seedPendingDisposition(t, e, sender, "vendorish.example", mail)
	runVerdict(t, e, &scriptedVerdictBrain{verdicts: map[string]string{row.String(): capture.KindContact}})

	again := seedCapturedMail(t, e, sender, "Our offer")
	row2 := seedPendingDisposition(t, e, sender, "vendorish.example", again)
	runVerdict(t, e, &scriptedVerdictBrain{verdicts: map[string]string{row2.String(): capture.KindVendorPitch}})

	if mark, _, _ := markOf(t, e, sender); mark != "" {
		t.Errorf("a verdict marked a contact that already existed (%q); the decision that made it stands", mark)
	}
}
