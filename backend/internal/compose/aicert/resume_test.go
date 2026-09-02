// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert

// The resume journal, proved the only way that matters: a second certification
// whose candidate REFUSES every call still produces the record the first one
// produced. A scripted refusal is what "did not pay for it again" looks like
// from inside the process — a test that merely counted calls would pass just as
// happily on a journal that replayed nothing and got lucky.

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/ai"
)

var (
	testCandidateBinding = ai.ProviderConfig{Provider: ai.ProviderFake, Model: "candidate"}
	testJudgeBinding     = ai.ProviderConfig{Provider: ai.ProviderFake, Model: "judge"}
)

// fixedResumeNow is the instant every test here measures its window from. Any
// instant does; pinning one keeps a journal's age arithmetic exact.
var fixedResumeNow = time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)

// withJournal opens a journal in dir, hands it to fn, and closes it — the shape
// Run itself has. The journal holds an exclusive claim on its directory while
// open, so a test that leaked one would refuse its own next "run" rather than
// exercise the replay it came to test.
func withJournal(t *testing.T, dir string, now time.Time, fn func(*runJournal)) {
	t.Helper()
	j, err := openRunJournal(context.Background(), dir, testJudgeBinding, ai.ProfileEUHosted, now, quietLogger())
	if err != nil {
		t.Fatalf("openRunJournal: %v", err)
	}
	defer func() {
		if cerr := j.close(); cerr != nil {
			t.Errorf("closing the journal: %v", cerr)
		}
	}()
	fn(j)
}

// replayable reports whether one run of sc could be replayed from dir's journal
// by a run on this file's own bindings.
func replayable(t *testing.T, dir string, now time.Time, sc Scenario, candidate ai.ProviderConfig) bool {
	t.Helper()
	var ok bool
	withJournal(t, dir, now, func(j *runJournal) {
		_, ok = j.forTask(ai.TaskSummarize, candidate).lookup(sc, stampFor(t, sc), 1)
	})
	return ok
}

// certifyOnce runs one whole certification of sc against dir's journal — open,
// drive, close — so each call is a separate "run" the way the lane's are.
func certifyOnce(t *testing.T, dir string, sc Scenario, candidate, judge *ai.FakeClient) (Record, error) {
	t.Helper()
	withFixedNow(t, fixedResumeNow)
	// A test that reaches the re-drive path must not wait out its backoff.
	recordSleeps(t)
	var rec Record
	var err error
	withJournal(t, dir, fixedResumeNow, func(j *runJournal) {
		rec, err = certifyTask(wsContext(t), ai.TaskSummarize, []Scenario{sc}, testCensus(t),
			testCandidateBinding, testJudgeBinding, ai.ProfileEUHosted, 3, quietLogger(), &certifyHooks{
				candidateOpts: []ai.LocalOption{ai.WithFakeClient(candidate)},
				judgeOpts:     []ai.LocalOption{ai.WithFakeClient(judge)},
				journal:       j.forTask(ai.TaskSummarize, testCandidateBinding),
			})
	})
	return rec, err
}

// answeringFakes are a candidate and judge that certify every run.
func answeringFakes() (*ai.FakeClient, *ai.FakeClient) {
	return ai.NewFakeClient().Script(containsWidget, containsWidget, containsWidget),
		ai.NewFakeClient().Script(scoreJSON(90), scoreJSON(90), scoreJSON(90))
}

// refusingFakes answer nothing at all: every call is the fault a resumed run
// must not need to make. A fake with an empty script falls back to a generated
// completion, so the refusal is scripted explicitly.
func refusingFakes(t *testing.T) (*ai.FakeClient, *ai.FakeClient) {
	t.Helper()
	var steps []ai.FakeStep
	for range ladderRungs(t) * runAttempts * 3 {
		steps = append(steps, ai.FakeStep{Err: errDroppedConnection})
	}
	return ai.NewFakeClient().ScriptSteps(steps...), ai.NewFakeClient().ScriptSteps(steps...)
}

