// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package notices

import (
	"strings"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestSelfMadeStageUpdatesDoNotConsumeTheUnreadPage(t *testing.T) {
	e := setupNotices(t)
	for _, tc := range []struct {
		name, actorType, actorID string
		hidden                   bool
	}{
		{"owner", "human", "human:" + e.recipient.String(), true},
		{"bare owner", "human", e.recipient.String(), true},
		{"uppercase UUID", "human", "human:" + strings.ToUpper(e.recipient.String()), true},
		{"colleague", "human", "human:" + e.other.String(), false},
		{"agent for owner", "agent", "agent:planner", false},
		{"system with owner id", "system", "human:" + e.recipient.String(), false},
		{"unknown actor", "", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			origin := &crmcontracts.NoticeOrigin{EventId: openapi_types.UUID(ids.NewV7()), ActorType: tc.actorType, ActorId: tc.actorID, OccurredAt: time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)}
			behalf := openapi_types.UUID(e.recipient.UUID)
			origin.OnBehalfOf = &behalf
			origin.StageChange = &struct {
				FromName *string `json:"from_name,omitempty"`
				ToName   *string `json:"to_name,omitempty"`
			}{}
			for _, historic := range []bool{false, true} {
				storedOrigin := *origin
				if historic {
					storedOrigin.StageChange = nil
				}
				in := NewNotice{Recipient: e.recipient, Kind: "automation", Subject: tc.name, Origin: &storedOrigin, Target: Target{Type: "deal", ID: ids.NewV7()}, DedupeKey: "stage_change_notify:" + ids.NewV7().String()}
				id, err := e.store.Create(e.engineCtx(), in)
				if err != nil {
					t.Fatal(err)
				}
				rows, err := e.store.UnreadFor(e.asUser(e.recipient), 100)
				if err != nil {
					t.Fatal(err)
				}
				found := false
				for _, row := range rows {
					if row.ID == id {
						found = true
					}
				}
				if found == tc.hidden {
					t.Fatalf("historic=%v: visible=%v, want %v", historic, found, !tc.hidden)
				}
				replayed, err := e.store.Create(e.engineCtx(), in)
				if err != nil || replayed != id {
					t.Fatalf("replay changed notice identity: %s, %v", replayed, err)
				}
				if err := e.store.MarkRead(e.asUser(e.recipient), id); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
	useful, err := e.store.Create(e.engineCtx(), NewNotice{Recipient: e.recipient, Kind: "automation", Subject: "A change from elsewhere"})
	if err != nil {
		t.Fatal(err)
	}
	for range 3 {
		origin := &crmcontracts.NoticeOrigin{EventId: openapi_types.UUID(ids.NewV7()), ActorType: "human", ActorId: "human:" + e.recipient.String(), OccurredAt: time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)}
		_, err := e.store.Create(e.engineCtx(), NewNotice{Recipient: e.recipient, Kind: "automation", Subject: "Self-made stage change", Origin: origin, Target: Target{Type: "deal", ID: ids.NewV7()}, DedupeKey: "stage_change_notify:" + ids.NewV7().String()})
		if err != nil {
			t.Fatal(err)
		}
	}
	rows, err := e.store.UnreadFor(e.asUser(e.recipient), 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].ID != useful {
		t.Fatalf("self-made updates consumed the page: %+v", rows)
	}
	var unread int
	if err := e.owner.QueryRow(e.engineCtx(), `SELECT count(*) FROM notice WHERE read_at IS NULL`).Scan(&unread); err != nil {
		t.Fatal(err)
	}
	if unread != 4 {
		t.Fatalf("filtering must preserve read state: %d unread, want 4", unread)
	}
}

func TestOtherSelfAttributedNoticesRemainVisible(t *testing.T) {
	e := setupNotices(t)
	origin := &crmcontracts.NoticeOrigin{EventId: openapi_types.UUID(ids.NewV7()), ActorType: "human", ActorId: "human:" + e.recipient.String(), OccurredAt: time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)}
	for _, tc := range []struct{ name, kind, target, dedupe string }{
		{"different event", "automation", "deal", "other-event:"},
		{"different record", "automation", "contact", "stage_change_notify:"},
		{"different kind", "lead_sla", "deal", "stage_change_notify:"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			id, err := e.store.Create(e.engineCtx(), NewNotice{Recipient: e.recipient, Kind: tc.kind, Subject: tc.name, Origin: origin, Target: Target{Type: tc.target, ID: ids.NewV7()}, DedupeKey: tc.dedupe + ids.NewV7().String()})
			if err != nil {
				t.Fatal(err)
			}
			rows, err := e.store.UnreadFor(e.asUser(e.recipient), 10)
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, row := range rows {
				if row.ID == id {
					found = true
				}
			}
			if !found {
				t.Fatal("self-attribution hid a notice unrelated to a stage change")
			}
		})
	}
}
