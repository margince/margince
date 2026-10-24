// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package compose is the composition layer the process roles share
// (ADR-0054, amended §2): it assembles the module providers into the one
// datasource.SystemOfRecordProvider seam the MCP surface binds, and (via
// server.go) the module transports into the contract HTTP surface.
// Modules never see each other; every cross-module edge is wired here.
package compose

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose/profileverdicts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/commissions"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/customfields"
	"github.com/margince/margince/backend/internal/modules/dealrooms"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/integrations"
	"github.com/margince/margince/backend/internal/modules/introductions"
	"github.com/margince/margince/backend/internal/modules/notices"
	"github.com/margince/margince/backend/internal/modules/projects"
	"github.com/margince/margince/backend/internal/modules/signals"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// Provider dispatches each datasource verb to the module that owns the
// entity type. It IS the system of record, so freshness is trivially
// authoritative (03e §2.3 — the overlay adapter is where that earns its
// keep).
type Provider struct {
	contacts    *contacts.Provider
	deals       *deals.Provider
	projects    *projects.Provider
	activities  *activities.Provider
	commissions *commissions.Provider
	dealRooms   *dealrooms.Provider
	reports     *reportEngine
}

func NewProvider(pool *pgxpool.Pool) *Provider {
	return NewProviderFor(InstallationDB(pool))
}

// NewProviderFor is NewProvider over a handle whose workspace is already
// decided: a suite that seeds a second workspace names the one it means.
func NewProviderFor(db *database.DB) *Provider {
	pool := db.Pool()
	return &Provider{
		// The fieldcatalog seam mirrors the HTTP wiring (server.go): the
		// MCP surface's record verbs carry cf_* values too.
		// The merge carriers mirror the HTTP wiring (serverassembly.go): the
		// MCP surface merges and promotes records too, and an unwired carrier
		// refuses every merge of a subject holding what it carries.
		contacts: contacts.NewProvider(db).
			WithFieldCatalog(customfields.NewService(pool, nil)).
			WithStopCarrier(consent.NewStore(db)).
			WithSatelliteCarriers(consent.NewStore(db), introductions.NewStore(db, time.Now)).
			WithParticipantRosterReader(activities.EnsureParticipantRosterVisibleTx).
			WithProviderRunReader(integrations.ReadRunForContactTx).
			WithMergeSignalReader(signals.PreviewSignalCarryTx).
			WithMergeCommitmentReader(weeklyPlanStore(pool).PreviewCommitmentCarryTx).
			WithMergeNoticeReader(notices.PreviewNoticeCarryTx).
			WithMergeContentReader(activities.NewStore(db)).
			WithProfileVerdictReader(profileverdicts.MergeReader(db)).
			WithFeedbackCarryReader(ai.NewFeedbackStore(db)).
			WithGrantCarrier(identity.NewServiceFor(db)).
			WithShareRequests(contactShareRequests(pool)).
			WithInstallationRenamer(installationRenamer(pool)),
		deals:       deals.NewProvider(db, DealsInstallation()).WithFieldCatalog(customfields.NewService(pool, nil)),
		projects:    projects.ProviderOver(ProjectsStoreOver(db)),
		activities:  activities.NewProvider(InstallationDB(pool)).WithFieldCatalog(customfields.NewService(pool, nil)),
		commissions: commissions.NewProvider(db),
		dealRooms:   dealrooms.NewProvider(db),
		reports:     newReportEngine(pool),
	}
}

var _ datasource.SystemOfRecordProvider = (*Provider)(nil)

// defaultSearchPageSize is the page a caller who named no limit gets: the
// shared Limit parameter's own declared default (crm.yaml
// components.parameters.Limit, `default: 50`).
const defaultSearchPageSize = 50

func (p *Provider) Read(ctx context.Context, ref datasource.EntityRef) (datasource.Record, error) {
	switch ref.Type {
	case datasource.EntityContact, datasource.EntityCompany, datasource.EntityLead,
		datasource.EntityRelationship, datasource.EntityPartner:
		return p.contacts.Read(ctx, ref)
	case datasource.EntityDeal, datasource.EntityProduct, datasource.EntityOfferTemplate, datasource.EntityOffer:
		return p.deals.Read(ctx, ref)
	case datasource.EntityCommission:
		return p.commissions.Read(ctx, ref)
	case datasource.EntityDealRoom, datasource.EntityDealRoomThread, datasource.EntityDealRoomDocument,
		datasource.EntityDealRoomParticipant, datasource.EntityDealRoomComment:
		return p.dealRooms.Read(ctx, ref)
	case datasource.EntityProject:
		return p.projects.Read(ctx, ref)
	case datasource.EntityActivity:
		return p.activities.Read(ctx, ref)
	default:
		return datasource.Record{}, &datasource.UnsupportedEntityError{Type: string(ref.Type)}
	}
}

