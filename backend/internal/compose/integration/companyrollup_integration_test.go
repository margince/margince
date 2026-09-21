// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The company hierarchy roll-up (GET /companies/{id}/hierarchy-rollup,
// RD-T04): roll-up(node) = self(node) + Σ roll-up(readable child) over
// the parent_company_id tree. A child the caller cannot read contributes
// nothing and is disclosed by {id, display_name} only; all money
// converts to the workspace base currency, and a missing stored FX rate
// fails the whole read rather than inventing a rate.

import (
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// rollupStages is one dedicated pipeline with a known 40% open stage and
// a won stage, so every weighted expectation below is arithmetic the
// test controls rather than a value inherited from the workspace seed.
type rollupStages struct {
	pipeline, open, won ids.UUID
}

const rollupOpenWinProbability = 40

func seedRollupStages(t *testing.T, e *Env) rollupStages {
	t.Helper()
	st := rollupStages{pipeline: ids.NewV7(), open: ids.NewV7(), won: ids.NewV7()}
	e.WsExec(t, `INSERT INTO pipeline (id, name) VALUES ($1, 'Rollup Pipeline')`,
		st.pipeline)
	e.WsExec(t, `INSERT INTO stage (id, pipeline_id, name, position, semantic, win_probability)
		VALUES ($1, $2, 'Qualified', 1, 'open', $3)`,
		st.open, st.pipeline, rollupOpenWinProbability)
	e.WsExec(t, `INSERT INTO stage (id, pipeline_id, name, position, semantic, win_probability)
		VALUES ($1, $2, 'Won', 2, 'won', 100)`,
		st.won, st.pipeline)
	return st
}

// seedRollupCompany inserts one hierarchy node directly: the rollup is a
// read, so the audit/outbox write shape the contacts store would add is
// noise here, and parent_company_id wiring has no store-level entry point.
func seedRollupCompany(t *testing.T, e *Env, name string, owner, parent *ids.UUID) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	e.WsExec(t, `INSERT INTO company (id, display_name, owner_id, parent_company_id, source, captured_by)
		VALUES ($1, $2, $3, $4, 'manual', 'human:test')`,
		id, name, owner, parent)
	return id
}

// seedRollupOpenDeal attaches one open deal; a nil amount/currency pair
// seeds the honest half-empty deal the weighted sum must count as 0.
func seedRollupOpenDeal(t *testing.T, e *Env, st rollupStages, company ids.UUID, amountMinor *int64, currency *string) {
	t.Helper()
	e.WsExec(t, `INSERT INTO deal (id, name, amount_minor, currency, pipeline_id, stage_id, company_id, status, source, captured_by)
		VALUES ($1, 'Open Deal', $2, $3, $4, $5, $6, 'open', 'manual', 'human:test')`,
		ids.NewV7(), amountMinor, currency, st.pipeline, st.open, company)
}

// seedRollupWonDeal closes a deal with the frozen FX rate the
// deal_closed_fx CHECK demands — the rate the quarter sum must reuse.
//
// amount_minor_base is set alongside it rather than left to a trigger: since
// 1788583500 the column is filled by the real close path (deals.freezeBaseRate),
// never generated — a generated expression cannot reach the base currency's own
// minor-unit scale. Computed here with deals.ConvertToBase, the same exported
// arithmetic that writer calls, rather than a second copy of the formula.
func seedRollupWonDeal(t *testing.T, e *Env, st rollupStages, company ids.UUID,
	amountMinor int64, currency, fxRateToBase string, closedAt time.Time,
) {
	t.Helper()
	var rate pgtype.Numeric
	if err := rate.Scan(fxRateToBase); err != nil {
		t.Fatalf("rate %q is not a number: %v", fxRateToBase, err)
	}
	amountMinorBase, err := deals.ConvertToBase(amountMinor, rate, currency, "EUR")
	if err != nil {
		t.Fatalf("converting %d %s to base: %v", amountMinor, currency, err)
	}
	e.WsExec(t, `INSERT INTO deal (id, name, amount_minor, currency, fx_rate_to_base, amount_minor_base, pipeline_id, stage_id, company_id, status, closed_at, source, captured_by)
		VALUES ($1, 'Won Deal', $2, $3, $4, $5, $6, $7, $8, 'won', $9, 'manual', 'human:test')`,
		ids.NewV7(), amountMinor, currency, fxRateToBase, amountMinorBase, st.pipeline, st.won, company, closedAt)
}

