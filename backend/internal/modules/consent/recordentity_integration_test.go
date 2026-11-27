// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

// The legal entity that answers for a record nobody is acting on: the one
// its owner works for. A branch entity with a test country and its own
// request deadlines stands in for a second company.

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
	"github.com/margince/margince/backend/internal/shared/ports/jurisdiction"
	"github.com/margince/margince/backend/internal/shared/ports/subjectrightsrules"
	pubjurisdiction "github.com/margince/margince/backend/pkg/extension/jurisdiction"
	"github.com/margince/margince/backend/pkg/extension/messaging"
)

const branchCountry jurisdiction.Code = "kq"

var branchRulesOnce sync.Once

// registerBranchRules gives the branch's country a 20-day erasure deadline.
func registerBranchRules() {
	branchRulesOnce.Do(func() {
		subjectrightsrules.Register(subjectrightsrules.Rules{
			Jurisdiction: branchCountry, Version: 1,
			Instruments: []messaging.Instrument{{Name: "Branch Act"}},
			Deadlines: map[subjectrightsrules.RequestKind]subjectrightsrules.Deadline{
				"erasure": {Period: pubjurisdiction.Period{Days: 20}},
			},
		})
	})
}

// joinBranch adds a branch entity in branchCountry and makes the env's user,
// who owns the env's contact, work for it.
func joinBranch(t *testing.T, e *resolveEnv) ids.UUID {
	t.Helper()
	branch := ids.NewV7()
	ctx := context.Background()
	if _, err := e.owner.Exec(ctx, `
		INSERT INTO legal_entity (id, display_name, legal_name, country, jurisdiction, captured_by)
		VALUES ($1, 'Branch', 'Branch Ltd', $2, $2, 'human:x')`, branch, string(branchCountry)); err != nil {
		t.Fatal(err)
	}
	if _, err := e.owner.Exec(ctx, `
		INSERT INTO legal_entity_member (legal_entity_id, user_id, assigned_by) VALUES ($1, $2, 'human:x')`,
		branch, e.user); err != nil {
		t.Fatal(err)
	}
	return branch
}

func ownContact(t *testing.T, e *resolveEnv) {
	t.Helper()
	if _, err := e.owner.Exec(context.Background(),
		`UPDATE contact SET owner_id = $2 WHERE id = $1`, e.contact, e.user); err != nil {
		t.Fatal(err)
	}
}

// TestTheContactOwnersEntityAnswersForTheRecord: once the contact's owner works
// for the branch, the erasure deadline is the branch's. The privacy page names
// the branch as controller, and Margince's own mail goes out for the branch.
// Before that, all three are the default entity's.
//
// Mutation: answer the default entity in recordEntityTx and every check after
// joinBranch fails.
func TestTheContactOwnersEntityAnswersForTheRecord(t *testing.T) {
	t.Parallel()
	registerBranchRules()
	e := setupResolve(t)
	ownContact(t, e)
	who := ruleSubject{Kind: entityContact, ID: e.contact.String()}
	received := time.Date(2026, time.March, 2, 9, 0, 0, 0, time.UTC)

	read := func() (time.Time, string, string, ids.UUID) {
		t.Helper()
		var due time.Time
		var rule, controller string
		var entity ids.UUID
		if err := e.store.db.Tx(e.ctx, func(tx pgx.Tx) error {
			var err error
			if due, rule, err = e.store.requestDeadlineTx(e.ctx, tx, who, "erasure", received); err != nil {
				return err
			}
			var page PrivacyInformation
			if err := readControllerTx(e.ctx, tx, &page, who); err != nil {
				return err
			}
			controller = page.Controller.LegalName
			send, err := withRecordEntity(e.ctx, tx, ConfirmationSend{ContactID: e.contact})
			entity = send.Entity
			return err
		}); err != nil {
			t.Fatalf("reading: %v", err)
		}
		return due, rule, controller, entity
	}

	due, _, controller, _ := read()
	if due.Equal(received.AddDate(0, 0, 20)) || controller == "Branch Ltd" {
		t.Fatalf("before the owner joined the branch: due %v, controller %q; want the default entity's", due, controller)
	}
	branch := joinBranch(t, e)
	due, rule, controller, entity := read()
	if !due.Equal(received.AddDate(0, 0, 20)) || rule == subjectrightsrules.GDPRRule {
		t.Errorf("erasure due %v under %q, want the branch's 20 days", due, rule)
	}
	if controller != "Branch Ltd" {
		t.Errorf("the privacy page names %q as controller, want Branch Ltd", controller)
	}
	if entity != branch {
		t.Errorf("Margince's own mail goes out for %v, want the branch %v", entity, branch)
	}
	var body string
	if err := e.store.db.Tx(e.ctx, func(tx pgx.Tx) error {
		rendered, err := e.store.controllerView(e.ctx, tx, Rendered{Body: "Please confirm."}, commsauthz.CategoryRecordConfirmation, e.address, who)
		body = rendered.Body
		return err
	}); err != nil {
		t.Fatalf("rendering the mail: %v", err)
	}
	if !strings.Contains(body, "Branch Ltd") {
		t.Errorf("the mail reads %q, want it sent as Branch Ltd", body)
	}
}
