// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"reflect"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// newTestLinkResolver answers every database step "not found", so a test sets
// only the step it is about.
func newTestLinkResolver(t *testing.T) *linkResolver {
	t.Helper()
	r := &linkResolver{
		pending: map[linkKey]bool{}, landed: map[linkKey]ids.UUID{}, cache: map[linkKey]linkAnswer{},
		names: map[string]pendingName{},
	}
	none := func(context.Context, string, string) (linkAnswer, error) { return linkAnswer{}, nil }
	r.byBinding, r.byNaturalKey, r.byID = none, none, none
	return r
}

func TestALinkTriesThisUploadBeforeAnythingInTheCRM(t *testing.T) {
	r := newTestLinkResolver(t)
	r.markPending("company", "CO-1")
	r.byNaturalKey = func(context.Context, string, string) (linkAnswer, error) {
		t.Fatal("a ref in this upload must answer before the natural key is read")
		return linkAnswer{}, nil
	}
	got, err := r.resolveCompany(context.Background(), "  CO-1 ")
	if err != nil || !got.Pending {
		t.Fatalf("got %+v err %v, want pending", got, err)
	}
}

func TestARefKeepsItsCase(t *testing.T) {
	r := newTestLinkResolver(t)
	r.markPending("company", "CO-1")
	if got, _ := r.resolveCompany(context.Background(), "co-1"); got.Pending || got.Found {
		t.Fatalf("co-1 matched CO-1: a ref is an identifier, not a word")
	}
}

// A row of this run that named its record by id answers its ref with that
// record, before the identity map is asked.
func TestARowNamedByIDAnswersItsRef(t *testing.T) {
	r := newTestLinkResolver(t)
	r.byBinding = func(context.Context, string, string) (linkAnswer, error) {
		t.Fatal("a ref this run landed must answer before the identity map is read")
		return linkAnswer{}, nil
	}
	id := ids.NewV7()
	r.markLanded("company", " CO-7", id)
	if got, err := r.resolveCompany(context.Background(), "CO-7"); err != nil || !got.Found || got.ID != id {
		t.Fatalf("got %+v err %v, want the record the row named", got, err)
	}
}

// Only a value that parses as an id is looked up as one. A miss on all three
// steps answers one sentence naming the value.
func TestOnlyAnIDIsLookedUpByIDAndAMissNamesTheValue(t *testing.T) {
	r := newTestLinkResolver(t)
	var byID []string
	r.byID = func(_ context.Context, _ string, key string) (linkAnswer, error) {
		byID = append(byID, key)
		return linkAnswer{}, nil
	}
	id := ids.NewV7().String()
	for _, raw := range []string{"Acme GmbH", id} {
		got, err := r.resolveCompany(context.Background(), raw)
		if err != nil || got.Found || got.Pending {
			t.Fatalf("resolve(%q) = %+v err %v, want not found", raw, got, err)
		}
		if !reflect.DeepEqual(got.Why, linkNotFoundWhy(raw)) {
			t.Fatalf("reason = %q, want the one not-found sentence naming %q", got.Why.English(), raw)
		}
	}
	if len(byID) != 1 || byID[0] != id {
		t.Fatalf("looked up by id: %v, want only the uuid", byID)
	}
}

// A ref the dry run asked about answers from the cache on the next row, and a
// later markPending replaces a cached miss.
func TestAResolvedRefIsAskedOnceUntilItIsMarkedPending(t *testing.T) {
	r := newTestLinkResolver(t)
	calls := 0
	r.byBinding = func(context.Context, string, string) (linkAnswer, error) {
		calls++
		return linkAnswer{}, nil
	}
	for range 2 {
		if _, err := r.resolveCompany(context.Background(), "CO-2"); err != nil {
			t.Fatal(err)
		}
	}
	if calls != 1 {
		t.Fatalf("binding asked %d times, want once", calls)
	}
	r.markPending("company", "CO-2")
	if got, _ := r.resolveCompany(context.Background(), "CO-2"); !got.Pending {
		t.Fatalf("got %+v after markPending, want pending", got)
	}
}

// The run's broken-link policy decides a row whose company_ref finds nothing.
// skip_row (and an unset policy) leaves it out; drop_link lands it with a warning.
func TestABrokenCompanyRefSkipsOrWarnsByPolicy(t *testing.T) {
	row := migration.Row{ExternalID: "P-1", Line: 2, Fields: map[string]any{csvTargetCompanyRef: "CO-404"}}
	for policy, want := range map[string]struct {
		skip bool
		kind string
	}{
		"": {true, migration.ProblemBrokenLink},
		string(crmcontracts.ImportOnBrokenLinkSkipRow):  {true, migration.ProblemBrokenLink},
		string(crmcontracts.ImportOnBrokenLinkDropLink): {false, migration.ProblemDroppedLink},
	} {
		w := &csvWriters{links: newTestLinkResolver(t), companyRefColumn: "company", onBrokenLink: policy}
		got, err := w.linkProblemOf(context.Background(), row)
		if err != nil || got.problem == nil {
			t.Fatalf("%q: got %+v err %v, want a problem", policy, got, err)
		}
		if got.skip != want.skip || got.problem.Kind != want.kind || got.problem.Value != "CO-404" || got.problem.Column != "company" {
			t.Fatalf("%q: got skip=%v %+v, want skip=%v kind %s naming CO-404 in column company", policy, got.skip, *got.problem, want.skip, want.kind)
		}
		if !strings.Contains(got.problem.Reason, `“CO-404”`) {
			t.Fatalf("%q: reason %q does not name the value", policy, got.problem.Reason)
		}
	}
}

// A ref this upload or the CRM answers to is no problem at all.
func TestACompanyRefThatResolvesIsNoProblem(t *testing.T) {
	r := newTestLinkResolver(t)
	r.markPending(migration.ObjectCompany, "CO-1")
	w := &csvWriters{links: r, companyRefColumn: "company"}
	got, err := w.linkProblemOf(context.Background(), migration.Row{Fields: map[string]any{csvTargetCompanyRef: "CO-1"}})
	if err != nil || got.skip || got.problem != nil {
		t.Fatalf("got %+v err %v, want no problem", got, err)
	}
}

// A name only a company of this upload wears is pending. A created company
// sharing a name with one in the estate makes the name ambiguous, as at commit.
// An updated one stays the name's one holder.
func TestACompanyNameOfThisUploadCountsAsOneMoreHolder(t *testing.T) {
	estate := employerCandidate{id: ids.From[ids.CompanyKind](ids.NewV7()), count: 1}
	r := newTestLinkResolver(t)
	r.markNamesPending(true, "New  Co", "new co")
	r.markNamesPending(true, "Twin AG")
	r.markNamesPending(false, "Kept Ltd")
	for _, tc := range []struct {
		name string
		hit  employerCandidate
		want linkAnswer
	}{
		{"new co", employerCandidate{}, linkAnswer{Pending: true}},
		{"twin ag", estate, linkAnswer{Why: ambiguousEmployerWhy("twin ag")}},
		{"kept ltd", estate, linkAnswer{Pending: true}},
		{"elsewhere", estate, linkAnswer{ID: estate.id.UUID, Found: true}},
	} {
		if got := employerAnswer(r.names[tc.name].over(tc.hit), tc.name); !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("%s: got %+v, want %+v", tc.name, got, tc.want)
		}
	}
}
