// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A cached move can name a CONTACT, not only a message.
//
// The meeting move files work against the stakeholder it names: "Book a meeting
// with Annabelle Malherbe", linked to her. That link and that name are stored
// per reader and served again from the queue long after the card was written,
// so they outlive the grant that allowed them. The sibling suite beside this one
// proves the same thing for the message a reply move names; this is the contact
// arm, and it is a separate loss because NamedActivity never looked at links.

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/compose/dealstatus"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// dealAndContactReader is a seat that may read both records a meeting move
// names. The sibling suite's dealReader holds no contact grant, which this
// change's own revalidation correctly refuses — so the case that proves the
// refusal needs a reader who starts out allowed.
func dealAndContactReader(e *SearchEnv) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + ids.NewV7().String(), UserID: ids.NewV7(),
		SeatType: principal.SeatFull,
		Permissions: principal.Permissions{
			Objects: map[string]principal.ObjectGrant{
				"deal":     {Read: true},
				"activity": {Read: true},
				"contact":  {Read: true},
			},
			// Owner scope, not RowScopeAll: the narrowing below is a change of
			// OWNER, and a reader who sees every row would be unaffected by it.
			RowScope: principal.RowScopeOwn,
		},
	})
}

func TestACachedMoveStopsNamingAContactTheReaderMayNoLongerRead(t *testing.T) {
	e := SetupSearch(t)
	ctx := context.Background()

	pipeline := e.SeedID(t, `INSERT INTO pipeline (id, name, is_default, position)
		VALUES ($1, 'Sales', true, 0)`)
	stage := e.SeedID(t, `INSERT INTO stage (id, pipeline_id, name, position, semantic, win_probability)
		VALUES ($1, $2, 'Qualify', 0, 'open', 10)`, pipeline)
	deal := e.SeedID(t, `INSERT INTO deal (id, owner_id, name, pipeline_id, stage_id, source, captured_by)
		VALUES ($1, $2, 'PIM-Rollout Phase 2', $3, $4, 'manual', 'human:x')`,
		e.Rep1, pipeline, stage)
	// Owned by the reader to begin with, which is the state the card was
	// written under: they may read this contact, so the move may name them.
	reader := dealAndContactReader(e)
	readerID := readerOf(reader, t)
	svc := dealstatus.NewService(e.Pool, nil, nil, nil, nil)
	// The contact is owned by the reader, which is what lets the move name them
	// to begin with. Seeded through Rep1 first and handed over below, because
	// the reader's own app_user row is created by the cache fixture.
	champion := e.SeedID(t, `INSERT INTO contact (id, owner_id, full_name, source, captured_by, visibility)
		VALUES ($1, $2, 'Annabelle Malherbe', 'manual', 'human:x', 'workspace')`, e.Rep1)
	seedCachedMeetingMove(t, e, readerID, deal, champion)
	if _, err := e.Owner.Exec(ctx,
		`UPDATE contact SET owner_id = $1 WHERE id = $2`, readerID, champion); err != nil {
		t.Fatal(err)
	}

	// Asserted FIRST, so the refusal below means something rather than passing
	// because the move never named anybody.
	before, err := svc.CachedMoves(reader, []ids.UUID{deal})
	if err != nil {
		t.Fatalf("reading the cached moves: %v", err)
	}
	named := dealstatus.NamedContacts(before[deal])
	if len(named) != 1 || named[0] != champion {
		t.Fatalf("the move names %v, wanted the contact %s it was written about — "+
			"without this the refusal below proves nothing", before[deal].Arguments, champion)
	}

	// The loss. The contact becomes the owner-only record of somebody on the OTHER
	// team, so this reader's own row scope no longer reaches it.
	if _, err := e.Owner.Exec(ctx,
		`UPDATE contact SET owner_id = $1, visibility = 'owner' WHERE id = $2`,
		e.Rep3, champion); err != nil {
		t.Fatal(err)
	}

	after, err := svc.CachedMoves(reader, []ids.UUID{deal})
	if err != nil {
		t.Fatalf("reading the cached moves after the narrowing: %v", err)
	}
	if move, present := after[deal]; present {
		t.Fatalf("the move survived the narrowing as %+v — it names a contact this reader "+
			"may no longer read, in its subject and in the record it would file work against", move)
	}
}

// seedCachedMeetingMove stores the move this change introduced: a task to book
// a meeting, filed against the deal and the contact it is with.
func seedCachedMeetingMove(t *testing.T, e *SearchEnv, userID, deal, contact ids.UUID) {
	t.Helper()
	seedCachedCard(t, e, userID, deal, crmcontracts.DealStatusCardMove{
		Action: "create_task",
		Reason: "The last contact was 23 days ago and nothing is booked. " +
			"Book a meeting with Annabelle Malherbe, the champion.",
		Arguments: &map[string]any{
			"subject": "Book a meeting with Annabelle Malherbe",
			"links": []any{
				map[string]any{"entity_type": "deal", "entity_id": deal.String()},
				map[string]any{"entity_type": "contact", "entity_id": contact.String()},
			},
			"source": "manual",
		},
	})
}
