// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A frozen snapshot meeting a narrower reader.
//
// The stored headline covers every deal the issuer could see. Serving that
// number to somebody who cannot see all of them discloses a total about deals
// they may not read, so a shared snapshot re-sums the rows the RECIPIENT may
// read and returns a headline about them. These tests are the proof that the
// recompute happens: each seeds two deals on two teams, freezes both, and asks
// what each reader gets back.

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/forecasting"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// freezeTwoTeams seeds one priced deal per team and freezes the pair, through
// the REAL writer and the real deal read, so what the recompute later re-sums
// is what production would have written.
func (e *forecastEnv) freezeTwoTeams(t *testing.T, mine, theirs int64) ids.UUID {
	t.Helper()
	// Inside the period on purpose. seedOpenDeal dates a deal 30 days out,
	// which near a quarter boundary lands in the NEXT one — the deal is then
	// eligible for no reading here, every headline is zero, and the test would
	// pass or fail by the calendar rather than by the recompute.
	e.seedDealClosingWithin(t, "Mine", &e.Rep1, mine)
	e.seedDealClosingWithin(t, "Theirs", &e.Rep3, theirs)

	store := forecasting.NewStore(InstallationDB(e.Pool))
	admin := snapshotWriterCtx(e.WS)
	var id ids.UUID
	if err := store.InTx(admin, func(ctx context.Context, tx pgx.Tx) error {
		period, baseCurrency, err := ForecastPeriodAt(ctx, tx, forecasting.PeriodQuarter, time.Now())
		if err != nil {
			return err
		}
		deals, _, _, err := ForecastDeals(ctx, tx, period,
			forecasting.Scope{Kind: forecasting.ScopeWorkspace}, time.Now(), baseCurrency)
		if err != nil {
			return err
		}
		readings, err := forecasting.Compute(period, period.LocalDay(time.Now()), deals)
		if err != nil {
			return err
		}
		id, err = store.TakeSnapshot(ctx, tx, forecasting.NewSnapshot{
			Period: period, Scope: forecasting.Scope{Kind: forecasting.ScopeWorkspace},
			Trigger: forecasting.TriggerCall, BaseCurrency: baseCurrency,
			Readings: readings, TakenAt: time.Now(),
		})
		return err
	}); err != nil {
		t.Fatalf("freezing the pair: %v", err)
	}
	return id
}

// seedDealClosingWithin plants a priced open deal expected two days from now,
// so it falls in the current quarter whatever day the suite runs on.
func (e *forecastEnv) seedDealClosingWithin(t *testing.T, name string, owner *ids.UUID, amountMinor int64) {
	t.Helper()
	e.seedID(t, `INSERT INTO deal (id, name, pipeline_id, stage_id, owner_id, amount_minor, currency,
			expected_close_date, source, captured_by)
		VALUES ($1, $2, $3, $4, $5, $6, 'EUR', (now() + interval '2 days')::date, 'manual', 'human:x')`,
		name, e.pipeline, e.stages[20], owner, amountMinor)
}

// writerUser is the seat snapshotWriterCtx acts as.
//
// A REAL app_user row, seeded by the env, because analytics_share.created_by is
// a foreign key: a principal whose user id names nobody can read a forecast but
// cannot issue a share, and a test that minted one would be exercising a
// principal production never builds.
func (e *forecastEnv) writerCtx(t *testing.T) context.Context {
	t.Helper()
	e.seedSeatHolding(t, writerUser, "writer@forecast.test", "share_issuer")
	return snapshotWriterCtx(e.WS)
}

