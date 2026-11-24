// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/jurisdiction"
	"github.com/margince/margince/backend/internal/shared/ports/messagingrules"
	"github.com/margince/margince/backend/internal/shared/ports/subjectrightsrules"
	pubjurisdiction "github.com/margince/margince/backend/pkg/extension/jurisdiction"
	"github.com/margince/margince/backend/pkg/extension/messaging"
)

// requestReceiptCountry follows the recipient and owes a first answer in two
// working days, as the Vietnamese pack does.
const requestReceiptCountry jurisdiction.Code = "xq"

var registerRequestReceiptCountry sync.Once

// TestARestrictionUnderARecipientBoundPackIsReceiptedThroughTheEngine: the
// request receipt goes out on the real lane, and the engine allows it on the
// standing restriction it answers.
func TestARestrictionUnderARecipientBoundPackIsReceiptedThroughTheEngine(t *testing.T) {
	registerRequestReceiptCountry.Do(func() {
		messagingrules.Register(messagingrules.Rules{
			Jurisdiction: requestReceiptCountry, Version: 1, DataProtectionFollowsRecipient: true,
			OptOutAcknowledgement: true, OptOutAcknowledgementUTCOffset: 7 * time.Hour,
		})
		subjectrightsrules.Register(subjectrightsrules.Rules{
			Jurisdiction: requestReceiptCountry, Version: 1,
			Instruments: []messaging.Instrument{{Name: "Test Decree 3/2026"}},
			Deadlines: map[subjectrightsrules.RequestKind]subjectrightsrules.Deadline{
				"restriction": {Period: pubjurisdiction.Period{Days: 15}},
			},
			ReplyWithinWorkingDays: 2,
		})
	})
	e := Setup(t)
	ApplyRiverSchema(t)
	inserter, err := jobs.NewInserter(e.Pool, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	contact := e.SeedContact(t, "Request Subject", &e.Rep1)
	e.WsExec(t, `INSERT INTO contact_email (contact_id, email, is_primary, source, captured_by)
		VALUES ($1, 'request-subject@example.test', true, 'manual', 'human:test')`, contact)
	e.WsExec(t, `UPDATE contact SET address_country = upper($2) WHERE id = $1`, contact, string(requestReceiptCountry))
	queue := compose.NewControllerMailQueue(e.Pool, inserter, "privacy@example.test")
	store := consent.NewStore(compose.InstallationDB(e.Pool)).
		WithConfirmationLane(queue, unusedLinkVault{}, "https://crm.example.test")

	if err := store.Suppress(e.Admin(), consent.SuppressInput{
		ContactID: ids.From[ids.ContactKind](contact), Kind: "processing_restriction", Reason: "Disputed data.",
	}); err != nil {
		t.Fatalf("recording the restriction: %v", err)
	}

	var verdict string
	if err := e.Pool.QueryRow(context.Background(), `
		SELECT d.verdict
		  FROM comms_outbound o
		  JOIN communication_decision d ON d.delivery_id = o.id
		 WHERE o.template_key = $1 AND o.sender_kind = 'controller'`,
		consent.TemplateRequestReceipt).Scan(&verdict); err != nil {
		t.Fatalf("reading the request receipt and its decision: %v", err)
	}
	if verdict != "allow" {
		t.Errorf("the engine answered %q for the request receipt, want allow on the standing restriction", verdict)
	}
}
