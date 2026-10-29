// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func replacedSignaturePhone(t *testing.T, e *integration.Env) (ids.UUID, ids.UUID) {
	t.Helper()
	contact := seedEnrichContact(t, e, "changed@recurrence.example", "Bob Contact\n"+germany)
	first := onlyMailOf(t, e, contact)
	applySignature(t, e, contact, first, phoneField(germany))
	seedLaterMail(t, e, contact, "changed@recurrence.example", "Bob Contact\n"+germanyNew, "1 hour")
	second := otherMailOf(t, e, contact, first)
	applySignature(t, e, contact, second, phoneField(germanyNew))
	return contact, second
}

func narrowSignatureMail(t *testing.T, e *integration.Env, mail ids.UUID) {
	t.Helper()
	e.WsExec(t, `UPDATE activity SET audience = 'participants' WHERE id = @id`, pgx.StrictNamedArgs{"id": mail})
	if err := NewAudienceRescopeGen(e.Pool).HandleEvent(context.Background(), narrowedEvent(t, mail)); err != nil {
		t.Fatal(err)
	}
}

func restorePhoneContact(t *testing.T, e *integration.Env, contact ids.UUID) {
	t.Helper()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := e.Contacts.RestoreContactTx(e.Admin(), tx, ids.From[ids.ContactKind](contact), nil, unarchiveWith(nil))
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

func TestANarrowingDoesNotReviveAnUnsupportedSignaturePredecessor(t *testing.T) {
	e := integration.Setup(t)
	contact, mail := replacedSignaturePhone(t, e)
	narrowSignatureMail(t, e, mail)
	if got := liveNumbers(t, e, contact); len(got) != 0 {
		t.Fatalf("narrowing revived an unsupported predecessor: %v", got)
	}
}

func TestAnUndoAfterThePredecessorWasReaddedKeepsTheReplacementEvidence(t *testing.T) {
	e := integration.Setup(t)
	contact, _ := replacedSignaturePhone(t, e)
	id := ids.From[ids.ContactKind](contact)
	if _, err := e.Contacts.UpdateContact(e.Admin(), id, contacts.UpdateContactInput{
		Source: "manual", Phones: []contacts.ContactPhoneInput{
			{Phone: "+491715550109", PhoneType: "work", IsPrimary: true},
			{Phone: "+491755550101", PhoneType: "work"},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := e.Contacts.RestoreProfileField(e.Admin(), id, "phone", "+491715550109"); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("undo over a re-added number = %v, want conflict", err)
	}
	if _, kept := numberEvidence(t, e, contact)["+491715550109"]; !kept {
		t.Fatal("refused undo deleted the live replacement's evidence")
	}
	if got := liveNumbers(t, e, contact); !slices.Equal(got, []string{"+491715550109", "+491755550101"}) {
		t.Fatalf("refused undo changed the number list: %v", got)
	}
}

func TestRestoringAContactDoesNotReviveUnsupportedSignatureNumbers(t *testing.T) {
	for _, evidenceRemoved := range []string{"before_archive", "while_archived"} {
		t.Run(evidenceRemoved, func(t *testing.T) {
			e := integration.Setup(t)
			contact := seedEnrichContact(t, e, "archived@recurrence.example", "Bob Contact\n"+germany)
			mail := onlyMailOf(t, e, contact)
			applySignature(t, e, contact, mail, phoneField(germany))
			if evidenceRemoved == "before_archive" {
				e.WsExec(t, `DELETE FROM contact_profile_field WHERE contact_id = @id`, pgx.StrictNamedArgs{"id": contact})
			}
			if _, err := e.Contacts.ArchiveContact(e.Admin(), ids.From[ids.ContactKind](contact), nil); err != nil {
				t.Fatal(err)
			}
			if evidenceRemoved == "while_archived" {
				narrowSignatureMail(t, e, mail)
			}
			ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
			runMachineAddressTask(ctx, t, e, signaturePhonesDataTask())
			restorePhoneContact(t, e, contact)
			if got := liveNumbers(t, e, contact); len(got) != 0 {
				t.Fatalf("contact restore revived unsupported numbers: %v", got)
			}
		})
	}
}

func TestTheNightlyPassRepairsEvidenceLostAfterTheDataTaskCompleted(t *testing.T) {
	e := integration.Setup(t)
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	runMachineAddressTask(ctx, t, e, signaturePhonesDataTask())
	contact := orphanSignatureNumber(t, e, "later@recurrence.example")
	worker := dedupeRecheckWorker{pool: e.Pool}
	if err := worker.recheckWorkspace(context.Background(), e.WS); err != nil {
		t.Fatal(err)
	}
	if got := liveNumbers(t, e, contact); len(got) != 0 {
		t.Fatalf("nightly pass kept unsupported number: %v", got)
	}
	if err := worker.recheckWorkspace(context.Background(), e.WS); err != nil {
		t.Fatal(err)
	}
}

func TestABoldOutlookQuoteAddsNoNumberWithoutAMemberSignature(t *testing.T) {
	e := integration.Setup(t)
	contact := seedEnrichContact(t, e, "bold@recurrence.example", "Works.\n\nBob Contact\n"+bobOwnNumber+"\n\n"+
		"*From:* Sam Seller <sam@seller.example>\n*Sent:* Monday\n\nSam Seller\n"+memberNumber)
	runEnrichPass(t, e, &signatureScriptBrain{fields: phoneReply(bobOwnNumber, memberNumber)})
	if got := liveNumbers(t, e, contact); !slices.Equal(got, []string{"+442079460000"}) {
		t.Fatalf("numbers = %v, want sender's own number", got)
	}
}

func TestRestoringAContactKeepsASupportedSignatureNumber(t *testing.T) {
	e := integration.Setup(t)
	contact := seedEnrichContact(t, e, "supported@recurrence.example", "Bob Contact\n"+germany)
	applySignature(t, e, contact, onlyMailOf(t, e, contact), phoneField(germany))
	if _, err := e.Contacts.ArchiveContact(e.Admin(), ids.From[ids.ContactKind](contact), nil); err != nil {
		t.Fatal(err)
	}
	restorePhoneContact(t, e, contact)
	if got := liveNumbers(t, e, contact); !slices.Equal(got, []string{"+491755550101"}) {
		t.Fatalf("restore lost supported number: %v", got)
	}
}

func TestSignaturePhoneSupportUsesTheCanonicalEvidenceKey(t *testing.T) {
	e := integration.Setup(t)
	contact := seedEnrichContact(t, e, "spelling@recurrence.example", "Bob Contact\n"+germany)
	applySignature(t, e, contact, onlyMailOf(t, e, contact), phoneField(germany))
	e.WsExec(t, `UPDATE contact_profile_field SET value = @value WHERE contact_id = @id AND field = 'phone'`, pgx.StrictNamedArgs{"id": contact, "value": germany})
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	runMachineAddressTask(ctx, t, e, signaturePhonesDataTask())
	if got := liveNumbers(t, e, contact); !slices.Equal(got, []string{"+491755550101"}) {
		t.Fatalf("repair lost number with canonical evidence: %v", got)
	}
	narrowSignatureMail(t, e, onlyMailOf(t, e, contact))
	if got := liveNumbers(t, e, contact); len(got) != 0 {
		t.Fatalf("withdrawal missed canonical number: %v", got)
	}
}

func TestRepairClosesUnsupportedPhonePairsWithoutRefilingThem(t *testing.T) {
	e := integration.Setup(t)
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	var subjects []ids.UUID
	for _, candidate := range []struct{ name, email string }{
		{"Ola Berg", "ola@recurrence.example"}, {"Kim Lund", "kim@recurrence.example"}, {"Zoe Quinn", "zoe@recurrence.example"},
	} {
		contact := seedEnrichContact(t, e, candidate.email, candidate.name+"\n"+germany)
		if _, err := e.Contacts.UpdateContact(e.Admin(), ids.From[ids.ContactKind](contact), contacts.UpdateContactInput{FullName: &candidate.name}); err != nil {
			t.Fatal(err)
		}
		applySignature(t, e, contact, onlyMailOf(t, e, contact), phoneField(germany))
		subjects = append(subjects, contact)
	}
	runMachineAddressTask(ctx, t, e, dedupeScanDataTask())
	if got := scannedPairs(t, e); got != 3 {
		t.Fatalf("shared numbers filed %d pairs, want three", got)
	}
	for _, id := range subjects[:2] {
		e.WsExec(t, `DELETE FROM contact_profile_field WHERE contact_id = @id AND field = 'phone'`, pgx.StrictNamedArgs{"id": id})
	}
	runMachineAddressTask(ctx, t, e, signaturePhonesDataTask())
	worker := dedupeRecheckWorker{pool: e.Pool}
	for range 2 {
		if err := worker.recheckWorkspace(context.Background(), e.WS); err != nil {
			t.Fatal(err)
		}
		if got := e.WsCount(t, `SELECT count(*) FROM dedupe_candidate WHERE disposition = 'open' AND archived_at IS NULL`); got != 0 {
			t.Fatalf("repair left or refiled %d unsupported pairs", got)
		}
	}
	if got := liveNumbers(t, e, subjects[2]); !slices.Equal(got, []string{"+491755550101"}) {
		t.Fatalf("repair removed supported number: %v", got)
	}
}

func TestTheUpdatedSignatureWindowTaskRepairsPreviouslyStoredBoldQuotes(t *testing.T) {
	e := integration.Setup(t)
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	previous := signatureWindowDataTask()
	previous.Version--
	runMachineAddressTask(ctx, t, e, previous)
	contact := seedEnrichContact(t, e, "stored-bold@recurrence.example", "Works.\n\nBob Contact\n"+bobOwnNumber+"\n\n"+
		"*From:* Sam Seller <sam@seller.example>\n*Sent:* Monday\n\nSam Seller\n"+memberNumber)
	applySignature(t, e, contact, onlyMailOf(t, e, contact), phoneField(bobOwnNumber), phoneField(memberNumber))
	runMachineAddressTask(ctx, t, e, signatureWindowDataTask())
	if got := liveNumbers(t, e, contact); !slices.Equal(got, []string{"+442079460000"}) {
		t.Fatalf("updated repair kept the quoted number: %v", got)
	}
}
