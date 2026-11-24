// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agentloope2e

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var seededAt = time.Date(2026, 11, 24, 8, 0, 0, 0, time.UTC)

func judgedScenario() Scenario {
	return Scenario{
		Name: "s", Agent: MorningBrief, Worklist: []string{"heat_recovery"}, Forbidden: []string{"other_book"},
		Criteria: []string{"Does the finding say Thursday?"},
		Seed: Seed{Deals: []SeedDeal{
			{Name: "heat_recovery", Activities: []SeedActivity{{DaysAgo: 2, Subject: "Re: proposal", Body: "CFO signs Thursday."}}},
			{Name: "other_book", OtherSeat: true},
		}},
	}
}

func TestTheFactSheetStatesTheSeedWithDatesAndOwners(t *testing.T) {
	sheet := FactSheet(judgedScenario(), seededAt)
	for _, want := range []string{
		"dated 2026-11-24 UTC", `Deal "Heat Recovery", owned by the member the agent acts for`,
		`Email of 2026-11-22 (2 days earlier), subject "Re: proposal": CFO signs Thursday.`,
		`Deal "Other Book", owned by another member`, "No email is logged on it",
	} {
		if !strings.Contains(sheet, want) {
			t.Errorf("the fact sheet lacks %q:\n%s", want, sheet)
		}
	}
}

// The judge reads the facts and the persisted writes. A write on a record the
// seed does not name is shown as that, never under a deal's name.
func TestTheJudgedAnswerHoldsTheFactsAndEveryWriteAndNothingElse(t *testing.T) {
	writes := []Write{
		{Tool: "annotate_brief", Subject: "heat_recovery", Text: "  Send the quote by Wednesday.  "},
		{Tool: "log_activity", Subject: "", Text: "a loose note"},
	}
	answer := Answer(judgedScenario(), seededAt, writes)
	for _, want := range []string{
		"WORKSPACE FACTS", "WHAT THE AGENT WROTE", `- annotate_brief on deal "Heat Recovery": Send the quote by Wednesday.`,
		"- log_activity on a record the seed does not name: a loose note",
	} {
		if !strings.Contains(answer, want) {
			t.Errorf("the answer lacks %q:\n%s", want, answer)
		}
	}
	if none := Answer(judgedScenario(), seededAt, nil); !strings.Contains(none, "Nothing was written.") {
		t.Errorf("an empty run reads:\n%s", none)
	}
}

func TestAScenarioCarriesTheDigestOfItsFileWhole(t *testing.T) {
	dir := t.TempDir()
	write := func(body string) Scenario {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, "a.yaml"), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		all, err := LoadScenarios(dir)
		if err != nil || len(all) != 1 {
			t.Fatalf("%v", err)
		}
		return all[0]
	}
	const body = `name: a
agent: morning_brief
worklist: [d]
criteria: ["Does it hold?"]
seed:
  deals:
    - name: d
      activities:
        - {days_ago: 1, subject: s, body: b}
`
	first := write(body)
	if first.Digest == "" || write(body).Digest != first.Digest {
		t.Fatalf("digest = %q, and it changed with no edit", first.Digest)
	}
	if write(strings.Replace(body, "body: b", "body: c", 1)).Digest == first.Digest {
		t.Error("an edit to the seed left the digest as it was")
	}
}

func TestVendorFamiliesMatchTheWayJudgePyReadsThem(t *testing.T) {
	for _, tc := range []struct {
		a, b string
		same bool
	}{
		{"gemini-3.8-flash", "gemini-3.1-flash-lite", true},
		{"claude-haiku-5-5", "anthropic/claude-haiku-5.5", true},
		{"gemini-3.8-flash", "claude-haiku-5-5", false},
		{"some-other-model", "some-other-model", false},
		{"gpt-5.6-sol", "gpt-5.6-sol", true},
	} {
		if SameFamily(tc.a, tc.b) != tc.same {
			t.Errorf("SameFamily(%q, %q) = %v", tc.a, tc.b, !tc.same)
		}
	}
}

// fakeRoot is a repository root whose judge door prints a canned answer.
func fakeRoot(t *testing.T, script string) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "e2e", "llm"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "e2e", "llm", "judge_cli.py"), []byte(script), 0o600); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestTheJudgeDoorRefusesWhatItCannotTrust(t *testing.T) {
	ask := func(script string) error {
		_, err := CommandJudge{Root: fakeRoot(t, script)}.Grade(context.Background(), []string{"one?", "two?"}, "answer")
		return err
	}
	for name, script := range map[string]string{
		"an unreadable answer":      "print('not json')",
		"too few verdicts":          `print('[{"criterion":"one?","met":true,"reason":"r"}]')`,
		"another wording":           `print('[{"criterion":"one?","met":true,"reason":"r"},{"criterion":"other?","met":true,"reason":"r"}]')`,
		"a failure that is no stop": "import sys; sys.exit(1)",
	} {
		if err := ask(script); err == nil || errors.Is(err, ErrJudgeUnavailable) {
			t.Errorf("%s: err = %v, want a plain refusal", name, err)
		}
	}
	err := ask("import sys; sys.stderr.write('no key'); sys.exit(3)")
	if !errors.Is(err, ErrJudgeUnavailable) || !strings.Contains(err.Error(), "no key") {
		t.Errorf("exit 3 = %v, want ErrJudgeUnavailable carrying the reason", err)
	}
	good := `print('[{"criterion":"one?","met":true,"reason":"a"},{"criterion":"two?","met":false,"reason":"b"}]')`
	root := fakeRoot(t, good)
	got, err := CommandJudge{Root: root, Env: []string{}}.Grade(context.Background(), []string{"one?", "two?"}, "answer")
	if err != nil || len(got) != 2 || !got[0].Met || got[1].Met || got[1].Reason != "b" {
		t.Errorf("verdicts = %+v, %v", got, err)
	}
}

// The real door, with the judge planted through judge.py's own hook, answers
// each criterion once, and says no judge is configured when none is.
func TestTheRealJudgeDoorGradesThroughThePlantedJudgeAndStopsWithoutOne(t *testing.T) {
	root := repoRootOf(t)
	planted := []string{"E2E_LLM_JUDGE=cmd:cat >/dev/null; printf '{\"reason\": \"planted\", \"verdict\": \"yes\"}'"}
	door := CommandJudge{Root: root, Env: append(os.Environ(), planted...)}
	got, err := door.Grade(context.Background(), []string{"one?", "two?"}, "an answer")
	if err != nil || len(got) != 2 || !got[0].Met || got[0].Reason != "planted" {
		t.Fatalf("verdicts = %+v, %v", got, err)
	}
	if err := door.Ready(context.Background()); err != nil {
		t.Errorf("a planted judge is not ready: %v", err)
	}
	bare := CommandJudge{Root: root, Env: []string{"PATH=" + os.Getenv("PATH")}}
	if err := bare.Ready(context.Background()); !errors.Is(err, ErrJudgeUnavailable) {
		t.Errorf("with no judge configured Ready = %v, want ErrJudgeUnavailable", err)
	}
	if _, err := bare.Grade(context.Background(), []string{"one?"}, "a"); !errors.Is(err, ErrJudgeUnavailable) {
		t.Errorf("with no judge configured Grade = %v, want ErrJudgeUnavailable", err)
	}
}

func repoRootOf(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.work")); err == nil {
			return dir
		}
		if parent := filepath.Dir(dir); parent != dir {
			dir = parent
			continue
		}
		t.Fatal("no go.work above the test's folder")
	}
}
