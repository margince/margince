// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// An activity and a task name where they came from, as the other records do.

import (
	"errors"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
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
	subject := "Met at the fair"
	in, err := LogActivityInputFrom(crmcontracts.CreateActivityRequest{
		Kind: crmcontracts.CreateActivityRequestKindCreateActivityRequestKindNote, Subject: &subject, Source: "  manual ",
	})
	if err != nil || in.Source != "manual" {
		t.Errorf("source = %q, %v; want manual", in.Source, err)
	}
}

// A source off the channel list is refused on both doors, a writer's older word
// among them. No client can file a row a reader takes for that writer's.
func TestAnActivityAndATaskRefuseASourceOffTheChannelList(t *testing.T) {
	t.Parallel()
	for _, origin := range []string{"hubspot_import:1", "meeting_notes", "ui"} {
		_, noteErr := LogActivityInputFrom(crmcontracts.CreateActivityRequest{
			Kind: crmcontracts.CreateActivityRequestKindCreateActivityRequestKindNote, Subject: new("Met at the fair"), Source: origin,
		})
		_, taskErr := TaskInputFrom(crmcontracts.CreateTaskRequest{Subject: "Call back", Source: origin})
		for name, err := range map[string]error{"note": noteErr, "task": taskErr} {
			refused, ok := errors.AsType[*provenance.RecordSourceError](err)
			if !ok {
				t.Errorf("%s with source %q: err = %v, want the source list refusal", name, origin, err)
				continue
			}
			if field, code, _ := refused.FieldFault(); field != "source" || code != "invalid_source" {
				t.Errorf("%s with source %q: refused on %s with %s, want source and invalid_source", name, origin, field, code)
			}
		}
	}
}

func TestAnActivityAndATaskRefuseASourcePastTheCapBeforeTheChannelList(t *testing.T) {
	t.Parallel()
	origin := strings.Repeat("é", provenance.SourceIDMax+1)
	_, noteErr := LogActivityInputFrom(crmcontracts.CreateActivityRequest{
		Kind: crmcontracts.CreateActivityRequestKindCreateActivityRequestKindNote, Source: origin,
	})
	_, taskErr := TaskInputFrom(crmcontracts.CreateTaskRequest{Subject: "Call back", Source: origin})
	for name, err := range map[string]error{"note": noteErr, "task": taskErr} {
		refused, ok := errors.AsType[*provenance.SourceIDError](err)
		if !ok {
			t.Errorf("%s with an oversized source: err = %v, want the length refusal", name, err)
			continue
		}
		if field, code, _ := refused.FieldFault(); field != "source" || code != "too_long" || refused.FieldMax() != provenance.SourceIDMax {
			t.Errorf("%s refused on %s with %s and limit %d, want source/too_long at %d", name, field, code, refused.FieldMax(), provenance.SourceIDMax)
		}
	}
}