// seedSeatHolding plants a seat and the ROLE that grants it forecast:read.
//
// The role is the point. Resolving a share re-evaluates the issuer's standing
// through the same loadGrants login runs, and that reads role_assignment — so a
// seat with no role issues a link that refuses to open, which is the check
// working and a fixture that would prove nothing.
func (e *forecastEnv) seedSeatHolding(t *testing.T, user ids.UUID, email, roleKey string) {
	t.Helper()
	ctx := context.Background()
	if _, err := e.owner.Exec(ctx,
		`INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, 'Seat')
		 ON CONFLICT (id) DO NOTHING`, user, email); err != nil {
		t.Fatalf("seeding the seat: %v", err)
	}
	var roleID ids.UUID
	if err := e.owner.QueryRow(ctx,
		`INSERT INTO role (key, name, permissions)
		 VALUES ($1, $1, '{"objects":{"forecast":{"read":true}},"row_scope":"all"}'::jsonb)
		 ON CONFLICT (key) DO UPDATE SET name = EXCLUDED.name
		 RETURNING id`, roleKey).Scan(&roleID); err != nil {
		t.Fatalf("seeding the %s role: %v", roleKey, err)
	}
	if _, err := e.owner.Exec(ctx,
		`INSERT INTO role_assignment (role_id, user_id) VALUES ($1, $2)
		 ON CONFLICT DO NOTHING`, roleID, user); err != nil {
		t.Fatalf("assigning the %s role: %v", roleKey, err)
	}
}

// writerUser is fixed rather than minted per call so the seeded row and the
// principal name the same seat.
var writerUser = ids.NewV7()

// snapshotWriterCtx is the unbounded principal the nightly job runs as — the
// issuer's side of a share, who sees both teams.
func snapshotWriterCtx(ws ids.UUID) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), ws)
	// The write shape links the audit row to a request's trace, and a snapshot
	// is a write. Without one, TakeSnapshot refuses rather than writing an
	// unattributable row — which is the guard working, not a test fixture gap.
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:writer", UserID: writerUser,
		Permissions: principal.Permissions{
			Objects: map[string]principal.ObjectGrant{
				"forecast":              {Read: true, Create: true, Delete: true},
				"deal":                  {Read: true},
				"installation_settings": {Read: true},
			},
			RowScope: principal.RowScopeAll,
		},
	})
}

// forecastReader adds forecast:read to an existing principal.
//
// A share recipient holds it: the route that opens a share is a forecast read,
// and forecasting.Store.InTx gates every transaction on it. The deal grants and
// the row scope are what the recompute then narrows by, and those are the two
// this file varies.
func (e *forecastEnv) forecastReader(ctx context.Context) context.Context {
	p, ok := principal.Actor(ctx)
	if !ok {
		panic("forecastReader: no actor to widen")
	}
	objects := map[string]principal.ObjectGrant{"forecast": {Read: true}}
	for name, grant := range p.Permissions.Objects {
		objects[name] = grant
	}
	p.Permissions.Objects = objects
	return principal.WithActor(ctx, p)
}

func (e *forecastEnv) readShared(ctx context.Context, t *testing.T, id ids.UUID) SharedSnapshot {
	t.Helper()
	var out SharedSnapshot
	if err := forecasting.NewStore(InstallationDB(e.Pool)).InTx(ctx,
		func(ctx context.Context, tx pgx.Tx) error {
			var err error
			out, err = ReadSharedSnapshot(ctx, tx, id)
			return err
		}); err != nil {
		t.Fatalf("reading the shared snapshot: %v", err)
	}
	return out
}