func seedRollupFxRate(t *testing.T, e *Env, fromCurrency, rate string, day time.Time) {
	t.Helper()
	e.WsExec(t, `INSERT INTO fx_rate (from_currency, to_currency, rate, rate_date)
		VALUES ($1, 'EUR', $2, $3)`,
		fromCurrency, rate, day)
}

func seedRollupCompanyActivity(t *testing.T, e *Env, company ids.UUID, occurredAt time.Time) {
	t.Helper()
	activityID := ids.NewV7()
	e.WsExec(t, `INSERT INTO activity (id, kind, subject, occurred_at, source, captured_by)
		VALUES ($1, 'note', 'rollup touch', $2, 'manual', 'human:test')`,
		activityID, occurredAt)
	e.WsExec(t, `INSERT INTO activity_link (activity_id, entity_type, company_id)
		VALUES ($1, 'company', $2)`, activityID, company)
}

// seedRollupDealLinkedActivity files one activity against a DEAL of the
// given company and never against the company itself — the
// second of the three arms an account's timeline walks.
func seedRollupDealLinkedActivity(t *testing.T, e *Env, st rollupStages, company ids.UUID, occurredAt time.Time) {
	t.Helper()
	dealID := ids.NewV7()
	e.WsExec(t, `INSERT INTO deal (id, name, pipeline_id, stage_id, company_id, status, source, captured_by)
		VALUES ($1, 'Deal With Mail', $2, $3, $4, 'open', 'manual', 'human:test')`,
		dealID, st.pipeline, st.open, company)
	activityID := ids.NewV7()
	e.WsExec(t, `INSERT INTO activity (id, kind, subject, occurred_at, source, captured_by)
		VALUES ($1, 'email', 'deal thread', $2, 'manual', 'human:test')`,
		activityID, occurredAt)
	e.WsExec(t, `INSERT INTO activity_link (activity_id, entity_type, deal_id)
		VALUES ($1, 'deal', $2)`, activityID, dealID)
}

// seedRollupContactLinkedActivity files one activity against a CONTACT
// currently employed by the given company and never against the
// company itself — the third arm, and the one that carries most of a
// real account's mail, because capture files a message against the contact
// it was with.
func seedRollupContactLinkedActivity(t *testing.T, e *Env, company ids.UUID, occurredAt time.Time) {
	t.Helper()
	contactID := ids.NewV7()
	e.WsExec(t, `INSERT INTO contact (id, full_name, source, captured_by)
		VALUES ($1, 'Rollup Contact', 'manual', 'human:test')`, contactID)
	e.WsExec(t, `INSERT INTO relationship (id, kind, contact_id, company_id, source, captured_by)
		VALUES ($1, 'employment', $2, $3, 'manual', 'human:test')`,
		ids.NewV7(), contactID, company)
	activityID := ids.NewV7()
	e.WsExec(t, `INSERT INTO activity (id, kind, subject, occurred_at, source, captured_by)
		VALUES ($1, 'email', 'contact thread', $2, 'connector:gmail', 'connector:gmail')`,
		activityID, occurredAt)
	e.WsExec(t, `INSERT INTO activity_link (activity_id, entity_type, contact_id)
		VALUES ($1, 'contact', $2)`, activityID, contactID)
}

// rollupCompanyReadPerms is the minimal caller the rollup admits: read on
// company, deal, AND activity at the given row-scope tier — the
// rollup surfaces deal money and activity counts, so it demands the same
// object grants the forecast and activity reports do.
func rollupCompanyReadPerms(scope principal.RowScope) principal.Permissions {
	return principal.Permissions{
		RoleKeys: []string{"rep"},
		Objects: map[string]principal.ObjectGrant{
			"company":  {Read: true},
			"deal":     {Read: true},
			"activity": {Read: true},
			// The contact count is a count over employment PAIRS, so it needs
			// the edge grant alongside contact:read. Every seeded role holds it;
			// the cases built on this fixture add or withhold `contact` and
			// `computed_field` deliberately, and the edge grant must not be the
			// accidental reason a count is absent.
			"relationship":          {Read: true},
			"installation_settings": {Read: true},
		},
		RowScope: scope,
	}
}