func TestAJournaledRunIsReplayedInsteadOfPaidForAgain(t *testing.T) {
	dir := t.TempDir()
	sc := testScenario("basic", wideBands)

	candidate, judge := answeringFakes()
	first, err := certifyOnce(t, dir, sc, candidate, judge)
	if err != nil {
		t.Fatalf("first certification: %v", err)
	}

	// Nothing here can answer. Every run of this record must come off the
	// journal or there is no record at all.
	refusingCandidate, refusingJudge := refusingFakes(t)
	second, err := certifyOnce(t, dir, sc, refusingCandidate, refusingJudge)
	if err != nil {
		t.Fatalf("a journaled run must be replayable without a single model call: %v", err)
	}
	if len(refusingCandidate.Calls()) != 0 || len(refusingJudge.Calls()) != 0 {
		t.Fatalf("the replay called the candidate %d time(s) and the judge %d time(s) — a replayed run pays for nothing",
			len(refusingCandidate.Calls()), len(refusingJudge.Calls()))
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("a record assembled from replayed runs must equal the one that measured them:\n first  %+v\n second %+v", first, second)
	}
}

func TestAJournaledRunIsNotReplayedWhenTheScenarioStampMoves(t *testing.T) {
	dir := t.TempDir()
	candidate, judge := answeringFakes()
	if _, err := certifyOnce(t, dir, testScenario("basic", wideBands), candidate, judge); err != nil {
		t.Fatalf("first certification: %v", err)
	}

	// A different rubric is a different grader request, so a different stamp:
	// the journaled runs measured a question this run is no longer asking.
	moved := testScenario("basic", wideBands)
	moved.Expect.Rubric = "Score higher for an answer that names a colour."
	refusingCandidate, refusingJudge := refusingFakes(t)
	if _, err := certifyOnce(t, dir, moved, refusingCandidate, refusingJudge); err == nil {
		t.Fatal("a scenario whose stamp moved must be measured again, not replayed from runs of the old one")
	}
	if len(refusingCandidate.Calls()) == 0 {
		t.Fatal("the run was never re-driven — it replayed a stamp that no longer describes this scenario")
	}
}

func TestAJournaledRunIsNotReplayedForADifferentCandidateBinding(t *testing.T) {
	dir := t.TempDir()
	sc := testScenario("basic", wideBands)
	candidate, judge := answeringFakes()
	if _, err := certifyOnce(t, dir, sc, candidate, judge); err != nil {
		t.Fatalf("first certification: %v", err)
	}

	// The same task, certified against another model — which is exactly what a
	// routed run does across its tasks. Nothing journaled under the first model
	// may stand in for it.
	other := ai.ProviderConfig{Provider: ai.ProviderFake, Model: "a-different-candidate"}
	if replayable(t, dir, fixedResumeNow, sc, other) {
		t.Fatal("a run measured on one model was offered as a replay for another")
	}
	if !replayable(t, dir, fixedResumeNow, sc, testCandidateBinding) {
		t.Fatal("the run measured on THIS model was not replayable — the binding key rejects too much")
	}
}

func TestAJournaledRunExpiresAfterTheResumeWindow(t *testing.T) {
	dir := t.TempDir()
	sc := testScenario("basic", wideBands)
	candidate, judge := answeringFakes()
	if _, err := certifyOnce(t, dir, sc, candidate, judge); err != nil {
		t.Fatalf("first certification: %v", err)
	}

	// The inside-the-window case first: opening a journal compacts it, so
	// checking expiry first would leave nothing behind to check freshness on.
	if !replayable(t, dir, fixedResumeNow.Add(resumeWindow-time.Minute), sc, testCandidateBinding) {
		t.Fatal("a run one minute inside the window was dropped — the expiry is off by a boundary")
	}
	if replayable(t, dir, fixedResumeNow.Add(resumeWindow), sc, testCandidateBinding) {
		t.Fatalf("a run exactly %v old was still offered as a replay", resumeWindow)
	}
}

