// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"errors"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"
)

// The definition is the (a) half of AC-R6: the exact filter + group +
// aggregate in plain language. These are golden strings — a wording
// change is a product change and must show up here.
func TestRenderDefinitionReadsAsPlainLanguage(t *testing.T) {
	spec := prebuiltReports["forecast"]

	got, err := renderDefinition(spec,
		[]boundPredicate{{Field: "pipeline_id", Value: "018f-pipe"}},
		[]boundPredicate{{Field: "owner_id", Value: "018f-owner"}},
		[]reportAggregate{
			{Fn: "count", As: "deals"},
			{Fn: "sum", Field: "amount_minor", As: "unweighted_minor"},
			{Fn: "sum", Field: "weighted_amount_minor", As: "weighted_minor"},
		})
	if err != nil {
		t.Fatal(err)
	}
	want := `Over open, unarchived deals (win probability read live from the deal's current stage; ` +
		`a commit/best_case deal whose close date is past, missing, or provisional reports as 'slipped' instead, per formulas §11), ` +
		`filtered to pipeline_id = "018f-pipe", within the group where owner_id = "018f-owner": ` +
		`the number of matching records as deals; the sum of amount_minor as unweighted_minor; ` +
		`the sum of weighted_amount_minor as weighted_minor.`
	if got != want {
		t.Errorf("definition:\n got %q\nwant %q", got, want)
	}
}

func TestRenderDefinitionSpellsOutTheNullGroup(t *testing.T) {
	got, err := renderDefinition(prebuiltReports["forecast"], nil,
		[]boundPredicate{{Field: "owner_id", IsNull: true}},
		[]reportAggregate{{Fn: "count"}})
	if err != nil {
		t.Fatal(err)
	}
	want := `Over open, unarchived deals (win probability read live from the deal's current stage; ` +
		`a commit/best_case deal whose close date is past, missing, or provisional reports as 'slipped' instead, per formulas §11), ` +
		`within the group where owner_id is not set: the number of matching records.`
	if got != want {
		t.Errorf("definition:\n got %q\nwant %q", got, want)
	}
}

// An unset column and a column holding the empty string are different facts,
// and the sentence a reader is shown has to say which one they are looking at.
func TestRenderDefinitionTellsUnsetApartFromEmptyText(t *testing.T) {
	unset, err := renderDefinition(prebuiltReports["forecast"], nil,
		[]boundPredicate{{Field: "owner_id", IsNull: true}}, []reportAggregate{{Fn: "count"}})
	if err != nil {
		t.Fatal(err)
	}
	empty, err := renderDefinition(prebuiltReports["forecast"], nil,
		[]boundPredicate{{Field: "owner_id", Value: ""}}, []reportAggregate{{Fn: "count"}})
	if err != nil {
		t.Fatal(err)
	}
	if unset == empty {
		t.Fatalf("an unset column and an empty one read identically: %q", unset)
	}
	if !strings.Contains(unset, "owner_id is not set") {
		t.Errorf("unset reads %q", unset)
	}
	if !strings.Contains(empty, `owner_id = ""`) {
		t.Errorf("empty text reads %q", empty)
	}
}

func TestRenderDefinitionRejectsUnknownAggregate(t *testing.T) {
	_, err := renderDefinition(prebuiltReports["forecast"], nil, nil,
		[]reportAggregate{{Fn: "median", Field: "amount_minor"}})
	var notAllowed *FieldNotAllowedError
	if !errors.As(err, &notAllowed) {
		t.Fatalf("unknown fn → %v, want FieldNotAllowedError", err)
	}
}

