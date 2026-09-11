// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// What a timeline row says HAPPENED to an outbound message.
//
// The row carried direction and nothing else, and "outbound" was drawn as
// SENT. A message parked because the channel refused its files, or one the
// receiving system handed straight back, rendered exactly like one the
// provider confirmed — the rep was told their mail went.
//
// Every delivery here is staged and closed through the real comms writers, so
// what the reader reports is what a send actually leaves behind rather than a
// row this file invented.

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/comms"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// deliveryEnv is one person's timeline plus the comms writers that stage
// against it.
type deliveryEnv struct {
	*Env
	comms   *comms.Store
	author  context.Context
	contact ids.UUID
}

func setupDelivery(t *testing.T) *deliveryEnv {
	t.Helper()
	e := Setup(t)
	author := e.As(e.Rep1, []ids.UUID{e.Team1}, activityLifecyclePerms)
	return &deliveryEnv{
		Env:     e,
		comms:   comms.NewStore(e.DB(), time.Now, activities.NewStore(e.DB())),
		author:  author,
		contact: e.SeedPerson(t, "Dana Buyer", &e.Rep1),
	}
}

// sent logs an outbound message on the contact's timeline and returns it.
func (d *deliveryEnv) sent(t *testing.T, subject string) ids.UUID {
	t.Helper()
	body := "As discussed."
	logged, _, err := d.Activities.LogActivity(d.author, activities.LogActivityInput{
		Kind: "email", Subject: &subject, Body: &body, Direction: strPtr("outbound"),
		Links: []activities.ActivityLinkInput{{EntityType: "person", EntityID: d.contact}},
	})
	if err != nil {
		t.Fatalf("logging %q: %v", subject, err)
	}
	return ids.UUID(logged.Id)
}

// stage hands the message to the delivery machinery, exactly as a send does.
func (d *deliveryEnv) stage(t *testing.T, activity ids.UUID, messageID string, files []comms.OutboundFile) ids.UUID {
	t.Helper()
	var delivery ids.UUID
	if err := database.WithWorkspaceTx(d.author, d.Pool, func(tx pgx.Tx) error {
		var txErr error
		delivery, txErr = d.comms.StageTx(d.author, tx, comms.StageInput{
			ActivityID: ids.From[ids.ActivityKind](activity), Provider: "gmail",
			MessageID: messageID, Recipients: []string{"dana@buyer.example"}, Cc: []string{},
			Subject: "Proposal", Body: "As discussed.", ConsentPurpose: "transactional",
			References: []string{}, Attachments: files,
		})
		return txErr
	}); err != nil {
		t.Fatalf("staging the delivery: %v", err)
	}
	return delivery
}

// timelineRow reads the contact's timeline and answers the email row for one
// message, which is the projection the screen draws.
func (d *deliveryEnv) timelineRow(t *testing.T, who context.Context, activity ids.UUID) *crmcontracts.EmailSummary {
	t.Helper()
	page, _, err := d.Activities.ListActivities(who, activities.ListActivitiesInput{
		EntityType: strPtr("person"), EntityID: &d.contact,
	})
	if err != nil {
		t.Fatalf("listing the timeline: %v", err)
	}
	row := rowFor(page, activity)
	if row == nil {
		t.Fatalf("the message carried no email row at all")
	}
	return row
}

// A parked message and a delivered one are two different answers, and a
// message that was only logged is a third: it has no delivery to report, which
// is not the same as one that has not left yet.
func TestTheTimelineTellsAParkedMessageFromADeliveredOne(t *testing.T) {
	d := setupDelivery(t)

	delivered := d.sent(t, "The proposal")
	if err := d.comms.RecordSent(d.author, d.stage(t, delivered, "ok@myco.test", nil),
		connector.SendReceipt{ProviderMessageID: "prov-1"}); err != nil {
		t.Fatalf("recording the send: %v", err)
	}
	refused := d.sent(t, "The signed contract")
	if err := d.comms.Park(d.author, d.stage(t, refused, "nope@myco.test", nil),
		"the channel refused the attachment"); err != nil {
		t.Fatalf("parking: %v", err)
	}
	logged := d.sent(t, "Called instead")

	ok := d.timelineRow(t, d.author, delivered).Delivery
	if ok == nil || ok.State != crmcontracts.EmailDeliveryStateSent {
		t.Fatalf("the delivered message reported %v, want sent", ok)
	}
	if ok.DeliveredAt == nil {
		t.Error("a sent message carried no moment it was accepted")
	}

	held := d.timelineRow(t, d.author, refused).Delivery
	if held == nil || held.State != crmcontracts.EmailDeliveryStateParked {
		t.Fatalf("the parked message reported %v — the rep is being told it went", held)
	}
	if held.Reason == nil || *held.Reason != "the channel refused the attachment" {
		t.Errorf("reason = %v, want the words the park was recorded with — a rep has to act on it",
			held.Reason)
	}

	if none := d.timelineRow(t, d.author, logged).Delivery; none != nil {
		t.Errorf("a message that was only logged reported %v; it was never handed to a provider",
			none)
	}
}

