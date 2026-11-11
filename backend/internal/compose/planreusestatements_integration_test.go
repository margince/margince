// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"maps"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose/attention"
	"github.com/margince/margince/backend/internal/compose/briefs"
	"github.com/margince/margince/backend/internal/compose/company360"
	"github.com/margince/margince/backend/internal/compose/contact360"
	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/capture/mailmap"
	"github.com/margince/margince/backend/internal/modules/collections"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// sentStatements records every statement a pool sends.
type sentStatements struct{ sql []string }

func (s *sentStatements) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	s.sql = append(s.sql, data.SQL)
	return ctx
}

func (*sentStatements) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

// predicateSpelling strips what a partial-index predicate says besides its
// columns: literals, casts, null tests (of a column or a call) and calls. A
// bound strict comparison proves the null test, so the generic plan keeps it.
var predicateSpelling = regexp.MustCompile(`'(?:[^']|'')*'|::\w+(?:\[\])?|\b\w+\s*\([^()]*\)\s+IS\s+(?:NOT\s+)?NULL\b|\b\w+\s+IS\s+(?:NOT\s+)?NULL\b|\b\w+\s*\(`)

// predicateWords are the SQL words left in a predicate once predicateSpelling ran.
var predicateWords = map[string]bool{
	"and": true, "or": true, "not": true, "any": true, "array": true,
	"is": true, "distinct": true, "from": true, "true": true, "false": true, "in": true, "null": true,
}

// boundIndexedColumns reads, from the migrated catalog, every column a partial
// index predicate compares with a value or reads as a boolean. A generic plan
// cannot see a bound value, so a read binding one of them loses that index.
// archived_at is added: live-row indexes test it with IS NULL, which a read
// must spell too.
func boundIndexedColumns(t *testing.T, pool *pgxpool.Pool) *regexp.Regexp {
	t.Helper()
	rows, err := pool.Query(context.Background(), `SELECT DISTINCT pg_get_expr(i.indpred, i.indrelid)
		FROM pg_index i JOIN pg_class c ON c.oid = i.indrelid JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE i.indpred IS NOT NULL AND n.nspname = 'public'`)
	if err != nil {
		t.Fatalf("reading partial index predicates: %v", err)
	}
	predicates, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatalf("reading partial index predicates: %v", err)
	}
	columns := map[string]bool{"archived_at": true}
	for _, predicate := range predicates {
		for _, word := range regexp.MustCompile(`[a-z_][a-z0-9_]*`).FindAllString(predicateSpelling.ReplaceAllString(predicate, " "), -1) {
			if !predicateWords[word] {
				columns[word] = true
			}
		}
	}
	for _, column := range []string{"kind", "direction", "role", "status", "audience", "entity_type", "meeting_status", "is_done"} {
		if !columns[column] {
			t.Fatalf("the partial-index census found no predicate on %s among %d predicates: it reads a smaller catalog than the schema has", column, len(predicates))
		}
	}
	return regexp.MustCompile(`(?i)\.(` + strings.Join(slices.Sorted(maps.Keys(columns)), "|") +
		`)\s*(=|<>|!=|IN|=\s*ANY)\s*\(?\s*\$\d+`)
}