// A handle we mint must resolve: parseDerivationQuery is derivationURL's
// exact inverse, including the empty-string spelling of a NULL group key.
func TestDerivationURLRoundTrip(t *testing.T) {
	aggs := []reportAggregate{
		{Fn: "count", As: "deals"},
		{Fn: "sum", Field: "amount_minor", As: "unweighted_minor"},
	}
	minted := derivationURL("forecast",
		map[string]any{"pipeline_id": "018f-pipe"},
		[]string{"owner_id", "forecast_category"},
		aggs,
		map[string]any{"owner_id": "018f-owner", "forecast_category": nil, "deals": int64(3)},
		time.Time{})

	parsed, err := url.Parse(minted)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Path != "/v1/reports/forecast/derivation" {
		t.Errorf("path = %q", parsed.Path)
	}
	q, err := parseDerivationQuery(parsed.Query())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(q.GroupBy, []string{"forecast_category", "owner_id"}) {
		t.Errorf("group_by = %v", q.GroupBy)
	}
	if !reflect.DeepEqual(q.Aggregates, aggs) {
		t.Errorf("aggregates = %+v", q.Aggregates)
	}
	wantPreds := map[string]string{
		"pipeline_id": "018f-pipe",
		"owner_id":    "018f-owner",
	}
	if !reflect.DeepEqual(q.Predicates, wantPreds) {
		t.Errorf("predicates = %v, want %v", q.Predicates, wantPreds)
	}
	// An unset group key is NAMED as unset rather than carried as an empty
	// value, so it cannot be confused with a column holding the empty string.
	if !reflect.DeepEqual(q.Unset, map[string]bool{"forecast_category": true}) {
		t.Errorf("unset = %v, want forecast_category", q.Unset)
	}
}

func TestParseDerivationQueryRejectsMalformedHandles(t *testing.T) {
	for name, raw := range map[string]string{
		"agg without triplet": "agg=sum",
		"agg without fn":      "agg=:amount_minor:x",
		"repeated predicate":  "owner_id=a&owner_id=b",
	} {
		values, err := url.ParseQuery(raw)
		if err != nil {
			t.Fatal(err)
		}
		_, err = parseDerivationQuery(values)
		var notAllowed *FieldNotAllowedError
		if !errors.As(err, &notAllowed) {
			t.Errorf("%s → %v, want FieldNotAllowedError", name, err)
		}
	}
}

// The handle's query string reserves `by`, `agg`, and the injected
// row key; no report vocabulary may squat on them, or a minted URL
// would be ambiguous. Derived from the catalog, not a list.
func TestReportVocabularyAvoidsReservedDerivationNames(t *testing.T) {
	reserved := map[string]bool{}
	for _, key := range reservedDerivationKeys {
		reserved[key] = true
	}
	for report, spec := range prebuiltReports {
		for _, vocab := range []map[string]string{spec.dimensions, spec.measures, spec.filters} {
			for field := range vocab {
				if reserved[field] {
					t.Errorf("report %q: field %q collides with a reserved derivation key", report, field)
				}
			}
		}
		for _, agg := range spec.defaultAggs {
			if reserved[agg.As] {
				t.Errorf("report %q: default aggregate alias %q collides with a reserved derivation key", report, agg.As)
			}
		}
	}
}

// A caller-chosen alias must not shadow the injected per-row handle.
//
// Its own refusal type, not the vocabulary one the other plan names get: an
// alias is OPEN — the caller invents it and every name but this one is accepted
// — so a message quoting the report's measures would state a rule that does not
// exist, and send a caller to pick from a list that was never the constraint.
func TestAggregateAliasCannotSquatOnDerivationURL(t *testing.T) {
	_, _, err := buildSelectList(prebuiltReports["forecast"],
		[]string{"owner_id"},
		[]reportAggregate{{Fn: "count", As: reservedDerivationColumn}})
	var reserved *ReservedAliasError
	if !errors.As(err, &reserved) || reserved.Alias != reservedDerivationColumn {
		t.Fatalf("alias %q → %v, want ReservedAliasError on that alias", reservedDerivationColumn, err)
	}
	_, message := reserved.MessageFault()
	for _, measure := range allowedReportNames(prebuiltReports["forecast"].measures) {
		if strings.Contains(message, measure) {
			t.Errorf("the refusal offers %q as an alias, but any alias but the reserved one is accepted: %s",
				measure, message)
		}
	}
	// Every other alias still passes — the refusal is about ONE name.
	if _, _, err := buildSelectList(prebuiltReports["forecast"], []string{"owner_id"},
		[]reportAggregate{{Fn: "count", As: "my_own_label"}}); err != nil {
		t.Errorf("a free-form alias was refused: %v", err)
	}
}

