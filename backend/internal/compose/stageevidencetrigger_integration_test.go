// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The three deterministic evidence writers, driven through the trigger the way
// the relay drives it.
//
// These are mostly ROUTING tests, for the reason graphconsumer's are: the
// consumer is where a wrong event name or a missing condition costs nothing at
// compile time and everything at runtime. So each case asserts both that the
// envelope it should act on writes a row, and that the near-miss beside it
// does not.

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/database"
	kevents "github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// staticDomains is the own-domain seam, answering a fixed list.
type staticDomains []string

func (d staticDomains) Domains(context.Context, pgx.Tx) ([]string, error) {
	return []string(d), nil
}

// evidenceTriggerEnv seeds a deal on a stage carrying one criterion of each
// kind the deterministic writers settle, and answers the trigger over it.
type evidenceTriggerEnv struct {
	*integration.Env
	trigger *StageEvidenceTrigger
	dealID  ids.DealID
	stageID ids.StageID
}

func newEvidenceTriggerEnv(t *testing.T, domains ...string) *evidenceTriggerEnv {
	t.Helper()
	e := integration.Setup(t)
	ctx := e.Admin()

	pipeline, err := e.Deals.CreatePipeline(ctx, deals.CreatePipelineInput{Name: "Sales"})
	if err != nil {
		t.Fatalf("seeding a pipeline: %v", err)
	}
	pipelineID := ids.From[ids.PipelineKind](ids.UUID(pipeline.Id))
	stage, err := e.Deals.CreateStage(ctx, deals.CreateStageInput{
		PipelineID: pipelineID, Name: "Qualified", Position: 0,
		Semantic: string(deals.SemanticOpen),
	})
	if err != nil {
		t.Fatalf("seeding a stage: %v", err)
	}
	stageID := ids.From[ids.StageKind](ids.UUID(stage.Id))
	for key, kind := range map[string]deals.CriterionKind{
		"signed": deals.CriterionDocumentSigned,
		"terms":  deals.CriterionTermsAccepted,
		"met":    deals.CriterionEventHeld,
	} {
		if _, err := e.Deals.CreateStageExitCriterion(ctx, deals.CreateCriterionInput{
			StageID: stageID, Key: key, Label: key, Kind: string(kind),
		}); err != nil {
			t.Fatalf("seeding the %s criterion: %v", key, err)
		}
	}
	dealID := e.SeedDeal(t, "Warehouse rollout", pipelineID, stageID, &e.AdminUser)

	return &evidenceTriggerEnv{
		Env: e,
		// A nil reading enqueuer AND a nil proposer: these tests are about the
		// DETERMINISTIC half, and both nils are the composition an installation
		// with no model lane runs. TestTheReadingIsQueuedForEveryActivityOnADeal
		// covers the reading; the proposing half is covered end to end by
		// compose/integration/stageprogression_e2e_integration_test.go, which
		// drives the real proposer rather than a stand-in.
		trigger: NewStageEvidenceTrigger(
			e.Pool, e.Deals, staticDomains(domains), nil, nil, slog.Default()),
		dealID:  ids.From[ids.DealKind](dealID),
		stageID: stageID,
	}
}