func TestAJournalEndingMidLineKeepsTheRunsBeforeIt(t *testing.T) {
	dir := t.TempDir()
	sc := testScenario("basic", wideBands)
	candidate, judge := answeringFakes()
	if _, err := certifyOnce(t, dir, sc, candidate, judge); err != nil {
		t.Fatalf("first certification: %v", err)
	}

	// A process killed mid-append is the case this whole file exists for, so
	// its journal must still be readable up to the cut.
	path := filepath.Join(dir, "aicert-resume.jsonl")
	whole, err := os.ReadFile(path) // #nosec G304 -- a t.TempDir path
	if err != nil {
		t.Fatalf("reading the journal: %v", err)
	}
	truncated := append(append([]byte{}, whole...), []byte(`{"at":"2026-09-02T11:5`)...)
	if werr := os.WriteFile(path, truncated, 0o600); werr != nil {
		t.Fatalf("truncating the journal: %v", werr)
	}

	refusingCandidate, refusingJudge := refusingFakes(t)
	if _, err := certifyOnce(t, dir, sc, refusingCandidate, refusingJudge); err != nil {
		t.Fatalf("a half-written last line must not cost the whole journal: %v", err)
	}
	if len(refusingCandidate.Calls()) != 0 {
		t.Fatalf("the candidate was called %d time(s) — the runs written whole before the cut were discarded", len(refusingCandidate.Calls()))
	}
}

func TestCompactionKeepsAnotherJudgesLiveRuns(t *testing.T) {
	dir := t.TempDir()
	sc := testScenario("basic", wideBands)
	candidate, judge := answeringFakes()
	if _, err := certifyOnce(t, dir, sc, candidate, judge); err != nil {
		t.Fatalf("first certification: %v", err)
	}

	// A run on a different grader compacts the file. Those lines are somebody
	// else's still-good measurement, not this run's to throw away.
	otherJudge, err := openRunJournal(context.Background(), dir,
		ai.ProviderConfig{Provider: ai.ProviderFake, Model: "a-different-judge"}, ai.ProfileEUHosted, fixedResumeNow, quietLogger())
	if err != nil {
		t.Fatalf("openRunJournal on another judge: %v", err)
	}
	_, replayed := otherJudge.forTask(ai.TaskSummarize, testCandidateBinding).lookup(sc, stampFor(t, sc), 1)
	if cerr := otherJudge.close(); cerr != nil {
		t.Fatalf("closing the other judge's journal: %v", cerr)
	}
	if replayed {
		t.Fatal("a run graded by one judge was offered as a replay to another")
	}

	if !replayable(t, dir, fixedResumeNow, sc, testCandidateBinding) {
		t.Fatal("compaction under another judge deleted this one's live runs")
	}
}

func TestAnExpiredRunIsCompactedOutOfTheJournal(t *testing.T) {
	dir := t.TempDir()
	candidate, judge := answeringFakes()
	if _, err := certifyOnce(t, dir, testScenario("basic", wideBands), candidate, judge); err != nil {
		t.Fatalf("first certification: %v", err)
	}
	path := filepath.Join(dir, "aicert-resume.jsonl")
	before, err := os.ReadFile(path) // #nosec G304 -- a t.TempDir path
	if err != nil {
		t.Fatalf("reading the journal: %v", err)
	}
	if len(before) == 0 {
		t.Fatal("the first run journaled nothing, so this test would prove nothing about compacting it away")
	}

	withJournal(t, dir, fixedResumeNow.Add(resumeWindow), func(j *runJournal) {
		if j.Path == "" {
			t.Fatal("the journal reported no path")
		}
	})
	after, err := os.ReadFile(path) // #nosec G304 -- a t.TempDir path
	if err != nil {
		t.Fatalf("re-reading the journal: %v", err)
	}
	if len(after) != 0 {
		t.Fatalf("expired runs survived compaction: %s", after)
	}
}

