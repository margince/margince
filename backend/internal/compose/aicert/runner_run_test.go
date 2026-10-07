// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert_test

// Black-box tests over the public Run() entry point: a real corpus on
// disk, a real routing file, records written to and read back from
// disk. certifyTask's own (white-box, same-package) tests already pin
// the scored verdict math against a scripted fake; these tests instead
// pin Run's OWN plumbing — corpus loading, task filtering, repeats
// validation, and record I/O — against the offline fake provider's
// UNSCRIPTED fallback, which is itself fully deterministic (a stable
// hash of the request payload, always prefixed "fake-completion:"), so
// an expected answer of "fake-completion" is a reliable, script-free
// HardPass signal. The judge side of that same unscripted
// fallback is never valid JSON, so every run here is left ungraded and
// every verdict is not_supported — Run() has no seam to script the judge, unlike
// certifyTask's own tests, so this is the honest ceiling of a pure
// black-box run.

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/aicert"
	"github.com/margince/margince/backend/internal/modules/ai"
)

// scenarioYAML builds one minimal, always-"basic"-named scenario for
// task — every call site in this file names a different task, never a
// different scenario name.
func scenarioYAML(task string) string {
	return `
name: basic
task: ` + task + `
site: ` + stubVariant + `
source: hand_authored
sanitized_by: tester
fixture:
  subject: Describe the widget.
expect:
  outcome: accepted
  answer: fake-completion
  rubric: Score higher for a longer, on-topic answer.
  bands:
    certified_min: 70
    degraded_min: 50
    floor: 40
`
}

func quietTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestRunWritesOneRecordPerTaskAndItLoadsBackIdentically(t *testing.T) {
	dir := t.TempDir()
	corpusDir := filepath.Join(dir, "corpus")
	recordDir := filepath.Join(dir, "records")
	writeCorpusFile(t, corpusDir, "summarize/basic_01.yaml", scenarioYAML("summarize"))

	records, err := aicert.Run(context.Background(), aicert.RunnerConfig{
		Census:       censusFor(t, ai.TaskSummarize, ai.TaskColdStart),
		Binding:      ai.ProviderConfig{Provider: ai.ProviderFake, Model: "candidate"},
		JudgeBinding: ai.ProviderConfig{Provider: ai.ProviderFake, Model: "judge"},
		Profile:      ai.ProfileEUHosted,
		CorpusDir:    corpusDir,
		RecordDir:    recordDir,
		Repeats:      3,
	}, quietTestLogger())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("got %d records, want 1", len(records))
	}
	rec := records[0]
	if rec.Task != "summarize" || rec.Provider != "fake" || rec.ServedModel != "fake" {
		t.Fatalf("record identity wrong: %+v", rec)
	}
	// One case right three of three is borderline, so it runs to its cap.
	if rec.Runs != aicert.AdaptiveMaxRuns || rec.Reliability != 1 {
		t.Fatalf("every run's output contains the required substring — want runs=%d reliability=1, got %+v", aicert.AdaptiveMaxRuns, rec)
	}
	if rec.PromptVersion == "" || rec.CorpusVersion == "" {
		t.Fatalf("prompt/corpus version must be stamped, got %+v", rec)
	}
	if _, err := time.Parse(time.RFC3339, rec.RanAt); err != nil {
		t.Fatalf("ran_at %q is not RFC3339: %v", rec.RanAt, err)
	}

	loaded, err := aicert.LoadRecords(recordDir)
	if err != nil {
		t.Fatalf("LoadRecords: %v", err)
	}
	if len(loaded) != 1 || !reflect.DeepEqual(loaded[0], rec) {
		t.Fatalf("LoadRecords round-trip mismatch: wrote %+v, loaded %+v", rec, loaded)
	}
}

