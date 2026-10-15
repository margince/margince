// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// refuseTooManyRows refuses an upload one test run cannot read, before any
// run or stored file exists for it.
func refuseTooManyRows(rows int) error {
	if rows <= importRowCap {
		return nil
	}
	return httperr.Validation("file", "too_many_rows", fmt.Sprintf(
		"this upload has %d rows; a test run takes at most %d, so split it into smaller files and import each one",
		rows, importRowCap))
}

// openDraft opens the upload's run in `pending`, keeping the profile so a
// reload shows the same evidence, and answers the profile naming the run.
func (h importHandlers) openDraft(
	ctx context.Context, profile crmcontracts.ImportSourceProfile, mapping migration.RunMapping,
) (crmcontracts.ImportSourceProfile, error) {
	mapping.OnDuplicate = string(crmcontracts.ImportOnDuplicateCreate)
	mapping.OnBrokenLink = string(crmcontracts.ImportOnBrokenLinkSkipRow)
	stored, err := json.Marshal(profile)
	if err != nil {
		return crmcontracts.ImportSourceProfile{}, fmt.Errorf("encoding the upload's profile: %w", err)
	}
	run, err := migration.NewRunStore(h.db).CreateDraftRun(ctx, migration.CreateDraftRunInput{
		SourceRef: profile.SourceRef, Source: importSourceProvenance, Profile: stored, Mapping: mapping,
	})
	if err != nil {
		return crmcontracts.ImportSourceProfile{}, err
	}
	profile.RunId = openapi_types.UUID(run.ID)
	return profile, nil
}

// draftOfSource answers the run createImportRun tests: the one the upload
// opened, or a new draft for a source uploaded before uploads opened one.
func (h importHandlers) draftOfSource(ctx context.Context, req crmcontracts.CreateImportRunRequest) (migration.Run, error) {
	runs := migration.NewRunStore(h.db)
	run, err := runs.RunOfSource(ctx, req.SourceRef)
	if errors.Is(err, apperrors.ErrNotFound) {
		fileName := ""
		if req.FileName != nil {
			fileName = *req.FileName
		}
		run, err = runs.CreateDraftRun(ctx, migration.CreateDraftRunInput{
			SourceRef: req.SourceRef, Source: importSourceProvenance, FileName: fileName,
		})
		if errors.Is(err, apperrors.ErrConflict) {
			// A concurrent request opened the draft first; test that one.
			run, err = runs.RunOfSource(ctx, req.SourceRef)
		}
	}
	if err != nil {
		return migration.Run{}, err
	}
	if run.SourceReleased {
		return migration.Run{}, fmt.Errorf(
			"import run %s has released this upload; upload the file again to import it: %w", run.ID, apperrors.ErrConflict)
	}
	return run, nil
}

// UpdateImportRun saves the choices a draft's importer made. They are checked
// for shape only: the test run checks them against the object's fields.
func (h importHandlers) UpdateImportRun(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	ctx := r.Context()
	if err := auth.Require(ctx, importRunGrant, principal.ActionUpdate); err != nil {
		httperr.Write(w, r, err)
		return
	}
	var req crmcontracts.UpdateImportRunRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	_, tagSent := httperr.PresentField(r, "context_tag_id")
	patch, err := draftPatchFrom(req, tagSent)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	if patch.PresetLinks != nil {
		if err := h.presetsAreVisible(ctx, *patch.PresetLinks); err != nil {
			httperr.Write(w, r, err)
			return
		}
	}
	run, err := h.stagedFor(ctx, id)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	if err := refuseNonActivityPreset(mergedWithPatch(run.Mapping, patch)); err != nil {
		httperr.Write(w, r, err)
		return
	}
	saved, err := migration.NewRunStore(h.db).UpdateDraft(ctx, run.ID, req.Version, patch)
	if err != nil {
		httperr.Write(w, r, h.sessionRefusal(ctx, err))
		return
	}
	httperr.WriteJSON(w, http.StatusOK, toContractImportRun(saved))
}

// mergedWithPatch is the mapping a draft would hold once patch is saved, for
// the checks that look at the object and the preset together.
func mergedWithPatch(stored *migration.RunMapping, patch migration.DraftPatch) migration.RunMapping {
	var merged migration.RunMapping
	if stored != nil {
		merged = *stored
	}
	if patch.Object != nil {
		merged.Object = *patch.Object
	}
	if patch.Parts != nil {
		merged.Parts = *patch.Parts
	}
	if patch.PresetLinks != nil {
		merged.PresetLinks = *patch.PresetLinks
	}
	return merged
}

