// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert

// The opt-in payload trace: a dev/tuning dump of every candidate and judge
// call's post-SecretStripper request and response — the SAME content the
// production router writes to ai_call_payload — to a JSONL file under a
// caller-named directory (MARGINCE_AICERT_TRACE). Off unless a directory is
// named: the lane's durable output is the verdict record, never the prompts.
// This exists so an author tuning a scenario can read exactly what the model
// saw and said without a database.

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"

	"github.com/margince/margince/backend/internal/modules/ai"
)

// tracedCall is one logical call as it would read joined across ai_call and
// ai_call_payload: the identifying metadata plus the two jsonb payload
// columns under their table names (request_payload / response_payload), so a
// trace line is the same shape a `SELECT` over those two tables would give.
// role, scenario, run and call are the trace-only context that tells two
// otherwise identical lines apart (which router, which repeat, which call
// within it) — the DB carries the same distinction across ai_call.task and the
// correlation/agent ids.
type tracedCall struct {
	Task     string `json:"task"`
	Role     string `json:"role"` // candidate | judge
	Scenario string `json:"scenario"`
	Run      int    `json:"run"`
	// Attempt is which drive of this run the call belongs to. A run whose
	// router exhausted every bound tier is driven again (driveRun), and the
	// discarded attempt has usually already written its candidate calls here —
	// so without this two blocks would sit under one run number, and a reader
	// would take the abandoned attempt's request for the one that was scored.
	// The same distinction Call draws inside one drive, one level up.
	Attempt int `json:"attempt"`
	// Call is this call's 1-based position within the run. A run is not one
	// call — a site retries, falls back, or turns a loop, and the judge is
	// asked twice on a parse failure — so without it a reader cannot tell the
	// draft that was served from the attempt it replaced.
	Call     int    `json:"call"`
	Tier     string `json:"tier"`
	Provider string `json:"provider"`
	ModelID  string `json:"model_id"`
	// ServedModel is what the provider reported serving, and ServedProvider the
	// UPSTREAM that served it when a broker sat in between. On the OpenAI-compat
	// wire the first is only our own request echoed back, so ServedProvider is
	// the field that actually attributes a row: one model id is served by hosts
	// differing in quantization, output ceiling and tail latency, and a run that
	// cannot name the host is pooling measurements of different things.
	ServedModel    string `json:"served_model"`
	ServedProvider string `json:"served_provider"`
	// FinishReason separates a truncated answer from a complete one — the
	// difference between a model that wrote the wrong shape and one that was cut
	// off before it could finish writing the right one.
	FinishReason    string          `json:"finish_reason"`
	TokensIn        int             `json:"tokens_in"`
	TokensOut       int             `json:"tokens_out"`
	ReasoningTokens int             `json:"reasoning_tokens"`
	LatencyMS       int64           `json:"latency_ms"`
	RequestPayload  json.RawMessage `json:"request_payload"`
	ResponsePayload json.RawMessage `json:"response_payload"`
}

// payloadTrace serializes tracedCall lines to one JSONL file for a whole
// run. A nil *payloadTrace is the disabled state — every method is a no-op
// on it — so the runner threads one value whether tracing is on or off.
type payloadTrace struct {
	sink *jsonlSink
	Path string // absolute, printed to stdout when the trace opens
}

// openPayloadTrace creates the trace file under dir (named by the run's
// timestamp so repeated runs never clobber each other) and prints its
// absolute path to stdout for the tuning loop. The caller gates on an empty
// dir (tracing off ⇒ a nil *payloadTrace whose methods no-op); dir here is
// always a real directory to write into.
func openPayloadTrace(dir string, stamp string) (*payloadTrace, error) {
	// Truncating, not appending: the filename carries the run's own timestamp,
	// so a trace never meets a previous run's lines to begin with.
	sink, err := openJSONLSink(dir, "aicert-trace-"+stamp+".jsonl", "payload trace",
		os.O_WRONLY|os.O_CREATE|os.O_TRUNC)
	if err != nil {
		return nil, err
	}
	return &payloadTrace{sink: sink, Path: sink.Path}, nil
}

// record appends one call's payloads to the trace. A call whose terminal
// attempt carries no Payload (capture off, or an error/cache-hit path that
// captures nothing) is skipped rather than written as a hollow line — the
// trace shows only calls it can actually explain.
func (t *payloadTrace) record(role string, task ai.Task, sc Scenario, run, attempt, call int, c ai.Call) error {
	if t == nil || c.Payload == nil {
		return nil
	}
	line := tracedCall{
		Task:            string(task),
		Role:            role,
		Scenario:        sc.Name,
		Run:             run,
		Attempt:         attempt,
		Call:            call,
		Tier:            string(c.Tier),
		Provider:        c.Provider,
		ModelID:         c.ModelID,
		ServedModel:     c.ServedModel,
		ServedProvider:  c.ServedProvider,
		FinishReason:    c.FinishReason,
		TokensIn:        c.TokensIn,
		TokensOut:       c.TokensOut,
		ReasoningTokens: c.ReasoningTokens,
		LatencyMS:       c.LatencyMS,
		RequestPayload:  c.Payload.Request,
		ResponsePayload: c.Payload.Response,
	}
	if err := encodeLine(t.sink, line); err != nil {
		return fmt.Errorf("aicert: trace line (%s %s): %w", task, role, err)
	}
	return nil
}

// traceCalls writes every call one run made to the trace, best-effort: a write
// failure is logged and swallowed here, never returned to the caller. The trace
// is an opt-in diagnostic side-channel; it must not become a new way for a
// completed, paid model call to fail — the same posture the production
// router already holds for this exact post-stripper content (ai/tracing.go:
// "payload capture must not become a new way for a working model call to
// fail"). The error is heard (logged with what and where), not ignored.
//
// Every call, not the last one: the trace exists so an author can read exactly
// what each model saw and said, and the reply a site served often comes from
// the second or third request it sent.
func traceCalls(ctx context.Context, t *payloadTrace, role string, task ai.Task, sc Scenario, run, attempt int, calls []ai.Call, log *slog.Logger) {
	for i, c := range calls {
		if err := t.record(role, task, sc, run, attempt, i+1, c); err != nil {
			log.WarnContext(ctx, "aicert: payload trace write failed — run continues",
				"task", string(task), "scenario", sc.Name, "role", role, "run", run, "attempt", attempt, "call", i+1, "err", err)
		}
	}
}

// close flushes and closes the underlying file. Nil-safe so the runner can
// defer it unconditionally.
func (t *payloadTrace) close() error {
	if t == nil {
		return nil
	}
	return t.sink.close()
}
