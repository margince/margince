// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/instrument"
	"github.com/margince/margince/backend/pkg/extension"
)

func TestStartInstrumentationSkipsAFailingOrPanickingUnitAndKeepsTheRest(t *testing.T) {
	t.Cleanup(instrument.Reset)
	var logged bytes.Buffer
	var gotProcess extension.Process
	shutdowns := 0
	exts := []extension.Extension{
		{Name: "fails", Instrumentation: &extension.Instrumentation{Start: func(context.Context, extension.Process) (extension.Instruments, error) {
			return extension.Instruments{}, errors.New("no dsn parse")
		}}},
		{Name: "panics", Instrumentation: &extension.Instrumentation{Start: func(context.Context, extension.Process) (extension.Instruments, error) {
			panic("start")
		}}},
		{Name: "browseronly", Instrumentation: &extension.Instrumentation{Browser: "instrument"}},
		{Name: "works", Instrumentation: &extension.Instrumentation{Start: func(_ context.Context, p extension.Process) (extension.Instruments, error) {
			gotProcess = p
			return extension.Instruments{Shutdown: func(context.Context) error { shutdowns++; return nil }}, nil
		}}},
	}
	startInstrumentation(context.Background(), exts, "api", slog.New(slog.NewTextHandler(&logged, nil)))
	instrument.Shutdown(context.Background())
	if shutdowns != 1 {
		t.Fatalf("shutdowns = %d, want 1 (only the working unit installed)", shutdowns)
	}
	if gotProcess.Role != "api" || gotProcess.RedactPath == nil || gotProcess.Logger == nil || gotProcess.Version == "" {
		t.Fatalf("process = %+v", gotProcess)
	}
	for _, want := range []string{"unit=fails", "unit=panics"} {
		if !strings.Contains(logged.String(), want) {
			t.Fatalf("log lacks %q: %s", want, logged.String())
		}
	}
}

func TestStartInstrumentationRedactsCapabilityPaths(t *testing.T) {
	t.Cleanup(instrument.Reset)
	var redact func(string) string
	startInstrumentation(context.Background(), []extension.Extension{{Name: "w", Instrumentation: &extension.Instrumentation{
		Start: func(_ context.Context, p extension.Process) (extension.Instruments, error) {
			redact = p.RedactPath
			return extension.Instruments{}, nil
		},
	}}}, "worker", slog.New(slog.DiscardHandler))
	if got := redact("/v1/public/confirm/secret-token"); strings.Contains(got, "secret-token") {
		t.Fatalf("RedactPath kept the credential: %q", got)
	}
}
