// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package extension

// System reads for a job tick or a bus delivery. Deals, Authority and the
// directory's Emails have no caller's grants to narrow them, so each answers
// ErrAttendedRead wherever a user gets the result. The newer reads (Companies,
// MailContent, ByRole) take actAs and run as the unit's service member.
// That guards the call, not a unit that stores what it read and serves it
// later. A unit asks Authority().CanRead before it shows a colleague anything,
// and CanRead answers a signed-in human about themselves on any call.

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// ErrAttendedRead is a system read from an invocation that is not a job tick or
// a bus delivery.
var ErrAttendedRead = errors.New("extension: a system read runs on a job tick or a bus delivery only")

// ErrUndeclaredRead is a system read the unit's Extension.SystemReads does not
// list.
var ErrUndeclaredRead = errors.New("extension: this unit does not declare the system read in Extension.SystemReads")

// SystemRead names one group of system reads a unit declares it makes.
type SystemRead string

const (
	// SystemReadDeals admits Deals().Get and Deals().Responsible.
	SystemReadDeals SystemRead = "deals"
	// SystemReadAuthority admits Authority().CanRead and Authority().CanWrite.
	SystemReadAuthority SystemRead = "authority"
	// SystemReadDirectory admits Directory().Emails and Directory().ByRole.
	SystemReadDirectory SystemRead = "directory"
	// SystemReadActivities admits Activities().Mail.
	SystemReadActivities SystemRead = "activities"
	// SystemReadSetAside admits SetAside().Contact and SetAside().Contacts.
	SystemReadSetAside SystemRead = "set_aside"
	// SystemReadCompanies admits Companies().Get and Companies().List.
	SystemReadCompanies SystemRead = "companies"
	// SystemReadMailContent admits MailContent().Get.
	SystemReadMailContent SystemRead = "mail_content"
	// SystemReadMailAttachments admits Activities().Attachments and
	// Activities().OpenAttachment.
	SystemReadMailAttachments SystemRead = "mail_attachments"
)

// Validate refuses a value that names no group of system reads.
func (r SystemRead) Validate() error {
	switch r {
	case SystemReadDeals, SystemReadAuthority, SystemReadDirectory, SystemReadActivities, SystemReadSetAside,
		SystemReadCompanies, SystemReadMailContent, SystemReadMailAttachments:
		return nil
	}
	return fmt.Errorf("extension: %q is not a system read; declare %q, %q, %q, %q, %q, %q, %q or %q",
		string(r), SystemReadDeals, SystemReadAuthority, SystemReadDirectory, SystemReadActivities, SystemReadSetAside,
		SystemReadCompanies, SystemReadMailContent, SystemReadMailAttachments)
}

// MaxResponsible bounds what Deals().Responsible answers and how many users one
// Directory().Emails call takes, so the second always fits what the first gave.
const MaxResponsible = 400

// DealSummary is one deal as a background reader sees it, archived or not.
//
// It is not the contract's Deal. That shape answers per caller (masked_fields,
// writable), and a read with no caller has nobody to answer for. It also names
// the stage by id, where a reader shows its name.
type DealSummary struct {
	ID        string
	Name      string
	StageName string
	// Status is open, won or lost.
	Status string
	// AmountMinor is in minor units of Currency; both are nil on a deal with
	// no amount.
	AmountMinor *int64
	Currency    *string
	// AmountDisplay is the amount as the product prints it in English
	// ("12,500.00 EUR", "120,000 JPY"), so a unit needs no table of each
	// currency's decimals. Empty on a deal with no amount.
	AmountDisplay string
	// ExpectedClose is a calendar date, carried at midnight UTC.
	ExpectedClose *time.Time
	// OwnerID is empty for a deal with no owner.
	OwnerID  UserID
	Archived bool
	// MergedIntoID names the deal this one was merged into, and is empty for a
	// deal that was not. A reader follows it to the deal that carries on.
	MergedIntoID string
}

