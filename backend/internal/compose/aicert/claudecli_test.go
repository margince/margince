// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// cliSuccess is the shape `claude -p --output-format json` prints for one
// answered turn: two models in the usage report, the grader and a small helper.
const cliSuccess = `{"type":"result","subtype":"success","is_error":false,"result":"{\"score\": 80}",
"stop_reason":"end_turn","modelUsage":{
"claude-haiku-4-5":{"inputTokens":7,"outputTokens":2,"cacheReadInputTokens":0,"cacheCreationInputTokens":0},
"claude-sonnet-4-6":{"inputTokens":100,"outputTokens":40,"cacheReadInputTokens":30,"cacheCreationInputTokens":5}}}`

// cliNotLoggedIn is what the CLI prints, exiting 1, when its credential is refused.
const cliNotLoggedIn = `{"type":"result","subtype":"success","is_error":true,"result":"Not logged in · Please run /login","modelUsage":{}}`

// stubClaude puts a fake `claude` first on PATH that records how it was run into
// the returned directory, then runs body.
func stubClaude(t *testing.T, body string) string {
	t.Helper()
	bin, record := t.TempDir(), t.TempDir()
	script := "#!/bin/sh\n" +
		"printf '%s\\n' \"$@\" > '" + record + "/args'\n" +
		"cat > '" + record + "/stdin'\n" +
		"env > '" + record + "/env'\n" +
		"pwd > '" + record + "/pwd'\n" +
		body + "\n"
	if err := os.WriteFile(filepath.Join(bin, claudeCLIBinary), []byte(script), 0o700); err != nil { // #nosec G306 -- the stub must be executable
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return record
}

// printing is a stub body that prints out and exits with code.
func printing(out, code string) string {
	return "cat <<'JSON'\n" + out + "\nJSON\nexit " + code
}

// withCredential leaves exactly the subscription token set, as .env.local does.
func withCredential(t *testing.T) {
	t.Helper()
	t.Setenv("ANTHROPIC_AUTH_TOKEN", "")
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("CLAUDE_CODE_OAUTH_TOKEN", "stub-subscription-token")
}

func recorded(t *testing.T, record, name string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(record, name)) // #nosec G304 -- a file the stub wrote into this test's own temp dir
	if err != nil {
		t.Fatalf("the stub CLI left no %s: %v", name, err)
	}
	return string(raw)
}

func judgeRequest() model.Request {
	return model.Request{System: "You grade answers.", Messages: []model.Message{{Role: roleUser, Content: "Grade this."}}}
}

// The served model is the one the CLI's usage report says wrote the answer, not
// the --model alias, and the call's tokens are what every model spent.
func TestTheCLIJudgeReportsTheModelThatAnswered(t *testing.T) {
	withCredential(t)
	record := stubClaude(t, printing(cliSuccess, "0"))
	resp, err := claudeCLIJudge{model: "sonnet"}.Complete(context.Background(), judgeRequest())
	if err != nil {
		t.Fatalf("a successful CLI run failed: %v", err)
	}
	if resp.Text != `{"score": 80}` || resp.ServedModel != "claude-sonnet-4-6" || resp.FinishReason != "stop" {
		t.Errorf("resp = %+v, want the result text served by claude-sonnet-4-6", resp)
	}
	if resp.InputTokens != 142 || resp.OutputTokens != 42 || resp.CachedTokens != 30 || resp.CacheWriteTokens != 5 {
		t.Errorf("tokens = in %d out %d cached %d written %d, want 142/42/30/5",
			resp.InputTokens, resp.OutputTokens, resp.CachedTokens, resp.CacheWriteTokens)
	}
	if stdin := recorded(t, record, "stdin"); stdin != "Grade this." {
		t.Errorf("stdin = %q, want the user turn alone", stdin)
	}
	args := recorded(t, record, "args")
	for _, want := range []string{
		"-p\n", "--model\nsonnet\n", "--output-format\njson\n", "--system-prompt-file\n",
		"--tools\n\n", "--setting-sources\n\n", "--no-session-persistence\n", "--strict-mcp-config\n",
	} {
		if !strings.Contains(args, want) {
			t.Errorf("args = %q, want %q", args, want)
		}
	}
	if strings.Contains(args, "--append-system-prompt") {
		t.Error("the judge prompt must replace Claude Code's system prompt, not be appended to it")
	}
}

