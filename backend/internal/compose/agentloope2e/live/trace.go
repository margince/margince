// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package live

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"

	"github.com/margince/margince/backend/internal/compose/agentloope2e"
	"github.com/margince/margince/backend/internal/compose/aitasks"
	"github.com/margince/margince/backend/internal/modules/agents/runner"
	"github.com/margince/margince/backend/internal/modules/ai"
)

// Recorder keeps every model call of a run in memory, as the call store the
// router writes to. A trace then shows what the model was asked and answered.
type Recorder struct {
	mu    sync.Mutex
	calls []ai.Call
}

// Record keeps the attempts of one logical call.
func (r *Recorder) Record(_ context.Context, attempts []ai.Call) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, attempts...)
	return nil
}

// EnsureConfig has no table to plant a row in.
func (r *Recorder) EnsureConfig(context.Context, ai.ConfigSnapshot) error { return nil }

// Calls is what was recorded so far.
func (r *Recorder) Calls() []ai.Call {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]ai.Call(nil), r.calls...)
}

// Trace is one run laid out for a reader to open after the fact. It holds
// everything the run touched, so it goes under the gitignored .tmp folder and
// never into a commit.
type Trace struct {
	Recovery *aitasks.LoopRecoveryMeasure `json:"recovery,omitempty"`
	Scenario string                       `json:"scenario"`
	Binding  agentloope2e.Binding         `json:"binding"`
	Run      int                          `json:"run"`
	Status   string                       `json:"status"`
	Steps    []runner.Step                `json:"steps"`
	Calls    []ai.Call                    `json:"model_calls"`
	Failures []agentloope2e.Failure       `json:"failures"`
	Writes   []agentloope2e.Write         `json:"writes"`
}

// Write files the trace under dir and answers its path.
func (t Trace) Write(dir string) (string, error) {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return "", err
	}
	body, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, fmt.Sprintf("%s.%s.run%d.json", t.Scenario, t.Binding.Key(), t.Run))
	return path, os.WriteFile(path, body, 0o600)
}

// Answered says whether the model answered at least one call, rather than every
// call failing on the route. A run that ended early with none points at the key
// or the model name, not at the agent.
func (r *Recorder) Answered() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.ContainsFunc(r.calls, func(call ai.Call) bool {
		return call.Kind != "embedding" && call.IsTerminal && !call.Degraded && call.ErrorSentinel == ""
	})
}
