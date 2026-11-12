// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// The activities slice of the SoR-mode SystemOfRecordProvider
// (interfaces.md §3): read + log. Activities are deliberately absent
// from the search sweep — the timeline is reached through
// read_record/list on a named entity, not blind full-text sweep.

import (
	"context"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// Provider answers the datasource verbs for activity.
type Provider struct {
	store *Store
}

// NewProvider builds this module's system-of-record provider over a
// workspace-bound handle.
func NewProvider(db *database.DB) *Provider {
	return &Provider{store: NewStore(db)}
}

// WithTranscriptEnqueue lets a transcript created through this seam start its
// own reading, the same way one created over REST does. Without it the tool
// surface can store a transcript and nothing reads it — which is how the
// extraction lane came to hold zero rows while being fully built.
func (p *Provider) WithTranscriptEnqueue(enqueue TranscriptReadEnqueue) *Provider {
	p.store = p.store.WithTranscriptEnqueue(enqueue)
	return p
}

func ref(t datasource.EntityType, id openapi_types.UUID) datasource.EntityRef {
	return datasource.EntityRef{Type: t, ID: ids.UUID(id)}
}

func (p *Provider) Read(ctx context.Context, r datasource.EntityRef) (datasource.Record, error) {
	if r.Type != datasource.EntityActivity {
		return datasource.Record{}, &datasource.UnsupportedEntityError{Type: string(r.Type)}
	}
	v, err := p.store.GetActivity(ctx, ids.From[ids.ActivityKind](r.ID), storekit.LiveOnly)
	if err != nil {
		return datasource.Record{}, err
	}
	return datasource.NewRecord(r, v, v.Version)
}

// SearchEntity pages the timeline under the shared search contract, through
// the store read the HTTP door runs. A row is listed by the same DISCOVER gate,
// and its content is withheld per row as there.
func (p *Provider) SearchEntity(ctx context.Context, t datasource.EntityType, text *string, limit int, cursor *string,
	filters map[string]string,
) ([]datasource.Record, string, bool, error) {
	if t != datasource.EntityActivity {
		return nil, "", false, &datasource.UnsupportedEntityError{Type: string(t)}
	}
	in := ListActivitiesInput{Query: text, Limit: &limit, Cursor: cursor}
	if err := activityListFilters.ApplyCustom(&in, filters, func(in *ListActivitiesInput, custom map[string]string) {
		in.CustomFilters = custom
	}); err != nil {
		return nil, "", false, err
	}
	if err := httperr.RequirePair(filterEntityType, in.EntityType, filterEntityID, in.EntityID); err != nil {
		return nil, "", false, err
	}
	// A record's timeline reads as the 360 pages show it: one row per email.
	in.CollapseSeatCopies = in.EntityID != nil
	rows, page, err := p.store.ListActivities(ctx, in)
	if err != nil {
		return nil, "", false, err
	}
	records := make([]datasource.Record, 0, len(rows))
	for _, v := range rows {
		rec, err := datasource.NewRecord(datasource.EntityRef{Type: t, ID: ids.UUID(v.Id)}, v, v.Version)
		if err != nil {
			return nil, "", false, err
		}
		records = append(records, rec)
	}
	return records, page.NextCursor, page.HasMore, nil
}

// Update patches an activity's own fields. UpdateActivityRequest carries no
// link field, so this cannot change who an activity is about — only what it
// says and when it happened; associations are the relink verb's, with its own
// audit action.
func (p *Provider) Update(ctx context.Context, in datasource.UpdateInput) (datasource.EntityRef, error) {
	if in.Ref.Type != datasource.EntityActivity {
		return datasource.EntityRef{}, &datasource.UnsupportedEntityError{Type: string(in.Ref.Type)}
	}
	raw, err := datasource.RawFields(in.Patch)
	if err != nil {
		return datasource.EntityRef{}, err
	}
	var req crmcontracts.UpdateActivityRequest
	if err := datasource.StrictCustomFieldDecode(raw, &req); err != nil {
		return datasource.EntityRef{}, err
	}
	update := activityUpdateInput(req, in.IfVersion)
	update.Clear = in.Clear
	update.Trail = in.Trail
	v, err := p.store.UpdateActivity(ctx, ids.From[ids.ActivityKind](in.Ref.ID), update)
	return ref(datasource.EntityActivity, v.Id), err
}

// Archive retires an activity, the same soft archive DELETE /v1/activities/{id}
// performs — the contract has always declared archiveActivity as archive_record's
// (agentPolicies), and the seam is what was missing: the tool staged a
// confirmation, a human approved it, and the call then died at this switch with
// `unsupported_entity_type`. An approval spent on a verb that could never run.
func (p *Provider) Archive(ctx context.Context, r datasource.EntityRef) (datasource.EntityRef, error) {
	return p.ArchiveAt(ctx, datasource.ArchiveInput{Ref: r})
}

// ArchivableTypes is datasource.RecordArchiverV2's: this module archives the
// one entity it owns.
func (p *Provider) ArchivableTypes(context.Context) ([]datasource.EntityType, error) {
	return []datasource.EntityType{datasource.EntityActivity}, nil
}

// RefuseArchive is datasource.RecordArchiverV2's stage-time half: the store's
// own authority probes, run without the write.
func (p *Provider) RefuseArchive(ctx context.Context, r datasource.EntityRef) error {
	if r.Type != datasource.EntityActivity {
		return &datasource.UnsupportedEntityError{Type: string(r.Type)}
	}
	return p.store.RefuseArchiveActivity(ctx, ids.From[ids.ActivityKind](r.ID))
}

// ArchiveAt is Archive carrying the version the caller's authority named.
func (p *Provider) ArchiveAt(ctx context.Context, in datasource.ArchiveInput) (datasource.EntityRef, error) {
	if in.Ref.Type != datasource.EntityActivity {
		return datasource.EntityRef{}, &datasource.UnsupportedEntityError{Type: string(in.Ref.Type)}
	}
	v, err := p.store.ArchiveActivity(ctx, ids.From[ids.ActivityKind](in.Ref.ID), in.IfVersion)
	if err != nil {
		return datasource.EntityRef{}, err
	}
	return ref(datasource.EntityActivity, v.Id), nil
}

// The signed system actor may stamp task identities that no request may forge.
// The importer namespace stays refused even for that actor.
func logInputForPrincipal(ctx context.Context, req crmcontracts.CreateActivityRequest) (LogActivityInput, error) {
	actor, ok := principal.Actor(ctx)
	engine := ok && actor.Type == principal.PrincipalSystem
	if engine && req.SourceSystem != nil && provenance.EngineTaskSource(*req.SourceSystem) {
		return logActivityInputAllowingTaskIdentity(req)
	}
	return LogActivityInputFrom(req)
}

func (p *Provider) Create(ctx context.Context, in datasource.CreateInput) (datasource.EntityRef, error) {
	if in.EntityType != datasource.EntityActivity {
		return datasource.EntityRef{}, &datasource.UnsupportedEntityError{Type: string(in.EntityType)}
	}
	raw, err := datasource.RawFields(in.Fields)
	if err != nil {
		return datasource.EntityRef{}, err
	}
	var req crmcontracts.CreateActivityRequest
	if err := datasource.StrictCustomFieldDecode(raw, &req); err != nil {
		return datasource.EntityRef{}, err
	}
	req.Source = in.Source
	mapped, err := logInputForPrincipal(ctx, req)
	if err != nil {
		return datasource.EntityRef{}, err
	}
	mapped.Origin = originOfCaller(ctx, mapped.Origin)
	if message := systemtext.MessageIn(ctx); message != nil {
		mapped.SystemText = message
	}
	v, _, err := p.store.LogActivity(ctx, mapped)
	return ref(datasource.EntityActivity, v.Id), err
}

// originOfCaller stamps OriginAgent on a row a scheduled agent run creates.
// That keeps its notes off the last-activity clocks
// (relstrength.LastActivityCountsSQL). The run marker decides, not the
// passport. A user's own assistant in a chat has a passport and no run.
// Its note counts as one typed in the app.
func originOfCaller(ctx context.Context, origin string) string {
	if _, scheduled := principal.AgentRunID(ctx); scheduled {
		return OriginAgent
	}
	return origin
}

// WithIdentityProver gives tool writes the same mailbox ownership proof as HTTP writes.
func (p *Provider) WithIdentityProver(prover AddressProver) *Provider {
	p.store = p.store.WithIdentityProver(prover)
	return p
}
