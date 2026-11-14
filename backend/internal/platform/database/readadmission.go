// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package database

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
	"weak"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/sync/semaphore"
)

const readAdmissionTimeout = 15 * time.Second

var readAdmissions = struct {
	sync.Mutex
	pools map[weak.Pointer[pgxpool.Pool]]*readAdmission
}{pools: make(map[weak.Pointer[pgxpool.Pool]]*readAdmission)}

type readAdmission struct {
	slots       *semaphore.Weighted
	attempts    atomic.Int64
	wait        atomic.Int64
	maxWait     atomic.Int64
	waiters     atomic.Int64
	peakWaiters atomic.Int64
	active      atomic.Int64
}

type readAdmissionKey struct{}

type readPermit struct {
	admission *readAdmission
	active    atomic.Bool
}

// Weak keys keep independent pools independent without retaining closed pools.
func admissionOf(pool *pgxpool.Pool) *readAdmission {
	readAdmissions.Lock()
	defer readAdmissions.Unlock()
	key := weak.Make(pool)
	if admission, ok := readAdmissions.pools[key]; ok {
		return admission
	}
	admission := &readAdmission{slots: semaphore.NewWeighted(int64(max(1, min(4, pool.Stat().MaxConns()/2))))}
	readAdmissions.pools[key] = admission
	runtime.AddCleanup(pool, func(key weak.Pointer[pgxpool.Pool]) {
		readAdmissions.Lock()
		defer readAdmissions.Unlock()
		delete(readAdmissions.pools, key)
	}, key)
	return admission
}

// WithReadAdmission waits before pool checkout, leaving room for short reads
// and writes while composed reads contend for a bounded allowance.
func WithReadAdmission(ctx context.Context, pool *pgxpool.Pool, fn func(context.Context) error) error {
	if _, joined := snapshotOf(ctx); pool == nil || joined {
		return fn(ctx)
	}
	admission := admissionOf(pool)
	defer runtime.KeepAlive(pool)
	if held, ok := ctx.Value(readAdmissionKey{}).(*readPermit); ok && held.admission == admission && held.active.Load() {
		return fn(ctx)
	}
	if err := admission.acquire(ctx); err != nil {
		return err
	}
	permit := &readPermit{admission: admission}
	permit.active.Store(true)
	defer func() {
		permit.active.Store(false)
		admission.active.Add(-1)
		admission.slots.Release(1)
	}()
	return fn(context.WithValue(ctx, readAdmissionKey{}, permit))
}

func (a *readAdmission) acquire(ctx context.Context) error {
	started := time.Now()
	a.attempts.Add(1)
	waiting := a.waiters.Add(1)
	updatePeak(&a.peakWaiters, waiting)
	queue, cancel := context.WithTimeout(ctx, readAdmissionTimeout)
	defer cancel()
	err := a.slots.Acquire(queue, 1)
	a.waiters.Add(-1)
	elapsed := time.Since(started)
	a.wait.Add(int64(elapsed))
	updatePeak(&a.maxWait, int64(elapsed))
	if meter, ok := StatementMeterOf(ctx); ok {
		meter.add(StatementCost{Admissions: 1, AdmissionWait: elapsed})
	}
	if err != nil {
		return fmt.Errorf("pg: waiting for read capacity: %w", err)
	}
	a.active.Add(1)
	return nil
}

func updatePeak(peak *atomic.Int64, value int64) {
	for current := peak.Load(); value > current; current = peak.Load() {
		if peak.CompareAndSwap(current, value) {
			return
		}
	}
}

// ReadAdmissionCost separates work waiting outside the pool from pool checkout.
type ReadAdmissionCost struct {
	Attempts    int64
	Wait        time.Duration
	MaxWait     time.Duration
	Waiters     int64
	PeakWaiters int64
	Active      int64
}

// ReadAdmissionStats reports this pool's expensive-read allowance.
func ReadAdmissionStats(pool *pgxpool.Pool) ReadAdmissionCost {
	a := admissionOf(pool)
	return ReadAdmissionCost{
		Attempts: a.attempts.Load(), Wait: time.Duration(a.wait.Load()), MaxWait: time.Duration(a.maxWait.Load()),
		Waiters: a.waiters.Load(), PeakWaiters: a.peakWaiters.Load(), Active: a.active.Load(),
	}
}