// draftPatchFrom reads a save into the store's patch. tagSent with a nil tag
// is an explicit null, which files created records under no tag.
func draftPatchFrom(req crmcontracts.UpdateImportRunRequest, tagSent bool) (migration.DraftPatch, error) {
	var patch migration.DraftPatch
	if err := patchEnums(req, &patch); err != nil {
		return migration.DraftPatch{}, err
	}
	if req.SourceName != nil {
		name, err := sourceNameFrom(req.SourceName)
		if err != nil {
			return migration.DraftPatch{}, err
		}
		patch.SourceName = &name
	}
	patch.Fields, patch.SourceKey = req.Mapping, req.SourceKey
	if req.PresetLinks != nil {
		presets, err := presetLinksFrom(req.PresetLinks)
		if err != nil {
			return migration.DraftPatch{}, err
		}
		patch.PresetLinks = &presets
	}
	if req.Parts != nil {
		parts := make([]migration.PartMapping, 0, len(*req.Parts))
		for _, p := range *req.Parts {
			if !p.Object.Valid() {
				return migration.DraftPatch{}, unsupportedObject()
			}
			parts = append(parts, migration.PartMapping{
				File: p.File, Object: string(p.Object), Fields: p.Mapping, SourceKey: optionalText(p.SourceKey),
			})
		}
		patch.Parts = &parts
	}
	if tagSent {
		tag := ""
		if req.ContextTagId != nil {
			if ids.UUID(*req.ContextTagId).IsZero() {
				return migration.DraftPatch{}, httperr.Validation("context_tag_id", "empty",
					"A context tag names an existing tag by id. Send null to file this run under no word.")
			}
			tag = req.ContextTagId.String()
		}
		patch.ContextTag = &tag
	}
	return patch, nil
}

// patchEnums copies the save's closed choices, refusing a value outside them.
func patchEnums(req crmcontracts.UpdateImportRunRequest, patch *migration.DraftPatch) error {
	if req.Object != nil {
		if !req.Object.Valid() {
			return unsupportedObject()
		}
		patch.Object = new(string(*req.Object))
	}
	if req.OnDuplicate != nil {
		if !req.OnDuplicate.Valid() {
			return httperr.Validation("on_duplicate", "invalid_enum", "A duplicate policy is create or skip.")
		}
		patch.OnDuplicate = new(string(*req.OnDuplicate))
	}
	if req.OnBrokenLink != nil {
		if !req.OnBrokenLink.Valid() {
			return httperr.Validation("on_broken_link", "invalid_enum", "A broken-link policy is skip_row or drop_link.")
		}
		patch.OnBrokenLink = new(string(*req.OnBrokenLink))
	}
	if req.DraftStep != nil {
		if !req.DraftStep.Valid() {
			return httperr.Validation("draft_step", "invalid_enum", "A draft step is files or map.")
		}
		patch.DraftStep = new(string(*req.DraftStep))
	}
	return nil
}

func optionalText(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// applySession copies what a saved session adds to a run onto the contract
// run. The profile is shown only while a choice can still be made from it.
func applySession(out *crmcontracts.ImportRun, run migration.Run) {
	out.Version = run.Version
	out.SourceReleased = run.SourceReleased
	out.ApprovedAt = run.ApprovedAt
	if run.DraftStep != "" {
		out.DraftStep = new(crmcontracts.ImportDraftStep(run.DraftStep))
	}
	if run.SupersededBy != nil {
		out.SupersededBy = new(openapi_types.UUID(*run.SupersededBy))
	}
	choosing := run.Status == migration.StatusPending || run.Status == migration.StatusAwaitingApproval
	if len(run.Profile) == 0 || !choosing || run.SourceReleased {
		return
	}
	var profile crmcontracts.ImportSourceProfile
	if err := json.Unmarshal(run.Profile, &profile); err != nil {
		// The run is still served; only the evidence it was mapped on is missing.
		slog.Error("an import run's stored profile is unreadable", "run", run.ID, "err", err)
		return
	}
	profile.RunId = openapi_types.UUID(run.ID)
	out.Profile = &profile
}
