// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package suggestionlanes

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/attention"
	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/collections"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// pastTheCensus is more suggestions of each kind than the decision lane reads
// before the scroll tail takes over.
const pastTheCensus = 230

// logged writes one activity as the admin and answers its id and stored time.
// A suggestion's evidence must cite the stored time.
func logged(t *testing.T, e *integration.Env, in activities.LogActivityInput) (ids.UUID, time.Time) {
	t.Helper()
	row, _, err := e.Activities.LogActivity(e.Admin(), in)
	if err != nil {
		t.Fatalf("logging the %s: %v", in.Kind, err)
	}
	var at time.Time
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(e.Admin(), `SELECT occurred_at FROM activity WHERE id = $1`, row.Id).Scan(&at)
	}); err != nil {
		t.Fatal(err)
	}
	return ids.UUID(row.Id), at
}

// seedSuggestions records count suggestions of each kind through the stores'
// own writers. A deal suggestion cites a held meeting on its own company, and a
// tag suggestion a note on its own contact.
func seedSuggestions(t *testing.T, e *integration.Env, count int) {
	t.Helper()
	integration.DealFixture(t, e)
	tags := collections.NewStore(e.DB())
	description := "product x pricing"
	tag, err := tags.CreateTag(e.Admin(), "Product X", nil, &description)
	if err != nil {
		t.Fatal(err)
	}
	suggestible := true
	if _, err := tags.UpdateTag(e.Admin(), tag.ID, collections.TagUpdate{Suggestible: &suggestible}, 0); err != nil {
		t.Fatal(err)
	}
	var dealDrafts []deals.SuggestionDraft
	var tagDrafts []collections.TagSuggestionDraft
	held, subject, when := "held", "Scoping workshop", time.Now().Add(-48*time.Hour)
	for i := range count {
		company := e.SeedCompany(t, fmt.Sprintf("Account %d", i+1), nil)
		meeting, at := logged(t, e, activities.LogActivityInput{
			Kind: "meeting", Subject: &subject, OccurredAt: &when, MeetingStatus: &held, Source: "manual",
		})
		dealDrafts = append(dealDrafts, deals.SuggestionDraft{
			CompanyID: company, NameHint: deals.HintMeetingHeld, Confidence: 0.8,
			Evidence: []deals.SuggestionEvidence{{Kind: deals.EvidenceMeeting, ActivityID: &meeting, OccurredAt: at}},
		})
		contact := e.SeedContact(t, fmt.Sprintf("Tagged %d", i+1), nil)
		body := "Asked about Product X pricing."
		note, noted := logged(t, e, activities.LogActivityInput{
			Kind: "note", Subject: &subject, Body: &body, OccurredAt: &when, Source: "manual",
			Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact}},
		})
		tagDrafts = append(tagDrafts, collections.TagSuggestionDraft{
			TagID: tag.ID, EntityType: "contact", EntityID: contact,
			Evidence: []collections.TagSuggestionEvidence{{ActivityID: note, OccurredAt: noted}},
		})
	}
	system := principal.SystemActing(principal.WithWorkspaceID(context.Background(), e.WS), "agent:deal-scout")
	if err := database.WithWorkspaceTx(system, e.Pool, func(tx pgx.Tx) error {
		for i := range count {
			raised, err := deals.RecordSuggestionTx(system, tx, dealDrafts[i])
			if err != nil || !raised {
				return fmt.Errorf("deal suggestion %d raised %v: %w", i, raised, err)
			}
			if raised, err = collections.RecordTagSuggestionTx(system, tx, tagDrafts[i]); err != nil || !raised {
				return fmt.Errorf("tag suggestion %d raised %v: %w", i, raised, err)
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

// walk follows a tail read from the beginning to its end in pages of size and
// answers how often each suggestion was served.
func walk[T any](
	t *testing.T, size int, id func(T) string, read func(after string, limit int) ([]attention.Positioned[T], bool, error),
) map[string]int {
	t.Helper()
	served := map[string]int{}
	after := ""
	for page := 0; ; page++ {
		if page > pastTheCensus {
			t.Fatal("the tail read did not end")
		}
		rows, more, err := read(after, size)
		if err != nil {
			t.Fatal(err)
		}
		for _, row := range rows {
			served[id(row.Row)]++
		}
		if !more {
			return served
		}
		after = rows[len(rows)-1].After
	}
}

// Past the decision lane's census, each suggestion kind's tail read reaches
// every suggestion its count names, once each.
func TestEachSuggestionTailReachesEverySuggestionItsCountNames(t *testing.T) {
	e := integration.Setup(t)
	seedSuggestions(t, e, pastTheCensus)
	ctx := e.Admin()
	dealLane, tagLane := Deals{Store: e.Deals}, Tags{Store: collections.NewStore(e.DB())}

	dealOpen, err := dealLane.CountOpen(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	tagOpen, err := tagLane.CountOpenTagSuggestions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for name, check := range map[string]struct {
		served map[string]int
		open   int
	}{
		"deal": {walk(t, 50, func(s crmcontracts.DealSuggestion) string { return s.Id.String() },
			func(after string, limit int) ([]attention.Positioned[crmcontracts.DealSuggestion], bool, error) {
				return dealLane.SuggestionsAfter(ctx, nil, after, limit)
			}), dealOpen},
		"tag": {walk(t, 50, func(s crmcontracts.TagSuggestion) string { return s.Id.String() },
			func(after string, limit int) ([]attention.Positioned[crmcontracts.TagSuggestion], bool, error) {
				return tagLane.TagSuggestionsAfter(ctx, after, limit)
			}), tagOpen},
	} {
		if check.open != pastTheCensus || len(check.served) != check.open {
			t.Errorf("%s: counted %d and walked %d, want both %d", name, check.open, len(check.served), pastTheCensus)
		}
		for id, times := range check.served {
			if times != 1 {
				t.Errorf("%s suggestion %s was served %d times, want once", name, id, times)
			}
		}
	}
}
