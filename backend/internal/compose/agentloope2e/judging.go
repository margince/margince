// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agentloope2e

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// FactSheet states what the seed holds, in the seed's own words. The judge needs
// it to tell a fact from an invention. It is built from the seed manifest and
// the instant the seeding counted from, never from anything the agent said.
func FactSheet(sc Scenario, seededAt time.Time) string {
	var b strings.Builder
	fmt.Fprintf(&b, "WORKSPACE FACTS (the seeded records, dated %s UTC)\n", seededAt.UTC().Format(time.DateOnly))
	for _, deal := range sc.Seed.Deals {
		owner := "the member the agent acts for"
		if deal.OtherSeat {
			owner = "another member"
		}
		fmt.Fprintf(&b, "Deal %q, owned by %s.\n", deal.DisplayName(), owner)
		if len(deal.Activities) == 0 {
			b.WriteString("  No email is logged on it.\n")
		}
		for _, activity := range deal.Activities {
			date := seededAt.UTC().AddDate(0, 0, -activity.DaysAgo).Format(time.DateOnly)
			fmt.Fprintf(&b, "  Email of %s (%d days earlier), subject %q: %s\n", date, activity.DaysAgo, activity.Subject, activity.Body)
		}
	}
	b.WriteString("The workspace's own tools also serve risk signals they compute from these records, such as a deal that rests on too few engaged contacts (single-threaded). A cause that names such a signal is one the records carry.\n")
	return b.String()
}

// Answer is the text the judge grades: the fact sheet, then what the run wrote,
// read back from the persisted rows. The agent's own final summary and
// reasoning are never in it. The judge grades the record of what happened.
func Answer(sc Scenario, seededAt time.Time, writes []Write) string {
	var b strings.Builder
	b.WriteString(FactSheet(sc, seededAt))
	b.WriteString("\nWHAT THE AGENT WROTE (read back from the database)\n")
	if len(writes) == 0 {
		b.WriteString("Nothing was written.\n")
	}
	for _, write := range writes {
		subject := "a record the seed does not name"
		if deal, ok := sc.Seed.Deal(write.Subject); ok {
			subject = fmt.Sprintf("deal %q", deal.DisplayName())
		}
		fmt.Fprintf(&b, "- %s on %s: %s\n", write.Tool, subject, strings.TrimSpace(write.Text))
	}
	return b.String()
}

// ErrJudgeUnavailable says no verdict could be reached. It is a harness stop and
// never a score: a criterion nobody judged must not read as one that passed.
var ErrJudgeUnavailable = errors.New("the judge is unavailable")

// Judge answers each criterion yes or no about one answer.
type Judge interface {
	Grade(ctx context.Context, criteria []string, answer string) ([]Criterion, error)
}

// Exit codes e2e/llm/judge_cli.py documents.
const exitJudgeUnavailable = 3

// CommandJudge is the MCP end-to-end lane's judge, reached through the door
// e2e/llm/judge_cli.py. Which backend answers is the caller's
// E2E_LLM_JUDGE, as for that lane.
type CommandJudge struct {
	// Root is the repository root.
	Root string
	// Env is the child's environment; nil lets the child inherit this process's.
	Env []string
}

// command builds the judge door's process. The interpreter is resolved to an
// absolute path first, so the launch never searches PATH a second time.
func (j CommandJudge) command(ctx context.Context, args ...string) (*exec.Cmd, error) {
	python, err := exec.LookPath("python3")
	if err != nil {
		return nil, fmt.Errorf("%w: python3 is not installed", ErrJudgeUnavailable)
	}
	cmd := exec.CommandContext(ctx, python, append([]string{filepath.Join(j.Root, "e2e", "llm", "judge_cli.py")}, args...)...) // #nosec G204 -- a fixed script of this repository
	cmd.Dir = filepath.Join(j.Root, "e2e", "llm")
	cmd.Env = j.Env
	return cmd, nil
}

// Ready fails before any model is paid for when no verdict could be reached. It
// names the judge's own reason.
func (j CommandJudge) Ready(ctx context.Context) error {
	cmd, err := j.command(ctx, "--ready")
	if err != nil {
		return err
	}
	_, err = j.run(cmd)
	return err
}

// Model is the judge model the lane is pinned to.
func (j CommandJudge) Model(ctx context.Context) (string, error) {
	cmd, err := j.command(ctx, "--model")
	if err != nil {
		return "", err
	}
	out, err := j.run(cmd)
	return strings.TrimSpace(out), err
}

// Grade asks the judge every criterion about the answer. The reply must hold one
// verdict for each criterion asked, under the same wording.
func (j CommandJudge) Grade(ctx context.Context, criteria []string, answer string) ([]Criterion, error) {
	request, err := json.Marshal(struct {
		Criteria []string `json:"criteria"`
		Answer   string   `json:"answer"`
	}{criteria, answer})
	if err != nil {
		return nil, err
	}
	cmd, err := j.command(ctx)
	if err != nil {
		return nil, err
	}
	cmd.Stdin = bytes.NewReader(request)
	out, err := j.run(cmd)
	if err != nil {
		return nil, err
	}
	var got []Criterion
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		return nil, fmt.Errorf("agentloope2e: the judge answered something unreadable: %w", err)
	}
	if len(got) != len(criteria) {
		return nil, fmt.Errorf("agentloope2e: the judge answered %d of %d criteria", len(got), len(criteria))
	}
	for i, want := range criteria {
		if got[i].Text != want {
			return nil, fmt.Errorf("agentloope2e: the judge answered criterion %d under other words", i+1)
		}
	}
	return got, nil
}

func (j CommandJudge) run(cmd *exec.Cmd) (string, error) {
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	var exit *exec.ExitError
	switch {
	case errors.As(err, &exit) && exit.ExitCode() == exitJudgeUnavailable:
		return "", fmt.Errorf("%w: %s", ErrJudgeUnavailable, strings.TrimSpace(stderr.String()))
	case err != nil:
		return "", fmt.Errorf("agentloope2e: running the judge: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}

// vendorFamilies are the model vendors judge.py's family() knows, in its order.
var vendorFamilies = []struct {
	name    string
	pattern *regexp.Regexp
}{
	{"claude", regexp.MustCompile(`(?:^|[/-])claude(?:[-.]|$)`)},
	{"gpt", regexp.MustCompile(`(?:^|[/-])gpt(?:[-.]|$)`)},
	{"gemini", regexp.MustCompile(`(?:^|[/-])gemini(?:[-.]|$)`)},
	{"mistral", regexp.MustCompile(`(?:^|[/-])mistral(?:[-.]|$)`)},
}

// SameFamily says whether two model ids belong to one vendor. A judge of the
// candidate's own family grades its own kind, which the verdict records.
func SameFamily(a, b string) bool {
	fa, fb := family(a), family(b)
	return fa != "" && fa == fb
}

func family(model string) string {
	for _, vendor := range vendorFamilies {
		if vendor.pattern.MatchString(model) {
			return vendor.name
		}
	}
	return ""
}
