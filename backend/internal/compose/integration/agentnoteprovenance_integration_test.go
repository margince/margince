// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A note an assistant writes carries source `manual`, like one typed in the
// app. The read a tool client gets names the assistant in recorded_by_agent,
// and a scheduled run's note says what it found in its body.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
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

// readNoteProvenance reads one note back through the composed tool registry,
// as an MCP client does.
func readNoteProvenance(ctx context.Context, t *testing.T, e *Env, note ids.UUID) noteProvenance {
	t.Helper()
	out, err := compose.NewRegistry(e.Pool, compose.SendPath{}).Invoke(ctx, "read_record",
		json.RawMessage(fmt.Sprintf(`{"record_type":"activity","id":%q}`, note)))
	if err != nil {
		t.Fatalf("read_record: %v", err)
	}
	var read noteProvenance
	if err := json.Unmarshal(ToolPayload(t, out), &read); err != nil {
		t.Fatal(err)
	}
	return read
}

// riskNote files a note on a contact through the activities provider, the
// seam log_activity reaches.
func riskNote(ctx context.Context, t *testing.T, e *Env, body string) (datasource.EntityRef, error) {
	t.Helper()
	fields := map[string]any{
		"kind": "note", "subject": "Risk assessment",
		"links": []map[string]any{{"entity_type": "contact", "entity_id": e.SeedContact(t, "Rita Risk", nil).String()}},
	}
	if body != "" {
		fields["body"] = body
	}
	return activities.NewProvider(e.DB()).Create(ctx, datasource.CreateInput{
		EntityType: datasource.EntityActivity, Source: "manual", Fields: fields,
	})
}

func TestAToolReadSaysAnAssistantWroteItsNote(t *testing.T) {
	e := Setup(t)
	assistant := e.AgentFor(t, e.AdminUser, nil, AdminPerms)

	for name, writer := range map[string]context.Context{
		"a chat assistant": assistant,
		"a scheduled run":  principal.WithAgentRunID(assistant, ids.NewV7()),
	} {
		t.Run(name, func(t *testing.T) {
			created, err := riskNote(writer, t, e, "No reply in three weeks.")
			if err != nil {
				t.Fatalf("filing the note: %v", err)
			}
			read := readNoteProvenance(e.Admin(), t, e, created.ID)
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
	e := Setup(t)
	created, err := riskNote(e.Admin(), t, e, "Spoke to them at the fair.")
	if err != nil {
		t.Fatalf("typing the note: %v", err)
	}

	read := readNoteProvenance(e.Admin(), t, e, created.ID)
	if read.Fields.RecordedByAgent == nil || *read.Fields.RecordedByAgent {
		t.Errorf("recorded_by_agent = %v, want false on a note a member typed", read.Fields.RecordedByAgent)
	}
}

func TestAScheduledRunsNoteWithoutABodyIsRefused(t *testing.T) {
	e := Setup(t)
	run := principal.WithAgentRunID(e.AgentFor(t, e.AdminUser, nil, AdminPerms), ids.NewV7())

	_, err := riskNote(run, t, e, "")
	var refusal *httperr.DetailedError
	if !errors.As(err, &refusal) || len(refusal.Fields) != 1 || refusal.Fields[0].Field != "body" {
		t.Fatalf("a run's note with only a subject answered %v, want a refusal naming the body", err)
	}
	if n := e.WsScalar(t, `SELECT count(*)::text FROM activity WHERE kind = 'note' AND origin = 'agent'`); n != "0" {
		t.Errorf("%s run notes were written, want none", n)
	}
}
