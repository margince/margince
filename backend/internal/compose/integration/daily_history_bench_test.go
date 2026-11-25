// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration && bench

package integration

import (
	"context"
	"fmt"
	"math"
	"net/url"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/modules/search"
)

// historySizes counts one record's history per kind: the activities linked to
// a company, a deal and a contact, and the mails of one thread.
type historySizes struct{ Company, Deal, Contact, Thread int }

// dailyHistoryTier is the longest history of each kind at the published tier,
// where a company averages about fifty activities. The company's history is
// four hundred times that, yet four percent of the tier's activities. Each
// history nests in the one before it: thread, contact, deal, company.
var dailyHistoryTier = historySizes{Company: 20_000, Deal: 10_000, Contact: 5_000, Thread: 2_000}

func (h historySizes) scaled(f float64) historySizes {
	at := func(n int) int { return max(1, int(math.Round(float64(n)*f))) }
	return historySizes{Company: at(h.Company), Deal: at(h.Deal), Contact: at(h.Contact), Thread: at(h.Thread)}
}

// nested reports whether each history fits inside the one it is filed under,
// the shape the seed's ordinals assume.
func (h historySizes) nested() bool {
	return 0 < h.Thread && h.Thread <= h.Contact && h.Contact <= h.Deal && h.Deal <= h.Company
}

// historyShortfalls names each kind whose history is shorter than wanted.
func historyShortfalls(want, got historySizes) []string {
	var short []string
	for _, kind := range []struct {
		name      string
		want, got int
	}{
		{"company", want.Company, got.Company},
		{"deal", want.Deal, got.Deal},
		{"contact", want.Contact, got.Contact},
		{"thread", want.Thread, got.Thread},
	} {
		if kind.got < kind.want {
			short = append(short, fmt.Sprintf("%s: %d activities, want at least %d", kind.name, kind.got, kind.want))
		}
	}
	return short
}

// dailyHistory names the records with a long history, which the history flows open.
type dailyHistory struct {
	CompanyID, CompanyName, DealID, DealName, ContactID, ThreadKey string
	Sizes                                                          historySizes
	// Longest is each kind's longest history in the corpus, read back after the seed.
	Longest historySizes
}

// input points a seat's flows at the long-history records instead of the corpus's own.
func (h dailyHistory) input(in dailyInput) dailyInput {
	in.CompanyID, in.CompanyName = h.CompanyID, h.CompanyName
	in.DealID, in.DealName = h.DealID, h.DealName
	in.ContactID, in.ThreadKey = h.ContactID, h.ThreadKey
	return in
}

// addTo counts the history's activities in the record's corpus facts. It also
// states the longest history of each kind as the database holds it.
func (h dailyHistory) addTo(c *CorpusFacts) {
	c.Activities += h.Sizes.Company
	c.LargestCompany, c.LargestDeal, c.LargestContact, c.LargestThread = h.Longest.Company, h.Longest.Deal, h.Longest.Contact, h.Longest.Thread
}

// composerThreadPage is how many messages the mail composer reads of one thread.
const composerThreadPage = 12

// dailyHistoryFlows open each long-history record the way its daily flow opens
// the corpus's own. They are recorded and not gated while no bound holds their reads.
var dailyHistoryFlows = []dailyFlow{
	{Name: "history_contact_360", ID: "PERF-7", Budget: search.Perf7Budget, Recorded: true, Calls: contact360Calls},
	{Name: "history_company_360", ID: "PERF-7", Budget: search.Perf7Budget, Recorded: true, Calls: company360Calls},
	{Name: "history_search_to_deal", Journey: true, Recorded: true, Calls: searchToDealCalls},
	{Name: "history_thread", ID: "PERF-1", Budget: perf1RecordOpenBudget, Recorded: true, Calls: func(in dailyInput) []dailyCall {
		args := url.Values{"thread_key": {in.ThreadKey}, "limit": {fmt.Sprint(composerThreadPage)}, "collapse_copies": {"true"}}
		return []dailyCall{{Path: "/v1/activities?" + args.Encode()}}
	}},
}

