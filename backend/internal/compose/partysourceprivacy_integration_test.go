// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestCapturedPartyEvidenceIsExportedAndErasedWithoutAProviderOriginal(t *testing.T) {
	e := integration.Setup(t)
	alice, err := e.Contacts.CreateContact(e.Admin(), contacts.CreateContactInput{FullName: "Alice Buyer", Source: "manual", Emails: []contacts.ContactEmailInput{{Email: "alice@buyer.test", EmailType: "work", IsPrimary: true}}})
	if err != nil {
		t.Fatal(err)
	}
	rec := partiesMailRecord(t, e, partyMailProof{outbound: false, sentByOwner: false}, "Bob Buyer <bob@buyer.test>", "", "Could we discuss the product requirements?")
	rec.Raw = nil
	sink := newCaptureSink(e.Pool, CaptureConfig{})
	ctx := threadConnectorCtx(e, e.AdminUser)
	ref, err := sink.Upsert(ctx, rec)
	if err != nil {
		t.Fatal(err)
	}
	bob, err := e.Contacts.CreateContact(e.Admin(), contacts.CreateContactInput{FullName: "Bob Buyer", Source: "manual", Emails: []contacts.ContactEmailInput{{Email: "bob@buyer.test", EmailType: "work", IsPrimary: true}}})
	if err != nil {
		t.Fatal(err)
	}
	if e.WsCount(t, `SELECT count(*) FROM activity_link WHERE activity_id=$1 AND contact_id=$2`, ref.ID, alice.Id) != 1 {
		t.Fatal("shared message lost the existing contact")
	}
	if e.WsCount(t, `SELECT count(*) FROM capture_party_source WHERE activity_id=$1 AND raw_capture_id IS NULL AND policy_addresses @> ARRAY['bob@buyer.test']`, ref.ID) != 1 {
		t.Fatal("fixture lacks evidence independent of a provider original")
	}
	exported, err := privacy.AssembleSAR(e.Admin(), InstallationDB(e.Pool), ids.From[ids.ContactKind](ids.UUID(bob.Id)))
	if err != nil {
		t.Fatal(err)
	}
	if len(exported.CapturePartySources) != 0 {
		t.Fatal("held message evidence was disclosed")
	}
	shared, err := NewThreadAudienceSetter(e.Pool).Decide(partyOwnerContext(t, e), rec.ThreadKey, true)
	if err != nil || !shared.Shared {
		t.Fatalf("owner sharing: %+v %v", shared, err)
	}
	exported, err = privacy.AssembleSAR(e.Admin(), InstallationDB(e.Pool), ids.From[ids.ContactKind](ids.UUID(bob.Id)))
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(exported.CapturePartySources)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(payload), "bob@buyer.test") || strings.Contains(string(payload), "alice@buyer.test") {
		t.Fatalf("subject evidence export=%s", payload)
	}
	if err := ComposedEraser(e.Pool, nil, nil).EraseContact(e.Admin(), ids.UUID(bob.Id), "Subject requested erasure"); err != nil {
		t.Fatal(err)
	}
	if e.WsCount(t, `SELECT count(*) FROM capture_party_source WHERE activity_id=$1 AND (recipients @> ARRAY['bob@buyer.test'] OR policy_addresses @> ARRAY['bob@buyer.test'])`, ref.ID) != 0 {
		t.Fatal("erased address survived in shared party evidence")
	}
	if e.WsCount(t, `SELECT count(*) FROM capture_party_source WHERE activity_id=$1 AND policy_addresses @> ARRAY['alice@buyer.test']`, ref.ID) != 1 {
		t.Fatal("erasing Bob destroyed Alice's source evidence")
	}
	if _, err := sink.Upsert(ctx, rec); err != nil {
		t.Fatal(err)
	}
	if e.WsCount(t, `SELECT count(*) FROM capture_party_source WHERE activity_id=$1 AND (recipients @> ARRAY['bob@buyer.test'] OR policy_addresses @> ARRAY['bob@buyer.test'])`, ref.ID) != 0 {
		t.Fatal("replay stored an erased address again")
	}
}

func TestPartySourceErasureFoldsSubjectAddressesWithoutErasingOtherParties(t *testing.T) {
	e := integration.Setup(t)
	ref := capturePartiesMailUsing(t, e, newCaptureSink(e.Pool, CaptureConfig{}).WithEnsurer(nil, nil), partyMailProof{outbound: true, sentByOwner: true}, "Bob Buyer <bob@buyer.test>", "", "Could we discuss the product requirements?")
	if err := e.DB().Tx(e.Admin(), func(tx pgx.Tx) error {
		return capture.NewPendingStore(InstallationDB(e.Pool)).PurgePartySourcesForEmailsTx(e.Admin(), tx, []string{" BOB@BUYER.TEST "})
	}); err != nil {
		t.Fatal(err)
	}
	if e.WsCount(t, `SELECT count(*) FROM capture_party_source WHERE activity_id=$1 AND (recipients @> ARRAY['bob@buyer.test'] OR policy_addresses @> ARRAY['bob@buyer.test'])`, ref.ID) != 0 {
		t.Fatal("mixed-case subject address survived erasure")
	}
	if e.WsCount(t, `SELECT count(*) FROM capture_party_source WHERE activity_id=$1 AND recipients @> ARRAY['alice@buyer.test'] AND policy_addresses @> ARRAY['alice@buyer.test']`, ref.ID) != 1 {
		t.Fatal("erasure removed another recipient's evidence")
	}
}
