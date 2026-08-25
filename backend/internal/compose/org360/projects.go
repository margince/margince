// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package org360

// The projects section: the bodies of work this company carries, read
// through the projects store so the page and the projects list agree about
// which rows a caller may see.

import (
	crmcontracts "github.com/gradionhq/margince/backend/internal/contracts"
	"github.com/gradionhq/margince/backend/internal/platform/auth"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/principal"
)

func (a *assembly) readProjects() error {
	if err := auth.Require(a.ctx, "project", principal.ActionRead); err != nil {
		return err
	}
	projects, hasMore, err := a.svc.projects.ListProjectsForOrganizationTx(a.ctx, a.tx, a.orgID)
	if err != nil {
		return err
	}
	a.out.Projects = &projects
	a.out.ProjectsPage = &crmcontracts.PageInfo{HasMore: hasMore}
	return nil
}
