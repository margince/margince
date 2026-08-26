// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The post-condition on a restore: a field it sent that is not the value the
// record now holds is not a success.
//
// The binding evaluation closes the windows it can see, by reading live state
// at write time. This closes any that remain — the decision and the write are
// separate transactions, and no version covers the custom-field catalog, so
// the required If-Match cannot span them. It is stated as a property of the
// WRITE rather than of one known race, because the failure is the same
// whatever produced it: a person told a change was put back when it was not.

import (
	"encoding/json"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/gradionhq/margince/backend/internal/compose/integration"
	"github.com/gradionhq/margince/backend/internal/modules/people"
	"github.com/gradionhq/margince/backend/internal/platform/database"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
)

func TestARestoreThatDidNotLandNamesTheFieldRatherThanReportingSuccess(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.Admin()

	title := "CTO"
	person, err := e.People.CreatePerson(ctx, people.CreatePersonInput{
		FullName: "Greta Landed", Title: &title, Source: "manual",
	})
	if err != nil {
		t.Fatalf("seed a person through the real writer: %v", err)
	}
	id := ids.UUID(person.Id)

	ask := func(patch map[string]json.RawMessage) []string {
		t.Helper()
		var missed []string
		err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
			var err error
			missed, err = fieldsThatDidNotLand(ctx, tx, "person", id, patch)
			return err
		})
		if err != nil {
			t.Fatalf("ask what did not land: %v", err)
		}
		return missed
	}

	// A value the record already holds names nothing. Without this the check
	// would refuse every restore, which is the opposite failure and just as bad.
	if missed := ask(map[string]json.RawMessage{"title": json.RawMessage(`"CTO"`)}); len(missed) != 0 {
		t.Errorf("a value the record holds reported as not landed: %v", missed)
	}

	// A value it does not hold names the field — the shape a write that
	// silently dropped a key leaves behind.
	missed := ask(map[string]json.RawMessage{"title": json.RawMessage(`"CEO"`)})
	if len(missed) != 1 || missed[0] != "title" {
		t.Errorf("a value the record does not hold reported as %v, want [title]", missed)
	}

	// A key the record has no column for is not silently ignored: that is
	// exactly the retired-custom-field case, and treating it as landed would
	// restore the defect this check exists for.
	if missed := ask(map[string]json.RawMessage{"cf_gone": json.RawMessage(`"x"`)}); len(missed) != 1 {
		t.Errorf("a key the record has no column for reported as %v, want it named", missed)
	}
}
