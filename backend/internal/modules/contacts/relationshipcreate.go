// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// Creating a relationship edge, in the two shapes a caller needs it: opening
// its own transaction, or borrowing one whose other writes must land with the
// edge or not at all. Both take the same gates in the same order and then meet
// in one writer, because the current-primary-employer rules below are the kind
// that drift silently — a second spelling would keep inserting rows while
// answering a different question about which employment is current.

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// CreateRelationshipInput is one edge to write: its kind, the endpoints that
// kind anchors, and the optional facts an employment carries.
type CreateRelationshipInput struct {
	EmploymentStatus      *string
	StartedPrecision      *string
	EndedPrecision        *string
	Kind                  string
	ContactID             *ids.ContactID
	CompanyID             *ids.CompanyID
	CounterpartyCompanyID *ids.CompanyID
	// CounterpartyContactID is the far end of the one contact↔contact kind
	// (works_with); nil for every other kind, whose shapes refuse it.
	CounterpartyContactID *ids.ContactID
	DealID                *ids.DealID
	ProjectID             *ids.ProjectID
	Role                  *string
	// IsCurrentPrimary is TRI-STATE, and the third state is what makes the rule
	// in the insert safe: nil means the caller expressed no opinion and the
	// store decides, false means they said this is NOT the contact's current
	// primary employment. Collapsing the two would silently invert a choice the
	// caller can see themselves making — the contact rail's "current employer"
	// checkbox sends exactly that false.
	IsCurrentPrimary *bool
	StartedAt        *time.Time
	EndedAt          *time.Time
	Source           string
}

// CreateRelationship writes one edge in a transaction of its own.
func (s *Store) CreateRelationship(ctx context.Context, in CreateRelationshipInput) (relationshipRow, error) {
	// A SUPPLIED kind outside the vocabulary is a different fault from an omitted
	// one, and they used to answer the same sentence: a caller who sent
	// kind="EMPLOYMENT" was told `kind` is required, which is factually wrong about
	// a field they can see in their own request. The case-sensitivity trap makes
	// that land in practice, so the refusal names the allowed set.
	if in.Kind == "" {
		return relationshipRow{}, &RequiredFieldError{Field: relationshipKindField}
	}
	if !relationshipKinds[in.Kind] {
		return relationshipRow{}, &RelationshipKindError{Kind: in.Kind}
	}
	if err := validBillingContactRole(in.Kind, in.Role); err != nil {
		return relationshipRow{}, err
	}
	anchorObject, _ := relationshipAnchor(in.Kind)
	if err := auth.Require(ctx, "relationship", principal.ActionCreate); err != nil {
		return relationshipRow{}, err
	}
	if err := auth.Require(ctx, anchorObject, principal.ActionUpdate); err != nil {
		// The edge annotates its anchor: without the anchor's write
		// grant, an edge would be an RBAC side door onto it.
		return relationshipRow{}, err
	}
	capturedBy, err := storekit.CapturedBy(ctx)
	if err != nil {
		return relationshipRow{}, err
	}

	var out relationshipRow
	err = s.tx(ctx, func(tx pgx.Tx) error {
		var err error
		out, err = writeRelationshipInTx(ctx, tx, in, capturedBy)
		return err
	})
	return out, err
}

// CreateRelationshipTx is CreateRelationship for a caller that already opened a
// transaction — one whose own write must land with this edge or not at all.
// Same gates in the same order; only the transaction is borrowed, exactly as
// CreateContactTx borrows one.
func (s *Store) CreateRelationshipTx(ctx context.Context, tx pgx.Tx, in CreateRelationshipInput) (relationshipRow, error) {
	if in.Kind == "" {
		return relationshipRow{}, &RequiredFieldError{Field: relationshipKindField}
	}
	if !relationshipKinds[in.Kind] {
		return relationshipRow{}, &RelationshipKindError{Kind: in.Kind}
	}
	if err := validBillingContactRole(in.Kind, in.Role); err != nil {
		return relationshipRow{}, err
	}
	anchorObject, _ := relationshipAnchor(in.Kind)
	if err := auth.Require(ctx, "relationship", principal.ActionCreate); err != nil {
		return relationshipRow{}, err
	}
	if err := auth.Require(ctx, anchorObject, principal.ActionUpdate); err != nil {
		return relationshipRow{}, err
	}
	capturedBy, err := storekit.CapturedBy(ctx)
	if err != nil {
		return relationshipRow{}, err
	}
	return writeRelationshipInTx(ctx, tx, in, capturedBy)
}