func TestASharedSnapshotRecomputesRatherThanServingTheStoredTotal(t *testing.T) {
	const mine, theirs = 100_000, 250_000
	e := setupForecast(t)
	id := e.freezeTwoTeams(t, mine, theirs)

	// The issuer's own reading is the whole frozen state: both deals.
	whole := e.readShared(snapshotWriterCtx(e.WS), t, id)
	if whole.Readings.OpenMinor != mine+theirs {
		t.Fatalf("the issuer reads %d open and the frozen state holds %d",
			whole.Readings.OpenMinor, mine+theirs)
	}
	if whole.Withheld {
		t.Error("nothing was withheld from a reader who sees every deal, and the answer says otherwise")
	}

	// A rep on one team reads THEIR team's half, not the whole frozen state.
	//
	// Two different questions, and only the first was ever asked here. Whether
	// this seat may READ a deal is row scope, and the answer is yes — a deal is
	// an identity table (auth/tableclass.go), read by every seat, and the
	// ruling behind that stands: the opposite once gave a consultant a 404 on
	// their own project. Whether they may MEASURE a population is the separate
	// question the analytics scope answers, and a snapshot's readings are a
	// measurement rather than a list of rows.
	//
	// Without that second question a link WIDENED its opener: issue one over
	// the workspace, hand it to a rep, and they read the whole installation's
	// forecast — authority lent by sharing rather than granted. A share may
	// narrow what its opener sees and must not widen it.
	narrow := e.readShared(e.forecastReader(
		e.dealReadCtx(e.Rep1, []ids.UUID{e.Team1}, principal.RowScopeTeam)), t, id)
	if narrow.Readings.OpenMinor != mine {
		t.Errorf("a team-scoped reader read %d open, want their own team's %d — a link "+
			"that answers wider than its opener may measure lends authority",
			narrow.Readings.OpenMinor, mine)
	}
	if narrow.Readings.EligibleCount != 1 {
		t.Errorf("a team-scoped reader counted %d deals, want the 1 in their population",
			narrow.Readings.EligibleCount)
	}
	// And the answer SAYS something was kept back, which is what stops the
	// narrower total reading as the whole frozen state.
	if !narrow.Withheld {
		t.Error("the snapshot held deals outside this reader's population and the answer " +
			"does not say so — a partial total presented as a complete one")
	}
}

func TestAMaskedRecipientReadsNoMoneyOutOfAFrozenSnapshot(t *testing.T) {
	const mine, theirs = 100_000, 250_000
	e := setupForecast(t)
	id := e.freezeTwoTeams(t, mine, theirs)

	// MaskAlways withholds the amount on every row, so every row leaves the
	// money. The counts go with them: a masked row excluded from the total and
	// counted in eligible_count would advertise a deal whose value is withheld,
	// and the two numbers would not reconcile against each other.
	got := e.readShared(e.forecastReader(e.maskedDealReader(principal.MaskAlways)), t, id)
	if got.Readings.OpenMinor != 0 {
		t.Errorf("a fully masked reader read %d open and may read no amount at all",
			got.Readings.OpenMinor)
	}
	if got.Readings.EligibleCount != 0 {
		t.Errorf("a fully masked reader counted %d deals; every row left the sum",
			got.Readings.EligibleCount)
	}
	if !got.Withheld {
		t.Error("every row was withheld and the answer does not say so")
	}
}

func TestASnapshotThatDoesNotExistIsNotAnEmptyQuarter(t *testing.T) {
	e := setupForecast(t)
	err := forecasting.NewStore(InstallationDB(e.Pool)).InTx(snapshotWriterCtx(e.WS),
		func(ctx context.Context, tx pgx.Tx) error {
			_, err := ReadSharedSnapshot(ctx, tx, ids.NewV7())
			return err
		})
	if err == nil {
		t.Fatal("reading a snapshot that was never taken answered zeroes, which reads as an empty quarter")
	}
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("reading an absent snapshot answered %v, and the caller needs not-found", err)
	}
}

// shareStore is the store under test, with a clock the expiry tests can move.
func (e *forecastEnv) shareStore(now func() time.Time) *AnalyticsShareStore {
	return NewAnalyticsShareStore(now)
}

func (e *forecastEnv) issue(ctx context.Context, t *testing.T, in NewShare) (Share, string) {
	t.Helper()
	var share Share
	var token string
	if err := forecasting.NewStore(InstallationDB(e.Pool)).InTx(ctx,
		func(ctx context.Context, tx pgx.Tx) error {
			var err error
			share, token, err = e.shareStore(time.Now).Issue(ctx, tx, in)
			return err
		}); err != nil {
		t.Fatalf("issuing the share: %v", err)
	}
	return share, token
}

