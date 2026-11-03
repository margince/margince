// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package instrument_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/instrument"
	"github.com/margince/margince/backend/pkg/extension"
)

func tracing(trail *[]string, name string) extension.Instruments {
	return extension.Instruments{
		HTTP: func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				*trail = append(*trail, name+">")
				next.ServeHTTP(w, r)
				*trail = append(*trail, "<"+name)
			})
		},
		Work: func(ctx context.Context, _ extension.Work, run func(context.Context) error) error {
			*trail = append(*trail, name+">")
			err := run(ctx)
			*trail = append(*trail, "<"+name)
			return err
		},
	}
}

func TestWithNothingInstalledEveryHookIsTheIdentity(t *testing.T) {
	t.Cleanup(instrument.Reset)
	instrument.Reset()
	h := http.NotFoundHandler()
	rec := httptest.NewRecorder()
	instrument.HTTP(h).ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d", rec.Code)
	}
	want := errors.New("boom")
	if got := instrument.Work(context.Background(), extension.Work{}, func(context.Context) error { return want }); got != want {
		t.Fatalf("Work = %v, want %v", got, want)
	}
}

func TestTheFirstUnitIsTheOutermostWrapper(t *testing.T) {
	t.Cleanup(instrument.Reset)
	var trail []string
	instrument.Install([]instrument.Started{
		{Unit: "a", Hooks: tracing(&trail, "a")},
		{Unit: "b", Hooks: tracing(&trail, "b")},
	}, slog.New(slog.DiscardHandler))
	instrument.HTTP(http.NotFoundHandler()).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))
	if err := instrument.Work(context.Background(), extension.Work{}, func(context.Context) error { return nil }); err != nil {
		t.Fatalf("Work = %v", err)
	}
	got := strings.Join(trail, " ")
	if got != "a> b> <b <a a> b> <b <a" {
		t.Fatalf("trail = %q", got)
	}
}

func TestAPanickingWorkHookStillRunsTheWorkOnce(t *testing.T) {
	t.Cleanup(instrument.Reset)
	var logged bytes.Buffer
	instrument.Install([]instrument.Started{{Unit: "bad", Hooks: extension.Instruments{
		Work: func(context.Context, extension.Work, func(context.Context) error) error { panic("hook") },
	}}}, slog.New(slog.NewTextHandler(&logged, nil)))
	runs := 0
	err := instrument.Work(context.Background(), extension.Work{}, func(context.Context) error { runs++; return nil })
	if err != nil || runs != 1 {
		t.Fatalf("err=%v runs=%d, want nil and 1", err, runs)
	}
	if !strings.Contains(logged.String(), "instrumentation hook panicked") || !strings.Contains(logged.String(), "unit=bad") {
		t.Fatalf("log = %q", logged.String())
	}
}

