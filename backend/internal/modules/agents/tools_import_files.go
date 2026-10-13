// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// previewArgs is preview_import's input in either shape: one file by object,
// csv and mapping, or several by files.
type previewArgs struct {
	Object       string            `json:"object"`
	CSV          string            `json:"csv"`
	Mapping      map[string]string `json:"mapping"`
	Files        []previewFileArg  `json:"files"`
	SourceName   string            `json:"source_name"`
	OnBrokenLink string            `json:"on_broken_link"`
	OnDuplicate  string            `json:"on_duplicate"`
}

type previewFileArg struct {
	Name    string            `json:"name"`
	Object  string            `json:"object"`
	CSV     string            `json:"csv"`
	Mapping map[string]string `json:"mapping"`
}

// applyRunOptions sets the run-wide choices a caller named, and leaves the
// contract's defaults for the rest.
func (a previewArgs) applyRunOptions(req *crmcontracts.CreateImportRunRequest) {
	if a.OnDuplicate != "" {
		policy := crmcontracts.ImportOnDuplicate(a.OnDuplicate)
		req.OnDuplicate = &policy
	}
	if a.OnBrokenLink != "" {
		policy := crmcontracts.ImportOnBrokenLink(a.OnBrokenLink)
		req.OnBrokenLink = &policy
	}
	if a.SourceName != "" {
		req.SourceName = &a.SourceName
	}
}

// previewFiles stages several files as one run. The seam stores them as one
// ZIP, so even a single file stages as a part. A run reads parts from a ZIP and
// a plain mapping from a CSV.
func (t previewImport) previewFiles(ctx context.Context, args previewArgs) (json.RawMessage, error) {
	files, err := importFilesFrom(args.Files)
	if err != nil {
		return nil, err
	}
	profile, err := t.imports.ProfileFiles(ctx, files)
	if err != nil {
		return nil, err
	}
	parts, previews, err := mappedParts(profile, args.Files)
	if err != nil {
		return nil, discarding(ctx, t.imports, profile.SourceRef, err)
	}
	req := crmcontracts.CreateImportRunRequest{
		Connector: crmcontracts.CreateImportRunRequestConnector(importConnectorCSV),
		SourceRef: profile.SourceRef,
		Parts:     &parts,
	}
	args.applyRunOptions(&req)
	run, err := t.imports.StageRun(ctx, req)
	if err != nil {
		// Kept for the reason the single-file path keeps it: the draft is testable again.
		return nil, namingTheBusyRun(err)
	}
	return t.answered(ctx, run, ImportPreviewResult{Files: previews})
}

// importFilesFrom checks every file before anything is stored: a known type,
// some content, one file per type, and the byte limit across all of them.
func importFilesFrom(args []previewFileArg) ([]ImportFile, error) {
	files := make([]ImportFile, 0, len(args))
	byObject := map[string]string{}
	total := 0
	for i, f := range args {
		if strings.TrimSpace(f.Name) == "" {
			return nil, &BadArgsError{Field: fmt.Sprintf("files[%d].name", i), Cause: errors.New(
				"a file has no name; name each file so the answer can say which is which")}
		}
		if err := refuseUnimportableObject(f.Object); err != nil {
			return nil, err
		}
		if strings.TrimSpace(f.CSV) == "" {
			return nil, &BadArgsError{Cause: fmt.Errorf(
				"%q is empty; send its contents with its header row first", f.Name)}
		}
		if first, taken := byObject[f.Object]; taken {
			return nil, &BadArgsError{Cause: fmt.Errorf(
				"%q and %q are both %s files, and an import takes one file per type; join them", first, f.Name, f.Object)}
		}
		byObject[f.Object] = f.Name
		total += len(f.CSV)
		files = append(files, ImportFile{Object: f.Object, CSV: f.CSV})
	}
	if total > maxImportCSVBytes {
		return nil, &BadArgsError{Cause: fmt.Errorf(
			"these files are %d bytes together and one pasted import takes at most %d; "+
				"upload them in the web app instead, or split them", total, maxImportCSVBytes)}
	}
	return files, nil
}

// mappedParts settles each file's mapping as the single-file path does. The
// caller's columns win and the proposal fills the rest. With no caller mapping,
// a proposal that leaves columns unplaced is refused.
func mappedParts(
	profile crmcontracts.ImportSourceProfile, args []previewFileArg,
) ([]crmcontracts.ImportPartRequest, []ImportPreviewFile, error) {
	var profiled []crmcontracts.ImportPartProfile
	if profile.Parts != nil {
		profiled = *profile.Parts
	}
	parts := make([]crmcontracts.ImportPartRequest, 0, len(args))
	previews := make([]ImportPreviewFile, 0, len(args))
	for _, f := range args {
		at := slices.IndexFunc(profiled, func(p crmcontracts.ImportPartProfile) bool {
			return string(p.Object) == f.Object
		})
		if at < 0 {
			return nil, nil, fmt.Errorf("the stored files hold no %s file for %q", f.Object, f.Name)
		}
		part := profiled[at]
		view := crmcontracts.ImportSourceProfile{Object: &part.Object, Columns: part.Columns}
		mapping := maps.Clone(part.SuggestedMappings[f.Object])
		if mapping == nil {
			mapping = map[string]string{}
		}
		maps.Copy(mapping, f.Mapping)
		if len(f.Mapping) == 0 {
			if err := proposalCoversTheFile(view, mapping); err != nil {
				return nil, nil, namingTheFile(f.Name, err)
			}
		}
		parts = append(parts, crmcontracts.ImportPartRequest{
			File: part.File, Object: crmcontracts.ImportObject(f.Object), Mapping: mapping,
		})
		previews = append(previews, ImportPreviewFile{
			Name: f.Name, Part: part.File, Object: f.Object, Mapping: mapping,
			Columns: columnNames(view), Unmapped: unmappedColumns(view, mapping),
		})
	}
	return parts, previews, nil
}

// namingTheFile puts the file's name in front of a refusal about its columns,
// so a caller sending three files knows which one to fix.
func namingTheFile(name string, err error) error {
	if bad, ok := errors.AsType[*BadArgsError](err); ok {
		named := *bad
		named.Cause = fmt.Errorf("%q: %w", name, bad.Cause)
		return &named
	}
	return err
}
