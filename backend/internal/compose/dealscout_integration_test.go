// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// Deal Scout against a real database. Every piece of evidence is written by
// the product's own writers: the activities store logs meetings and mail, the
// capture writer files documents, the signals module records signals, and the
// contacts store plants employment.

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/signals"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/extraction"
)

type scoutEnv struct {
	*integration.Env
	pipeline ids.PipelineID
	open     ids.StageID
	won      ids.StageID
	now      time.Time
}

func setupScout(t *testing.T) *scoutEnv {
	t.Helper()
	e := integration.Setup(t)
	pipeline, open, won := integration.DealFixture(t, e)
	return &scoutEnv{Env: e, pipeline: pipeline, open: open, won: won, now: time.Now()}
}

// system is the scout's own principal on this workspace.
func (e *scoutEnv) system() context.Context {
	return principal.SystemActing(principal.WithWorkspaceID(context.Background(), e.WS), "agent:deal-scout")
}

// pass runs one scout pass at the env's clock.
func (e *scoutEnv) pass(t *testing.T) DealScoutPass {
	t.Helper()
	ctx := e.system()
	var pass DealScoutPass
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		var err error
		pass, err = RunDealScout(ctx, tx, e.now)
		return err
	}); err != nil {
		t.Fatalf("the scout pass: %v", err)
	}
	return pass
}

// employee creates a contact working at the company now.
func (e *scoutEnv) employee(t *testing.T, name string, company ids.UUID) ids.UUID {
	t.Helper()
	contact := e.SeedContact(t, name, nil)
	contactID, employer := ids.From[ids.ContactKind](contact), ids.From[ids.CompanyKind](company)
	if _, err := e.Contacts.CreateRelationship(e.Admin(), contacts.CreateRelationshipInput{
		Kind: "employment", ContactID: &contactID, CompanyID: &employer, Source: "manual",
	}); err != nil {
		t.Fatalf("employing %s: %v", name, err)
	}
	return contact
}

// meeting logs a held meeting with the contact, as the seat in ctx.
func (e *scoutEnv) meeting(ctx context.Context, t *testing.T, subject string, contact *ids.UUID, at time.Time) ids.UUID {
	t.Helper()
	held := "held"
	in := activities.LogActivityInput{
		Kind: "meeting", Subject: &subject, OccurredAt: &at, MeetingStatus: &held, Source: "manual",
	}
	if contact != nil {
		in.Links = []activities.ActivityLinkInput{{EntityType: "contact", EntityID: *contact}}
	}
	logged, _, err := e.Activities.LogActivity(ctx, in)
	if err != nil {
		t.Fatalf("logging the meeting %q: %v", subject, err)
	}
	return ids.UUID(logged.Id)
}

// email logs a mail on the company in the given direction.
func (e *scoutEnv) email(t *testing.T, subject, direction string, company ids.UUID, at time.Time) ids.UUID {
	t.Helper()
	logged, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "email", Subject: &subject, Direction: &direction, OccurredAt: &at, Source: "manual",
		Links: []activities.ActivityLinkInput{{EntityType: "company", EntityID: company}},
	})
	if err != nil {
		t.Fatalf("logging the mail %q: %v", subject, err)
	}
	return ids.UUID(logged.Id)
}

// document files one captured part on the mail, through capture's own writer.
func (e *scoutEnv) document(t *testing.T, mail ids.UUID, filename, contentType string) ids.UUID {
	t.Helper()
	ctx := principal.WithCorrelationID(principal.WithActor(principal.WithWorkspaceID(context.Background(), e.WS),
		principal.Principal{
			Type: principal.PrincipalConnector, ID: "connector:imap",
			Permissions: principal.Permissions{
				Objects:  map[string]principal.ObjectGrant{"activity": {Create: true}},
				RowScope: principal.RowScopeAll,
			},
		}), ids.NewV7())
	store := activities.NewStore(InstallationDB(e.Pool)).WithBlobstore(blobstore.NewMemory())
	staged, err := store.StageCapturedFiles(ctx, []activities.CapturedFile{{
		PartID: filename, Filename: filename, ContentType: contentType, Body: []byte(filename),
	}})
	if err != nil {
		t.Fatalf("staging %s: %v", filename, err)
	}
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		return store.RecordCapturedFiles(ctx, tx, ids.From[ids.ActivityKind](mail), activities.CapturedFileSource{
			System: "imap", MessageID: "m-" + mail.String(), CapturedBy: "connector:imap", Category: "email_attachment",
		}, staged)
	}); err != nil {
		t.Fatalf("recording %s: %v", filename, err)
	}
	var id ids.UUID
	if err := e.Pool.QueryRow(context.Background(),
		`SELECT id FROM attachment WHERE activity_id = $1 AND filename = $2`, mail, filename).Scan(&id); err != nil {
		t.Fatalf("reading back %s: %v", filename, err)
	}
	return id
}

