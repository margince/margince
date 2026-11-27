// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const (
	sourceOwnerMemo = "source_owner:"
	sourceAuditMemo = "source_audit:"
	sourceEventMark = "source_updated:"
)

type sourceChange struct {
	ChangeID      ids.UUID  `json:"change_id"`
	SystemChanged bool      `json:"system_changed"`
	BeforeSystem  *string   `json:"before_system"`
	AfterSystem   *string   `json:"after_system"`
	EntityType    string    `json:"entity_type"`
	ID            ids.UUID  `json:"id"`
	Before        string    `json:"before"`
	After         string    `json:"after"`
	DealID        *ids.UUID `json:"deal_id"`
}

var sourceEntities = map[string]bool{"contact": true, "company": true, "lead": true, "deal": true, "offer": true, "project": true}

func readSourceChanges(ctx context.Context, tx pgx.Tx) (map[string]sourceChange, error) {
	var raw []byte
	if err := tx.QueryRow(ctx, `SELECT coalesce(nullif(current_setting('margince.source_changes', true), ''), '{}')::jsonb`).Scan(&raw); err != nil {
		return nil, err
	}
	var changes map[string]sourceChange
	if err := json.Unmarshal(raw, &changes); err != nil {
		return nil, err
	}
	return changes, nil
}

func sourceAuditRows(ctx context.Context, tx pgx.Tx, action, entityType string, rows []auditRow) (map[int]sourceChange, error) {
	owner := database.CommitOwner(tx)
	if !database.Memoized(owner) {
		return nil, nil
	}
	changes, err := readSourceChanges(ctx, tx)
	if err != nil {
		return nil, err
	}
	matched := map[int]sourceChange{}
	for i := range rows {
		key := entityType + ":" + rows[i].entityID.String()
		change, found := changes[key]
		if !found {
			continue
		}
		if slices.Contains(AuditRedactionActions(), action) {
			delete(changes, key)
			continue
		}
		if len(rows[i].before) == 0 || len(rows[i].after) == 0 || foreignSource(rows[i].before, change.Before) || foreignSource(rows[i].after, change.After) {
			continue
		}
		if rows[i].before, err = sourceImage(rows[i].before, change.Before, change.SystemChanged, change.BeforeSystem); err != nil {
			return nil, err
		}
		if rows[i].after, err = sourceImage(rows[i].after, change.After, change.SystemChanged, change.AfterSystem); err != nil {
			return nil, err
		}
		if err := maskAuditColumns(entityType, &rows[i].before, &rows[i].after, &rows[i].evidence); err != nil {
			return nil, err
		}
		matched[i] = change
		delete(changes, key)
	}

	for _, change := range changes {
		if _, err := database.Remember(database.CommitOwner(tx), sourceOwnerMemo+change.ChangeID.String(), func() (context.Context, error) { return ctx, nil }); err != nil {
			return nil, err
		}
	}
	if len(matched) > 0 || slices.Contains(AuditRedactionActions(), action) {
		if err := saveSourceChanges(ctx, tx, changes); err != nil {
			return nil, err
		}
	}
	return matched, nil
}

func saveSourceChanges(ctx context.Context, tx pgx.Tx, changes map[string]sourceChange) error {
	raw, err := json.Marshal(changes)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `SELECT set_config('margince.source_changes',$1,true)`, string(raw))
	return err
}

func sourceImage(raw []byte, source string, systemChanged bool, system *string) ([]byte, error) {
	image := map[string]json.RawMessage{}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &image); err != nil {
			return nil, fmt.Errorf("source change needs an object audit image: %w", err)
		}
	}
	if image == nil {
		image = map[string]json.RawMessage{}
	}
	value, err := json.Marshal(source)
	if err != nil {
		return nil, err
	}
	if systemChanged {
		systemValue, err := json.Marshal(system)
		if err != nil {
			return nil, err
		}
		image["source_system"] = systemValue
	}
	image["source"] = value
	return json.Marshal(image)
}

