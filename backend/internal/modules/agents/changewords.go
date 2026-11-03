// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// What a write changed, in the words its reader uses.
//
// A view in a chat host has no catalog and no network. On its own it could
// show only a field name and a record id. The values come from the approval
// card's engine, so one change reads alike in the inbox and the chat.

import (
	"context"
	"encoding/json"
	"slices"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// ChangeWords renders the fields a write sets as a reader reads them. current is
// the whole record before the write, with every field in changes present, and
// changes holds each field's value after it.
type ChangeWords func(ctx context.Context, current, changes map[string]json.RawMessage) ([]crmcontracts.ApprovalPreviewChange, error)

// WithChangeWords injects the renderer the write tools describe their changes
// with. A registry composed without one answers no changes.
func WithChangeWords(words ChangeWords) RegistryOption {
	return func(r *Registry) { r.changeWords = words }
}

// FieldChange is one field of a write: its label in the installation's
// language, and its value before and after. A record's name stands in place of
// its id.
type FieldChange struct {
	Field  string                            `json:"field"`
	Label  string                            `json:"label"`
	Before crmcontracts.ApprovalPreviewValue `json:"before"`
	After  crmcontracts.ApprovalPreviewValue `json:"after"`
}

// warningChangesUnnamed says the write landed but its changes could not be put
// in words, so a reader of the result is shown none.
const warningChangesUnnamed = "changes_unnamed"

const changesUnnamedMessage = "The write succeeded, but its changed fields could not be described for the user, " +
	"so the result carries no changes list. Describe the change from the record's fields instead."

// describeCreate names every field a create set, each from nothing to what
// the created record holds.
func (words ChangeWords) describeCreate(ctx context.Context, lang textlang.Lang, sent, created json.RawMessage) []FieldChange {
	return words.describe(ctx, lang, sent, created, created, true)
}

// describeUpdate names every field an update set, from what the record held
// before the write to what it holds after.
func (words ChangeWords) describeUpdate(ctx context.Context, lang textlang.Lang, sent, before, after json.RawMessage) []FieldChange {
	return words.describe(ctx, lang, sent, before, after, false)
}

// describe renders the fields sent names. The write has already landed, so a
// failure to name it does not fail the call. The answer then carries no
// changes, and a warning says why.
func (words ChangeWords) describe(ctx context.Context, lang textlang.Lang, sent, before, after json.RawMessage, created bool) []FieldChange {
	if words == nil {
		return nil
	}
	current, changes, err := changesOf(sent, before, after, created)
	if err == nil && len(changes) == 0 {
		return nil
	}
	var rendered []crmcontracts.ApprovalPreviewChange
	if err == nil {
		rendered, err = words(ctx, current, changes)
	}
	if err != nil {
		noteWarning(ctx, warningChangesUnnamed, changesUnnamedMessage)
		return nil
	}
	out := make([]FieldChange, 0, len(rendered))
	for _, change := range rendered {
		out = append(out, FieldChange{
			Field: change.Field, Label: FieldLabel(lang, change.Field),
			Before: change.Before, After: change.After,
		})
	}
	return out
}

// changesOf answers the record before the write, and the value after it of each
// field sent names. A created record had none of them before. A field withheld
// from this reader is left out, because its stored null would read as an empty
// value it does not hold.
func changesOf(sent, before, after json.RawMessage, created bool) (current, changes map[string]json.RawMessage, err error) {
	var named, record map[string]json.RawMessage
	if err := json.Unmarshal(sent, &named); err != nil {
		return nil, nil, err
	}
	if err := json.Unmarshal(before, &current); err != nil {
		return nil, nil, err
	}
	if err := json.Unmarshal(after, &record); err != nil {
		return nil, nil, err
	}
	var masked []string
	if raw, ok := record["masked_fields"]; ok {
		if err := json.Unmarshal(raw, &masked); err != nil {
			return nil, nil, err
		}
	}
	if current == nil {
		current = map[string]json.RawMessage{}
	}
	changes = map[string]json.RawMessage{}
	for name := range named {
		if slices.Contains(masked, name) {
			continue
		}
		changes[name] = orNull(record[name])
		if created {
			current[name] = nil
		}
		current[name] = orNull(current[name])
	}
	return current, changes, nil
}

func orNull(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return json.RawMessage("null")
	}
	return raw
}
