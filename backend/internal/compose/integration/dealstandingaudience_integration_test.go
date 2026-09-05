// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A cached STANDING, read after the message it was written from was narrowed.
//
// The sibling suite beside this one (dealmoveaudience) proves the same rule for
// the deal's next STEP, and the two hazards are not the same one twice. A move
// names an activity id, so serving it says a message exists. A standing names
// nothing — it is a sentence — and the first version of this read shipped an
// argument that this made it safe to serve ungated.
//
// That argument is wrong, and this suite is the reason it cannot come back. The
// sentence is written BY A MODEL FROM the timeline, and the card's grounding
// requires every sentence to cite the records it rests on, so its text restates
// what those records say. auth's rule is about content derived from an activity,
// not about ids: everything so derived carries the audience predicate wherever
// it is served.
//
// The deal page does not have this problem, which is why it needs no such gate:
// Service.Get re-gathers the timeline under the caller's CURRENT grants and
// fingerprints the card against it, so a narrowed message changes the input and
// the card is rewritten without it. This read has no fingerprint to miss — that
// is what makes it cheap enough for a page of thirty rows — so it asks the
// audience question directly instead.
//
// AT THE DATABASE rather than over a stub, because the predicate is SQL. A unit
// test can prove that allReadable refuses a missing id; only Postgres can prove
// that auth.ActivityContentClause puts this reader outside the audience.

import (
	"context"
	"encoding/json"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/dealstatus"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestACachedStandingStopsRestatingAMessageTheReaderMayNoLongerRead(t *testing.T) {
	e := SetupSearch(t)
	ctx := context.Background()

	pipeline := e.SeedID(t, `INSERT INTO pipeline (id, name, is_default, position)
		VALUES ($1, 'Sales', true, 0)`)
	stage := e.SeedID(t, `INSERT INTO stage (id, pipeline_id, name, position, semantic, win_probability)
		VALUES ($1, $2, 'Qualify', 0, 'open', 10)`, pipeline)
	deal := e.SeedID(t, `INSERT INTO deal (id, owner_id, name, pipeline_id, stage_id, source, captured_by)
		VALUES ($1, $2, 'Turbinenbau Renewal', $3, $4, 'manual', 'human:x')`,
		e.Rep1, pipeline, stage)
	// Workspace audience to begin with: the state the card was written under.
	mail := e.SeedID(t, `
		INSERT INTO activity (id, kind, subject, body, occurred_at, direction, source, captured_by, audience)
		VALUES ($1, 'email', 'Vertragsentwurf', 'die Rechtsabteilung prüft noch', now(), 'inbound',
		        'gmail', 'connector:gmail:x', 'workspace')`)

	reader := dealReader(e)
	svc := dealstatus.NewService(e.Pool, nil, nil, nil, nil)
	seedCachedStanding(t, e, readerOf(reader, t), deal, mail,
		"blocked", "Legal has not returned the contract they promised on Tuesday.")

	// While the reader is in the audience the standing is served. Asserting it
	// FIRST is what makes the refusal below mean something rather than passing
	// because nothing was ever there.
	before, err := svc.CachedCards(reader, []ids.UUID{deal})
	if err != nil {
		t.Fatalf("reading the cached standings: %v", err)
	}
	if before[deal].DecisiveLine == "" {
		t.Fatalf("the reader in the audience got %+v, wanted the standing its card was written with — "+
			"without this the refusal below proves nothing", before[deal])
	}

	// The narrowing. `participants` is the humans on the message, and this
	// reader is not one: they were reading it as a workspace member.
	if _, err := e.Owner.Exec(ctx,
		`UPDATE activity SET audience = 'participants' WHERE id = $1`, mail); err != nil {
		t.Fatal(err)
	}

	after, err := svc.CachedCards(reader, []ids.UUID{deal})
	if err != nil {
		t.Fatalf("reading the cached standings after the narrowing: %v", err)
	}
	if card, present := after[deal]; present {
		t.Fatalf("the standing survived the narrowing as %+v — its sentence restates what a message "+
			"this reader is now outside the audience of said", card)
	}
}

// The OTHER half of the same admission, and it is a separate loss.
//
// A seat can keep `deal.read` and lose `activity.read` outright. The audience
// predicate alone would not stop this: that clause renders a ROW filter, and a
// row filter answers "which of them", never "may this seat read any".
func TestACachedStandingStopsWhenTheSeatLosesActivitiesEntirely(t *testing.T) {
	e := SetupSearch(t)

	pipeline := e.SeedID(t, `INSERT INTO pipeline (id, name, is_default, position)
		VALUES ($1, 'Sales', true, 0)`)
	stage := e.SeedID(t, `INSERT INTO stage (id, pipeline_id, name, position, semantic, win_probability)
		VALUES ($1, $2, 'Qualify', 0, 'open', 10)`, pipeline)
	deal := e.SeedID(t, `INSERT INTO deal (id, owner_id, name, pipeline_id, stage_id, source, captured_by)
		VALUES ($1, $2, 'Turbinenbau Renewal', $3, $4, 'manual', 'human:x')`,
		e.Rep1, pipeline, stage)
	// Workspace audience throughout: the audience is NOT what changes here, so a
	// fix that only closed the audience hole leaves this test failing.
	mail := e.SeedID(t, `
		INSERT INTO activity (id, kind, subject, body, occurred_at, direction, source, captured_by, audience)
		VALUES ($1, 'email', 'Vertragsentwurf', 'die Rechtsabteilung prüft noch', now(), 'inbound',
		        'gmail', 'connector:gmail:x', 'workspace')`)

	svc := dealstatus.NewService(e.Pool, nil, nil, nil, nil)
	granted := dealReader(e)
	seedCachedStanding(t, e, readerOf(granted, t), deal, mail,
		"blocked", "Legal has not returned the contract they promised on Tuesday.")

	before, err := svc.CachedCards(granted, []ids.UUID{deal})
	if err != nil {
		t.Fatalf("reading the cached standings: %v", err)
	}
	if before[deal].DecisiveLine == "" {
		t.Fatalf("the granted seat got %+v, wanted its standing — without this the refusal below "+
			"proves nothing", before[deal])
	}

	// The same reader, the same card, one grant fewer.
	revoked := dealReaderWithoutActivities(e, readerOf(granted, t))
	after, err := svc.CachedCards(revoked, []ids.UUID{deal})
	if err != nil {
		t.Fatalf("reading the cached standings without the activity grant: %v", err)
	}
	if card, present := after[deal]; present {
		t.Fatalf("a seat that may not read activities at all was served %+v — the audience clause "+
			"is a row filter and answers no grant question", card)
	}
}

