// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/modules/projects"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestTheSameOutsideIDCannotNameTwoProjects(t *testing.T) {
	e := Setup(t)
	companyID := ids.From[ids.CompanyKind](e.SeedCompany(t, "Rollout Partner GmbH", nil))
	open := func(name string) error {
		_, err := e.Projects.CreateProject(e.Admin(), projects.CreateProjectInput{
			Name: name, CompanyID: companyID, Source: "manual",
			SourceSystem: new("project-erp"), SourceID: new("J-1"),
		})
		return err
	}
	if err := open("Rollout"); err != nil {
		t.Fatalf("opening the project: %v", err)
	}
	if err := open("Rollout again"); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("a second project with the same pair = %v, want a conflict", err)
	}
}
