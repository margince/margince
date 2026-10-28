// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package database

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/jackc/pgx/v5"
)

// SQLCensusDirEnv names a directory; when set, every pool this package builds
// records each distinct statement it sends, with the Go call chain that sent it.
const SQLCensusDirEnv = "MARGINCE_SQL_CENSUS_DIR"

type censusRecord struct {
	Hash   string   `json:"hash"`
	SQL    string   `json:"sql"`
	Frames []string `json:"frames"`
}

// sqlCensus writes one JSON line per (statement, sending function) pair it has
// not seen before in this process.
type sqlCensus struct {
	mu   sync.Mutex
	seen map[string]bool
	out  *os.File
}

var (
	censusOnce sync.Once
	census     *sqlCensus
	censusErr  error
)

// processCensus is shared by every pool in the process, so one file and one
// dedupe map serve them all.
func processCensus(dir string) (*sqlCensus, error) {
	censusOnce.Do(func() { census, censusErr = newSQLCensus(dir) })
	return census, censusErr
}

func newSQLCensus(dir string) (*sqlCensus, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("sql census: %w", err)
	}
	name := filepath.Join(dir, fmt.Sprintf("census-%d.jsonl", os.Getpid()))
	out, err := os.OpenFile(name, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("sql census: %w", err)
	}
	return &sqlCensus{seen: map[string]bool{}, out: out}, nil
}

func (c *sqlCensus) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	c.record(data.SQL)
	return ctx
}

func (c *sqlCensus) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

func (c *sqlCensus) TraceBatchStart(ctx context.Context, _ *pgx.Conn, _ pgx.TraceBatchStartData) context.Context {
	return ctx
}

func (c *sqlCensus) TraceBatchQuery(_ context.Context, _ *pgx.Conn, data pgx.TraceBatchQueryData) {
	c.record(data.SQL)
}

func (c *sqlCensus) TraceBatchEnd(context.Context, *pgx.Conn, pgx.TraceBatchEndData) {}

func (c *sqlCensus) record(sql string) {
	frames := productFrames()
	sum := sha256.Sum256([]byte(sql))
	hash := hex.EncodeToString(sum[:8])
	key := hash
	if len(frames) > 0 {
		key += frames[0]
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.seen[key] {
		return
	}
	c.seen[key] = true
	line, err := json.Marshal(censusRecord{Hash: hash, SQL: sql, Frames: frames})
	if err != nil {
		return
	}
	if _, err := c.out.Write(append(line, '\n')); err != nil {
		fmt.Fprintf(os.Stderr, "sql census: %v\n", err)
	}
}

// productFrames is the call chain inside this repository, nearest first, with
// the pool plumbing and pgx itself left out.
func productFrames() []string {
	pcs := make([]uintptr, 64)
	n := runtime.Callers(4, pcs)
	iter := runtime.CallersFrames(pcs[:n])
	var frames []string
	for {
		f, more := iter.Next()
		i := strings.Index(f.File, "/backend/")
		if i >= 0 && !strings.Contains(f.File, "/platform/database/") && !strings.Contains(f.File, "/platform/testdb/") {
			frames = append(frames, fmt.Sprintf("%s:%d %s", f.File[i+1:], f.Line, f.Function[strings.LastIndex(f.Function, "/")+1:]))
		}
		if !more || len(frames) == 24 {
			return frames
		}
	}
}
