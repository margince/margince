// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert

// The resume journal: every scored run appended to a JSONL file the moment it
// is scored, so an interrupted certification never pays for the same run twice.
//
// certifyTask is all-or-nothing by construction — one Record covers a whole
// task, so a fault anywhere in it returns before WriteRecord and every run
// already paid for is gone. That is the right shape for the record and the
// wrong one for the spend: a task with seven scenarios at three repeats
// discards twenty-one real model calls because the twenty-second lost its
// connection. This file keeps the record's shape and gives the spend a place
// to survive.
//
// A journaled run stands in for a fresh one ONLY when nothing it measured can
// have moved: the same candidate binding, the same judge, the same environment
// class, the same corpus format, the same scenario stamp (the scenario whole
// plus the requests this build constructs from it), the same repeat index —
// and inside resumeWindow. Anything else and the run is paid for again.
//
// The journal assumes ONE certification run at a time per directory, which is
// what the lane is: a paid run an operator starts and watches. Two at once
// would interleave appends and compact each other's lines away.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/margince/margince/backend/internal/modules/ai"
)

// resumeWindow is how long a journaled run may stand in for a fresh one.
//
// A record pools its runs into one measurement, so replaying an older run is
// honest only while nothing it depended on can have moved. The key fields cover
// everything this build can see — both bindings, the environment class, the
// corpus format, the scenario's own stamp. The window covers what it cannot: a
// provider serving a different build of the same model name behind an unchanged
// binding. Six hours is one working session, which is the span an interrupted
// run is actually restarted within.
const resumeWindow = 6 * time.Hour

// journaledRun is one scored run as it survives on disk, and it carries every
// field a replay must match rather than trusting the file's name to carry them:
// one file holds whatever the last six hours ran, including other bindings.
type journaledRun struct {
	At string `json:"at"`
	// Candidate and Judge are bindingKey strings — what the run ASKED to be
	// served by, not what answered it. What answered is inside Outcome, and
	// taskAccumulation.addRun still holds every replayed row to the same
	// served-identity uniformity a live one must meet.
	Candidate     string     `json:"candidate"`
	Judge         string     `json:"judge"`
	Profile       string     `json:"profile"`
	CorpusVersion string     `json:"corpus_version"`
	Task          string     `json:"task"`
	Scenario      string     `json:"scenario"`
	Stamp         string     `json:"stamp"`
	Run           int        `json:"run"`
	Outcome       runOutcome `json:"outcome"`
}

// resumeKey identifies the one run a journaled line may stand in for.
//
// The judge, profile and corpus version are not here: they are constant for a
// whole certification run, so they are filtered once at load rather than
// compared per lookup. The candidate binding IS here, because a routed run
// certifies each task against its own resolved model.
type resumeKey struct {
	candidate, task, scenario, stamp string
	run                              int
}

// bindingKey renders a binding as the string two runs are compared on. Input is
// folded in with the rest because it changes what the model may be GIVEN, which
// is part of what a run measured and not a label on it.
func bindingKey(c ai.ProviderConfig) string {
	return strings.Join(append([]string{c.Provider, c.Model, c.BaseURL}, c.Input...), "|")
}

// runJournal is the whole run's journal file: the live runs it loaded, and the
// appender the runs scored from here go to. A nil *runJournal is the disabled
// state — every method no-ops on it — so the runner threads one value whether
// resuming is on or off, the way payloadTrace already does for the trace.
//
// The encoder writes STRAIGHT to the file, with nothing buffered in front of
// it. That is the whole mechanism: a journal flushed on close would be lost to
// exactly the kill, panic or dropped connection it exists to survive, and would
// fail silently, looking like a journal that simply had nothing to replay.
type runJournal struct {
	mu     sync.Mutex
	w      io.WriteCloser
	enc    *json.Encoder
	loaded map[resumeKey]runOutcome
	judge  string
	// profile is the environment class this run files records under, held so an
	// appended line states it rather than a reader inferring it from the file.
	profile string
	Path    string // absolute, printed to stdout when the journal opens
}

// openRunJournal loads dir's journal, drops from it everything that has expired
// or that this run could not replay anyway, rewrites the file with what
// survives, and opens it for append.
//
// Compaction happens here, before the first paid call, because that is where
// failing is free: a journal that could not be rewritten is a journal that
// cannot be trusted to be appended to either, and finding out after an hour of
// spend would be the same defect this file exists to remove.
func openRunJournal(ctx context.Context, dir string, judge ai.ProviderConfig, profile ai.Profile, now time.Time, log *slog.Logger) (*runJournal, error) {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("aicert: resume dir %s: %w", dir, err)
	}
	path := filepath.Join(dir, "aicert-resume.jsonl")
	j := &runJournal{
		loaded:  map[resumeKey]runOutcome{},
		judge:   bindingKey(judge),
		profile: string(profile),
	}
	live, err := j.readLive(ctx, path, now, log)
	if err != nil {
		return nil, err
	}
	if err := rewriteJournal(path, live); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600) // #nosec G304 -- the operator-named resume dir (MARGINCE_AICERT_RESUME) + a fixed filename; a dev lane, no request input
	if err != nil {
		return nil, fmt.Errorf("aicert: open resume journal %s: %w", path, err)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		//craft:ignore swallowed-errors close-then-report on the error path
		_ = f.Close()
		return nil, fmt.Errorf("aicert: absolute resume path for %s: %w", path, err)
	}
	j.w, j.enc, j.Path = f, json.NewEncoder(f), abs
	if _, err := fmt.Fprintf(os.Stdout, "aicert: resume journal → %s (%d run(s) replayable)\n", abs, len(j.loaded)); err != nil {
		//craft:ignore swallowed-errors close-then-report on the error path
		_ = f.Close()
		return nil, fmt.Errorf("aicert: announce resume path %s: %w", abs, err)
	}
	return j, nil
}

