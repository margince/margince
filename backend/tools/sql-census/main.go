// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Command sql-census reads the statements an integration run sent, from the
// JSONL files a run writes when MARGINCE_SQL_CENSUS_DIR is set. Postgres plans
// every copy of a clause a statement carries. A pooled backend keeps that
// memory until its connection closes, so a result test on small data cannot
// see the cost.
//
//	go run ./tools/sql-census check -census DIR -allow FILE [-seen FILE]
//	go run ./tools/sql-census stale -allow FILE SEEN...
//	go run ./tools/sql-census rank -census DIR -dsn SUPERUSER_DSN > report.md
//
// check fails on a statement that carries the activity content gate more than
// once, unless its sender is in the allowlist. stale fails on an allowlist entry
// that no run saw repeat, so the list only shrinks. rank plans each statement
// once and orders them by the planner memory they cost. It reads the peak from
// the backend's /proc status, so it needs a superuser. It is a diagnosis, not a
// gate: the figure moves a few MB between runs.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
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
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "sql-census: name a command: check, stale or rank")
		os.Exit(2)
	}
	var err error
	switch cmd, args := os.Args[1], os.Args[2:]; cmd {
	case "check":
		err = runCheck(args)
	case "stale":
		err = runStale(args)
	case "rank":
		err = runRank(args)
	default:
		fmt.Fprintf(os.Stderr, "sql-census: unknown command %q: use check, stale or rank\n", cmd)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "sql-census:", err)
		os.Exit(1)
	}
}

func runRank(args []string) error {
	fs := flag.NewFlagSet("rank", flag.ExitOnError)
	dir := fs.String("census", "", "directory of census-*.jsonl files")
	dsn := fs.String("dsn", "", "superuser DSN of a migrated database")
	flagMB := fs.Int("flag-mb", 64, "flag a statement whose planning grows the backend by this many MB")
	workers := fs.Int("workers", 1, "statements planned at once; each can hold hundreds of MB")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *dir == "" || *dsn == "" {
		return errors.New("rank: -census and -dsn are both required")
	}
	stmts, err := load(*dir)
	if err != nil {
		return err
	}
	stmts = slices.DeleteFunc(stmts, func(s *statement) bool { return !explainable.MatchString(s.sql) })
	measureAll(*dsn, stmts, *workers)
	return report(os.Stdout, stmts, *flagMB*1024)
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
	stmts := slices.Collect(maps.Values(byHash))
	sort.Slice(stmts, func(i, j int) bool { return stmts[i].hash < stmts[j].hash })
	return stmts, nil
}

func loadFile(name string, byHash map[string]*statement) (err error) {
	f, err := os.Open(name) //nolint:gosec // a census file from the directory the developer passed
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, f.Close()) }()
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
func measure(ctx context.Context, dsn string, s *statement) (err error) {
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, conn.Close(ctx)) }()
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