// Responsible is one colleague a deal names.
type Responsible struct {
	UserID UserID
	// Owner marks the deal's owner; everyone else is assigned to it, alone or
	// through a team.
	Owner bool
}

// Deals reads the installation's deals for a job tick or a bus delivery.
type Deals interface {
	// Get reads one deal. A deal that does not exist, or was erased, answers
	// ErrNotFound; an id that is not a canonical UUID answers ErrInvalid.
	Get(ctx context.Context, dealID string) (DealSummary, error)

	// Responsible answers the colleagues a deal names: its owner, the users
	// assigned to it, and the members of the teams assigned to it. Agents,
	// members who cannot sign in, archived assignments and archived teams are
	// left out. The owner comes first, then the rest by email, and the list
	// stops at MaxResponsible.
	Responsible(ctx context.Context, dealID string) ([]Responsible, error)
}

// Authority answers what one colleague may read, under their own grants.
type Authority interface {
	// CanRead reports whether user may open record and read every one of
	// fields, named as the contract names them (`owner_id`). A mask that
	// lifts only where the user could write counts as withholding.
	//
	// An archived deal answers as a live one; a deal no row holds, false.
	// Only Type "deal" is answered. Another type, an unknown field or a
	// malformed id answers ErrInvalid; an agent seat or a user who cannot sign
	// in, false.
	//
	// On a call a human makes, it answers only about that human. So a route
	// can check its own caller before serving what the unit stored. An agent's
	// or a connector's call, or a question about anyone else, answers
	// ErrAttendedRead.
	CanRead(ctx context.Context, user UserID, record EntityRef, fields []string) (bool, error)

	// CanWrite reports whether user could change record now. It is live and
	// they hold a full seat. Their own grants and the row's sharing let them
	// update it. It answers on the same calls, about the same users, as CanRead.
	CanWrite(ctx context.Context, user UserID, record EntityRef) (bool, error)
}

// Directory reads colleagues' details for a job tick or a bus delivery.
type Directory interface {
	// Emails answers the address of each active human member among users. An
	// agent seat, or a user who does not exist or cannot sign in, is absent. More than
	// MaxResponsible users, or an id that is not a canonical UUID, answers
	// ErrInvalid.
	Emails(ctx context.Context, users []UserID) (map[UserID]string, error)

	// ByRole answers the active human members who hold at least one of roles,
	// named by role key. Agent seats and members who cannot sign in are left
	// out. The ids come in id order and stop at MaxResponsible. More than
	// MaxResponsible roles answers ErrInvalid.
	//
	// It answers as actAs, the unit's service member, and only when that seat
	// may read the roster the way an administrator does. Another seat answers
	// ErrForbidden. A key that names no role adds nobody and is not an error.
	// So the answer does not tell whether the role exists. The core logs the
	// keys asked for. The list is cut at MaxResponsible with no sign of the cut.
	// A unit that asks for a large role must not take the list as complete.
	ByRole(ctx context.Context, actAs UserID, roles []string) ([]UserID, error)
}

// MailSummary is one email on a deal as a background reader sees it.
//
// It is not the contract's Activity, which answers per caller (masked_fields,
// writable). It holds no subject, body, address or attachment, so a unit that
// needs the words reads them as a user with their own rights.
type MailSummary struct {
	ID string
	// Direction is inbound or outbound.
	Direction  string
	OccurredAt time.Time
	// AutomaticKind says what sent an email no human wrote: product, calendar
	// or sending_system. Empty when a human wrote it.
	AutomaticKind string
	DealID        string
}

// MailQuery pages the emails of one deal, newest first.
type MailQuery struct {
	ReadPage
	// IncludeArchived adds the emails that were archived. They are left out
	// unless asked for.
	IncludeArchived bool
}

