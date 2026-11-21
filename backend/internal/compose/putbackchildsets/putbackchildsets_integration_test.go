// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package putbackchildsets

// A merge can bring emails or domains onto a record after an edit of them.
// Put back of that edit must not be offered, because replaying it would drop
// the added rows.

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func seamFor(e *integration.Env) compose.RestoreSeam {
	return compose.NewRestoreSeam(e.Pool, compose.NewProvider(e.Pool), deals.NewStore(e.DB(), compose.DealsInstallation()))
}

func latestUpdate(t *testing.T, e *integration.Env, entityType string, id ids.UUID) (entry ids.UUID, version int64) {
	t.Helper()
	admin := e.Admin()
	err := database.WithWorkspaceTx(admin, e.Pool, func(tx pgx.Tx) error {
		if err := tx.QueryRow(admin, `SELECT id FROM audit_log WHERE entity_type = $1 AND entity_id = $2 AND action = 'update'
			ORDER BY occurred_at DESC, id DESC LIMIT 1`, entityType, id).Scan(&entry); err != nil {
			return err
		}
		return tx.QueryRow(admin, `SELECT version FROM `+pgx.Identifier{entityType}.Sanitize()+` WHERE id = $1`, id).Scan(&version)
	})
	if err != nil {
		t.Fatalf("read the latest %s update entry: %v", entityType, err)
	}
	return entry, version
}

// wantSuperseded asks the history listing and the restore itself about one entry.
func wantSuperseded(t *testing.T, e *integration.Env, entityType string, id, entry ids.UUID, version int64) {
	t.Helper()
	admin := e.Admin()
	answers, err := compose.NewUndoabilityPage(seamFor(e)).ForRecord(admin, entityType, id, []ids.UUID{entry})
	if err != nil {
		t.Fatalf("judging the entry: %v", err)
	}
	if answers[entry].Undoable {
		t.Error("the listing offers Put back over a set a merge changed since")
	}
	_, err = seamFor(e).Restore(admin, entityType, id, entry, version)
	var refusal compose.RefusedRestore
	if !errors.As(err, &refusal) || refusal.Reason != compose.ReasonSuperseded {
		t.Fatalf("putting the earlier edit back answered %v, want superseded", err)
	}
}

func TestACompanyMergeThatBringsDomainsOverridesAnEarlierDomainEdit(t *testing.T) {
	e := integration.Setup(t)
	target, source := e.SeedCompany(t, "Survivor Co", nil), e.SeedCompany(t, "Retired Co", nil)
	tag := target.String()[:8]
	setDomains := func(id ids.UUID, names ...string) {
		t.Helper()
		in := make([]contacts.CompanyDomainInput, len(names))
		for i, name := range names {
			in[i] = contacts.CompanyDomainInput{Domain: name + "-" + tag + ".test", IsPrimary: i == 0}
		}
		if _, err := e.Contacts.UpdateCompany(e.Admin(), ids.From[ids.CompanyKind](id), contacts.UpdateCompanyInput{Domains: &in}); err != nil {
			t.Fatalf("set domains %v: %v", names, err)
		}
	}
	setDomains(target, "a")
	setDomains(target, "b")
	entry, _ := latestUpdate(t, e, "company", target)
	setDomains(source, "carried")
	if _, err := e.Contacts.MergeCompany(e.Admin(), ids.From[ids.CompanyKind](source), ids.From[ids.CompanyKind](target), nil); err != nil {
		t.Fatalf("merging the companies: %v", err)
	}
	_, version := latestUpdate(t, e, "company", target)
	wantSuperseded(t, e, "company", target, entry, version)
}

func TestAContactMergeThatBringsEmailsOverridesAnEarlierEmailEdit(t *testing.T) {
	e := integration.Setup(t)
	target, source := e.SeedContact(t, "Surviving Contact", nil), e.SeedContact(t, "Retired Contact", nil)
	tag := target.String()[:8]
	setEmail := func(id ids.UUID, local string) {
		t.Helper()
		in := []contacts.ContactEmailInput{{Email: local + "-" + tag + "@example.test", EmailType: "work", IsPrimary: true}}
		if _, err := e.Contacts.UpdateContact(e.Admin(), ids.From[ids.ContactKind](id), contacts.UpdateContactInput{Emails: in}); err != nil {
			t.Fatalf("set email %s: %v", local, err)
		}
	}
	setEmail(target, "a")
	setEmail(target, "b")
	entry, _ := latestUpdate(t, e, "contact", target)
	setEmail(source, "carried")
	if _, err := e.Contacts.MergeContact(e.Admin(), ids.From[ids.ContactKind](source), ids.From[ids.ContactKind](target), nil); err != nil {
		t.Fatalf("merging the contacts: %v", err)
	}
	_, version := latestUpdate(t, e, "contact", target)
	wantSuperseded(t, e, "contact", target, entry, version)
}
