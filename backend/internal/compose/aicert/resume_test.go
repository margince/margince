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

// openTestJournal opens a journal in dir on this run's own bindings.
func openTestJournal(t *testing.T, dir string, now time.Time) *runJournal {
	t.Helper()
	j, err := openRunJournal(context.Background(), dir, testJudgeBinding, ai.ProfileEUHosted, now, quietLogger())
	if err != nil {
		t.Fatalf("openRunJournal: %v", err)
	}
	t.Cleanup(func() {
		if cerr := j.close(); cerr != nil {
			t.Errorf("closing the journal: %v", cerr)
		}
	})
	return j
}

// certifyWithJournal runs one certification of testScenario over the given
// scripted clients, through journal.
func certifyWithJournal(t *testing.T, journal *runJournal, sc Scenario, candidate, judge *ai.FakeClient) (Record, error) {
	t.Helper()
	withFixedNow(t, fixedResumeNow)
	// A test that reaches the re-drive path must not wait out its backoff.
	recordSleeps(t)
	return certifyTask(wsContext(t), ai.TaskSummarize, []Scenario{sc}, testCensus(t),
		testCandidateBinding, testJudgeBinding, ai.ProfileEUHosted, 3, quietLogger(), &certifyHooks{
			candidateOpts: []ai.LocalOption{ai.WithFakeClient(candidate)},
			judgeOpts:     []ai.LocalOption{ai.WithFakeClient(judge)},
			journal:       journal.forTask(ai.TaskSummarize, testCandidateBinding),
		})
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
	first, err := certifyWithJournal(t, openTestJournal(t, dir, fixedResumeNow), sc, candidate, judge)
	if err != nil {
		t.Fatalf("first certification: %v", err)
	}

	// Nothing here can answer. Every run of this record must come off the
	// journal or there is no record at all.
	refusingCandidate, refusingJudge := refusingFakes(t)
	second, err := certifyWithJournal(t, openTestJournal(t, dir, fixedResumeNow), sc, refusingCandidate, refusingJudge)
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
	if _, err := certifyWithJournal(t, openTestJournal(t, dir, fixedResumeNow), testScenario("basic", wideBands), candidate, judge); err != nil {
		t.Fatalf("first certification: %v", err)
	}

	// A different rubric is a different grader request, so a different stamp:
	// the journaled runs measured a question this run is no longer asking.
	moved := testScenario("basic", wideBands)
	moved.Expect.Rubric = "Score higher for an answer that names a colour."
	refusingCandidate, refusingJudge := refusingFakes(t)
	if _, err := certifyWithJournal(t, openTestJournal(t, dir, fixedResumeNow), moved, refusingCandidate, refusingJudge); err == nil {
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
	if _, err := certifyWithJournal(t, openTestJournal(t, dir, fixedResumeNow), sc, candidate, judge); err != nil {
		t.Fatalf("first certification: %v", err)
	}

	// The same task, certified against another model — which is exactly what a
	// routed run does across its tasks. Nothing journaled under the first model
	// may stand in for it.
	other := ai.ProviderConfig{Provider: ai.ProviderFake, Model: "a-different-candidate"}
	journal := openTestJournal(t, dir, fixedResumeNow)
	if _, ok := journal.forTask(ai.TaskSummarize, other).lookup(sc, stampFor(t, sc), 1); ok {
		t.Fatal("a run measured on one model was offered as a replay for another")
	}
	if _, ok := journal.forTask(ai.TaskSummarize, testCandidateBinding).lookup(sc, stampFor(t, sc), 1); !ok {
		t.Fatal("the run measured on THIS model was not replayable — the binding key rejects too much")
	}
}

func TestAJournaledRunExpiresAfterTheResumeWindow(t *testing.T) {
	dir := t.TempDir()
	sc := testScenario("basic", wideBands)
	candidate, judge := answeringFakes()
	if _, err := certifyWithJournal(t, openTestJournal(t, dir, fixedResumeNow), sc, candidate, judge); err != nil {
		t.Fatalf("first certification: %v", err)
	}

	// The inside-the-window case first: opening a journal compacts it, so
	// checking expiry first would leave nothing behind to check freshness on.
	fresh := openTestJournal(t, dir, fixedResumeNow.Add(resumeWindow-time.Minute))
	if _, ok := fresh.forTask(ai.TaskSummarize, testCandidateBinding).lookup(sc, stampFor(t, sc), 1); !ok {
		t.Fatal("a run one minute inside the window was dropped — the expiry is off by a boundary")
	}
	stale := openTestJournal(t, dir, fixedResumeNow.Add(resumeWindow))
	if _, ok := stale.forTask(ai.TaskSummarize, testCandidateBinding).lookup(sc, stampFor(t, sc), 1); ok {
		t.Fatalf("a run exactly %v old was still offered as a replay", resumeWindow)
	}
}

func TestAJournalEndingMidLineKeepsTheRunsBeforeIt(t *testing.T) {
	dir := t.TempDir()
	sc := testScenario("basic", wideBands)
	candidate, judge := answeringFakes()
	if _, err := certifyWithJournal(t, openTestJournal(t, dir, fixedResumeNow), sc, candidate, judge); err != nil {
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
	if _, err := certifyWithJournal(t, openTestJournal(t, dir, fixedResumeNow), sc, refusingCandidate, refusingJudge); err != nil {
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
	if _, err := certifyWithJournal(t, openTestJournal(t, dir, fixedResumeNow), sc, candidate, judge); err != nil {
		t.Fatalf("first certification: %v", err)
	}

	// A run on a different grader compacts the file. Those lines are somebody
	// else's still-good measurement, not this run's to throw away.
	otherJudge, err := openRunJournal(context.Background(), dir,
		ai.ProviderConfig{Provider: ai.ProviderFake, Model: "a-different-judge"}, ai.ProfileEUHosted, fixedResumeNow, quietLogger())
	if err != nil {
		t.Fatalf("openRunJournal on another judge: %v", err)
	}
	if _, ok := otherJudge.forTask(ai.TaskSummarize, testCandidateBinding).lookup(sc, stampFor(t, sc), 1); ok {
		t.Fatal("a run graded by one judge was offered as a replay to another")
	}
	if cerr := otherJudge.close(); cerr != nil {
		t.Fatalf("closing the other judge's journal: %v", cerr)
	}

	back := openTestJournal(t, dir, fixedResumeNow)
	if _, ok := back.forTask(ai.TaskSummarize, testCandidateBinding).lookup(sc, stampFor(t, sc), 1); !ok {
		t.Fatal("compaction under another judge deleted this one's live runs")
	}
}

func TestAnExpiredRunIsCompactedOutOfTheJournal(t *testing.T) {
	dir := t.TempDir()
	candidate, judge := answeringFakes()
	if _, err := certifyWithJournal(t, openTestJournal(t, dir, fixedResumeNow), testScenario("basic", wideBands), candidate, judge); err != nil {
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

	expired := openTestJournal(t, dir, fixedResumeNow.Add(resumeWindow))
	if expired.Path == "" {
		t.Fatal("the journal reported no path")
	}
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
	if _, err := certifyWithJournal(t, openTestJournal(t, dir, fixedResumeNow), sc, candidate, judge); err != nil {
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
	_, err = certifyWithJournal(t, openTestJournal(t, dir, fixedResumeNow), sc, replayCandidate, replayJudge)
	if err == nil {
		t.Fatal("a replayed run served by another model must void the record, exactly as a live one does")
	}
	if !strings.Contains(err.Error(), "refusing to certify a mixed run set") {
		t.Fatalf("error %q does not name the mixed run set it refused", err)
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