// ledger answers how many evidence rows the deal carries.
func (e *evidenceTriggerEnv) ledger(t *testing.T) int {
	t.Helper()
	var rows int
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT count(*) FROM deal_stage_evidence WHERE deal_id = $1`,
			e.dealID.UUID).Scan(&rows)
	}); err != nil {
		t.Fatalf("counting the ledger: %v", err)
	}
	return rows
}

//craft:ignore naked-any the payload goes straight to json.Marshal, and the tests pass a different generated event struct on each call
func evidenceEnvelope(eventType, entityType string, entity ids.UUID, payload any) kevents.Envelope {
	raw, err := json.Marshal(payload)
	if err != nil {
		panic(err)
	}
	return kevents.Envelope{
		EventID: ids.NewV7(), Type: eventType, Version: 1,
		OccurredAt: time.Now().UTC(),
		Entity:     kevents.EntityRef{Type: entityType, ID: entity},
		Payload:    raw,
		Trace:      kevents.Trace{CorrelationID: ids.NewV7()},
	}
}

// A contract turning active is a signature, and no model is asked about it.
func TestAContractTurningActiveRecordsDocumentSignedWithoutAModel(t *testing.T) {
	e := newEvidenceTriggerEnv(t)
	contractID := e.seedContract(t)

	env := evidenceEnvelope("contract.status_changed", "contract", contractID,
		crmcontracts.PublicEventContractStatusChanged{FromStatus: "draft", ToStatus: "active"})
	if err := e.trigger.HandleEvent(context.Background(), env); err != nil {
		t.Fatalf("handling the signature: %v", err)
	}
	if got := e.ledger(t); got != 1 {
		t.Fatalf("a signed contract wrote %d evidence rows", got)
	}
	e.assertClaim(t, deals.SourceContract, deals.ExtractedByDeterministic)
}

// Only the transition INTO active. A contract expiring does not un-sign the
// document it was, and a draft one was never signed at all.
func TestOnlyTheMoveIntoActiveIsASignature(t *testing.T) {
	e := newEvidenceTriggerEnv(t)
	contractID := e.seedContract(t)

	for _, to := range []string{"draft", "expired", "cancelled", "superseded"} {
		env := evidenceEnvelope("contract.status_changed", "contract", contractID,
			crmcontracts.PublicEventContractStatusChanged{FromStatus: "active", ToStatus: to})
		if err := e.trigger.HandleEvent(context.Background(), env); err != nil {
			t.Fatalf("handling a move to %s: %v", to, err)
		}
	}
	if got := e.ledger(t); got != 0 {
		t.Fatalf("%d rows were written for transitions that are not signatures", got)
	}
}

// An activity reaching no deal settles nothing: there is no deal whose stage
// could be asking for it.
func TestAnUnlinkedActivityWritesNoEvidence(t *testing.T) {
	e := newEvidenceTriggerEnv(t, "acme-sales.example")
	activityID := e.seedMeeting(t, meetingSeed{occurredAt: time.Now().Add(-time.Hour), status: "held"})

	env := evidenceEnvelope("activity.captured", "activity", activityID,
		crmcontracts.PublicEventActivityCaptured{Kind: "meeting"})
	if err := e.trigger.HandleEvent(context.Background(), env); err != nil {
		t.Fatalf("handling an unlinked meeting: %v", err)
	}
	if got := e.ledger(t); got != 0 {
		t.Fatalf("an activity linked to no deal wrote %d rows", got)
	}
}

// A meeting only we attended is a meeting we held with ourselves.
func TestAMeetingWithNobodyFromTheirSideSettlesNothing(t *testing.T) {
	e := newEvidenceTriggerEnv(t, "acme-sales.example")
	activityID := e.seedMeeting(t, meetingSeed{linked: true, occurredAt: time.Now().Add(-time.Hour), status: "held"})
	e.onlyOurSeatsAttend(t, activityID)

	env := evidenceEnvelope("activity.captured", "activity", activityID,
		crmcontracts.PublicEventActivityCaptured{Kind: "meeting"})
	if err := e.trigger.HandleEvent(context.Background(), env); err != nil {
		t.Fatalf("handling an internal meeting: %v", err)
	}
	if got := e.ledger(t); got != 0 {
		t.Fatalf("a meeting only our own seats attended wrote %d rows", got)
	}
}

// A meeting in the future has not been held. Capture records scheduled ones
// too, and one settles event_held only once it happened.
func TestAScheduledMeetingHasNotBeenHeldYet(t *testing.T) {
	e := newEvidenceTriggerEnv(t, "acme-sales.example")
	activityID := e.seedMeeting(t, meetingSeed{linked: true, occurredAt: time.Now().Add(48 * time.Hour), status: "booked"})

	env := evidenceEnvelope("activity.captured", "activity", activityID,
		crmcontracts.PublicEventActivityCaptured{Kind: "meeting"})
	if err := e.trigger.HandleEvent(context.Background(), env); err != nil {
		t.Fatalf("handling a scheduled meeting: %v", err)
	}
	if got := e.ledger(t); got != 0 {
		t.Fatalf("a meeting two days from now wrote %d rows; it has not happened", got)
	}
}

// The positive control for the refusals above: a past meeting marked held with
// the buyer on it DOES settle event_held — even though capture stamped our own
// seat as its sender, which is what an authorship rule tripped over.
func TestAPastMeetingTheBuyerAttendedRecordsEventHeld(t *testing.T) {
	e := newEvidenceTriggerEnv(t, "acme-sales.example")
	activityID := e.seedMeeting(t, meetingSeed{linked: true, occurredAt: time.Now().Add(-time.Hour), status: "held"})

	env := evidenceEnvelope("activity.captured", "activity", activityID,
		crmcontracts.PublicEventActivityCaptured{Kind: "meeting"})
	if err := e.trigger.HandleEvent(context.Background(), env); err != nil {
		t.Fatalf("handling a held meeting: %v", err)
	}
	if got := e.ledger(t); got != 1 {
		t.Fatalf("a past meeting with the buyer in it wrote %d rows", got)
	}
	e.assertClaim(t, deals.SourceActivity, deals.ExtractedByDeterministic)
}

// A transcript is a recording of people talking, so it cannot exist for a
// meeting that did not happen — it settles event_held on its own, and the row
// cites the lines a human can open and read.
func TestAMeetingWithATranscriptRecordsEventHeldAndCitesIt(t *testing.T) {
	e := newEvidenceTriggerEnv(t, "acme-sales.example")
	activityID := e.seedMeeting(t, meetingSeed{
		linked: true, occurredAt: time.Now().Add(-time.Hour),
		transcript: "Ines: thanks for the walkthrough.\nRep: our pleasure.",
	})

	env := evidenceEnvelope("activity.captured", "activity", activityID,
		crmcontracts.PublicEventActivityCaptured{Kind: "meeting"})
	if err := e.trigger.HandleEvent(context.Background(), env); err != nil {
		t.Fatalf("handling a transcribed meeting: %v", err)
	}
	if got := e.ledger(t); got != 1 {
		t.Fatalf("a meeting with a transcript wrote %d rows", got)
	}
	if lines := e.citedLines(t); len(lines) != 2 || lines[0] != 1 || lines[1] != 2 {
		t.Errorf("the claim cites lines %v, want the whole two-line transcript", lines)
	}
}

// The meeting's own status says it did not happen, and that beats every other
// signal. A transcript attached to a canceled meeting is a recording of
// something else.
func TestACanceledMeetingSettlesNothingEvenWithATranscript(t *testing.T) {
	e := newEvidenceTriggerEnv(t, "acme-sales.example")
	activityID := e.seedMeeting(t, meetingSeed{
		linked: true, occurredAt: time.Now().Add(-time.Hour), status: "canceled",
		transcript: "Ines: sorry, we had to call this off.",
	})

	env := evidenceEnvelope("activity.captured", "activity", activityID,
		crmcontracts.PublicEventActivityCaptured{Kind: "meeting"})
	if err := e.trigger.HandleEvent(context.Background(), env); err != nil {
		t.Fatalf("handling a canceled meeting: %v", err)
	}
	if got := e.ledger(t); got != 0 {
		t.Fatalf("a canceled meeting wrote %d rows", got)
	}
}

// Delivery lag is not staleness. A worker down for hours must still write the
// evidence when it comes back: the earlier bound measured how long delivery
// took and silently dropped every record in the outage window, permanently —
// skipping answers nil, the subscriber acks, and nothing replays it.
func TestAnEventDeliveredLateIsStillWritten(t *testing.T) {
	e := newEvidenceTriggerEnv(t)
	contractID := e.seedContract(t)

	env := evidenceEnvelope("contract.status_changed", "contract", contractID,
		crmcontracts.PublicEventContractStatusChanged{FromStatus: "draft", ToStatus: "active"})
	// Signed two hours ago, delivered now: an outage, not history.
	env.OccurredAt = time.Now().Add(-2 * time.Hour)
	if err := e.trigger.HandleEvent(context.Background(), env); err != nil {
		t.Fatalf("handling a late-delivered signature: %v", err)
	}
	if got := e.ledger(t); got != 1 {
		t.Fatalf("a signature delivered two hours late wrote %d rows; the "+
			"outage window would be lost for good", got)
	}
}

// Capture never sets meeting_status — it writes the row from the calendar and
// leaves the column NULL — so a synced meeting without a transcript arrives
// with no proof it took place. The rep marking it held afterwards is the moment
// it acquires one, and that moment emits activity.updated and nothing else.
//
// Without this lane the feature is nearly inert: the meeting is skipped at
// capture and never looked at again.
func TestMarkingAMeetingHeldAfterwardsSettlesEventHeld(t *testing.T) {
	e := newEvidenceTriggerEnv(t, "acme-sales.example")
	activityID := e.seedMeeting(t, meetingSeed{
		linked: true, occurredAt: time.Now().Add(-time.Hour),
	})

	captured := evidenceEnvelope("activity.captured", "activity", activityID,
		crmcontracts.PublicEventActivityCaptured{Kind: "meeting"})
	if err := e.trigger.HandleEvent(context.Background(), captured); err != nil {
		t.Fatalf("handling the capture: %v", err)
	}
	if got := e.ledger(t); got != 0 {
		t.Fatalf("a meeting with no status and no transcript wrote %d rows at "+
			"capture; nothing yet says it happened", got)
	}

	e.markHeld(t, activityID)
	held := crmcontracts.MeetingWasHeld
	updated := evidenceEnvelope("activity.updated", "activity", activityID,
		crmcontracts.PublicEventActivityUpdated{
			ChangedFields: crmcontracts.PublicEventActivityChangedFields{MeetingStatus: &held},
		})
	if err := e.trigger.HandleEvent(context.Background(), updated); err != nil {
		t.Fatalf("handling the status change: %v", err)
	}
	if got := e.ledger(t); got != 1 {
		t.Fatalf("marking the meeting held wrote %d rows; the criterion would "+
			"stay unsettleable for every synced meeting", got)
	}
}

// Only a MEETING_STATUS change re-judges. Every other edit leaves the question
// of whether the meeting happened exactly where it was, and re-judging on each
// would rewrite the ledger for a fact that did not change.
func TestAnUnrelatedActivityEditDoesNotWriteEvidence(t *testing.T) {
	e := newEvidenceTriggerEnv(t, "acme-sales.example")
	activityID := e.seedMeeting(t, meetingSeed{
		linked: true, occurredAt: time.Now().Add(-time.Hour), status: "held",
	})

	updated := evidenceEnvelope("activity.updated", "activity", activityID,
		crmcontracts.PublicEventActivityUpdated{
			ChangedFields: crmcontracts.PublicEventActivityChangedFields{},
		})
	if err := e.trigger.HandleEvent(context.Background(), updated); err != nil {
		t.Fatalf("handling an unrelated edit: %v", err)
	}
	if got := e.ledger(t); got != 0 {
		t.Fatalf("re-subjecting a meeting wrote %d evidence rows", got)
	}
}

// A colleague logged through the manual path is a person-linked row with a
// NULL user_id, so counting "not a seat" as the buyer would let a meeting only
// our own people attended settle a criterion about the buyer turning up.
func TestAnInternalMeetingWithAPersonLinkedColleagueSettlesNothing(t *testing.T) {
	e := newEvidenceTriggerEnv(t, "acme-sales.example")
	activityID := e.seedMeeting(t, meetingSeed{
		linked: true, occurredAt: time.Now().Add(-time.Hour), status: "held",
	})
	e.replaceAttendeeWithPersonLink(t, activityID)

	env := evidenceEnvelope("activity.captured", "activity", activityID,
		crmcontracts.PublicEventActivityCaptured{Kind: "meeting"})
	if err := e.trigger.HandleEvent(context.Background(), env); err != nil {
		t.Fatalf("handling an internal meeting: %v", err)
	}
	if got := e.ledger(t); got != 0 {
		t.Fatalf("a meeting whose only non-seat attendee is a person link "+
			"wrote %d rows; nobody from their side was on it", got)
	}
}

// An attendee on one of OUR OWN domains is our own person, whatever the
// participant row does or does not carry as a seat.
func TestAnAttendeeOnOurOwnDomainIsNotTheBuyer(t *testing.T) {
	e := newEvidenceTriggerEnv(t, "acme-sales.example")
	activityID := e.seedMeeting(t, meetingSeed{
		linked: true, occurredAt: time.Now().Add(-time.Hour), status: "held",
	})
	e.setAttendeeAddress(t, activityID, "colleague@acme-sales.example")

	env := evidenceEnvelope("activity.captured", "activity", activityID,
		crmcontracts.PublicEventActivityCaptured{Kind: "meeting"})
	if err := e.trigger.HandleEvent(context.Background(), env); err != nil {
		t.Fatalf("handling the meeting: %v", err)
	}
	if got := e.ledger(t); got != 0 {
		t.Fatalf("a meeting attended only by our own domain wrote %d rows", got)
	}
}

// An envelope's occurred_at is stamped when the event is EMITTED, so a
// transcript of an old meeting imported today carries a fresh envelope. The
// window must read the meeting's own time or that import lands as if the
// meeting had just happened.
func TestAnOldMeetingImportedTodaySettlesNothing(t *testing.T) {
	e := newEvidenceTriggerEnv(t, "acme-sales.example")
	activityID := e.seedMeeting(t, meetingSeed{
		linked: true, occurredAt: time.Now().Add(-180 * 24 * time.Hour),
		transcript: "Ines: thanks for the walkthrough.",
	})

	env := evidenceEnvelope("activity.captured", "activity", activityID,
		crmcontracts.PublicEventActivityCaptured{Kind: "meeting"})
	// A fresh envelope, which is what an import produces.
	env.OccurredAt = time.Now()
	if err := e.trigger.HandleEvent(context.Background(), env); err != nil {
		t.Fatalf("handling an imported old meeting: %v", err)
	}
	if got := e.ledger(t); got != 0 {
		t.Fatalf("a six-month-old meeting imported today wrote %d rows dated "+
			"as though it had just happened", got)
	}
}

// A mail thread is not an event that was held, whoever was on it.
//
// The ROW's kind decides, not the event payload's. The payload is a
// counterparty-influenced summary of what landed; the column is what the
// capture path wrote, and a criterion about a meeting having happened must
// rest on the second.
func TestAnEmailIsNotAMeeting(t *testing.T) {
	e := newEvidenceTriggerEnv(t, "acme-sales.example")
	activityID := e.seedMeeting(t, meetingSeed{
		linked: true, occurredAt: time.Now().Add(-time.Hour), kind: "message",
	})

	env := evidenceEnvelope("activity.captured", "activity", activityID,
		crmcontracts.PublicEventActivityCaptured{Kind: "message"})
	if err := e.trigger.HandleEvent(context.Background(), env); err != nil {
		t.Fatalf("handling an email: %v", err)
	}
	if got := e.ledger(t); got != 0 {
		t.Fatalf("an email wrote %d event_held rows", got)
	}
}

// The bus is at-least-once, so the same event arrives more than once. A second
// row would double-count one fact for every reader that counts met criteria.
func TestARedeliveredEventWritesOneRow(t *testing.T) {
	e := newEvidenceTriggerEnv(t)
	contractID := e.seedContract(t)
	env := evidenceEnvelope("contract.status_changed", "contract", contractID,
		crmcontracts.PublicEventContractStatusChanged{FromStatus: "draft", ToStatus: "active"})

	for range 3 {
		if err := e.trigger.HandleEvent(context.Background(), env); err != nil {
			t.Fatalf("a redelivery errored: %v", err)
		}
	}
	if got := e.ledger(t); got != 1 {
		t.Fatalf("three deliveries of one signature wrote %d rows", got)
	}
}

// A new consumer group starts at stream position 0, so first boot replays
// every contract this installation ever signed. Writing evidence from those
// would date today's row against whatever stage the deal sits on NOW.
func TestAReplayedBacklogEventWritesNothing(t *testing.T) {
	e := newEvidenceTriggerEnv(t)
	contractID := e.seedContract(t)
	env := evidenceEnvelope("contract.status_changed", "contract", contractID,
		crmcontracts.PublicEventContractStatusChanged{FromStatus: "draft", ToStatus: "active"})
	env.OccurredAt = time.Now().Add(-30 * 24 * time.Hour)

	if err := e.trigger.HandleEvent(context.Background(), env); err != nil {
		t.Fatalf("a replayed event errored instead of being skipped: %v", err)
	}
	if got := e.ledger(t); got != 0 {
		t.Fatalf("a month-old replayed signature wrote %d rows dated today", got)
	}
}

// An event this consumer does not care about keeps the group flowing.
func TestAnUnrelatedEventIsIgnored(t *testing.T) {
	e := newEvidenceTriggerEnv(t)
	env := evidenceEnvelope("person.updated", "person", ids.NewV7(), map[string]any{})
	if err := e.trigger.HandleEvent(context.Background(), env); err != nil {
		t.Fatalf("an unrelated event errored and would wedge the group: %v", err)
	}
	if got := e.ledger(t); got != 0 {
		t.Fatalf("an unrelated event wrote %d rows", got)
	}
}

// seedContract writes a contract bound to the fixture's deal.
func (e *evidenceTriggerEnv) seedContract(t *testing.T) ids.UUID {
	t.Helper()
	var id ids.UUID
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		company := e.SeedCompany(t, "Acme GmbH", &e.AdminUser)
		return tx.QueryRow(context.Background(), `
			INSERT INTO contract (company_id, deal_id, title, status, starts_on, captured_by)
			VALUES ($1, $2, 'Rollout agreement', 'draft', current_date, 'human:test')
			RETURNING id`, company, e.dealID.UUID).Scan(&id)
	}); err != nil {
		t.Fatalf("seeding a contract: %v", err)
	}
	return id
}

// meetingSeed is how one seeded meeting differs from the default.
type meetingSeed struct {
	linked     bool
	occurredAt time.Time
	// status is activity.meeting_status. Empty leaves it NULL, which is what a
	// meeting logged without one carries.
	status string
	// transcript is the activity body, with source_system = 'transcript'.
	transcript string
	// kind is activity.kind. Empty means meeting, which is what most of these
	// tests are about; "message" seeds the ordinary mail a criterion settled
	// in prose arrives on.
	kind string
}

// seedMeeting writes a meeting in the shape CAPTURE ACTUALLY PRODUCES.
//
// OUR OWN SEAT IS THE `from` PARTICIPANT, which is the detail that matters:
// capture stamps the connected calendar's owner as the sender of every meeting
// it syncs, whoever called it. An earlier version of this fixture wrote the
// buyer as `organizer` with no `from` row at all, and passed a rule that no
// real meeting could satisfy — a test supplying its own version of production
// proves nothing about production.
func (e *evidenceTriggerEnv) seedMeeting(t *testing.T, seed meetingSeed) ids.UUID {
	t.Helper()
	var id ids.UUID
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		ctx := context.Background()
		var status, sourceSystem, body *string
		if seed.status != "" {
			status = &seed.status
		}
		if seed.transcript != "" {
			marker := deals.TranscriptSourceSystem
			sourceSystem, body = &marker, &seed.transcript
		}
		kind := seed.kind
		if kind == "" {
			kind = "meeting"
		}
		// activity_message_has_provider: a message row must name its channel,
		// and a meeting must not.
		var provider *string
		if kind == "message" {
			// One of the two providers the baseline seeds; the column is a
			// foreign key, so an unseeded name is refused.
			telegram := "telegram"
			provider = &telegram
		}
		if err := tx.QueryRow(ctx, `
			INSERT INTO activity (kind, subject, occurred_at, source, captured_by,
			                      meeting_status, source_system, body, channel_provider)
			VALUES ($5, 'Demo', $1, 'manual', 'human:test', $2, $3, $4, $6)
			RETURNING id`, seed.occurredAt, status, sourceSystem, body, kind, provider).Scan(&id); err != nil {
			return err
		}
		// Capture's own convention: the connected seat is stamped `from`.
		if _, err := tx.Exec(ctx, `
			INSERT INTO activity_participant (activity_id, user_id, role)
			VALUES ($1, $2, 'from')`, id, e.AdminUser); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO activity_participant (activity_id, address, role)
			VALUES ($1, 'ines@customer.example', 'attendee')`, id); err != nil {
			return err
		}
		if !seed.linked {
			return nil
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO activity_link (activity_id, entity_type, deal_id)
			VALUES ($1, 'deal', $2)`, id, e.dealID.UUID)
		return err
	}); err != nil {
		t.Fatalf("seeding a meeting: %v", err)
	}
	return id
}

// onlyOurSeatsAttend removes every participant who is not one of our seats,
// leaving a meeting we held with ourselves.
func (e *evidenceTriggerEnv) onlyOurSeatsAttend(t *testing.T, activityID ids.UUID) {
	t.Helper()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `
			UPDATE activity_participant SET user_id = $2, address = NULL
			 WHERE activity_id = $1 AND user_id IS NULL`, activityID, e.AdminUser)
		return err
	}); err != nil {
		t.Fatalf("making the meeting internal: %v", err)
	}
}

// markHeld stamps the meeting held, the way a rep does after it happens.
func (e *evidenceTriggerEnv) markHeld(t *testing.T, activityID ids.UUID) {
	t.Helper()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(),
			`UPDATE activity SET meeting_status = 'held' WHERE id = $1`, activityID)
		return err
	}); err != nil {
		t.Fatalf("marking the meeting held: %v", err)
	}
}

// replaceAttendeeWithPersonLink rewrites the outside attendee as the manual
// logging path writes a colleague: a person link with no address and no seat.
func (e *evidenceTriggerEnv) replaceAttendeeWithPersonLink(t *testing.T, activityID ids.UUID) {
	t.Helper()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		ctx := context.Background()
		var personID ids.UUID
		if err := tx.QueryRow(ctx, `
			INSERT INTO person (first_name, last_name, full_name, source, captured_by)
			VALUES ('Kim', 'Colleague', 'Kim Colleague', 'manual', 'human:test')
			RETURNING id`).Scan(&personID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `
			UPDATE activity_participant SET address = NULL, person_id = $2
			 WHERE activity_id = $1 AND user_id IS NULL`, activityID, personID)
		return err
	}); err != nil {
		t.Fatalf("rewriting the attendee as a person link: %v", err)
	}
}

// setAttendeeAddress puts the outside attendee on a given address.
func (e *evidenceTriggerEnv) setAttendeeAddress(t *testing.T, activityID ids.UUID, address string) {
	t.Helper()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `
			UPDATE activity_participant SET address = $2
			 WHERE activity_id = $1 AND user_id IS NULL`, activityID, address)
		return err
	}); err != nil {
		t.Fatalf("setting the attendee's address: %v", err)
	}
}

// citedLines answers the source_lines the one claim in the ledger carries.
func (e *evidenceTriggerEnv) citedLines(t *testing.T) []int32 {
	t.Helper()
	var lines []int32
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT source_lines FROM deal_stage_evidence WHERE deal_id = $1`,
			e.dealID.UUID).Scan(&lines)
	}); err != nil {
		t.Fatalf("reading the claim's cited lines: %v", err)
	}
	return lines
}