// ListFilters answers which filters an enumeration of one record type can be
// narrowed by, by asking the module that owns the type.
//
// It is the STORE half of list_records' vocabulary — the contract's half is
// derived from crm.yaml — and it is here for the same reason every other
// cross-module edge is: only this layer sees both modules. An entity type no
// module lists answers nothing, and the tool then publishes no filters for it
// rather than a name it would refuse.
func (p *Provider) ListFilters(t datasource.EntityType) []string {
	switch t {
	case datasource.EntityContact, datasource.EntityCompany, datasource.EntityLead,
		datasource.EntityPartner:
		return p.contacts.ListFilters(t)
	case datasource.EntityDeal, datasource.EntityProduct, datasource.EntityOfferTemplate, datasource.EntityOffer:
		return p.deals.ListFilters(t)
	case datasource.EntityCommission:
		return p.commissions.ListFilters(t)
	case datasource.EntityDealRoom, datasource.EntityDealRoomThread, datasource.EntityDealRoomComment, datasource.EntityDealRoomDocument, datasource.EntityDealRoomParticipant:
		return p.dealRooms.ListFilters(t)
	case datasource.EntityProject:
		return p.projects.ListFilters(t)
	case datasource.EntityActivity:
		return p.activities.ListFilters(t)
	default:
		return nil
	}
}

// Search answers one page of one record type's records, which is what
// list_records asks for. Ranking by words is the search module's engine, which
// search_records, an intent tool's record_name and GET /v1/search share.
//
// The cursor names the type it was minted for. A token presented on another
// type's search is refused rather than resumed in the wrong table.
func (p *Provider) Search(ctx context.Context, q datasource.SearchQuery) (datasource.SearchResult, error) {
	if len(q.EntityTypes) != 1 {
		return datasource.SearchResult{}, fmt.Errorf(
			"compose: Search pages exactly one record type, and this query names %d", len(q.EntityTypes))
	}
	et := q.EntityTypes[0]
	position, err := storekit.DecodeSweepCursor(q.Cursor)
	if err != nil {
		return datasource.SearchResult{}, err
	}
	if position.Stream != "" && position.Stream != string(et) {
		return datasource.SearchResult{}, &storekit.MalformedCursorError{}
	}
	text := &q.Text
	if q.Text == "" {
		text = nil
	}
	limit := q.Limit
	if limit <= 0 {
		limit = defaultSearchPageSize
	}
	records, next, err := p.searchOneType(ctx, et, text, limit, position.Inner, q.Filters)
	if err != nil {
		return datasource.SearchResult{}, err
	}
	out := datasource.SearchResult{Records: records}
	if out.Records == nil {
		out.Records = []datasource.Record{}
	}
	if next == "" {
		return out, nil
	}
	cursor, err := storekit.EncodeSweepCursor(storekit.SweepCursor{Stream: string(et), Inner: next})
	if err != nil {
		return datasource.SearchResult{}, err
	}
	out.NextCursor, out.HasMore = cursor, true
	return out, nil
}

// searchOneType pages one entity type through the module that owns it. A
// denied type is the caller's to hear: an empty page would say it holds nothing.
func (p *Provider) searchOneType(ctx context.Context, t datasource.EntityType, text *string, limit int,
	cursor string, filters map[string]string,
) ([]datasource.Record, string, error) {
	var inner *string
	if cursor != "" {
		inner = &cursor
	}
	var (
		records []datasource.Record
		next    string
		err     error
	)
	switch t {
	case datasource.EntityContact, datasource.EntityCompany, datasource.EntityLead,
		datasource.EntityPartner:
		records, next, _, err = p.contacts.SearchEntity(ctx, t, text, limit, inner, filters)
	case datasource.EntityDeal, datasource.EntityProduct, datasource.EntityOfferTemplate, datasource.EntityOffer:
		records, next, _, err = p.deals.SearchEntity(ctx, t, text, limit, inner, filters)
	case datasource.EntityCommission:
		records, next, _, err = p.commissions.SearchEntity(ctx, t, text, limit, inner, filters)
	case datasource.EntityDealRoom, datasource.EntityDealRoomThread, datasource.EntityDealRoomComment, datasource.EntityDealRoomDocument, datasource.EntityDealRoomParticipant:
		records, next, _, err = p.dealRooms.SearchEntity(ctx, t, text, limit, inner, filters)
	case datasource.EntityProject:
		records, next, _, err = p.projects.SearchEntity(ctx, t, text, limit, inner, filters)
	case datasource.EntityActivity:
		records, next, _, err = p.activities.SearchEntity(ctx, t, text, limit, inner, filters)
	default:
		return nil, "", &datasource.UnsupportedEntityError{Type: string(t)}
	}
	return records, next, err
}