// reading finishes a document reading through the extraction writer.
func (e *scoutEnv) reading(t *testing.T, attachment ids.UUID, fields ...extraction.ExtractedField) {
	t.Helper()
	read, _, err := e.Activities.StartExtractionReadQueued(e.Admin(), attachment, "human:"+e.AdminUser.String(), nil)
	if err != nil {
		t.Fatalf("starting the reading: %v", err)
	}
	claimed, err := e.Activities.BeginExtractionRead(e.Admin(), read.ID, time.Hour)
	if err != nil {
		t.Fatalf("claiming the reading: %v", err)
	}
	if err := e.Activities.FinishExtractionRead(e.Admin(), read.ID, activities.ExtractionReadOutcome{
		Status: activities.ExtractionReadDone, ClaimedAt: *claimed.StartedAt, Fields: fields, Detail: "read",
	}); err != nil {
		t.Fatalf("finishing the reading: %v", err)
	}
}

// signal records one derived signal on the company citing the mail.
func (e *scoutEnv) signal(t *testing.T, kind string, company, cited ids.UUID, at time.Time, privateTo ids.UUID) ids.UUID {
	t.Helper()
	ctx := principal.SystemActing(principal.WithWorkspaceID(context.Background(), e.WS), "agent:signal-scan")
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		_, err := signals.RecordDerived(ctx, tx, signals.DerivedSignal{
			Kind: kind, CompanyID: company, Summary: kind + " on the account", Severity: "info",
			Fingerprint: kind + ":" + cited.String(),
			Evidence:    []signals.DerivedEvidence{{Snippet: kind, ActivityID: cited}},
			PrivateTo:   privateTo,
		}, at)
		return err
	}); err != nil {
		t.Fatalf("recording %s: %v", kind, err)
	}
	var id ids.UUID
	if err := e.Pool.QueryRow(context.Background(),
		`SELECT id FROM signal WHERE fingerprint = $1`, kind+":"+cited.String()).Scan(&id); err != nil {
		t.Fatalf("reading back %s: %v", kind, err)
	}
	return id
}

// suggestions lists what the reader in ctx is shown for the company.
func (e *scoutEnv) suggestions(ctx context.Context, t *testing.T, company ids.UUID) []deals.Suggestion {
	t.Helper()
	list, _, err := e.Deals.ListSuggestions(ctx, deals.SuggestionQuery{CompanyID: &company})
	if err != nil {
		t.Fatalf("listing suggestions: %v", err)
	}
	return list
}

// stored counts suggestions about the company in a state, whoever may see them.
func (e *scoutEnv) stored(t *testing.T, company ids.UUID, state string) int {
	t.Helper()
	return e.WsCount(t, `SELECT count(*) FROM deal_suggestion WHERE company_id = $1 AND state = $2`, company, state)
}

func (e *scoutEnv) daysAgo(n int) time.Time { return e.now.AddDate(0, 0, -n) }

// signalReader is an admin who also holds the signal grant a suggestion citing
// signals needs.
func (e *scoutEnv) signalReader() context.Context {
	return e.As(e.AdminUser, nil, integration.AdminWithSignals)
}