func int64Ptr(v int64) *int64 { return &v }

// fixedClock pins CompanyHierarchyRollup's injected clock to instant, so a
// test that seeds rows relative to a captured "now" reads at that exact
// same instant rather than racing a fresh time.Now() call inside the
// read — the gap between the two is normally sub-millisecond, but a
// quarter or calendar-day window boundary crossed in that gap would
// otherwise flake the assertion.
func fixedClock(instant time.Time) func() time.Time {
	return func() time.Time { return instant }
}

// TestCompanyRollupReconcilesTreeToSelves is the reconciliation invariant:
// the tree total equals the sum of every included node's scope=self
// figures, an empty node contributes a real 0 (and still counts in
// aggregated_account_count), a NULL-amount deal contributes 0, and a
// non-base-currency open deal converts at the stored as-of rate.
func TestCompanyRollupReconcilesTreeToSelves(t *testing.T) {
	e := Setup(t)
	st := seedRollupStages(t, e)
	root := seedRollupCompany(t, e, "Root Co", nil, nil)
	childA := seedRollupCompany(t, e, "Child A", nil, &root)
	childB := seedRollupCompany(t, e, "Child B (empty)", nil, &root)
	grandchild := seedRollupCompany(t, e, "Grandchild", nil, &childA)
	now := time.Now().UTC()

	seedRollupOpenDeal(t, e, st, root, int64Ptr(100_000), strPtr("EUR"))
	seedRollupOpenDeal(t, e, st, root, nil, nil) // NULL amount: a real 0, never an error
	seedRollupOpenDeal(t, e, st, childA, int64Ptr(50_000), strPtr("EUR"))
	seedRollupOpenDeal(t, e, st, childA, int64Ptr(10_000), strPtr("USD")) // 0.5 → 5_000 base
	seedRollupOpenDeal(t, e, st, grandchild, int64Ptr(20_000), strPtr("EUR"))
	seedRollupFxRate(t, e, "USD", "0.5", now.AddDate(0, 0, -2))
	seedRollupWonDeal(t, e, st, root, 30_000, "EUR", "1.0", now)
	seedRollupCompanyActivity(t, e, root, now.Add(-24*time.Hour))
	seedRollupCompanyActivity(t, e, grandchild, now.Add(-24*time.Hour))
	seedRollupCompanyActivity(t, e, childA, now.Add(-40*24*time.Hour)) // outside the 30d window
	seedRollupCompanyActivity(t, e, root, now.Add(24*time.Hour))       // future-dated: never counts

	tree, err := compose.CompanyHierarchyRollup(e.Admin(), e.Pool, root, "tree", fixedClock(now))
	if err != nil {
		t.Fatalf("tree rollup: %v", err)
	}
	// Per-deal weighted at 40%: 40_000 (root) + 20_000 + 2_000 (childA) + 8_000 (grandchild).
	if tree.WeightedPipelineMinor != 70_000 {
		t.Errorf("tree weighted = %d, want 70000", tree.WeightedPipelineMinor)
	}
	if tree.ClosedWonMinor != 30_000 {
		t.Errorf("tree closed-won = %d, want 30000", tree.ClosedWonMinor)
	}
	if tree.ActivityCount30d != 2 {
		t.Errorf("tree activity count = %d, want 2 (the 40-day-old touch and the future-dated one are both out of window)", tree.ActivityCount30d)
	}
	if tree.AggregatedAccountCount != 4 {
		t.Errorf("aggregated account count = %d, want 4 (the empty sibling still counts)", tree.AggregatedAccountCount)
	}
	if tree.BaseCurrency != "EUR" {
		t.Errorf("base currency = %q, want EUR", tree.BaseCurrency)
	}
	if tree.RestrictedExcluded == nil || len(tree.RestrictedExcluded) != 0 {
		t.Errorf("restricted = %v, want non-nil empty", tree.RestrictedExcluded)
	}
	if tree.RootID != root || tree.Scope != "tree" || tree.ComputedAt.IsZero() {
		t.Errorf("result envelope = {%v %q %v}, want root id, tree scope, real computed_at",
			tree.RootID, tree.Scope, tree.ComputedAt)
	}

	var sumWeighted, sumWon int64
	var sumActivity, sumNodes int
	for _, node := range []ids.UUID{root, childA, childB, grandchild} {
		self, err := compose.CompanyHierarchyRollup(e.Admin(), e.Pool, node, "self", fixedClock(now))
		if err != nil {
			t.Fatalf("self rollup of %v: %v", node, err)
		}
		if self.AggregatedAccountCount != 1 {
			t.Errorf("self count of %v = %d, want 1", node, self.AggregatedAccountCount)
		}
		if node == childB && (self.WeightedPipelineMinor != 0 || self.ClosedWonMinor != 0 || self.ActivityCount30d != 0) {
			t.Errorf("empty sibling self = %+v, want real zeros", self)
		}
		sumWeighted += self.WeightedPipelineMinor
		sumWon += self.ClosedWonMinor
		sumActivity += self.ActivityCount30d
		sumNodes += self.AggregatedAccountCount
	}
	if sumWeighted != tree.WeightedPipelineMinor || sumWon != tree.ClosedWonMinor ||
		sumActivity != tree.ActivityCount30d || sumNodes != tree.AggregatedAccountCount {
		t.Errorf("Σ(self) = {%d %d %d %d}, want the tree totals {%d %d %d %d}",
			sumWeighted, sumWon, sumActivity, sumNodes,
			tree.WeightedPipelineMinor, tree.ClosedWonMinor, tree.ActivityCount30d, tree.AggregatedAccountCount)
	}
}

