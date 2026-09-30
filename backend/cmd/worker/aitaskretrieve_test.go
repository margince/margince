// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

// What `aitask retrieve` does without a model and without a database: what it
// refuses, what it chunks, what it pays for twice, and what it writes down.
//
// The one thing NOT covered here is whether the in-process ranking agrees with
// the SQL — that needs Postgres, and it is the mirror gate in the integration
// lane (TestTheInMemoryRankerSelectsExactlyWhatTheSQLDoes).

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
)

func TestRetrieveRefusesWhatCannotRun(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"no question", []string{"--model", "fake:fake"}, "--question"},
		{"a question of spaces", []string{"--model", "fake:fake", "-q", "   "}, "--question"},
		{"no embedding binding", []string{"-q", "how do I create a project"}, "--model"},
		{"a floor that is not a cosine", []string{"-q", "x", "--model", "fake:fake", "--floor", "1.5"}, "[0,1]"},
		{"stdout, which would interleave with the report", []string{"-q", "x", "--model", "fake:fake", "--out", "-"}, "names a file"},
		{"an unquoted question, which arrives as positionals", []string{"--model", "fake:fake", "how", "now"}, "no positional"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseAITaskRetrieveFlags(tc.args)
			if err == nil {
				t.Fatalf("parseAITaskRetrieveFlags(%q) = nil error, want one mentioning %q", tc.args, tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %q, want it to mention %q", err, tc.want)
			}
		})
	}
}

// The short form is the whole reason -q exists: it is retyped on every iteration
// of a retrieval loop.
func TestRetrieveAcceptsTheShortQuestionForm(t *testing.T) {
	cfg, err := parseAITaskRetrieveFlags([]string{"-q", "how can I create a project", "--model", "mistral:mistral-embed"})
	if err != nil {
		t.Fatalf("parsing: %v", err)
	}
	if cfg.question != "how can I create a project" {
		t.Errorf("question = %q", cfg.question)
	}
	if cfg.floor != compose.CorpusProbeFloor {
		t.Errorf("floor = %v, want the shipped default %v — a retrieval run at another floor is not the one production runs",
			cfg.floor, compose.CorpusProbeFloor)
	}
	if cfg.workDir != workDirDefault {
		t.Errorf("work dir = %q, want the gitignored default %q", cfg.workDir, workDirDefault)
	}
}

// An empty ranking writes no fixture and says why. Production never asks the
// lane without passages, so a fixture carrying none would measure a call the
// product does not make.
func TestNothingIsWrittenWhenNothingClearsTheFloor(t *testing.T) {
	var out strings.Builder
	cfg := aiTaskRetrieveFlags{question: "how can I create a project", floor: 0.9, workDir: t.TempDir()}
	result := compose.CorpusProbeResult{Embedded: 42}
	if err := writeRetrieval(&out, cfg, result); err != nil {
		t.Fatalf("report: %v", err)
	}
	if err := emitRetrievedFixture(&out, cfg, result); err != nil {
		t.Fatalf("emit: %v", err)
	}
	report := out.String()
	for _, want := range []string{"not_covered", "42 embedded passage(s)", "no fixture to write"} {
		if !strings.Contains(report, want) {
			t.Errorf("the report does not say %q:\n%s", want, report)
		}
	}
	if strings.Contains(report, "--fixture") {
		t.Errorf("a follow-up run was suggested for a retrieval that produced no fixture:\n%s", report)
	}
}

// The table is what an operator reads before deciding anything, so it has to
// carry the four facts a passage is judged on.
func TestTheReportNamesEveryPassageAndWhereItCameFrom(t *testing.T) {
	var out strings.Builder
	cfg := aiTaskRetrieveFlags{question: "how can I create a project", page: "records.md", floor: 0.35}
	err := writeRetrieval(&out, cfg, compose.CorpusProbeResult{Embedded: 7, Passages: []compose.CorpusProbePassage{
		{DocumentName: "records.md", Text: "A project is\nnot a folder you create when you win.", StartLine: 198, Similarity: 0.7123},
	}})
	if err != nil {
		t.Fatalf("report: %v", err)
	}
	report := out.String()
	for _, want := range []string{"records.md", "198", "0.7123", "answered", "A project is not a folder"} {
		if !strings.Contains(report, want) {
			t.Errorf("the report does not carry %q:\n%s", want, report)
		}
	}
	// One passage, one row: a passage spanning several lines must not break the
	// table it is being read in.
	header := strings.Index(report, "LABEL")
	if header < 0 {
		t.Fatalf("the report has no table header:\n%s", report)
	}
	if lines := strings.Count(strings.TrimSpace(report[header:]), "\n"); lines != 1 {
		t.Errorf("the table is %d rows for one passage:\n%s", lines+1, report)
	}
}

