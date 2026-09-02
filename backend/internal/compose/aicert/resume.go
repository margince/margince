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
// plus the requests this build constructs from it), the same BINARY, the same
// repeat index — and inside resumeWindow. Anything else and the run is paid for
// again. The binary is in that list because the stamp is not enough on its own:
// it covers the requests, never the code that judges the replies.
//
// One run at a time per directory, held by an exclusive lock file rather than
// assumed — see claimResumeDir.
//
// The journal is a local cache on the operator's own machine, and it is trusted
// as far as that: anything able to write into the resume directory can put a
// run there that no model served. That is the same trust the committed records
// themselves sit behind, one directory up, and the guards above are what keep an
// HONEST stale line from being replayed — they are not an authenticity claim
// against a tampered one.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
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
	// Build identifies the binary that scored the run, and it is here because
	// the scenario stamp does NOT cover the code that turns a reply into a
	// measurement. A stamp digests the scenario and the two requests built from
	// it; the site's own Evaluate, checkCaps and the judge-reply parsing all sit
	// outside it. Without this, tightening a validator and re-running inside the
	// window replays hard_pass values the OLD validator produced, and the record
	// certifies this build on measurements it would itself have failed.
	Build string `json:"build"`
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

// binaryIdentity is the SHA-256 of the running executable — the exact code that
// will evaluate, cap-check and score whatever this run replays.
//
// The binary itself rather than a VCS stamp, because this lane runs as a `go
// test` binary and those carry no vcs.revision setting: an identity read from
// build info would be the empty string here, compare equal to every other empty
// string, and hold nothing at all while looking like a guard. It is computed
// once, before the first paid call, so a run pays a fraction of a second for it
// and a failure to read it costs nothing.
func binaryIdentity() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("aicert: locating this binary to identify what scored a run: %w", err)
	}
	f, err := os.Open(exe) // #nosec G304 -- os.Executable, not caller input
	if err != nil {
		return "", fmt.Errorf("aicert: reading this binary to identify what scored a run: %w", err)
	}
	defer func() {
		//craft:ignore swallowed-errors a read-only handle already fully consumed; nothing it could report changes the digest
		_ = f.Close()
	}()
	sum := sha256.New()
	if _, err := io.Copy(sum, f); err != nil {
		return "", fmt.Errorf("aicert: digesting this binary to identify what scored a run: %w", err)
	}
	return hex.EncodeToString(sum.Sum(nil)), nil
}

// bindingKey renders a binding as the string two runs are compared on. Input is
// folded in with the rest because it changes what the model may be GIVEN, which
// is part of what a run measured and not a label on it.
//
// Each component is QUOTED rather than joined on a separator, so no value
// containing the separator can spell another binding's key: two bindings that
// collided here would replay one model's runs under the other's name, which is
// the one mistake this whole file exists to make impossible.
func bindingKey(c ai.ProviderConfig) string {
	return fmt.Sprintf("%q|%q|%q|%q", c.Provider, c.Model, c.BaseURL, strings.Join(c.Input, ","))
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
	sink   *jsonlSink
	loaded map[resumeKey]runOutcome
	judge  string
	// build is this binary's identity, filed on every appended line and required
	// of every replayed one — see journaledRun.Build.
	build string
	// lock is the exclusive claim on the resume directory, released on close.
	lock *os.File
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
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("aicert: resume dir %s: %w", dir, err)
	}
	build, err := binaryIdentity()
	if err != nil {
		return nil, err
	}
	lock, err := claimResumeDir(dir)
	if err != nil {
		return nil, err
	}
	path := filepath.Join(dir, journalFilename)
	j := &runJournal{
		loaded:  map[resumeKey]runOutcome{},
		judge:   bindingKey(judge),
		profile: string(profile),
		build:   build,
		lock:    lock,
	}
	live, err := j.readLive(ctx, path, now, log)
	if err != nil {
		return nil, errors.Join(err, releaseResumeDir(lock))
	}
	if err := rewriteJournal(path, live); err != nil {
		return nil, errors.Join(err, releaseResumeDir(lock))
	}
	// Appending, not truncating: the filename is stable BECAUSE a later run has
	// to find the earlier one's lines, which is the whole point.
	sink, err := openJSONLSink(dir, journalFilename, "resume journal", os.O_APPEND|os.O_CREATE|os.O_WRONLY)
	if err != nil {
		return nil, errors.Join(err, releaseResumeDir(lock))
	}
	j.sink, j.Path = sink, sink.Path
	if _, err := fmt.Fprintf(os.Stdout, "aicert: %d journaled run(s) replayable\n", len(j.loaded)); err != nil {
		return nil, errors.Join(fmt.Errorf("aicert: announce replayable runs: %w", err), sink.close(), releaseResumeDir(lock))
	}
	return j, nil
}

