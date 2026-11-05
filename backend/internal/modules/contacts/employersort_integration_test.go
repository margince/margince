// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The Company sort reads for a reader who may see employers. It also reads for
// one who may read the edge but not the company; that sort binds no argument.
func TestTheEmployerSortReadsWithAndWithoutAVisibleEmployer(t *testing.T) {
	e := setupCapturePrivacy(t)
	seedContacts(t, e, e.owner, 3)
	withEdges := func(company bool) context.Context {
		ctx := e.as(e.owner, principal.RowScopeOwn)
		actor, ok := principal.Actor(ctx)
		if !ok {
			t.Fatal("no actor bound")
		}
		actor.Permissions.Objects["relationship"] = principal.ObjectGrant{Read: true}
		if !company {
			delete(actor.Permissions.Objects, "company")
		}
		return principal.WithActor(ctx, actor)
	}
	for name, ctx := range map[string]context.Context{
		"employer visible": withEdges(true),
		"company refused":  withEdges(false),
	} {
		for _, sort := range []string{contactEmployerField, "-" + contactEmployerField} {
			rows, _, err := e.store.ListContacts(ctx, ListContactsInput{Sort: &sort})
			if err != nil {
				t.Fatalf("%s, sort %s: %v", name, sort, err)
			}
			if len(rows) != 3 {
				t.Errorf("%s, sort %s: %d rows, want 3", name, sort, len(rows))
			}
		}
	}
}
