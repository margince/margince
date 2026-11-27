// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package testdb

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"
)

// CREATE DATABASE waits for two checkpoints and DROP DATABASE for one. A
// test that copied and dropped its own database inline sat idle through all
// three. So the next copy is made while the current test runs. A finished
// test's database is dropped behind it. When a process exits, the lane's
// drop-db removes any copy it left, along with the package's clone.

// AdminConnsEnv is how many admin connections the lane budgeted for each
// package process. One makes the copies, and the rest drop finished tests'
// databases. Drops waiting on one checkpoint finish together, so more of them at
// once keep pace with a package of short tests.
const AdminConnsEnv = "MARGINCE_TEST_ADMIN_CONNS"

// pendingDrops bounds how far drops may fall behind the tests. A test that
// finds the queue full waits, so the cluster's RAM holds a bounded number of
// databases no test uses.
const pendingDrops = 8

// spare is the copy made for the next test that asks for one.
var spare struct {
	mu   sync.Mutex
	next chan madeCopy
	from string
}

type madeCopy struct {
	db  Database
	err error
}

// behind is the queue of databases whose tests ended, and the drops that
// failed, for the next test to report.
var behind struct {
	once   sync.Once
	queue  chan Database
	mu     sync.Mutex
	failed []error
	// done closes each queued database's channel once its drop ran.
	done map[string]chan struct{}
}

// nextCopy answers a copy of template: the spare when one is waiting,
// otherwise a fresh one. Either way it starts the copy for the next test.
func nextCopy(ctx context.Context, template string) (Database, error) {
	if err := droppedBehind(); err != nil {
		return Database{}, err
	}
	spare.mu.Lock()
	ready, from := spare.next, spare.from
	spare.next = nil
	spare.mu.Unlock()
	var made madeCopy
	if ready != nil && from == template {
		made = <-ready
	} else {
		if ready != nil {
			go discardSpare(ready)
		}
		made.db, made.err = copyTemplate(ctx, template)
	}
	if made.err != nil {
		return Database{}, made.err
	}
	copyAhead(ctx, template)
	return made.db, nil
}

// copyAhead starts the next copy of template unless one is already on its way.
// The copy outlives the test that started it, so it ignores that test's end.
func copyAhead(ctx context.Context, template string) {
	spare.mu.Lock()
	defer spare.mu.Unlock()
	if spare.next != nil {
		return
	}
	ready := make(chan madeCopy, 1)
	spare.next, spare.from = ready, template
	go func() {
		bounded, cancel := context.WithTimeout(context.WithoutCancel(ctx), adminStatementCeiling)
		defer cancel()
		db, err := copyTemplate(bounded, template)
		ready <- madeCopy{db: db, err: err}
	}()
}

// discardSpare drops a spare copied from a template this process no longer uses.
func discardSpare(ready chan madeCopy) {
	if made := <-ready; made.err == nil {
		dropBehind(made.db)
	}
}

// dropBehind forgets db and queues its drop after the test that used it.
func dropBehind(db Database) {
	forget("", db)
	behind.once.Do(func() {
		behind.queue = make(chan Database, pendingDrops)
		behind.done = map[string]chan struct{}{}
		for range dropWorkers() {
			go dropQueued()
		}
	})
	behind.mu.Lock()
	behind.done[db.Name] = make(chan struct{})
	behind.mu.Unlock()
	behind.queue <- db
}

// droppedEnd answers a channel that closes once the drop queued for name ran,
// or nil when no drop of name was queued.
func droppedEnd(name string) <-chan struct{} {
	behind.mu.Lock()
	defer behind.mu.Unlock()
	return behind.done[name]
}

// dropWorkers is how many drops run at once: every admin connection but the
// one the copies take, and at least one.
func dropWorkers() int {
	conns, err := adminConns()
	if err != nil || conns < 2 {
		return 1
	}
	return int(conns) - 1
}

// dropQueued drops queued databases one at a time, on one admin connection.
func dropQueued() {
	for db := range behind.queue {
		err := drop(context.Background(), db.Name)
		behind.mu.Lock()
		if err != nil {
			behind.failed = append(behind.failed, err)
		}
		close(behind.done[db.Name])
		delete(behind.done, db.Name)
		behind.mu.Unlock()
	}
}

// droppedBehind reports the drops that failed since the last report. Each one
// left a database on the test cluster, which the lane's sweep removes later.
func droppedBehind() error {
	behind.mu.Lock()
	defer behind.mu.Unlock()
	err := errors.Join(behind.failed...)
	behind.failed = nil
	if err != nil {
		return fmt.Errorf("an earlier test's database was not dropped: %w", err)
	}
	return nil
}

// adminConns is the admin pool's size: the lane's budget for it, or one
// connection when no lane declared one.
func adminConns() (int32, error) {
	raw := os.Getenv(AdminConnsEnv)
	if raw == "" {
		return 1, nil
	}
	n, err := strconv.ParseInt(raw, 10, 32)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("%s=%q is not a positive connection count; the lane sets it to the admin connections its budget holds", AdminConnsEnv, raw)
	}
	return int32(n), nil
}

// slowAdmin is how long a copy or a queued drop may hold a test up before the
// test's log says so. A lane that waits on the cluster then shows where.
const slowAdmin = time.Second

func logSlow(t testing.TB, step string, began time.Time) {
	t.Helper()
	if waited := time.Since(began); waited > slowAdmin {
		t.Logf("testdb: %s held the test up for %s", step, waited.Round(time.Millisecond))
	}
}
