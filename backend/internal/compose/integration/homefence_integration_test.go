// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The Home cache serves an answer only while the access fence holds. Every
// write that can take a record out of somebody's reach must move it. A write
// that only widens reach must not, or the cache would never be served.

import (
	"context"
	"strconv"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// fenceFixture is one case's own records, so no case moves another's fence.
type fenceFixture struct {
	contact, company, linked, unlinked, task ids.UUID
}

func seedFenceFixture(t *testing.T, e *Env) fenceFixture {
	t.Helper()
	f := fenceFixture{
		contact:  e.SeedContact(t, "Fence Contact", &e.Rep1),
		company:  e.SeedCompany(t, "Fence Company", &e.Rep1),
		linked:   ids.NewV7(),
		unlinked: ids.NewV7(),
		task:     ids.NewV7(),
	}
	for _, id := range []ids.UUID{f.linked, f.unlinked} {
		e.WsExec(t, `INSERT INTO activity (id, kind, subject, source, captured_by)
			VALUES ($1, 'note', 'Fence note', 'manual', 'human:fence')`, id)
	}
	e.WsExec(t, `INSERT INTO activity (id, kind, subject, assignee_id, source, captured_by)
		VALUES ($1, 'task', 'Fence task', $2, 'manual', 'human:fence')`, f.task, e.Rep1)
	e.WsExec(t, `INSERT INTO activity_link (activity_id, entity_type, contact_id) VALUES ($1, 'contact', $2)`,
		f.linked, f.contact)
	e.WsExec(t, `INSERT INTO activity_participant (activity_id, role, address) VALUES ($1, 'to', 'fence@example.test')`,
		f.linked)
	return f
}

func homeFence(t *testing.T, e *Env) int64 {
	t.Helper()
	n, err := strconv.ParseInt(e.WsScalar(t, `SELECT generation::text FROM home_read_fence`), 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

// inOneTx runs the statements as one transaction, the way a writer would.
func inOneTx(t *testing.T, e *Env, statements []fenceStatement) {
	t.Helper()
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		for _, s := range statements {
			if _, err := tx.Exec(ctx, s.sql, s.args...); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("writing: %v", err)
	}
}

type fenceStatement struct {
	sql  string
	args []any
}

func TestEveryNarrowingWriteAdvancesTheHomeFenceOncePerTransaction(t *testing.T) {
	e := Setup(t)
	cases := map[string]func(f fenceFixture) []fenceStatement{
		"a contact changes owner": func(f fenceFixture) []fenceStatement {
			return []fenceStatement{{`UPDATE contact SET owner_id = $1 WHERE id = $2`, []any{e.Rep2, f.contact}}}
		},
		"a contact is narrowed to its owner": func(f fenceFixture) []fenceStatement {
			return []fenceStatement{{`UPDATE contact SET visibility = 'owner' WHERE id = $1`, []any{f.contact}}}
		},
		"a company changes owner": func(f fenceFixture) []fenceStatement {
			return []fenceStatement{{`UPDATE company SET owner_id = $1 WHERE id = $2`, []any{e.Rep2, f.company}}}
		},
		"a record is shared": func(f fenceFixture) []fenceStatement {
			return []fenceStatement{{`INSERT INTO record_grant
				(record_type, record_id, subject_type, subject_id, access, granted_by)
				VALUES ('contact', $1, 'user', $2, 'read', $3)`, []any{f.contact, e.Rep3, e.AdminUser}}}
		},
		"a message's audience is limited": func(f fenceFixture) []fenceStatement {
			return []fenceStatement{{`UPDATE activity SET audience = 'participants' WHERE id = $1`, []any{f.linked}}}
		},
		"a task changes assignee": func(f fenceFixture) []fenceStatement {
			return []fenceStatement{{`UPDATE activity SET assignee_id = $1 WHERE id = $2`, []any{e.Rep2, f.task}}}
		},
		"a message loses its link": func(f fenceFixture) []fenceStatement {
			return []fenceStatement{{`DELETE FROM activity_link WHERE activity_id = $1`, []any{f.linked}}}
		},
		"an unlinked message gains its first link": func(f fenceFixture) []fenceStatement {
			return []fenceStatement{{
				`INSERT INTO activity_link (activity_id, entity_type, contact_id) VALUES ($1, 'contact', $2)`,
				[]any{f.unlinked, f.contact},
			}}
		},
		"a participant is removed": func(f fenceFixture) []fenceStatement {
			return []fenceStatement{{`DELETE FROM activity_participant WHERE activity_id = $1`, []any{f.linked}}}
		},
		"a contact is erased": func(f fenceFixture) []fenceStatement {
			return []fenceStatement{{`INSERT INTO audit_log (actor_type, actor_id, action, entity_type, entity_id)
				VALUES ('system', 'system:erasure', 'erase', 'contact', $1)`, []any{f.contact}}}
		},
		"two owners change in one transaction": func(f fenceFixture) []fenceStatement {
			return []fenceStatement{
				{`UPDATE contact SET owner_id = $1 WHERE id = $2`, []any{e.Rep2, f.contact}},
				{`UPDATE company SET owner_id = $1 WHERE id = $2`, []any{e.Rep2, f.company}},
			}
		},
	}
	for name, write := range cases {
		t.Run(name, func(t *testing.T) {
			f := seedFenceFixture(t, e)
			before := homeFence(t, e)
			inOneTx(t, e, write(f))
			if after := homeFence(t, e); after != before+1 {
				t.Errorf("fence %d → %d, want one step for one transaction", before, after)
			}
		})
	}

	t.Run("a role assignment changes", func(t *testing.T) {
		before := homeFence(t, e)
		e.GrantRole(t, e.Rep3, "manager")
		if homeFence(t, e) <= before {
			t.Error("a role change left the fence where it was")
		}
	})
}

func TestAWriteThatOnlyWidensReachLeavesTheHomeFence(t *testing.T) {
	e := Setup(t)
	cases := map[string]func(f fenceFixture) []fenceStatement{
		"a message's subject is edited": func(f fenceFixture) []fenceStatement {
			return []fenceStatement{{`UPDATE activity SET subject = 'Edited' WHERE id = $1`, []any{f.linked}}}
		},
		"a captured message arrives with its link": func(f fenceFixture) []fenceStatement {
			fresh := ids.NewV7()
			return []fenceStatement{
				{`INSERT INTO activity (id, kind, subject, source, captured_by)
					VALUES ($1, 'email', 'Arrived', 'manual', 'human:fence')`, []any{fresh}},
				{
					`INSERT INTO activity_link (activity_id, entity_type, contact_id) VALUES ($1, 'contact', $2)`,
					[]any{fresh, f.contact},
				},
			}
		},
		"a linked message gains a second link": func(f fenceFixture) []fenceStatement {
			return []fenceStatement{{
				`INSERT INTO activity_link (activity_id, entity_type, company_id) VALUES ($1, 'company', $2)`,
				[]any{f.linked, f.company},
			}}
		},
		"a participant is added": func(f fenceFixture) []fenceStatement {
			return []fenceStatement{{
				`INSERT INTO activity_participant (activity_id, role, address) VALUES ($1, 'cc', 'more@example.test')`,
				[]any{f.linked},
			}}
		},
		"a contact's name is edited": func(f fenceFixture) []fenceStatement {
			return []fenceStatement{{`UPDATE contact SET full_name = 'Renamed' WHERE id = $1`, []any{f.contact}}}
		},
	}
	for name, write := range cases {
		t.Run(name, func(t *testing.T) {
			f := seedFenceFixture(t, e)
			before := homeFence(t, e)
			inOneTx(t, e, write(f))
			if after := homeFence(t, e); after != before {
				t.Errorf("fence %d → %d, want it left where it was", before, after)
			}
		})
	}
}