// TestAJournaledRunCarriesEveryFieldOfARunOutcome is the round-trip census: a
// field added to runOutcome that JSON cannot carry would otherwise be replayed
// as its zero value, and a token bucket or a served identity silently zeroed is
// a record that reports a cheaper, faster run than the one that happened.
//
// The fully-populated value is built by hand and then CHECKED to have no zero
// field, so adding a field to runOutcome fails this test until the author
// decides what it should round-trip as.
func TestAJournaledRunCarriesEveryFieldOfARunOutcome(t *testing.T) {
	want := runOutcome{
		RunResult: RunResult{
			Output: "the widget is blue", Outcome: "accepted", LatencyMS: 1234,
			TokensIn: 11, TokensOut: 22, CachedTokens: 33, CacheWriteTokens: 44,
			Degraded: true, HardPass: true, Score: 87,
		},
		Provider: "openai_compatible", ServedModel: "z-ai/glm-5.2",
		ServedIdentitySource: "provider_reported", JudgeServedModel: "claude-haiku-4.5",
		CertifiedScope: "full_invocation", JudgeDegraded: true,
	}
	assertNoZeroField(t, reflect.ValueOf(want), "runOutcome")

	encoded, err := json.Marshal(journaledRun{Outcome: want})
	if err != nil {
		t.Fatalf("marshalling a journal line: %v", err)
	}
	var got journaledRun
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatalf("unmarshalling a journal line: %v", err)
	}
	if !reflect.DeepEqual(got.Outcome, want) {
		t.Fatalf("a run did not survive the journal:\n want %+v\n got  %+v", want, got.Outcome)
	}
}

// assertNoZeroField fails when any field of v — walking into embedded structs —
// is still its zero value, naming the one that is.
func assertNoZeroField(t *testing.T, v reflect.Value, path string) {
	t.Helper()
	for i := range v.NumField() {
		field, name := v.Field(i), path+"."+v.Type().Field(i).Name
		if field.Kind() == reflect.Struct {
			assertNoZeroField(t, field, name)
			continue
		}
		if field.IsZero() {
			t.Fatalf("%s is still its zero value — set it here so the journal round-trip actually covers it", name)
		}
	}
}

func TestReplayedRunsStillMeetTheServedIdentityGate(t *testing.T) {
	dir := t.TempDir()
	sc := testScenario("basic", wideBands)
	candidate, judge := answeringFakes()
	if _, err := certifyOnce(t, dir, sc, candidate, judge); err != nil {
		t.Fatalf("first certification: %v", err)
	}

	// Rewrite run 2 as though another model had served it. A record that pooled
	// this would name one model over scores two produced.
	path := filepath.Join(dir, "aicert-resume.jsonl")
	raw, err := os.ReadFile(path) // #nosec G304 -- a t.TempDir path
	if err != nil {
		t.Fatalf("reading the journal: %v", err)
	}
	lines := strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n")
	if len(lines) < 2 {
		t.Fatalf("want at least 2 journaled runs to make one of them diverge, got %d", len(lines))
	}
	var line journaledRun
	if err := json.Unmarshal([]byte(lines[1]), &line); err != nil {
		t.Fatalf("decoding the second journaled run: %v", err)
	}
	line.Outcome.ServedModel = "some-other-model"
	edited, err := json.Marshal(line)
	if err != nil {
		t.Fatalf("re-encoding the second journaled run: %v", err)
	}
	lines[1] = string(edited)
	if werr := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); werr != nil {
		t.Fatalf("rewriting the journal: %v", werr)
	}

	replayCandidate, replayJudge := answeringFakes()
	_, err = certifyOnce(t, dir, sc, replayCandidate, replayJudge)
	if err == nil {
		t.Fatal("a replayed run served by another model must void the record, exactly as a live one does")
	}
	if !strings.Contains(err.Error(), "refusing to certify a mixed run set") {
		t.Fatalf("error %q does not name the mixed run set it refused", err)
	}
}

func TestAJournaledRunIsNotReplayedUnderADifferentProfile(t *testing.T) {
	dir := t.TempDir()
	sc := testScenario("basic", wideBands)
	candidate, judge := answeringFakes()
	if _, err := certifyOnce(t, dir, sc, candidate, judge); err != nil {
		t.Fatalf("first certification: %v", err)
	}

	// The environment class is part of a record's IDENTITY — it is what the
	// record is filed under and what the binding is validated against — so a run
	// measured under one may never stand in for a run under another. Getting
	// this wrong does not cost a re-run; it publishes a certification of an
	// environment class nobody exercised.
	other, err := openRunJournal(context.Background(), dir, testJudgeBinding, ai.ProfileCloudFrontier, fixedResumeNow, quietLogger())
	if err != nil {
		t.Fatalf("openRunJournal under another profile: %v", err)
	}
	_, replayed := other.forTask(ai.TaskSummarize, testCandidateBinding).lookup(sc, stampFor(t, sc), 1)
	if cerr := other.close(); cerr != nil {
		t.Fatalf("closing the other profile's journal: %v", cerr)
	}
	if replayed {
		t.Fatalf("a run measured under %s was offered as a replay under %s", ai.ProfileEUHosted, ai.ProfileCloudFrontier)
	}
	if !replayable(t, dir, fixedResumeNow, sc, testCandidateBinding) {
		t.Fatal("opening under another profile deleted this one's live runs")
	}
}

