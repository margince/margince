// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package migration

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/storedobjects"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// CreateDraftRunInput opens a draft for an upload that has just been read.
type CreateDraftRunInput struct {
	SourceRef string
	Source    string
	FileName  string
	Profile   json.RawMessage
	Mapping   RunMapping
}

// CreateDraftRun opens a csv run in `pending` for one upload. An upload holds
// one unreleased run, so a second draft of the same file is a conflict.
func (s *RunStore) CreateDraftRun(ctx context.Context, in CreateDraftRunInput) (Run, error) {
	if err := auth.Require(ctx, ImportRunObject, principal.ActionCreate); err != nil {
		return Run{}, err
	}
	capturedBy, err := storekit.CapturedBy(ctx)
	if err != nil {
		return Run{}, err
	}
	mapping := in.Mapping
	if in.FileName != "" {
		mapping.FileName = in.FileName
	}
	encoded, err := json.Marshal(mapping)
	if err != nil {
		return Run{}, fmt.Errorf("encoding the import mapping: %w", err)
	}
	var run Run
	err = s.tx(ctx, func(tx pgx.Tx) error {
		var id RunID
		err := tx.QueryRow(ctx, `
			INSERT INTO import_run (connector, status, mapping, source_ref, source, captured_by, profile)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			RETURNING id`,
			ConnectorCSV, StatusPending, encoded, in.SourceRef, in.Source, capturedBy, in.Profile).Scan(&id)
		if storekit.IsUniqueViolation(err) {
			return fmt.Errorf("the upload already has an import run: %w", apperrors.ErrConflict)
		}
		if err != nil {
			return fmt.Errorf("creating the import draft: %w", err)
		}
		if _, err := storekit.Audit(ctx, tx, "create", ImportRunObject, id, nil, map[string]any{
			"connector": ConnectorCSV, auditFieldStatus: StatusPending, "source_ref": in.SourceRef,
		}); err != nil {
			return err
		}
		if err := storedobjects.Clear(ctx, tx, in.SourceRef); err != nil {
			return err
		}
		run, err = readStaged(ctx, tx, id, true)
		return err
	})
	if err != nil {
		return Run{}, err
	}
	return run, nil
}

// DraftPatch is the choices one PATCH changes; a nil field is left as it is.
type DraftPatch struct {
	Parts        *[]PartMapping
	Object       *string
	Fields       *map[string]string
	SourceKey    *string
	SourceName   *string
	OnDuplicate  *string
	OnBrokenLink *string
	ContextTag   *string
	DraftStep    *string
}

// UpdateDraft writes the choices patch changes on a run at version. A patch
// that changes nothing writes nothing. A changed choice on a report returns
// it to draft, because the report no longer describes what would be imported.
func (s *RunStore) UpdateDraft(ctx context.Context, id RunID, version int, patch DraftPatch) (Run, error) {
	if err := auth.Require(ctx, ImportRunObject, principal.ActionUpdate); err != nil {
		return Run{}, err
	}
	if patch.DraftStep != nil && *patch.DraftStep != DraftStepFiles && *patch.DraftStep != DraftStepMap {
		return Run{}, fmt.Errorf("draft step %q is not files or map: %w", *patch.DraftStep, apperrors.ErrInvalidArgument)
	}
	var run Run
	err := s.tx(ctx, func(tx pgx.Tx) error {
		state, err := lockSession(ctx, tx, id)
		if err != nil {
			return err
		}
		change := runChange{version: version}
		if err := change.admits(id, moveRedraft, state); err != nil {
			return err
		}
		var mapping RunMapping
		if err := json.Unmarshal(state.mapping, &mapping); err != nil {
			return fmt.Errorf("reading the mapping of import run %s: %w", id, err)
		}
		change.before, change.after = map[string]any{}, map[string]any{}
		patched := patch.applyTo(mapping, change.before, change.after)
		choiceChanged := len(change.after) > 0
		if patch.DraftStep != nil && *patch.DraftStep != state.draftStep {
			if state.status == StatusAwaitingApproval && !choiceChanged {
				return fmt.Errorf("import run %s holds a report; only a changed choice returns it to draft: %w", id, apperrors.ErrConflict)
			}
			change.before["draft_step"], change.after["draft_step"] = state.draftStep, *patch.DraftStep
			change.values = append(change.values, runValue{"draft_step", *patch.DraftStep})
		}
		if len(change.after) > 0 {
			if err := redraft(ctx, tx, id, patched, change); err != nil {
				return err
			}
		}
		run, err = readStaged(ctx, tx, id, true)
		return err
	})
	if err != nil {
		return Run{}, err
	}
	return run, nil
}

// redraft writes a patched mapping through the transition table, bumping the
// version and dropping any report the old choices produced.
func redraft(ctx context.Context, tx pgx.Tx, id RunID, mapping RunMapping, change runChange) error {
	encoded, err := json.Marshal(mapping)
	if err != nil {
		return fmt.Errorf("encoding the import mapping: %w", err)
	}
	change.values = append(change.values, runValue{"mapping", encoded})
	change.literals = append(change.literals, "version = version + 1", "report = NULL")
	return stageTransition(ctx, tx, id, moveRedraft, change)
}

// applyTo answers mapping with the patch applied, recording each field that
// changed in before and after under its contract name.
func (p DraftPatch) applyTo(mapping RunMapping, before, after map[string]any) RunMapping {
	patchField("parts", p.Parts, &mapping.Parts, before, after)
	patchField("object", p.Object, &mapping.Object, before, after)
	patchField("mapping", p.Fields, &mapping.Fields, before, after)
	patchField("source_key", p.SourceKey, &mapping.SourceKey, before, after)
	patchField("source_name", p.SourceName, &mapping.SourceName, before, after)
	patchField("on_duplicate", p.OnDuplicate, &mapping.OnDuplicate, before, after)
	patchField("on_broken_link", p.OnBrokenLink, &mapping.OnBrokenLink, before, after)
	patchField("context_tag", p.ContextTag, &mapping.ContextTag, before, after)
	return mapping
}

func patchField[T any](name string, patch *T, field *T, before, after map[string]any) {
	if patch == nil || equalChoice(*patch, *field) {
		return
	}
	before[name], after[name] = *field, *patch
	*field = *patch
}

// equalChoice compares two values of one choice. An empty and an absent list
// or map are the same choice, as they are once encoded with omitempty.
func equalChoice[T any](a, b T) bool {
	va, vb := reflect.ValueOf(a), reflect.ValueOf(b)
	if kind := va.Kind(); (kind == reflect.Map || kind == reflect.Slice) && va.Len() == 0 && vb.Len() == 0 {
		return true
	}
	return reflect.DeepEqual(a, b)
}
