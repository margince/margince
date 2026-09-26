// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert

// The claude_cli judge transport: JUDGE=claude_cli:<model> grades through the
// Claude Code CLI in headless mode, on a subscription token, instead of through
// a provider adapter. A harness transport, never a product one — the product
// must not shell out to a CLI — so it lives here and reaches the judge router
// through ai.WithHarnessClient, which gates/harnesstransport_test.go confines
// to this package.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

const (
	// providerClaudeCLI is the provider half of a JUDGE=claude_cli:<model> binding.
	providerClaudeCLI = "claude_cli"
	// claudeCLIBinary is looked up on PATH per call, so a missing CLI fails the
	// pre-flight with its own message rather than the router's "unknown provider".
	claudeCLIBinary = "claude"
	// claudeCLIFamily is the served identity the family rule compares a
	// candidate against: every model the CLI serves is a Claude model.
	claudeCLIFamily = "anthropic/claude"
	// cliWaitDelay bounds how long a cancelled CLI may keep its pipes open.
	cliWaitDelay = 5 * time.Second
	// cliStderrLimit caps how much of the CLI's stderr an error repeats.
	cliStderrLimit = 400
)

// claudeCLICredentials are the variables the CLI authenticates with, in the
// order this lane prefers them: the subscription token first, so a Console key
// also in .env.local is never billed for a judge the subscription covers. The
// CLI's own ranking (scripts/lib-llm-credential.sh) differs, and does not
// apply here: cliEnv hands the CLI the chosen credential alone.
var claudeCLICredentials = []string{"CLAUDE_CODE_OAUTH_TOKEN", "ANTHROPIC_AUTH_TOKEN", "ANTHROPIC_API_KEY"}

// judgeTransport is the LocalOption that serves a claude_cli judge binding, and
// nothing for every other binding: the provider judge path is the default and
// is left exactly as it was.
func judgeTransport(judge ai.ProviderConfig) []ai.LocalOption {
	if judge.Provider != providerClaudeCLI {
		return nil
	}
	return []ai.LocalOption{ai.WithHarnessClient(providerClaudeCLI, claudeCLIJudge{model: judge.Model})}
}

// cliJudgesOwnFamily reports whether a claude_cli judge would grade a Claude
// candidate. Refused outright rather than flagged, because the CLI serves
// nothing but Claude, so the collision holds on every run of the task.
func cliJudgesOwnFamily(candidate, judge ai.ProviderConfig) bool {
	if judge.Provider != providerClaudeCLI {
		return false
	}
	return candidate.Provider == providerClaudeCLI || selfJudged(candidate.Model, claudeCLIFamily)
}

// claudeCLIJudge is a model.Client over `claude -p`. It carries text only: no
// tools, attachments, streaming or embeddings, which the judge never asks for.
type claudeCLIJudge struct {
	model string
	// env is where the credential, PATH and passthrough variables come from;
	// nil is the process environment.
	env config.Lookup
}

func (j claudeCLIJudge) lookup() config.Lookup {
	if j.env == nil {
		return config.FromOS
	}
	return j.env
}

// Caps declares a text-only client with no window worth planning around.
func (claudeCLIJudge) Caps() model.Capabilities { return model.Capabilities{} }

// Stream is refused: the judge completes, it never streams.
//
//nolint:ireturn // the port's own signature
func (claudeCLIJudge) Stream(context.Context, model.Request) (model.TokenStream, error) {
	return nil, errors.New("aicert: the claude_cli judge completes only; it has no streaming lane")
}

// Embed is refused: the router needs an embedder bound, and the judge never embeds.
func (claudeCLIJudge) Embed(context.Context, model.EmbedRequest) (model.Embeddings, error) {
	return model.Embeddings{}, fmt.Errorf("aicert: the claude_cli judge: %w", model.ErrEmbeddingsUnsupported)
}

// cliWire is what one call sends: the system prompt and the one user turn.
// Marshalled so the request's SecretStripper runs over it, as every adapter's
// wire body is stripped before it leaves the process.
type cliWire struct {
	System string `json:"system"`
	Prompt string `json:"prompt"`
}