// The CLI runs from an empty directory that is also its HOME, with only the
// credential this lane chose, so nothing of the operator's reaches the grader.
func TestTheCLIJudgeRunsIsolatedFromTheOperatorsSetup(t *testing.T) {
	withCredential(t)
	t.Setenv("ANTHROPIC_API_KEY", "a-second-credential")
	t.Setenv("AICERT_OPERATOR_ONLY", "leaks")
	proxies := map[string]string{
		"HTTPS_PROXY": "http://proxy.test:3128", "https_proxy": "http://proxy.test:3129",
		"HTTP_PROXY": "http://proxy.test:3130", "http_proxy": "http://proxy.test:3131",
		"NO_PROXY": "localhost", "no_proxy": "127.0.0.1",
	}
	for name, value := range proxies {
		t.Setenv(name, value)
	}
	record := stubClaude(t, printing(cliSuccess, "0"))
	if _, err := (claudeCLIJudge{model: "sonnet"}).Complete(context.Background(), judgeRequest()); err != nil {
		t.Fatal(err)
	}
	env, pwd := recorded(t, record, "env"), recorded(t, record, "pwd")
	for name, value := range proxies {
		if !strings.Contains(env, name+"="+value+"\n") {
			t.Errorf("env = %q: %s did not reach the CLI, so a proxied host could not dial out", env, name)
		}
	}
	if !strings.Contains(pwd, "aicert-claude-judge-") || !strings.Contains(env, "HOME=") ||
		!strings.Contains(env, "aicert-claude-judge-") {
		t.Errorf("pwd = %q, env = %q: want the CLI in, and homed at, its own temp dir", pwd, env)
	}
	if strings.Contains(env, "AICERT_OPERATOR_ONLY") || strings.Contains(env, "CLAUDE_CODE_OAUTH_TOKEN") {
		t.Errorf("env = %q: only the ranked-first credential may reach the CLI", env)
	}
	if !strings.Contains(env, "ANTHROPIC_API_KEY=a-second-credential") {
		t.Errorf("env = %q: the credential the CLI ranks first was not passed", env)
	}
}

// Every way the CLI can fail is an error that says what the CLI said, and never
// an answer the grader did not give.
func TestTheCLIJudgeRefusesEveryFailedRun(t *testing.T) {
	for name, tc := range map[string]struct{ body, want string }{
		"non-zero exit, no JSON": {"echo 'network unreachable' >&2; exit 3", "network unreachable"},
		"refused credential":     {printing(cliNotLoggedIn, "1"), "Not logged in"},
		"is_error on exit 0":     {printing(`{"is_error":true,"subtype":"error_max_turns","result":"stopped"}`, "0"), "error_max_turns"},
		"garbage":                {"echo 'this is not json'", "not the JSON result"},
		"empty result":           {printing(`{"is_error":false,"result":"  ","modelUsage":{}}`, "0"), "empty result"},
		"no usage report":        {printing(`{"is_error":false,"result":"ok","modelUsage":{}}`, "0"), "no model usage"},
	} {
		t.Run(name, func(t *testing.T) {
			withCredential(t)
			stubClaude(t, tc.body)
			_, err := claudeCLIJudge{model: "sonnet"}.Complete(context.Background(), judgeRequest())
			if !errors.Is(err, errCLIJudge) || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want errCLIJudge naming %q", err, tc.want)
			}
			if strings.Contains(err.Error(), "stub-subscription-token") {
				t.Errorf("err = %v leaks the credential", err)
			}
		})
	}
}

// A missing credential or a missing CLI is named before anything is run.
func TestTheCLIJudgeNamesAMissingPrerequisite(t *testing.T) {
	t.Run("no credential", func(t *testing.T) {
		withCredential(t)
		t.Setenv("CLAUDE_CODE_OAUTH_TOKEN", "")
		record := stubClaude(t, printing(cliSuccess, "0"))
		_, err := claudeCLIJudge{model: "sonnet"}.Complete(context.Background(), judgeRequest())
		if err == nil || !strings.Contains(err.Error(), "CLAUDE_CODE_OAUTH_TOKEN") {
			t.Fatalf("err = %v, want it to name the credential to set", err)
		}
		if _, statErr := os.Stat(filepath.Join(record, "args")); !os.IsNotExist(statErr) {
			t.Error("the CLI was run without a credential")
		}
	})
	t.Run("no CLI", func(t *testing.T) {
		withCredential(t)
		t.Setenv("PATH", t.TempDir())
		_, err := claudeCLIJudge{model: "sonnet"}.Complete(context.Background(), judgeRequest())
		if err == nil || !strings.Contains(err.Error(), "claude CLI on PATH") {
			t.Fatalf("err = %v, want it to say the CLI is not on PATH", err)
		}
	})
}