func TestAJournaledRunIsNotReplayedForAnotherCorpusVersion(t *testing.T) {
	dir := t.TempDir()
	sc := testScenario("basic", wideBands)
	candidate, judge := answeringFakes()
	if _, err := certifyOnce(t, dir, sc, candidate, judge); err != nil {
		t.Fatalf("first certification: %v", err)
	}

	// A corpus format this build no longer speaks: the runs were scored against
	// scenarios read by other rules, so whatever they measured, it was not this.
	rewriteFirstJournaledRun(t, dir, func(line *journaledRun) { line.CorpusVersion = "v0" })

	if replayable(t, dir, fixedResumeNow, sc, testCandidateBinding) {
		t.Fatal("a run journaled under another corpus version was offered as a replay")
	}
}

func TestAReplayedRunStillMeetsTheDegradeGate(t *testing.T) {
	dir := t.TempDir()
	sc := testScenario("basic", wideBands)
	candidate, judge := answeringFakes()
	if _, err := certifyOnce(t, dir, sc, candidate, judge); err != nil {
		t.Fatalf("first certification: %v", err)
	}

	// Only a run that passed the gate is ever journaled, so the gate on the
	// replay path is reachable solely through a journal that says otherwise —
	// which is the case worth holding: the file is on disk, hand-editable, and a
	// demoted answer replayed into a record is a certification of a route the
	// record does not name.
	rewriteFirstJournaledRun(t, dir, func(line *journaledRun) { line.Outcome.Degraded = true })

	replayCandidate, replayJudge := answeringFakes()
	_, err := certifyOnce(t, dir, sc, replayCandidate, replayJudge)
	if err == nil {
		t.Fatal("a replayed run marked budget-degraded must void the record, exactly as a live one does")
	}
	if !strings.Contains(err.Error(), "refusing to certify a demoted answer") {
		t.Fatalf("error %q does not name the demoted answer it refused", err)
	}
}

// TestALookupAndAJournaledLineAgreeOnTheirKey holds resumeKeyFor's claim to be
// the one place a key is built. Two literals that agree today would let a field
// added to resumeKey be filled in on the append side only, and a lookup keyed on
// less than the append was filed under replays a run nobody asked for.
func TestALookupAndAJournaledLineAgreeOnTheirKey(t *testing.T) {
	line := journaledRun{
		Candidate: "fake|candidate", Task: string(ai.TaskSummarize),
		Scenario: "basic", Stamp: "stamp-a", Run: 2,
	}
	view := taskJournal{j: &runJournal{}, task: ai.TaskSummarize, candidate: line.Candidate}
	view.j.loaded = map[resumeKey]runOutcome{keyOf(line): {RunResult: RunResult{Score: 42}}}

	got, ok := view.lookup(Scenario{Name: line.Scenario}, line.Stamp, line.Run)
	if !ok {
		t.Fatal("a lookup did not find the line its own key was built from — the read and write sides disagree")
	}
	if got.Score != 42 {
		t.Fatalf("looked up score %d, want 42", got.Score)
	}
}

