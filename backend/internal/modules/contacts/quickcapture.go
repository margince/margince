// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// The fast path for a contact read off a public profile and typed in by hand.
//
// It is CreateContact plus the employer, in ONE transaction. Two calls were the
// obvious shape and the wrong one: a contact created for the company they work
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
	"regexp"
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

// profileURLField is where a stated profile address lives on a contact. The
// wire's `social` map is open, and `linkedin` is the key the rail reads and
// the site read writes — a second key would leave the rail blind to the value
// this path just stored.
const profileURLField = "linkedin"

// phoneTypeWork is the phone half of the same default the email above takes.
// Spelled apart from emailTypeWork because the two vocabularies are different
// closed sets that happen to share a word — contact_phone admits `mobile` and
// `home`, contact_email does not.
const phoneTypeWork = "work"

// QuickCaptureInput is one contact as a reader of their public profile can
// state them, with the employer they named beside it.
type QuickCaptureInput struct {
	FullName string
	Title    *string
	// CompanyID attaches an existing employer, CompanyName creates
	// one. Both may be absent: a contact with no employer is a contact.
	// An id WINS over a name — a caller who picked a record from the list has
	// answered the question the name was only guessing at.
	CompanyID   *ids.CompanyID
	CompanyName *string
	Role        *string
	ProfileURL  *string
	Email       *string
	Phone       *string
}

// QuickCaptureResult is the contact that landed, plus the employer they were
// attached to and whether that employer is a record this call created.
type QuickCaptureResult struct {
	Contact        crmcontracts.Contact
	CompanyID      *ids.CompanyID
	CompanyCreated bool
}

// QuickCapture writes the contact, their employer and the edge between them, or
// writes none of them.
//
// Each write keeps its own gates: CreateContactTx takes contact:create,
// CreateCompanyTx takes company:create, and CreateRelationshipTx
// takes relationship:create plus contact:update on the anchor. A seat holding
// only the first gets a contact and a refusal, not a half-written pair, because
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
	contact, err := s.CreateContactTx(ctx, tx, contactFromQuickCapture(in))
	if err != nil {
		return out, err
	}
	out.Contact = contact

	companyID, created, err := s.employerForQuickCapture(ctx, tx, in)
	if err != nil {
		return out, err
	}
	if companyID == nil {
		return out, nil
	}
	out.CompanyID = companyID
	out.CompanyCreated = created

	contactID := ids.From[ids.ContactKind](ids.UUID(contact.Id))
	if _, err := s.CreateRelationshipTx(ctx, tx, CreateRelationshipInput{
		Kind:      employmentKind,
		ContactID: &contactID,
		CompanyID: companyID,
		Role:      in.Role,
		Source:    quickCaptureSource,
	}); err != nil {
		return out, err
	}
	return out, nil
}

// employerForQuickCapture resolves the employer the caller named, creating one
// only when they gave a name and no id. A blank name is not a company: it is a
// field the reader left alone, and creating a company called "" would put
// a record in the list that nobody can find or delete by name.
func (s *Store) employerForQuickCapture(
	ctx context.Context,
	tx pgx.Tx,
	in QuickCaptureInput,
) (companyID *ids.CompanyID, created bool, err error) {
	if in.CompanyID != nil {
		return in.CompanyID, false, nil
	}
	if in.CompanyName == nil {
		return nil, false, nil
	}
	name := strings.TrimSpace(*in.CompanyName)
	if name == "" {
		return nil, false, nil
	}
	company, err := s.CreateCompanyTx(ctx, tx, CreateCompanyInput{
		DisplayName: name,
		Source:      quickCaptureSource,
	})
	if err != nil {
		return nil, false, err
	}
	made := ids.From[ids.CompanyKind](ids.UUID(company.Id))
	return &made, true, nil
}

func contactFromQuickCapture(in QuickCaptureInput) CreateContactInput {
	contact := CreateContactInput{
		FullName: strings.TrimSpace(in.FullName),
		Title:    in.Title,
		Source:   quickCaptureSource,
	}
	if url := profileURLWithScheme(trimmedValue(in.ProfileURL)); url != "" {
		contact.Social = map[string]any{profileURLField: url}
	}
	if email := trimmedValue(in.Email); email != "" {
		contact.Emails = []ContactEmailInput{{
			Email:     email,
			EmailType: emailTypeWork,
			IsPrimary: true,
			// A profile states an address; it is not correspondence, and the
			// mail ladder must not read it as a settled verdict about whether
			// this contact writes to us.
			VouchedNotCorresponded: true,
		}}
	}
	if phone := trimmedValue(in.Phone); phone != "" {
		contact.Phones = []ContactPhoneInput{{
			Phone:     phone,
			PhoneType: phoneTypeWork,
			IsPrimary: true,
		}}
	}
	return contact
}

// profileURLWithScheme puts a scheme on a bare host.
//
// The browser hides `https://`, so what somebody copies out of the address bar
// and types here is `linkedin.com/in/jdoe`. Stored as-is it is not an absolute
// URL, the frontend's own `webUrl` refuses it, and the row is permanently
// unlinkable on every surface that reads it — which is exactly what the
// profile-as-a-link work was for. The web client normalizes before sending;
// this is the same rule for every other caller of the endpoint.
//
// Only a value with NO scheme is touched. One that carries `http://` keeps it:
// the caller typed a scheme, and quietly upgrading it would claim a
// certificate nobody has seen.
func profileURLWithScheme(url string) string {
	if url == "" || bareProfileHost.MatchString(url) {
		if url == "" {
			return ""
		}
		return "https://" + url
	}
	return url
}

// bareProfileHost matches a value that opens with a hostname rather than a
// scheme — `linkedin.com/in/jdoe`, `www.example.com`.
var bareProfileHost = regexp.MustCompile(`^[a-zA-Z0-9-]+(\.[a-zA-Z0-9-]+)+(/|$)`)

func trimmedValue(v *string) string {
	if v == nil {
		return ""
	}
	return strings.TrimSpace(*v)
}