func TestASharesPlaintextTokenNeverReachesTheTable(t *testing.T) {
	e := setupForecast(t)
	ctx := e.writerCtx(t)
	_, token := e.issue(ctx, t, NewShare{
		Kind: shareKindLive, Target: "forecast",
		Scope: forecasting.Scope{Kind: forecasting.ScopeWorkspace},
	})
	if token == "" {
		t.Fatal("issuing a share returned no token, so nothing was handed out")
	}

	// The whole row, as text. A column-by-column check would pass over a column
	// added later that happened to carry the token.
	var rows int
	if err := e.owner.QueryRow(context.Background(),
		`SELECT count(*) FROM analytics_share WHERE analytics_share::text LIKE '%' || $1 || '%'`,
		token).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 0 {
		t.Errorf("%d share row(s) hold the plaintext token; a database dump opens the link", rows)
	}

	// And the digest IS there, so the check above passed for the right reason
	// rather than because nothing was written.
	var stored int
	if err := e.owner.QueryRow(context.Background(),
		`SELECT count(*) FROM analytics_share WHERE token_hash IS NOT NULL`).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored != 1 {
		t.Fatalf("expected one share row holding a digest and found %d", stored)
	}
}

func TestARevokedShareStopsServing(t *testing.T) {
	e := setupForecast(t)
	ctx := e.writerCtx(t)
	share, token := e.issue(ctx, t, NewShare{
		Kind: shareKindLive, Target: "forecast",
		Scope: forecasting.Scope{Kind: forecasting.ScopeWorkspace},
	})

	store := forecasting.NewStore(InstallationDB(e.Pool))
	if err := store.InTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		_, err := e.shareStore(time.Now).Resolve(ctx, tx, token)
		return err
	}); err != nil {
		t.Fatalf("the share refused to open before it was revoked: %v", err)
	}

	if err := store.InTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return e.shareStore(time.Now).Revoke(ctx, tx, share.ID)
	}); err != nil {
		t.Fatalf("revoking: %v", err)
	}

	err := store.InTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		_, err := e.shareStore(time.Now).Resolve(ctx, tx, token)
		return err
	})
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("a revoked share answered %v; every refusal is the same not-found", err)
	}

	// Revoking twice is the outcome the caller asked for, not a conflict.
	if err := store.InTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return e.shareStore(time.Now).Revoke(ctx, tx, share.ID)
	}); err != nil {
		t.Errorf("revoking an already-revoked share failed with %v; it is idempotent", err)
	}
}

func TestAnExpiredShareStopsServing(t *testing.T) {
	e := setupForecast(t)
	ctx := e.writerCtx(t)
	_, token := e.issue(ctx, t, NewShare{
		Kind: shareKindLive, Target: "forecast",
		Scope: forecasting.Scope{Kind: forecasting.ScopeWorkspace},
	})

	// The clock moves past the ceiling rather than the row being edited: what
	// is under test is the resolve path's expiry check, and a test that
	// back-dated the row would prove the same thing about a row production
	// never writes.
	later := func() time.Time { return time.Now().Add(ShareLifetimeCeiling + time.Hour) }
	err := forecasting.NewStore(InstallationDB(e.Pool)).InTx(ctx,
		func(ctx context.Context, tx pgx.Tx) error {
			_, err := e.shareStore(later).Resolve(ctx, tx, token)
			return err
		})
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("an expired share answered %v; every refusal is the same not-found", err)
	}
}

func TestAnExpiryBeyondTheCeilingIsRefusedRatherThanShortened(t *testing.T) {
	e := setupForecast(t)
	err := forecasting.NewStore(InstallationDB(e.Pool)).InTx(snapshotWriterCtx(e.WS),
		func(ctx context.Context, tx pgx.Tx) error {
			_, _, err := e.shareStore(time.Now).Issue(ctx, tx, NewShare{
				Kind: shareKindLive, Target: "forecast",
				Scope:     forecasting.Scope{Kind: forecasting.ScopeWorkspace},
				ExpiresAt: time.Now().Add(365 * 24 * time.Hour),
			})
			return err
		})
	if !errors.Is(err, apperrors.ErrInvalidArgument) {
		t.Fatalf("a year-long share was accepted (%v); the ceiling refuses rather than shortening, so the caller is not handed a link that dies early in front of somebody", err)
	}

	// Nothing was written. A refusal that left a row would leave a share
	// nobody holds a token for and nobody can revoke.
	var rows int
	if err := e.owner.QueryRow(context.Background(),
		`SELECT count(*) FROM analytics_share`).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 0 {
		t.Errorf("a refused issue left %d row(s) behind", rows)
	}
}

