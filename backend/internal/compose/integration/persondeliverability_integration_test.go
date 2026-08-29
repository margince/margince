// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The person page's deliverability section, end to end through Assemble: a
// hard-bounced address is named on the page, and a caller without the
// activity grant gets the section withheld and named — never an empty list
// dressed as good news. Seeded through the real writers (CreatePerson,
// StageTx, RecordSent, RecordBounce).

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/person360"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/comms"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

func TestPerson360NamesABouncedAddressAndWithholdsWithoutTheGrant(t *testing.T) {
	e := Setup(t)
	owner := OwnerConn(t)

	person, err := e.People.CreatePerson(e.Admin(), people.CreatePersonInput{
		FullName: "Anna Weber", Source: "manual",
		Emails: []people.PersonEmailInput{{Email: "anna@dead.example", EmailType: "work", IsPrimary: true}},
	})
	if err != nil {
		t.Fatalf("creating the person: %v", err)
	}

	// One send to her address through the real comms writers, then the
	// delivery report that names it.
	activityID := ids.New[ids.ActivityKind]()
	if _, err := owner.Exec(context.Background(),
		`INSERT INTO activity (id, kind, source, captured_by) VALUES ($1, 'email', 'test', 'human:x')`,
		activityID); err != nil {
		t.Fatalf("seeding the send's activity: %v", err)
	}
	// Filed under Anna, the way capture files a send: the link is what the
	// discover clause scopes on, and a link-less activity is deliberately
	// discoverable by everyone (the note rule).
	if _, err := owner.Exec(context.Background(),
		`INSERT INTO activity_link (id, activity_id, entity_type, person_id) VALUES ($1, $2, 'person', $3)`,
		ids.NewV7(), activityID, ids.UUID(person.Id)); err != nil {
		t.Fatalf("filing the send under the person: %v", err)
	}
	commsStore := comms.NewStore(e.DB(), time.Now, activities.NewStore(e.DB()))
	sender := e.As(e.Rep1, []ids.UUID{e.Team1}, AccountRepPerms)
	var deliveryID ids.UUID
	if err := database.WithWorkspaceTx(sender, e.Pool, func(tx pgx.Tx) error {
		var txErr error
		deliveryID, txErr = commsStore.StageTx(sender, tx, comms.StageInput{
			ActivityID: activityID, Provider: "gmail", MessageID: "dead@myco.test",
			Recipients: []string{"anna@dead.example"}, Cc: []string{},
			Subject: "Proposal", Body: "As discussed.", ConsentPurpose: "transactional",
			References: []string{},
		})
		return txErr
	}); err != nil {
		t.Fatalf("staging the send: %v", err)
	}
	if err := commsStore.RecordSent(sender, deliveryID, connector.SendReceipt{ProviderMessageID: "prov-1"}); err != nil {
		t.Fatalf("recording the send: %v", err)
	}
	reporter := principal.WithWorkspaceID(context.Background(), e.WS)
	reporter = principal.WithActor(reporter, principal.Principal{
		Type: principal.PrincipalConnector, ID: "connector:gmail",
		UserID: e.Rep1, OnBehalfOf: e.Rep1,
	})
	reporter = principal.WithCorrelationID(reporter, ids.NewV7())
	if marked, err := commsStore.RecordBounce(reporter, connector.BounceReport{
		MessageID: "dead@myco.test", Recipient: "anna@dead.example",
		Kind: connector.BounceHard, Reason: "550 5.1.1 user unknown",
	}); err != nil || !marked {
		t.Fatalf("recording the bounce: marked=%v err=%v", marked, err)
	}

	svc := person360.NewService(e.Pool, e.People, e.Deals, e.Projects, consent.NewStore(e.DB()),
		commsStore, ai.NewFeedbackStore(e.DB()), time.Now)

	page, err := svc.Assemble(e.Admin(), ids.From[ids.PersonKind](ids.UUID(person.Id)))
	if err != nil {
		t.Fatalf("assembling the page: %v", err)
	}
	if page.DeadAddresses == nil || !slices.Contains(*page.DeadAddresses, "anna@dead.example") {
		t.Fatalf("dead_addresses = %v, want the bounced address named", page.DeadAddresses)
	}

	// A caller with the person grant but no activity grant: the section is
	// withheld and NAMED, never an empty list dressed as good news.
	limited := AccountRepPerms
	limited.Objects = map[string]principal.ObjectGrant{"person": {Read: true}}
	blind, err := svc.Assemble(e.As(e.Rep2, []ids.UUID{e.Team1}, limited), ids.From[ids.PersonKind](ids.UUID(person.Id)))
	if err != nil {
		t.Fatalf("assembling without the activity grant: %v", err)
	}
	if blind.DeadAddresses != nil {
		t.Fatalf("dead_addresses = %v for a caller without the grant, want the section absent", blind.DeadAddresses)
	}
	if !slices.Contains(blind.SectionsOmitted, crmcontracts.Person360SectionsOmittedDeadAddresses) {
		t.Fatalf("sections_omitted = %v, want dead_addresses named", blind.SectionsOmitted)
	}

	// The oracle the discover clause closes: minting a throwaway person with
	// an address someone ELSE mailed must not read that colleague's failed
	// correspondence back. A second send to an address no person record
	// claims bounces on the same activity, the activity's one person link
	// (Anna) becomes capture-private to Rep1 — the state capture's own
	// person minting produces — and a third rep then creates their own
	// person carrying that address.
	probeDelivery := ids.UUID{}
	if err := database.WithWorkspaceTx(sender, e.Pool, func(tx pgx.Tx) error {
		var txErr error
		probeDelivery, txErr = commsStore.StageTx(sender, tx, comms.StageInput{
			ActivityID: activityID, Provider: "gmail", MessageID: "probe@myco.test",
			Recipients: []string{"ceo@probe.test"}, Cc: []string{},
			Subject: "Intro", Body: "Hello.", ConsentPurpose: "transactional",
			References: []string{},
		})
		return txErr
	}); err != nil {
		t.Fatalf("staging the probe send: %v", err)
	}
	if err := commsStore.RecordSent(sender, probeDelivery, connector.SendReceipt{ProviderMessageID: "prov-2"}); err != nil {
		t.Fatalf("recording the probe send: %v", err)
	}
	if marked, err := commsStore.RecordBounce(reporter, connector.BounceReport{
		MessageID: "probe@myco.test", Recipient: "ceo@probe.test",
		Kind: connector.BounceHard, Reason: "550 5.1.1 user unknown",
	}); err != nil || !marked {
		t.Fatalf("recording the probe bounce: marked=%v err=%v", marked, err)
	}
	if _, err := owner.Exec(context.Background(),
		`UPDATE person SET visibility = 'owner', owner_id = $2 WHERE id = $1`,
		ids.UUID(person.Id), e.Rep1); err != nil {
		t.Fatalf("making Anna capture-private: %v", err)
	}
	prober := e.As(e.Rep3, []ids.UUID{e.Team2}, AccountRepPerms)
	probe, err := e.People.CreatePerson(prober, people.CreatePersonInput{
		FullName: "Probe Target", Source: "manual",
		Emails: []people.PersonEmailInput{{Email: "ceo@probe.test", EmailType: "work", IsPrimary: true}},
	})
	if err != nil {
		t.Fatalf("creating the probe person: %v", err)
	}
	probed, err := svc.Assemble(prober, ids.From[ids.PersonKind](ids.UUID(probe.Id)))
	if err != nil {
		t.Fatalf("assembling the probe page: %v", err)
	}
	if probed.DeadAddresses == nil || len(*probed.DeadAddresses) != 0 {
		t.Fatalf("dead_addresses = %v on the probe page, want empty — the send's activity is not the prober's to discover", probed.DeadAddresses)
	}
}
