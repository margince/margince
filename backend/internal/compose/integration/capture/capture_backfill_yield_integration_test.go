// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

// What a backfill reports as its reach must be what IT created. The hero number
// on the activation screen is the user's evidence that the import was worth its
// spend, so it is counted by the pages that did the work — not inferred from
// every connector-created row that happens to share the run's clock window.

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/capture/mailmap"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// mailPageConnector serves its whole fixture as ONE backfill page, through the
// production mailmap → Sink path — so the counterparties the run yields are the
// ones the real resolver creates, not seeded rows standing in for them.
type mailPageConnector struct {
	raws [][]byte
	sent map[string]bool // Message-IDs the provider filed as the owner's own sent mail
	// afterMessage hands control to the test once a message has been captured
	// and its counterparties resolved, so the live tally is observable MID-page.
	afterMessage func()
	// failAfterMessages > 0 abandons the page transiently once that many
	// messages have been captured — the retryable fault whose counterparty
	// yields no later attempt can re-count.
	failAfterMessages int
	// parallel walks the page's messages at once instead of in order. Nothing
	// in the connector contract forbids it, and what the counters do under it
	// is the question TestConcurrentCreationsAreAllCounted asks.
	parallel bool
	// prelude is walked SERIALLY, and to completion, before the parallel batch
	// starts. It exists because "at once" is two different races and only one
	// of them is ever the subject: eight creations recomputing one run's reach
	// is the race under test, while a reply overtaking the send it answers is
	// the ladder deferring correctly. Racing both meant a loaded runner could
	// defer every counterparty and leave nothing to count — the test then
	// failing for the reason it says in its own comment is not a failure.
	prelude [][]byte
}

func (m *mailPageConnector) Descriptor() connector.Descriptor {
	return connector.Descriptor{
		Name: "gmail", Version: "1",
		Scopes:   []principal.Scope{principal.ScopeRead},
		RiskTier: mcp.TierAutoExecute,
		Produces: []datasource.EntityType{datasource.EntityActivity},
	}
}

func (m *mailPageConnector) Authenticate(context.Context, connector.AuthRequest) (connector.Auth, error) {
	return connector.Auth("token"), nil
}

func (m *mailPageConnector) Sync(_ context.Context, _ connector.Auth, cursor connector.Cursor, _ connector.Sink) (connector.Cursor, error) {
	return cursor, nil
}

func (m *mailPageConnector) Normalize(context.Context, connector.RawRecord) ([]connector.NormalizedRecord, error) {
	return nil, connector.ErrSkip
}

func (m *mailPageConnector) HealthCheck(context.Context, connector.Auth) error { return nil }

func (m *mailPageConnector) EstimateBackfill(context.Context, connector.Auth, time.Time) (int, error) {
	return len(m.raws), nil
}

func (m *mailPageConnector) BackfillPage(ctx context.Context, _ connector.Auth, _ time.Time, _ string, sink connector.Sink) (connector.BackfillPageResult, error) {
	if m.parallel {
		return m.walkAtOnce(ctx, sink)
	}
	res := connector.BackfillPageResult{}
	for _, raw := range m.raws {
		msg, err := mailmap.Parse(raw, captureOwner)
		if err != nil {
			return connector.BackfillPageResult{}, err
		}
		res.Scanned++
		if _, drop := msg.SkipReason(); drop {
			res.Skipped++
			continue
		}
		msg = msg.AttestSentByOwner(m.sent[msg.ID()])
		// Reported BEFORE the capture, which the seam permits and the shipped
		// connectors do not do. That is the point: reporting afterwards would
		// carry the counterparty counts along with it, and this fixture exists
		// to prove the Sink's own flush surfaces them without help.
		connector.BackfillProgressFrom(ctx).Observed(ctx, res.Scanned, res.Captured, res.Skipped)
		if _, err := sink.Upsert(ctx, msg.ToRecord("gmail", raw)); err != nil {
			return connector.BackfillPageResult{}, err
		}
		res.Captured++
		if m.afterMessage != nil {
			m.afterMessage()
		}
		if m.failAfterMessages > 0 && res.Captured == m.failAfterMessages {
			return connector.BackfillPageResult{}, &connector.RateLimitedError{}
		}
	}
	return res, nil
}

// walkAtOnce captures every message in the page concurrently, which is what the
// counters have to survive: they are written per creation, from whichever
// goroutine made it.
func (m *mailPageConnector) walkAtOnce(ctx context.Context, sink connector.Sink) (connector.BackfillPageResult, error) {
	var mu sync.Mutex
	var walking sync.WaitGroup
	res := connector.BackfillPageResult{}
	var failed error
	// Everything the parallel batch needs already committed, in order, first.
	for _, raw := range m.prelude {
		msg, err := mailmap.Parse(raw, captureOwner)
		if err != nil {
			return connector.BackfillPageResult{}, err
		}
		msg = msg.AttestSentByOwner(m.sent[msg.ID()])
		if _, err := sink.Upsert(ctx, msg.ToRecord("gmail", raw)); err != nil {
			return connector.BackfillPageResult{}, err
		}
		res.Scanned++
		res.Captured++
	}
	for _, raw := range m.raws {
		walking.Add(1)
		go func() {
			defer walking.Done()
			msg, err := mailmap.Parse(raw, captureOwner)
			if err == nil {
				msg = msg.AttestSentByOwner(m.sent[msg.ID()])
				_, err = sink.Upsert(ctx, msg.ToRecord("gmail", raw))
			}
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				failed = err
				return
			}
			res.Scanned++
			res.Captured++
		}()
	}
	walking.Wait()
	if failed != nil {
		return connector.BackfillPageResult{}, failed
	}
	return res, nil
}

