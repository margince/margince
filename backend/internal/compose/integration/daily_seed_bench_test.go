// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration && bench

package integration

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/shared/kernel/dealrole"
)

// dailyCounts sizes the daily corpus, one count per record table it fills.
type dailyCounts struct{ Contacts, Companies, Deals, Leads, Projects, Activities int }

// dailyTier is the published mid-market tier; every other size is a scaled copy.
var dailyTier = dailyCounts{
	Contacts: 250_000, Companies: 10_000, Deals: 25_000, Leads: 100_000, Projects: 500, Activities: 500_000,
}

func (c dailyCounts) scaled(f float64) dailyCounts {
	at := func(n int) int { return max(1, int(math.Round(float64(n)*f))) }
	return dailyCounts{
		Contacts: at(c.Contacts), Companies: at(c.Companies), Deals: at(c.Deals),
		Leads: at(c.Leads), Projects: at(c.Projects), Activities: at(c.Activities),
	}
}

func (c dailyCounts) plus(o dailyCounts) dailyCounts {
	return dailyCounts{
		Contacts: c.Contacts + o.Contacts, Companies: c.Companies + o.Companies, Deals: c.Deals + o.Deals,
		Leads: c.Leads + o.Leads, Projects: c.Projects + o.Projects, Activities: c.Activities + o.Activities,
	}
}

// DailyCorpus names the records the flows open: a handful of each, owned
// inside team A (the measured rep's team), one per rep who owns one.
type DailyCorpus struct {
	DealIDs, CompanyIDs, ContactIDs, ProjectIDs []string
	DealNames, CompanyNames                     []string
	// MedianRepID is the rep whose contact count is the median of all reps:
	// the typical seat, as opposed to the heaviest one.
	MedianRepID string
}

// The bench's own shape choices. The skew exponent puts the top rep several
// times above the median; month weights make the year uneven (0 = this month).
const (
	dailyOwnerSkew      = 2.2
	dailyPrivateShare   = 0.18
	dailyLimitedShare   = 0.26
	dailyDealLinkShare  = 0.30
	dailyWaitingShare   = 0.10
	dailyThreadLength   = 4
	dailyParagraphsPool = 2048
	dailySubjectsPool   = 1024
	dailyTermPool       = 16
	dailyNamedSubjects  = 0.2
)

var dailyMonthWeights = []int{12, 11, 9, 10, 8, 7, 9, 6, 5, 7, 8, 8}

// dailyMonths expands the month weights into a lookup array, so one random
// index picks a month with its weight and no per-row search is needed.
func dailyMonths() []int32 {
	var months []int32
	for month, weight := range dailyMonthWeights {
		for range weight {
			months = append(months, int32(month))
		}
	}
	return months
}

// dailySeeder bulk-loads through the owner connection. Session temp tables
// (daily_seed_*) map ordinals to ids, so children join parents on an integer.
type dailySeeder struct {
	t     *testing.T
	owner *pgx.Conn
	n     dailyCounts
	reps  []string
}

func (s dailySeeder) exec(step, sql string, args ...any) {
	s.t.Helper()
	if _, err := s.owner.Exec(context.Background(), sql, args...); err != nil {
		s.t.Fatalf("seeding the daily corpus (%s): %v", step, err)
	}
}

func (s dailySeeder) timed(step string, seed func()) {
	s.t.Helper()
	start := time.Now()
	seed()
	s.t.Logf("seed %-24s %8s", step, time.Since(start).Round(time.Millisecond))
}

// analyze refreshes statistics between steps: the link and employment
// triggers plan their reads against whatever statistics exist at insert time.
func (s dailySeeder) analyze(tables ...string) {
	s.t.Helper()
	names := make([]string, len(tables))
	for i, table := range tables {
		names[i] = pgx.Identifier{table}.Sanitize()
	}
	s.exec("analyze", `ANALYZE `+strings.Join(names, ", "))
}

