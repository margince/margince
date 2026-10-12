// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// An activity and a task name where they came from, as the other records do.

import (
	"errors"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
)

func TestAnActivityAndATaskMustNameTheirSource(t *testing.T) {
	t.Parallel()
	for _, origin := range []string{"", "   "} {
		_, noteErr := LogActivityInputFrom(crmcontracts.CreateActivityRequest{
			Kind: crmcontracts.CreateActivityRequestKindCreateActivityRequestKindNote, Source: origin,
		})
		_, taskErr := TaskInputFrom(crmcontracts.CreateTaskRequest{Subject: "Call back", Source: origin})
		for name, err := range map[string]error{"note": noteErr, "task": taskErr} {
			detailed, ok := errors.AsType[*httperr.DetailedError](err)
			if !ok || len(detailed.Fields) != 1 || detailed.Fields[0].Field != "source" {
				t.Errorf("%s with source %q: err = %v, want the required refusal naming source", name, origin, err)
			}
		}
	}
}

func TestAnActivityKeepsItsSourceTrimmed(t *testing.T) {
	t.Parallel()
	in, err := LogActivityInputFrom(crmcontracts.CreateActivityRequest{
		Kind: crmcontracts.CreateActivityRequestKindCreateActivityRequestKindNote, Source: "  manual ",
	})
	if err != nil || in.Source != "manual" {
		t.Errorf("source = %q, %v; want manual", in.Source, err)
	}
}
