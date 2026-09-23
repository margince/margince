// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import "github.com/margince/margince/backend/internal/shared/kernel/ids"

// Which endpoints each kind of edge takes, refused at the door.
//
// The database already says this, in one `rel_<kind>_shape` CHECK per kind, and
// two of those said it incompletely: `employment` and `deal_stakeholder` never
// required `counterparty_company_id IS NULL`, so an employment carrying a
// counterparty company LANDED. Nothing in Go closed the gap either — the insert
// writes whatever endpoints it is handed, and the endpoint probe beside it only
// asks whether each named record exists, never whether the SET of them belongs
// to the kind.
//
// What that costs is a row every reader is quietly wrong about. A surface
// naming "the other end" walks the endpoint columns in a fixed order and names
// the first it finds, so a three-endpoint employment reads as a link to the
// company and never mentions the counterparty. Not a disclosure — each column
// carries its own visibility and erasure conjunctions, so an endpoint the
// caller may not see is withheld rather than leaked — but a history line that
// is incomplete and says nothing about being incomplete.
//
// HERE RATHER THAN IN THE CHECK, and the reason is what a refusal can say. A
// CHECK violation names a constraint; this names the field, at the door, before
// anything is locked or probed. Tightening the two CHECKs is the other half and
// it is a separate change: an existing three-endpoint row would fail the new
// constraint, so it wants a check-and-report pass first.
//
// THE TABLE IS A DECLARED MIRROR of those constraints, not a second opinion:
// TestTheEndpointShapesAgreeWithTheDatabase reads `pg_get_constraintdef` for
// every `rel_%_shape` and fails in both directions — a kind whose SQL shape
// this table does not match, and a kind in this table the database has never
// heard of. Two answers to "which endpoints does this kind take" is precisely
// how the incomplete pair survived, so there is one answer and a test that the
// other party still gives it.

// relationshipShape names the endpoint columns one kind requires and the ones
// it refuses. Every column not listed as required must be absent: a kind takes
// exactly what it declares, so a column added to the table without a decision
// per kind is refused by default rather than admitted by omission.
type relationshipShape struct {
	// requires are the endpoint columns that must carry an id.
	requires []string
	// distinct names two columns that must not be the same record — a company
	// is not its own partner, and a contact does not work with themselves.
	distinct [2]string
}

// Column names as the schema spells them, so the mirror test can compare this
// table with a constraint definition without translating either.
const (
	endContactColumn             = "contact_id"
	endCompanyColumn             = "company_id"
	endCounterpartyContactColumn = "counterparty_contact_id"
	endCounterpartyCompanyColumn = "counterparty_company_id"
	endDealColumn                = "deal_id"
	endProjectColumn             = "project_id"
)

// relationshipEndpointColumns is the endpoint columns the table carries, which
// is what makes "everything not required is refused" a closed statement rather
// than a list somebody must remember to extend.
//
// Held by: TestTheEndpointColumnsAreTheSchemasOwn
// (internal/modules/contacts/relationshipshape_integration_test.go), which
// reads the columns the shape constraints speak about and fails when this list
// is not them.
var relationshipEndpointColumns = []string{
	endContactColumn, endCompanyColumn, endCounterpartyContactColumn,
	endCounterpartyCompanyColumn, endDealColumn, endProjectColumn,
}

var relationshipShapes = map[string]relationshipShape{
	employmentKind:         {requires: []string{endContactColumn, endCompanyColumn}},
	dealStakeholderKind:    {requires: []string{endDealColumn, endContactColumn}},
	ProjectStakeholderKind: {requires: []string{endProjectColumn, endContactColumn}},
	ProjectCompanyKind:     {requires: []string{endProjectColumn, endCompanyColumn}},
	BillingContactKind:     {requires: []string{endContactColumn, endCompanyColumn}},
	worksWithKind: {
		requires: []string{endContactColumn, endCounterpartyContactColumn},
		distinct: [2]string{endContactColumn, endCounterpartyContactColumn},
	},
	"partner_of": {
		requires: []string{endCompanyColumn, endCounterpartyCompanyColumn},
		distinct: [2]string{endCompanyColumn, endCounterpartyCompanyColumn},
	},
	"referred_by": {
		requires: []string{endCompanyColumn, endCounterpartyCompanyColumn},
		distinct: [2]string{endCompanyColumn, endCounterpartyCompanyColumn},
	},
	"co_sell_with": {
		requires: []string{endCompanyColumn, endCounterpartyCompanyColumn},
		distinct: [2]string{endCompanyColumn, endCounterpartyCompanyColumn},
	},
}

// suppliedEndpoints reads the request's endpoint ids by column name, so the
// shape rule and the mirror test both speak the schema's vocabulary.
func (in CreateRelationshipInput) suppliedEndpoints() map[string]ids.UUID {
	supplied := map[string]ids.UUID{}
	add := func(column string, id *ids.UUID) {
		if id != nil {
			supplied[column] = *id
		}
	}
	add(endContactColumn, untypedPtr(in.ContactID))
	add(endCompanyColumn, untypedPtr(in.CompanyID))
	add(endCounterpartyContactColumn, untypedPtr(in.CounterpartyContactID))
	add(endCounterpartyCompanyColumn, untypedPtr(in.CounterpartyCompanyID))
	add(endDealColumn, untypedPtr(in.DealID))
	add(endProjectColumn, untypedPtr(in.ProjectID))
	return supplied
}

// validRelationshipShape refuses an edge whose endpoint SET does not match its
// kind — a required end missing, or an end the kind does not take.
func validRelationshipShape(kind string, in CreateRelationshipInput) error {
	shape, known := relationshipShapes[kind]
	if !known {
		// Unreachable through either entry point: both check relationshipKinds
		// first. Refusing rather than admitting keeps that true when a kind is
		// added to the vocabulary and not to this table.
		return &RelationshipShapeError{Kind: kind}
	}
	supplied := in.suppliedEndpoints()
	required := map[string]bool{}
	for _, column := range shape.requires {
		required[column] = true
		if _, given := supplied[column]; !given {
			return &RelationshipShapeError{Kind: kind}
		}
	}
	for _, column := range relationshipEndpointColumns {
		if _, given := supplied[column]; given && !required[column] {
			return &RelationshipShapeError{Kind: kind}
		}
	}
	if left, right := shape.distinct[0], shape.distinct[1]; left != "" && supplied[left] == supplied[right] {
		return &RelationshipShapeError{Kind: kind}
	}
	return nil
}
