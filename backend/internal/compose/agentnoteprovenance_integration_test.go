// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A note an assistant writes carries source `manual`, like one typed in the
// app. The read a tool client gets names the assistant in recorded_by_agent,
// and a scheduled run's note says what it found in its body.

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// noteProvenance is what read_record answers about who wrote a note.
type noteProvenance struct {
	Fields struct {
		Source          string `json:"source"`
		RecordedByAgent *bool  `json:"recorded_by_agent"`
	} `json:"fields"`
}

// readNoteProvenance reads one note back through the composed tool registry.
func (f accountFixture) readNoteProvenance(ctx context.Context, t *testing.T, note ids.UUID) noteProvenance {
	t.Helper()
	var read noteProvenance
	invokeAnswer(ctx, t, composedRegistryOn(InstallationDB(f.e.Pool)), "read_record",
		fmt.Sprintf(`{"record_type":"activity","id":%q}`, note), &read)
	return read
}

// riskNote files a note on the deal through the activities provider, the seam
// log_activity reaches.
func (f accountFixture) riskNote(ctx context.Context, body string) (datasource.EntityRef, error) {
	fields := map[string]any{
		"kind": "note", "subject": "Risk assessment",
		"links": []map[string]any{{"entity_type": "deal", "entity_id": f.deal.String()}},
	}
	if body != "" {
		fields["body"] = body
	}
	return activities.NewProvider(f.e.DB()).Create(ctx, datasource.CreateInput{
		EntityType: datasource.EntityActivity, Source: "manual", Fields: fields,
	})
}

func TestAToolReadSaysAnAssistantWroteItsNote(t *testing.T) {
	f := newAccountFixture(t)
	assistant := f.e.AgentFor(t, f.e.AdminUser, nil, integration.AdminPerms)

	for name, writer := range map[string]context.Context{
		"a chat assistant": assistant,
		"a scheduled run":  principal.WithAgentRunID(assistant, ids.NewV7()),
	} {
		t.Run(name, func(t *testing.T) {
			created, err := f.riskNote(writer, "No reply in three weeks.")
			if err != nil {
				t.Fatalf("filing the note: %v", err)
			}
			read := f.readNoteProvenance(assistant, t, created.ID)
			if read.Fields.Source != "manual" {
				t.Errorf("source = %q, want manual", read.Fields.Source)
			}
			if read.Fields.RecordedByAgent == nil || !*read.Fields.RecordedByAgent {
				t.Errorf("recorded_by_agent = %v, want true: a client cannot tell this note from a typed one",
					read.Fields.RecordedByAgent)
			}
		})
	}
}

func TestAToolReadSaysNoAssistantWroteATypedNote(t *testing.T) {
	f := newAccountFixture(t)
	created, err := f.riskNote(f.e.Admin(), "Spoke to them at the fair.")
	if err != nil {
		t.Fatalf("typing the note: %v", err)
	}
	assistant := f.e.AgentFor(t, f.e.AdminUser, nil, integration.AdminPerms)

	read := f.readNoteProvenance(assistant, t, created.ID)
	if read.Fields.RecordedByAgent == nil || *read.Fields.RecordedByAgent {
		t.Errorf("recorded_by_agent = %v, want false on a note a member typed", read.Fields.RecordedByAgent)
	}
}

func TestAScheduledRunsNoteWithoutABodyIsRefused(t *testing.T) {
	f := newAccountFixture(t)
	run := principal.WithAgentRunID(f.e.AgentFor(t, f.e.AdminUser, nil, integration.AdminPerms), ids.NewV7())

	_, err := f.riskNote(run, "")
	var refusal *httperr.DetailedError
	if !errors.As(err, &refusal) || len(refusal.Fields) != 1 || refusal.Fields[0].Field != "body" {
		t.Fatalf("a run's note with only a subject answered %v, want a refusal naming the body", err)
	}
	if n := f.e.WsScalar(t, `SELECT count(*)::text FROM activity WHERE kind = 'note' AND origin = 'agent'`); n != "0" {
		t.Errorf("%s run notes were written, want none", n)
	}
}
