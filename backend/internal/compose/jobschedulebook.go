// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The schedules an admin sets. A {setting: …} cadence in api/jobs.yaml names
// the setting its interval is read from; this book holds the interval each one
// currently has, and moves a running schedule when an admin changes it.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/platform/jobs"
)

// scheduleReadActor attributes the worker's reads of its own schedules.
const scheduleReadActor = "system:schedule_read"

// scheduleRecheck is how often a running worker looks for a changed schedule:
// often enough that an admin sees the change take within a minute or two, and
// cheap, one indexed read per setting.
const scheduleRecheck = time.Minute

// liveInterval is a River schedule whose interval can move while the job is
// registered. River asks Next only after each run, which is why a change also
// re-registers the job (see apply): the next run then counts from the change.
type liveInterval struct{ nanos atomic.Int64 }

func (l *liveInterval) Next(t time.Time) time.Time { return t.Add(l.get()) }

func (l *liveInterval) get() time.Duration { return time.Duration(l.nanos.Load()) }

// tracked is one scheduled kind and the job River was handed for it.
type tracked struct {
	setting string
	job     *river.PeriodicJob
}

// ScheduleBook is the interval every setting-driven schedule currently runs at,
// keyed by setting, and the jobs built from them, keyed by kind.
type ScheduleBook struct {
	mu        sync.Mutex
	intervals map[string]*liveInterval
	kinds     map[string]tracked
}

// newScheduleBook seeds the book from the values read, one per setting key.
func newScheduleBook(values map[string]time.Duration) *ScheduleBook {
	book := &ScheduleBook{intervals: map[string]*liveInterval{}, kinds: map[string]tracked{}}
	for key, value := range values {
		live := &liveInterval{}
		live.nanos.Store(int64(value))
		book.intervals[key] = live
	}
	return book
}

// scheduleSettings lists the setting keys declared cadences read, derived from
// the compiled contract rather than kept by hand.
func scheduleSettings() []string {
	keys := map[string]bool{}
	for _, spec := range jobs.Declared() {
		if spec.Cadence.Setting != "" {
			keys[spec.Cadence.Setting] = true
		}
	}
	return slices.Sorted(maps.Keys(keys))
}

// DefaultSchedules is the book every registered default builds: the schedules
// of an installation nobody has tuned, and what a wiring read without a
// database (the census) schedules from.
func DefaultSchedules() *ScheduleBook {
	values := map[string]time.Duration{}
	for _, key := range scheduleSettings() {
		value, err := scheduleValue(key, settingDefault)
		if err != nil {
			panic(err.Error())
		}
		values[key] = value
	}
	return newScheduleBook(values)
}

// SchedulesForTest is the default book with the given settings moved, for a
// suite that needs a schedule shorter than any an admin may set. Nothing in the
// product calls it.
func SchedulesForTest(moved map[string]time.Duration) *ScheduleBook {
	book := DefaultSchedules()
	for key, value := range moved {
		live, known := book.intervals[key]
		if !known {
			panic("compose: no cadence schedules from " + key)
		}
		live.nanos.Store(int64(value))
	}
	return book
}

// ReadSchedules reads every schedule setting as it stands. An installation not
// yet provisioned has no workspace to read under and takes the defaults, which
// is what it would read anyway.
func ReadSchedules(ctx context.Context, pool *pgxpool.Pool) (*ScheduleBook, error) {
	values, err := readScheduleValues(ctx, pool)
	if err != nil {
		return nil, err
	}
	return newScheduleBook(values), nil
}

func readScheduleValues(ctx context.Context, pool *pgxpool.Pool) (map[string]time.Duration, error) {
	read, err := installationSettingReader(ctx, pool)
	if err != nil {
		return nil, err
	}
	values := map[string]time.Duration{}
	for _, key := range scheduleSettings() {
		value, err := scheduleValue(key, read)
		if err != nil {
			return nil, err
		}
		values[key] = value
	}
	return values, nil
}

// installationSettingReader reads installation settings by key as the worker,
// on the installation's one workspace. An installation not yet provisioned has
// no workspace to read under and answers every key with its default, which is
// what it would read anyway.
func installationSettingReader(ctx context.Context, pool *pgxpool.Pool) (func(key string) (json.RawMessage, error), error) {
	ws, err := singletonWorkspace(ctx, pool)
	if err != nil {
		return nil, err
	}
	if ws.IsZero() {
		return settingDefault, nil
	}
	store, readCtx := NewSettingsStore(pool), bootCtx(ctx, ws, scheduleReadActor)
	return func(key string) (json.RawMessage, error) { return store.Raw(readCtx, key) }, nil
}