// assertClaim checks the one row the ledger holds names the right source and
// the deterministic writer, and carries no confidence.
func (e *evidenceTriggerEnv) assertClaim(t *testing.T, sourceType, extractedBy string) {
	t.Helper()
	var gotSource, gotBy string
	var confidence *float64
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `
			SELECT source_type, extracted_by, confidence
			  FROM deal_stage_evidence WHERE deal_id = $1`,
			e.dealID.UUID).Scan(&gotSource, &gotBy, &confidence)
	}); err != nil {
		t.Fatalf("reading the claim: %v", err)
	}
	if gotSource != sourceType {
		t.Errorf("the claim cites %q, not %q", gotSource, sourceType)
	}
	if gotBy != extractedBy {
		t.Errorf("the claim names writer %q, not %q", gotBy, extractedBy)
	}
	if confidence != nil {
		t.Errorf("a deterministic claim carries confidence %v; a record either says the thing or it does not", *confidence)
	}
}

// recordingEnqueuer captures what the trigger queued, so a test can assert the
// reading was asked for without standing a River worker up.
type recordingEnqueuer struct{ queued []StageEvidenceReadArgs }

func (r *recordingEnqueuer) EnqueueTx(
	_ context.Context, _ pgx.Tx, args river.JobArgs, _ *river.InsertOpts,
) error {
	read, ok := args.(StageEvidenceReadArgs)
	if !ok {
		return fmt.Errorf("the trigger queued %T, not a stage evidence reading", args)
	}
	r.queued = append(r.queued, read)
	return nil
}