// The forecast is a parameterized report over the shared engine
// (B-E09.10): its weighted measure must be the ENGINE's, with its per-deal
// rounding, and its plan must aggregate the deal table alone — the
// stage join is a to-one lookup, never a row multiplier.
//
// The measure is compared against weightedAmountMinorExpr rather than against
// a copy of it. A copy here would be a second spelling of the arithmetic, in
// the one place a reader trusts to be authoritative about it; what the
// expression COMPUTES is held against the Go spelling of the same figure by
// TestTheTwoSpellingsOfWeightedValueAgree.
func TestForecastSpecShape(t *testing.T) {
	spec, ok := prebuiltReports["forecast"]
	if !ok {
		t.Fatal("forecast report missing from the prebuilt catalog")
	}
	// One join, a to-one lookup for win_probability, never a row multiplier.
	// The workspace join went with the §11 reporting-zone "today": that zone
	// is an installation SETTING now, bound as a parameter, so the join had
	// nothing left to carry.
	if got := spec.fromClause(); got != "deal t JOIN stage s ON s.id = t.stage_id" {
		t.Errorf("fromClause = %q", got)
	}
	if spec.measures["weighted_amount_minor"] != weightedAmountMinorExpr {
		t.Errorf("weighted measure = %q, want the engine's own %q",
			spec.measures["weighted_amount_minor"], weightedAmountMinorExpr)
	}
	if spec.measures["amount_minor"] != "t.amount_minor" {
		t.Errorf("unweighted measure = %q", spec.measures["amount_minor"])
	}
}

// A handle carries the instant its headline was computed at, and gives it back.
//
// This is the half a database test cannot show: the FX lookup compares by DAY,
// so two reads taken minutes apart see the same rate however the sheet moved.
// The case that matters crosses a date boundary — a stage totalled at 23:50 and
// opened at 00:10 — and stating both instants exactly is what makes it testable
// at all.
func TestADerivationHandleCarriesTheInstantItWasComputedAt(t *testing.T) {
	lateThursday := time.Date(2026, 9, 3, 23, 50, 0, 0, time.UTC)

	minted := derivationURL("pipeline-current", nil, []string{"stage_id"},
		[]reportAggregate{{Fn: "sum", Field: "amount_base_minor", As: "base"}},
		map[string]any{"stage_id": "s1", "base": int64(5000)}, lateThursday)

	parsed, err := url.Parse(minted)
	if err != nil {
		t.Fatalf("the minted handle is not a URL: %v", err)
	}
	q, err := parseDerivationQuery(parsed.Query())
	if err != nil {
		t.Fatalf("the engine cannot read back a handle it minted: %v", err)
	}
	if !q.AsOf.Equal(lateThursday) {
		t.Fatalf("handle round-tripped as_of as %v, want %v.\n\n"+
			"Opened after midnight, a handle that lost this converts at Friday's "+
			"rate while the headline above it was Thursday's.", q.AsOf, lateThursday)
	}
}

// A handle minted before this key existed still resolves, at the current
// instant, exactly as it always did. Those links are in bookmarks and in sent
// mail, and refusing them would be a worse answer than the old behaviour.
func TestAHandleWithoutAnInstantStillResolves(t *testing.T) {
	q, err := parseDerivationQuery(url.Values{"by": {"stage_id"}, "stage_id": {"s1"}})
	if err != nil {
		t.Fatalf("a handle with no as_of was refused: %v", err)
	}
	if !q.AsOf.IsZero() {
		t.Errorf("a handle naming no instant produced %v, want the zero time that "+
			"tells the engine to read the frame's own", q.AsOf)
	}
}