// TestCompanyRollupRestrictedNodeDisclosedAndGrantRestores: an unreadable
// child is excluded from every total and disclosed by identity only, its
// subtree is never visited (the ownerless — hence readable — grandchild
// is neither summed nor separately disclosed), and a live record_grant
// flips the child and its readable subtree back in on the next call.
func TestCompanyRollupRestrictedNodeDisclosedAndGrantRestores(t *testing.T) {
	e := Setup(t)
	st := seedRollupStages(t, e)
	root := seedRollupCompany(t, e, "Root Co", &e.Rep1, nil)
	child := seedRollupCompany(t, e, "Restricted Child", &e.Rep3, &root)
	// Ownership alone no longer hides an account from a colleague; capture
	// privacy does, and a record_grant still opens it.
	e.MakeCapturePrivate(t, "company", child, e.Rep3)
	grandchild := seedRollupCompany(t, e, "Ownerless Grandchild", nil, &child)
	for _, company := range []ids.UUID{root, child, grandchild} {
		seedRollupOpenDeal(t, e, st, company, int64Ptr(10_000), strPtr("EUR"))
	}

	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, rollupCompanyReadPerms(principal.RowScopeOwn))
	pre, err := compose.CompanyHierarchyRollup(rep, e.Pool, root, "tree", time.Now)
	if err != nil {
		t.Fatalf("pre-grant rollup: %v", err)
	}
	if pre.WeightedPipelineMinor != 4_000 {
		t.Errorf("pre-grant weighted = %d, want 4000 (root only)", pre.WeightedPipelineMinor)
	}
	if pre.AggregatedAccountCount != 1 {
		t.Errorf("pre-grant account count = %d, want 1", pre.AggregatedAccountCount)
	}
	if len(pre.RestrictedExcluded) != 1 ||
		pre.RestrictedExcluded[0].ID != child || pre.RestrictedExcluded[0].DisplayName != "Restricted Child" {
		t.Fatalf("restricted = %+v, want exactly the child disclosed by id+name (grandchild never visited)",
			pre.RestrictedExcluded)
	}

	e.WsExec(t, `INSERT INTO record_grant (record_type, record_id, subject_type, subject_id, access, granted_by)
		VALUES ('company', $1, 'user', $2, 'read', $2)`, child, e.Rep1)

	post, err := compose.CompanyHierarchyRollup(rep, e.Pool, root, "tree", time.Now)
	if err != nil {
		t.Fatalf("post-grant rollup: %v", err)
	}
	if post.WeightedPipelineMinor != 12_000 {
		t.Errorf("post-grant weighted = %d, want 12000 (child + readable grandchild restored)", post.WeightedPipelineMinor)
	}
	if post.AggregatedAccountCount != 3 {
		t.Errorf("post-grant account count = %d, want 3", post.AggregatedAccountCount)
	}
	if len(post.RestrictedExcluded) != 0 {
		t.Errorf("post-grant restricted = %+v, want empty", post.RestrictedExcluded)
	}
}