// A standing whose sentence cites NO message is untouched by the audience,
// because there is no correspondence behind it to judge.
//
// Without this case the fix could be "drop every cached standing" and both tests
// above would still pass — the feature gone, and the leak closed by deleting it.
func TestACachedStandingCitingNoMessageIsUnaffectedByAnAudience(t *testing.T) {
	e := SetupSearch(t)

	pipeline := e.SeedID(t, `INSERT INTO pipeline (id, name, is_default, position)
		VALUES ($1, 'Sales', true, 0)`)
	stage := e.SeedID(t, `INSERT INTO stage (id, pipeline_id, name, position, semantic, win_probability)
		VALUES ($1, $2, 'Qualify', 0, 'open', 10)`, pipeline)
	deal := e.SeedID(t, `INSERT INTO deal (id, owner_id, name, pipeline_id, stage_id, source, captured_by)
		VALUES ($1, $2, 'Turbinenbau Renewal', $3, $4, 'manual', 'human:x')`,
		e.Rep1, pipeline, stage)
	// A narrowed message exists in the workspace, and this standing does not
	// rest on it. A gate that judged the page rather than the sentence would
	// withhold this one too.
	narrowed := e.SeedID(t, `
		INSERT INTO activity (id, kind, subject, body, occurred_at, direction, source, captured_by, audience)
		VALUES ($1, 'email', 'Intern', 'nicht fuer alle', now(), 'inbound',
		        'gmail', 'connector:gmail:x', 'participants')`)

	reader := dealReader(e)
	svc := dealstatus.NewService(e.Pool, nil, nil, nil, nil)
	// The standing rests on the DEAL's own dates, which every reader holding
	// deal.read has already reached.
	seedCachedStandingCiting(t, e, readerOf(reader, t), deal,
		"drifting", "The close date has passed twice without a new one.",
		crmcontracts.OrganizationBriefEvidence{
			EntityType: crmcontracts.OrganizationBriefEvidenceEntityTypeDeal,
			EntityId:   openapi_types.UUID(deal),
		})

	cards, err := svc.CachedCards(reader, []ids.UUID{deal})
	if err != nil {
		t.Fatalf("reading the cached standings: %v", err)
	}
	card, present := cards[deal]
	if !present {
		t.Fatalf("a standing citing no message was dropped — the audience filter has nothing to "+
			"judge here, and %s is not what it rests on", narrowed)
	}
	if card.Standing != "drifting" {
		t.Fatalf("the standing came back as %q, wanted the drifting it was stored as", card.Standing)
	}
}

// seedCachedStanding writes the card a deal page would have written for this
// reader while they could still read the message: a verdict whose sentence
// cites that message.
func seedCachedStanding(t *testing.T, e *SearchEnv, userID, deal, activity ids.UUID, standing, line string) {
	t.Helper()
	seedCachedStandingCiting(t, e, userID, deal, standing, line,
		crmcontracts.OrganizationBriefEvidence{
			EntityType: crmcontracts.OrganizationBriefEvidenceEntityTypeActivity,
			EntityId:   openapi_types.UUID(activity),
		})
}

// seedCachedStandingCiting puts one card carrying a verdict in the per-reader
// cache, through the owner connection for the reason seedCachedCard gives:
// these suites test READ semantics, and writing through the service under test
// would make the fixture depend on it.
func seedCachedStandingCiting(
	t *testing.T, e *SearchEnv, userID, deal ids.UUID,
	standing, line string, cites ...crmcontracts.OrganizationBriefEvidence,
) {
	t.Helper()
	payload, err := json.Marshal(map[string]any{
		"card": crmcontracts.DealStatusCard{
			Verdict: &crmcontracts.DealStatusCardVerdict{
				Standing: standing,
				Because: crmcontracts.DealStatusCardSection{
					Sentences: []crmcontracts.OrganizationBriefSentence{
						{Text: line, Evidence: cites},
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("encoding the cached card: %v", err)
	}
	if _, err := e.Owner.Exec(context.Background(), `
		INSERT INTO app_user (id, email, display_name)
		VALUES ($1, $2, 'Cache Reader') ON CONFLICT (id) DO NOTHING`,
		userID, userID.String()+"@example.test"); err != nil {
		t.Fatalf("seeding the reader: %v", err)
	}
	if _, err := e.Owner.Exec(context.Background(), `
		INSERT INTO deal_status_card (user_id, deal_id, fingerprint, generated_at, generated_by, payload)
		VALUES ($1, $2, 'fixture', now(), 'deterministic', $3)`,
		userID, deal, payload); err != nil {
		t.Fatalf("seeding the cached card: %v", err)
	}
}
