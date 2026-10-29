// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package database

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash/maphash"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

// SQLCensusDirEnv names a directory. An integration build that sets it has
// every pool record each distinct statement, with the call chain that sent it.
const SQLCensusDirEnv = "MARGINCE_SQL_CENSUS_DIR"

// censusDir is SQLCensusDirEnv as the integration build read it. A product
// build never sets it, so no product pool records.
var censusDir string

// CensusMinBytes skips the statements too small to plan expensively. Over the
// integration lane, every statement that grew a backend by 64 MB was over 18 KB,
// and the median statement is under 200 bytes. One activity content gate
// renders larger than this, so the census sees every statement that holds one.
const CensusMinBytes = 8 << 10

type censusRecord struct {
	Hash   string   `json:"hash"`
	SQL    string   `json:"sql"`
	Frames []string `json:"frames"`
}

// sqlCensus writes one JSON line per (statement, sending function) pair it has
// not seen before in this process.
type sqlCensus struct {
	mu   sync.Mutex
	seed maphash.Seed
	seen map[censusKey]bool
	out  *os.File
}

// censusKey is a statement and the first frames that sent it, cheap to build on
// every call.
type censusKey struct {
	sql uint64
	pcs [6]uintptr
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
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("sql census: %w", err)
	}
	name := filepath.Join(dir, fmt.Sprintf("census-%d.jsonl", os.Getpid()))
	out, err := os.OpenFile(name, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600) //nolint:gosec // a file of this process, in the directory the integration lane names
	if err != nil {
		return nil, fmt.Errorf("sql census: %w", err)
	}
	return &sqlCensus{seed: maphash.MakeSeed(), seen: map[censusKey]bool{}, out: out}, nil
}

// record runs inside a statementTracer hook, so three frames up is the pgx call
// the product made. A nil census records nothing.
func (c *sqlCensus) record(sql string) {
	if c == nil || len(sql) < CensusMinBytes {
		return
	}
	var pcs [64]uintptr
	n := runtime.Callers(3, pcs[:])
	// Raw program counters and an in-memory hash key the pair. Resolving frames
	// and a stable hash cost far more, so only a new pair pays for them.
	key := censusKey{sql: maphash.String(c.seed, sql)}
	copy(key.pcs[:], pcs[:n])
	c.mu.Lock()
	seen := c.seen[key]
	c.seen[key] = true
	c.mu.Unlock()
	if seen {
		return
	}
	sum := sha256.Sum256([]byte(sql))
	line, err := json.Marshal(censusRecord{Hash: hex.EncodeToString(sum[:8]), SQL: sql, Frames: productFrames(pcs[:n])})
	if err != nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, err := c.out.Write(append(line, '\n')); err != nil {
		fmt.Fprintf(os.Stderr, "sql census: %v\n", err)
	}
}

// productFrames is the call chain inside this repository, nearest first, with
// the pool plumbing and pgx itself left out.
func productFrames(pcs []uintptr) []string {
	iter := runtime.CallersFrames(pcs)
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
