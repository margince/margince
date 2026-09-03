// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A cached next step, read after its message was narrowed.
//
// The deal's status card decides one move per deal and caches it per reader, and
// the worklist queue now reads that cache so a drifting deal names a step rather
// than only a problem. The cache is the hazard: it stores an admission taken
// when the card was WRITTEN, and an audience is a thing a human changes later.
//
// The sequence that leaks, and the reason this test exists at the database
// rather than over a stub:
//
//  1. A rep may read a message, so the card names it — "they wrote, nobody has
//     answered, draft the reply" — and stores that activity id.
//  2. Somebody narrows the message's audience and the rep is no longer in it.
//  3. The rep still holds deal.read, so that deal still reaches their queue
//     through the at-risk lane, which asks for the deal grant and nothing more.
//
// Serving the stored id at step 3 tells the rep the message exists and is
// unanswered, which is content derived from an activity. auth's own rule says
// everything derived from activity content carries the audience predicate, so
// the read asks again rather than trusting what it stored.
//
// This is the fifth system reader that had to learn the audience, beside the
// four in audiencederived_integration_test.go.

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/margince/margince/backend/internal/compose/dealstatus"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestACachedMoveStopsNamingAMessageTheReaderMayNoLongerRead(t *testing.T) {
	e := SetupSearch(t)
	ctx := context.Background()

	pipeline := e.SeedID(t, `INSERT INTO pipeline (id, name, is_default, position)
		VALUES ($1, 'Sales', true, 0)`)
	stage := e.SeedID(t, `INSERT INTO stage (id, pipeline_id, name, position, semantic, win_probability)
		VALUES ($1, $2, 'Qualify', 0, 'open', 10)`, pipeline)
	deal := e.SeedID(t, `INSERT INTO deal (id, owner_id, name, pipeline_id, stage_id, source, captured_by)
		VALUES ($1, $2, 'Turbinenbau Renewal', $3, $4, 'manual', 'human:x')`,
		e.Rep1, pipeline, stage)
	// Workspace audience to begin with: everybody who can discover it reads it,
	// which is the state the card was written under.
	mail := e.SeedID(t, `
		INSERT INTO activity (id, kind, subject, body, occurred_at, direction, source, captured_by, audience)
		VALUES ($1, 'email', 'Vertragsentwurf', 'bitte um Rückmeldung', now(), 'inbound',
		        'gmail', 'connector:gmail:x', 'workspace')`)

	reader := dealReader(e)
	svc := dealstatus.NewService(e.Pool, nil, nil, nil, nil)
	seedCachedMove(t, e, readerOf(reader, t), deal, mail)

	// While the reader is in the audience the move names its message — this is
	// the feature, and asserting it first is what makes the refusal below mean
	// something rather than passing because nothing was ever there.
	before, err := svc.CachedMoves(reader, []ids.UUID{deal})
	if err != nil {
		t.Fatalf("reading the cached moves: %v", err)
	}
	named, ok := dealstatus.NamedActivity(before[deal])
	if !ok || named != mail {
		t.Fatalf("the move names %v, wanted the message %s it was written about — "+
			"without this the refusal below proves nothing", before[deal].Arguments, mail)
	}

	// The narrowing. `participants` is the humans on the message, and this
	// reader is not one: they were reading it as a workspace member.
	if _, err := e.Owner.Exec(ctx,
		`UPDATE activity SET audience = 'participants' WHERE id = $1`, mail); err != nil {
		t.Fatal(err)
	}

	after, err := svc.CachedMoves(reader, []ids.UUID{deal})
	if err != nil {
		t.Fatalf("reading the cached moves after the narrowing: %v", err)
	}
	if move, present := after[deal]; present {
		t.Fatalf("the move survived the narrowing as %+v — the stored activity id tells a reader "+
			"outside the audience that the message exists and is unanswered", move)
	}
}