func TestAShareStopsServingWhenItsIssuerLoses(t *testing.T) {
	e := setupForecast(t)
	issuer := ids.NewV7()
	e.seedSeatHolding(t, issuer, "issuer@forecast.test", "share_issuer_two")
	ctx := principal.WithCorrelationID(
		principal.WithWorkspaceID(context.Background(), e.WS), ids.NewV7())
	ctx = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + issuer.String(), UserID: issuer,
		Permissions: principal.Permissions{
			Objects: map[string]principal.ObjectGrant{
				"forecast": {Read: true, Create: true}, "deal": {Read: true},
				"installation_settings": {Read: true},
			},
			RowScope: principal.RowScopeAll,
		},
	})
	_, token := e.issue(ctx, t, NewShare{
		Kind: shareKindLive, Target: "forecast",
		Scope: forecasting.Scope{Kind: forecasting.ScopeWorkspace},
	})

	// Deactivated, NOT archived. This is the half a predicate on archived_at
	// alone would miss, and it is the ordinary way a colleague leaves.
	if _, err := e.owner.Exec(context.Background(),
		`UPDATE app_user SET status = 'deactivated' WHERE id = $1`, issuer); err != nil {
		t.Fatal(err)
	}

	err := forecasting.NewStore(InstallationDB(e.Pool)).InTx(snapshotWriterCtx(e.WS),
		func(ctx context.Context, tx pgx.Tx) error {
			_, err := e.shareStore(time.Now).Resolve(ctx, tx, token)
			return err
		})
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("a departed colleague's link still serves (%v); a share is not a grant that outlives the seat behind it", err)
	}
}

func TestTheExportedRowsAreTheRowsTheHeadlineWasSummedFrom(t *testing.T) {
	const mine, theirs = 100_000, 250_000
	e := setupForecast(t)
	id := e.freezeTwoTeams(t, mine, theirs)
	store := forecasting.NewStore(InstallationDB(e.Pool))

	// A reader who sees everything: two rows, and a headline over two.
	whole := e.readShared(e.writerCtx(t), t, id)
	var rows []SharedContribution
	if err := store.InTx(e.writerCtx(t), func(ctx context.Context, tx pgx.Tx) error {
		var err error
		rows, err = SharedSnapshotRows(ctx, tx, id)
		return err
	}); err != nil {
		t.Fatalf("reading the rows: %v", err)
	}
	if len(rows) != whole.Readings.EligibleCount {
		t.Errorf("the export has %d rows and the headline counted %d deals; a file that does not add up to the total above it gets reconciled by hand",
			len(rows), whole.Readings.EligibleCount)
	}

	// A masked reader: no rows, and a headline over none. Both narrow through
	// sharedVisibilityClause, and this is what proves they narrow together.
	masked := e.forecastReader(e.maskedDealReader(principal.MaskAlways))
	narrow := e.readShared(masked, t, id)
	var maskedRows []SharedContribution
	if err := store.InTx(masked, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		maskedRows, err = SharedSnapshotRows(ctx, tx, id)
		return err
	}); err != nil {
		t.Fatalf("reading the masked rows: %v", err)
	}
	if len(maskedRows) != narrow.Readings.EligibleCount {
		t.Errorf("a masked reader's export has %d rows and their headline counted %d",
			len(maskedRows), narrow.Readings.EligibleCount)
	}
	if len(maskedRows) != 0 {
		t.Errorf("a fully masked reader was handed %d row(s) of deal money", len(maskedRows))
	}
}

func TestAnExportedCellCannotCarryAFormulaIntoASpreadsheet(t *testing.T) {
	rows := []SharedContribution{{DealName: "=cmd|' /c calc'!A1", Currency: "EUR"}}
	body, err := shareRowsCSV(rows)
	if err != nil {
		t.Fatal(err)
	}
	// The quote is what makes a spreadsheet read the cell as text. Without it
	// the file is an attack on whoever opens it, delivered by the product.
	if !strings.Contains(string(body), `,'=cmd`) {
		t.Errorf("a formula-leading name was exported unguarded:\n%s", body)
	}
}