// A validated retry reaches the CLI as one turn, with the reply that failed
// labelled as the grader's own and the request's secrets stripped.
func TestTheCLIJudgeFoldsARetryIntoOneStrippedTurn(t *testing.T) {
	withCredential(t)
	record := stubClaude(t, printing(cliSuccess, "0"))
	req := judgeRequest()
	req.Messages = append(req.Messages,
		model.Message{Role: "assistant", Content: "score: eighty"},
		model.Message{Role: roleUser, Content: "Reply with JSON. key sk-live-123"})
	req.SecretStripper = redacting{secret: "sk-live-123"}
	if _, err := (claudeCLIJudge{model: "sonnet"}).Complete(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	want := "Grade this.\n\n[Your previous reply]\nscore: eighty\n\nReply with JSON. key [redacted]"
	if stdin := recorded(t, record, "stdin"); stdin != want {
		t.Errorf("stdin = %q, want %q", stdin, want)
	}
}

type redacting struct{ secret string }

func (r redacting) Strip(_ context.Context, payload []byte) ([]byte, model.StripReport, error) {
	return []byte(strings.ReplaceAll(string(payload), r.secret, "[redacted]")), model.StripReport{Findings: 1}, nil
}

// Through the judge router the pre-flight uses, the trace names the CLI and the
// model that answered — the identity a record's judge_served_model is read from.
func TestTheJudgeRouterTracesTheCLIsServedModel(t *testing.T) {
	withCredential(t)
	stubClaude(t, printing(cliSuccess, "0"))
	judge := ai.ProviderConfig{Provider: providerClaudeCLI, Model: "sonnet"}
	routing, err := ladderForTask("judge", judge, ai.ProfileCloudFrontier, ai.TaskCertJudge)
	if err != nil {
		t.Fatal(err)
	}
	rec := newTraceRecorder()
	opts := append([]ai.LocalOption{ai.WithoutResultCache(), ai.WithCallStore(rec)}, judgeTransport(judge)...)
	router, err := compose.NewLocalRouterForCert(routing, opts...)
	if err != nil {
		t.Fatalf("a claude_cli judge router could not be built: %v", err)
	}
	if _, _, err := router.Complete(wsContext(t), ai.TaskCertJudge, judgeRequest()); err != nil {
		t.Fatal(err)
	}
	term, ok := rec.lastTerminal()
	if !ok || term.Provider != providerClaudeCLI || term.ServedModel != "claude-sonnet-4-6" {
		t.Errorf("terminal = %+v, want claude_cli served by claude-sonnet-4-6", term)
	}
}

// The pre-flight probes the CLI judge before any paid candidate call and fails
// naming what is missing.
func TestThePreflightProbesTheCLIJudge(t *testing.T) {
	cfg := RunnerConfig{
		Binding:      ai.ProviderConfig{Provider: ai.ProviderFake, Model: "candidate"},
		JudgeBinding: ai.ProviderConfig{Provider: providerClaudeCLI, Model: "sonnet"},
		Profile:      ai.ProfileCloudFrontier,
	}
	withCredential(t)
	record := stubClaude(t, printing(cliSuccess, "0"))
	if err := preflight(wsContext(t), cfg, []ai.Task{ai.TaskSummarize}, nil, quietLogger()); err != nil {
		t.Fatalf("a servable CLI judge failed its pre-flight: %v", err)
	}
	if !strings.Contains(recorded(t, record, "stdin"), preflightPrompt) {
		t.Error("the pre-flight never reached the CLI")
	}
	t.Setenv("CLAUDE_CODE_OAUTH_TOKEN", "")
	err := preflight(wsContext(t), cfg, []ai.Task{ai.TaskSummarize}, nil, quietLogger())
	if err == nil || !strings.Contains(err.Error(), "judge") || !strings.Contains(err.Error(), "CLAUDE_CODE_OAUTH_TOKEN") {
		t.Fatalf("err = %v, want the judge's pre-flight to name the missing credential", err)
	}
}

// The CLI serves nothing but Claude, so it may not grade a Claude candidate on
// any wire, and grades every other family.
func TestACLIJudgeRefusesAClaudeCandidate(t *testing.T) {
	t.Parallel()
	cfg := RunnerConfig{JudgeBinding: ai.ProviderConfig{Provider: providerClaudeCLI, Model: "sonnet"}}
	for _, tc := range []struct {
		candidate ai.ProviderConfig
		refused   bool
	}{
		{ai.ProviderConfig{Provider: "anthropic", Model: "claude-haiku-4-5"}, true},
		{ai.ProviderConfig{Provider: "openai_compatible", Model: "anthropic/claude-haiku-4.5"}, true},
		{ai.ProviderConfig{Provider: providerClaudeCLI, Model: "opus"}, true},
		{ai.ProviderConfig{Provider: "openai_compatible", Model: "us.anthropic.claude-haiku-4-5-20251001-v1:0"}, true},
		{ai.ProviderConfig{Provider: "openai_compatible", Model: "anthropic.claude-3-5-sonnet-20240620-v1:0"}, true},
		{ai.ProviderConfig{Provider: "gemini", Model: "gemini-3.5-flash"}, false},
		{ai.ProviderConfig{Provider: "openai_compatible", Model: "openai/gpt-oss-120b"}, false},
	} {
		candidate, refused := tc.candidate, tc.refused
		_, err := cfg.judgeFor(candidate)
		if got := errors.Is(err, errSelfJudged); got != refused {
			t.Errorf("%s:%s graded by claude_cli: refused = %v, want %v", candidate.Provider, candidate.Model, got, refused)
		}
	}
}

// A --model value is the operator's, and one shaped like a flag would be read
// by the CLI as one, so it is refused before anything runs.
func TestTheCLIJudgeRefusesAModelShapedLikeAFlag(t *testing.T) {
	withCredential(t)
	record := stubClaude(t, printing(cliSuccess, "0"))
	_, err := claudeCLIJudge{model: "--dangerously-skip-permissions"}.Complete(context.Background(), judgeRequest())
	if err == nil || !strings.Contains(err.Error(), "--dangerously-skip-permissions") {
		t.Fatalf("err = %v, want the flag-shaped model refused by name", err)
	}
	if _, statErr := os.Stat(filepath.Join(record, "args")); !os.IsNotExist(statErr) {
		t.Error("the CLI was run with a flag-shaped model")
	}
}

// A call the caller cancelled and a call that ran out of time are different
// failures, and the error says which.
func TestTheCLIJudgeTellsACancelledCallFromALateOne(t *testing.T) {
	withCredential(t)
	stubClaude(t, printing(cliSuccess, "0"))
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	late, stop := context.WithDeadline(context.Background(), time.Unix(0, 0))
	defer stop()
	for name, tc := range map[string]struct {
		ctx       context.Context
		want      string
		wantCause error
	}{
		"cancelled": {cancelled, "was cancelled before it answered", context.Canceled},
		"late":      {late, "did not answer before its deadline", context.DeadlineExceeded},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := claudeCLIJudge{model: "sonnet"}.Complete(tc.ctx, judgeRequest())
			if !errors.Is(err, tc.wantCause) || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q wrapping %v", err, tc.want, tc.wantCause)
			}
		})
	}
}