// seedDailyCorpusWithHistory is seedDailyCorpus, then the long histories, which
// need the picked corpus to stay clear of the records the other flows open.
func seedDailyCorpusWithHistory(t *testing.T, e *apptest.AppEnv, seats Seats, scale float64) (DailyCorpus, dailyHistory) {
	t.Helper()
	s := loadDailyCorpus(t, e, seats, scale)
	corpus := s.pickCorpus(seats)
	var history dailyHistory
	s.timed("long history", func() { history = s.longHistory(e, corpus, dailyHistoryTier.scaled(scale)) })
	s.dropOrdinals()
	return corpus, history
}

// longHistory gives one company of the median rep a long history, through the
// same steps that wrote the corpus's activities. Those steps read the activity
// ordinals, so the history replaces them once the corpus is written.
func (s dailySeeder) longHistory(e *apptest.AppEnv, c DailyCorpus, sizes historySizes) dailyHistory {
	s.t.Helper()
	if !sizes.nested() {
		s.t.Fatalf("history sizes %+v do not nest; each must fit inside the one it is filed under", sizes)
	}
	before := dailyCensus(s.t, e)
	h, contactI, others := s.pickHistory(c)
	h.Sizes = sizes
	inbound := (float64(dailyThreadLength)*2/3 - dailyWaitingShare) / float64(dailyThreadLength-1)
	s.exec("history ordinals", `DROP TABLE daily_seed_activity`)
	s.exec("history ordinals", `CREATE TEMP TABLE daily_seed_activity AS
	      SELECT h.i, public.uuidv7() AS id, 'email'::text AS kind, h.p,
	             1 + floor(-ln(1 - random()) * 4)::int AS paragraphs, (h.g * 7919 % 100000)::int AS subject_n,
	             '{}'::int[] AS topics, p.first_name AS contact_first, c.stem || ' ' || c.suffix AS company_name,
	             p.i AS contact_i, p.company_i, p.owner_id AS rep_id, rep.email AS rep_email, p.lang,
	             'daily-history-' || p.i || '-' || h.g || '@bench.example' AS thread_key,
	             CASE WHEN h.p = h.last_p THEN 'outbound' WHEN random() < $1 THEN 'inbound' ELSE 'outbound' END AS direction,
	             h.occurred_at, 'workspace'::text AS audience, h.j <= $2 AS deal_link,
	             CASE WHEN random() < 0.5 THEN 1 ELSE 2 END AS cc
	      FROM (SELECT j, $3 + j AS i,
	                   CASE WHEN j <= $4 THEN 0 ELSE 1 + (j - $4 - 1) / $5 END AS g,
	                   CASE WHEN j <= $4 THEN j - 1 ELSE (j - $4 - 1) % $5 END AS p,
	                   CASE WHEN j <= $4 THEN $4 - 1 ELSE $5 - 1 END AS last_p,
	                   CASE WHEN j <= $6 THEN $7
	                        ELSE ($8::int[])[1 + ((j - $4 - 1) / $5) % cardinality($8::int[])] END AS contact_i,
	                   CASE WHEN j <= $4 THEN now() - interval '1 day' - ($4 - j) * (interval '365 days' / $4)
	                        ELSE now() - (2 + ((j - $4 - 1) / $5) * 37 % 728) * interval '1 day'
	                             + ((j - $4 - 1) % $5) * interval '14 hours' END AS occurred_at
	            FROM generate_series(1, $9) AS j) h
	      JOIN daily_seed_contact p ON p.i = h.contact_i
	      JOIN daily_seed_company c ON c.i = p.company_i
	      JOIN daily_seed_rep rep ON rep.id = p.owner_id`,
		inbound, sizes.Deal, s.n.Activities, sizes.Thread, dailyThreadLength, sizes.Contact, contactI, others, sizes.Company)
	s.analyze("daily_seed_activity")
	if err := s.owner.QueryRow(context.Background(), `SELECT thread_key FROM daily_seed_activity WHERE i = $1`,
		s.n.Activities+1).Scan(&h.ThreadKey); err != nil {
		s.t.Fatalf("reading the long thread's key: %v", err)
	}
	s.insertActivities()
	s.captureImports()
	s.participants()
	s.interactionEdges()
	s.links()
	s.exec("history last activity", `SELECT move_last_activity('company'::regclass, $1::uuid),
	             move_last_activity('deal'::regclass, $2::uuid)`, h.CompanyID, h.DealID)
	s.exec("history last activity", `SELECT move_last_activity('contact'::regclass, p.id)
	      FROM (SELECT DISTINCT contact_i FROM daily_seed_activity) a JOIN daily_seed_contact p ON p.i = a.contact_i`)
	stampSeededMail(s.t, e)
	s.exec("vacuum", `VACUUM ANALYZE activity, capture_import, activity_participant, activity_link, graph_interaction_edge`)
	assertDailyCensus(s.t, e, before.plus(dailyCounts{Activities: sizes.Company}))
	opened := s.historyOf(h)
	if short := historyShortfalls(sizes, opened); len(short) > 0 {
		s.t.Fatalf("the long-history records the flows open are short, so their rows would time an ordinary record: %v", short)
	}
	h.Longest = s.longestHistories()
	return h
}