func TestBackfillCountsOnlyTheCounterpartiesItsOwnPagesCreated(t *testing.T) {
	e := integration.SetupSearch(t)
	seedCaptureRole(t, e)
	// The production wiring, because the counters under test are filled by the
	// real auto-create resolver: a bare Sink creates nothing to count.
	registry := compose.NewCaptureRegistry(e.Pool, newTestKeyvault(t, e), compose.CaptureConfig{})
	registry.Register(&mailPageConnector{
		raws: [][]byte{
			// Three recipients the workspace has EXCHANGED mail with: it wrote
			// to each, and each wrote back on that thread. One send alone is
			// intent and often unreturned, so it defers to the verdict; a reply
			// is the exchange that makes a contact without asking. All three
			// share ONE corporate domain, and a domain is asked about once —
			// three people, one company question.
			email(captureOwner, "", "alice@globex.example", "y1@myco.example", ""),
			email("alice@globex.example", "Alice", captureOwner, "y1r@globex.example", "y1@myco.example"),
			email(captureOwner, "", "bob@globex.example", "y2@myco.example", ""),
			email("bob@globex.example", "Bob", captureOwner, "y2r@globex.example", "y2@myco.example"),
			email(captureOwner, "", "dave@globex.example", "y3@myco.example", ""),
			email("dave@globex.example", "Dave", captureOwner, "y3r@globex.example", "y3@myco.example"),
			// A free-mail sender on the same page, contributing to NEITHER
			// counter: it defers to the verdict rather than minting, and a
			// consumer domain is never a company question. Without it both
			// assertions below would pass for a trivial reason — three people
			// on one domain is one domain however the counters work.
			email("zoe@gmail.com", "Zoe Example", captureOwner, "y4@gmail.com", ""),
		},
		sent: map[string]bool{
			"y1@myco.example": true,
			"y2@myco.example": true,
			"y3@myco.example": true,
		},
	})

	grantCtx := humanWithScopes(e, e.Rep1, []principal.Scope{principal.ScopeRead})
	if _, err := registry.Connect(grantCtx, "gmail", connector.Auth("refresh")); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	rep := ids.From[ids.UserKind](e.Rep1)
	run, err := registry.StartBackfill(grantCtx, "gmail", rep, 6, 3, enqueueNothing)
	if err != nil {
		t.Fatalf("StartBackfill: %v", err)
	}

	// What must NOT be credited to this run, all landing inside its clock
	// window: another gmail connection's captures, and a human's own typing.
	seedForeignCounterparties(t, e)

	wsCtx := principal.WithWorkspaceID(context.Background(), e.WS)
	done, _, retryAfter, err := registry.RunBackfillStep(wsCtx, run.ID)
	if err != nil || !done || retryAfter != 0 {
		t.Fatalf("the single page must finish the run: done=%v retryAfter=%v err=%v", done, retryAfter, err)
	}

	status, err := registry.BackfillStatus(grantCtx, "gmail", rep)
	if err != nil || status == nil {
		t.Fatalf("BackfillStatus: %v (run=%v)", err, status)
	}
	if status.People != 3 {
		t.Fatalf("people = %d, want 3 — the three attested recipients and NOT the free-mail sender, which defers", status.People)
	}
	// The company column counts domains this run QUEUED for a verdict, not
	// companies it created — capture creates none. The three recipients share
	// one domain, and a domain is asked about once; gmail.com asks nothing,
	// which is what makes this a count of QUESTIONS rather than of senders.
	if status.Organizations != 1 {
		t.Fatalf("company questions = %d, want 1 — one corporate domain asks, gmail.com does not", status.Organizations)
	}

	// The persisted columns are the proof the run counted at page-commit time
	// rather than the read inferring it: BackfillYields and the cost estimator
	// read these, and a live query could never serve them.
	people, orgs := readBackfillYieldColumns(t, e, run.ID)
	if people != 3 || orgs != 1 {
		t.Fatalf("stored people_created=%d company questions=%d, want 3/1", people, orgs)
	}
}