// The OTHER half of the same admission, and it is a separate loss.
//
// A seat can keep `deal.read` and lose `activity.read` outright — a reassignment
// to a role that reads pipeline but not correspondence. The deal still reaches
// their queue, and the audience predicate alone would not stop the cached move:
// that clause renders a ROW filter, and a row filter answers "which of them",
// never "may this seat read any". Taking a scope clause for a gate is how a
// reader with no grant at all gets served every row it would have matched.
func TestACachedMoveStopsWhenTheSeatLosesActivitiesEntirely(t *testing.T) {
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
		VALUES ($1, 'email', 'Vertragsentwurf', 'bitte um Rückmeldung', now(), 'inbound',
		        'gmail', 'connector:gmail:x', 'workspace')`)

	svc := dealstatus.NewService(e.Pool, nil, nil, nil, nil)
	// ONE cached card, read by two seats that differ in exactly one grant. The
	// card is keyed per reader, so each seat needs its own copy of it.
	granted := dealReader(e)
	seedCachedMove(t, e, readerOf(granted, t), deal, mail)

	before, err := svc.CachedMoves(granted, []ids.UUID{deal})
	if err != nil {
		t.Fatalf("reading the cached moves: %v", err)
	}
	if _, ok := dealstatus.NamedActivity(before[deal]); !ok {
		t.Fatalf("the granted seat got %+v, wanted the move naming its message — "+
			"without this the refusal below proves nothing", before[deal])
	}

	// The same reader, the same card, one grant fewer.
	revoked := dealReaderWithoutActivities(e, readerOf(granted, t))
	after, err := svc.CachedMoves(revoked, []ids.UUID{deal})
	if err != nil {
		t.Fatalf("reading the cached moves without the activity grant: %v", err)
	}
	if move, present := after[deal]; present {
		t.Fatalf("a seat that may not read activities at all was served %+v — "+
			"the audience clause is a row filter and answers no grant question", move)
	}

	// And the grant loss must narrow this read rather than empty it. A seat
	// without `activity.read` may still be told to agree a next step on a deal
	// it owns — that move names no correspondence. Refusing it too would make
	// the fix "drop every move from a seat that lost one grant".
	other := e.SeedID(t, `INSERT INTO deal (id, owner_id, name, pipeline_id, stage_id, source, captured_by)
		VALUES ($1, $2, 'Globex Renewal', $3, $4, 'manual', 'human:x')`,
		e.Rep1, pipeline, stage)
	seedCachedCard(t, e, readerOf(granted, t), other, crmcontracts.DealStatusCardMove{
		Action:    "create_task",
		Reason:    "The last contact was 30 days ago and nothing is booked.",
		Arguments: &map[string]any{"subject": "Agree the next step", "source": "ui"},
	})

	narrowed, err := svc.CachedMoves(revoked, []ids.UUID{deal, other})
	if err != nil {
		t.Fatalf("reading both cached moves without the activity grant: %v", err)
	}
	if _, present := narrowed[other]; !present {
		t.Fatal("a move naming no correspondence was dropped from a seat that lost only " +
			"`activity.read` — the grant narrows which moves survive, it does not empty the page")
	}
}

// dealReaderWithoutActivities is the same person as dealReader, one grant fewer:
// they read deals and no longer read activities at all. The user id is passed in
// because the card cache is keyed by it — a different id would be a different
// reader with no card, which would pass this test for the wrong reason.
func dealReaderWithoutActivities(e *SearchEnv, userID ids.UUID) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + ids.NewV7().String(), UserID: userID,
		SeatType: principal.SeatFull,
		Permissions: principal.Permissions{
			Objects:  map[string]principal.ObjectGrant{"deal": {Read: true}},
			RowScope: principal.RowScopeAll,
		},
	})
}

// A move that names NO record is untouched by the audience, because there is no
// activity behind it to judge. Without this case the fix could be "drop every
// cached move" and the test above would still pass — the feature would be gone
// and the leak closed by deleting the feature.
func TestACachedMoveNamingNoRecordIsUnaffectedByAnAudience(t *testing.T) {
	e := SetupSearch(t)

	pipeline := e.SeedID(t, `INSERT INTO pipeline (id, name, is_default, position)
		VALUES ($1, 'Sales', true, 0)`)
	stage := e.SeedID(t, `INSERT INTO stage (id, pipeline_id, name, position, semantic, win_probability)
		VALUES ($1, $2, 'Qualify', 0, 'open', 10)`, pipeline)
	deal := e.SeedID(t, `INSERT INTO deal (id, owner_id, name, pipeline_id, stage_id, source, captured_by)
		VALUES ($1, $2, 'Turbinenbau Renewal', $3, $4, 'manual', 'human:x')`,
		e.Rep1, pipeline, stage)

	reader := dealReader(e)
	svc := dealstatus.NewService(e.Pool, nil, nil, nil, nil)
	// The create_task shape: a subject and a link, no existing record.
	seedCachedCard(t, e, readerOf(reader, t), deal, crmcontracts.DealStatusCardMove{
		Action: "create_task",
		Reason: "The last contact was 30 days ago and nothing is booked.",
		Arguments: &map[string]any{
			"subject": "Agree the next step on Turbinenbau Renewal",
			"source":  "ui",
		},
	})

	moves, err := svc.CachedMoves(reader, []ids.UUID{deal})
	if err != nil {
		t.Fatalf("reading the cached moves: %v", err)
	}
	move, present := moves[deal]
	if !present {
		t.Fatal("a move that names no record was dropped — the audience filter has nothing to judge here")
	}
	if move.Action != "create_task" {
		t.Fatalf("the move came back as %q, wanted the create_task it was stored as", move.Action)
	}
}

// dealReader is a seat that may read deals and activities across the workspace —
// the ordinary rep whose queue carries a drifting deal. It is deliberately NOT
// a participant on anything, so its access to a message is the workspace
// audience and nothing narrower.
func dealReader(e *SearchEnv) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + ids.NewV7().String(), UserID: ids.NewV7(),
		SeatType: principal.SeatFull,
		Permissions: principal.Permissions{
			Objects: map[string]principal.ObjectGrant{
				"deal":     {Read: true},
				"activity": {Read: true},
			},
			RowScope: principal.RowScopeAll,
		},
	})
}

// readerOf is the app_user the cache is keyed by. The card table has a foreign
// key onto app_user, so the row the fixture writes needs a real one.
func readerOf(ctx context.Context, t *testing.T) ids.UUID {
	t.Helper()
	actor, ok := principal.Actor(ctx)
	if !ok {
		t.Fatal("the reader context carries no principal")
	}
	return actor.UserID
}

// seedCachedMove writes the card a deal page would have written for this reader
// while they could still read the message: the unanswered-inbound move, naming
// that message.
func seedCachedMove(t *testing.T, e *SearchEnv, userID, deal, activity ids.UUID) {
	t.Helper()
	seedCachedCard(t, e, userID, deal, crmcontracts.DealStatusCardMove{
		Action:    "draft_email",
		Reason:    "They wrote 6 days ago and nobody has answered — draft the reply.",
		Arguments: &map[string]any{"activity_id": activity.String()},
	})
}

// seedCachedCard puts one card in the per-reader cache, through the owner
// connection for the reason SeedID gives: these suites test READ semantics, and
// writing through the service under test would make the fixture depend on it.
func seedCachedCard(t *testing.T, e *SearchEnv, userID, deal ids.UUID, move crmcontracts.DealStatusCardMove) {
	t.Helper()
	payload, err := json.Marshal(map[string]any{
		"card": crmcontracts.DealStatusCard{Next: &move},
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
