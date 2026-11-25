// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A request can commit an event after clearOutbox and before the sweep. That
// event survives the reset and names a row the sweep deleted. These tests stage
// such events inside the real reset and hand them to their consumers. Each
// consumer must skip the event and never leave it pending.

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/commissions"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/dealrooms"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// strayEventPerms lets the in-window request win a partner deal: the harness's
// AdminPerms carries no `partner` grant.
var strayEventPerms = principal.Permissions{
	RoleKeys: []string{"admin"},
	Objects: map[string]principal.ObjectGrant{
		"partner":               {Create: true, Read: true, Update: true},
		"company":               {Create: true, Read: true, Update: true},
		"deal":                  {Create: true, Read: true, Update: true, Delete: true},
		"installation_settings": {Read: true},
	},
	RowScope: principal.RowScopeAll,
}

// strayDeal is a partner deal, priced, on its open stage.
type strayDeal struct {
	deal      ids.DealID
	open, won ids.StageID
}

func seedStrayDeal(t *testing.T, e *integration.Env) strayDeal {
	t.Helper()
	pipeline, open, won := integration.DealFixture(t, e)
	seat := e.As(e.AdminUser, nil, strayEventPerms)
	partner := ids.From[ids.CompanyKind](e.SeedCompany(t, "Northgate Partners", nil))
	tier := "tier2_20"
	if _, err := e.Contacts.UpsertPartner(seat, contacts.UpsertPartnerInput{
		CompanyID: partner, PartnerRole: "consulting", MarginTier: &tier,
	}); err != nil {
		t.Fatalf("making the company a partner: %v", err)
	}
	deal := ids.From[ids.DealKind](e.SeedDeal(t, "Northgate rollout", pipeline, open, &e.Rep1))
	amount, currency := int64(100_000), "EUR"
	if _, err := e.Deals.UpdateDeal(seat, deal, deals.UpdateDealInput{
		PartnerCompanyID: &partner, AmountMinor: &amount, Currency: &currency,
	}); err != nil {
		t.Fatalf("pricing the deal and naming its partner: %v", err)
	}
	return strayDeal{deal: deal, open: open, won: won}
}

// resetWithAWriteInTheWindow runs the real reset, and runs write after the
// outbox drain and before the sweep, where a concurrent request can land.
func resetWithAWriteInTheWindow(t *testing.T, e *integration.Env, write func()) {
	t.Helper()
	ctx := e.Admin()
	h := dataResetHandlers{
		pool:             e.Pool,
		dataResetAllowed: true,
		log:              slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	if _, err := h.runQuiesced(ctx, e.WS,
		func() error {
			if err := h.clearOutbox(ctx); err != nil {
				return err
			}
			write()
			return nil
		},
		func(counts *resetCounts) error { return h.sweepAndReseed(ctx, e.WS, "Authz", counts) },
	); err != nil {
		t.Fatalf("reset: %v", err)
	}
}

// strayEnvelopes reads, oldest first, the staged events of one type that name
// one record, as the relay would hand them to a subscriber.
func strayEnvelopes(t *testing.T, e *integration.Env, eventType string, entity ids.UUID) []events.Envelope {
	t.Helper()
	var out []events.Envelope
	err := e.DB().Tx(context.Background(), func(tx pgx.Tx) error {
		rows, err := tx.Query(context.Background(), `
			SELECT envelope FROM event_outbox
			 WHERE envelope->>'type' = $1 AND envelope->'entity'->>'id' = $2::text
			 ORDER BY created_at, id`, eventType, entity)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var raw []byte
			if err := rows.Scan(&raw); err != nil {
				return err
			}
			var env events.Envelope
			if err := json.Unmarshal(raw, &env); err != nil {
				return err
			}
			out = append(out, env)
		}
		return rows.Err()
	})
	if err != nil {
		t.Fatalf("reading the staged %s events: %v", eventType, err)
	}
	return out
}