func TestAnUnpricedDealExportsAnEmptyCellRatherThanAZero(t *testing.T) {
	rows := []SharedContribution{{DealName: "Unpriced", ExclusionReason: "unpriced"}}
	body, err := shareRowsCSV(rows)
	if err != nil {
		t.Fatal(err)
	}
	// A zero here reads as a deal somebody expects nothing from, which is a
	// different fact from having no price yet.
	if strings.Contains(string(body), ",0,") {
		t.Errorf("an unpriced deal exported a zero amount:\n%s", body)
	}
}

func TestASeatThatCanIssueAShareCanCloseIt(t *testing.T) {
	e := setupForecast(t)
	// The role a real installation seeds, read from the policy defaults rather
	// than invented here. The first version of this file built a principal with
	// Delete: true and passed — against an authority no seat in the product
	// holds, because the forecast object is seeded create+read and nothing
	// grants delete. Revoke was gated on that verb, so every share was
	// permanent and no test noticed.
	seat := ids.NewV7()
	e.seedSeatHolding(t, seat, "issuer-real@forecast.test", "share_issuer_real")
	ctx := principal.WithCorrelationID(
		principal.WithWorkspaceID(context.Background(), e.WS), ids.NewV7())
	ctx = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + seat.String(), UserID: seat,
		Permissions: principal.Permissions{
			Objects: map[string]principal.ObjectGrant{
				// EXACTLY the seeded forecast posture: create and read, no
				// delete. Spelled out so a reader can see that the absence is
				// the point of the test.
				"forecast":              {Create: true, Read: true},
				"deal":                  {Read: true},
				"installation_settings": {Read: true},
			},
			RowScope: principal.RowScopeAll,
		},
	})

	share, token := e.issue(ctx, t, NewShare{
		Kind: shareKindLive, Target: "forecast",
		Scope: forecasting.Scope{Kind: forecasting.ScopeWorkspace},
	})

	store := forecasting.NewStore(InstallationDB(e.Pool))
	if err := store.InTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return e.shareStore(time.Now).Revoke(ctx, tx, share.ID)
	}); err != nil {
		t.Fatalf("the seat that issued this share cannot close it: %v — a link nobody can revoke is permanent", err)
	}

	// And it actually stopped serving, rather than the call merely succeeding.
	err := store.InTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		_, err := e.shareStore(time.Now).Resolve(ctx, tx, token)
		return err
	})
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("a revoked share answered %v; it must stop serving", err)
	}
}

// A team manager cannot issue a link to a population they may not measure.
//
// `forecast.create` says they may make a share; it does not say which
// population they may make one ABOUT. The shape check beside it only asked
// whether a workspace scope carried no id — never whether this seat held the
// installation. So a team-lens manager could mint a link to the whole
// forecast and hand it to anybody, which is authority granted by issuing.
func TestATeamManagerCannotIssueAWorkspaceShare(t *testing.T) {
	e := setupForecast(t)
	id := e.freezeTwoTeams(t, 100_000, 250_000)

	manager := e.forecastReader(e.dealReadCtx(e.Rep1, []ids.UUID{e.Team1}, principal.RowScopeTeam))
	manager = withForecastCreate(manager)

	_, _, err := e.issueShare(manager, NewShare{
		Kind: "snapshot", Target: "someone@example.test",
		Scope:      forecasting.Scope{Kind: forecasting.ScopeWorkspace},
		SnapshotID: &id,
		ExpiresAt:  time.Now().UTC().Add(24 * time.Hour),
	})
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("a team-lens manager issued a workspace share and got %v, want a refusal — "+
			"a link they may mint is a link about a population they may measure", err)
	}
}

