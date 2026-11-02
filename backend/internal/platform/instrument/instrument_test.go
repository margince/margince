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
	_ = instrument.Work(context.Background(), extension.Work{}, func(context.Context) error { return nil })
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
	instrument.Install([]instrument.Started{
		{Unit: "bad", Hooks: extension.Instruments{
			Panic:    func(context.Context, any, []byte) { panic("hook") },
			Shutdown: func(context.Context) error { panic("hook") },
		}},
		{Unit: "good", Hooks: extension.Instruments{
			Panic:    func(_ context.Context, v any, _ []byte) { seen = append(seen, "panic:"+v.(string)) },
			Shutdown: func(context.Context) error { seen = append(seen, "shutdown"); return nil },
		}},
	}, slog.New(slog.DiscardHandler))
	instrument.Panic(context.Background(), "x", nil)
	instrument.Shutdown(context.Background())
	if strings.Join(seen, ",") != "panic:x,shutdown" {
		t.Fatalf("seen = %v", seen)
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