func (p *Provider) Create(ctx context.Context, in datasource.CreateInput) (datasource.EntityRef, error) {
	switch in.EntityType {
	case datasource.EntityContact, datasource.EntityCompany, datasource.EntityLead,
		datasource.EntityRelationship:
		return p.contacts.Create(ctx, in)
	case datasource.EntityDeal, datasource.EntityProduct, datasource.EntityOfferTemplate, datasource.EntityOffer:
		return p.deals.Create(ctx, in)
	case datasource.EntityDealRoom, datasource.EntityDealRoomThread, datasource.EntityDealRoomComment:
		return p.dealRooms.Create(ctx, in)
	case datasource.EntityProject:
		return p.projects.Create(ctx, in)
	case datasource.EntityActivity:
		return p.activities.Create(ctx, in)
	default:
		return datasource.EntityRef{}, &datasource.UnsupportedEntityError{Type: string(in.EntityType)}
	}
}

func (p *Provider) Update(ctx context.Context, in datasource.UpdateInput) (datasource.EntityRef, error) {
	switch in.Ref.Type {
	case datasource.EntityContact, datasource.EntityCompany, datasource.EntityLead,
		datasource.EntityRelationship:
		return p.contacts.Update(ctx, in)
	case datasource.EntityDeal, datasource.EntityProduct, datasource.EntityOfferTemplate, datasource.EntityOffer:
		return p.deals.Update(ctx, in)
	case datasource.EntityProject:
		return p.projects.Update(ctx, in)
	case datasource.EntityActivity:
		return p.activities.Update(ctx, in)
	default:
		return datasource.EntityRef{}, &datasource.UnsupportedEntityError{Type: string(in.Ref.Type)}
	}
}

func (p *Provider) Archive(ctx context.Context, r datasource.EntityRef) (datasource.EntityRef, error) {
	return p.ArchiveAt(ctx, datasource.ArchiveInput{Ref: r})
}

// archiverFor routes one entity type to the module that archives it.
//
// It is the switch Archive, ArchiveAt and RefuseArchive all used to spell
// separately, and separately is how three copies of one routing table drift.
// The unsupported answer is the caller's to give, so this returns nil rather
// than an error: each caller's own signature decides what "no module archives
// this" looks like on the wire.
//
//nolint:ireturn // the routing IS the return: three module providers answer one question, and naming one of them here would be a fourth copy of the switch
func (p *Provider) archiverFor(t datasource.EntityType) datasource.RecordArchiverV2 {
	switch t {
	case datasource.EntityContact, datasource.EntityCompany, datasource.EntityRelationship:
		return p.contacts
	case datasource.EntityDeal, datasource.EntityProduct, datasource.EntityOfferTemplate, datasource.EntityOffer:
		return p.deals
	case datasource.EntityProject:
		return p.projects
	case datasource.EntityActivity:
		return p.activities
	default:
		return nil
	}
}

// ArchivableTypes is datasource.RecordArchiverV2's: the union of what the
// modules below archive, asked of them rather than restated here. A module that
// learns a new type enrols it by answering for it, which is what stops this
// list and the switch above from disagreeing.
func (p *Provider) ArchivableTypes(ctx context.Context) ([]datasource.EntityType, error) {
	var out []datasource.EntityType
	for _, module := range []datasource.RecordArchiverV2{p.contacts, p.deals, p.projects, p.activities} {
		types, err := module.ArchivableTypes(ctx)
		if err != nil {
			return nil, err
		}
		out = append(out, types...)
	}
	slices.Sort(out)
	// Compacted because this list is rendered to a model in a refusal
	// ("it archives contact, company, deal, …"), and the modules below
	// are disjoint today by construction rather than by anything that
	// checks. A type two of them both claimed would read as said twice.
	return slices.Compact(out), nil
}

// RefuseArchive is datasource.RecordArchiverV2's stage-time half, routed to the
// module that would perform the write.
func (p *Provider) RefuseArchive(ctx context.Context, r datasource.EntityRef) error {
	module := p.archiverFor(r.Type)
	if module == nil {
		return &datasource.UnsupportedEntityError{Type: string(r.Type)}
	}
	return module.RefuseArchive(ctx, r)
}

// ArchiveAt is Archive carrying the version the caller's authority named.
func (p *Provider) ArchiveAt(ctx context.Context, in datasource.ArchiveInput) (datasource.EntityRef, error) {
	module := p.archiverFor(in.Ref.Type)
	if module == nil {
		return datasource.EntityRef{}, &datasource.UnsupportedEntityError{Type: string(in.Ref.Type)}
	}
	return module.ArchiveAt(ctx, in)
}

// ReadLeadForReversal retains native authorization for a promoted lead that ordinary reads hide.
func (p *Provider) ReadLeadForReversal(ctx context.Context, id ids.UUID) (datasource.Record, error) {
	return p.contacts.ReadLeadForReversal(ctx, id)
}