type sourceCommitState struct {
	audits map[ids.UUID]sourceChange
	owners map[ids.UUID]context.Context
	ctx    context.Context
}

func keepSourceAudits(ctx context.Context, tx pgx.Tx, idsWritten []ids.UUID, matched map[int]sourceChange) error {
	owner := database.CommitOwner(tx)
	if !database.Memoized(owner) {
		return nil
	}
	state, err := database.Remember(owner, "source_commit_state", func() (*sourceCommitState, error) {
		return &sourceCommitState{audits: map[ids.UUID]sourceChange{}, owners: map[ids.UUID]context.Context{}, ctx: ctx}, nil
	})
	if err != nil {
		return err
	}
	for index, change := range matched {
		auditID := idsWritten[index]
		if _, err := database.Remember(owner, sourceAuditMemo+auditID.String(), func() (sourceChange, error) { return change, nil }); err != nil {
			return err
		}
		database.Mark(owner, sourceAuditMemo+auditID.String())
		state.audits[auditID] = change
		state.owners[auditID] = ctx
	}
	if !database.Mark(owner, "source_commit_registered") {
		return nil
	}
	if !database.BeforeCommit(owner, func(_ context.Context, finishing pgx.Tx) error {
		if err := flushSourceChanges(state.ctx, finishing); err != nil {
			return err
		}
		for auditID, change := range state.audits {
			if database.Marked(finishing, sourceEventMark+auditID.String()) {
				continue
			}
			var exists bool
			if err := finishing.QueryRow(state.ctx, `SELECT EXISTS(SELECT 1 FROM audit_log WHERE id=$1 AND after->>'source'=$2)`, auditID, change.After).Scan(&exists); err != nil {
				return err
			}
			if !exists {
				continue
			}
			if err := emitSourceChange(state.owners[auditID], finishing, auditID, change); err != nil {
				return err
			}
		}
		return nil
	}) {
		return fmt.Errorf("source changes need a database-owned transaction")
	}
	return nil
}

func auditSourceChange(ctx context.Context, tx pgx.Tx, change sourceChange) error {
	if !sourceEntities[change.EntityType] {
		return fmt.Errorf("source change has unknown entity %q", change.EntityType)
	}
	before, after := map[string]any{"source": change.Before}, map[string]any{"source": change.After}
	if change.SystemChanged {
		before["source_system"] = change.BeforeSystem
		after["source_system"] = change.AfterSystem
	}
	auditID, err := AuditWithEvidence(ctx, tx, "update", change.EntityType, change.ID, before, after, map[string]any{sourceNormalizedKey: true})
	if err != nil {
		return err
	}
	return emitSourceChange(ctx, tx, auditID, change)
}

func flushSourceChanges(ctx context.Context, tx pgx.Tx) error {
	pending, err := readSourceChanges(ctx, tx)
	if err != nil || len(pending) == 0 {
		return err
	}
	if err := saveSourceChanges(ctx, tx, map[string]sourceChange{}); err != nil {
		return err
	}
	ordered := make([]sourceChange, 0, len(pending))
	for _, change := range pending {
		ordered = append(ordered, change)
	}
	slices.SortFunc(ordered, func(a, b sourceChange) int { return strings.Compare(a.ID.String(), b.ID.String()) })
	for _, change := range ordered {
		owner, err := database.Remember(database.CommitOwner(tx), sourceOwnerMemo+change.ChangeID.String(), func() (context.Context, error) { return ctx, nil })
		if err != nil {
			return err
		}
		if err := auditSourceChange(owner, tx, change); err != nil {
			return err
		}
	}
	return nil
}

// foreignSource leaves provenance fields to their existing writer. The record's
// channel gets its own source-only audit instead of replacing that evidence.
func foreignSource(raw []byte, channel string) bool {
	var image map[string]json.RawMessage
	if json.Unmarshal(raw, &image) != nil {
		return false
	}
	value, exists := image["source"]
	if !exists {
		return false
	}
	var source string
	return json.Unmarshal(value, &source) == nil && source != channel
}