// The reads that reuse generic plans spell every partial-index predicate as a
// literal. That lets a generic plan keep the index.
func TestTheReadsThatReuseGenericPlansBindNoPartialIndexPredicate(t *testing.T) {
	e := integration.Setup(t)
	sent := &sentStatements{}
	config := e.Pool.Config()
	config.ConnConfig.Tracer = sent
	pool, err := testdb.OwnPoolFromConfig(context.Background(), config)
	if err != nil {
		t.Fatalf("opening a traced pool: %v", err)
	}
	defer pool.Close()
	contact := e.SeedContact(t, "Planned Once", &e.Rep1)
	pipeline, open, _ := integration.DealFixture(t, e)
	deal := e.SeedDeal(t, "Advised Once", pipeline, open, &e.Rep1)
	subject, body, direction, at := "Following up", "Any news?", "outbound", clocktest.Now(t)
	mail, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "email", Subject: &subject, Body: &body, Direction: &direction, OccurredAt: &at, Source: "manual",
		EmailFrom: "rep@ourco.test", EmailTo: []string{"buyer@customer.test"},
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact}, {EntityType: "deal", EntityID: deal}},
	})
	if err != nil {
		t.Fatal(err)
	}
	seedPlannedMeeting(t, e, contact, at.Add(time.Hour))
	db := database.BindTo(pool, ids.From[ids.WorkspaceKind](e.WS))
	store := activities.NewStore(db)
	reader := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms)
	engine := briefs.NewBriefEngine(pool, contacts.NewStore(db))
	if _, err := engine.SnapshotRun(reader, at); err != nil {
		t.Fatalf("taking the rep's brief run: %v", err)
	}
	if err := database.WithWorkspaceSnapshot(reader, pool, func(ctx context.Context) error {
		if _, err := emailSummariesReusingPlans(ctx, store, []ids.UUID{ids.UUID(mail.Id)}, at); err != nil {
			return err
		}
		if _, err := (attentionTasks{store: store}).RemindersForViewer(ctx, at); err != nil {
			return err
		}
		if _, _, err := (attentionAwaiting{store: store}).AwaitingReplies(ctx, at); err != nil {
			return err
		}
		advice := attentionDealMoves{cards: newDealStatusService(pool, func() time.Time { return at })}
		if _, err := advice.CurrentMoves(ctx, []ids.UUID{deal}); err != nil {
			return err
		}
		if err := readSuggestionLanes(ctx, db); err != nil {
			return err
		}
		if err := readPlannedOnceLanes(ctx, plannedOnceLanes{db: db, pool: pool, engine: engine}, deal, ids.UUID(mail.Id), at); err != nil {
			return err
		}
		if err := readMeetingLanes(ctx, store, at, e.Rep1); err != nil {
			return err
		}
		return database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
			if _, err := contact360.LastTouchFor(ctx, tx, []ids.ContactID{ids.From[ids.ContactKind](contact)}, contact360.AssembleOptions{AsOf: at}); err != nil {
				return err
			}
			return activities.WithCommitmentAdmissions(ctx, tx, []crmcontracts.Activity{mail})
		})
	}); err != nil {
		t.Fatalf("running the reads: %v", err)
	}
	inbound, quote := "inbound", "Could you send the quote?"
	account := ids.From[ids.CompanyKind](e.SeedEmployedAccount(t, contact, e.Rep1, pipeline, open, activities.LogActivityInput{
		Kind:    "email",
		Subject: &subject, Body: &quote, Direction: &inbound, OccurredAt: &at, Source: "manual", EmailFrom: "buyer@customer.test", EmailTo: []string{"rep@ourco.test"},
	}))
	page := company360.NewService(pool, e.Contacts, e.Deals, e.Projects, approvals.NewService(e.DB()), func() time.Time { return at })
	if _, err := page.Assemble(reader, account); err != nil {
		t.Fatalf("opening the account page: %v", err)
	}
	sent.sql = append(sent.sql, capturedPartyPlanStatements(t, e, pool)...)
	boundIndexedColumn := boundIndexedColumns(t, e.Pool)
	for _, bad := range []string{"WHERE a.kind = $3", "AND p.role IN ($2", "AND m.status = ANY($7)", "AND a.is_done = $4"} {
		if !boundIndexedColumn.MatchString(bad) {
			t.Fatalf("the check misses %q, so it would pass a bound predicate", bad)
		}
	}
	if len(sent.sql) == 0 {
		t.Fatal("no statement was sent, so there is nothing to hold")
	}
	for _, held := range []string{
		"FROM activity_commitment_admission d", "FROM deal_suggestion s", "FROM tag_suggestion s",
		"FROM brief_item bi", "'deal_stakeholder'", "wd.source = 'domain_question'", "task_claims AS MATERIALIZED",
		"a.meeting_status IS NULL OR a.meeting_status = 'booked'", "NOT (a.captured_by = 'system'",
	} {
		if !slices.ContainsFunc(sent.sql, func(sql string) bool { return strings.Contains(sql, held) }) {
			t.Fatalf("no statement %s was sent, so the check does not hold that read", held)
		}
	}
	for _, sql := range sent.sql {
		if found := boundIndexedColumn.FindString(sql); found != "" {
			t.Errorf("a statement under generic plans binds %q; spell it as a literal or leave the read out of plan reuse:\n%s", found, sql)
		}
	}
}

// plannedOnceLanes is what the Worklist's other plan-reusing reads are built on.
type plannedOnceLanes struct {
	db     *database.DB
	pool   *pgxpool.Pool
	engine *briefs.BriefEngine
}

