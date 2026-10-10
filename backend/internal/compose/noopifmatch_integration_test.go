// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/projects"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// noopUpdate is one module's update, sent with a body that names no field, so
// its patch is empty and the store writes nothing.
type noopUpdate struct {
	name string
	// seed creates the record and returns its version.
	seed func(t *testing.T) int64
	// sendEmpty sends the update with no field, under ifVersion.
	sendEmpty func(ifVersion int64) error
}

// An update that changes nothing still answers its If-Match: a stale version
// is version skew, and only the version the row holds goes through.
func TestAnUpdateThatChangesNothingStillRefusesAStaleIfMatch(t *testing.T) {
	e := integration.Setup(t)
	admin := e.Admin()

	for _, tc := range noopUpdates(admin, t, e) {
		t.Run(tc.name, func(t *testing.T) {
			version := tc.seed(t)
			if err := tc.sendEmpty(version - 1); !errors.Is(err, apperrors.ErrVersionSkew) {
				t.Fatalf("no-op update under a stale If-Match: err = %v, want ErrVersionSkew", err)
			}
			if err := tc.sendEmpty(version); err != nil {
				t.Fatalf("no-op update under the current If-Match: %v", err)
			}
		})
	}
}

func noopUpdates(admin context.Context, t *testing.T, e *integration.Env) []noopUpdate {
	t.Helper()
	var contactID ids.ContactID
	var companyID ids.CompanyID
	var projectID ids.ProjectID
	return []noopUpdate{
		{
			name: "contact",
			seed: func(t *testing.T) int64 {
				c, err := e.Contacts.CreateContact(admin, contacts.CreateContactInput{FullName: "Ilse Brandauer", Source: "manual"})
				if err != nil {
					t.Fatalf("create contact: %v", err)
				}
				contactID = ids.From[ids.ContactKind](ids.UUID(c.Id))
				return *c.Version
			},
			sendEmpty: func(ifVersion int64) error {
				_, err := e.Contacts.UpdateContact(admin, contactID,
					contacts.UpdateContactInput{IfVersion: &ifVersion})
				return err
			},
		},
		{
			name: "company",
			seed: func(t *testing.T) int64 {
				c, err := e.Contacts.CreateCompany(admin, contacts.CreateCompanyInput{DisplayName: "Brandauer GmbH", Source: "manual"})
				if err != nil {
					t.Fatalf("create company: %v", err)
				}
				companyID = ids.From[ids.CompanyKind](ids.UUID(c.Id))
				return *c.Version
			},
			sendEmpty: func(ifVersion int64) error {
				_, err := e.Contacts.UpdateCompany(admin, companyID,
					contacts.UpdateCompanyInput{IfVersion: &ifVersion})
				return err
			},
		},
		{
			name: "project",
			seed: func(t *testing.T) int64 {
				p, err := e.Projects.CreateProject(admin, projects.CreateProjectInput{
					Name: "Brandauer rollout", CompanyID: companyID, Source: "manual",
				})
				if err != nil {
					t.Fatalf("create project: %v", err)
				}
				projectID = ids.From[ids.ProjectKind](ids.UUID(p.Id))
				return *p.Version
			},
			sendEmpty: func(ifVersion int64) error {
				_, err := e.Projects.UpdateProject(admin, projectID,
					projects.UpdateProjectInput{IfVersion: &ifVersion})
				return err
			},
		},
	}
}
