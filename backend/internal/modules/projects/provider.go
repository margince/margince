// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package projects

// The projects slice of the SoR-mode SystemOfRecordProvider (interfaces.md
// §3). The composition root assembles the module providers into the one
// datasource seam the MCP surface binds, so an agent reads and writes a project
// through the same store the HTTP surface uses.

import (
	"context"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
	"github.com/margince/margince/backend/internal/shared/ports/fieldcatalog"
)

// Provider answers the datasource verbs for project.
type Provider struct {
	store *Store
}

// NewProvider wires the datasource verbs over the same store the transport
// uses, so a tool call and an HTTP call take the identical gates.
func NewProvider(db *database.DB) *Provider {
	return &Provider{store: NewStore(db)}
}

// ProviderOver binds the datasource verbs to a store the caller has already
// built. Identical in purpose to HandlersOver: a tool call and an HTTP call must
// reach ONE store, or the seams wired on one of them are missing from the other.
func ProviderOver(store *Store) *Provider {
	return &Provider{store: store}
}

// WithFieldCatalog wires the workspace custom-field catalog into the provider's
// store (see Store.WithFieldCatalog), so the MCP surface's record verbs carry
// cf values exactly like REST.
func (p *Provider) WithFieldCatalog(catalog fieldcatalog.Reader) *Provider {
	p.store = p.store.WithFieldCatalog(catalog)
	return p
}

// ref names one project on the datasource seam. This module answers for exactly
// one entity type, so the type is not a parameter: a caller able to pass another
// could mint a reference this provider cannot serve.
func ref(id openapi_types.UUID) datasource.EntityRef {
	return datasource.EntityRef{Type: datasource.EntityProject, ID: ids.UUID(id)}
}

// Read answers one project as a versioned record.
func (p *Provider) Read(ctx context.Context, r datasource.EntityRef) (datasource.Record, error) {
	if r.Type != datasource.EntityProject {
		return datasource.Record{}, &datasource.UnsupportedEntityError{Type: string(r.Type)}
	}
	v, err := p.store.GetProject(ctx, ids.From[ids.ProjectKind](r.ID), storekit.LiveOnly)
	if err != nil {
		return datasource.Record{}, err
	}
	return datasource.NewRecord(r, v, v.Version)
}

// SearchEntity lists projects under the shared search contract.
//
// A filter this type has no binding for is an ERROR rather than a dropped
// clause — see listfilters.go: a caller who narrowed a search and got every row
// back has been told something false about what they are looking at.
func (p *Provider) SearchEntity(ctx context.Context, t datasource.EntityType, text *string, limit int, cursor *string,
	filters map[string]string,
) ([]datasource.Record, string, bool, error) {
	if t != datasource.EntityProject {
		return nil, "", false, &datasource.UnsupportedEntityError{Type: string(t)}
	}
	in := ListProjectsInput{Query: text, Limit: &limit, Cursor: cursor}
	if err := projectListFilters.Apply(&in, filters); err != nil {
		return nil, "", false, err
	}
	rows, page, err := p.store.ListProjects(ctx, in)
	if err != nil {
		return nil, "", false, err
	}
	records := make([]datasource.Record, 0, len(rows))
	for _, v := range rows {
		rec, err := datasource.NewRecord(ref(v.Id), v, v.Version)
		if err != nil {
			return nil, "", false, err
		}
		records = append(records, rec)
	}
	return records, page.NextCursor, page.HasMore, nil
}

// ListFilters names what SearchEntity can narrow a project by.
func (p *Provider) ListFilters(t datasource.EntityType) []string {
	if t != datasource.EntityProject {
		return nil
	}
	return projectListFilters.Names()
}

// Create mints a project from the tool surface's raw body.
func (p *Provider) Create(ctx context.Context, in datasource.CreateInput) (datasource.EntityRef, error) {
	if in.EntityType != datasource.EntityProject {
		return datasource.EntityRef{}, &datasource.UnsupportedEntityError{Type: string(in.EntityType)}
	}
	raw, err := datasource.RawFields(in.Fields)
	if err != nil {
		return datasource.EntityRef{}, err
	}
	var req crmcontracts.CreateProjectRequest
	if err := datasource.StrictDecode(raw, &req); err != nil {
		return datasource.EntityRef{}, err
	}
	req.Source = in.Source
	mapped, err := projectCreateInput(req)
	if err != nil {
		return datasource.EntityRef{}, err
	}
	v, err := p.store.CreateProject(ctx, mapped)
	return ref(v.Id), err
}

// Update patches a project from the tool surface's raw body.
func (p *Provider) Update(ctx context.Context, in datasource.UpdateInput) (datasource.EntityRef, error) {
	if in.Ref.Type != datasource.EntityProject {
		return datasource.EntityRef{}, &datasource.UnsupportedEntityError{Type: string(in.Ref.Type)}
	}
	raw, err := datasource.RawFields(in.Patch)
	if err != nil {
		return datasource.EntityRef{}, err
	}
	var req crmcontracts.UpdateProjectRequest
	if err := datasource.StrictDecode(raw, &req); err != nil {
		return datasource.EntityRef{}, err
	}
	update, err := projectUpdateInput(req, in.IfVersion)
	if err != nil {
		return datasource.EntityRef{}, err
	}
	update.Trail = in.Trail
	update.Clear = in.Clear
	v, err := p.store.UpdateProject(ctx, ids.From[ids.ProjectKind](in.Ref.ID), update)
	return ref(v.Id), err
}

// Archive retires one project, without a version claim.
func (p *Provider) Archive(ctx context.Context, r datasource.EntityRef) (datasource.EntityRef, error) {
	return p.ArchiveAt(ctx, datasource.ArchiveInput{Ref: r})
}

// ArchivableTypes is datasource.RecordArchiverV2's: the one this module serves.
func (p *Provider) ArchivableTypes(context.Context) ([]datasource.EntityType, error) {
	return []datasource.EntityType{datasource.EntityProject}, nil
}

// RefuseArchive is datasource.RecordArchiverV2's stage-time half: the store's
// own authority probe, run without the write.
func (p *Provider) RefuseArchive(ctx context.Context, r datasource.EntityRef) error {
	if r.Type != datasource.EntityProject {
		return &datasource.UnsupportedEntityError{Type: string(r.Type)}
	}
	return p.store.RefuseArchiveProject(ctx, ids.From[ids.ProjectKind](r.ID))
}

// ArchiveAt is Archive carrying the version the caller's authority named.
func (p *Provider) ArchiveAt(ctx context.Context, in datasource.ArchiveInput) (datasource.EntityRef, error) {
	if in.Ref.Type != datasource.EntityProject {
		return datasource.EntityRef{}, &datasource.UnsupportedEntityError{Type: string(in.Ref.Type)}
	}
	v, err := p.store.ArchiveProject(ctx, ids.From[ids.ProjectKind](in.Ref.ID), in.IfVersion)
	return ref(v.Id), err
}