// readPlannedOnceLanes runs the Worklist's other plan-reusing reads through
// their seams. They are the task flags and their claims, the domain questions,
// the champion cover and the brief run items. readMeetingLanes runs the
// meetings.
func readPlannedOnceLanes(ctx context.Context, lanes plannedOnceLanes, deal, task ids.UUID, at time.Time) error {
	store := activities.NewStore(lanes.db)
	claims := contacts.NewStore(lanes.db)
	if _, err := (attentionTasks{store: store, claims: claims}).markReadOnly(ctx, []attention.Task{{ID: task}}, at); err != nil {
		return err
	}
	if _, err := (attentionDomainQuestions{store: claims}).OpenDomainQuestions(ctx); err != nil {
		return err
	}
	if _, err := (attentionAtRisk{pool: lanes.pool}).championCover(ctx, []agents.SlippingDeal{{DealID: deal}}, at); err != nil {
		return err
	}
	briefing := attentionBriefing{engine: lanes.engine, figures: attentionDealFacts{store: deals.NewStore(lanes.db, DealsInstallation())}}
	_, _, _, err := briefing.Queue(ctx, at)
	return err
}

// readSuggestionLanes runs the needs_you lane's four suggestion reads through
// the seams that reuse their plans.
func readSuggestionLanes(ctx context.Context, db *database.DB) error {
	dealSuggestions := attentionDealSuggestions{store: deals.NewStore(db, DealsInstallation())}
	if _, err := dealSuggestions.OpenSuggestions(ctx, nil, 10); err != nil {
		return err
	}
	if _, err := dealSuggestions.CountOpen(ctx, nil); err != nil {
		return err
	}
	tagSuggestions := attentionTagSuggestions{store: collections.NewStore(db)}
	if _, err := tagSuggestions.OpenTagSuggestions(ctx, 10); err != nil {
		return err
	}
	_, err := tagSuggestions.CountOpenTagSuggestions(ctx)
	return err
}

// seedPlannedMeeting books a meeting with the contact at the given start, so the
// meeting lanes read a row and finish it.
func seedPlannedMeeting(t *testing.T, e *integration.Env, contact ids.UUID, startsAt time.Time) {
	t.Helper()
	subject, status := "Kick-off", "booked"
	if _, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "meeting", Subject: &subject, MeetingStatus: &status, OccurredAt: &startsAt, Source: "manual",
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact}},
	}); err != nil {
		t.Fatalf("book the meeting: %v", err)
	}
}

// readMeetingLanes runs the meetings lanes' reads through the seams that reuse
// their plans, under every scope a lane can be asked for.
func readMeetingLanes(ctx context.Context, store *activities.Store, at time.Time, owner ids.UUID) error {
	window := attention.MeetingWindow{From: at.Add(-24 * time.Hour), Until: at.Add(12 * time.Hour), DaysFrom: at, DaysUntil: at.AddDate(0, 0, 1)}
	for _, scope := range []attention.TaskScope{attention.TasksVisible, attention.TasksMine, attention.TasksUnassigned, attention.TasksOwnedBy} {
		if _, _, err := (attentionMeetings{store: store}).Today(ctx, window, 5, scope, owner); err != nil {
			return err
		}
		if _, err := (attentionMeetingsAwaitingOutcome{store: store}).Since(ctx, window, 5, scope, owner); err != nil {
			return err
		}
	}
	_, _, err := (attentionMeetingHorizon{store: store}).NextCustomerMeeting(ctx, window)
	return err
}

func capturedPartyPlanStatements(t *testing.T, e *integration.Env, pool *pgxpool.Pool) []string {
	t.Helper()
	authorizeCaptureMailbox(t, e, e.Rep1, "planner@ourco.test")
	sent := &sentStatements{}
	config := pool.Config()
	config.ConnConfig.Tracer = sent
	traced, err := testdb.OwnPoolFromConfig(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	defer traced.Close()
	raw := []byte("From: Pat Buyer <plan@buyer.test>\r\nTo: planner@ourco.test\r\nMessage-ID: <plan-proof@buyer.test>\r\nSubject: Project\r\nDate: Wed, 04 Jun 2026 08:00:00 +0000\r\n\r\nHello\r\n")
	mail, err := mailmap.Parse(raw, "planner@ourco.test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := newCaptureSink(traced, CaptureConfig{}).Upsert(mailboxOwnerCtx(e, e.Rep1), mail.ToRecord("gmail", raw)); err != nil {
		t.Fatal(err)
	}
	var proof []string
	generic := false
	for _, query := range sent.sql {
		if strings.Contains(query, "set_config('plan_cache_mode', 'force_generic_plan'") {
			generic = true
		}
		if strings.HasPrefix(query, "SELECT set_config('plan_cache_mode', $1") {
			generic = false
		}
		if strings.Contains(query, "SELECT a.id, a.occurred_at, coalesce(a.subject,'')") {
			if !generic {
				t.Fatal("party proof did not run under generic plans")
			}
			proof = append(proof, query)
		}
	}
	if len(proof) == 0 {
		t.Fatal("capture sent no party-proof statement")
	}
	return proof
}
