// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package crmhello

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
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

func TestTheHooksRefuseOnlyTheirSentinels(t *testing.T) {
	e := New()
	write, operation := e.WriteHooks[0].Handle, e.OperationHooks[0].Handle
	if err := write(context.Background(), nil, extension.Write{After: []byte(`{"name":"Fleet"}`)}); err != nil {
		t.Fatalf("an ordinary deal was refused: %v", err)
	}
	if err := write(context.Background(), nil, extension.Write{After: []byte(`{"name":"` + sentinelWrite + `"}`)}); !errors.Is(err, extension.ErrConflict) {
		t.Fatalf("err = %v, want the sentinel write refused as a conflict", err)
	}
	if err := operation(context.Background(), nil, &extension.Operation{Body: []byte(`{"name":"Fleet"}`)}); err != nil {
		t.Fatalf("an ordinary call was refused: %v", err)
	}
	if err := operation(context.Background(), nil, &extension.Operation{Body: []byte(`{"name":"` + sentinelOperation + `"}`)}); !errors.Is(err, extension.ErrInvalid) {
		t.Fatalf("err = %v, want the sentinel call refused as invalid", err)
	}
}