func TestRunTaskFilterRestrictsCertificationToOneTask(t *testing.T) {
	dir := t.TempDir()
	corpusDir := filepath.Join(dir, "corpus")
	writeCorpusFile(t, corpusDir, "summarize/basic_01.yaml", scenarioYAML("summarize"))
	writeCorpusFile(t, corpusDir, "cold_start/basic_01.yaml", scenarioYAML("cold_start"))

	records, err := aicert.Run(context.Background(), aicert.RunnerConfig{
		Census:       censusFor(t, ai.TaskSummarize, ai.TaskColdStart),
		Binding:      ai.ProviderConfig{Provider: ai.ProviderFake, Model: "candidate"},
		JudgeBinding: ai.ProviderConfig{Provider: ai.ProviderFake, Model: "judge"},
		Profile:      ai.ProfileEUHosted,
		CorpusDir:    corpusDir,
		RecordDir:    filepath.Join(dir, "records"),
		TaskFilter:   "cold_start",
		Repeats:      1,
	}, quietTestLogger())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(records) != 1 || records[0].Task != "cold_start" {
		t.Fatalf("TaskFilter must restrict to exactly cold_start, got %+v", records)
	}
}

func TestRunUnknownTaskFilterFailsLoudly(t *testing.T) {
	dir := t.TempDir()
	corpusDir := filepath.Join(dir, "corpus")
	writeCorpusFile(t, corpusDir, "summarize/basic_01.yaml", scenarioYAML("summarize"))

	_, err := aicert.Run(context.Background(), aicert.RunnerConfig{
		Census:       censusFor(t, ai.TaskSummarize, ai.TaskColdStart),
		Binding:      ai.ProviderConfig{Provider: ai.ProviderFake, Model: "candidate"},
		JudgeBinding: ai.ProviderConfig{Provider: ai.ProviderFake, Model: "judge"},
		Profile:      ai.ProfileEUHosted,
		CorpusDir:    corpusDir,
		RecordDir:    filepath.Join(dir, "records"),
		TaskFilter:   "offer_draft",
		Repeats:      1,
	}, quietTestLogger())
	if err == nil || !strings.Contains(err.Error(), "offer_draft") {
		t.Fatalf("want an error naming the unmatched task filter, got %v", err)
	}
}

func TestRunRejectsANegativeRepeatsBeforeTouchingAnything(t *testing.T) {
	dir := t.TempDir()
	corpusDir := filepath.Join(dir, "corpus")
	writeCorpusFile(t, corpusDir, "summarize/basic_01.yaml", scenarioYAML("summarize"))

	_, err := aicert.Run(context.Background(), aicert.RunnerConfig{
		Census:       censusFor(t, ai.TaskSummarize, ai.TaskColdStart),
		Binding:      ai.ProviderConfig{Provider: ai.ProviderFake, Model: "candidate"},
		JudgeBinding: ai.ProviderConfig{Provider: ai.ProviderFake, Model: "judge"},
		Profile:      ai.ProfileEUHosted,
		CorpusDir:    corpusDir,
		RecordDir:    filepath.Join(dir, "records"),
		Repeats:      -1,
	}, quietTestLogger())
	if err == nil || !strings.Contains(err.Error(), "positive") {
		t.Fatalf("want a non-positive-repeats complaint, got %v", err)
	}
}