func TestBackfillYieldsAreVisibleWhileThePageRuns(t *testing.T) {
	// The counterparty half of the live tally. The Sink counts a person or an
	// organization as it creates one, so the two numbers beside "emails
	// captured" have to move during the page as well — a screen where only the
	// mail count advances tells the user the import found nobody.
	e := integration.SetupSearch(t)
	seedCaptureRole(t, e)
	prov := &mailPageConnector{
		raws: [][]byte{
			// Two attested own-sends on ONE corporate domain: two people by
			// demonstrated intent, and one company question between them.
			email(captureOwner, "", "alice@globex.example", "y1@myco.example", ""),
			email("alice@globex.example", "Alice", captureOwner, "y1r@globex.example", "y1@myco.example"),
			email(captureOwner, "", "dave@globex.example", "y3@myco.example", ""),
			email("dave@globex.example", "Dave", captureOwner, "y3r@globex.example", "y3@myco.example"),
		},
		sent: map[string]bool{"y1@myco.example": true, "y3@myco.example": true},
	}
	// The production wiring, because the counters under test are filled by the
	// real auto-create resolver. Unpaced, because a two-message fixture walks
	// well inside one pacing window.
	registry := compose.NewCaptureRegistry(e.Pool, newTestKeyvault(t, e), compose.CaptureConfig{}).
		WithProgressPacing(0)
	registry.Register(prov)

	grantCtx := humanWithScopes(e, e.Rep1, []principal.Scope{principal.ScopeRead})
	if _, err := registry.Connect(grantCtx, "gmail", connector.Auth("refresh")); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	rep := ids.From[ids.UserKind](e.Rep1)
	run, err := registry.StartBackfill(grantCtx, "gmail", rep, 6, 2, enqueueNothing)
	if err != nil {
		t.Fatalf("StartBackfill: %v", err)
	}

	var midPagePeople, midPageOrganizations int
	prov.afterMessage = func() {
		status, err := registry.BackfillStatus(grantCtx, "gmail", rep)
		if err != nil || status == nil {
			t.Fatalf("mid-page status read: %v (run=%v)", err, status)
		}
		midPagePeople, midPageOrganizations = status.People, status.Organizations
	}

	wsCtx := principal.WithWorkspaceID(context.Background(), e.WS)
	if done, _, _, err := registry.RunBackfillStep(wsCtx, run.ID); err != nil || !done {
		t.Fatalf("the single page must finish the run: done=%v err=%v", done, err)
	}

	// Read after the LAST message but before the commit: both counterparty
	// kinds were already visible.
	if midPagePeople != 2 {
		t.Fatalf("mid-page people = %d, want 2 — both attested recipients, before the page committed", midPagePeople)
	}
	if midPageOrganizations != 1 {
		t.Fatalf("mid-page company questions = %d, want 1 — the corporate domain, before the page committed", midPageOrganizations)
	}

	status, err := registry.BackfillStatus(grantCtx, "gmail", rep)
	if err != nil || status == nil {
		t.Fatalf("BackfillStatus: %v (run=%v)", err, status)
	}
	if status.People != 2 || status.Organizations != 1 {
		t.Fatalf("after the commit = %d people / %d organizations, want exactly the page's 2/1", status.People, status.Organizations)
	}
}

// seedForeignCounterparties lands the rows a workspace-wide, clock-windowed
// count would wrongly credit to the run under test: three counterparties
// indistinguishable from a second gmail connection's captures, and one person a
// human typed in.
func seedForeignCounterparties(t *testing.T, e *integration.SearchEnv) {
	t.Helper()
	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		for _, q := range []string{
			`INSERT INTO person (full_name, source, captured_by)
			   VALUES ('Other Connection One', 'capture', 'connector:gmail'),
			          ('Other Connection Two', 'capture', 'connector:gmail'),
			          ('Other Connection Three', 'capture', 'connector:gmail')`,
			`INSERT INTO person (full_name, source, captured_by)
			   VALUES ('Manually Typed', 'manual', 'human:someone')`,
			`INSERT INTO organization (display_name, source, captured_by)
			   VALUES ('Other Connection Co', 'capture', 'connector:gmail')`,
		} {
			if _, execErr := tx.Exec(e.Admin(), q); execErr != nil {
				return execErr
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("seed foreign counterparties: %v", err)
	}
}

func readBackfillYieldColumns(t *testing.T, e *integration.SearchEnv, id ids.UUID) (people, organizations int) {
	t.Helper()
	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(e.Admin(), `
			SELECT people_created, organizations_created FROM capture_backfill WHERE id = $1`, id).
			Scan(&people, &organizations)
	})
	if err != nil {
		t.Fatal(err)
	}
	return people, organizations
}

