// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// What the deals list orders by when the column it draws is not a column of
// `deal`.
//
// A column the list SHOWS is a column the list SORTS BY (Lars, 2026-08-21), and
// three of the eight it draws are references: a stage and the two
// companies. Each is spelled here as the expression that orders it, beside
// the rule it mirrors.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"maps"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/ports/fieldcatalog"
)

// orderByStagePosition orders by a stage's place in its PIPELINE, not by its
// name.
//
// Alphabetical is almost never what somebody sorting by stage means: they want
// the funnel, and "Discovery, Negotiation, Proposal" is the funnel shuffled.
// `stage` is workspace configuration and carries no row scope, so the position
// is the same number for every reader.
func orderByStagePosition(context.Context, func(any) int) (string, error) {
	return "(SELECT stage_sort.position FROM stage stage_sort WHERE stage_sort.id = deal.stage_id)", nil
}

// orderByPriorityRank orders by the generated rank beside `priority`, so the
// order is the business one — High, Medium, Low, then the deals nobody has
// prioritised — rather than the alphabetical one the text column would give
// ('high' < 'low' < 'medium' interleaves them meaninglessly).
//
// The rank is a STORED generated column rather than a CASE rendered here, so
// the index on it can serve this ordering and there is no writer that could
// set one without the other.
func orderByPriorityRank(context.Context, func(any) int) (string, error) {
	return "deal.priority_rank", nil
}

// orderByReadableCompanyName orders by the referenced company's name, and by
// NOTHING for a reference this caller may not read.
//
// Ordering by a value is reading it — the rule refuseMaskedSort already applies
// to masked amounts — so BOTH halves of RBAC bound it.
//
// The object grant first: auth.ScopeClauseFor answers row visibility and never
// asks whether this caller may read companies at all, so a seat holding
// deal.read and no company.read would otherwise have its page arranged by
// company names it is refused on every other surface.
//
// Then the row scope, INSIDE the subquery rather than beside it: a reference
// outside the caller's scope answers NULL, which the ORDER BY already puts
// last, so those deals land in the tail together and the order says nothing
// about which company they name. That is the same answer the row itself gives,
// where the reference is withheld.
func orderByReadableCompanyName(column string) func(context.Context, func(any) int) (string, error) {
	return func(ctx context.Context, arg func(any) int) (string, error) {
		if !auth.ReadGranted(ctx, "company") {
			// Ordered by nothing: every row sits in the tail and the page
			// falls back to its tie-breaker.
			return "NULL::text", nil
		}
		scope, err := auth.ScopeClauseFor(ctx, "company", "company_sort", arg)
		if err != nil {
			return "", err
		}
		if scope != "" {
			scope = " AND " + scope
		}
		// The column is one of this map's own keys, never a caller's string.
		return storekit.SQLf(
			"(SELECT company_sort.display_name FROM company company_sort WHERE company_sort.id = deal.%s%s)",
			column, scope), nil
	}
}

// orderByBaseValue orders by the deal's value in the installation's base
// currency, through BaseValueSQL on the valuation day. A deal with no rate
// answers NULL and sits in the tail with the deals that carry no amount.
func orderByBaseValue(v dealValuation) func(context.Context, func(any) int) (string, error) {
	return func(_ context.Context, arg func(any) int) (string, error) {
		return valueInBaseSQL(v, arg), nil
	}
}

// valueInBaseSQL is the deal's base-currency value on the valuation day, the one
// expression the Value sort and the value filter both read.
func valueInBaseSQL(v dealValuation, arg func(any) int) string {
	return BaseValueSQL(storekit.SQLf("$%d::date", arg(v.day)), storekit.SQLf("$%d::text", arg(v.base)), dealTable)
}

// closeDayField sorts by the "Close" column: the day a deal closed, or for an
// open deal the day it is expected to.
const closeDayField = "close_day"

// orderByCloseDay orders by the day the Close column shows. A closed deal
// uses its closed_at in the installation's zone, an open deal its forecast.
// A closed deal with no closed_at sits in the tail, as its column cell is empty.
func orderByCloseDay(zone string) func(context.Context, func(any) int) (string, error) {
	return func(_ context.Context, arg func(any) int) (string, error) {
		return closeDaySQL(arg(zone)), nil
	}
}

// closeDaySQL is the Close column's day, cut in the zone bound at zonePos. The
// close-day sort and the close-date filter both read it.
func closeDaySQL(zonePos int) string {
	return storekit.SQLf(
		"(CASE WHEN deal.status IN ('won', 'lost') THEN (deal.closed_at AT TIME ZONE $%d::text)::date ELSE deal.expected_close_date END)",
		zonePos)
}

// dealListVocabulary is dealListFields with the sorts that need the
// installation added.
//
// Value orders in the base currency, because raw minor units rank ¥150,000
// above €1,000. Each cursor field digests what its key depends on, so a cursor
// minted under another base, day or zone is refused. The close day depends on
// the zone alone, so a page continues across local midnight.
func dealListVocabulary(v dealValuation) map[string]storekit.SortField {
	vocab := maps.Clone(dealListFields)
	vocab[dealAmountField] = storekit.SortField{
		Kind:        fieldcatalog.TypeCurrency,
		Expr:        orderByBaseValue(v),
		CursorField: "amount_base:" + v.digest(),
	}
	vocab[closeDayField] = storekit.SortField{
		Kind:        fieldcatalog.TypeDate,
		Expr:        orderByCloseDay(v.zone),
		CursorField: "close_day:" + digestOf(v.zone),
	}
	return vocab
}

// dealValuation is what the installation-bound sorts read: base currency and
// rate day for Value, and the zone name close days are cut in.
type dealValuation struct {
	base string
	day  string
	zone string
}

// digest names the valuation in a cursor without spelling it out. Only
// equality is ever asked of it.
func (v dealValuation) digest() string {
	return digestOf(v.base, v.day)
}

func digestOf(parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, "|")))
	return hex.EncodeToString(sum[:8])
}

// valuationTx reads the valuation through the ungated readers: a seat that
// may read deals may order them by value.
func (s *Store) valuationTx(ctx context.Context, tx pgx.Tx) (dealValuation, error) {
	base, err := s.installation.BaseCurrencyApplied(ctx, tx)
	if err != nil {
		return dealValuation{}, err
	}
	zone, err := s.installation.TimezoneApplied(ctx, tx)
	if err != nil {
		return dealValuation{}, fmt.Errorf("resolve the installation's timezone: %w", err)
	}
	day, err := s.todayIn(ctx, tx, func(context.Context, pgx.Tx) (string, error) { return zone, nil })
	if err != nil {
		return dealValuation{}, err
	}
	return dealValuation{base: base, day: day.Format(time.DateOnly), zone: zone}, nil
}

// sortsByCloseDay reports whether a sort spec names the close day, in either
// direction.
func sortsByCloseDay(sort *string) bool {
	return sort != nil && strings.TrimPrefix(strings.TrimSpace(*sort), "-") == closeDayField
}

// sortsByValue reports whether a sort spec names Value, in either direction.
func sortsByValue(sort *string) bool {
	return sort != nil && strings.TrimPrefix(strings.TrimSpace(*sort), "-") == dealAmountField
}
