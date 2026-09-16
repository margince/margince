// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

// Keeping the stripper's answer, which every real adapter used to discard.
//
// model.StripReport says in its own doc what it is for — "StripReport says what
// was removed, FOR THE AUDIT TRAIL" — and then anthropic.go, gemini.go and
// ollama.go each took it as `_`. A control that exists for an audit and leaves
// no witness cannot answer the one question an audit asks: what did we remove
// from that request.
//
// Recorded by WRAPPING the stripper rather than threading the report back
// through four adapters. The router already owns the one place where a
// request's stripper is settled, and the one place where the trace row it will
// write is finalised; putting the recorder between them means a fifth adapter
// records its strips by existing, rather than by remembering to.

import (
	"context"
	"sort"
	"sync"

	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// stripRecorder wraps a stripper and accumulates what it removed.
//
// Additive across calls, because one traced attempt can marshal more than one
// body — a structured-output retry inside the adapter is still this attempt —
// and the question the row answers is what left the process under this trace,
// not what the last marshalling happened to contain.
//
// Mutex-guarded: an adapter is free to strip from more than one goroutine, and
// a counter that undercounted would be a quieter version of the defect this
// closes.
type stripRecorder struct {
	inner model.SecretStripper
	mu    sync.Mutex
	count int
	kinds map[string]bool
}

func newStripRecorder(inner model.SecretStripper) *stripRecorder {
	return &stripRecorder{inner: inner, kinds: map[string]bool{}}
}

// Strip delegates and remembers. A nil inner strips nothing, which is the
// no-stripper case the router leaves intact rather than a reason to be absent:
// "nothing was removed" is an honest answer and the count says so.
func (s *stripRecorder) Strip(ctx context.Context, payload []byte) ([]byte, model.StripReport, error) {
	if s.inner == nil {
		return payload, model.StripReport{}, nil
	}
	out, report, err := s.inner.Strip(ctx, payload)
	if err != nil {
		return out, report, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.count += report.Findings
	for _, k := range report.Kinds {
		s.kinds[k] = true
	}
	return out, report, nil
}

// report is what to write on the trace row: a count and the sorted kinds.
func (s *stripRecorder) report() (int, []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	kinds := make([]string, 0, len(s.kinds))
	for k := range s.kinds {
		kinds = append(kinds, k)
	}
	sort.Strings(kinds)
	return s.count, kinds
}
