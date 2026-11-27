// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The undo of an imported project asks whether a colleague worked on it, and
// archives it. A colleague's edit that commits between the two must keep it.

import (
	"errors"
	"maps"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/projects"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestAnEditCommittedWhileTheProjectArchiveWaitsKeepsTheProject(t *testing.T) {
	e := Setup(t)
	admin := e.Admin()
	company := ids.From[ids.CompanyKind](e.SeedCompany(t, "Archive Race GmbH", nil))
	created, err := e.Projects.CreateProject(admin, projects.CreateProjectInput{Name: "Imported", CompanyID: company, Source: "import"})
	if err != nil {
		t.Fatalf("creating the project: %v", err)
	}
	project := ids.From[ids.ProjectKind](ids.UUID(created.Id))
	imported := seedInstant(t)

	editing, archived := make(chan struct{}), make(chan error, 1)
	description := "A rep called them"
	go func() {
		<-editing
		_, err := e.Projects.ArchiveProject(admin, project, nil, storekit.NotTouchedByHumanSince(imported))
		archived <- err
	}()
	err = database.WithWorkspaceTx(admin, e.Pool, func(tx pgx.Tx) error {
		if _, err := e.Projects.UpdateProjectTx(admin, tx, project, projects.UpdateProjectInput{Description: &description}, projects.CustomColumns{}); err != nil {
			return err
		}
		close(editing)
		waitForALockWaiter(t)
		return nil
	})
	if err != nil {
		t.Fatalf("the colleague's edit: %v", err)
	}
	var touched *storekit.HumanTouchedError
	if err := <-archived; !errors.As(err, &touched) {
		t.Fatalf("archive that waited on a colleague's edit → %v, want the touched refusal", err)
	}
	kept, err := e.Projects.GetProject(admin, project, storekit.LiveOnly)
	if err != nil || kept.ArchivedAt != nil {
		t.Fatalf("project after the refusal = %+v, %v, want it live", kept, err)
	}
}

// waitForALockWaiter returns once some session waits on a row lock, which is
// the archive standing behind the edit.
func waitForALockWaiter(t *testing.T) {
	t.Helper()
	conn := OwnerConn(t)
	for range 500 {
		var waiting bool
		if err := conn.QueryRow(t.Context(), `SELECT EXISTS (SELECT 1 FROM pg_stat_activity WHERE wait_event_type = 'Lock' AND datname = current_database())`).Scan(&waiting); err != nil {
			t.Fatalf("reading the lock waiters: %v", err)
		}
		if waiting {
			return
		}
		//craft:ignore test-sleep the wait is on a database lock state read from the server, which has no signal to block on
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("the archive never waited on the edit's row lock")
}

// REST creates a project on a company its caller may read and not change.
// The import's test run and its commit take the same company.
func TestAProjectMayBeCreatedOnACompanyThatIsOnlyReadable(t *testing.T) {
	e := Setup(t)
	company := ids.From[ids.CompanyKind](e.SeedCompany(t, "Read Only AG", nil))
	objects := maps.Clone(AdminPerms.Objects)
	objects["company"] = principal.ObjectGrant{Read: true}
	reader := e.As(e.AdminUser, nil, principal.Permissions{RoleKeys: AdminPerms.RoleKeys, Objects: objects, RowScope: principal.RowScopeAll})
	if _, err := e.Projects.CreateProject(reader, projects.CreateProjectInput{Name: "Through REST", CompanyID: company, Source: "manual"}); err != nil {
		t.Fatalf("the REST create on a readable company: %v", err)
	}
	in := projects.CreateProjectInput{Name: "Through the import", CompanyID: company, Source: "import"}
	err := database.WithWorkspaceTx(reader, e.Pool, func(tx pgx.Tx) error {
		if err := e.Projects.RefuseImportCreate(reader, tx, in, projects.CustomColumns{}); err != nil {
			return err
		}
		_, err := e.Projects.CreateProjectTx(reader, tx, in, projects.CustomColumns{})
		return err
	})
	if err != nil {
		t.Fatalf("the import's create on a readable company: %v", err)
	}
}