func TestAPanickingHTTPHookIsSkippedNotPropagated(t *testing.T) {
	t.Cleanup(instrument.Reset)
	instrument.Install([]instrument.Started{{Unit: "bad", Hooks: extension.Instruments{
		HTTP: func(http.Handler) http.Handler { panic("hook") },
	}}}, slog.New(slog.DiscardHandler))
	rec := httptest.NewRecorder()
	instrument.HTTP(http.NotFoundHandler()).ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestPanicAndShutdownReachEveryUnitAndSurviveAPanickingOne(t *testing.T) {
	t.Cleanup(instrument.Reset)
	var seen []string
	var logged bytes.Buffer
	instrument.Install([]instrument.Started{
		{Unit: "bad", Hooks: extension.Instruments{
			Panic:    func(context.Context, any, []byte) { panic("hook") },
			Shutdown: func(context.Context) error { panic("hook") },
		}},
		{Unit: "good", Hooks: extension.Instruments{
			Panic:    func(_ context.Context, v any, _ []byte) { seen = append(seen, "panic:"+v.(string)) },
			Shutdown: func(context.Context) error { seen = append(seen, "shutdown"); return nil },
		}},
	}, slog.New(slog.NewTextHandler(&logged, nil)))
	instrument.Panic(context.Background(), "x", nil)
	instrument.Shutdown(context.Background())
	if strings.Join(seen, ",") != "panic:x,shutdown" {
		t.Fatalf("seen = %v", seen)
	}
	if n := strings.Count(logged.String(), "unit=bad"); n != 2 {
		t.Fatalf("bad unit failures logged %d times, want 2: %q", n, logged.String())
	}
}

func TestLogHandlerWrapsInOrder(t *testing.T) {
	t.Cleanup(instrument.Reset)
	var order []string
	wrap := func(name string) func(slog.Handler) slog.Handler {
		return func(h slog.Handler) slog.Handler { order = append(order, name); return h }
	}
	instrument.Install([]instrument.Started{
		{Unit: "a", Hooks: extension.Instruments{LogHandler: wrap("a")}},
		{Unit: "b", Hooks: extension.Instruments{LogHandler: wrap("b")}},
	}, slog.New(slog.DiscardHandler))
	instrument.LogHandler(slog.DiscardHandler)
	// Built inside-out: b wraps the base first, then a wraps b, so a is outermost.
	if strings.Join(order, ",") != "b,a" {
		t.Fatalf("order = %v", order)
	}
}

func passThrough(ctx context.Context, _ extension.Work, run func(context.Context) error) error {
	return run(ctx)
}

//craft:ignore naked-any the value is whatever recover() returns, which is any
func recovered(fn func()) (v any) {
	defer func() { v = recover() }()
	fn()
	return nil
}

func installWork(t *testing.T, log *slog.Logger, hooks ...func(context.Context, extension.Work, func(context.Context) error) error) {
	t.Helper()
	t.Cleanup(instrument.Reset)
	var units []instrument.Started
	for i, h := range hooks {
		units = append(units, instrument.Started{Unit: string(rune('a' + i)), Hooks: extension.Instruments{Work: h}})
	}
	instrument.Install(units, log)
}

func TestAWorkPanicReachesTheCallerThroughANonDeferringHook(t *testing.T) {
	var logged bytes.Buffer
	installWork(t, slog.New(slog.NewTextHandler(&logged, nil)), passThrough)
	got := recovered(func() {
		//craft:ignore swallowed-errors the work panics, so Work never returns a value
		_ = instrument.Work(context.Background(), extension.Work{}, func(context.Context) error { panic("work") })
	})
	if got != "work" {
		t.Fatalf("recovered %v, want the work's own panic", got)
	}
	if strings.Contains(logged.String(), "instrumentation hook panicked") {
		t.Fatalf("a work panic was logged as a hook panic: %q", logged.String())
	}
}

func TestAWorkPanicIsObservableByADeferringHookAndStillReachesTheCaller(t *testing.T) {
	var hookSaw any
	installWork(t, slog.New(slog.DiscardHandler), func(ctx context.Context, _ extension.Work, run func(context.Context) error) error {
		defer func() { hookSaw = recover(); panic(hookSaw) }()
		return run(ctx)
	})
	got := recovered(func() {
		//craft:ignore swallowed-errors the work panics, so Work never returns a value
		_ = instrument.Work(context.Background(), extension.Work{}, func(context.Context) error { panic("work") })
	})
	if hookSaw != "work" || got != "work" {
		t.Fatalf("hook saw %v, caller saw %v, want the work's value for both", hookSaw, got)
	}
}

func TestAWorkPanicThroughTwoNestedUnitsReachesTheCallerOnceWithItsOriginalValue(t *testing.T) {
	var logged bytes.Buffer
	installWork(t, slog.New(slog.NewTextHandler(&logged, nil)), passThrough, passThrough)
	runs := 0
	got := recovered(func() {
		//craft:ignore swallowed-errors the work panics, so Work never returns a value
		_ = instrument.Work(context.Background(), extension.Work{}, func(context.Context) error { runs++; panic("work") })
	})
	if got != "work" || runs != 1 {
		t.Fatalf("recovered %v after %d runs, want the original value after 1", got, runs)
	}
	if logged.Len() != 0 {
		t.Fatalf("nothing should be logged for a work panic: %q", logged.String())
	}
}

func TestAHookThatCallsRunTwiceRunsTheWorkOnce(t *testing.T) {
	installWork(t, slog.New(slog.DiscardHandler), func(ctx context.Context, _ extension.Work, run func(context.Context) error) error {
		//craft:ignore swallowed-errors the work under test returns nil; the hook only drives it
		_ = run(ctx)
		return run(ctx)
	})
	runs := 0
	if err := instrument.Work(context.Background(), extension.Work{}, func(context.Context) error { runs++; return nil }); err != nil {
		t.Fatalf("Work = %v", err)
	}
	if runs != 1 {
		t.Fatalf("runs = %d, want 1", runs)
	}
}

func TestAHookThatPanicsAfterRunReturnsTheWorksErrorAndIsLogged(t *testing.T) {
	var logged bytes.Buffer
	installWork(t, slog.New(slog.NewTextHandler(&logged, nil)), func(ctx context.Context, _ extension.Work, run func(context.Context) error) error {
		//craft:ignore swallowed-errors the work under test returns nil; the hook only drives it
		_ = run(ctx)
		panic("late hook")
	})
	want := errors.New("work failed")
	runs := 0
	err := instrument.Work(context.Background(), extension.Work{}, func(context.Context) error { runs++; return want })
	if err != want || runs != 1 {
		t.Fatalf("err=%v runs=%d, want the work's error and 1 run", err, runs)
	}
	if !strings.Contains(logged.String(), "instrumentation hook panicked") {
		t.Fatalf("hook panic not logged: %q", logged.String())
	}
}

func TestAHookThatReturnsWithoutCallingRunStillGetsTheWorkRunOnce(t *testing.T) {
	installWork(t, slog.New(slog.DiscardHandler), func(context.Context, extension.Work, func(context.Context) error) error { return nil })
	runs := 0
	if err := instrument.Work(context.Background(), extension.Work{}, func(context.Context) error { runs++; return nil }); err != nil {
		t.Fatalf("Work = %v", err)
	}
	if runs != 1 {
		t.Fatalf("runs = %d, want 1", runs)
	}
}

func TestALogHandlerHookThatPanicsOrReturnsNilLeavesTheInnerHandlerInUse(t *testing.T) {
	t.Cleanup(instrument.Reset)
	instrument.Install([]instrument.Started{
		{Unit: "panics", Hooks: extension.Instruments{LogHandler: func(slog.Handler) slog.Handler { panic("hook") }}},
		{Unit: "nils", Hooks: extension.Instruments{LogHandler: func(slog.Handler) slog.Handler { return nil }}},
	}, slog.New(slog.DiscardHandler))
	base := slog.NewTextHandler(&bytes.Buffer{}, nil)
	if got := instrument.LogHandler(base); got != slog.Handler(base) {
		t.Fatalf("handler = %v, want the inner handler", got)
	}
}

func TestInstallCopiesTheCallersSlice(t *testing.T) {
	t.Cleanup(instrument.Reset)
	var trail []string
	units := []instrument.Started{{Unit: "a", Hooks: tracing(&trail, "a")}}
	instrument.Install(units, slog.New(slog.DiscardHandler))
	units[0] = instrument.Started{Unit: "z"}
	if err := instrument.Work(context.Background(), extension.Work{}, func(context.Context) error { return nil }); err != nil {
		t.Fatalf("Work = %v", err)
	}
	if strings.Join(trail, " ") != "a> <a" {
		t.Fatalf("trail = %v, caller mutation leaked in", trail)
	}
}