// The model's half is asked about EVERY activity that reaches a deal, not only
// the meetings the deterministic arm settles. A criterion like "the buyer
// stated the problem in their own words" is settled in an ordinary email, and
// the deterministic writers have nothing to say about one.
func TestTheReadingIsQueuedForEveryActivityOnADeal(t *testing.T) {
	e := newEvidenceTriggerEnv(t, "acme-sales.example")
	queue := &recordingEnqueuer{}
	e.trigger.read = queue

	// An ordinary MESSAGE, not a meeting: nothing the deterministic arm can
	// settle, and the shape a criterion settled in prose actually arrives on.
	activityID := e.seedMeeting(t, meetingSeed{
		linked: true, occurredAt: time.Now().Add(-time.Hour), kind: "message",
	})
	env := evidenceEnvelope("activity.captured", "activity", activityID,
		crmcontracts.PublicEventActivityCaptured{Kind: "message"})
	if err := e.trigger.HandleEvent(context.Background(), env); err != nil {
		t.Fatalf("handling the activity: %v", err)
	}
	if got := e.ledger(t); got != 0 {
		t.Fatalf("the deterministic arm wrote %d rows for an activity that "+
			"settles nothing; this test would not be about the reading", got)
	}
	if len(queue.queued) != 1 {
		t.Fatalf("the trigger queued %d readings, want one", len(queue.queued))
	}
	if queue.queued[0].ActivityID != activityID {
		t.Errorf("the reading names activity %s, want %s",
			queue.queued[0].ActivityID, activityID)
	}
	if queue.queued[0].DealID != e.dealID.UUID {
		t.Errorf("the reading names deal %s, want %s",
			queue.queued[0].DealID, e.dealID.UUID)
	}
	// THE WORKSPACE, asserted because it is the field that was wrong: a bus
	// envelope carries none and the subscriber binds none, so reading it off
	// the context answered the zero id — and the worker refuses those before
	// doing anything. Every reading was silently discarded and every other
	// assertion here still passed.
	if queue.queued[0].Workspace != e.WS {
		t.Errorf("the reading names workspace %s, want %s; a zero id is refused "+
			"by the worker and the activity is never read",
			queue.queued[0].Workspace, e.WS)
	}
}

