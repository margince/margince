// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contact360

// The projects section: the bodies of work this contact is part of — a live
// stakeholder seat, or a project of the company they work for today.

import (
	"context"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func (s *Service) projectsSection(ctx context.Context, tx pgx.Tx, contactID ids.ContactID, out *crmcontracts.Contact360) error {
	if err := requireRead(ctx, "project"); err != nil {
		return err
	}
	projects, err := s.projects.ListProjectsForContactTx(ctx, tx, contactID)
	if err != nil {
		return err
	}
	out.Projects = &projects
	return nil
}