func TestBackfillYieldsSurviveATransientFault(t *testing.T) {
	// A page that fails transiently is retried from the committed cursor, and
	// capture is idempotent: every message it already captured replays as
	// created=false and never reaches the counterparty resolver again. So the
	// people and companies the failed attempt minted are counted by that
	// attempt or by nobody — dropping them with the rest of its tally
	// undercounts the run for good.
	e := integration.SetupSearch(t)
	seedCaptureRole(t, e)
	prov := &mailPageConnector{
		raws: [][]byte{
			// Two attested own-sends on ONE corporate domain: two people by
			// demonstrated intent, and one company question between them.
			email(captureOwner, "", "alice@globex.example", "y1@myco.example", ""),
			email("alice@globex.example", "Alice", captureOwner, "y1r@globex.example", "y1@myco.example"),
			email(captureOwner, "", "dave@globex.example", "y3@myco.example", ""),
			email("dave@globex.example", "Dave", captureOwner, "y3r@globex.example", "y3@myco.example"),
		},
		sent: map[string]bool{"y1@myco.example": true, "y3@myco.example": true},
	}
	registry := compose.NewCaptureRegistry(e.Pool, newTestKeyvault(t, e), compose.CaptureConfig{}).
		WithProgressPacing(0)
	registry.Register(prov)

	grantCtx := humanWithScopes(e, e.Rep1, []principal.Scope{principal.ScopeRead})
	if _, err := registry.Connect(grantCtx, "gmail", connector.Auth("refresh")); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	rep := ids.From[ids.UserKind](e.Rep1)
	run, err := registry.StartBackfill(grantCtx, "gmail", rep, 6, 2, enqueueNothing)
	if err != nil {
		t.Fatalf("StartBackfill: %v", err)
	}

	// Fail the page after both exchanges have been captured and resolved, which
	// is the worst case: every counterparty exists, and none of them will be
	// offered to the resolver again. Four messages, because an exchange is a
	// send and its reply — one send alone defers to the verdict and creates
	// nobody.
	prov.failAfterMessages = 4
	wsCtx := principal.WithWorkspaceID(context.Background(), e.WS)
	if _, _, retryAfter, err := registry.RunBackfillStep(wsCtx, run.ID); err == nil || retryAfter <= 0 {
		t.Fatalf("the page must fail transiently and ask for a retry: retryAfter=%v err=%v", retryAfter, err)
	}

	// Promoted into the committed columns by the fault, not discarded with the
	// message tally.
	people, orgs := readBackfillYieldColumns(t, e, run.ID)
	if people != 2 || orgs != 1 {
		t.Fatalf("after the transient fault people_created=%d company questions=%d, want 2/1 — the work happened and no retry will count it again", people, orgs)
	}

	// The retry replays both messages, mints nothing, and must not inflate the
	// count it inherited.
	prov.failAfterMessages = 0
	if _, _, _, err := registry.RunBackfillStep(wsCtx, run.ID); err != nil {
		t.Fatalf("retry: %v", err)
	}
	status, err := registry.BackfillStatus(grantCtx, "gmail", rep)
	if err != nil || status == nil {
		t.Fatalf("BackfillStatus: %v (run=%v)", err, status)
	}
	if status.People != 2 || status.Organizations != 1 {
		t.Fatalf("after the retry = %d people / %d organizations, want 2/1 — counted once, by the attempt that minted them", status.People, status.Organizations)
	}
}

// yieldFixture is the two-message fixture both edge cases below use: two
// attested own-sends to two people on ONE corporate domain, so a correct credit
// is exactly 2 people / 1 company question.
func yieldFixture() [][]byte {
	return [][]byte{
		email(captureOwner, "", "alice@globex.example", "y1@myco.example", ""),
		email("alice@globex.example", "Alice", captureOwner, "y1r@globex.example", "y1@myco.example"),
		email(captureOwner, "", "dave@globex.example", "y3@myco.example", ""),
		email("dave@globex.example", "Dave", captureOwner, "y3r@globex.example", "y3@myco.example"),
	}
}

func TestBackfillYieldsSurviveACancelUnderTheRunningPage(t *testing.T) {
	// The user stops the import while the page is still walking. Every write
	// that carries the run's STATE is fenced on the live statuses, so after the
	// cancel none of them match — and the page's counterparties, which exist
	// and which no replay will offer again, were credited by nobody.
	e := integration.SetupSearch(t)
	seedCaptureRole(t, e)
	prov := &mailPageConnector{raws: yieldFixture(), sent: map[string]bool{"y1@myco.example": true, "y3@myco.example": true}}
	registry := compose.NewCaptureRegistry(e.Pool, newTestKeyvault(t, e), compose.CaptureConfig{}).
		WithProgressPacing(0)
	registry.Register(prov)

	grantCtx := humanWithScopes(e, e.Rep1, []principal.Scope{principal.ScopeRead})
	if _, err := registry.Connect(grantCtx, "gmail", connector.Auth("refresh")); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	rep := ids.From[ids.UserKind](e.Rep1)
	run, err := registry.StartBackfill(grantCtx, "gmail", rep, 6, 2, enqueueNothing)
	if err != nil {
		t.Fatalf("StartBackfill: %v", err)
	}

	cancelled := false
	prov.afterMessage = func() {
		if cancelled {
			return
		}
		cancelled = true
		if _, err := registry.CancelBackfill(grantCtx, "gmail", rep); err != nil {
			t.Fatalf("CancelBackfill mid-page: %v", err)
		}
	}

	wsCtx := principal.WithWorkspaceID(context.Background(), e.WS)
	if _, _, _, err := registry.RunBackfillStep(wsCtx, run.ID); err != nil {
		t.Fatalf("the cancelled page's step: %v", err)
	}

	status, people, orgs := readRunAndYields(t, e, run.ID)
	if status != "cancelled" {
		t.Fatalf("status = %s, want cancelled", status)
	}
	if people != 2 || orgs != 1 {
		t.Fatalf("after a mid-page cancel people_created=%d organizations_created=%d, want 2/1 — the rows exist and no retry will offer them again", people, orgs)
	}
}

