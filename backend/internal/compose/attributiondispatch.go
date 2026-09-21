// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Which store owns which object type, and what grant each one takes.
//
// Split from attributionrepair.go, which is the ROUTE — the admission checks,
// the batch validation, and the per-row transaction that writes the ledger.
// This is the table that says where a row goes, and it is its own concept: the
// route can be read without knowing which module holds `lead`, and this can be
// read without knowing how a batch is validated.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The RBAC object names, spelled here rather than borrowed from
// agentpolicy_gen.go's recordType* constants. Those are `agentRecordType` — the
// vocabulary of what an AGENT may touch, generated from the contract's
// x-agent-access annotations — and these are authz object names. The two happen
// to share six spellings today, and binding one to the other would make a
// change to the agent policy silently re-point this route's permission checks.
const (
	authzObjectActivity = "activity"
	authzObjectContact  = "contact"
	authzObjectCompany  = "company"
	authzObjectDeal     = "deal"
	authzObjectLead     = "lead"
	authzObjectProject  = "project"
)

// attributableObjects maps an object_type this route dispatches to the RBAC
// object whose `update` grant it takes.
//
// ONE map for both questions, because they are the same question asked twice.
// The validation above and the dispatch below each need to know which types are
// real, and a type present in one list and missing from the other is a row
// accepted by the door and dropped by the floor — reported `applied` with
// nothing written, or refused for a type the store would have handled.
//
// The grant is the record's OWN object, not a single administrative one: this
// writes a column on a deal, and `deal:update` is what the RBAC vocabulary
// calls that. Who may reach the route at all is answered separately and more
// narrowly — RepairSourceAttribution takes auth.RequireAdmin, because restating
// who wrote somebody else's correspondence in batches is not an ordinary edit.
var attributableObjects = map[crmcontracts.SourceAttributionRowObjectType]string{
	crmcontracts.SourceAttributionRowObjectTypeActivity: authzObjectActivity,
	crmcontracts.SourceAttributionRowObjectTypeContact:  authzObjectContact,
	crmcontracts.SourceAttributionRowObjectTypeCompany:  authzObjectCompany,
	crmcontracts.SourceAttributionRowObjectTypeDeal:     authzObjectDeal,
	crmcontracts.SourceAttributionRowObjectTypeLead:     authzObjectLead,
	crmcontracts.SourceAttributionRowObjectTypeProject:  authzObjectProject,
}

// attributeRecord sends one row to the store that owns its table.
//
// The dispatch lives here because compose is the only layer permitted to call
// several modules, and each arm hands over a typed id: the stores take the id
// kind their own table is keyed on, so a contact id cannot reach the deal store
// by being a uuid.
//
// `contacts` owns three of the five record tables and answers them with three
// methods rather than one taking the table by name. The ownership census cannot
// attribute a write whose table it reads from a variable, so each of those
// spells its own table as a literal — which is why this switch has three arms
// where one would have read better.
func (h attributionHandlers) attributeRecord(
	ctx context.Context, tx pgx.Tx, row crmcontracts.SourceAttributionRow, in storekit.SourceAuthorInput,
) (storekit.SourceAuthorOutcome, string, error) {
	id := ids.UUID(row.ObjectId)
	switch row.ObjectType {
	case crmcontracts.SourceAttributionRowObjectTypeActivity:
		return h.activities.SetSourceAuthorTx(ctx, tx, ids.From[ids.ActivityKind](id), in)
	case crmcontracts.SourceAttributionRowObjectTypeContact:
		return h.contacts.SetContactSourceAuthorTx(ctx, tx, ids.From[ids.ContactKind](id), in)
	case crmcontracts.SourceAttributionRowObjectTypeCompany:
		return h.contacts.SetCompanySourceAuthorTx(ctx, tx, ids.From[ids.CompanyKind](id), in)
	case crmcontracts.SourceAttributionRowObjectTypeLead:
		return h.contacts.SetLeadSourceAuthorTx(ctx, tx, ids.From[ids.LeadKind](id), in)
	case crmcontracts.SourceAttributionRowObjectTypeDeal:
		return h.deals.SetDealSourceAuthorTx(ctx, tx, ids.From[ids.DealKind](id), in)
	case crmcontracts.SourceAttributionRowObjectTypeProject:
		return h.projects.SetProjectSourceAuthorTx(ctx, tx, ids.From[ids.ProjectKind](id), in)
	}
	// Unreachable through the route: attributionRowsRefused rejects any type
	// absent from attributableObjects before a row reaches here, and both read
	// the same map. Stated rather than omitted, because a future type added to
	// the map and not to this switch would otherwise fall out of the function
	// with a zero-valued outcome that reads as a successful skip.
	return storekit.SourceAuthorSkipped, "", fmt.Errorf(
		"compose: %q is in attributableObjects but no store claims it", row.ObjectType)
}