func TestAHeldMeetingWithSomebodyAtACompanySuggestsOneDeal(t *testing.T) {
	e := setupScout(t)
	acme := e.SeedCompany(t, "Acme GmbH", nil)
	dana := e.employee(t, "Dana Buyer", acme)
	meeting := e.meeting(e.Admin(), t, "Scoping workshop", &dana, e.daysAgo(3))

	if pass := e.pass(t); pass.Raised != 1 {
		t.Fatalf("first pass raised %d, want 1", pass.Raised)
	}
	e.pass(t)
	shown := e.suggestions(e.Admin(), t, acme)
	if len(shown) != 1 {
		t.Fatalf("after two passes the company shows %d suggestions, want exactly 1", len(shown))
	}
	got := shown[0]
	if got.NameHint != deals.HintMeetingHeld || got.CompanyName != "Acme GmbH" || got.AmountMinor != nil || len(got.Evidence) != 1 {
		t.Fatalf("suggestion = %+v, want Acme with the meeting hint, no amount, one piece of evidence", got)
	}
	if ev := got.Evidence[0]; ev.Kind != deals.EvidenceMeeting || *ev.ActivityID != meeting || ev.Title != "Scoping workshop" {
		t.Fatalf("evidence = %+v, want the meeting by its subject", ev)
	}
}