func TestBackfillYieldsAreCreditedOnceAtTheRetryCeiling(t *testing.T) {
	// A page that fails transiently AT the ceiling runs two writes: the
	// failure ladder and the terminal one. When both credited the yields, the
	// run reported twice the people it actually created.
	e := integration.SetupSearch(t)
	seedCaptureRole(t, e)
	prov := &mailPageConnector{
		raws: yieldFixture(),
		sent: map[string]bool{"y1@myco.example": true, "y3@myco.example": true},
		// Fail after both counterparties exist, so a double credit is visible.
		failAfterMessages: 4,
	}
	registry := compose.NewCaptureRegistry(e.Pool, newTestKeyvault(t, e), compose.CaptureConfig{}).
		WithProgressPacing(0)
	registry.Register(prov)

	grantCtx := humanWithScopes(e, e.Rep1, []principal.Scope{principal.ScopeRead})
	if _, err := registry.Connect(grantCtx, "gmail", connector.Auth("refresh")); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	rep := ids.From[ids.UserKind](e.Rep1)
	run, err := registry.StartBackfill(grantCtx, "gmail", rep, 6, 2, enqueueNothing)
	if err != nil {
		t.Fatalf("StartBackfill: %v", err)
	}
	// Sit the run ON the ceiling, so this page's fault is the one that ends it.
	seedBackfillFailuresAtCeiling(t, e, run.ID)

	wsCtx := principal.WithWorkspaceID(context.Background(), e.WS)
	done, _, retryAfter, err := registry.RunBackfillStep(wsCtx, run.ID)
	if err == nil {
		t.Fatal("the page must surface its fault")
	}
	if !done || retryAfter != 0 {
		t.Fatalf("at the ceiling the run ends rather than retrying (done=%v retryAfter=%v)", done, retryAfter)
	}
	status, people, orgs := readRunAndYields(t, e, run.ID)
	if status != "error" {
		t.Fatalf("status = %s, want error — the ceiling ends the run", status)
	}
	if people != 2 || orgs != 1 {
		t.Fatalf("at the ceiling people_created=%d organizations_created=%d, want 2/1 counted ONCE", people, orgs)
	}
}

// readRunAndYields reads the run's state and its committed yield columns.
func readRunAndYields(t *testing.T, e *integration.SearchEnv, id ids.UUID) (status string, people, organizations int) {
	t.Helper()
	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(e.Admin(), `
			SELECT status, people_created, organizations_created FROM capture_backfill WHERE id = $1`, id).
			Scan(&status, &people, &organizations)
	})
	if err != nil {
		t.Fatal(err)
	}
	return status, people, organizations
}

// seedBackfillFailuresAtCeiling puts the run one fault below
// backfillMaxConsecutiveFailures (10), so the next fault is the one that both
// climbs the ladder and ends the run — the interleaving that double-credited.
func seedBackfillFailuresAtCeiling(t *testing.T, e *integration.SearchEnv, id ids.UUID) {
	t.Helper()
	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, execErr := tx.Exec(e.Admin(), `
			UPDATE capture_backfill SET consecutive_failures = 9 WHERE id = $1`, id)
		return execErr
	})
	if err != nil {
		t.Fatalf("seed the failure ladder: %v", err)
	}
}

