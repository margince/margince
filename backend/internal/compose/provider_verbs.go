// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The provider's verbs beyond the record reads and writes in provider.go: the
// orchestration verbs, the schema descriptors and the options a role wires on.

import (
	"context"
	"time"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

func (p *Provider) Merge(ctx context.Context, in datasource.MergeInput) (datasource.EntityRef, error) {
	switch in.Type {
	case datasource.EntityContact, datasource.EntityCompany:
		return p.contacts.Merge(ctx, in)
	default:
		return datasource.EntityRef{}, &datasource.UnsupportedEntityError{Type: string(in.Type)}
	}
}

func (p *Provider) AdvanceDeal(ctx context.Context, in datasource.AdvanceDealInput) (datasource.EntityRef, error) {
	return p.deals.AdvanceDeal(ctx, in)
}

// StageSemantic feeds the advance_deal tier resolver: won or lost is read
// from the pipeline's configuration, never from a label or a request argument.
func (p *Provider) StageSemantic(ctx context.Context, stageID ids.UUID) (semantic string, pipelineID ids.UUID, err error) {
	return p.deals.StageSemantic(ctx, stageID)
}

// PromoteLead graduates a lead into a contact and its company, in the
// contacts module's transaction.
func (p *Provider) PromoteLead(ctx context.Context, id ids.UUID, trigger string, evidenceNote *string, ifVersion *int64) (datasource.EntityRef, bool, error) {
	return p.contacts.PromoteLead(ctx, id, trigger, evidenceNote, ifVersion)
}

// Freshness in SoR-mode is trivially authoritative: there is no mirror
// to go stale.
func (p *Provider) Freshness(_ context.Context, _ datasource.EntityRef) (datasource.FreshnessInfo, error) {
	return datasource.FreshnessInfo{LastSyncedAt: time.Now().UTC(), Authoritative: true}, nil
}

// ListObjects and ListFields expose the schema descriptors: static, and
// versioned with the code.
func (p *Provider) ListObjects(context.Context) ([]datasource.ObjectDef, error) {
	return schemaObjects, nil
}

func (p *Provider) ListFields(_ context.Context, entity datasource.EntityType) ([]datasource.FieldDef, error) {
	fields, ok := schemaFields(entity)
	if !ok {
		return nil, &datasource.UnsupportedEntityError{Type: string(entity)}
	}
	return fields, nil
}

// RunReport executes a seam-level plan against the descriptor vocabulary, on
// the engine the HTTP surface and the run_report tool both ride.
func (p *Provider) RunReport(ctx context.Context, plan datasource.ReportPlan) (datasource.ReportResult, error) {
	return p.reports.runAdHocPlan(ctx, plan)
}

// WithListMembers lets the record lists narrow by list_id, as the HTTP door's
// do while lists are on.
func (p *Provider) WithListMembers(resolve storekit.ListMemberFilterResolver) *Provider {
	p.contacts, p.deals = p.contacts.WithListMembers(resolve), p.deals.WithListMembers(resolve)
	return p
}

// WithTranscriptEnqueue lets a transcript CREATED over this seam start its own
// reading, the way one created over REST does. Without it the tool surface can
// store a transcript that nothing reads.
func (p *Provider) WithTranscriptEnqueue(enqueue activities.TranscriptReadEnqueue) *Provider {
	p.activities = p.activities.WithTranscriptEnqueue(enqueue)
	return p
}