// execDeferringTrigger turns a per-row last-activity trigger (quadratic per company)
// off for one insert, in one transaction so a failure never leaves it off.
func (s dailySeeder) execDeferringTrigger(step, table, trigger, sql string, args ...any) {
	s.t.Helper()
	ctx := context.Background()
	tx, err := s.owner.Begin(ctx)
	if err != nil {
		s.t.Fatalf("seeding the daily corpus (%s): %v", step, err)
	}
	defer func() {
		if err := tx.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			s.t.Errorf("seeding the daily corpus (%s): rollback: %v", step, err)
		}
	}()
	alter := "ALTER TABLE " + pgx.Identifier{table}.Sanitize() + " %s TRIGGER " + pgx.Identifier{trigger}.Sanitize()
	for _, statement := range []struct {
		sql  string
		args []any
	}{{fmt.Sprintf(alter, "DISABLE"), nil}, {sql, args}, {fmt.Sprintf(alter, "ENABLE"), nil}} {
		if _, err := tx.Exec(ctx, statement.sql, statement.args...); err != nil {
			s.t.Fatalf("seeding the daily corpus (%s): %v", step, err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		s.t.Fatalf("seeding the daily corpus (%s): commit: %v", step, err)
	}
}

// moveLastActivity replays the deferred triggers once per record through the
// product's own move_last_activity, so the end state is the triggers' own.
func (s dailySeeder) moveLastActivity() {
	s.exec("last activity", `DO $$ BEGIN
	        PERFORM move_last_activity('contact'::regclass, id) FROM contact;
	        PERFORM move_last_activity('deal'::regclass, id) FROM deal;
	        PERFORM move_last_activity('company'::regclass, id) FROM company;
	      END $$`)
}

// seedDailyCorpus loads the corpus at a scale, asserts its census and returns
// what the flows open. Seats come first: reps own every record and capture every email.
func seedDailyCorpus(t *testing.T, e *apptest.AppEnv, seats Seats, scale float64) DailyCorpus {
	t.Helper()
	if len(seats.Reps) == 0 {
		t.Fatal("the daily corpus needs the rep seats: run the seats subtest first")
	}
	s := dailySeeder{t: t, owner: e.Owner, n: dailyTier.scaled(scale)}
	emails := make([]string, len(seats.Reps))
	for i, rep := range seats.Reps {
		s.reps = append(s.reps, rep.UserID)
		emails[i] = rep.Email
	}
	before := dailyCensus(t, e)
	stages := apptest.DiscoverSeededPipeline(t, e)

	start := time.Now()
	s.exec("seed", `SELECT setseed(0.42)`)
	s.exec("reps", `CREATE TEMP TABLE daily_seed_rep AS
	      SELECT r.n::int AS n, r.id, r.email FROM unnest($1::uuid[], $2::text[]) WITH ORDINALITY AS r(id, email, n)`,
		s.reps, emails)
	s.timed("company", s.companies)
	s.timed("contact", s.contacts)
	s.timed("relationship employment", s.employment)
	s.timed("deal", func() { s.deals(stages) })
	s.timed("relationship stakeholder", s.stakeholders)
	s.timed("lead", s.leads)
	s.timed("project", s.projects)
	s.timed("activity", s.activities)
	s.analyze("activity", "daily_seed_activity")
	s.timed("capture_import", s.captureImports)
	s.timed("activity_participant", s.participants)
	s.analyze("activity_participant", "capture_import")
	s.timed("activity_link", s.links)
	s.analyze("activity_link", "contact", "deal", "company", "relationship")
	s.timed("last_activity_at", s.moveLastActivity)
	s.timed("vacuum analyze", func() {
		s.exec("vacuum", `VACUUM ANALYZE company, company_domain, contact, contact_email, relationship, deal,
		      lead, project, activity, capture_import, activity_participant, activity_link`)
	})
	t.Logf("seed total %s at scale %g", time.Since(start).Round(time.Millisecond), scale)

	assertDailyCensus(t, e, before.plus(s.n))
	corpus := s.pickCorpus(seats)
	s.exec("drop", `DROP TABLE daily_seed_rep, daily_seed_company, daily_seed_contact, daily_seed_deal,
	      daily_seed_thread, daily_seed_activity`)
	return corpus
}

func (s dailySeeder) companies() {
	s.exec("company ordinals", `CREATE TEMP TABLE daily_seed_company AS
	      SELECT c.*, trim(both '-' from regexp_replace(lower(public.f_unaccent(c.stem)), '[^a-z0-9]+', '-', 'g'))
	                  || '-' || c.i || '.example' AS domain
	      FROM (SELECT i, public.uuidv7() AS id,
	                   ($1::uuid[])[1 + floor(power(random(), $2) * cardinality($1::uuid[]))::int] AS owner_id,
	                   ($3::text[])[1 + floor(random() * cardinality($3::text[]))::int] || ' ' ||
	                   ($4::text[])[1 + floor(random() * cardinality($4::text[]))::int] AS stem,
	                   ($5::text[])[1 + floor(random() * cardinality($5::text[]))::int] AS suffix
	            FROM generate_series(1, $6) AS i) c`,
		s.reps, dailyOwnerSkew, dailyCompanyStems, dailyCompanySectors, dailyCompanySuffixes, s.n.Companies)
	s.analyze("daily_seed_company")
	s.exec("company", `INSERT INTO company (id, display_name, legal_name, industry, size_band, owner_id, lifecycle,
	                                       address_city, address_country, source, captured_by, created_at)
	      SELECT c.id, c.stem || ' ' || c.suffix, c.stem || ' ' || c.suffix,
	             ($1::text[])[1 + c.i % cardinality($1::text[])],
	             (ARRAY['1-10','11-50','51-200','201-500','501-1000','1001-5000'])[1 + floor(random() * 6)::int],
	             c.owner_id,
	             (ARRAY['target','prospect','opportunity','customer','customer','former_customer'])[1 + floor(random() * 6)::int],
	             (ARRAY['Hamburg','München','Köln','Leipzig','London','Manchester','Hà Nội','Hồ Chí Minh','Đà Nẵng'])[1 + c.i % 9],
	             (ARRAY['DE','DE','DE','DE','GB','GB','VN','VN','VN'])[1 + c.i % 9],
	             'manual', 'human:' || c.owner_id, now() - random() * interval '730 days'
	      FROM daily_seed_company c ORDER BY c.i`, dailyIndustries)
	s.exec("company_domain", `INSERT INTO company_domain (company_id, domain, is_primary, source, captured_by)
	      SELECT c.id, c.domain, true, 'manual', 'human:' || c.owner_id FROM daily_seed_company c ORDER BY c.i`)
	s.analyze("company", "company_domain")
}

// contacts are spread round-robin over the companies, so contact i works at
// company ((i-1) mod companies)+1 and i±companies is a colleague there.
func (s dailySeeder) contacts() {
	firsts, lasts, languages := dailyContactNames()
	s.exec("contact ordinals", `CREATE TEMP TABLE daily_seed_contact AS
	      SELECT p.i, public.uuidv7() AS id, p.owner_id, p.company_i, p.private,
	             ($4::text[])[p.name_n] AS first_name, ($5::text[])[p.name_n] AS last_name,
	             ($6::text[])[p.name_n] AS lang,
	             lower(regexp_replace(public.f_unaccent(($4::text[])[p.name_n] || '.' || ($5::text[])[p.name_n]),
	                                  '[^A-Za-z0-9.]+', '', 'g')) || '.' || p.i || '@' || c.domain AS email
	      FROM (SELECT i, (i - 1) % $3 + 1 AS company_i,
	                   ($1::uuid[])[1 + floor(power(random(), $7) * cardinality($1::uuid[]))::int] AS owner_id,
	                   1 + floor(random() * cardinality($4::text[]))::int AS name_n,
	                   random() < $8 AS private
	            FROM generate_series(1, $2) AS i) p
	      JOIN daily_seed_company c ON c.i = p.company_i`,
		s.reps, s.n.Contacts, s.n.Companies, firsts, lasts, languages, dailyOwnerSkew, dailyPrivateShare)
	s.analyze("daily_seed_contact")
	s.exec("contact", `INSERT INTO contact (id, first_name, last_name, full_name, title, owner_id, visibility,
	                                       source, captured_by, created_at)
	      SELECT p.id, p.first_name, p.last_name, p.first_name || ' ' || p.last_name,
	             ($1::text[])[1 + p.i % cardinality($1::text[])], p.owner_id,
	             CASE WHEN p.private THEN 'owner' ELSE 'workspace' END,
	             'manual', 'human:' || p.owner_id, now() - random() * interval '730 days'
	      FROM daily_seed_contact p ORDER BY p.i`, dailyTitles)
	s.exec("contact_email", `INSERT INTO contact_email (contact_id, email, email_type, is_primary, source, captured_by)
	      SELECT p.id, p.email, 'work', true, 'manual', 'human:' || p.owner_id FROM daily_seed_contact p ORDER BY p.i`)
	s.analyze("contact", "contact_email")
}

func (s dailySeeder) employment() {
	s.execDeferringTrigger("employment", "relationship", "relationship_last_activity", `INSERT INTO relationship (kind, contact_id, company_id, is_current_primary,
	                                               employment_status, started_at, source, captured_by)
	      SELECT 'employment', p.id, c.id, true, 'current', (now() - random() * interval '3000 days')::date,
	             'manual', 'human:' || p.owner_id
	      FROM daily_seed_contact p JOIN daily_seed_company c ON c.i = p.company_i ORDER BY p.i`)
	s.analyze("relationship")
}

// deals are spread round-robin over companies like contacts, so deal c is
// company c's first deal; 60% open across the open stages, 25% won, 15% lost.
func (s dailySeeder) deals(stages apptest.SeededStages) {
	var open []string
	var currency string
	if err := s.owner.QueryRow(context.Background(), `
	      SELECT array_agg(id::text ORDER BY position),
	             coalesce((SELECT value #>> '{}' FROM setting WHERE key = 'installation.base_currency'), 'EUR')
	      FROM stage WHERE pipeline_id = $1 AND semantic = 'open' AND archived_at IS NULL`,
		stages.PipelineID).Scan(&open, &currency); err != nil {
		s.t.Fatalf("reading the seeded pipeline's open stages: %v", err)
	}
	s.exec("deal ordinals", `CREATE TEMP TABLE daily_seed_deal AS
	      SELECT d.i, public.uuidv7() AS id, d.company_i, c.id AS company_id, c.owner_id, c.stem,
	             CASE WHEN d.roll < 0.60 THEN 'open' WHEN d.roll < 0.85 THEN 'won' ELSE 'lost' END AS status,
	             d.word_n, d.amount, d.days, d.stage_n
	      FROM (SELECT i, (i - 1) % $1 + 1 AS company_i, random() AS roll,
	                   1 + floor(random() * 1000)::int AS word_n,
	                   (round(exp(ln(5000) + random() * ln(50))) * 100)::bigint AS amount,
	                   1 + floor(random() * 360)::int AS days, 1 + floor(random() * $3)::int AS stage_n
	            FROM generate_series(1, $2) AS i) d
	      JOIN daily_seed_company c ON c.i = d.company_i`, s.n.Companies, s.n.Deals, len(open))
	s.analyze("daily_seed_deal")
	s.exec("deal", `INSERT INTO deal (id, name, amount_minor, amount_minor_base, currency, fx_rate_to_base, fx_rate_date,
	                                 pipeline_id, stage_id, company_id, owner_id, status, lost_reason,
	                                 expected_close_date, closed_at, forecast_category, priority, commercial_motion,
	                                 source, captured_by, created_at)
	      SELECT d.id, d.stem || ' ' || ($1::text[])[1 + d.word_n % cardinality($1::text[])], d.amount, d.amount, $2,
	             CASE WHEN d.status <> 'open' THEN 1 END, CASE WHEN d.status <> 'open' THEN (now() - d.days * interval '1 day')::date END,
	             $3::uuid, CASE d.status WHEN 'open' THEN ($4::uuid[])[d.stage_n] WHEN 'won' THEN $5::uuid ELSE $6::uuid END,
	             d.company_id, d.owner_id, d.status,
	             CASE WHEN d.status = 'lost' THEN (ARRAY['price','timing','competitor','no decision'])[1 + d.word_n % 4] END,
	             CASE WHEN d.status = 'open' THEN current_date + (d.days / 2 + 1) ELSE (now() - d.days * interval '1 day')::date END,
	             CASE WHEN d.status <> 'open' THEN now() - d.days * interval '1 day' END,
	             CASE WHEN d.status = 'open' THEN (ARRAY['pipeline','best_case','commit'])[1 + d.word_n % 3] END,
	             (ARRAY['low','medium','high'])[1 + d.days % 3],
	             (ARRAY['new_business','renewal','upsell','expansion'])[1 + d.word_n % 4],
	             'manual', 'human:' || d.owner_id, now() - (d.days + 45) * interval '1 day'
	      FROM daily_seed_deal d ORDER BY d.i`,
		dailyDealWords, currency, stages.PipelineID, open, stages.Won, stages.Lost)
	s.analyze("deal")
}

// stakeholders name one or two contacts at the deal's company: deal c's
// company is company c, and contacts c and c+companies both work there.
func (s dailySeeder) stakeholders() {
	s.exec("deal stakeholders", `INSERT INTO relationship (kind, deal_id, contact_id, role, source, captured_by)
	      SELECT 'deal_stakeholder', d.id, p.id, ($2::text[])[k], 'manual', 'human:' || d.owner_id
	      FROM daily_seed_deal d CROSS JOIN generate_series(1, 2) AS k
	      JOIN daily_seed_contact p ON p.i = d.company_i + $1 * (k - 1)
	      WHERE k = 1 OR d.i % 2 = 0 ORDER BY d.i, k`,
		s.n.Companies, []string{dealrole.Champion, dealrole.EconomicBuyer})
	s.analyze("relationship")
}

func (s dailySeeder) leads() {
	firsts, lasts, _ := dailyContactNames()
	s.exec("lead", `INSERT INTO lead (full_name, email, title, company_name, status, score, owner_id, source, captured_by, created_at)
	      SELECT l.first || ' ' || l.last,
	             lower(regexp_replace(public.f_unaccent(l.first || '.' || l.last), '[^A-Za-z0-9.]+', '', 'g'))
	               || '.' || l.i || '@' || regexp_replace(lower(l.company), '[^a-z0-9]+', '-', 'g') || '.example',
	             ($4::text[])[1 + l.i % cardinality($4::text[])], l.company,
	             (ARRAY['new','new','contacted','engaged'])[1 + l.i % 4], floor(random() * 101)::int, l.owner_id,
	             'manual', 'human:' || l.owner_id, now() - random() * interval '365 days'
	      FROM (SELECT i, ($2::text[])[n] AS first, ($3::text[])[n] AS last,
	                   ($5::text[])[1 + floor(random() * cardinality($5::text[]))::int] AS company,
	                   ($1::uuid[])[1 + floor(power(random(), $6) * cardinality($1::uuid[]))::int] AS owner_id
	            FROM (SELECT i, 1 + floor(random() * cardinality($2::text[]))::int AS n
	                  FROM generate_series(1, $7) AS i) picked) l
	      ORDER BY l.i`,
		s.reps, firsts, lasts, dailyTitles, dailyLeadCompanies, dailyOwnerSkew, s.n.Leads)
	s.analyze("lead")
}

// projects run at companies taken one per owning rep in turn, owned by the
// company owner, so every rep who owns a company owns a project to open.
func (s dailySeeder) projects() {
	s.exec("project", `INSERT INTO project (name, key, company_id, owner_id, phase, started_at, target_end_date,
	                                       source, captured_by)
	      SELECT c.stem || ' ' || ($1::text[])[1 + c.i % cardinality($1::text[])], 'DP-' || c.i, c.id, c.owner_id,
	             (ARRAY['initiative','pursuing','delivering'])[1 + c.i % 3],
	             current_date - (c.i % 200), current_date + (c.i % 300), 'manual', 'human:' || c.owner_id
	      FROM (SELECT sc.*, row_number() OVER (PARTITION BY sc.owner_id ORDER BY sc.i) AS turn
	            FROM daily_seed_company sc) c
	      ORDER BY c.turn, c.i LIMIT $2`, dailyProjectWords, s.n.Projects)
	s.analyze("project")
}

// activities: 85% email in four-message threads with the contact's owning rep, 6% meetings,
// 4% notes, 1% calls, the rest open tasks. A thread ends outbound unless left waiting.
// Topics are the dailyTerms an activity names, drawn per thread so every reply names them too.
func (s dailySeeder) activities() {
	share := func(f float64) int { return int(math.Round(float64(s.n.Activities) * f)) }
	emails := share(0.85)
	meetings, notes, calls := emails+share(0.06), emails+share(0.06)+share(0.04), emails+share(0.06)+share(0.04)+share(0.01)
	threads := (emails + dailyThreadLength - 1) / dailyThreadLength
	// The non-final messages carry enough inbound mail that two thirds of all email is inbound.
	inbound := (float64(dailyThreadLength)*2/3 - dailyWaitingShare) / float64(dailyThreadLength-1)
	s.exec("thread ordinals", `CREATE TEMP TABLE daily_seed_thread AS
	      SELECT t, 1 + floor(random() * $1)::int AS contact_i,
	             ($2::int[])[1 + floor(random() * cardinality($2::int[]))::int] * 30 + random() * 30 + 2 AS days_back,
	             random() < $3 AS waiting, random() < $4 AS limited, floor(random() * 100000)::int AS subject_n,
	             -- Naming t makes the subquery correlated, so each thread draws its own topics.
	             ARRAY(SELECT k FROM generate_series(1, cardinality($6::float8[])) AS k
	                   WHERE random() < ($6::float8[])[k] AND t > 0) AS topics
	      FROM generate_series(1, $5) AS t`,
		s.n.Contacts, dailyMonths(), dailyWaitingShare, dailyLimitedShare, threads, dailyTermShares())
	s.analyze("daily_seed_thread")
	s.exec("activity ordinals", `CREATE TEMP TABLE daily_seed_activity AS
	      SELECT r.i, public.uuidv7() AS id, r.kind, r.p, r.paragraphs, coalesce(th.subject_n, r.subject_n) AS subject_n,
	             coalesce(th.topics, r.topics) AS topics, p.first_name AS contact_first, c.stem || ' ' || c.suffix AS company_name,
	             p.i AS contact_i, p.company_i, p.owner_id AS rep_id, rep.email AS rep_email, p.lang,
	             CASE WHEN r.kind = 'email' THEN 'daily-thread-' || r.t || '@bench.example' END AS thread_key,
	             CASE WHEN r.kind = 'email' AND (r.p = $6 - 1 OR r.i = $1)
	                    THEN CASE WHEN th.waiting THEN 'inbound' ELSE 'outbound' END
	                  WHEN r.kind = 'email' THEN CASE WHEN r.roll < $7 THEN 'inbound' ELSE 'outbound' END
	                  WHEN r.kind = 'call' THEN CASE WHEN r.roll < 0.3 THEN 'inbound' ELSE 'outbound' END END AS direction,
	             CASE WHEN r.kind = 'email'
	                    THEN now() - th.days_back * interval '1 day' + r.p * interval '14 hours' + r.jitter * interval '1 minute'
	                  WHEN r.kind = 'task' THEN now() - r.days_back / 30 * interval '1 day'
	                  ELSE now() - r.days_back * interval '1 day' END AS occurred_at,
	             CASE WHEN th.limited THEN 'participants' ELSE 'workspace' END AS audience,
	             r.deal_roll < $8 AS deal_link, CASE WHEN r.cc_roll < 0.5 THEN 1 ELSE 2 END AS cc
	      FROM (SELECT i,
	                   CASE WHEN i <= $1 THEN 'email' WHEN i <= $2 THEN 'meeting' WHEN i <= $3 THEN 'note'
	                        WHEN i <= $4 THEN 'call' ELSE 'task' END AS kind,
	                   (i - 1) / $6 + 1 AS t, (i - 1) % $6 AS p,
	                   1 + floor(random() * $9)::int AS contact_roll,
	                   ($10::int[])[1 + floor(random() * cardinality($10::int[]))::int] * 30 + random() * 30 + 1 AS days_back,
	                   random() AS roll, random() AS deal_roll, random() AS cc_roll, random() * 20 AS jitter,
	                   1 + floor(-ln(1 - random()) * 4)::int AS paragraphs, floor(random() * 100000)::int AS subject_n,
	                   ARRAY(SELECT k FROM generate_series(1, cardinality($11::float8[])) AS k
	                         WHERE random() < ($11::float8[])[k] AND i > 0) AS topics
	            FROM generate_series(1, $5) AS i) r
	      LEFT JOIN daily_seed_thread th ON r.kind = 'email' AND th.t = r.t
	      JOIN daily_seed_contact p ON p.i = coalesce(th.contact_i, r.contact_roll)
	      JOIN daily_seed_company c ON c.i = p.company_i
	      JOIN daily_seed_rep rep ON rep.id = p.owner_id`,
		emails, meetings, notes, calls, s.n.Activities, dailyThreadLength, inbound, dailyDealLinkShare,
		s.n.Contacts, dailyMonths(), dailyTermShares())
	s.analyze("daily_seed_activity")
	s.insertActivities()
}

// insertActivities writes the text: a subject names the first topic, else the company
// for some and plain words for the rest; a body greets the contact, names every topic, then reads on.
func (s dailySeeder) insertActivities() {
	termSubjects, termSentences := dailyTermPools()
	s.exec("activity", `INSERT INTO activity (id, kind, subject, body, occurred_at, due_at, assignee_id, direction,
	                                         meeting_status, duration_seconds, source_system, source_id, source,
	                                         captured_by, thread_key, audience, language, created_at)
	      SELECT a.id, a.kind,
	             CASE a.kind WHEN 'email' THEN CASE WHEN a.p > 0 THEN 'Re: ' ELSE '' END
	                         WHEN 'task' THEN ($2::text[])[1 + a.subject_n % cardinality($2::text[])] || ': '
	                         WHEN 'call' THEN 'Call: ' WHEN 'note' THEN 'Notiz: ' ELSE '' END
	             || CASE WHEN cardinality(a.topics) > 0 THEN ($3::text[])[(a.topics[1] - 1) * $5::int + 1 + a.subject_n % $5::int]
	                     WHEN a.subject_n % 1000 < $6::float8 * 1000 THEN a.company_name || ': ' || ($1::text[])[1 + a.subject_n % cardinality($1::text[])]
	                     ELSE ($1::text[])[1 + a.subject_n % cardinality($1::text[])] END,
	             concat_ws(E'\n\n',
	               CASE WHEN a.kind = 'email' THEN CASE a.lang WHEN 'de' THEN 'Hallo ' WHEN 'vi' THEN 'Chào ' ELSE 'Hi ' END
	                                               || a.contact_first || ',' END,
	               (SELECT string_agg(($4::text[])[(k - 1) * $5::int + 1 + floor(random() * $5::int)::int], ' ' ORDER BY k)
	                  FROM unnest(a.topics) AS k),
	               -- ORDER BY g.n is what makes the aggregate the subquery's: its arguments name only outer columns.
	               (SELECT string_agg(CASE a.lang WHEN 'de' THEN ($7::text[])[1 + floor(random() * cardinality($7::text[]))::int]
	                                              WHEN 'vi' THEN ($9::text[])[1 + floor(random() * cardinality($9::text[]))::int]
	                                              ELSE ($8::text[])[1 + floor(random() * cardinality($8::text[]))::int] END,
	                                  E'\n\n' ORDER BY g.n)
	                  FROM generate_series(1, CASE WHEN a.kind IN ('email', 'note') THEN least(a.paragraphs, 24) ELSE 1 END) AS g(n))),
	             a.occurred_at,
	             CASE WHEN a.kind = 'task' THEN now() + (a.subject_n % 35 - 7) * interval '1 day' END,
	             CASE WHEN a.kind = 'task' THEN a.rep_id END,
	             a.direction, CASE WHEN a.kind = 'meeting' THEN 'held' END,
	             CASE a.kind WHEN 'meeting' THEN 3600 WHEN 'call' THEN 600 END,
	             CASE WHEN a.kind = 'email' THEN 'email' END,
	             CASE WHEN a.kind = 'email' THEN 'daily-' || a.i || '@bench.example' END,
	             CASE WHEN a.kind = 'email' THEN 'email' ELSE 'manual' END,
	             'human:' || a.rep_id, a.thread_key, a.audience, a.lang, a.occurred_at
	      FROM daily_seed_activity a ORDER BY a.i`,
		dailyPlainSubjects(dailySubjectsPool), dailyTaskVerbs, termSubjects, termSentences, dailyTermPool,
		dailyNamedSubjects, dailyParagraphs("de", dailyParagraphsPool),
		dailyParagraphs("en", dailyParagraphsPool), dailyParagraphs("vi", dailyParagraphsPool))
}

// captureImports records each email as delivered to its rep's own mailbox,
// the evidence the content and attendance arms read.
func (s dailySeeder) captureImports() {
	s.exec("capture_import", `INSERT INTO capture_import (activity_id, user_id, imported_at)
	      SELECT a.id, a.rep_id, a.occurred_at FROM daily_seed_activity a WHERE a.kind = 'email' ORDER BY a.i`)
}

// participants writes capture's shape, which the Worklist reads: the rep by seat (no address),
// the rep's address on their side, the contact on the other, one or two colleagues on cc.
func (s dailySeeder) participants() {
	s.exec("activity_participant", `INSERT INTO activity_participant (activity_id, user_id, contact_id, address, role, display_name)
	      SELECT a.id, x.user_id, x.contact_id, x.address, x.role, x.display_name
	      FROM daily_seed_activity a
	      JOIN daily_seed_contact p ON p.i = a.contact_i
	      CROSS JOIN LATERAL (VALUES
	        (a.rep_id, NULL::uuid, NULL::text, CASE a.direction WHEN 'inbound' THEN 'to' ELSE 'from' END, NULL::text),
	        (NULL::uuid, NULL::uuid, a.rep_email, CASE a.direction WHEN 'inbound' THEN 'to' ELSE 'from' END, NULL::text),
	        (NULL::uuid, p.id, p.email, CASE a.direction WHEN 'inbound' THEN 'from' ELSE 'to' END, p.first_name || ' ' || p.last_name)
	      ) AS x(user_id, contact_id, address, role, display_name)
	      WHERE a.kind = 'email'
	      UNION ALL
	      SELECT a.id, NULL, cc.id, cc.email, 'cc', cc.first_name || ' ' || cc.last_name
	      FROM daily_seed_activity a CROSS JOIN generate_series(1, 2) AS k
	      JOIN daily_seed_contact cc ON cc.i = CASE WHEN a.contact_i + k * $1 <= $2 THEN a.contact_i + k * $1
	                                               ELSE a.contact_i - k * $1 END
	      WHERE a.kind = 'email' AND k <= a.cc`, s.n.Companies, s.n.Contacts)
}

// links file every activity under its contact, email/notes/tasks also under the employer,
// and 30% under its first deal. Meetings and calls never link a company: a trigger refuses it.
func (s dailySeeder) links() {
	s.execDeferringTrigger("activity_link", "activity_link", "activity_link_last_activity", `INSERT INTO activity_link (activity_id, entity_type, contact_id, company_id, deal_id)
	      SELECT a.id, 'contact', p.id, NULL::uuid, NULL::uuid
	      FROM daily_seed_activity a JOIN daily_seed_contact p ON p.i = a.contact_i
	      UNION ALL
	      SELECT a.id, 'company', NULL, c.id, NULL
	      FROM daily_seed_activity a JOIN daily_seed_company c ON c.i = a.company_i
	      WHERE a.kind IN ('email', 'note', 'task')
	      UNION ALL
	      SELECT a.id, 'deal', NULL, NULL, d.id
	      FROM daily_seed_activity a JOIN daily_seed_deal d ON d.i = a.company_i
	      WHERE a.deal_link`)
}

// pickCorpus takes each team A rep's first record of each kind by ordinal, so two runs
// open the same records; team A holds the median rep, who must own one of each.
func (s dailySeeder) pickCorpus(seats Seats) DailyCorpus {
	s.t.Helper()
	teamA := make([]string, 0, dailyRepsPerTeam)
	for _, rep := range seats.Reps[:dailyRepsPerTeam] {
		teamA = append(teamA, rep.UserID)
	}
	var c DailyCorpus
	if err := s.owner.QueryRow(context.Background(), `
	      SELECT r.id::text FROM daily_seed_rep r LEFT JOIN daily_seed_contact p ON p.owner_id = r.id
	      GROUP BY r.id, r.n ORDER BY count(p.i) DESC, r.n OFFSET $1 LIMIT 1`, (len(s.reps)-1)/2).Scan(&c.MedianRepID); err != nil {
		s.t.Fatalf("finding the median-ownership rep: %v", err)
	}
	if !slices.Contains(teamA, c.MedianRepID) {
		s.t.Fatalf("the median-ownership rep %s is not in team A; the flows measure team A", c.MedianRepID)
	}
	var medianOwns []string
	pick := func(kind, from string) (picked, names []string) {
		s.t.Helper()
		rows, err := s.owner.Query(context.Background(), `
		      SELECT picked.id::text, picked.name, rep.id = $2::uuid
		      FROM unnest($1::uuid[]) WITH ORDINALITY AS rep(id, n)
		      CROSS JOIN LATERAL (`+from+` LIMIT 1) picked ORDER BY rep.n`, teamA, c.MedianRepID)
		if err != nil {
			s.t.Fatalf("picking the corpus %s rows: %v", kind, err)
		}
		defer rows.Close()
		for rows.Next() {
			var id, name string
			var median bool
			if err := rows.Scan(&id, &name, &median); err != nil {
				s.t.Fatalf("picking the corpus %s rows: %v", kind, err)
			}
			picked, names = append(picked, id), append(names, name)
			if median {
				medianOwns = append(medianOwns, kind)
			}
		}
		if err := rows.Err(); err != nil || len(picked) == 0 {
			s.t.Fatalf("picking the corpus %s rows: %d picked, err %v", kind, len(picked), err)
		}
		return picked, names
	}
	c.DealIDs, c.DealNames = pick("deal", `SELECT dl.id, dl.name FROM daily_seed_deal sd JOIN deal dl ON dl.id = sd.id
	      WHERE sd.owner_id = rep.id AND sd.status = 'open' ORDER BY sd.i`)
	c.CompanyIDs, c.CompanyNames = pick("company", `SELECT co.id, co.display_name AS name FROM daily_seed_company sc
	      JOIN company co ON co.id = sc.id WHERE sc.owner_id = rep.id ORDER BY sc.i`)
	c.ContactIDs, _ = pick("contact", `SELECT p.id, p.first_name || ' ' || p.last_name AS name FROM daily_seed_contact p
	      WHERE p.owner_id = rep.id AND NOT p.private ORDER BY p.i`)
	c.ProjectIDs, _ = pick("project", `SELECT pr.id, pr.name FROM daily_seed_company sc
	      JOIN project pr ON pr.company_id = sc.id WHERE pr.owner_id = rep.id ORDER BY sc.i`)
	if missing := missingMedianKinds(medianOwns); len(missing) > 0 {
		s.t.Fatalf("the median rep owns no %s in the corpus; the flows would never open one of theirs", strings.Join(missing, ", "))
	}
	return c
}

// dailyMedianKinds are the records the flows open from the median rep's own.
var dailyMedianKinds = []string{"deal", "company", "contact", "project"}

func missingMedianKinds(owned []string) []string {
	return slices.DeleteFunc(slices.Clone(dailyMedianKinds), func(kind string) bool { return slices.Contains(owned, kind) })
}

// dailyCensus counts the tables the tier sizes. The anchor company is the
// workspace's own and no part of the corpus.
func dailyCensus(t *testing.T, e *apptest.AppEnv) dailyCounts {
	t.Helper()
	var c dailyCounts
	if err := e.Owner.QueryRow(context.Background(), `SELECT
	        (SELECT count(*) FROM contact), (SELECT count(*) FROM company WHERE NOT is_anchor),
	        (SELECT count(*) FROM deal), (SELECT count(*) FROM lead), (SELECT count(*) FROM project),
	        (SELECT count(*) FROM activity)`).
		Scan(&c.Contacts, &c.Companies, &c.Deals, &c.Leads, &c.Projects, &c.Activities); err != nil {
		t.Fatalf("counting the daily corpus: %v", err)
	}
	return c
}

// assertDailyCensus fails a short or long corpus before any timing: a statement that
// dropped rows would make every number describe a smaller tier than the record claims.
func assertDailyCensus(t *testing.T, e *apptest.AppEnv, want dailyCounts) {
	t.Helper()
	got := dailyCensus(t, e)
	var off []string
	for _, row := range []struct {
		table     string
		want, got int
	}{
		{"contact", want.Contacts, got.Contacts},
		{"company", want.Companies, got.Companies},
		{"deal", want.Deals, got.Deals},
		{"lead", want.Leads, got.Leads},
		{"project", want.Projects, got.Projects},
		{"activity", want.Activities, got.Activities},
	} {
		if row.got != row.want {
			off = append(off, fmt.Sprintf("%s: want %d, got %d", row.table, row.want, row.got))
		}
	}
	if len(off) > 0 {
		t.Fatalf("the daily corpus census is off, so its timings would describe another corpus: %s", strings.Join(off, "; "))
	}
}

// assertDailyContactScope: only owner-only contacts narrow the list, and only to their owner,
// so the median rep sees the manager's count plus exactly their own (managers own none).
func assertDailyContactScope(t *testing.T, e *apptest.AppEnv, seats Seats, corpus DailyCorpus) {
	t.Helper()
	rep := seats.Reps[slices.IndexFunc(seats.Reps, func(s Seat) bool { return s.UserID == corpus.MedianRepID })]
	manager := seats.Managers[0]
	var repPrivate, allPrivate int
	if err := e.Owner.QueryRow(context.Background(), `
	      SELECT count(*) FILTER (WHERE owner_id = $1::uuid), count(*) FROM contact
	      WHERE visibility = 'owner' AND archived_at IS NULL`, rep.UserID).Scan(&repPrivate, &allPrivate); err != nil {
		t.Fatalf("counting owner-only contacts: %v", err)
	}
	total := func(seat Seat) int {
		t.Helper()
		var page struct {
			Page struct {
				Total *int `json:"total"`
			} `json:"page"`
		}
		mustCall(t, seat.env(e), "GET", "/v1/contacts?limit=1", nil, http.StatusOK, &page)
		if page.Page.Total == nil {
			t.Fatalf("GET /v1/contacts as %s sent no page.total; the scope check needs the count", seat.Name)
		}
		return *page.Page.Total
	}
	repSees, managerSees := total(rep), total(manager)
	t.Logf("contacts visible: %s %d, %s %d (rep owns %d of %d owner-only)",
		rep.Name, repSees, manager.Name, managerSees, repPrivate, allPrivate)
	if repPrivate == 0 || repSees != managerSees+repPrivate || repSees >= managerSees+allPrivate {
		t.Fatalf("contact scope: %s sees %d and %s sees %d, want the rep to see the manager's %d plus their own %d owner-only contacts and none of the other %d",
			rep.Name, repSees, manager.Name, managerSees, managerSees, repPrivate, allPrivate-repPrivate)
	}
}