// TestTheReachIsACountRatherThanAnAccumulation holds what the ledger buys.
//
// The reported reach used to be `SET col = col + 1` per creation, in a
// transaction of its own, after the row it counted had already committed
// elsewhere. Nothing replays a lost one — capture is idempotent, so no retry
// re-offers the message to the resolver — so the number was a FLOOR, and it
// divides into the cost estimator's ratios where a floor understates cost.
//
// Two properties make it a count, and they are the two asserted here: what the
// run created is on the ledger row by row, and the reported number is that
// ledger's size rather than a running total that can drift from it.
func TestTheReachIsACountRatherThanAnAccumulation(t *testing.T) {
	e := integration.SetupSearch(t)
	seedCaptureRole(t, e)
	registry := compose.NewCaptureRegistry(e.Pool, newTestKeyvault(t, e), compose.CaptureConfig{})
	registry.Register(&mailPageConnector{
		raws: [][]byte{
			email(captureOwner, "", "alice@globex.example", "l1@myco.example", ""),
			email("alice@globex.example", "Alice", captureOwner, "l1r@globex.example", "l1@myco.example"),
			email(captureOwner, "", "dave@globex.example", "l2@myco.example", ""),
			email("dave@globex.example", "Dave", captureOwner, "l2r@globex.example", "l2@myco.example"),
		},
		sent: map[string]bool{"l1@myco.example": true, "l2@myco.example": true},
	})

	grantCtx := humanWithScopes(e, e.Rep1, []principal.Scope{principal.ScopeRead})
	if _, err := registry.Connect(grantCtx, "gmail", connector.Auth("refresh")); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	run, err := registry.StartBackfill(grantCtx, "gmail", ids.From[ids.UserKind](e.Rep1), 6, 3, enqueueNothing)
	if err != nil {
		t.Fatalf("StartBackfill: %v", err)
	}
	wsCtx := principal.WithWorkspaceID(context.Background(), e.WS)
	if done, _, _, stepErr := registry.RunBackfillStep(wsCtx, run.ID); stepErr != nil || !done {
		t.Fatalf("the page must finish the run: done=%v err=%v", done, stepErr)
	}

	people, organizations := readBackfillYieldColumns(t, e, run.ID)
	ledgeredPeople, ledgeredOrgs := readCreationLedger(t, e, run.ID)
	if ledgeredPeople == 0 {
		t.Fatal("the run ledgered no creation at all, so this test is measuring an import that made nothing")
	}
	if people != ledgeredPeople || organizations != ledgeredOrgs {
		t.Errorf("the run reports %d people and %d company questions; the ledger holds %d and %d — the "+
			"reported numbers are a projection of the ledger, so the two cannot disagree",
			people, organizations, ledgeredPeople, ledgeredOrgs)
	}

	// The write is idempotent, which is what lets it be retried. Replaying the
	// whole ledger must leave both the rows and the reported numbers where they
	// are: a retry that double-counted would be the old accumulation with more
	// steps.
	replayCreationLedger(t, e, run.ID)
	afterPeople, afterOrgs := readBackfillYieldColumns(t, e, run.ID)
	nowPeople, nowOrgs := readCreationLedger(t, e, run.ID)
	if afterPeople != people || afterOrgs != organizations || nowPeople != ledgeredPeople || nowOrgs != ledgeredOrgs {
		t.Errorf("replaying the ledger moved the reach from %d/%d to %d/%d — writing the same creation "+
			"twice must write it once, or the retry is not safe to take",
			people, organizations, afterPeople, afterOrgs)
	}
}

// TestALostCountIsRecoveredByTheNextCreation is the ticket's own defect,
// inverted.
//
// A count used to be lost permanently: the row was created in one transaction
// and counted in another, nothing replayed the second, and capture's
// idempotency means no later attempt ever re-offers that message. So a fault
// between the two lowered the run's reported reach for good.
//
// With the columns a PROJECTION of the ledger, the next creation recomputes
// them from every row the ledger holds — so a creation that reached the ledger
// and lost its column update heals itself. Under an accumulation it cannot: an
// increment missed is an increment gone.
func TestALostCountIsRecoveredByTheNextCreation(t *testing.T) {
	e := integration.SetupSearch(t)
	seedCaptureRole(t, e)
	registry := compose.NewCaptureRegistry(e.Pool, newTestKeyvault(t, e), compose.CaptureConfig{})
	conn := &mailPageConnector{
		raws: [][]byte{
			email(captureOwner, "", "erin@globex.example", "h1@myco.example", ""),
			email("erin@globex.example", "Erin", captureOwner, "h1r@globex.example", "h1@myco.example"),
			email(captureOwner, "", "frank@globex.example", "h2@myco.example", ""),
			email("frank@globex.example", "Frank", captureOwner, "h2r@globex.example", "h2@myco.example"),
		},
		sent: map[string]bool{"h1@myco.example": true, "h2@myco.example": true},
	}
	// After the first message's counterparty is created and counted, take its
	// count away without touching the ledger — the state a failed column update
	// leaves behind. The second message is then the "next creation".
	var run capture.BackfillRun
	captured := 0
	conn.afterMessage = func() {
		captured++
		if captured == 1 {
			loseTheCount(t, e, run.ID)
		}
	}
	registry.Register(conn)

	grantCtx := humanWithScopes(e, e.Rep1, []principal.Scope{principal.ScopeRead})
	if _, err := registry.Connect(grantCtx, "gmail", connector.Auth("refresh")); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	started, err := registry.StartBackfill(grantCtx, "gmail", ids.From[ids.UserKind](e.Rep1), 6, 10, enqueueNothing)
	if err != nil {
		t.Fatalf("StartBackfill: %v", err)
	}
	run = started
	wsCtx := principal.WithWorkspaceID(context.Background(), e.WS)
	if _, _, _, stepErr := registry.RunBackfillStep(wsCtx, run.ID); stepErr != nil {
		t.Fatalf("RunBackfillStep: %v", stepErr)
	}

	people, _ := readBackfillYieldColumns(t, e, run.ID)
	ledgered, _ := readCreationLedger(t, e, run.ID)
	if ledgered != 2 {
		t.Fatalf("the fixture created %d people, so there was no lost count to recover", ledgered)
	}
	if people != ledgered {
		t.Errorf("after the next creation the run reports %d people and its ledger holds %d — the reach "+
			"is recomputed from the ledger, so an update lost between the two heals rather than "+
			"lowering the run's reported reach for good", people, ledgered)
	}
}

