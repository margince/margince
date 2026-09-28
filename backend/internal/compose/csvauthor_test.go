// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

// The import writes through the stores rather than the create wires, so the
// author column asks the importer question itself. An agent carrying its
// human's import grant stages and commits imports through the same code, and
// must not become the one who names authors.
func TestTheAuthorColumnIsTheDeclaredImportersAlone(t *testing.T) {
	grant := principal.Permissions{Objects: map[string]principal.ObjectGrant{"import_run": {Create: true}}}
	as := func(p principal.Principal) context.Context {
		return principal.WithActor(context.Background(), p)
	}
	human := as(principal.Principal{Type: principal.PrincipalHuman, ID: "human:admin", Permissions: grant})
	agent := as(principal.Principal{Type: principal.PrincipalAgent, ID: "agent:assistant", OnBehalfOf: ids.NewV7(), Permissions: grant})
	withAuthor := map[string]string{"Name": "display_name", "Owner": csvTargetAuthor}
	withoutAuthor := map[string]string{"Name": "display_name"}

	if err := refuseAuthorFromNonImporter(human, withAuthor); err != nil {
		t.Errorf("the declared importer was refused its author column: %v", err)
	}
	var refused *provenance.AuthorError
	if err := refuseAuthorFromNonImporter(agent, withAuthor); !errors.As(err, &refused) || refused.Code != "reserved_source_author" {
		t.Errorf("an agent mapped an author column: %v", err)
	}
	if err := refuseAuthorFromNonImporter(agent, withoutAuthor); err != nil {
		t.Errorf("an agent's import without an author column was refused: %v", err)
	}
}
