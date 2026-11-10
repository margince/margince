// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package httpserver_test

import (
	"bytes"
	"context"
	"log/slog"
	"testing"

	"github.com/margince/margince/backend/internal/platform/httpserver"
	"github.com/margince/margince/backend/internal/platform/instrument"
	"github.com/margince/margince/backend/pkg/extension"
)

type countingHandler struct {
	slog.Handler
	n *int
}

func (c countingHandler) Handle(ctx context.Context, r slog.Record) error {
	*c.n++
	return c.Handler.Handle(ctx, r)
}

func TestInstallProcessLoggerPassesRecordsThroughTheInstalledInstruments(t *testing.T) {
	t.Cleanup(instrument.Reset)
	prev := slog.Default()
	t.Cleanup(func() { slog.SetDefault(prev) })
	seen := 0
	instrument.Install([]instrument.Started{{Unit: "u", Hooks: extension.Instruments{
		LogHandler: func(h slog.Handler) slog.Handler { return countingHandler{h, &seen} },
	}}}, slog.New(slog.DiscardHandler))
	var out bytes.Buffer
	logger, err := httpserver.InstallProcessLogger(&out, "info", "json")
	if err != nil {
		t.Fatal(err)
	}
	logger.Error("boom")
	if seen != 1 || out.Len() == 0 {
		t.Fatalf("seen=%d out=%q: the record must reach both the instrument and the sink", seen, out.String())
	}
}