// Complete runs one `claude -p` from an empty temporary directory, which is
// also its HOME, so no CLAUDE.md, settings, hooks or memory reach the grader.
func (j claudeCLIJudge) Complete(ctx context.Context, req model.Request) (resp model.Response, err error) {
	if strings.HasPrefix(j.model, "-") {
		return model.Response{}, fmt.Errorf("aicert: JUDGE=claude_cli:%s names a flag, not a model: %w", j.model, model.ErrRequestRejected)
	}
	binary, credential, err := cliPrerequisites(j.lookup())
	if err != nil {
		return model.Response{}, err
	}
	wire, err := strippedWire(ctx, req)
	if err != nil {
		return model.Response{}, err
	}
	dir, err := os.MkdirTemp("", "aicert-claude-judge-*")
	if err != nil {
		return model.Response{}, fmt.Errorf("aicert: the claude_cli judge could not make its empty working directory: %w", err)
	}
	defer func() {
		if rmErr := os.RemoveAll(dir); rmErr != nil {
			err = errors.Join(err, fmt.Errorf("aicert: removing the claude_cli judge's working directory: %w", rmErr))
		}
	}()
	systemFile := filepath.Join(dir, "system.txt")
	if err := os.WriteFile(systemFile, []byte(wire.System), 0o600); err != nil {
		return model.Response{}, fmt.Errorf("aicert: the claude_cli judge could not write its system prompt: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, ai.CallCeiling)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, j.args(systemFile)...) // #nosec G204 -- the binary is the claude CLI found on PATH; every argument is a flag this file spells or the operator's JUDGE= model
	cmd.Dir = dir
	cmd.Env = cliEnv(j.lookup(), dir, credential)
	cmd.Stdin = strings.NewReader(wire.Prompt)
	cmd.WaitDelay = cliWaitDelay
	killProcessGroupOnCancel(cmd)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	runErr := cmd.Run()
	if ctxErr := ctx.Err(); ctxErr != nil {
		if errors.Is(ctxErr, context.DeadlineExceeded) {
			return model.Response{}, fmt.Errorf("aicert: the claude_cli judge did not answer before its deadline (the call ceiling is %s): %w", ai.CallCeiling, ctxErr)
		}
		return model.Response{}, fmt.Errorf("aicert: the claude_cli judge call was cancelled before it answered: %w", ctxErr)
	}
	return parseCLIResult(stdout.Bytes(), stderr.String(), runErr)
}

// args are the flags one call runs with, each chosen to leave the grader with
// our instructions alone:
//
//   - --system-prompt-file REPLACES Claude Code's own system prompt; the
//     --append form would grade under the coding assistant's instructions too.
//   - --tools "" disables every built-in tool, and --strict-mcp-config with no
//     --mcp-config attaches no MCP server.
//   - --setting-sources "" loads no user, project or local settings, so no hook
//     or permission rule of the operator's runs.
//   - --no-session-persistence writes no session to disk; --max-turns 1 is one
//     reply, which is all a grade is.
func (j claudeCLIJudge) args(systemFile string) []string {
	return []string{
		"-p",
		"--model", j.model,
		"--output-format", "json",
		"--system-prompt-file", systemFile,
		"--tools", "",
		"--strict-mcp-config",
		"--setting-sources", "",
		"--no-session-persistence",
		"--max-turns", "1",
	}
}

// cliPrerequisites finds the CLI and the credential it will spend, so a missing
// one fails the pre-flight in words rather than as a CLI that printed nothing.
func cliPrerequisites(env config.Lookup) (binary, credential string, err error) {
	binary, err = exec.LookPath(claudeCLIBinary)
	if err != nil {
		return "", "", fmt.Errorf("aicert: JUDGE=claude_cli needs the claude CLI on PATH (install Claude Code): %w", err)
	}
	for _, name := range claudeCLICredentials {
		if env(name) != "" {
			return binary, name, nil
		}
	}
	return "", "", fmt.Errorf("aicert: JUDGE=claude_cli needs a credential in the environment — set %s "+
		"(CLAUDE_CODE_OAUTH_TOKEN is a subscription token from `claude setup-token`; "+
		"`make e2e-ai` reads it from .env.local)", strings.Join(claudeCLICredentials, ", "))
}

// cliPassthrough is what the CLI inherits beyond HOME, PATH and its credential:
// where to write temporary files, and how this host reaches the network. Not
// ANTHROPIC_BASE_URL: an operator's gateway would receive the subscription
// token, and grade on a model this lane did not name.
var cliPassthrough = []string{
	"TMPDIR",
	"HTTPS_PROXY", "https_proxy", "HTTP_PROXY", "http_proxy", "NO_PROXY", "no_proxy",
}

// cliEnv is the whole environment the CLI runs in. Built rather than inherited:
// an inherited HOME would load the operator's own CLAUDE.md and memory, and an
// inherited second credential would let the CLI pick one this lane did not.
func cliEnv(lookup config.Lookup, home, credential string) []string {
	env := []string{"HOME=" + home, "PATH=" + lookup("PATH"), credential + "=" + lookup(credential)}
	for _, name := range cliPassthrough {
		if value := lookup(name); value != "" {
			env = append(env, name+"="+value)
		}
	}
	return env
}

// strippedWire is req as the CLI will receive it, after the SecretStripper.
func strippedWire(ctx context.Context, req model.Request) (cliWire, error) {
	if len(req.Tools) > 0 || len(req.Attachments) > 0 {
		return cliWire{}, fmt.Errorf("aicert: the claude_cli judge carries text only: %w", model.ErrRequestRejected)
	}
	prompt, err := cliPrompt(req.Messages)
	if err != nil {
		return cliWire{}, err
	}
	wire := cliWire{System: req.System, Prompt: prompt}
	stripped, _, err := ai.SendablePayload(ctx, wire, req.SecretStripper)
	if err != nil {
		return cliWire{}, err
	}
	if err := json.Unmarshal(stripped, &wire); err != nil {
		return cliWire{}, fmt.Errorf("aicert: the stripped claude_cli judge request no longer parses: %w", err)
	}
	return wire, nil
}

// cliPrompt folds a conversation into the one user turn `claude -p` takes. A
// retry arrives as the ask, the reply that failed and the feedback on it, so
// the earlier reply is labelled as the grader's own rather than read as input.
func cliPrompt(messages []model.Message) (string, error) {
	if len(messages) == 0 {
		return "", errors.New("aicert: the claude_cli judge was handed a request with no message")
	}
	turns := make([]string, 0, len(messages))
	for _, m := range messages {
		if m.Role == roleUser {
			turns = append(turns, m.Content)
			continue
		}
		turns = append(turns, "[Your previous reply]\n"+m.Content)
	}
	return strings.Join(turns, "\n\n"), nil
}