// writeRelationshipInTx is the edge's write, with the gates already taken by
// whichever entry point above the caller came through. It is one function
// rather than two copies because the primary-employer rules below are the kind
// that drift silently: a second spelling would keep inserting rows while
// answering a different question about which employment is current.
func writeRelationshipInTx(
	ctx context.Context,
	tx pgx.Tx,
	in CreateRelationshipInput,
	capturedBy string,
) (relationshipRow, error) {
	var out relationshipRow
	if err := validEmploymentAssertion(in.Kind, in.EmploymentStatus, in.StartedPrecision, in.EndedPrecision); err != nil {
		return out, err
	}
	// BEFORE anything is locked or probed. The endpoint probe below asks
	// whether each named record exists; this asks whether the SET of them
	// belongs to the kind, which is the question neither it nor the shape
	// CHECKs were asking in full (relationshipshape.go).
	if err := validRelationshipShape(in.Kind, in); err != nil {
		return out, err
	}
	// Before the endpoints are checked, because the check is what this lock
	// makes true: an archive in flight either commits first and LiveOnly
	// refuses this attach, or waits and sweeps the edge this writes with
	// everything else. Without it the two can interleave into a live
	// relationship on an archived contact.
	if in.ContactID != nil {
		if err := lockContactForAttach(ctx, tx, *in.ContactID); err != nil {
			return out, err
		}
	}
	// The counterparty contact is an attach target exactly like the anchor: an
	// archive racing this write must either refuse it or sweep the edge.
	if in.CounterpartyContactID != nil {
		if err := lockContactForAttach(ctx, tx, *in.CounterpartyContactID); err != nil {
			return out, err
		}
	}
	if err := ensureRelationshipEndpoints(ctx, tx, in); err != nil {
		return out, err
	}
	// The endpoint probe above answered which records the caller may SEE. This
	// answers the narrower question the write owes on the one record the edge
	// annotates: may they CHANGE it. Both entry points funnel through here, so
	// the gate is taken once for the transaction-borrowing shape as well.
	if err := ensureRelationshipAnchorWritable(ctx, tx, in.Kind, in.endpoints()); err != nil {
		return out, err
	}
	// Both rules below read the contact's OTHER employments and then write
	// against what they read, so they are one unit per contact or they are a
	// race: two concurrent unmarked employments at different companies would
	// each see no primary, each claim it, and one would come back 409 naming
	// a flag its caller never sent.
	if in.Kind == employmentKind && in.ContactID != nil {
		if err := storekit.LockWriteIdentity(ctx, tx, employmentKind, in.ContactID.String()); err != nil {
			return out, err
		}
	}
	if err := demoteForRelationshipCreate(ctx, tx, in); err != nil {
		return out, err
	}
	var err error
	if out, err = insertRelationshipRow(ctx, tx, in, capturedBy); err != nil {
		return out, mapRelationshipConstraint(err, in.Kind)
	}
	return out, emitRelationshipChange(ctx, tx, "create", nil, out)
}

// ensureRelationshipEndpoints validates every supplied endpoint as a
// client-supplied FK argument (H1): each named target must be visible
// under the caller's row scope before the edge lands.
func ensureRelationshipEndpoints(ctx context.Context, tx pgx.Tx, in CreateRelationshipInput) error {
	for _, ref := range []struct {
		table string
		id    *ids.UUID
	}{
		{anchorContact, untypedPtr(in.ContactID)},
		{anchorContact, untypedPtr(in.CounterpartyContactID)},
		{companyEntity, untypedPtr(in.CompanyID)},
		{companyEntity, untypedPtr(in.CounterpartyCompanyID)},
		{anchorDeal, untypedPtr(in.DealID)},
		{projectObjectName, untypedPtr(in.ProjectID)},
	} {
		if ref.id == nil {
			continue
		}
		if err := auth.EnsureLinkTarget(ctx, tx, ref.table, *ref.id); err != nil {
			return err
		}
	}
	return nil
}

// untypedPtr narrows an optional typed id back to the kernel UUID for
// the platform seams (auth, storekit) that speak untyped ids.
func untypedPtr[K ids.EntityKind](id *ids.ID[K]) *ids.UUID {
	if id == nil {
		return nil
	}
	return &id.UUID
}
