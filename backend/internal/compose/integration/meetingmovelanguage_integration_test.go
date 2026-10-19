// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/meetingbrief"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/settings"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestDealMovementReadUsesBaseLanguageAndPreservesItsSource(t *testing.T) {
	e := Setup(t)
	pipeline, open, _ := DealFixture(t, e)
	deal, err := e.Deals.CreateDeal(e.Admin(), deals.CreateDealInput{Name: "Original deal", PipelineID: pipeline, StageID: open, Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	stage, err := e.Deals.GetStage(e.Admin(), open)
	if err != nil {
		t.Fatal(err)
	}
	var changed time.Time
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(e.Admin(), `SELECT changed_at FROM deal_stage_history WHERE deal_id = $1`, deal.Id).Scan(&changed)
	}); err != nil {
		t.Fatal(err)
	}
	owner := OwnerConn(t)
	attendee := e.SeedContact(t, "Original contact", &e.Rep1)
	meeting := SeedIDRow(t, owner, `INSERT INTO activity (id, kind, subject, occurred_at, source, captured_by)
		VALUES ($1, 'meeting', 'Next discussion', $2, 'manual', 'human:x')`, changed.Add(2*time.Hour))
	LinkActivity(t, owner, meeting, "contact", attendee)
	LinkActivity(t, owner, meeting, "deal", ids.UUID(deal.Id))
	seatInRoom(t, owner, e.WS, meeting, attendee)
	prior := SeedIDRow(t, owner, `INSERT INTO activity (id, kind, subject, occurred_at, source, captured_by)
		VALUES ($1, 'meeting', 'Prior discussion', $2, 'manual', 'human:x')`, changed.Add(-time.Hour))
	LinkActivity(t, owner, prior, "contact", attendee)
	seatInRoom(t, owner, e.WS, prior, attendee)
	seatUserInRoom(t, owner, prior, e.AdminUser)
	service := meetingBriefServiceAt(e, changed.Add(time.Hour))
	store := settings.New(e.Pool, settings.NewRegistry(identity.BaseLanguage))
	for _, tc := range []struct{ language, prefix string }{{"en", "Since then the deal"}, {"de", "Seitdem ist der Deal"}, {"vi", "Kể từ đó, giao dịch"}} {
		if err := settings.Set(e.Admin(), store, identity.BaseLanguage, tc.language); err != nil {
			t.Fatal(err)
		}
		brief, err := service.Get(e.Admin(), meeting)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, section := range brief.Sections {
			if section.Kind != "what_changed" {
				continue
			}
			for _, sentence := range section.Sentences {
				if strings.HasPrefix(sentence.Text, tc.prefix) && strings.Contains(sentence.Text, stage.Name) {
					found = true
				}
			}
		}
		if !found {
			t.Fatalf("%s service omitted localized movement: %+v", tc.language, brief.Sections)
		}
		if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
			moves, _, err := meetingbrief.ReadDealMovesForTest(e.Admin(), tx, ids.UUID(deal.Id), changed.Add(-time.Hour), changed.Add(time.Hour), tc.language)
			if err != nil {
				return err
			}
			if len(moves) != 1 || moves[0].DealID != ids.UUID(deal.Id).String() || !moves[0].At.Equal(changed) || !strings.HasPrefix(moves[0].Text, tc.prefix) || !strings.Contains(moves[0].Text, stage.Name) {
				t.Fatalf("%s moved source or missed language: %+v", tc.language, moves)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
}