// pickHistory takes the median rep's first company, by ordinal, whose first deal
// and two workspace contacts the other flows never open. It returns the company,
// its deal and first contact, that contact's ordinal and the other contacts'.
func (s dailySeeder) pickHistory(c DailyCorpus) (dailyHistory, int, []int) {
	s.t.Helper()
	var h dailyHistory
	var contactI int
	var others []int
	err := s.owner.QueryRow(context.Background(), `
	      SELECT co.id::text, co.display_name, dl.id::text, dl.name, p.id::text, p.i, others.i
	      FROM daily_seed_company sc
	      JOIN company co ON co.id = sc.id
	      JOIN daily_seed_deal sd ON sd.i = sc.i
	      JOIN deal dl ON dl.id = sd.id
	      CROSS JOIN LATERAL (SELECT p.i, p.id FROM daily_seed_contact p
	                          WHERE p.company_i = sc.i AND NOT p.private AND p.id <> ALL($3::uuid[])
	                          ORDER BY p.i LIMIT 1) p
	      CROSS JOIN LATERAL (SELECT array_agg(o.i ORDER BY o.i) AS i FROM daily_seed_contact o
	                          WHERE o.company_i = sc.i AND NOT o.private AND o.i <> p.i) others
	      WHERE sc.owner_id = $1::uuid AND sc.id <> ALL($2::uuid[]) AND sd.id <> ALL($4::uuid[])
	        AND cardinality(others.i) > 0
	      ORDER BY sd.status = 'open' DESC, sc.i LIMIT 1`,
		c.MedianRepID, c.CompanyIDs, c.ContactIDs, c.DealIDs).
		Scan(&h.CompanyID, &h.CompanyName, &h.DealID, &h.DealName, &h.ContactID, &contactI, &others)
	if err != nil {
		s.t.Fatalf("picking the long-history company: the median rep needs a company beside the one the flows open, with a deal and two workspace contacts: %v", err)
	}
	return h, contactI, others
}

// historyOf counts the history of the records the history flows open.
func (s dailySeeder) historyOf(h dailyHistory) historySizes {
	s.t.Helper()
	var got historySizes
	if err := s.owner.QueryRow(context.Background(), `SELECT
	        (SELECT count(*) FROM activity_link WHERE entity_type = 'company' AND company_id = $1::uuid),
	        (SELECT count(*) FROM activity_link WHERE entity_type = 'deal' AND deal_id = $2::uuid),
	        (SELECT count(*) FROM activity_link WHERE entity_type = 'contact' AND contact_id = $3::uuid),
	        (SELECT count(*) FROM activity WHERE thread_key = $4)`,
		h.CompanyID, h.DealID, h.ContactID, h.ThreadKey).Scan(&got.Company, &got.Deal, &got.Contact, &got.Thread); err != nil {
		s.t.Fatalf("counting the long histories: %v", err)
	}
	return got
}

