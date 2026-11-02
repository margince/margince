// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"log/slog"
	"testing"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/instrument"
	kevents "github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/pkg/extension"
)

// The delivery runs over a handle that names its workspace without a pool, so
// the whole closure is reachable with no database.
func instrumentedDelivery(t *testing.T, handle func(context.Context, extension.Runtime, extension.Delivery) error, hooks extension.Instruments) error {
	t.Helper()
	t.Cleanup(instrument.Reset)
	instrument.Install([]instrument.Started{{Unit: "alpha", Hooks: hooks}}, slog.New(slog.DiscardHandler))
	sub := ComposedSubscription{
		Unit: "alpha", Version: "1.0.0",
		Sub: extension.Subscription{Name: "on_deal", Events: []string{"activity.captured"}, Handle: handle},
	}
	return sub.handlerOver(database.BindTo(nil, ids.From[ids.WorkspaceKind](ids.NewV7())), slog.New(slog.DiscardHandler))(context.Background(), kevents.Envelope{
		EventID: ids.NewV7(), Type: "activity.captured",
		Entity: kevents.EntityRef{Type: "activity", ID: ids.NewV7()},
	})
}

func TestExtensionSubscriptionDeliveryRunsInsideTheWorkInstruments(t *testing.T) {
	var got extension.Work
	var ran bool
	err := instrumentedDelivery(t,
		func(context.Context, extension.Runtime, extension.Delivery) error { ran = true; return nil },
		extension.Instruments{Work: func(ctx context.Context, w extension.Work, run func(context.Context) error) error {
			got = w
			return run(ctx)
		}})
	if err != nil {
		t.Fatal(err)
	}
	if !ran {
		t.Fatal("the handler did not run")
	}
	want := extension.Work{Kind: extension.WorkSubscription, Name: "on_deal", Unit: "alpha"}
	if got != want {
		t.Fatalf("work = %+v, want %+v", got, want)
	}
}

func TestExtensionSubscriptionPanicReachesTheInstruments(t *testing.T) {
	var reported any
	err := instrumentedDelivery(t,
		func(context.Context, extension.Runtime, extension.Delivery) error { panic("delivery blew up") },
		extension.Instruments{Panic: func(_ context.Context, v any, _ []byte) { reported = v }})
	if err == nil {
		t.Fatal("a panicking delivery was not turned into an error")
	}
	if reported != "delivery blew up" {
		t.Fatalf("reported = %v", reported)
	}
}
