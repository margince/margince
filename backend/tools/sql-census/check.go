// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"maps"
	"os"
	"slices"
	"strings"

	"github.com/margince/margince/backend/internal/platform/auth"
)

// repeat is one sender whose statement carries the content gate more than once.
type repeat struct {
	sender string
	copies int
	sqlKB  float64
}

func runCheck(args []string) error {
	fs := flag.NewFlagSet("check", flag.ExitOnError)
	dir := fs.String("census", "", "directory of census-*.jsonl files")
	allowPath := fs.String("allow", "", "allowlist of senders that may still repeat the gate")
	seenPath := fs.String("seen", "", "write the senders seen repeating here, for stale")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *dir == "" || *allowPath == "" {
		return errors.New("check: -census and -allow are both required")
	}
	allow, err := readAllow(*allowPath)
	if err != nil {
		return err
	}
	stmts, err := load(*dir)
	if err != nil {
		return err
	}
	repeats := repeatedGates(stmts)
	if *seenPath != "" {
		if err := writeSeen(*seenPath, repeats); err != nil {
			return err
		}
	}
	return judge(os.Stdout, repeats, allow)
}

// repeatedGates answers every sender whose statement carries more than one
// content gate. One statement sent from two places is two senders, because
// the fix belongs to whichever composed it.
func repeatedGates(stmts []*statement) []repeat {
	var out []repeat
	for _, s := range stmts {
		copies := auth.ContentGateCopies(s.sql)
		if copies < 2 {
			continue
		}
		for _, chain := range s.chains {
			out = append(out, repeat{sender: senderKey(chain), copies: copies, sqlKB: float64(len(s.sql)) / 1024})
		}
	}
	return out
}

func judge(w io.Writer, repeats []repeat, allow map[string]string) error {
	// One line per sender, at its worst statement: a sender reached by many
	// tests is still one fix.
	worst := map[string]repeat{}
	for _, r := range repeats {
		if _, ok := allow[r.sender]; !ok && r.copies >= worst[r.sender].copies {
			worst[r.sender] = r
		}
	}
	refused := make([]repeat, 0, len(worst))
	for _, sender := range slices.Sorted(maps.Keys(worst)) {
		refused = append(refused, worst[sender])
	}
	if len(refused) == 0 {
		fmt.Fprintf(w, "sql-census check: ok, %d repeated-gate statements, all from allowlisted senders\n", len(repeats))
		return nil
	}
	fmt.Fprintln(w, "A statement carries the activity content gate more than once. Postgres plans every copy,")
	fmt.Fprintln(w, "and a pooled connection keeps that memory. Gate the activities once, in a CTE, and test")
	fmt.Fprintln(w, "membership in that set (compose/assurancesignalread.go is a worked example).")
	for _, r := range refused {
		fmt.Fprintf(w, "  %s: %d copies, %.0f KB of SQL\n", r.sender, r.copies, r.sqlKB)
	}
	return fmt.Errorf("check: %d statement senders repeat the content gate", len(refused))
}

// senderKey names the product function that sent a statement, as "file func".
// The line is left out so an edit above it does not orphan an allowlist entry.
func senderKey(chain []string) string {
	for _, frame := range chain {
		loc, fn, _ := strings.Cut(frame, " ")
		file, _, _ := strings.Cut(loc, ":")
		if !strings.HasSuffix(file, "_test.go") {
			return file + " " + fn
		}
	}
	return "unknown"
}

// readAllow reads "file func reason" lines. A reason is required: an entry
// with none is a waiver nobody can judge.
func readAllow(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	allow := map[string]string{}
	sc := bufio.NewScanner(f)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 3 {
			return nil, fmt.Errorf("%s:%d: want \"file func reason\", got %q", path, n, line)
		}
		allow[fields[0]+" "+fields[1]] = strings.Join(fields[2:], " ")
	}
	return allow, sc.Err()
}

func writeSeen(path string, repeats []repeat) error {
	senders := map[string]bool{}
	for _, r := range repeats {
		senders[r.sender] = true
	}
	return os.WriteFile(path, []byte(strings.Join(slices.Sorted(maps.Keys(senders)), "\n")+"\n"), 0o644)
}

// runStale fails on an allowlist entry no run saw repeat. Its statement was
// fixed or is gone, and an entry that stays would excuse its next regression.
// It reads every shard's seen file, because one shard runs a slice of the lane.
func runStale(args []string) error {
	fs := flag.NewFlagSet("stale", flag.ExitOnError)
	allowPath := fs.String("allow", "", "allowlist of senders that may still repeat the gate")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *allowPath == "" || fs.NArg() == 0 {
		return errors.New("stale: -allow and at least one seen file are required")
	}
	allow, err := readAllow(*allowPath)
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, name := range fs.Args() {
		body, err := os.ReadFile(name)
		if err != nil {
			return err
		}
		for sender := range strings.Lines(string(body)) {
			if sender = strings.TrimSpace(sender); sender != "" {
				seen[sender] = true
			}
		}
	}
	var gone []string
	for sender := range allow {
		if !seen[sender] {
			gone = append(gone, sender)
		}
	}
	if len(gone) == 0 {
		fmt.Printf("sql-census stale: ok, every one of %d allowlist entries still repeats\n", len(allow))
		return nil
	}
	slices.Sort(gone)
	fmt.Println("These allowlist entries no longer repeat the content gate. Remove them from the allowlist:")
	for _, sender := range gone {
		fmt.Println("  " + sender)
	}
	return fmt.Errorf("stale: %d allowlist entries no longer repeat", len(gone))
}