// MailAttachment is one file that came with an email, as a background reader
// sees it. It holds no bytes; OpenAttachment reads them.
type MailAttachment struct {
	ID     string
	MailID string
	// Filename is the name the sender gave the file.
	Filename string
	// ContentType is the stored media type of the file. OpenAttachment answers
	// the type its bytes show.
	ContentType string
	// ByteSize is 0 when the size was not recorded.
	ByteSize  int64
	CreatedAt time.Time
}

// MailAttachmentFile is one opened attachment with its bytes.
type MailAttachmentFile struct {
	MailAttachment
	// Content is at most MaxInboundFileBytes long.
	Content []byte
}

// MaxAttachmentOpensPerDay is how many attachments one unit may open in one
// workspace per UTC day.
const MaxAttachmentOpensPerDay = 200

// ErrAttachmentCapReached is an OpenAttachment past MaxAttachmentOpensPerDay.
// The count starts again at midnight UTC.
var ErrAttachmentCapReached = errors.New("extension: this unit has opened as many attachments as it may this UTC day; try again on the next")

// Activities reads emails and their attachments for a job tick or a bus
// delivery.
type Activities interface {
	// Mail answers the emails of one deal as actAs may see the deal, under that
	// member's own grants and row scope, the way Authority.CanRead answers. A
	// deal actAs cannot open answers ErrNotFound. It answers the emails of the
	// deal that the deal's team may read. An
	// email private to its participants is left out, and so is one held back
	// from reading or under a restriction of processing. A reply that sits only in a private mailbox is therefore
	// invisible here. A unit that decides "no reply yet" from this read is
	// reminding a user to check. It must not act on it alone, such as by
	// sending a message.
	//
	// A deal that does not exist answers ErrNotFound; an id that is not a
	// canonical UUID, ErrInvalid. A page holds at most MaxReadPage emails.
	Mail(ctx context.Context, actAs UserID, dealID string, q MailQuery) (Page[MailSummary], error)

	// Attachments lists the files that came with one email, oldest first, at
	// most MaxReadPage. It answers as actAs, the unit's service member, under
	// the rule MailContents.Get applies. A mail Get would refuse answers
	// ErrNotFound. A file that was archived or whose bytes were withheld is
	// left out. An id that is not a canonical UUID answers ErrInvalid. Each
	// call writes an export row to the audit log.
	Attachments(ctx context.Context, actAs UserID, mailID string) ([]MailAttachment, error)

	// OpenAttachment reads one listed file with its bytes, under the same rule.
	// Every way the file or its mail is hidden answers ErrNotFound, so an id
	// cannot be used to probe. A file larger than MaxInboundFileBytes answers
	// ErrInvalid. Each open writes an export row to the audit log. Past
	// MaxAttachmentOpensPerDay opens it answers ErrAttachmentCapReached.
	OpenAttachment(ctx context.Context, actAs UserID, attachmentID string) (MailAttachmentFile, error)
}

// SetAsideKind is how one user stands towards one contact they were nudged to
// follow up.
type SetAsideKind string

const (
	// SetAsideNone is: not set aside, or the dismissal has run out.
	SetAsideNone SetAsideKind = "none"
	// SetAsideUntil is: set aside until SetAsideState.Until.
	SetAsideUntil SetAsideKind = "until"
	// SetAsidePermanent is: set aside for good.
	SetAsidePermanent SetAsideKind = "permanent"
)

// SetAsideState is one user's choice about one contact, and nothing else
// about the contact.
type SetAsideState struct {
	Kind SetAsideKind
	// Until is set only when Kind is SetAsideUntil.
	Until *time.Time
}