// A snapshot share's scope must be the snapshot's own.
//
// The frozen readings were computed over ONE population. A link claiming a
// different one labels those numbers with a population nobody computed them
// for, and the recipient has no way to tell.
func TestASnapshotShareCannotClaimADifferentPopulation(t *testing.T) {
	e := setupForecast(t)
	id := e.freezeTwoTeams(t, 100_000, 250_000)

	// A REAL seat holding the whole workspace, so the issue reaches the scope
	// check rather than dying on the created_by foreign key — and so removing
	// the check produces a successfully issued link, which is the thing this
	// case has to be able to see.
	issuer := principal.WithCorrelationID(
		withForecastCreate(e.forecastReader(
			e.dealReadCtx(e.Rep1, nil, principal.RowScopeAll))),
		ids.NewV7())
	team := e.Team1
	_, _, err := e.issueShare(issuer, NewShare{
		Kind: "snapshot", Target: "someone@example.test",
		Scope:      forecasting.Scope{Kind: forecasting.ScopeTeam, ID: &team},
		SnapshotID: &id,
		ExpiresAt:  time.Now().UTC().Add(24 * time.Hour),
	})
	if !errors.Is(err, apperrors.ErrInvalidArgument) {
		t.Fatalf("a snapshot share naming a different population than the snapshot "+
			"answered %v, want a refusal — the stored numbers would be labelled with "+
			"a population nobody computed them for", err)
	}
}

// withForecastCreate lends the create grant without widening the row lens,
// which is the pair the defect needed: authority to share, no authority over
// the population being shared.
func withForecastCreate(ctx context.Context) context.Context {
	p, _ := principal.Actor(ctx)
	objects := map[string]principal.ObjectGrant{}
	for name, grant := range p.Permissions.Objects {
		objects[name] = grant
	}
	forecast := objects["forecast"]
	forecast.Create = true
	objects["forecast"] = forecast
	p.Permissions.Objects = objects
	return principal.WithActor(ctx, p)
}

func (e *forecastEnv) issueShare(
	ctx context.Context, in NewShare,
) (Share, string, error) {
	var (
		out Share
		raw string
	)
	err := forecasting.NewStore(InstallationDB(e.Pool)).InTx(ctx,
		func(ctx context.Context, tx pgx.Tx) error {
			var issueErr error
			out, raw, issueErr = NewAnalyticsShareStore(
				func() time.Time { return time.Now().UTC() }).Issue(ctx, tx, in)
			return issueErr
		})
	return out, raw, err
}

// A team manager cannot assert the WHOLE installation's forecast.
//
// `POST /forecast/calls` took `scope_kind` from the body and wrote it. The RBAC
// gate asks whether this seat may create a forecast call, never which
// population they may assert one for — and the field defaults to `workspace`,
// so a team-lens manager who sent no scope at all was creating the
// installation's standing call and superseding whatever management had
// asserted. The app hides the editor for them, which is a courtesy and not a
// check.
//
// Driven through the seam the handler now applies, which is where the answer
// is decided. That the handler cannot skip it is structural rather than
// asserted: `resolve` is a required argument to NewHandlers, so a forecast
// surface built without one does not compile.
func TestATeamManagerCannotAssertTheWorkspacesForecast(t *testing.T) {
	e := setupForecast(t)
	manager := e.dealReadCtx(e.Rep1, []ids.UUID{e.Team1}, principal.RowScopeTeam)

	_, err := e.writableScope(manager, forecasting.Scope{Kind: forecasting.ScopeWorkspace})
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("a team-lens manager claiming the workspace answered %v, want a refusal — "+
			"the standing forecast of the whole installation is management's to assert", err)
	}

	// And naming nothing does not silently mean the workspace: it resolves to
	// what they DO hold, which is the default the contract's own wording made
	// dangerous by spelling it `workspace`.
	got, err := e.writableScope(manager, forecasting.Scope{})
	if err != nil {
		t.Fatalf("a manager's own default was refused: %v", err)
	}
	if got.Kind == forecasting.ScopeWorkspace {
		t.Errorf("naming no scope resolved to the workspace for a team-lens manager — "+
			"the defaulting is the whole defect, and it answered %q", got.Kind)
	}
}

func (e *forecastEnv) writableScope(
	ctx context.Context, requested forecasting.Scope,
) (forecasting.Scope, error) {
	var out forecasting.Scope
	err := forecasting.NewStore(InstallationDB(e.Pool)).InTx(e.forecastReader(ctx),
		func(ctx context.Context, tx pgx.Tx) error {
			var scopeErr error
			out, scopeErr = ForecastWritableScope(ctx, tx, requested)
			return scopeErr
		})
	return out, err
}
