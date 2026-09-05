// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// Replaying a write that names no record.
//
// The surface promises every tool takes an idempotency_key and that a replay
// returns the same result. A write to VOCABULARY named no record, so the
// replay had nothing to re-read and refused — an agent retrying a create after
// a timeout was told the call never happened, and could coin a second word.
//
// For those the object grant IS what the handler checked, so it is what the
// replay checks. These cases hold the three states apart: a grant still held,
// a grant since revoked, and a tool that names neither a record nor a grant.

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// norecordReader answers every read, so a case that reaches it is measuring
// the wrong thing: these answers name no record at all.
type norecordReader struct{ reads int }

func (r *norecordReader) Read(context.Context, datasource.EntityRef) (datasource.Record, error) {
	r.reads++
	return datasource.Record{}, nil
}

func tagWriter(t *testing.T, grants map[string]principal.ObjectGrant) context.Context {
	t.Helper()
	return principal.WithActor(
		principal.WithWorkspaceID(context.Background(), ids.NewV7()),
		principal.Principal{
			Type: principal.PrincipalAgent, ID: "agent:tools",
			Permissions: principal.Permissions{
				Objects: grants, RowScope: principal.RowScopeAll,
			},
			Scopes: principal.NewScopeSet(principal.ScopeWrite, principal.ScopeRead),
		})
}

// recordlessSpec is a create_tag-shaped tool: it answers with a word rather
// than a record, and says which grant a replay re-proves.
func recordlessSpec(grant *mcp.ReplayGrant) mcp.ToolSpec {
	return mcp.ToolSpec{
		Name: "create_tag", Title: "Create a tag", Version: "v1",
		Description:   "Coin a word.",
		RequiredScope: principal.ScopeWrite,
		Tier:          mcp.TierAutoExecute,
		ReplayGrant:   grant,
		InputSchema:   json.RawMessage(`{"type":"object"}`),
	}
}

func servedRecordless(
	ctx context.Context, t *testing.T, grant *mcp.ReplayGrant,
) (json.RawMessage, *norecordReader, error) {
	t.Helper()
	reader := &norecordReader{}
	r := NewRegistry(nil, auth.NewGate(fullSeatAuthority{}), WithReplayReader(reader))
	r.Register(&fakeTool{spec: recordlessSpec(grant)})
	// An envelope citing no evidence, which is what a vocabulary write records.
	recorded := json.RawMessage(`{"result":{"id":"t1","name":"Renewal"}}`)
	out, err := r.ServeRecorded(ctx, "create_tag", recorded, 1)
	return out, reader, err
}

func TestARecordLessWriteReplaysOnItsOwnGrant(t *testing.T) {
	t.Parallel()
	ctx := tagWriter(t, map[string]principal.ObjectGrant{"tag": {Create: true}})

	out, reader, err := servedRecordless(
		ctx, t, &mcp.ReplayGrant{Object: "tag", Action: principal.ActionCreate})
	if err != nil {
		t.Fatalf("replaying a tag create: %v", err)
	}
	if string(out) == "" {
		t.Error("the replay served nothing; want the original answer back")
	}
	// Nothing was re-read, because there is nothing to re-read. A case that
	// touched the reader would be proving the record path over again.
	if reader.reads != 0 {
		t.Errorf("the replay re-read %d record(s); a vocabulary write names none",
			reader.reads)
	}
}

// The point of re-checking rather than replaying: a passport whose grant was
// withdrawn after the original call is refused.
func TestARecordLessReplayIsRefusedOnceTheGrantIsGone(t *testing.T) {
	t.Parallel()
	ctx := tagWriter(t, map[string]principal.ObjectGrant{"tag": {Read: true}})

	_, _, err := servedRecordless(
		ctx, t, &mcp.ReplayGrant{Object: "tag", Action: principal.ActionCreate})
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("replaying without the grant = %v, want not-found — a caller "+
			"learns no more from a withdrawn grant than from a record they "+
			"can no longer see", err)
	}
}

// Record-less is not the same as authority-less. A tool that names neither
// keeps the refusal, because nothing about that answer can be re-established.
func TestAnAnswerWithNeitherRecordNorGrantIsStillRefused(t *testing.T) {
	t.Parallel()
	ctx := tagWriter(t, map[string]principal.ObjectGrant{"tag": {Create: true}})

	if _, _, err := servedRecordless(ctx, t, nil); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("replaying an answer with no record and no grant = %v, want not-found", err)
	}
}

// The vocabulary tools have to DECLARE the grant, or the path above is code
// nothing reaches. Each one's grant is its own handler's: create_tag's store
// call requires `create` and update_tag's requires `update`, and a replay that
// re-proved the wrong one would admit a caller the original call refused.
func TestTheVocabularyWritesDeclareTheGrantTheirHandlersCheck(t *testing.T) {
	t.Parallel()
	want := map[string]principal.Action{
		"create_tag": principal.ActionCreate,
		"update_tag": principal.ActionUpdate,
	}
	for _, tool := range []mcp.Tool{createTag{}, updateTag{}} {
		action, vocabulary := want[tool.Spec().Name]
		if !vocabulary {
			continue
		}
		grant := tool.Spec().ReplayGrant
		if grant == nil {
			t.Errorf("%s answers with a word rather than a record and declares no "+
				"ReplayGrant, so its retry is refused — an agent that timed out "+
				"can coin a second one", tool.Spec().Name)
			continue
		}
		if grant.Object != tagRecordType || grant.Action != action {
			t.Errorf("%s replays against %s:%s, want %s:%s — its own handler's",
				tool.Spec().Name, grant.Object, grant.Action, tagRecordType, action)
		}
		delete(want, tool.Spec().Name)
	}
	for name := range want {
		t.Errorf("%s is not among the tag vocabulary tools any more; this case "+
			"names a tool the surface does not have", name)
	}
}