// TestCompanyRollupWeightedPipelineSurvivesStageArchival: an open deal whose
// stage is archived still contributes its weighted value — archiving a
// stage reshapes the pipeline's vocabulary, it never silently zeroes the
// money already sitting in that stage (matching the forecast report's
// stage join, which carries no archived_at filter either).
func TestCompanyRollupWeightedPipelineSurvivesStageArchival(t *testing.T) {
	e := Setup(t)
	st := seedRollupStages(t, e)
	root := seedRollupCompany(t, e, "Root Co", nil, nil)
	seedRollupOpenDeal(t, e, st, root, int64Ptr(10_000), strPtr("EUR"))
	e.WsExec(t, `UPDATE stage SET archived_at = now() WHERE id = $1`, st.open)

	res, err := compose.CompanyHierarchyRollup(e.Admin(), e.Pool, root, "tree", time.Now)
	if err != nil {
		t.Fatalf("rollup: %v", err)
	}
	if res.WeightedPipelineMinor != 4_000 {
		t.Errorf("weighted = %d, want 4000 (the archived stage's live deal still counts)", res.WeightedPipelineMinor)
	}
}

// TestCompanyRollupFXRateUnavailableFailsWholeRead: an open deal in a
// currency with no stored rate to base fails the WHOLE read with the
// typed error — never a partial sum, never a silent rate of 1.
func TestCompanyRollupFXRateUnavailableFailsWholeRead(t *testing.T) {
	e := Setup(t)
	st := seedRollupStages(t, e)
	root := seedRollupCompany(t, e, "Root Co", nil, nil)
	seedRollupOpenDeal(t, e, st, root, int64Ptr(100_000), strPtr("EUR"))
	seedRollupOpenDeal(t, e, st, root, int64Ptr(10_000), strPtr("USD")) // no USD→EUR rate seeded

	_, err := compose.CompanyHierarchyRollup(e.Admin(), e.Pool, root, "tree", time.Now)
	var fxErr *compose.FXRateUnavailableError
	if !errors.As(err, &fxErr) {
		t.Fatalf("err = %v, want a typed FX-rate-unavailable failure", err)
	}
	if fxErr.Currency != "USD" || fxErr.AsOf.IsZero() {
		t.Errorf("fx error = %+v, want the missing currency and a real as-of instant", fxErr)
	}
}

// TestCompanyRollupClosedWonQuarterWindow: closed-won counts only won deals
// whose closed_at falls in the current workspace-timezone quarter
// [start, end), converted at each deal's FROZEN rate — no fx_rate row
// exists here, so a live-rate lookup would fail the read instead.
//
// asOf is pinned to a fixed mid-quarter instant (not time.Now()): the
// workspace's reporting timezone defaults to UTC, so an asOf resolved a
// hair after the seeded "in-quarter" deal's closed_at could otherwise
// cross a real quarter boundary right as the suite runs near one, and
// the -100-day deal would then land in a different (but still
// out-of-window) quarter — pinning removes the wall-clock dependency
// entirely rather than merely making it a rare flake.
func TestCompanyRollupClosedWonQuarterWindow(t *testing.T) {
	e := Setup(t)
	st := seedRollupStages(t, e)
	root := seedRollupCompany(t, e, "Root Co", nil, nil)
	asOf := time.Date(2026, time.May, 15, 12, 0, 0, 0, time.UTC)
	seedRollupWonDeal(t, e, st, root, 10_000, "USD", "0.5", asOf)
	// 100 days back is outside any calendar quarter containing asOf (a
	// quarter spans at most 92 days), whatever asOf itself resolves to.
	seedRollupWonDeal(t, e, st, root, 99_999, "USD", "1.0", asOf.AddDate(0, 0, -100))

	res, err := compose.CompanyHierarchyRollup(e.Admin(), e.Pool, root, "tree", fixedClock(asOf))
	if err != nil {
		t.Fatalf("rollup: %v", err)
	}
	if res.ClosedWonMinor != 5_000 {
		t.Errorf("closed-won = %d, want 5000 (in-quarter deal at its frozen 0.5 rate only)", res.ClosedWonMinor)
	}
	if res.WeightedPipelineMinor != 0 {
		t.Errorf("weighted = %d, want 0 (won deals never re-enter the pipeline sum)", res.WeightedPipelineMinor)
	}
}

