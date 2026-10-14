// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agentvolume

import (
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestTheStateIsTheWorstAllowance(t *testing.T) {
	for _, tc := range []struct {
		name  string
		usage Usage
		want  UsageState
	}{
		{"untouched", Usage{Reads: CounterUsage{Limit: 100}}, UsageOK},
		{"just under 80%", Usage{Reads: CounterUsage{Used: 79, Limit: 100}}, UsageOK},
		{"at 80%", Usage{Writes: CounterUsage{Used: 8, Limit: 10}}, UsageNear},
		{"one used up", Usage{Reads: CounterUsage{Used: 1, Limit: 100}, Egress: CounterUsage{Used: 20, Limit: 20}}, UsageStopped},
	} {
		if got := tc.usage.State(); got != tc.want {
			t.Errorf("%s: state %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestUsageWithoutACounterStoreIsUnanswerable(t *testing.T) {
	for name, meter := range map[string]*Meter{"no store": New(nil, Limits{}, time.Hour), "unmetered": Unmetered()} {
		if _, err := meter.Usage(t.Context(), ids.NewV7(), []string{"x"}); !errors.Is(err, ErrUnanswerable) {
			t.Errorf("%s: usage read answered %v, want ErrUnanswerable", name, err)
		}
	}
}
