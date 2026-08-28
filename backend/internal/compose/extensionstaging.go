// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// How a served extension operation puts a call in front of a human.
//
// The core gate parks a refused 🟡 call as an approval only for a tool that can
// describe its own staging subject (agents.Stager). Until this file, no
// extension tool could: the adapted tool is data plus a handler, and the
// composed declaration said nothing about what the call was ABOUT. So a served
// 🟡 tool would have been refused on every call with nowhere to land, and
// extensiontools.go rejected the tier outright rather than serve a capability
// that could only ever fail.
//
// What closes it is the unit saying which argument carries the subject's id and
// which of its own tables the row lives in (pkg/extension's Subject). That is
// enough for all three things a staged row needs: the inbox shows the row, the
// existence probe proves it still exists, and the decision authority is the
// grant the operation itself gates on.
//
// The three checks below run in the order they do because each is only
// meaningful after the one before it. The GRANT first, so a principal who may
// not touch these records cannot mint approval rows for them by asking; then
// the ARGUMENT, because a call with no subject names nothing; then the ROW,
// because an approval against a row that is not there is one nobody can ever
// release.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// StageInfo describes the call this tool would put in front of a human.
//
// Implemented on the adapted tool rather than on anything a unit writes: a
// handler is ordinary Go, and a handler that described its own staging could
// name a row it does not own, or a summary that says one thing while the call
// does another. Everything here comes from the COMPOSED DECLARATION and the
// arguments — nothing an extension author can supply at call time.
func (t extensionTool) StageInfo(ctx context.Context, in json.RawMessage) (agents.StageInfo, error) {
	if t.subject.IsZero() {
		// Unreachable while adaptExtensionTool refuses a served 🟡 tool with no
		// subject, and stated rather than assumed: a tool that reached here
		// without one would stage against the zero row.
		return agents.StageInfo{}, fmt.Errorf("compose: tool %q stages with no declared subject", t.spec.Name)
	}
	// The same grant Handle takes, taken here too. Handle runs only on the
	// approved retry, so without this a principal holding the passport scope
	// but not the unit's object grant could park approval rows against records
	// they may not touch — and a human would be asked to release a call its
	// asker was never allowed to make.
	if err := auth.Require(ctx, t.rbacObject, principal.Action(t.rbacAction)); err != nil {
		return agents.StageInfo{}, err
	}
	id, err := t.stagedSubjectID(in)
	if err != nil {
		return agents.StageInfo{}, err
	}
	if err := t.subjectRowExists(ctx, id); err != nil {
		return agents.StageInfo{}, err
	}
	return agents.StageInfo{
		TargetType: t.subject.Table,
		TargetID:   id,
		// No version. A unit's row carries no version this surface can read, so
		// an approval here binds to the row rather than to the row AS IT WAS —
		// which is the honest claim, and why the summary names the operation
		// rather than describing a change.
		Summary: fmt.Sprintf("%s (%s)", t.spec.Title, t.unit),
	}, nil
}

// stagedSubjectID reads the declared argument out of the call.
//
// A missing or unreadable id is the CALLER's fault and is answered as one: the
// staging read failing is the real answer to the call, not "needs approval",
// which is what Registry.stageRefusedCall does with an error from here.
func (t extensionTool) stagedSubjectID(in json.RawMessage) (ids.UUID, error) {
	var args map[string]json.RawMessage
	if err := json.Unmarshal(in, &args); err != nil {
		return ids.UUID{}, &agents.BadArgsError{Cause: fmt.Errorf("arguments are not an object: %w", err)}
	}
	raw, sent := args[t.subject.Arg]
	if !sent {
		return ids.UUID{}, &agents.BadArgsError{
			Cause: fmt.Errorf("%s names the record this call is about and was not sent", t.subject.Arg),
		}
	}
	var text string
	if err := json.Unmarshal(raw, &text); err != nil {
		return ids.UUID{}, &agents.BadArgsError{Cause: fmt.Errorf("%s must be a uuid string", t.subject.Arg)}
	}
	id, err := ids.Parse(text)
	if err != nil {
		return ids.UUID{}, &agents.BadArgsError{Cause: fmt.Errorf("%s is not a uuid: %w", t.subject.Arg, err)}
	}
	return id, nil
}

// subjectRowExists refuses a staging against a row that is not there.
//
// Not-found rather than a staged approval, because the two are different
// answers and only one of them is true: parking a call against a row nobody can
// find produces an inbox item that can never be released or rejected, which is
// the dead authority object this module's target-visibility rules exist to
// prevent. The read is workspace-scoped like every other, so a row in another
// workspace is not found here either.
func (t extensionTool) subjectRowExists(ctx context.Context, id ids.UUID) error {
	// The same binding a handler's Runtime reaches the installation through,
	// read per call for the same reason: the ordering between binding and
	// building a registry cannot matter, only the ordering against the first
	// call.
	pool := boundExtensionRuntime().pool
	if pool == nil {
		return errors.New("compose: no pool is bound for the extension runtime, so a staged row could not be " +
			"proven to exist — and an approval against an unprovable row is one nobody can release")
	}
	return database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
		// The approvals module's own probe, asked one frame earlier. Writing a
		// second query here is how a staging check and the inbox that later
		// reads it come to disagree about what "still exists" means.
		exists, err := approvals.ExtensionTargetExists(ctx, tx, t.subject.Table, id)
		if err != nil {
			return fmt.Errorf("proving the staged %s row exists: %w", t.subject.Table, err)
		}
		if !exists {
			return apperrors.ErrNotFound
		}
		return nil
	})
}

// extensionStagingKinds is what the composed set tells the approvals module
// about the verbs it serves confirm-first: the kind, the table its staged rows
// live in, and the grant deciding one requires.
//
// Derived from the SAME adapted tools the registry serves, so a verb that can
// stage is a verb the inbox can decide — the two cannot be wired from different
// lists and disagree.
func extensionStagingKinds(tools []mcp.Tool) []approvals.ExtensionKind {
	var kinds []approvals.ExtensionKind
	for _, tool := range tools {
		t, adapted := tool.(extensionTool)
		if !adapted || t.subject.IsZero() {
			continue
		}
		kinds = append(kinds, approvals.ExtensionKind{
			Verb:        t.spec.Name,
			TargetTable: t.subject.Table,
			RbacObject:  t.rbacObject,
			RbacAction:  principal.Action(t.rbacAction),
		})
	}
	return kinds
}