// TestRunWritesTaskARecordAndSurfacesTaskBsWriteErrorInTheSameCall proves
// the "one task fails, its sibling still gets recorded, in the same
// Run() call" property that TestRunAnUnrunnableBindingJoinsAnErrorPerTaskAndAbortsNone
// cannot: a malformed override fails every task identically, so it can
// never show one task succeeding alongside another failing in one Run.
// Here both tasks certify cleanly, but a plain FILE pre-created at the
// exact path WriteRecord needs as a directory for "summarize" makes its
// own os.MkdirAll fail — for that task only. "cold_start" sorts first
// (sortedTasks is alphabetical) and its own records/cold_start directory
// is untouched, so this proves both halves of the contract in one call:
// cold_start's record is written, and summarize's write error is heard
// (errors.Join), not swallowed.
func TestRunWritesTaskARecordAndSurfacesTaskBsWriteErrorInTheSameCall(t *testing.T) {
	dir := t.TempDir()
	corpusDir := filepath.Join(dir, "corpus")
	recordDir := filepath.Join(dir, "records")
	writeCorpusFile(t, corpusDir, "cold_start/basic_01.yaml", scenarioYAML("cold_start"))
	writeCorpusFile(t, corpusDir, "summarize/basic_01.yaml", scenarioYAML("summarize"))

	if err := os.MkdirAll(recordDir, 0o750); err != nil {
		t.Fatalf("pre-creating the records dir: %v", err)
	}
	// WriteRecord's own recordPath for "summarize" is
	// records/summarize/<file>.json; MkdirAll needs records/summarize to
	// be a directory (or absent) — a plain file occupying that exact
	// path makes MkdirAll fail for summarize only.
	if err := os.WriteFile(filepath.Join(recordDir, "summarize"), []byte("occupied"), 0o600); err != nil {
		t.Fatalf("pre-creating the blocking file: %v", err)
	}

	records, err := aicert.Run(context.Background(), aicert.RunnerConfig{
		Census:       censusFor(t, ai.TaskSummarize, ai.TaskColdStart),
		Binding:      ai.ProviderConfig{Provider: ai.ProviderFake, Model: "candidate"},
		JudgeBinding: ai.ProviderConfig{Provider: ai.ProviderFake, Model: "judge"},
		Profile:      ai.ProfileEUHosted,
		CorpusDir:    corpusDir,
		RecordDir:    recordDir,
		Repeats:      1,
	}, quietTestLogger())

	if len(records) != 1 || records[0].Task != "cold_start" {
		t.Fatalf("want exactly cold_start's record written despite summarize's write failure, got %+v", records)
	}
	if err == nil || !strings.Contains(err.Error(), "summarize") {
		t.Fatalf("want an error naming summarize's write failure, got %v", err)
	}
}

// TestRunAnUnrunnableBindingJoinsAnErrorPerTaskAndAbortsNone proves the
// "heard, never swallowed" contract on the error path every task
// actually reaches: a cloud vendor under a sovereign profile is refused
// identically for every task in the corpus (each task's own certifyTask
// call refuses it independently), and Run reports every one of them —
// via errors.Join, not just the first — rather than stopping at the
// first failure.
func TestRunAnUnrunnableBindingJoinsAnErrorPerTaskAndAbortsNone(t *testing.T) {
	dir := t.TempDir()
	corpusDir := filepath.Join(dir, "corpus")
	writeCorpusFile(t, corpusDir, "summarize/basic_01.yaml", scenarioYAML("summarize"))
	writeCorpusFile(t, corpusDir, "cold_start/basic_01.yaml", scenarioYAML("cold_start"))

	records, err := aicert.Run(context.Background(), aicert.RunnerConfig{
		Census: censusFor(t, ai.TaskSummarize, ai.TaskColdStart),
		// A cloud vendor under a sovereign profile: refused per task, which is
		// what makes this the errors.Join case rather than an early return.
		Binding:      ai.ProviderConfig{Provider: "anthropic", Model: "claude-cert-test"},
		JudgeBinding: ai.ProviderConfig{Provider: ai.ProviderFake, Model: "judge"},
		Profile:      ai.ProfileSovereign,
		CorpusDir:    corpusDir,
		RecordDir:    filepath.Join(dir, "records"),
		Repeats:      1,
	}, quietTestLogger())
	if len(records) != 0 {
		t.Fatalf("an unrunnable binding must certify nothing, got %+v", records)
	}
	if err == nil {
		t.Fatal("want a non-nil error")
	}
	for _, task := range []string{"summarize", "cold_start"} {
		if !strings.Contains(err.Error(), task) {
			t.Errorf("joined error must name task %s, got %v", task, err)
		}
	}
}