// TestCompanyRollupRootGates: the threefold gate at the root — a missing
// object-read grant answers 403 before any row is touched; a nonexistent
// root and an out-of-scope root both answer 404, indistinguishable by
// design; an out-of-vocabulary scope is refused, not defaulted.
func TestCompanyRollupRootGates(t *testing.T) {
	e := Setup(t)
	foreign := seedRollupCompany(t, e, "Foreign Company", &e.Rep3, nil)
	e.MakeCapturePrivate(t, "company", foreign, e.Rep3)

	// Nonexistent root, unbounded admin: the tree walk itself must
	// answer not-found (the visibility gate has nothing to probe for an
	// unbounded caller).
	if _, err := compose.CompanyHierarchyRollup(e.Admin(), e.Pool, ids.NewV7(), "tree", time.Now); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("nonexistent root: err = %v, want not found", err)
	}

	// Rep3 captured the root privately, so Rep1 cannot read it — out of
	// scope reads as not-there, never as an empty rollup.
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, rollupCompanyReadPerms(principal.RowScopeTeam))
	if _, err := compose.CompanyHierarchyRollup(rep, e.Pool, foreign, "tree", time.Now); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("out-of-scope root: err = %v, want not found", err)
	}

	// RepPerms grants contact/deal/pipeline but not company: 403.
	noPerm := e.As(e.Rep1, []ids.UUID{e.Team1}, RepPerms)
	if _, err := compose.CompanyHierarchyRollup(noPerm, e.Pool, foreign, "tree", time.Now); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("no company:read: err = %v, want permission denied", err)
	}

	// The rollup surfaces deal money and activity counts, so
	// company:read alone is not enough — a caller missing deal:read
	// (or activity:read) is refused before any row is touched.
	for missing, perms := range map[string]principal.Permissions{
		"deal": {
			RoleKeys: []string{"rep"},
			Objects: map[string]principal.ObjectGrant{
				"company": {Read: true}, "activity": {Read: true},
				"installation_settings": {Read: true},
			},
			RowScope: principal.RowScopeTeam,
		},
		"activity": {
			RoleKeys: []string{"rep"},
			Objects: map[string]principal.ObjectGrant{
				"company": {Read: true}, "deal": {Read: true},
				"installation_settings": {Read: true},
			},
			RowScope: principal.RowScopeTeam,
		},
	} {
		caller := e.As(e.Rep1, []ids.UUID{e.Team1}, perms)
		if _, err := compose.CompanyHierarchyRollup(caller, e.Pool, foreign, "tree", time.Now); !errors.Is(err, apperrors.ErrPermissionDenied) {
			t.Errorf("missing %s:read: err = %v, want permission denied", missing, err)
		}
	}

	// Permission refusal precedes input validation: a caller without
	// company:read gets 403 even for a scope outside the vocabulary
	// — the bogus scope must never be judged before the grant is.
	if _, err := compose.CompanyHierarchyRollup(noPerm, e.Pool, foreign, "subtree", time.Now); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("no company:read + bogus scope: err = %v, want permission denied", err)
	}

	// A scope outside {tree, self} is a refused input, not a default.
	if _, err := compose.CompanyHierarchyRollup(e.Admin(), e.Pool, foreign, "subtree", time.Now); err == nil {
		t.Error("invalid scope accepted — must be refused")
	}
}

