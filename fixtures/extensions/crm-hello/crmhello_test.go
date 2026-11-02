// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package crmhello

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/margince/margince/backend/pkg/extension"
)

func TestNewDeclaresTheFixtureUnit(t *testing.T) {
	e := New()
	if e.Name != "crm-hello" {
		t.Fatalf("Name = %q, want the unit's directory name crm-hello", e.Name)
	}
	if e.Version == "" {
		t.Fatal("Version is empty — the boot inventory records it")
	}
	if len(e.Jurisdictions) != 1 {
		t.Fatalf("declaration carries %d jurisdiction packs, want 1", len(e.Jurisdictions))
	}
	if got := e.Jurisdictions[0].Code(); got != "zz" {
		t.Fatalf("pack code = %q, want the user-assigned fixture code zz", got)
	}
	if ret := e.Jurisdictions[0].Retention(); ret != nil {
		t.Fatalf("fixture pack carries retention classes %v, want none", ret.Classes())
	}
}

func TestTheFixtureDeclaresAValidDealPanel(t *testing.T) {
	e := New()
	if len(e.Panels) != 1 || e.Panels[0].Slot != extension.SlotDealPanel {
		t.Fatalf("panels = %+v, want one on the deal page", e.Panels)
	}
	if err := e.Panels[0].Validate(e.Name); err != nil {
		t.Fatalf("panel: %v", err)
	}
}

// dealReader answers one scripted Deals().Get and records what it was asked.
type dealReader struct {
	extension.Deals
	asked  []string
	answer extension.DealSummary
	err    error
}

func (d *dealReader) Get(_ context.Context, id string) (extension.DealSummary, error) {
	d.asked = append(d.asked, id)
	return d.answer, d.err
}

// readerRuntime is a Runtime whose only port is Deals; any other call panics.
type readerRuntime struct {
	extension.Runtime
	deals *dealReader
}

func (r readerRuntime) Deals() extension.Deals { return r.deals }

func TestTheFixtureDeclaresTheDealReadItsSubscriptionMakes(t *testing.T) {
	e := New()
	if len(e.Subscriptions) != 1 || len(e.SystemReads) != 1 || e.SystemReads[0] != extension.SystemReadDeals {
		t.Fatalf("subscriptions = %d, system reads = %v, want one subscription reading deals",
			len(e.Subscriptions), e.SystemReads)
	}
}

func TestADealChangeIsNotedWithItsStage(t *testing.T) {
	var logged bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logged, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })

	deals := &dealReader{answer: extension.DealSummary{ID: "d-1", StageName: "Qualified"}}
	delivery := extension.Delivery{Type: "deal.updated", Entity: extension.EntityRef{Type: "deal", ID: "d-1"}}
	if err := New().Subscriptions[0].Handle(context.Background(), readerRuntime{deals: deals}, delivery); err != nil {
		t.Fatal(err)
	}
	if len(deals.asked) != 1 || deals.asked[0] != "d-1" {
		t.Errorf("the handler read %v, want the delivered deal d-1", deals.asked)
	}
	if !strings.Contains(logged.String(), "stage=Qualified") {
		t.Errorf("the log does not name the stage it read:\n%s", logged.String())
	}
}

func TestAGoneDealIsHandledAndAFailedReadIsRetried(t *testing.T) {
	handle := New().Subscriptions[0].Handle
	delivery := extension.Delivery{Entity: extension.EntityRef{Type: "deal", ID: "d-1"}}
	gone := readerRuntime{deals: &dealReader{err: extension.ErrNotFound}}
	if err := handle(context.Background(), gone, delivery); err != nil {
		t.Errorf("a deleted deal: err = %v, want it handled", err)
	}
	failed := readerRuntime{deals: &dealReader{err: errors.New("the core could not answer")}}
	if err := handle(context.Background(), failed, delivery); err == nil {
		t.Error("a failed read was marked handled, so the bus never delivers it again")
	}
}

func TestTheFixtureDeclaresAValidInstrumentation(t *testing.T) {
	in := New().Instrumentation
	if in == nil {
		t.Fatal("Instrumentation is nil — the census needs a live declaring unit")
	}
	if err := in.Validate(); err != nil {
		t.Fatalf("Validate() = %v, want nil", err)
	}
	if in.Browser != "instrument" {
		t.Fatalf("Browser = %q, want the frontend export instrument", in.Browser)
	}
}

func TestTheInstrumentsWorkHookRunsTheWorkOnceAndReturnsItsError(t *testing.T) {
	proc := extension.Process{
		Role:       "api",
		Logger:     slog.New(slog.DiscardHandler),
		RedactPath: func(s string) string { return s },
	}
	ins, err := New().Instrumentation.Start(context.Background(), proc)
	if err != nil {
		t.Fatalf("Start() = %v, want nil", err)
	}
	if ins.Work == nil {
		t.Fatal("Start returned no Work hook")
	}
	want := errors.New("the job failed")
	calls := 0
	before := workRuns.Load()
	got := ins.Work(context.Background(), extension.Work{Kind: extension.WorkJob, Name: "x"}, func(context.Context) error {
		calls++
		return want
	})
	if calls != 1 {
		t.Fatalf("run called %d times, want exactly once", calls)
	}
	if !errors.Is(got, want) {
		t.Fatalf("Work returned %v, want the run's error unchanged", got)
	}
	if runs := workRuns.Load() - before; runs != 1 {
		t.Fatalf("the Work hook counted %d runs, want 1", runs)
	}
}

func TestTheInstrumentationEdgeAnswersGetPingAndRefusesPost(t *testing.T) {
	edge := New().Instrumentation.Edge
	for method, want := range map[string]int{http.MethodGet: http.StatusNoContent, http.MethodPost: http.StatusMethodNotAllowed} {
		rec := httptest.NewRecorder()
		edge.ServeHTTP(rec, httptest.NewRequest(method, "/ping", nil))
		if rec.Code != want {
			t.Errorf("%s /ping = %d, want %d", method, rec.Code, want)
		}
	}
}