// A routed run certifies tasks against different candidates, and ONE judge
// grades all of them: the journal files every task's runs under that judge.
func TestARoutedRunGradesEveryTaskWithTheOneJudge(t *testing.T) {
	local, premium := ai.TaskCaptureConfidentialityVerdict, ai.TaskDocumentExtract
	premiumLead := ai.TaskLadder(premium)[0]
	if ai.TaskLadder(local)[0] == premiumLead {
		t.Fatalf("%s and %s lead on the same rung, so they cannot resolve to different candidates", local, premium)
	}
	routing := ai.RoutingConfig{Profile: ai.ProfileEUHosted, Tiers: map[ai.Tier]ai.ProviderConfig{}}
	for _, tier := range ai.AllTiers() {
		routing.Tiers[tier] = ai.ProviderConfig{Provider: ai.ProviderFake, Model: "candidate-a"}
	}
	routing.Tiers[premiumLead] = ai.ProviderConfig{Provider: ai.ProviderFake, Model: "candidate-b"}

	dir := t.TempDir()
	corpusDir := filepath.Join(dir, "corpus")
	resumeDir := filepath.Join(dir, "resume")
	for _, task := range []ai.Task{local, premium} {
		writeCorpusFile(t, corpusDir, string(task)+"/basic_01.yaml", scenarioYAML(string(task)))
	}
	records, err := aicert.Run(context.Background(), aicert.RunnerConfig{
		Census:       censusFor(t, local, premium),
		Routing:      &routing,
		JudgeBinding: ai.ProviderConfig{Provider: ai.ProviderFake, Model: "grader"},
		CorpusDir:    corpusDir,
		RecordDir:    filepath.Join(dir, "records"),
		ResumeDir:    resumeDir,
		Repeats:      1,
	}, quietTestLogger())
	if err != nil {
		t.Fatalf("a judge distinct from every candidate must let the run proceed, got %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("got %d records, want one per task", len(records))
	}
	journal, err := os.ReadFile(filepath.Join(resumeDir, "aicert-resume.jsonl")) // #nosec G304 -- a t.TempDir path
	if err != nil {
		t.Fatalf("reading the resume journal: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(journal)), "\n")
	for _, candidate := range []string{"candidate-a", "candidate-b"} {
		if !strings.Contains(string(journal), `"candidate":"\"fake\"|\"`+candidate+`\"`) {
			t.Errorf("no run was certified against %s — the tasks did not resolve to their own rungs:\n%s", candidate, journal)
		}
	}
	for _, line := range lines {
		if !strings.Contains(line, `"judge":"\"fake\"|\"grader\"`) {
			t.Errorf("a run was graded by something other than the one judge:\n%s", line)
		}
	}
}

// A routed TASK= run certifies one task, so only that task's candidate can collide
// with the judge: a collision elsewhere in the routing refuses the run naming it,
// never the run that does not.
func TestARoutedRunValidatesOnlyTheTasksItCertifies(t *testing.T) {
	ranked, colliding := ai.TaskCaptureConfidentialityVerdict, ai.TaskDocumentExtract
	judge := ai.ProviderConfig{Provider: ai.ProviderFake, Model: "judge"}
	routing := ai.RoutingConfig{Profile: ai.ProfileEUHosted, Tiers: map[ai.Tier]ai.ProviderConfig{}}
	for _, tier := range ai.AllTiers() {
		routing.Tiers[tier] = ai.ProviderConfig{Provider: ai.ProviderFake, Model: "candidate"}
	}
	collidingLead := ai.TaskLadder(colliding)[0]
	if ai.TaskLadder(ranked)[0] == collidingLead {
		t.Fatalf("%s and %s lead on the same rung, so one cannot collide without the other", ranked, colliding)
	}
	routing.Tiers[collidingLead] = judge

	dir := t.TempDir()
	corpusDir := filepath.Join(dir, "corpus")
	for _, task := range []ai.Task{ranked, colliding} {
		writeCorpusFile(t, corpusDir, string(task)+"/basic_01.yaml", scenarioYAML(string(task)))
	}
	run := func(task ai.Task) ([]aicert.Record, error) {
		return aicert.Run(context.Background(), aicert.RunnerConfig{
			Census:       censusFor(t, ranked, colliding),
			Routing:      &routing,
			JudgeBinding: judge,
			CorpusDir:    corpusDir,
			RecordDir:    filepath.Join(dir, "records"),
			TaskFilter:   string(task),
			Repeats:      1,
		}, quietTestLogger())
	}

	records, err := run(ranked)
	if err != nil {
		t.Fatalf("TASK=%s was refused over %s, a task it does not certify: %v", ranked, colliding, err)
	}
	if len(records) != 1 || records[0].Task != string(ranked) {
		t.Fatalf("TASK=%s wrote %+v, want exactly its own record", ranked, records)
	}
	_, err = run(colliding)
	if err == nil || !strings.Contains(err.Error(), string(colliding)) {
		t.Fatalf("TASK=%s grades itself and must be refused naming it; got %v", colliding, err)
	}
}

// A restart that will replay every run from the journal sends no scenario, so a
// pre-flight would be the one paid call it makes. It is skipped, and said so.
func TestARunThatReplaysEveryRunSkipsThePreflight(t *testing.T) {
	task := ai.TaskCaptureConfidentialityVerdict
	dir := t.TempDir()
	corpusDir := filepath.Join(dir, "corpus")
	writeCorpusFile(t, corpusDir, string(task)+"/basic_01.yaml", scenarioYAML(string(task)))
	run := func() string {
		var out strings.Builder
		if _, err := aicert.Run(context.Background(), aicert.RunnerConfig{
			Census:       censusFor(t, task),
			Binding:      ai.ProviderConfig{Provider: ai.ProviderFake, Model: "candidate"},
			JudgeBinding: ai.ProviderConfig{Provider: ai.ProviderFake, Model: "grader"},
			Profile:      ai.ProfileEUHosted,
			CorpusDir:    corpusDir,
			RecordDir:    filepath.Join(dir, "records"),
			ResumeDir:    filepath.Join(dir, "resume"),
			Repeats:      1,
		}, slog.New(slog.NewTextHandler(&out, nil))); err != nil {
			t.Fatalf("certifying: %v", err)
		}
		return out.String()
	}
	const served = "aicert: pre-flight served"
	if first := run(); !strings.Contains(first, served) {
		t.Fatalf("the first run sent no pre-flight, so the second proves nothing about skipping one:\n%s", first)
	}
	if second := run(); strings.Contains(second, served) || !strings.Contains(second, "pre-flight skipped") {
		t.Errorf("a run replaying every run from the journal still paid for a pre-flight, or did not say it skipped one:\n%s", second)
	}
}

// A routed run writes the answering rung's record before it tries the fallback,
// so a fallback that cannot run costs its own record and names itself, never
// the record of the model a buyer is answered by.
func TestAFailingFallbackKeepsTheAnsweringRungsRecord(t *testing.T) {
	ladder := ai.TaskLadder(ai.TaskSummarize)
	if len(ladder) != 2 {
		t.Fatalf("%s's ladder is %v; this test needs two rungs", ai.TaskSummarize, ladder)
	}
	dir := t.TempDir()
	corpusDir := filepath.Join(dir, "corpus")
	writeCorpusFile(t, corpusDir, "summarize/basic_01.yaml", scenarioYAML("summarize"))
	routing := ai.RoutingConfig{Profile: ai.ProfileCloudFrontier, Tiers: map[ai.Tier]ai.ProviderConfig{
		ladder[0]: {Provider: ai.ProviderFake, Model: "answers"},
		// openai_compatible fails closed without a base_url: a fallback that cannot run.
		ladder[1]: {Provider: "openai_compatible", Model: "vendor/unreachable"},
	}}
	records, err := aicert.Run(context.Background(), aicert.RunnerConfig{
		Census:       censusFor(t, ai.TaskSummarize),
		Routing:      &routing,
		JudgeBinding: ai.ProviderConfig{Provider: ai.ProviderFake, Model: "grader"},
		CorpusDir:    corpusDir,
		RecordDir:    filepath.Join(dir, "records"),
		Repeats:      1,
	}, quietTestLogger())
	if len(records) != 1 || records[0].Provider != ai.ProviderFake {
		t.Fatalf("records = %+v, want the answering rung's alone", records)
	}
	onDisk, loadErr := aicert.LoadRecords(filepath.Join(dir, "records"))
	if loadErr != nil || len(onDisk) != 1 {
		t.Fatalf("records on disk = %d (%v), want the answering rung's record kept", len(onDisk), loadErr)
	}
	if err == nil || !strings.Contains(err.Error(), "vendor/unreachable") {
		t.Errorf("err = %v, want the fallback named", err)
	}
}

// staleOnlyRun certifies summarize on the offline fake into dir, with STALE_ONLY
// as given. The binding names the fake's own served identity, so its record
// grades the binding it measured.
func staleOnlyRun(t *testing.T, dir string, staleOnly bool) ([]aicert.Record, error) {
	t.Helper()
	return aicert.Run(context.Background(), aicert.RunnerConfig{
		Census:       censusFor(t, ai.TaskSummarize),
		Binding:      ai.ProviderConfig{Provider: ai.ProviderFake, Model: "fake"},
		JudgeBinding: ai.ProviderConfig{Provider: ai.ProviderFake, Model: "grader"},
		Profile:      ai.ProfileCloudFrontier,
		CorpusDir:    filepath.Join(dir, "corpus"),
		RecordDir:    filepath.Join(dir, "records"),
		Repeats:      1,
		StaleOnly:    staleOnly,
	}, quietTestLogger())
}

// A STALE_ONLY run measures only what is missing or stale: a record current for
// this build is left alone, so a sweep pays for what changed and nothing else.
func TestAStaleOnlyRunMeasuresOnlyWhatIsMissingOrStale(t *testing.T) {
	dir := t.TempDir()
	writeCorpusFile(t, filepath.Join(dir, "corpus"), "summarize/basic_01.yaml", scenarioYAML("summarize"))
	if first, err := staleOnlyRun(t, dir, true); err != nil || len(first) != 1 {
		t.Fatalf("the first run wrote %d record(s) (%v), want one", len(first), err)
	}
	if again, err := staleOnlyRun(t, dir, true); err != nil || len(again) != 0 {
		t.Fatalf("a run over a current record certified %d (%v), want none", len(again), err)
	}
	writeCorpusFile(t, filepath.Join(dir, "corpus"), "summarize/basic_01.yaml",
		strings.Replace(scenarioYAML("summarize"), "Describe the widget.", "Describe the gadget.", 1))
	if stale, err := staleOnlyRun(t, dir, true); err != nil || len(stale) != 1 {
		t.Fatalf("after the scenario changed the run certified %d (%v), want the record re-measured", len(stale), err)
	}
	writeCorpusFile(t, filepath.Join(dir, "corpus"), "summarize/grown_02.yaml",
		strings.Replace(scenarioYAML("summarize"), "name: basic", "name: grown", 1))
	if partial, err := staleOnlyRun(t, dir, true); err != nil || len(partial) != 1 {
		t.Fatalf("after the corpus grew a case the run certified %d (%v), want the partial record re-measured", len(partial), err)
	}
	if err := os.RemoveAll(filepath.Join(dir, "records")); err != nil {
		t.Fatal(err)
	}
	if missing, err := staleOnlyRun(t, dir, true); err != nil || len(missing) != 1 {
		t.Fatalf("with no record the run certified %d (%v), want one", len(missing), err)
	}
}

// STALE_ONLY=0 re-measures a current record: a same-prompt variance check asks
// for exactly that.
func TestStaleOnlyOffMeasuresACurrentRecord(t *testing.T) {
	dir := t.TempDir()
	writeCorpusFile(t, filepath.Join(dir, "corpus"), "summarize/basic_01.yaml", scenarioYAML("summarize"))
	if _, err := staleOnlyRun(t, dir, false); err != nil {
		t.Fatal(err)
	}
	if again, err := staleOnlyRun(t, dir, false); err != nil || len(again) != 1 {
		t.Fatalf("with STALE_ONLY off the run certified %d (%v), want the record again", len(again), err)
	}
}

// The preset report reads a record the run just wrote as current, and the rung
// no run measured as absent: the same judgement STALE_ONLY skips on.
func TestThePresetReportReadsTheRecordARunWrote(t *testing.T) {
	dir := t.TempDir()
	corpusDir := filepath.Join(dir, "corpus")
	writeCorpusFile(t, corpusDir, "summarize/basic_01.yaml", scenarioYAML("summarize"))
	if _, err := staleOnlyRun(t, dir, true); err != nil {
		t.Fatal(err)
	}
	census := censusFor(t, ai.TaskSummarize)
	corpus, err := aicert.LoadCorpus(corpusDir, census)
	if err != nil {
		t.Fatal(err)
	}
	records, err := aicert.LoadRecords(filepath.Join(dir, "records"))
	if err != nil {
		t.Fatal(err)
	}
	ladder := ai.TaskLadder(ai.TaskSummarize)
	routing := ai.RoutingConfig{Profile: ai.ProfileCloudFrontier, Tiers: map[ai.Tier]ai.ProviderConfig{
		ladder[0]: {Provider: ai.ProviderFake, Model: "fake"},
		ladder[1]: {Provider: ai.ProviderFake, Model: "unmeasured"},
	}}
	rungs, err := aicert.PresetRungs(context.Background(), routing, corpus, census, records)
	if err != nil {
		t.Fatal(err)
	}
	if len(rungs) != 1 || rungs[0].FirstState != aicert.StatusCurrent || rungs[0].FallbackState != aicert.StatusAbsent {
		t.Fatalf("rungs = %+v, want summarize current on its first rung and absent on its fallback", rungs)
	}
}

// A fallback the judge cannot grade is named before anything is spent and again
// when the run ends, so an operator reading either end of a long log sees it.
func TestASkippedFallbackIsNamedAtTheStartAndInTheClosingSummary(t *testing.T) {
	dir := t.TempDir()
	writeCorpusFile(t, filepath.Join(dir, "corpus"), "summarize/basic_01.yaml", scenarioYAML("summarize"))
	ladder := ai.TaskLadder(ai.TaskSummarize)
	judge := ai.ProviderConfig{Provider: ai.ProviderFake, Model: "grader"}
	routing := ai.RoutingConfig{Profile: ai.ProfileCloudFrontier, Tiers: map[ai.Tier]ai.ProviderConfig{
		ladder[0]: {Provider: ai.ProviderFake, Model: "fake"}, ladder[1]: judge,
	}}
	var logged strings.Builder
	_, err := aicert.Run(context.Background(), aicert.RunnerConfig{
		Census: censusFor(t, ai.TaskSummarize), Routing: &routing, JudgeBinding: judge,
		CorpusDir: filepath.Join(dir, "corpus"), RecordDir: filepath.Join(dir, "records"), Repeats: 1,
	}, slog.New(slog.NewTextHandler(&logged, nil)))
	if err != nil {
		t.Fatal(err)
	}
	out := logged.String()
	first, certifying := strings.Index(out, "skipped fallback"), strings.Index(out, "aicert: certifying")
	if first < 0 || certifying < 0 || first > certifying {
		t.Errorf("the skip must be logged before the first certification:\n%s", out)
	}
	if !strings.Contains(out[certifying:], "fallbacks not measured") {
		t.Errorf("the run's closing summary does not list the skipped fallback:\n%s", out)
	}
}