// longestHistories reads each kind's longest history in the whole corpus.
func (s dailySeeder) longestHistories() historySizes {
	s.t.Helper()
	var got historySizes
	if err := s.owner.QueryRow(context.Background(), `SELECT
	        (SELECT coalesce(max(n), 0) FROM (SELECT count(*) AS n FROM activity_link WHERE entity_type = 'company' GROUP BY company_id) k),
	        (SELECT coalesce(max(n), 0) FROM (SELECT count(*) AS n FROM activity_link WHERE entity_type = 'deal' GROUP BY deal_id) k),
	        (SELECT coalesce(max(n), 0) FROM (SELECT count(*) AS n FROM activity_link WHERE entity_type = 'contact' GROUP BY contact_id) k),
	        (SELECT coalesce(max(n), 0) FROM (SELECT count(*) AS n FROM activity WHERE thread_key IS NOT NULL GROUP BY thread_key) k)`).
		Scan(&got.Company, &got.Deal, &got.Contact, &got.Thread); err != nil {
		s.t.Fatalf("reading the corpus's longest histories: %v", err)
	}
	return got
}

func TestHistorySizesNestAtEveryScale(t *testing.T) {
	for _, scale := range []float64{0.001, 0.01, 0.05, 0.2, 1, 2} {
		if sizes := dailyHistoryTier.scaled(scale); !sizes.nested() {
			t.Errorf("scale %g: %+v; each history must fit inside the one it is filed under", scale, sizes)
		}
	}
	if (historySizes{Company: 10, Deal: 20, Contact: 5, Thread: 2}).nested() {
		t.Error("a deal longer than its company's history cannot be filed under it")
	}
}

func TestAShortHistoryIsNamedByKind(t *testing.T) {
	want := historySizes{Company: 1000, Deal: 500, Contact: 250, Thread: 100}
	if short := historyShortfalls(want, want); len(short) != 0 {
		t.Fatalf("histories at their stated sizes: %v; want none short", short)
	}
	got := historySizes{Company: 1200, Deal: 499, Contact: 250, Thread: 4}
	short := historyShortfalls(want, got)
	if !slices.Equal(short, []string{"deal: 499 activities, want at least 500", "thread: 4 activities, want at least 100"}) {
		t.Fatalf("got %q; want the deal and the thread named with their counts", short)
	}
}

func TestEveryHistoryRowIsRecordedAndNeverGated(t *testing.T) {
	for _, f := range dailyHistoryFlows {
		if !f.Recorded || f.note() != dailyRecordedNote {
			t.Errorf("%s: recorded %v, note %q; a history row is recorded beside its budget until its reader is bounded", f.Name, f.Recorded, f.note())
		}
		if slices.ContainsFunc(dailyFlows, func(g dailyFlow) bool { return g.Name == f.Name }) {
			t.Errorf("%s shares its name with a daily flow, so the two rows would read as one", f.Name)
		}
	}
	for _, f := range dailyFlows {
		if f.Recorded {
			t.Errorf("daily flow %s is recorded only; every corpus flow is judged against its budget", f.Name)
		}
	}
}

func TestTheHistoryInputOpensTheLongHistoryRecords(t *testing.T) {
	h := dailyHistory{CompanyID: "c", CompanyName: "Long Co", DealID: "d", DealName: "Long deal", ContactID: "p", ThreadKey: "t <x>"}
	in := h.input(dailyInput{CompanyID: "c0", DealID: "d0", ContactID: "p0", ProjectID: "pr0", Team: "team"})
	if in.CompanyID != "c" || in.DealName != "Long deal" || in.ContactID != "p" || in.ProjectID != "pr0" || in.Team != "team" {
		t.Fatalf("got %+v; want the history's records and the seat's own project and team", in)
	}
	thread := dailyHistoryFlows[slices.IndexFunc(dailyHistoryFlows, func(f dailyFlow) bool { return f.Name == "history_thread" })]
	if got := thread.Calls(in)[0].Path; got != "/v1/activities?collapse_copies=true&limit=12&thread_key=t+%3Cx%3E" {
		t.Fatalf("thread read %q; want the composer's page of the thread, its key escaped", got)
	}
}

func TestTheRecordStatesTheHistoryFromTheDatabase(t *testing.T) {
	facts := CorpusFacts{Activities: 25_000}
	dailyHistory{Sizes: historySizes{Company: 1000}, Longest: historySizes{Company: 1000, Deal: 500, Contact: 251, Thread: 100}}.addTo(&facts)
	if facts.Activities != 26_000 || facts.LargestCompany != 1000 || facts.LargestDeal != 500 || facts.LargestContact != 251 || facts.LargestThread != 100 {
		t.Fatalf("got %+v; want the history's activities counted and the longest histories as read back", facts)
	}
}