// A record names the judge's transport, so a sweep graded through the CLI is
// told apart from one graded over a broker by the files alone.
func TestARecordNamesTheJudgesTransport(t *testing.T) {
	withCredential(t)
	stubClaude(t, printing(cliSuccess, "0"))
	candidateFake := ai.NewFakeClient().Script("the widget is blue", "the widget is blue", "the widget is blue")
	rec, err := certifyTask(wsContext(t), ai.TaskSummarize, []Scenario{testScenario("basic", wideBands)}, testCensus(t),
		ai.ProviderConfig{Provider: ai.ProviderFake, Model: "candidate"},
		ai.ProviderConfig{Provider: providerClaudeCLI, Model: "sonnet"}, ai.ProfileCloudFrontier, 3, quietLogger(),
		&certifyHooks{candidateOpts: []ai.LocalOption{ai.WithFakeClient(candidateFake)}})
	if err != nil {
		t.Fatalf("certifyTask: %v", err)
	}
	if rec.JudgeProvider != providerClaudeCLI {
		t.Errorf("judge_provider = %q, want %q", rec.JudgeProvider, providerClaudeCLI)
	}
	raw, err := json.Marshal(Record{})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(raw), "judge_provider") {
		t.Errorf("a record with no judge transport names one, so every committed record would change: %s", raw)
	}
}