func TestAMeetingAmongColleaguesOrWithTheOwnCompanySuggestsNothing(t *testing.T) {
	e := setupScout(t)
	own, err := e.Contacts.SaveCompany(e.Admin(), contacts.SaveCompanyInput{DisplayName: "Our Company"})
	if err != nil {
		t.Fatalf("saving the installation's own company: %v", err)
	}
	colleague := e.employee(t, "Kim Colleague", own.CompanyID.UUID)
	e.meeting(e.Admin(), t, "Weekly sync", nil, e.daysAgo(2))
	e.meeting(e.Admin(), t, "Pipeline review", &colleague, e.daysAgo(2))

	if pass := e.pass(t); pass.Raised != 0 {
		t.Fatalf("a pass over internal meetings raised %d, want 0", pass.Raised)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM deal_suggestion`); n != 0 {
		t.Fatalf("%d suggestions stored, want none", n)
	}
}

func TestACompanyWithAnOpenDealIsNotSuggested(t *testing.T) {
	e := setupScout(t)
	acme := e.SeedCompany(t, "Acme GmbH", nil)
	dana := e.employee(t, "Dana Buyer", acme)
	e.meeting(e.Admin(), t, "Scoping workshop", &dana, e.daysAgo(3))
	company := ids.From[ids.CompanyKind](acme)
	if _, err := e.Deals.CreateDeal(e.Admin(), deals.CreateDealInput{
		Name: "Acme platform", PipelineID: e.pipeline, StageID: e.open, CompanyID: &company, Source: "manual",
	}); err != nil {
		t.Fatalf("opening the deal: %v", err)
	}

	e.pass(t)
	if n := e.stored(t, acme, deals.SuggestionOpen); n != 0 {
		t.Fatalf("a company with an open deal got %d suggestions, want none", n)
	}
}

func TestADealClosedAfterTheEvidenceKeepsItFromBeingSuggested(t *testing.T) {
	e := setupScout(t)
	acme := e.SeedCompany(t, "Acme GmbH", nil)
	dana := e.employee(t, "Dana Buyer", acme)
	e.meeting(e.Admin(), t, "Scoping workshop", &dana, e.daysAgo(3))
	company := ids.From[ids.CompanyKind](acme)
	deal, err := e.Deals.CreateDeal(e.Admin(), deals.CreateDealInput{
		Name: "Acme platform", PipelineID: e.pipeline, StageID: e.open, CompanyID: &company, Source: "manual",
	})
	if err != nil {
		t.Fatalf("opening the deal: %v", err)
	}
	reason := "verbal"
	if _, err := e.Deals.AdvanceDeal(e.Admin(), ids.From[ids.DealKind](ids.UUID(deal.Id)), deals.AdvanceDealInput{
		ToStageID: e.won, WonWithoutContractReason: &reason,
	}); err != nil {
		t.Fatalf("winning the deal: %v", err)
	}

	e.pass(t)
	if n := e.stored(t, acme, deals.SuggestionOpen); n != 0 {
		t.Fatalf("evidence from before the win raised %d suggestions, want none", n)
	}
	later := e.employee(t, "Lee Buyer", acme)
	e.now = e.now.Add(time.Hour)
	e.meeting(e.Admin(), t, "Phase two", &later, e.now.Add(-time.Minute))
	e.pass(t)
	if n := e.stored(t, acme, deals.SuggestionOpen); n != 1 {
		t.Fatalf("a meeting after the win raised %d suggestions, want 1", n)
	}
}

func TestTheTwoSignalsWithinThirtyDaysSuggestADeal(t *testing.T) {
	e := setupScout(t)
	acme := e.SeedCompany(t, "Acme GmbH", nil)
	globex := e.SeedCompany(t, "Globex", nil)
	initech := e.SeedCompany(t, "Initech", nil)
	mailAt := e.daysAgo(20)

	// Acme: both kinds, ten days apart.
	acmeMail := e.email(t, "Next steps", "inbound", acme, mailAt)
	e.signal(t, "new_opportunity", acme, acmeMail, e.daysAgo(15), ids.Nil)
	e.signal(t, "commitment_made", acme, e.email(t, "Confirmed", "inbound", acme, mailAt), e.daysAgo(5), ids.Nil)
	// Globex: only one of the two.
	e.signal(t, "new_opportunity", globex, e.email(t, "Idea", "inbound", globex, mailAt), e.daysAgo(5), ids.Nil)
	// Initech: both, but the commitment is private to one seat.
	e.signal(t, "new_opportunity", initech, e.email(t, "Idea", "inbound", initech, mailAt), e.daysAgo(5), ids.Nil)
	e.signal(t, "commitment_made", initech, e.email(t, "Yes", "inbound", initech, mailAt), e.daysAgo(4), e.Rep1)

	e.pass(t)
	shown := e.suggestions(e.signalReader(), t, acme)
	if len(shown) != 1 || len(shown[0].Evidence) != 2 {
		t.Fatalf("Acme shows %+v, want one suggestion citing both signals", shown)
	}
	if n := e.stored(t, globex, deals.SuggestionOpen); n != 0 {
		t.Errorf("one signal alone raised %d suggestions, want none", n)
	}
	if n := e.stored(t, initech, deals.SuggestionOpen); n != 0 {
		t.Errorf("a pair relying on an owner-private signal raised %d suggestions, want none", n)
	}
}

func TestSignalsFurtherApartThanThirtyDaysSuggestNothing(t *testing.T) {
	e := setupScout(t)
	acme := e.SeedCompany(t, "Acme GmbH", nil)
	mail := e.email(t, "Next steps", "inbound", acme, e.daysAgo(80))
	e.signal(t, "new_opportunity", acme, mail, e.daysAgo(75), ids.Nil)
	e.signal(t, "commitment_made", acme, e.email(t, "Yes", "inbound", acme, e.daysAgo(80)), e.daysAgo(5), ids.Nil)

	e.pass(t)
	if n := e.stored(t, acme, deals.SuggestionOpen); n != 0 {
		t.Fatalf("signals seventy days apart raised %d suggestions, want none", n)
	}
}

func TestASentProposalSuggestsADeal(t *testing.T) {
	e := setupScout(t)
	acme := e.SeedCompany(t, "Acme GmbH", nil)
	inbound := e.SeedCompany(t, "Inbound AG", nil)
	image := e.SeedCompany(t, "Image AG", nil)
	news := e.SeedCompany(t, "News AG", nil)
	const pdf = "application/pdf"
	proposal := e.document(t, e.email(t, "Unser Angebot", "outbound", acme, e.daysAgo(2)), "Angebot_2026.pdf", pdf)
	e.document(t, e.email(t, "Their offer", "inbound", inbound, e.daysAgo(2)), "Angebot_2026.pdf", pdf)
	e.document(t, e.email(t, "Logo", "outbound", image, e.daysAgo(2)), "Angebot.png", "image/png")
	e.document(t, e.email(t, "Newsletter", "outbound", news, e.daysAgo(2)), "Newsletter_Angebote.pdf", pdf)

	e.pass(t)
	shown := e.suggestions(e.Admin(), t, acme)
	if len(shown) != 1 || shown[0].NameHint != deals.HintProposalSent {
		t.Fatalf("Acme shows %+v, want one suggestion led by the proposal", shown)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM deal_suggestion s
		  LEFT JOIN deal_suggestion_evidence ev ON ev.suggestion_id = s.id
		 WHERE row_to_json(s)::text ILIKE '%angebot%' OR row_to_json(ev)::text ILIKE '%angebot%'`); n != 0 {
		t.Fatalf("%d stored rows repeat the file name; a suggestion stores no text from the evidence", n)
	}
	if ev := shown[0].Evidence; len(ev) != 1 || *ev[0].AttachmentID != proposal {
		t.Fatalf("evidence = %+v, want the proposal document", ev)
	}
	for name, company := range map[string]ids.UUID{"inbound": inbound, "an image": image, "a newsletter": news} {
		if n := e.stored(t, company, deals.SuggestionOpen); n != 0 {
			t.Errorf("%s raised %d suggestions, want none", name, n)
		}
	}
}

