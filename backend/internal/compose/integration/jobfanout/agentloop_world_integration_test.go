// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package jobfanout

import (
	"encoding/json"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/agentloope2e"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
)

// briefItem is one row of the acting rep's persisted queue as the REST read
// serves it. The tool names the same row item_id.
type briefItem struct {
	ID       string   `json:"id"`
	DealID   string   `json:"deal_id"`
	Evidence []string `json:"evidence_ids"`
}

// loopWorld is the seeded workspace of one scenario, with the ids the real
// writers minted kept under the seed's logical names.
type loopWorld struct {
	// deals maps a seeded deal name to its id.
	deals map[string]string
	// activities lists each deal's seeded activity ids in seed order.
	activities map[string][]string
	// seededAt is the one instant every seeded offset counts from.
	seededAt time.Time
	// day and queue describe the persisted brief run of a morning brief: its
	// local day, and each queued deal's item.
	day   string
	queue map[string]briefItem
}

// nameOf is the logical subject of a deal id or a queue item id, or "" for an
// id the seed does not know.
func (w *loopWorld) nameOf(id string) string {
	for name, dealID := range w.deals {
		if dealID == id || (w.queue[name].ID != "" && w.queue[name].ID == id) {
			return name
		}
	}
	return ""
}

// created posts a body to a collection and answers the id the writer minted.
func (re *runnerEnv) created(t *testing.T, path string, body integration.AnyMap) string {
	t.Helper()
	var out struct {
		ID string `json:"id"`
	}
	var raw json.RawMessage
	if status := re.Call(t, "POST", path, body, nil, &raw); status != http.StatusCreated || json.Unmarshal(raw, &out) != nil || out.ID == "" {
		t.Fatalf("POST %s → %d %s", path, status, raw)
	}
	return out.ID
}

// seedLoopWorld writes a scenario's seed through the API, so the clocks and
// links a deal derives from its activities are the ones production keeps. Every
// offset counts from one instant read at the start.
func (re *runnerEnv) seedLoopWorld(t *testing.T, sc agentloope2e.Scenario) *loopWorld {
	t.Helper()
	now := wallNow()
	stages := apptest.DiscoverSeededPipeline(t, re.AppEnv)
	world := &loopWorld{seededAt: now, deals: map[string]string{}, activities: map[string][]string{}}
	var otherSeat string
	for _, deal := range sc.Seed.Deals {
		body := integration.AnyMap{
			"name": deal.DisplayName(), "pipeline_id": stages.PipelineID, "stage_id": stages.Open, "source": "manual",
		}
		if deal.OtherSeat {
			if otherSeat == "" {
				re.DescribeCompany(t)
				otherSeat = re.created(t, "/v1/users", integration.AnyMap{
					"email": "other-seat@fable.test", "display_name": "Other Seat", "role": "rep",
				})
			}
			body["owner_id"] = otherSeat
		}
		id := re.created(t, "/v1/deals", body)
		world.deals[deal.Name] = id
		for _, activity := range deal.Activities {
			world.activities[deal.Name] = append(world.activities[deal.Name], re.created(t, "/v1/activities", integration.AnyMap{
				"kind": "email", "source": "manual", "subject": activity.Subject, "body": activity.Body,
				"occurred_at": now.AddDate(0, 0, -activity.DaysAgo).Format(time.RFC3339),
				"links":       []integration.AnyMap{{"entity_type": "deal", "entity_id": id}},
			}))
		}
	}
	if sc.Agent == agentloope2e.MorningBrief {
		re.persistBrief(t, sc, world)
	}
	return world
}

// persistBrief assembles today's brief for the rep the passport acts for.
// POST /v1/brief is not the worker's brief_generate. Both call the same engine,
// but the order of the worker's queue may differ. The queue persisted here must
// be the scenario's worklist, so a drifting seed fails before any model runs.
func (re *runnerEnv) persistBrief(t *testing.T, sc agentloope2e.Scenario, world *loopWorld) {
	t.Helper()
	if status := re.Call(t, "POST", "/v1/brief", nil, nil, nil); status != http.StatusCreated {
		t.Fatalf("assembling the brief → %d", status)
	}
	var run struct {
		LocalDay string      `json:"local_day"`
		Items    []briefItem `json:"items"`
	}
	if status := re.Call(t, "GET", "/v1/brief", nil, nil, &run); status != http.StatusOK {
		t.Fatalf("reading the brief → %d", status)
	}
	world.day, world.queue = run.LocalDay, map[string]briefItem{}
	var queued []string
	for _, item := range run.Items {
		name := world.nameOf(item.DealID)
		world.queue[name] = item
		queued = append(queued, name)
	}
	slices.Sort(queued)
	want := slices.Sorted(slices.Values(sc.Worklist))
	if !slices.Equal(queued, want) {
		t.Fatalf("harness: the seeded brief queue is %v, the scenario says %v", queued, want)
	}
}