// settingDefault is a registered setting's default, by key.
func settingDefault(key string) (json.RawMessage, error) {
	for _, def := range settingsDefinitions() {
		if def.Key() == key {
			return def.DefaultJSON()
		}
	}
	return nil, fmt.Errorf("compose: %s is not a registered setting", key)
}

// scheduleValue reads a schedule setting's whole seconds. Every schedule
// setting is an int of seconds, which TestEveryScheduleSettingDefaultsToWholeSeconds
// and each setting's validator hold, so a value that does not decode is a
// stored row nobody wrote through the settings surface.
func scheduleValue(key string, read func(string) (json.RawMessage, error)) (time.Duration, error) {
	raw, err := read(key)
	if err != nil {
		return 0, fmt.Errorf("compose: reading the schedule %s: %w", key, err)
	}
	var seconds int
	if err := json.Unmarshal(raw, &seconds); err != nil {
		return 0, fmt.Errorf("compose: the schedule setting %s does not hold whole seconds: %w", key, err)
	}
	return time.Duration(seconds) * time.Second, nil
}

// schedule builds the periodic job for one setting-driven kind and records it,
// and reports whether it runs now: a kind whose setting holds zero is built,
// so switching it back on can add it, but not handed to River.
func (b *ScheduleBook) schedule(spec jobs.Spec, args river.JobArgs, opts *river.InsertOpts) (*river.PeriodicJob, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	live, known := b.intervals[spec.Cadence.Setting]
	if !known {
		panic("compose: " + spec.Kind + " schedules from " + spec.Cadence.Setting + ", which this book was not read for")
	}
	job := river.NewPeriodicJob(live, func() (river.JobArgs, *river.InsertOpts) { return args, opts },
		&river.PeriodicJobOpts{ID: spec.Kind, RunOnStart: true})
	b.kinds[spec.Kind] = tracked{setting: spec.Cadence.Setting, job: job}
	return job, live.get() > 0
}

// periodicRegistry is the part of the runner a changed schedule touches.
type periodicRegistry interface {
	RemovePeriodic(id string)
	AddPeriodic(job *river.PeriodicJob) error
}

var _ periodicRegistry = (*jobs.Runner)(nil)

// apply moves every schedule whose setting changed to its new value. A kind is
// taken off River's list and put back, so it runs once now and then on the new
// interval, rather than waiting out the run the old one had already planned;
// one switched to zero is only taken off. A setting whose kinds could not be
// put back keeps its old value in the book, so the next check sees the change
// again and retries it.
func (b *ScheduleBook) apply(values map[string]time.Duration, runner periodicRegistry) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	var failed []error
	for _, key := range slices.Sorted(maps.Keys(values)) {
		live, known := b.intervals[key]
		if !known || live.get() == values[key] {
			continue
		}
		old := live.get()
		live.nanos.Store(int64(values[key]))
		if err := b.reregister(key, runner); err != nil {
			live.nanos.Store(int64(old))
			failed = append(failed, err)
		}
	}
	return errors.Join(failed...)
}

// reregister takes every kind scheduled from one setting off River and puts
// back the ones whose interval is still positive.
func (b *ScheduleBook) reregister(key string, runner periodicRegistry) error {
	for _, kind := range slices.Sorted(maps.Keys(b.kinds)) {
		entry := b.kinds[kind]
		if entry.setting != key {
			continue
		}
		runner.RemovePeriodic(kind)
		if b.intervals[key].get() <= 0 {
			continue
		}
		if err := runner.AddPeriodic(entry.job); err != nil {
			return fmt.Errorf("compose: rescheduling %s: %w", kind, err)
		}
	}
	return nil
}

// Watch re-reads the schedules every scheduleRecheck and applies what moved,
// until ctx ends. A failed read is logged and retried on the next tick: the
// schedules already running are the right ones until a read says otherwise.
func (b *ScheduleBook) Watch(ctx context.Context, pool *pgxpool.Pool, runner periodicRegistry, log *slog.Logger) {
	ticker := time.NewTicker(scheduleRecheck)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		values, err := readScheduleValues(ctx, pool)
		if err != nil {
			log.WarnContext(ctx, "schedules: could not read the schedule settings; the current schedules keep running", "err", err)
			continue
		}
		if err := b.apply(values, runner); err != nil {
			log.WarnContext(ctx, "schedules: a changed schedule could not be put back on the runner; that job is off until the next check retries it", "err", err)
		}
	}
}
