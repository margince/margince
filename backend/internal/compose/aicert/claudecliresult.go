// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert

// Reading `claude -p --output-format json` back into a model.Response. Its own
// file because what the CLI prints is a different question from how it is run,
// and the honest-identity rule lives here: the served model comes from the
// CLI's own usage report, never from the --model flag.

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// cliResult is the part of the CLI's single JSON result this lane reads.
type cliResult struct {
	IsError    bool                     `json:"is_error"`
	Subtype    string                   `json:"subtype"`
	Result     string                   `json:"result"`
	StopReason string                   `json:"stop_reason"`
	ModelUsage map[string]cliModelUsage `json:"modelUsage"` //nolint:tagliatelle // the CLI names this field
}

// cliModelUsage is one model's share of a call, keyed by its full model id.
type cliModelUsage struct {
	InputTokens              int `json:"inputTokens"`              //nolint:tagliatelle // the CLI names this field
	OutputTokens             int `json:"outputTokens"`             //nolint:tagliatelle // the CLI names this field
	CacheReadInputTokens     int `json:"cacheReadInputTokens"`     //nolint:tagliatelle // the CLI names this field
	CacheCreationInputTokens int `json:"cacheCreationInputTokens"` //nolint:tagliatelle // the CLI names this field
}

// errCLIJudge marks every failure the CLI itself reported, so a test (and a
// reader of the run's error) can tell it from a harness fault.
var errCLIJudge = errors.New("the claude_cli judge failed")

// parseCLIResult turns one run's output into a Response. The CLI prints its
// JSON result on failure too, so stdout is read before the exit status: its
// own message says more than "exit status 1".
func parseCLIResult(stdout []byte, stderr string, runErr error) (model.Response, error) {
	var out cliResult
	if parseErr := json.Unmarshal(stdout, &out); parseErr != nil {
		if runErr != nil {
			return model.Response{}, fmt.Errorf("%w: the CLI exited (%v) without a JSON result; stderr: %s",
				errCLIJudge, runErr, clipped(stderr))
		}
		return model.Response{}, fmt.Errorf("%w: its output is not the JSON result --output-format json promises: %v",
			errCLIJudge, parseErr)
	}
	if out.IsError || runErr != nil {
		return model.Response{}, fmt.Errorf("%w (%s): %s", errCLIJudge, out.Subtype, clipped(out.Result))
	}
	if strings.TrimSpace(out.Result) == "" {
		return model.Response{}, fmt.Errorf("%w: the CLI reported success with an empty result", errCLIJudge)
	}
	served, ok := servedCLIModel(out.ModelUsage)
	if !ok {
		return model.Response{}, fmt.Errorf("%w: the CLI reported no model usage, so the model that graded is unknown", errCLIJudge)
	}
	resp := model.Response{Text: out.Result, ServedModel: served, FinishReason: cliFinishReason(out.StopReason)}
	// Summed over every model the call used: tokens spent are spent, whichever
	// model spent them. InputTokens is inclusive of both cache buckets, as the
	// port requires of every adapter.
	for _, u := range out.ModelUsage {
		resp.InputTokens += u.InputTokens + u.CacheReadInputTokens + u.CacheCreationInputTokens
		resp.OutputTokens += u.OutputTokens
		resp.CachedTokens += u.CacheReadInputTokens
		resp.CacheWriteTokens += u.CacheCreationInputTokens
	}
	return resp, nil
}

// servedCLIModel is the model that wrote the answer: the one that produced the
// most output, ties broken by name so a record never depends on map order.
func servedCLIModel(usage map[string]cliModelUsage) (string, bool) {
	names := make([]string, 0, len(usage))
	for name := range usage {
		names = append(names, name)
	}
	sort.Strings(names)
	served, most := "", -1
	for _, name := range names {
		if usage[name].OutputTokens > most {
			served, most = name, usage[name].OutputTokens
		}
	}
	return served, served != ""
}

// cliFinishReason normalizes the CLI's Anthropic stop reason to the port's
// vocabulary; one it does not know is passed through rather than guessed at.
func cliFinishReason(stop string) string {
	switch stop {
	case "end_turn", "stop_sequence":
		return "stop"
	case "max_tokens":
		return model.FinishReasonLength
	default:
		return stop
	}
}

// clipped keeps an error readable when the CLI prints a page of output.
func clipped(text string) string {
	text = strings.TrimSpace(text)
	if len(text) > cliStderrLimit {
		return text[:cliStderrLimit] + "…"
	}
	return text
}