// An activity reaching no deal has no criteria to be read against, so nothing
// is queued — a reading of it would ask about a stage that does not exist.
func TestNoReadingIsQueuedForAnUnlinkedActivity(t *testing.T) {
	e := newEvidenceTriggerEnv(t, "acme-sales.example")
	queue := &recordingEnqueuer{}
	e.trigger.read = queue

	activityID := e.seedMeeting(t, meetingSeed{occurredAt: time.Now().Add(-time.Hour)})
	env := evidenceEnvelope("activity.captured", "activity", activityID,
		crmcontracts.PublicEventActivityCaptured{Kind: "meeting"})
	if err := e.trigger.HandleEvent(context.Background(), env); err != nil {
		t.Fatalf("handling an unlinked activity: %v", err)
	}
	if len(queue.queued) != 0 {
		t.Fatalf("the trigger queued %d readings for an activity on no deal", len(queue.queued))
	}
}

// An installation with no model lane composes a nil enqueuer, and the
// deterministic evidence must still be written. River discards a job whose
// kind no worker claims, so queueing one there would turn every activity into
// a discarded row.
func TestWithNoReadingLaneTheDeterministicEvidenceStillLands(t *testing.T) {
	e := newEvidenceTriggerEnv(t, "acme-sales.example")
	// The env's own trigger already carries a nil enqueuer, which is the
	// composition under test.
	activityID := e.seedMeeting(t, meetingSeed{
		linked: true, occurredAt: time.Now().Add(-time.Hour), status: "held",
	})
	env := evidenceEnvelope("activity.captured", "activity", activityID,
		crmcontracts.PublicEventActivityCaptured{Kind: "meeting"})
	if err := e.trigger.HandleEvent(context.Background(), env); err != nil {
		t.Fatalf("handling a meeting with no reading lane: %v", err)
	}
	if got := e.ledger(t); got != 1 {
		t.Fatalf("a held meeting wrote %d rows without a model lane; the "+
			"deterministic half does not depend on one", got)
	}
}
