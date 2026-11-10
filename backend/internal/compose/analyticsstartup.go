// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func (s *Server) analyticsStartup(ctx context.Context) (*crmcontracts.AnalyticsStartup, error) {
	if !auth.Allows(ctx, "report_definition", principal.ActionRead) || !auth.Allows(ctx, "reporting_framework", principal.ActionRead) {
		return nil, nil //nolint:nilnil // ungranted callers omit optional startup data
	}
	frame, err := s.analyticsContextHandlers.read(ctx)
	if err != nil {
		return nil, err
	}
	catalog, err := s.reportMetrics.Catalog(ctx)
	if err != nil {
		return nil, err
	}
	framework, err := s.reportMetrics.GetFramework(ctx)
	if err != nil {
		return nil, err
	}
	settings, err := s.installationSettingsHandlers.read(ctx)
	if err != nil {
		return nil, err
	}
	out := &crmcontracts.AnalyticsStartup{Context: frame, Catalog: catalog, Framework: framework, InstallationSettings: settings, CompanyContext: s.contextCapabilities()}
	if auth.Allows(ctx, "pipeline", principal.ActionRead) {
		pipelines, err := s.dealsStore.ListPipelines(ctx, storekit.LiveOnly)
		if err != nil {
			return nil, err
		}
		out.Pipelines = &pipelines
	}
	return out, nil
}