func TestAJournaledRunIsNotReplayedByADifferentBinary(t *testing.T) {
	dir := t.TempDir()
	sc := testScenario("basic", wideBands)
	candidate, judge := answeringFakes()
	if _, err := certifyOnce(t, dir, sc, candidate, judge); err != nil {
		t.Fatalf("first certification: %v", err)
	}

	// The scenario stamp covers the REQUESTS, never the code that judges the
	// replies: a tightened site validator, a tightened caps check or a changed
	// judge-reply parser all leave every stamp exactly where it was. Replaying
	// across that would certify this build on hard_pass values the previous one
	// produced — the one failure mode where a resumed record is not merely stale
	// but wrong about this build.
	rewriteFirstJournaledRun(t, dir, func(line *journaledRun) { line.Build = "some-other-binary" })

	if replayable(t, dir, fixedResumeNow, sc, testCandidateBinding) {
		t.Fatal("a run scored by a different binary was offered as a replay")
	}
}

func TestASecondRunIsRefusedTheResumeDirectoryRatherThanDestroyingIt(t *testing.T) {
	dir := t.TempDir()
	first, err := openRunJournal(context.Background(), dir, testJudgeBinding, ai.ProfileEUHosted, fixedResumeNow, quietLogger())
	if err != nil {
		t.Fatalf("openRunJournal: %v", err)
	}
	defer func() {
		if cerr := first.close(); cerr != nil {
			t.Errorf("closing the first journal: %v", cerr)
		}
	}()

	// Splitting a long paid corpus into parallel TASK= runs is the obvious
	// operator move, and both would share this directory. The second one's
	// compaction renames a fresh file over the first's, leaving the first
	// appending into an unlinked inode — every run it journals from then on is
	// written where nobody will read it, silently.
	_, second := openRunJournal(context.Background(), dir, testJudgeBinding, ai.ProfileEUHosted, fixedResumeNow, quietLogger())
	if second == nil {
		t.Fatal("a second run took the same resume directory — the first's journal is now write-only garbage")
	}
	if !strings.Contains(second.Error(), "RESUME=") {
		t.Fatalf("error %q does not tell the operator how to run anyway", second)
	}
}

func TestTheResumeDirectoryIsReleasedForTheNextRun(t *testing.T) {
	dir := t.TempDir()
	first, err := openRunJournal(context.Background(), dir, testJudgeBinding, ai.ProfileEUHosted, fixedResumeNow, quietLogger())
	if err != nil {
		t.Fatalf("openRunJournal: %v", err)
	}
	if cerr := first.close(); cerr != nil {
		t.Fatalf("closing the first journal: %v", cerr)
	}
	// A claim that outlived its run would make resuming a one-shot feature and
	// send every later run to the manual-cleanup message.
	second, err := openRunJournal(context.Background(), dir, testJudgeBinding, ai.ProfileEUHosted, fixedResumeNow, quietLogger())
	if err != nil {
		t.Fatalf("the resume directory was not released by the run that closed it: %v", err)
	}
	if cerr := second.close(); cerr != nil {
		t.Fatalf("closing the second journal: %v", cerr)
	}
}

// rewriteFirstJournaledRun edits the journal's first line in place, for a test
// that must make a journaled run diverge from what this run is asking for.
func rewriteFirstJournaledRun(t *testing.T, dir string, edit func(*journaledRun)) {
	t.Helper()
	path := filepath.Join(dir, "aicert-resume.jsonl")
	raw, err := os.ReadFile(path) // #nosec G304 -- a t.TempDir path
	if err != nil {
		t.Fatalf("reading the journal: %v", err)
	}
	lines := strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n")
	var line journaledRun
	if uerr := json.Unmarshal([]byte(lines[0]), &line); uerr != nil {
		t.Fatalf("decoding the first journaled run: %v", uerr)
	}
	edit(&line)
	edited, err := json.Marshal(line)
	if err != nil {
		t.Fatalf("re-encoding the first journaled run: %v", err)
	}
	lines[0] = string(edited)
	if werr := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); werr != nil {
		t.Fatalf("rewriting the journal: %v", werr)
	}
}

// stampFor is the scenario stamp this build computes for sc — the same value
// certifyTask files a journal line under.
func stampFor(t *testing.T, sc Scenario) string {
	t.Helper()
	stamps, err := ScenarioStamps(wsContext(t), []Scenario{sc}, testCensus(t))
	if err != nil {
		t.Fatalf("ScenarioStamps: %v", err)
	}
	return stamps[sc.Name]
}