// wallNow is the real clock. The tools read it as they run, so the seed and the
// day guards must too, and no fixture clock can stand in.
func wallNow() time.Time { return time.Now().UTC() }

// dayHeadroom is the time a case needs before a day boundary. The tools read the
// clock as they run, so a case that straddles midnight reads another day.
const dayHeadroom = 10 * time.Minute

// requireDayHeadroom waits out a day boundary a case depends on when it is too
// close. The sweep counts a note per UTC day. The brief is one run per local
// day. Waiting at most dayHeadroom for the next day is what a rerun would do,
// so callers ask before they seed anything dated by the day.
func (re *runnerEnv) requireDayHeadroom(t *testing.T, sc agentloope2e.Scenario) {
	t.Helper()
	loc := time.UTC
	if sc.Agent == agentloope2e.MorningBrief {
		loc = re.installationZone(t)
	}
	waitOutDayEnd(t, wallNow(), loc, time.After)
}

// waitOutDayEnd waits on after until the day in loc that now falls in is over,
// when less than dayHeadroom of it is left.
func waitOutDayEnd(t *testing.T, now time.Time, loc *time.Location, after func(time.Duration) <-chan time.Time) {
	t.Helper()
	local := now.In(loc)
	midnight := time.Date(local.Year(), local.Month(), local.Day()+1, 0, 0, 0, 0, loc)
	left := midnight.Sub(now)
	if left >= dayHeadroom {
		return
	}
	t.Logf("harness: only %s remain of the day this case reads; it waits for the next day", left.Round(time.Second))
	select {
	case <-after(left + time.Second):
	case <-t.Context().Done():
		t.Fatal("harness: the test ended while it waited for the next day")
	}
}

func (re *runnerEnv) installationZone(t *testing.T) *time.Location {
	t.Helper()
	var name string
	err := database.WithWorkspaceTx(re.wsCtx, re.pool, func(tx pgx.Tx) error {
		var err error
		name, err = identity.TimezoneAppliedTx(re.wsCtx, tx)
		return err
	})
	if err != nil {
		t.Fatalf("reading the installation's timezone: %v", err)
	}
	loc, err := storekit.LoadZone(name)
	if err != nil {
		t.Fatal(err)
	}
	return loc
}

// assertDayHeld fails as a harness error when the seeded day moved during the
// run. That day is the brief's local day, or the sweep's UTC day.
func (re *runnerEnv) assertDayHeld(t *testing.T, sc agentloope2e.Scenario, world *loopWorld) {
	t.Helper()
	if sc.Agent == agentloope2e.OvernightSweep {
		if was, is := world.seededAt.Format(time.DateOnly), wallNow().Format(time.DateOnly); was != is {
			t.Fatalf("harness: the sweep was seeded on %s (UTC) and ended on %s; rerun the case", was, is)
		}
		return
	}
	var run struct {
		LocalDay string `json:"local_day"`
	}
	if status := re.Call(t, "GET", "/v1/brief", nil, nil, &run); status != http.StatusOK || run.LocalDay != world.day {
		t.Fatalf("harness: the brief was seeded for %s and now reads %q (status %d); rerun the case", world.day, run.LocalDay, status)
	}
}

// A case that starts too close to midnight waits for the next day instead of
// failing. One with the day ahead of it does not wait.
func TestACaseNearMidnightWaitsForTheNextDay(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		now  time.Time
		wait time.Duration
	}{
		"two minutes left": {time.Date(2025, 3, 10, 23, 58, 0, 0, time.UTC), 2*time.Minute + time.Second},
		"the day ahead":    {time.Date(2025, 3, 10, 9, 0, 0, 0, time.UTC), 0},
	} {
		t.Run(name, func(t *testing.T) {
			var asked time.Duration
			waitOutDayEnd(t, tc.now, time.UTC, func(d time.Duration) <-chan time.Time {
				asked = d
				done := make(chan time.Time, 1)
				done <- tc.now.Add(d)
				return done
			})
			if asked != tc.wait {
				t.Errorf("waited %s, want %s", asked, tc.wait)
			}
		})
	}
}