// loseTheCount lowers the run's reported people by one and leaves the ledger
// alone, which is what a creation whose column update failed leaves behind.
func loseTheCount(t *testing.T, e *integration.SearchEnv, id ids.UUID) {
	t.Helper()
	if _, err := e.Pool.Exec(context.Background(),
		`UPDATE capture_backfill SET people_created = greatest(people_created - 1, 0) WHERE id = $1`,
		id); err != nil {
		t.Fatalf("losing a count: %v", err)
	}
}

// readCreationLedger returns how many people and how many queued organizations
// the run's ledger holds — the rows the reported reach is a projection of.
func readCreationLedger(t *testing.T, e *integration.SearchEnv, id ids.UUID) (int, int) {
	t.Helper()
	var people, organizations int
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(e.Admin(), `
			SELECT count(*) FILTER (WHERE kind = 'person'),
			       count(*) FILTER (WHERE kind = 'organization_queued')
			  FROM capture_backfill_creation
			 WHERE backfill_id = $1`, id).Scan(&people, &organizations)
	}); err != nil {
		t.Fatalf("reading the creation ledger: %v", err)
	}
	return people, organizations
}

// replayCreationLedger writes every creation the ledger already holds a second
// time AND recomputes the run's columns, which is both halves of what a retried
// page does. Replaying only the rows would assert nothing: the insert is a
// no-op by the primary key, and a projection that double-counted would never
// be asked to.
func replayCreationLedger(t *testing.T, e *integration.SearchEnv, id ids.UUID) {
	t.Helper()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(e.Admin(), `
			INSERT INTO capture_backfill_creation (backfill_id, kind, subject)
			SELECT backfill_id, kind, subject FROM capture_backfill_creation WHERE backfill_id = $1
			ON CONFLICT (backfill_id, kind, subject) DO NOTHING`, id); err != nil {
			return err
		}
		_, err := tx.Exec(e.Admin(), `
			UPDATE capture_backfill b
			SET people_created = greatest(counted.people, b.people_created),
			    organizations_created = greatest(counted.organizations, b.organizations_created)
			FROM (
				SELECT count(*) FILTER (WHERE kind = 'person') AS people,
				       count(*) FILTER (WHERE kind = 'organization_queued') AS organizations
				  FROM capture_backfill_creation
				 WHERE backfill_id = $1
			) counted
			WHERE b.id = $1`, id)
		return err
	}); err != nil {
		t.Fatalf("replaying the creation ledger: %v", err)
	}
}

// TestConcurrentCreationsAreAllCounted walks one page's messages at once.
//
// The reported reach is a projection of the ledger, recomputed per creation.
// Two recomputes running at once at READ COMMITTED each count a snapshot
// without the other's uncommitted row, so the later committer would write a
// total that is missing the earlier one — a lost update the accumulation this
// replaced could not have had, and one no later creation repairs once the run
// has no creations left to make. What stops it is the run row's lock, taken
// before the ledger rows are written.
func TestConcurrentCreationsAreAllCounted(t *testing.T) {
	e := integration.SetupSearch(t)
	seedCaptureRole(t, e)
	registry := compose.NewCaptureRegistry(e.Pool, newTestKeyvault(t, e), compose.CaptureConfig{})
	const counterparties = 8
	raws := make([][]byte, 0, counterparties)
	// Each racer's send and its reply, in that order. Only a counterparty the
	// ladder ADMITS reaches the ledger, and one send is intent rather than an
	// exchange — so the reply is what makes each of the eight creatable, and
	// the race is eight person creations recomputing the same run's reach at
	// once.
	//
	// A reply read before its send has committed is also real — the evidence
	// for an exchange is two rows, and a reader seeing one defers the sender to
	// the verdict rather than creating. But that is the ladder working, not
	// this test's subject, and racing it alongside the creations meant a loaded
	// runner could defer every counterparty and leave no creations to count.
	// So the sends are the connector's prelude and only the replies race.
	sends := make([][]byte, 0, counterparties)
	sent := make(map[string]bool, counterparties)
	for i := range counterparties {
		msgID := fmt.Sprintf("race%d@myco.example", i)
		addr := fmt.Sprintf("racer%d@globex.example", i)
		sent[msgID] = true
		sends = append(sends, email(captureOwner, "", addr, msgID, ""))
		raws = append(raws, email(addr, "Racer", captureOwner, fmt.Sprintf("race%dr@globex.example", i), msgID))
	}
	// The eight sends first and in order, the eight replies at once. Both
	// orderings are real, and separating them is what makes this test about
	// ONE of them: with the sends committed, every reply completes an exchange
	// the ladder admits, so all eight counterparties are creatable and the
	// race is eight creations recomputing the same run's reach — which is the
	// lost update the run row's lock exists to prevent.
	registry.Register(&mailPageConnector{prelude: sends, raws: raws, sent: sent, parallel: true})

	grantCtx := humanWithScopes(e, e.Rep1, []principal.Scope{principal.ScopeRead})
	if _, err := registry.Connect(grantCtx, "gmail", connector.Auth("refresh")); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	run, err := registry.StartBackfill(grantCtx, "gmail", ids.From[ids.UserKind](e.Rep1),
		6, counterparties, enqueueNothing)
	if err != nil {
		t.Fatalf("StartBackfill: %v", err)
	}
	wsCtx := principal.WithWorkspaceID(context.Background(), e.WS)
	if _, _, _, stepErr := registry.RunBackfillStep(wsCtx, run.ID); stepErr != nil {
		t.Fatalf("RunBackfillStep: %v", stepErr)
	}

	people, _ := readBackfillYieldColumns(t, e, run.ID)
	ledgered, _ := readCreationLedger(t, e, run.ID)
	// Every one of them, now that each reply meets a committed send. The floor
	// used to be two — a hedge against the send/reply race above, which cost the
	// test its own subject: two concurrent creations out of eight is a weaker
	// claim than the lock owes, and zero, which a loaded runner really did
	// produce, is no claim at all.
	if ledgered != counterparties {
		t.Fatalf("the page ledgered %d creations, want all %d — each reply meets a committed send, "+
			"so every counterparty is creatable and the race is between the creations alone",
			ledgered, counterparties)
	}
	if people != ledgered {
		t.Errorf("the run reports %d people and its ledger holds %d — every creation counts, "+
			"whichever goroutine made it", people, ledgered)
	}
}