// A ranking that cleared the floor is written down, and the report says where
// and what to run next — the capture is only useful if the next command can be
// pointed at it.
func TestARankingIsCapturedAsAFixtureTheRunVerbCanBePointedAt(t *testing.T) {
	dir := t.TempDir()
	var out strings.Builder
	cfg := aiTaskRetrieveFlags{question: "How can I create a project?", workDir: dir}
	fixture := []byte(`{"question":"How can I create a project?","passages":[]}`)
	if err := emitRetrievedFixture(&out, cfg, compose.CorpusProbeResult{Embedded: 31, Fixture: fixture}); err != nil {
		t.Fatalf("emit: %v", err)
	}
	path := filepath.Join(dir, "retrieve-How_can_I_create_a_project.json")
	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("no fixture landed in the work directory: %v", err)
	}
	if string(written) != string(fixture) {
		t.Errorf("the file holds %q, not the captured retrieval", written)
	}
	report := out.String()
	if !strings.Contains(report, path) {
		t.Errorf("the report does not say where the capture went:\n%s", report)
	}
	// The follow-up is the point of the capture: it names the site that reads
	// this fixture and the flag that takes it.
	for _, want := range []string{"--fixture", "corpus_ask"} {
		if !strings.Contains(report, want) {
			t.Errorf("the report does not tell the operator how to run it (%q):\n%s", want, report)
		}
	}
}

// Two retrievals in one session must not overwrite each other, which is why the
// capture is named after the question rather than after the verb.
func TestACaptureIsNamedAfterItsQuestion(t *testing.T) {
	first := retrieveArtifactName("how can I create a project")
	second := retrieveArtifactName("how long are messages kept")
	if first == second {
		t.Fatalf("both questions capture to %q, so the second run overwrites the first", first)
	}
	for _, name := range []string{first, second} {
		if !strings.HasPrefix(name, "retrieve-") || !strings.HasSuffix(name, ".json") {
			t.Errorf("capture name = %q, want retrieve-<question>.json", name)
		}
	}
	// A pasted question can be arbitrarily long; a filename cannot.
	long := retrieveArtifactName(strings.Repeat("why", 500))
	if len(long) > maxArtifactSlug+len("retrieve-.json") {
		t.Errorf("a long question produced a %d-character filename", len(long))
	}
}

// A passage spans several lines and the table row it is read in does not. The
// preview is also cut, because eight rows have to stay one screen.
func TestALongMultiLinePassageStaysOneTableRow(t *testing.T) {
	var out strings.Builder
	cfg := aiTaskRetrieveFlags{question: "how can I create a project", floor: 0.35}
	passage := "A project is\nnot a folder you create when you win — it is born while you are still selling,\nand it carries the deal's own history with it."
	err := writeRetrieval(&out, cfg, compose.CorpusProbeResult{Embedded: 31, Passages: []compose.CorpusProbePassage{
		{DocumentName: "records.md", Text: passage, StartLine: 198, Similarity: 0.7123},
	}})
	if err != nil {
		t.Fatalf("report: %v", err)
	}
	report := out.String()
	header := strings.Index(report, "LABEL")
	if header < 0 {
		t.Fatalf("the report has no table header:\n%s", report)
	}
	rows := strings.Split(strings.TrimSpace(report[header:]), "\n")
	if len(rows) != 2 {
		t.Fatalf("the table is %d lines for one passage:\n%s", len(rows), report)
	}
	// The row is cut to the preview width, and says it was cut: a silently
	// truncated passage reads as a passage that ends there.
	if !strings.Contains(rows[1], "…") {
		t.Errorf("a passage past the preview width was not marked as cut:\n%s", rows[1])
	}
	if !strings.Contains(rows[1], "A project is not a folder") {
		t.Errorf("the preview does not open with the passage's own words:\n%s", rows[1])
	}
}

// The verb stops before it spends anything when the run cannot be made, and
// names what would fix each. Every one of these is reachable only after the
// flags parsed, so a refusal here is one nothing else catches.
//
// It goes no further than that on purpose: driving the verb's own embed lane
// from this package would route a real embedding through the PROCESS-WIDE AI
// collector, and the worker's observe surface asserts that a process which has
// routed no call serves no AI sample. The ranking itself is proved without that
// collector in compose (TestTheProbeRanksTheShippedHandbookOverABoundEmbedLane).
func TestRetrieveStopsBeforeSpendingWhenTheRunCannotBeMade(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{
			name: "a log level nothing can be reported at",
			args: []string{"-q", "how can I create a project", "--model", "fake:fake-embed", "--log-level", "chatty"},
			want: "chatty",
		},
		{
			name: "a page this build does not ship",
			args: []string{"-q", "how can I create a project", "--model", "fake:fake-embed", "--page", "not-a-page.md"},
			want: "records.md",
		},
		{
			name: "a binding that is not provider:model",
			args: []string{"-q", "how can I create a project", "--model", "justamodel"},
			want: "provider:model",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var report strings.Builder
			err := runAITaskRetrieve(context.Background(), append(tc.args, "--work-dir", t.TempDir()), &report)
			if err == nil {
				t.Fatalf("the run went ahead; want a refusal naming %q", tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %q, want it to name %q", err, tc.want)
			}
			if report.Len() > 0 {
				t.Errorf("a refused run printed a report:\n%s", report.String())
			}
		})
	}
}