// journalFilename is stable across runs on purpose — a resume that could not
// find the previous run's file would resume nothing.
const journalFilename = "aicert-resume.jsonl"

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
		if line.Judge == j.judge && line.Profile == j.profile && line.CorpusVersion == corpusVersionV1 && line.Build == j.build {
			j.loaded[keyOf(line)] = line.Outcome
		}
	}
	return live, nil
}

// resumeKeyFor builds the key both sides go through — the load that files a
// journaled line and the lookup that asks for one. Held by
// TestALookupAndAJournaledLineAgreeOnTheirKey: two literals that happened to
// agree today would let a field added to resumeKey be filled in on one side
// only, and a lookup keyed on less than the append was filed under silently
// replays the wrong run.
func resumeKeyFor(candidate, task, scenario, stamp string, run int) resumeKey {
	return resumeKey{candidate: candidate, task: task, scenario: scenario, stamp: stamp, run: run}
}

// keyOf is resumeKeyFor over a line already on disk.
func keyOf(line journaledRun) resumeKey {
	return resumeKeyFor(line.Candidate, line.Task, line.Scenario, line.Stamp, line.Run)
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
	return errors.Join(j.sink.close(), releaseResumeDir(j.lock))
}

// claimResumeDir takes an exclusive claim on the resume directory, so a second
// certification run refuses rather than quietly destroying the first's journal.
//
// Two runs in one directory is not a rare race: splitting a 90-minute paid
// corpus into parallel TASK= runs is the obvious operator move, and they share
// the per-worktree default directory. The second run's compaction renames a
// fresh file over the first's, whose already-open append handle then points at
// an unlinked inode — so the first journals every remaining run into a file
// nobody will ever read, with no error, and its crash recovery silently does not
// exist. The cost is exactly the re-paid corpus this file was written to prevent.
func claimResumeDir(dir string) (*os.File, error) {
	path := filepath.Join(dir, "aicert-resume.lock")
	lock, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600) // #nosec G304 -- see openRunJournal
	if errors.Is(err, os.ErrExist) {
		return nil, fmt.Errorf("aicert: another certification run holds the resume directory %s; "+
			"wait for it, or point this run elsewhere with RESUME=<dir> — and if no run is going, "+
			"a previous one was killed and %s is stale: delete it", dir, path)
	}
	if err != nil {
		return nil, fmt.Errorf("aicert: claiming resume directory %s: %w", dir, err)
	}
	return lock, nil
}

// releaseResumeDir drops the claim. Nil-safe, so every error path out of
// openRunJournal can join it unconditionally.
func releaseResumeDir(lock *os.File) error {
	if lock == nil {
		return nil
	}
	return errors.Join(lock.Close(), os.Remove(lock.Name()))
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

// restartHint says what a restart will actually cost, and says nothing when it
// would cost everything.
//
// A message that promised a free restart under RESUME= — resuming switched off
// — would send an operator back into a full paid corpus believing the runs
// already scored were waiting for them. It reports what this run's journal can
// really offer, or stays quiet.
func (t taskJournal) restartHint() string {
	if t.j == nil {
		return " (resuming is off, so it will pay for every run again: RESUME=<dir> keeps them next time)"
	}
	return " — every run already scored replays from the resume journal for six hours, so a restart pays only for what is left"
}

// lookup returns the journaled outcome for one repeat of one scenario, if this
// run may replay it.
func (t taskJournal) lookup(sc Scenario, stamp string, run int) (runOutcome, bool) {
	if t.j == nil {
		return runOutcome{}, false
	}
	out, ok := t.j.loaded[resumeKeyFor(t.candidate, string(t.task), sc.Name, stamp, run)]
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
		Build:         t.j.build,
		Judge:         t.j.judge,
		Profile:       t.j.profile,
		CorpusVersion: corpusVersionV1,
		Task:          string(t.task),
		Scenario:      sc.Name,
		Stamp:         stamp,
		Run:           run,
		Outcome:       outcome,
	}
	if err := encodeLine(t.j.sink, line); err != nil {
		log.WarnContext(ctx, "aicert: resume journal write failed — the run stands, but a restart will pay for it again",
			"task", string(t.task), "scenario", sc.Name, "run", run, "err", err)
	}
}
