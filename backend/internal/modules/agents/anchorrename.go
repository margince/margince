// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

const fieldDisplayName = "display_name"

// refuseAnchorRename refuses a machine's patch renaming the installation's own
// company before a human is asked, since its release would be refused anyway.
func refuseAnchorRename(ctx context.Context, rec datasource.Record, fields json.RawMessage) error {
	if rec.Ref.Type != datasource.EntityCompany {
		return nil
	}
	if actor, ok := principal.Actor(ctx); ok {
		if _, human := principal.HumanUserID(actor.ID); human {
			return nil
		}
	}
	var patch map[string]json.RawMessage
	if err := json.Unmarshal(fields, &patch); err != nil {
		return &BadArgsError{Field: fieldsArg, Cause: fmt.Errorf("fields must be a JSON object: %w", err)}
	}
	raw, renames := patch[fieldDisplayName]
	if !renames {
		return nil
	}
	var name string
	if err := json.Unmarshal(raw, &name); err != nil {
		return &BadArgsError{Field: fieldDisplayName, Cause: errors.New("display_name must be a string")}
	}
	var target struct {
		IsAnchor    bool   `json:"is_anchor"`
		DisplayName string `json:"display_name"`
	}
	if err := json.Unmarshal(rec.Fields, &target); err != nil {
		return fmt.Errorf("agents: reading company %s for an anchor rename: %w", rec.Ref.ID, err)
	}
	if !target.IsAnchor || strings.TrimSpace(name) == target.DisplayName {
		return nil
	}
	return &BadArgsError{
		Field: fieldDisplayName,
		Code:  "anchor_protected",
		Cause: errors.New("this is the workspace's own company; " + values.AnchorRenameRefusal),
	}
}