func TestADealEventTheResetOrphanedIsSkippedByCommissionsAndTheRoomTimeline(t *testing.T) {
	e := integration.Setup(t)
	fx := seedStrayDeal(t, e)
	seat := e.As(e.AdminUser, nil, strayEventPerms)

	resetWithAWriteInTheWindow(t, e, func() {
		if _, err := e.Deals.AdvanceDeal(seat, fx.deal, deals.AdvanceDealInput{
			ToStageID: fx.won, WonWithoutContractReason: integration.WonByImport(),
		}); err != nil {
			t.Fatalf("winning the deal in the window: %v", err)
		}
		if _, err := e.Deals.AdvanceDeal(seat, fx.deal, deals.AdvanceDealInput{ToStageID: fx.open}); err != nil {
			t.Fatalf("reopening the deal in the window: %v", err)
		}
	})
	if n := e.WsCount(t, `SELECT count(*) FROM deal WHERE id = $1`, fx.deal); n != 0 {
		t.Fatalf("the sweep left the deal behind (%d rows), so nothing here is orphaned", n)
	}
	moved := strayEnvelopes(t, e, "deal.stage_changed", fx.deal.UUID)
	if len(moved) != 2 {
		t.Fatalf("%d stage changes survived the reset, want the win and the reopen", len(moved))
	}

	gen := NewCommissionGen(e.Pool, commissions.NewStore(e.DB()), contacts.NewStore(e.DB()), slog.New(slog.DiscardHandler))
	if err := gen.HandleEvent(context.Background(), moved[0]); !errors.Is(err, events.ErrUnactionable) {
		t.Errorf("commissions on the orphaned win answered %v, want ErrUnactionable — any other error keeps the entry pending on cg:commissions", err)
	}
	// The entries a reopen would reverse went with the deal, so there is
	// nothing to do and nothing to report.
	if err := gen.HandleEvent(context.Background(), moved[1]); err != nil {
		t.Errorf("commissions on the orphaned reopen answered %v, want nil", err)
	}

	timeline := NewDealRoomTimeline(e.Pool, activities.NewStore(e.DB()), slog.New(slog.DiscardHandler))
	if err := timeline.HandleEvent(context.Background(), orphanedRoomComment(t, fx.deal)); !errors.Is(err, events.ErrUnactionable) {
		t.Errorf("the room timeline on a comment about the swept deal answered %v, want ErrUnactionable — any other error stalls cg:deal-room-timeline", err)
	}
}

// An archived deal still exists and can be restored, so a room note about it
// stays owed. The entry is retried rather than acked away.
func TestARoomCommentOnAnArchivedDealIsRetriedNotSkipped(t *testing.T) {
	e := integration.Setup(t)
	fx := seedStrayDeal(t, e)
	if _, err := e.Deals.ArchiveDeal(e.As(e.AdminUser, nil, strayEventPerms), fx.deal, nil); err != nil {
		t.Fatalf("archiving the deal: %v", err)
	}

	timeline := NewDealRoomTimeline(e.Pool, activities.NewStore(e.DB()), slog.New(slog.DiscardHandler))
	err := timeline.HandleEvent(context.Background(), orphanedRoomComment(t, fx.deal))
	if err == nil {
		t.Fatal("a note was written onto an archived deal, which is frozen")
	}
	if errors.Is(err, events.ErrUnactionable) {
		t.Errorf("the room timeline answered %v for an archived deal — the entry is acked and the note is lost for good, though the deal can be restored", err)
	}
}

// orphanedRoomComment is a buyer comment about the deal. It carries a
// correlation id, because the note's write refuses to run without one.
func orphanedRoomComment(t *testing.T, deal ids.DealID) events.Envelope {
	t.Helper()
	name := "Thorsten Sifferlien"
	raw, err := json.Marshal(crmcontracts.PublicEventDealRoomCommentPosted{
		DealId:      openapi_types.UUID(deal.UUID),
		ThreadId:    openapi_types.UUID(ids.NewV7()),
		CommentId:   openapi_types.UUID(ids.NewV7()),
		Side:        "buyer",
		OpensThread: true,
		AuthorName:  &name,
	})
	if err != nil {
		t.Fatalf("encoding the comment: %v", err)
	}
	return events.Envelope{
		EventID:    ids.NewV7(),
		Type:       dealrooms.EventCommentPosted,
		OccurredAt: time.Now().UTC(),
		Entity:     events.EntityRef{Type: roomTimelineEntity, ID: ids.NewV7()},
		Trace:      events.Trace{CorrelationID: ids.NewV7()},
		Payload:    raw,
	}
}

// The notice-case consumer reads a missing contact as nothing owed. The cohort
// repair reports it as unactionable. Neither answer leaves the entry pending.
func TestAContactCreatedTheResetOrphanedIsSkippedByItsConsumers(t *testing.T) {
	e := integration.Setup(t)
	var contact ids.UUID
	resetWithAWriteInTheWindow(t, e, func() { contact = e.SeedContact(t, "Selma Window", nil) })

	created := strayEnvelopes(t, e, "contact.created", contact)
	if len(created) != 1 {
		t.Fatalf("%d contact.created events survived the reset, want 1", len(created))
	}
	opener := NewNoticeCaseOpen(e.Pool, func() time.Time { return clocktest.Now(t) }, slog.New(slog.DiscardHandler))
	if err := opener.HandleEvent(context.Background(), created[0]); err != nil {
		t.Errorf("notice cases on the orphaned contact answered %v, want nil — an error keeps the entry pending", err)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM privacy_notice_case`); n != 0 {
		t.Errorf("%d notice cases exist for a contact the reset deleted, want 0", n)
	}

	cohort := NewCohortPromoteGen(e.Pool, contacts.NewStore(e.DB()), slog.New(slog.DiscardHandler))
	if err := cohort.HandleEvent(context.Background(), created[0]); !errors.Is(err, events.ErrUnactionable) {
		t.Errorf("the cohort repair on the orphaned contact answered %v, want ErrUnactionable", err)
	}
}
