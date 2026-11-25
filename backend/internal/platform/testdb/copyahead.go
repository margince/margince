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
)

// CREATE DATABASE waits for two checkpoints and DROP DATABASE for one. A
// test that copied and dropped its own database inline sat idle through all
// three. The next copy is made while the current test runs, and a finished
// test's database is dropped behind it, so the test runs instead of waiting.
// A copy or drop a process leaves unfinished when it exits is swept by the
// lane, whose drop-db removes every test copy of the package's clone.

// AdminConnsEnv is how many admin connections the lane budgeted for each
// package process: one for the drops behind the tests and one for the copies.
const AdminConnsEnv = "MARGINCE_TEST_ADMIN_CONNS"

// pendingDrops bounds how far drops may fall behind the tests. A test that
// finds the queue full waits, so the cluster's RAM holds a bounded number of
// databases no test uses.
const pendingDrops = 4

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
	copyAhead(template)
	return made.db, nil
}

// copyAhead starts the next copy of template unless one is already on its way.
func copyAhead(template string) {
	spare.mu.Lock()
	defer spare.mu.Unlock()
	if spare.next != nil {
		return
	}
	ready := make(chan madeCopy, 1)
	spare.next, spare.from = ready, template
	go func() {
		db, err := copyTemplate(context.Background(), template)
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
		go dropQueued()
	})
	behind.queue <- db
}

// dropQueued drops the queued databases one at a time, on one admin connection.
func dropQueued() {
	for db := range behind.queue {
		if err := drop(context.Background(), db.Name); err != nil {
			behind.mu.Lock()
			behind.failed = append(behind.failed, err)
			behind.mu.Unlock()
		}
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
