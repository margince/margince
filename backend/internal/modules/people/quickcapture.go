// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

// The fast path for a person read off a public profile and typed in by hand.
//
// It is CreatePerson plus the employer, in ONE transaction. Two calls were the
// obvious shape and the wrong one: a person created for the company they work
// at, whose employment write then fails, is a record nobody asked for sitting
// in the list with no employer — and the surface that made it has already told
// the reader it saved. One transaction has one outcome.
//
// The profile URL is stored exactly as given and is never fetched, here or
// anywhere downstream. That is the product's position, not an implementation
// detail: the reader visits the profile in their own browser, and the software
// only keeps what they bring back.

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// quickCaptureSource is the provenance every row this path writes carries. It
// is the same token the ordinary contact form sends, because it is the same
// claim: a human typed this. A separate token would split one provenance
// question into two answers for no gain a reader could name.
const quickCaptureSource = "manual"

// profileURLField is where a stated profile address lives on a person. The
// wire's `social` map is open, and `linkedin` is the key the rail reads and
// the site read writes — a second key would leave the rail blind to the value
// this path just stored.
const profileURLField = "linkedin"

// phoneTypeWork is the phone half of the same default the email above takes.
// Spelled apart from emailTypeWork because the two vocabularies are different
// closed sets that happen to share a word — person_phone admits `mobile` and
// `home`, person_email does not.
const phoneTypeWork = "work"

// QuickCaptureInput is one person as a reader of their public profile can
// state them, with the employer they named beside it.
type QuickCaptureInput struct {
	FullName string
	Title    *string
	// OrganizationID attaches an existing employer, OrganizationName creates
	// one. Both may be absent: a person with no employer is a person.
	// An id WINS over a name — a caller who picked a record from the list has
	// answered the question the name was only guessing at.
	OrganizationID   *ids.OrganizationID
	OrganizationName *string
	Role             *string
	ProfileURL       *string
	Email            *string
	Phone            *string
}

// QuickCaptureResult is the person that landed, plus the employer they were
// attached to and whether that employer is a record this call created.
type QuickCaptureResult struct {
	Person              crmcontracts.Person
	OrganizationID      *ids.OrganizationID
	OrganizationCreated bool
}

// QuickCapture writes the person, their employer and the edge between them, or
// writes none of them.
//
// Each write keeps its own gates: CreatePersonTx takes person:create,
// CreateOrganizationTx takes organization:create, and CreateRelationshipTx
// takes relationship:create plus person:update on the anchor. A seat holding
// only the first gets a person and a refusal, not a half-written pair, because
// the refusal rolls the transaction back.
func (s *Store) QuickCapture(ctx context.Context, in QuickCaptureInput) (QuickCaptureResult, error) {
	if strings.TrimSpace(in.FullName) == "" {
		return QuickCaptureResult{}, &RequiredFieldError{Field: fieldFullName}
	}
	var out QuickCaptureResult
	err := s.tx(ctx, func(tx pgx.Tx) error {
		var err error
		out, err = s.quickCaptureInTx(ctx, tx, in)
		return err
	})
	if err != nil {
		return QuickCaptureResult{}, err
	}
	return out, nil
}

func (s *Store) quickCaptureInTx(
	ctx context.Context,
	tx pgx.Tx,
	in QuickCaptureInput,
) (QuickCaptureResult, error) {
	var out QuickCaptureResult
	person, err := s.CreatePersonTx(ctx, tx, personFromQuickCapture(in))
	if err != nil {
		return out, err
	}
	out.Person = person

	orgID, created, err := s.employerForQuickCapture(ctx, tx, in)
	if err != nil {
		return out, err
	}
	if orgID == nil {
		return out, nil
	}
	out.OrganizationID = orgID
	out.OrganizationCreated = created

	personID := ids.From[ids.PersonKind](ids.UUID(person.Id))
	if _, err := s.CreateRelationshipTx(ctx, tx, CreateRelationshipInput{
		Kind:           employmentKind,
		PersonID:       &personID,
		OrganizationID: orgID,
		Role:           in.Role,
		Source:         quickCaptureSource,
	}); err != nil {
		return out, err
	}
	return out, nil
}

// employerForQuickCapture resolves the employer the caller named, creating one
// only when they gave a name and no id. A blank name is not a company: it is a
// field the reader left alone, and creating an organization called "" would put
// a record in the list that nobody can find or delete by name.
func (s *Store) employerForQuickCapture(
	ctx context.Context,
	tx pgx.Tx,
	in QuickCaptureInput,
) (orgID *ids.OrganizationID, created bool, err error) {
	if in.OrganizationID != nil {
		return in.OrganizationID, false, nil
	}
	if in.OrganizationName == nil {
		return nil, false, nil
	}
	name := strings.TrimSpace(*in.OrganizationName)
	if name == "" {
		return nil, false, nil
	}
	org, err := s.CreateOrganizationTx(ctx, tx, CreateOrganizationInput{
		DisplayName: name,
		Source:      quickCaptureSource,
	})
	if err != nil {
		return nil, false, err
	}
	made := ids.From[ids.OrganizationKind](ids.UUID(org.Id))
	return &made, true, nil
}

func personFromQuickCapture(in QuickCaptureInput) CreatePersonInput {
	person := CreatePersonInput{
		FullName: strings.TrimSpace(in.FullName),
		Title:    in.Title,
		Source:   quickCaptureSource,
	}
	if url := trimmedValue(in.ProfileURL); url != "" {
		person.Social = map[string]any{profileURLField: url}
	}
	if email := trimmedValue(in.Email); email != "" {
		person.Emails = []PersonEmailInput{{
			Email:     email,
			EmailType: emailTypeWork,
			IsPrimary: true,
			// A profile states an address; it is not correspondence, and the
			// mail ladder must not read it as a settled verdict about whether
			// this person writes to us.
			VouchedNotCorresponded: true,
		}}
	}
	if phone := trimmedValue(in.Phone); phone != "" {
		person.Phones = []PersonPhoneInput{{
			Phone:     phone,
			PhoneType: phoneTypeWork,
			IsPrimary: true,
		}}
	}
	return person
}

func trimmedValue(v *string) string {
	if v == nil {
		return ""
	}
	return strings.TrimSpace(*v)
}
