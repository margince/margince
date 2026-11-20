// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package runner

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// The model reads a refusal in the surface's words, never in the error's own.
// An unclassified fault's text, such as a driver error, stays in the log.
func TestARefusalReachesTheModelInTheSurfacesWords(t *testing.T) {
	const raw = "pg: dial tcp 10.0.0.7:5432: connection refused"
	surface := &fakeSurface{
		errs:      map[string]error{"read_record": errors.New(raw)},
		explained: "The tool failed for an internal reason.",
	}
	brain := &scriptedBrain{texts: []string{
		`{"tool":"read_record","args":{}}`,
		`{"final":{"summary":"done without the read"}}`,
	}}
	res, err := New(surface, brain).Run(context.Background(), Job{Goal: "g", Tools: []string{"read_record"}})
	if err != nil {
		t.Fatal(err)
	}
	var prompt strings.Builder
	for _, m := range brain.requests[len(brain.requests)-1].Messages {
		prompt.WriteString(m.Content)
	}
	if strings.Contains(prompt.String(), raw) || strings.Contains(res.Steps[0].Observation, raw) {
		t.Fatalf("the fault's own text reached the model or the trace: %q", res.Steps[0].Observation)
	}
	if !strings.Contains(prompt.String(), surface.explained) {
		t.Fatalf("the model was not told the surface's explanation: %q", prompt.String())
	}
}
