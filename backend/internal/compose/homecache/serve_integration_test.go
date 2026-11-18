// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package homecache_test

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"testing"

	"github.com/margince/margince/backend/internal/compose/homecache"
	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// logRecorder keeps the level and message of every record logged.
type logRecorder struct {
	mu      sync.Mutex
	records []slog.Record
}

func (r *logRecorder) Enabled(context.Context, slog.Level) bool { return true }
func (r *logRecorder) WithAttrs([]slog.Attr) slog.Handler       { return r }
func (r *logRecorder) WithGroup(string) slog.Handler            { return r }

func (r *logRecorder) Handle(_ context.Context, record slog.Record) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.records = append(r.records, record)
	return nil
}

// logged counts the records at level whose message starts with prefix.
func (r *logRecorder) logged(level slog.Level, prefix string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, record := range r.records {
		if record.Level == level && strings.HasPrefix(record.Message, prefix) {
			n++
		}
	}
	return n
}

func recordLogs(t *testing.T) *logRecorder {
	t.Helper()
	recorder := &logRecorder{}
	previous := slog.Default()
	slog.SetDefault(slog.New(recorder))
	t.Cleanup(func() { slog.SetDefault(previous) })
	return recorder
}

func serveWorklist(ctx context.Context, c *homecache.Cache, reader ids.UUID) error {
	_, _, err := homecache.Serve(ctx, c, reader, homecache.Worklist, "home",
		func(context.Context) (crmcontracts.Worklist, error) { return crmcontracts.Worklist{}, nil },
		func(crmcontracts.Worklist) (*ids.UUID, error) {
			return nil, nil //nolint:nilnil // an empty Worklist starts no walk, which Serve reads as nil
		})
	return err
}

func TestAReaderWhoLeftIsLoggedAtDebugAndACacheFaultAtError(t *testing.T) {
	e := integration.Setup(t)
	logs := recordLogs(t)
	c := homecache.New(e.Pool)
	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.RepPerms)

	left, leave := context.WithCancel(ctx)
	leave()
	if err := serveWorklist(left, c, e.Rep1); err != nil {
		t.Fatalf("serving a reader who left: %v", err)
	}
	c.Settle()
	if n := logs.logged(slog.LevelError, ""); n != 0 {
		t.Errorf("a reader who left produced %d ERROR records, want none", n)
	}
	if logs.logged(slog.LevelDebug, "a kept Home answer could not be read") == 0 {
		t.Error("a reader who left produced no DEBUG record of the read it abandoned")
	}

	stamp, err := c.Stamp(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := homecache.Store(ctx, c, e.Rep1, homecache.Worklist, "home", stamp, nil, crmcontracts.Worklist{}); err != nil {
		t.Fatal(err)
	}
	e.WsExec(t, `UPDATE home_read_cache SET payload = '{"view": 5}'::jsonb WHERE reader_id = $1`, e.Rep1)
	before := logs.logged(slog.LevelError, "a kept Home answer could not be read")
	if err := serveWorklist(ctx, c, e.Rep1); err != nil {
		t.Fatalf("serving past an unreadable answer: %v", err)
	}
	c.Settle()
	if logs.logged(slog.LevelError, "a kept Home answer could not be read") != before+1 {
		t.Error("an answer the cache cannot decode was not logged as an ERROR")
	}
}