// readLive returns every unexpired line in the journal, and fills j.loaded with
// the subset THIS run could replay. The two differ on purpose: compaction must
// keep a line belonging to another judge or another profile, which is somebody
// else's still-good measurement, not this run's to discard.
func (j *runJournal) readLive(ctx context.Context, path string, now time.Time, log *slog.Logger) ([]journaledRun, error) {
	f, err := os.Open(path) // #nosec G304 -- see openRunJournal
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("aicert: read resume journal %s: %w", path, err)
	}
	defer func() {
		if cerr := f.Close(); cerr != nil {
			log.WarnContext(ctx, "aicert: closing the resume journal after reading it", "path", path, "err", cerr)
		}
	}()

	var live []journaledRun
	dec := json.NewDecoder(f)
	for {
		var line journaledRun
		if derr := dec.Decode(&line); derr != nil {
			if !errors.Is(derr, io.EOF) {
				// A truncated or unreadable tail is the very case this journal
				// exists for: a run killed mid-write. Everything before it was
				// written whole and still stands, so the scan stops rather than
				// failing the run.
				log.WarnContext(ctx, "aicert: resume journal ends mid-line — the runs before it still replay",
					"path", path, "kept", len(live), "err", derr)
			}
			break
		}
		at, terr := time.Parse(time.RFC3339Nano, line.At)
		if terr != nil || now.Sub(at) >= resumeWindow || at.After(now) {
			continue
		}
		live = append(live, line)
		if line.Judge == j.judge && line.Profile == j.profile && line.CorpusVersion == corpusVersionV1 {
			j.loaded[keyOf(line)] = line.Outcome
		}
	}
	return live, nil
}

// keyOf is the one place a line becomes a lookup key, so a replay can never be
// matched on a different set of fields from the one an append was filed under.
func keyOf(line journaledRun) resumeKey {
	return resumeKey{candidate: line.Candidate, task: line.Task, scenario: line.Scenario, stamp: line.Stamp, run: line.Run}
}

// rewriteJournal replaces path with exactly the lines given, via a temporary
// file in the same directory and a rename, so an interrupted compaction leaves
// the previous journal whole rather than a half-written one.
func rewriteJournal(path string, live []journaledRun) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "aicert-resume-*.jsonl")
	if err != nil {
		return fmt.Errorf("aicert: compact resume journal %s: %w", path, err)
	}
	enc := json.NewEncoder(tmp)
	for _, line := range live {
		if eerr := enc.Encode(line); eerr != nil {
			return errors.Join(fmt.Errorf("aicert: compact resume journal %s: %w", path, eerr), tmp.Close(), os.Remove(tmp.Name()))
		}
	}
	if cerr := tmp.Close(); cerr != nil {
		return errors.Join(fmt.Errorf("aicert: compact resume journal %s: %w", path, cerr), os.Remove(tmp.Name()))
	}
	if rerr := os.Rename(tmp.Name(), path); rerr != nil {
		return errors.Join(fmt.Errorf("aicert: compact resume journal %s: %w", path, rerr), os.Remove(tmp.Name()))
	}
	return nil
}

// close flushes and closes the underlying file. Nil-safe so the runner can
// defer it unconditionally.
func (j *runJournal) close() error {
	if j == nil {
		return nil
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.w.Close()
}

// forTask is the journal as ONE task's certification sees it: the file, plus
// the candidate binding every line of this task's own must match. A routed run
// resolves a different model per task, so binding it here is what keeps one
// task from replaying a run another task's model produced.
func (j *runJournal) forTask(task ai.Task, candidate ai.ProviderConfig) taskJournal {
	return taskJournal{j: j, task: task, candidate: bindingKey(candidate)}
}

// taskJournal is one task's view of the journal, so runScenario carries a
// single value instead of a file, a binding and a task name.
type taskJournal struct {
	j         *runJournal
	task      ai.Task
	candidate string
}

// lookup returns the journaled outcome for one repeat of one scenario, if this
// run may replay it.
func (t taskJournal) lookup(sc Scenario, stamp string, run int) (runOutcome, bool) {
	if t.j == nil {
		return runOutcome{}, false
	}
	t.j.mu.Lock()
	defer t.j.mu.Unlock()
	out, ok := t.j.loaded[resumeKey{candidate: t.candidate, task: string(t.task), scenario: sc.Name, stamp: stamp, run: run}]
	return out, ok
}

// append files one scored run, best-effort: a write failure is logged and
// swallowed, never returned. The journal exists so a paid run survives a fault;
// letting it become a new way for a paid run to FAIL would invert that. The
// error is heard — logged with the run it belongs to — not ignored.
func (t taskJournal) append(ctx context.Context, sc Scenario, stamp string, run int, outcome runOutcome, at time.Time, log *slog.Logger) {
	if t.j == nil {
		return
	}
	line := journaledRun{
		At:            at.UTC().Format(time.RFC3339Nano),
		Candidate:     t.candidate,
		Judge:         t.j.judge,
		Profile:       t.j.profile,
		CorpusVersion: corpusVersionV1,
		Task:          string(t.task),
		Scenario:      sc.Name,
		Stamp:         stamp,
		Run:           run,
		Outcome:       outcome,
	}
	t.j.mu.Lock()
	defer t.j.mu.Unlock()
	if err := t.j.enc.Encode(line); err != nil {
		log.WarnContext(ctx, "aicert: resume journal write failed — the run stands, but a restart will pay for it again",
			"task", string(t.task), "scenario", sc.Name, "run", run, "err", err)
	}
}