// TestACountTheLedgerCannotSeeIsKept is the migration window.
//
// The migration seeds the ledger from a SNAPSHOT of the counted columns. A run
// still walked by the old binary keeps incrementing the column after that
// snapshot with no ledger row to show for it, so a recompute that took the
// ledger's count alone would discard exactly those increments — the reach too
// low again, in the one window where nothing repairs it. The projection takes
// greatest(ledger, column), and this is the direction that guard exists for.
func TestACountTheLedgerCannotSeeIsKept(t *testing.T) {
	e := integration.SetupSearch(t)
	seedCaptureRole(t, e)
	registry := compose.NewCaptureRegistry(e.Pool, newTestKeyvault(t, e), compose.CaptureConfig{})
	conn := &mailPageConnector{
		raws: [][]byte{
			email(captureOwner, "", "gina@globex.example", "w1@myco.example", ""),
			email("gina@globex.example", "Gina", captureOwner, "w1r@globex.example", "w1@myco.example"),
			email(captureOwner, "", "hank@globex.example", "w2@myco.example", ""),
			email("hank@globex.example", "Hank", captureOwner, "w2r@globex.example", "w2@myco.example"),
		},
		sent: map[string]bool{"w1@myco.example": true, "w2@myco.example": true},
	}
	// Three creations the old binary counted and the seed did not see, added
	// after the first EXCHANGE so the second one is the recompute that would
	// have thrown them away.
	//
	// After the second message, not the first: an exchange is a send and its
	// reply, and the send alone creates nobody — so a hook on message one would
	// fire before any creation had happened and there would be nothing for the
	// recompute to threaten.
	const unledgered = 3
	var run capture.BackfillRun
	captured := 0
	conn.afterMessage = func() {
		captured++
		if captured == 2 {
			keepUnledgeredCount(t, e, run.ID, unledgered)
		}
	}
	registry.Register(conn)

	grantCtx := humanWithScopes(e, e.Rep1, []principal.Scope{principal.ScopeRead})
	if _, err := registry.Connect(grantCtx, "gmail", connector.Auth("refresh")); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	started, err := registry.StartBackfill(grantCtx, "gmail", ids.From[ids.UserKind](e.Rep1), 6, 10, enqueueNothing)
	if err != nil {
		t.Fatalf("StartBackfill: %v", err)
	}
	run = started
	wsCtx := principal.WithWorkspaceID(context.Background(), e.WS)
	if _, _, _, stepErr := registry.RunBackfillStep(wsCtx, run.ID); stepErr != nil {
		t.Fatalf("RunBackfillStep: %v", stepErr)
	}

	people, _ := readBackfillYieldColumns(t, e, run.ID)
	ledgered, _ := readCreationLedger(t, e, run.ID)
	if ledgered != 2 {
		t.Fatalf("the fixture ledgered %d creations, so the column never sat above the ledger", ledgered)
	}
	// The guard is a FLOOR, not an addition: the column stood at 1+3 when the
	// second creation recomputed, the ledger said 2, and what must survive is
	// the 4. Adding the two would double-count the creation the column and the
	// ledger both already hold.
	if want := 1 + unledgered; people != want {
		t.Errorf("the run reports %d people and the ledger holds %d, but %d had already been "+
			"reported when the recompute ran — a recompute keeps the higher of the two, or the "+
			"migration window loses every count the seed's snapshot missed", people, ledgered, want)
	}
}

// keepUnledgeredCount raises the run's reported people without writing ledger
// rows, which is what an old binary's increment after the seed leaves.
func keepUnledgeredCount(t *testing.T, e *integration.SearchEnv, id ids.UUID, by int) {
	t.Helper()
	if _, err := e.Pool.Exec(context.Background(),
		`UPDATE capture_backfill SET people_created = people_created + $2 WHERE id = $1`,
		id, by); err != nil {
		t.Fatalf("keeping an unledgered count: %v", err)
	}
}