// SetAside reads whether a user has set a lapsed contact aside, for a job tick
// or a bus delivery. It reads only: a user changes the choice through the
// product's own routes.
type SetAside interface {
	// Contact answers one user's choice about one contact right now. It is
	// the user's own choice and never a colleague's. It answers only for an
	// active user who connected the unit, and any other user answers ErrForbidden. A contact or a user
	// that does not exist answers none, so an id cannot be used to probe. An id
	// that is not a canonical UUID answers ErrInvalid.
	Contact(ctx context.Context, user UserID, contact string) (SetAsideState, error)

	// Contacts is Contact for up to MaxResponsible contacts in one call, so a
	// nightly job does not ask once per contact. Every contact asked about is in
	// the answer, keyed by the id as given. More than MaxResponsible answers
	// ErrInvalid.
	Contacts(ctx context.Context, user UserID, contacts []string) (map[string]SetAsideState, error)
}

// CompanySummary is one company as a background reader sees it, archived or
// not. Like DealSummary it is not the contract's Company, which answers per
// caller. It holds the facts a unit needs to push or match a company, and no
// contact, note or custom field.
type CompanySummary struct {
	ID           string
	Name         string
	LegalName    string
	Status       string
	OwnerID      UserID
	SourceSystem string
	SourceID     string
	UpdatedAt    time.Time
	Archived     bool
}

// CompanyPageQuery pages the companies of the installation by id.
type CompanyPageQuery struct {
	ReadPage
	// IncludeArchived adds the companies that were archived.
	IncludeArchived bool
}

// Companies reads the installation's companies for a job tick or a bus
// delivery. It answers as actAs, the unit's service member, and shows only
// what that seat's own grants, row scope and field masks show in the app.
// Any other member, or none, answers ErrForbidden or ErrInvalid.
type Companies interface {
	// Get reads one company. A company that does not exist, was erased or is
	// outside the seat's scope answers ErrNotFound. An id that is not a
	// canonical UUID answers ErrInvalid. A field the seat's mask withholds is
	// empty.
	Get(ctx context.Context, actAs UserID, companyID string) (CompanySummary, error)

	// List answers a page of the companies the seat may open, in the order the
	// product lists them. A page holds at most MaxReadPage companies.
	List(ctx context.Context, actAs UserID, q CompanyPageQuery) (Page[CompanySummary], error)
}

// MailContentState says whether the words of a mail are in a MailContent.
type MailContentState string

const (
	// MailContentAvailable is: the subject and body are given.
	MailContentAvailable MailContentState = "available"
	// MailContentWithheld is: a redaction removed the subject or body, and
	// neither is given.
	MailContentWithheld MailContentState = "withheld"
)

// MailContent is one email as a background reader sees it, with its words.
// Mail's MailSummary has none; this does, for the units that must read what a
// mail says.
type MailContent struct {
	ID      string
	Subject string
	Body    string
	// Audience is workspace. A mail kept to its participants or to a chosen few
	// is never read here.
	Audience     string
	ContentState MailContentState
	// BodyTruncated is true when Body is the first MaxMailBody characters of a
	// longer text.
	BodyTruncated bool
	// CaptureLabel is commitment, meeting or noise; empty when none was set.
	CaptureLabel string
	// BulkMail is true when the mail was attested as bulk mail.
	BulkMail bool
	// SourceID is the id the mail has in the system it came from; empty when it
	// has none.
	SourceID   string
	OccurredAt time.Time
}

// MaxMailBody is the most characters of a mail's body that MailContents.Get
// gives.
const MaxMailBody = 8000

// MailContents reads one email's words for a job tick or a bus delivery. It
// answers as actAs, the unit's service member, under the same content rule the
// product applies to that seat. A unit that keeps what it read asks
// Authority().CanRead first. It asks before it shows the mail to a colleague.
type MailContents interface {
	// Get reads one mail by id. No such mail answers ErrNotFound. So does a mail
	// that is not an email or one the seat may not read. So does one private to
	// its participants or a chosen few. So do an archived one, one held back,
	// and one under a restriction of processing. An id cannot be used to probe. An id
	// that is not a canonical UUID answers ErrInvalid. A mail whose subject or
	// body was redacted answers ContentState withheld with neither. The body is
	// cut at MaxMailBody characters.
	Get(ctx context.Context, actAs UserID, mailID string) (MailContent, error)
}
