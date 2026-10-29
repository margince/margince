// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// confirm_meeting_ready and undo_meeting_ready: an agent saying its user is
// ready for a meeting, as the user could unaided. The activities store owns
// the write; compose binds it as MeetingReadiness.

import (
	"context"
	"encoding/json"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// MeetingReadiness is the caller's own preparation for a meeting. Read answers
// false for a meeting the caller does not attend or may not read.
type MeetingReadiness interface {
	Read(ctx context.Context, activityID ids.UUID) (MeetingPreparationResult, bool, error)
	Confirm(ctx context.Context, activityID ids.UUID, revision string) (MeetingPreparationResult, error)
	Withdraw(ctx context.Context, activityID ids.UUID, revision string) (MeetingPreparationResult, error)
}

// MeetingPreparationResult is the contract's MeetingPreparation.
type MeetingPreparationResult struct {
	ActivityID ids.UUID `json:"activity_id"`
	State      string   `json:"state"`
	Revision   string   `json:"revision"`
	Changed    []string `json:"changed"`
}

// The two verbs, as change_activities routes to them.
const (
	confirmMeetingReadyTool = "confirm_meeting_ready"
	undoMeetingReadyTool    = "undo_meeting_ready"
)

// RegisterMeetingReadinessTools wires both verbs; a nil seam wires neither.
func RegisterMeetingReadinessTools(r *Registry, readiness MeetingReadiness) {
	if readiness == nil {
		return
	}
	r.Register(meetingReady{readiness: readiness, name: confirmMeetingReadyTool, op: "confirmMeetingReady", copy: confirmMeetingReadyCopy})
	r.Register(meetingReady{readiness: readiness, name: undoMeetingReadyTool, op: "undoMeetingReady", copy: undoMeetingReadyCopy})
}

// meetingReady is either verb: they take the same arguments and differ only
// in which write they run.
type meetingReady struct {
	readiness MeetingReadiness
	name, op  string
	copy      toolCopy
}

func (t meetingReady) Spec() mcp.ToolSpec {
	title := "Say the user is ready for a meeting"
	if t.name == undoMeetingReadyTool {
		title = "Take back the user's readiness for a meeting"
	}
	return mcp.ToolSpec{
		Name: t.name, Title: title, Version: toolVersionV1,
		Description: t.copy.render(), Instead: t.copy.Instead,
		RequiredScope: principal.ScopeWrite, Tier: mcp.TierAutoExecute, Unlisted: true,
		OpenAPIOp: t.op,
		InputSchema: schema(`{"type":"object","required":["activity_id","preparation_revision"],"properties":{
			"activity_id":{"type":"string","format":"uuid","description":"The meeting"},
			"preparation_revision":{"type":"string","minLength":1,"maxLength":64,
				"description":"The preparation revision prep_for_meeting answered"}},
			"additionalProperties":false}`),
		OutputSchema: schemaFor[MeetingPreparationResult](),
	}
}

func (t meetingReady) Handle(ctx context.Context, in json.RawMessage) (json.RawMessage, error) {
	var args struct {
		ActivityID ids.UUID `json:"activity_id"`
		Revision   string   `json:"preparation_revision"`
	}
	if err := decodeArgs(in, &args); err != nil {
		return nil, err
	}
	write := t.readiness.Confirm
	if t.name == undoMeetingReadyTool {
		write = t.readiness.Withdraw
	}
	out, err := write(ctx, args.ActivityID, args.Revision)
	if err != nil {
		return nil, err
	}
	return json.Marshal(out)
}