func TestAProposedAmountNeedsBothTheAmountAndTheCurrency(t *testing.T) {
	e := setupScout(t)
	priced := e.SeedCompany(t, "Priced GmbH", nil)
	bare := e.SeedCompany(t, "Bare GmbH", nil)
	const pdf = "application/pdf"
	full := e.document(t, e.email(t, "Angebot", "outbound", priced, e.daysAgo(2)), "Angebot.pdf", pdf)
	half := e.document(t, e.email(t, "Angebot", "outbound", bare, e.daysAgo(2)), "Angebot.pdf", pdf)
	e.reading(t, full,
		extraction.ExtractedField{Field: "amount_minor", Value: "1250000", Confidence: "high"},
		extraction.ExtractedField{Field: "currency", Value: "EUR", Confidence: "high"})
	e.reading(t, half,
		extraction.ExtractedField{Field: "amount_minor", Value: "990000", Confidence: "high"},
		extraction.ExtractedField{Field: "currency", Omitted: true, OmittedReason: "not_found"})

	e.pass(t)
	got := e.suggestions(e.Admin(), t, priced)
	if len(got) != 1 || got[0].AmountMinor == nil || *got[0].AmountMinor != 1250000 || *got[0].Currency != "EUR" {
		t.Fatalf("the priced proposal suggested %+v, want 12,500.00 EUR", got)
	}
	unpriced := e.suggestions(e.Admin(), t, bare)
	if len(unpriced) != 1 || unpriced[0].AmountMinor != nil || unpriced[0].Currency != nil {
		t.Fatalf("an amount without a currency suggested %+v, want a suggestion with no amount", unpriced)
	}
}

func TestEvidenceFromAWithheldConversationIsIgnored(t *testing.T) {
	e := setupScout(t)
	limited := e.SeedCompany(t, "Limited GmbH", nil)
	dana := e.employee(t, "Dana Buyer", limited)
	meeting := e.meeting(e.Admin(), t, "Confidential call", &dana, e.daysAgo(3))
	if _, err := e.Activities.SetAudience(e.Admin(), ids.From[ids.ActivityKind](meeting),
		activities.SetAudienceInput{Audience: "participants"}); err != nil {
		t.Fatalf("limiting the meeting: %v", err)
	}
	// The mail's own audience stays the workspace's: only the thread's hold
	// says it is withheld, which is the case a check on the audience alone misses.
	held := e.SeedCompany(t, "Held GmbH", nil)
	subject, outbound, at := "Angebot", "outbound", e.daysAgo(2)
	heldMail, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "email", Subject: &subject, Direction: &outbound, OccurredAt: &at, Source: "manual",
		ThreadKey: "held-thread", Links: []activities.ActivityLinkInput{{EntityType: "company", EntityID: held}},
	})
	if err != nil {
		t.Fatalf("logging the held mail: %v", err)
	}
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return capture.NewThreadVerdictStore(InstallationDB(e.Pool)).DecideAsOwner(e.Admin(), tx, "held-thread", false)
	}); err != nil {
		t.Fatalf("holding the thread: %v", err)
	}
	e.document(t, ids.UUID(heldMail.Id), "Angebot.pdf", "application/pdf")

	e.pass(t)
	if n := e.WsCount(t, `SELECT count(*) FROM deal_suggestion`); n != 0 {
		t.Fatalf("withheld evidence raised %d suggestions, want none", n)
	}
}