// TestCompanyRollupSelfScopeSkipsPruning: scope=self returns the root's own
// figures without consulting child readability at all — unreadable
// children neither block the read nor appear as restricted.
func TestCompanyRollupSelfScopeSkipsPruning(t *testing.T) {
	e := Setup(t)
	st := seedRollupStages(t, e)
	root := seedRollupCompany(t, e, "Root Co", &e.Rep1, nil)
	child := seedRollupCompany(t, e, "Hidden Child", &e.Rep3, &root)
	seedRollupOpenDeal(t, e, st, root, int64Ptr(10_000), strPtr("EUR"))
	seedRollupOpenDeal(t, e, st, child, int64Ptr(50_000), strPtr("EUR"))

	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, rollupCompanyReadPerms(principal.RowScopeOwn))
	res, err := compose.CompanyHierarchyRollup(rep, e.Pool, root, "self", time.Now)
	if err != nil {
		t.Fatalf("self rollup: %v", err)
	}
	if res.WeightedPipelineMinor != 4_000 {
		t.Errorf("self weighted = %d, want 4000 (root's own deal only)", res.WeightedPipelineMinor)
	}
	if res.AggregatedAccountCount != 1 || res.Scope != "self" {
		t.Errorf("self envelope = {count %d, scope %q}, want {1, self}", res.AggregatedAccountCount, res.Scope)
	}
	if len(res.RestrictedExcluded) != 0 {
		t.Errorf("self restricted = %+v, want empty — self scope never prunes", res.RestrictedExcluded)
	}
}

// TestCompanyRollupCounts30dActivityThroughEveryLinkTheTimelineWalks pins the
// count to the same reachability the timeline uses. The number sits above
// a list the reader can scroll: when it counted only activities carrying a
// direct company link, an account whose mail is filed against its
// contacts — which is what capture does — reported a fraction of what the
// page below it displayed, and the busier the account the wider the gap.
func TestCompanyRollupCounts30dActivityThroughEveryLinkTheTimelineWalks(t *testing.T) {
	e := Setup(t)
	st := seedRollupStages(t, e)
	company := seedRollupCompany(t, e, "Reachable Co", nil, nil)
	now := time.Now().UTC()

	seedRollupCompanyActivity(t, e, company, now.Add(-24*time.Hour))
	seedRollupDealLinkedActivity(t, e, st, company, now.Add(-48*time.Hour))
	seedRollupContactLinkedActivity(t, e, company, now.Add(-72*time.Hour))
	// Out of window through the same two indirect arms: reachability
	// widening must not smuggle past the 30-day bound.
	seedRollupDealLinkedActivity(t, e, st, company, now.AddDate(0, 0, -40))
	seedRollupContactLinkedActivity(t, e, company, now.Add(24*time.Hour))

	res, err := compose.CompanyHierarchyRollup(e.Admin(), e.Pool, company, "self", fixedClock(now))
	if err != nil {
		t.Fatalf("self rollup: %v", err)
	}
	if res.ActivityCount30d != 3 {
		t.Errorf("activity count = %d, want 3 (own link + its deal's + its contact's, in window)", res.ActivityCount30d)
	}
}

// TestCompanyRollupCountsAnActivityReachingTheTreeTwiceOnlyOnce guards the
// EXISTS: one message linked to both the account and its deal is one
// message, and a join would have counted it per link.
func TestCompanyRollupCountsAnActivityReachingTheTreeTwiceOnlyOnce(t *testing.T) {
	e := Setup(t)
	st := seedRollupStages(t, e)
	company := seedRollupCompany(t, e, "Double Linked Co", nil, nil)
	now := time.Now().UTC()

	dealID := ids.NewV7()
	e.WsExec(t, `INSERT INTO deal (id, name, pipeline_id, stage_id, company_id, status, source, captured_by)
		VALUES ($1, 'Both Links', $2, $3, $4, 'open', 'manual', 'human:test')`,
		dealID, st.pipeline, st.open, company)
	activityID := ids.NewV7()
	e.WsExec(t, `INSERT INTO activity (id, kind, subject, occurred_at, source, captured_by)
		VALUES ($1, 'email', 'filed twice', $2, 'manual', 'human:test')`,
		activityID, now.Add(-24*time.Hour))
	e.WsExec(t, `INSERT INTO activity_link (activity_id, entity_type, company_id)
		VALUES ($1, 'company', $2)`, activityID, company)
	e.WsExec(t, `INSERT INTO activity_link (activity_id, entity_type, deal_id)
		VALUES ($1, 'deal', $2)`, activityID, dealID)

	res, err := compose.CompanyHierarchyRollup(e.Admin(), e.Pool, company, "self", fixedClock(now))
	if err != nil {
		t.Fatalf("self rollup: %v", err)
	}
	if res.ActivityCount30d != 1 {
		t.Errorf("activity count = %d, want 1 (two links, one message)", res.ActivityCount30d)
	}
}