// A bounce outranks the status it sits on. The provider accepted the message,
// so comms_outbound keeps `sent` — and the mail did not arrive, which is the
// one thing that status can never say.
func TestABouncedMessageIsNotReportedAsSent(t *testing.T) {
	d := setupDelivery(t)

	returned := d.sent(t, "The proposal")
	delivery := d.stage(t, returned, "dead@myco.test", nil)
	if err := d.comms.RecordSent(d.author, delivery,
		connector.SendReceipt{ProviderMessageID: "prov-2"}); err != nil {
		t.Fatalf("recording the send: %v", err)
	}
	// Admitted as sent first, so the state below is the bounce's doing rather
	// than a delivery that never reported anything.
	if before := d.timelineRow(t, d.author, returned).Delivery; before == nil ||
		before.State != crmcontracts.EmailDeliveryStateSent {
		t.Fatalf("before the bounce the row reported %v; the case below would prove nothing", before)
	}

	reporter := principal.WithCorrelationID(
		principal.WithActor(principal.WithWorkspaceID(context.Background(), d.WS),
			principal.Principal{
				Type: principal.PrincipalConnector, ID: "connector:gmail",
				UserID: d.Rep1, OnBehalfOf: d.Rep1,
			}), ids.NewV7())
	if marked, err := d.comms.RecordBounce(reporter, connector.BounceReport{
		MessageID: "dead@myco.test", Recipient: "dana@buyer.example",
		Kind: connector.BounceHard, Reason: "550 5.1.1 user unknown",
	}); err != nil || !marked {
		t.Fatalf("recording the bounce: marked=%v err=%v", marked, err)
	}

	after := d.timelineRow(t, d.author, returned).Delivery
	if after == nil || after.State != crmcontracts.EmailDeliveryStateBounced {
		t.Fatalf("the returned message reported %v, want bounced", after)
	}
	if after.Reason == nil || *after.Reason != "550 5.1.1 user unknown" {
		t.Errorf("reason = %v, want what the receiving system said", after.Reason)
	}
}

// What the timeline says a message carried is the SNAPSHOT taken at staging,
// so archiving the document afterwards changes the library and changes nothing
// about what already went out.
func TestTheTimelineSaysWhatAMessageCarriedAfterTheDocumentIsArchived(t *testing.T) {
	d := setupDelivery(t)

	carried := d.sent(t, "The signed contract")
	file := seedAttachment(t, carried, "contract.pdf")
	delivery := d.stage(t, carried, "files@myco.test", []comms.OutboundFile{{
		AttachmentID: file, Filename: "contract.pdf",
		ContentType: "application/pdf", ByteSize: 20_480,
	}})
	if err := d.comms.RecordSent(d.author, delivery,
		connector.SendReceipt{ProviderMessageID: "prov-3"}); err != nil {
		t.Fatalf("recording the send: %v", err)
	}
	if err := d.Activities.ArchiveAttachment(d.author, file); err != nil {
		t.Fatalf("archiving the document: %v", err)
	}

	row := d.timelineRow(t, d.author, carried)
	if row.AttachmentCount != 0 {
		t.Fatalf("attachment_count = %d after archiving; the case below would not be showing "+
			"the snapshot", row.AttachmentCount)
	}
	if row.Delivery == nil || row.Delivery.Files == nil {
		t.Fatalf("the sent message said nothing about what it carried: %v", row.Delivery)
	}
	files := *row.Delivery.Files
	if len(files) != 1 || files[0].Filename != "contract.pdf" {
		t.Fatalf("files = %v, want the one document the message was staged with", files)
	}
	if files[0].ContentType == nil || *files[0].ContentType != "application/pdf" {
		t.Errorf("content_type = %v; the chip reads the kind of file from it", files[0].ContentType)
	}
	if files[0].ByteSize == nil || *files[0].ByteSize != 20_480 {
		t.Errorf("byte_size = %v, want the size recorded at staging", files[0].ByteSize)
	}
}

// A withheld row says NOTHING about delivery.
//
// The summary has had its content stripped, and "this message you may not read
// was parked because the recipient blocked us" is that content in smaller
// print — as is the name of the file it carried.
func TestAWithheldRowSaysNothingAboutDelivery(t *testing.T) {
	d := setupDelivery(t)
	colleague := d.As(d.Rep3, []ids.UUID{d.Team2}, activityLifecyclePerms)

	message := d.sent(t, "Severance agreement")
	file := seedAttachment(t, message, "severance.pdf")
	if err := d.comms.Park(d.author, d.stage(t, message, "held@myco.test",
		[]comms.OutboundFile{{AttachmentID: file, Filename: "severance.pdf"}}),
		"the recipient blocked the sender"); err != nil {
		t.Fatalf("parking: %v", err)
	}

	// Admitted first, so the silence below is the audience's doing rather than
	// a delivery that never reported anything.
	if before := d.timelineRow(t, colleague, message).Delivery; before == nil ||
		before.State != crmcontracts.EmailDeliveryStateParked {
		t.Fatalf("the colleague saw %v before limiting; the withheld case would prove nothing",
			before)
	}
	if _, err := d.Activities.SetAudience(d.author, ids.From[ids.ActivityKind](message),
		activities.SetAudienceInput{Audience: "participants"}); err != nil {
		t.Fatalf("limiting: %v", err)
	}

	held := d.timelineRow(t, colleague, message)
	if held.DisplayStatus != crmcontracts.EmailAccessStatusWithheld {
		t.Fatalf("display_status = %q, want withheld", held.DisplayStatus)
	}
	if held.Delivery != nil {
		t.Errorf("a withheld row told the colleague %v about a message they may not read",
			held.Delivery)
	}

	// The author is in the audience and still sees what happened to their own.
	if own := d.timelineRow(t, d.author, message).Delivery; own == nil ||
		own.State != crmcontracts.EmailDeliveryStateParked {
		t.Errorf("the author lost the state of their own message: %v", own)
	}
}
