// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Command sql-census ranks the statements an integration run sent by what they
// cost Postgres to plan. A pooled backend keeps that memory until its
// connection closes. So a statement that plans large costs memory long after it
// returns, and a result test on small data cannot see it.
//
// It reads the JSONL files a run writes when MARGINCE_SQL_CENSUS_DIR is set,
// and plans each statement once on a fresh connection to a migrated database:
//
//	go run ./tools/sql-census -census DIR -dsn SUPERUSER_DSN > report.md
//
// The DSN must be a superuser's, because the peak memory is read from the
// backend's own /proc status through pg_read_file.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/jackc/pgx/v5"
)

type censusLine struct {
	Hash   string   `json:"hash"`
	SQL    string   `json:"sql"`
	Frames []string `json:"frames"`
}

type statement struct {
	hash   string
	sql    string
	chains [][]string
	planMs float64
	peakKB int
	err    string
}

var (
	explainable = regexp.MustCompile(`(?is)^\s*(--[^\n]*\n\s*)*(select|with|insert|update|delete|merge|values|table)\b`)
	vmHWM       = regexp.MustCompile(`VmHWM:\s+(\d+) kB`)
)

func main() {
	dir := flag.String("census", "", "directory of census-*.jsonl files")
	dsn := flag.String("dsn", "", "superuser DSN of a migrated database")
	flagMB := flag.Int("flag-mb", 64, "flag a statement whose planning grows the backend by this many MB")
	workers := flag.Int("workers", 1, "statements planned at once; each can hold hundreds of MB")
	flag.Parse()
	if *dir == "" || *dsn == "" {
		fmt.Fprintln(os.Stderr, "sql-census: -census and -dsn are both required")
		os.Exit(2)
	}
	stmts, err := load(*dir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "sql-census:", err)
		os.Exit(1)
	}
	measureAll(*dsn, stmts, *workers)
	report(os.Stdout, stmts, *flagMB*1024)
}

// load merges every process's file. One statement sent from several places
// keeps each call chain, because the fix belongs to whichever composed it.
func load(dir string) ([]*statement, error) {
	files, err := filepath.Glob(filepath.Join(dir, "census-*.jsonl"))
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no census-*.jsonl in %s: was MARGINCE_SQL_CENSUS_DIR set for the run?", dir)
	}
	byHash := map[string]*statement{}
	for _, name := range files {
		if err := loadFile(name, byHash); err != nil {
			return nil, err
		}
	}
	stmts := make([]*statement, 0, len(byHash))
	for _, s := range byHash {
		if explainable.MatchString(s.sql) {
			stmts = append(stmts, s)
		}
	}
	sort.Slice(stmts, func(i, j int) bool { return stmts[i].hash < stmts[j].hash })
	return stmts, nil
}

func loadFile(name string, byHash map[string]*statement) error {
	f, err := os.Open(name)
	if err != nil {
		return err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 64<<20)
	for sc.Scan() {
		var line censusLine
		if err := json.Unmarshal(sc.Bytes(), &line); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		s, ok := byHash[line.Hash]
		if !ok {
			s = &statement{hash: line.Hash, sql: line.SQL}
			byHash[line.Hash] = s
		}
		s.chains = append(s.chains, line.Frames)
	}
	return sc.Err()
}

func measureAll(dsn string, stmts []*statement, workers int) {
	jobs := make(chan *statement)
	var wg sync.WaitGroup
	for range max(workers, 1) {
		wg.Go(func() {
			for s := range jobs {
				if err := measure(context.Background(), dsn, s); err != nil {
					s.err = strings.SplitN(err.Error(), "\n", 2)[0]
				}
			}
		})
	}
	for _, s := range stmts {
		jobs <- s
	}
	close(jobs)
	wg.Wait()
}

// measure plans s on a connection of its own, so the growth of the backend's
// high-water mark is this statement's alone. A generic plan needs no argument
// values, and EXPLAIN without ANALYZE runs nothing.
func measure(ctx context.Context, dsn string, s *statement) error {
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return err
	}
	defer conn.Close(ctx)
	if _, err := conn.Exec(ctx, `SET jit = off`); err != nil {
		return err
	}
	before, err := peakKB(ctx, conn)
	if err != nil {
		return err
	}
	results, err := conn.PgConn().Exec(ctx, "EXPLAIN (GENERIC_PLAN, SUMMARY, FORMAT JSON) "+s.sql).ReadAll()
	if err != nil {
		return err
	}
	if len(results) != 1 || len(results[0].Rows) != 1 {
		return errors.New("EXPLAIN returned no plan")
	}
	var plans []struct {
		PlanningTime float64 `json:"Planning Time"` //nolint:tagliatelle // fixed by the server's plan format
	}
	if err := json.Unmarshal(results[0].Rows[0][0], &plans); err != nil || len(plans) != 1 {
		return fmt.Errorf("reading the plan: %w", err)
	}
	after, err := peakKB(ctx, conn)
	if err != nil {
		return err
	}
	s.planMs, s.peakKB = plans[0].PlanningTime, after-before
	return nil
}

func peakKB(ctx context.Context, conn *pgx.Conn) (int, error) {
	var status string
	if err := conn.QueryRow(ctx, `SELECT pg_read_file('/proc/' || pg_backend_pid() || '/status')`).Scan(&status); err != nil {
		return 0, err
	}
	m := vmHWM.FindStringSubmatch(status)
	if m == nil {
		return 0, errors.New("the backend's status has no VmHWM line")
	}
	return strconv.Atoi(m[1])
}
