// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// The installation's OWN words, and the seam that sends them.
//
// A registered catalog rather than strings composed at a call site, because
// these messages are the only ones the product sends in its own name. A rep's
// mail is theirs and says what they typed; a confirmation link is the
// installation speaking, and what it says is a fact about the installation that
// must be the same every time and must be versioned when it changes.
//
// comms refuses to stage a template it does not know (ControllerTemplates), so
// this registry is what makes a controller send possible at all — and a body
// assembled anywhere else cannot reach the lane.

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/emailview"
	"github.com/margince/margince/backend/internal/platform/mailcopy"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// Template keys. Each names one thing the installation may say for itself.
const (
	// TemplateRecordConfirmation asks a contact to check what is held about them.
	TemplateRecordConfirmation = "record_confirmation"
	// TemplateConsentConfirmation carries the double-opt-in link.
	TemplateConsentConfirmation = "consent_confirmation"
	// TemplatePrivacyNotice tells a contact what is held about them and asks
	// for nothing.
	//
	// Separate from the record confirmation because the two do different jobs
	// and only one of them is owed: Art. 14 requires telling somebody, not
	// asking them anything. It also reaches contacts the record confirmation
	// cannot — a contact who asked us to stop is still owed their disclosure,
	// and only CategoryPrivacyNotice survives that stop.
	TemplatePrivacyNotice = "privacy_notice"
	// TemplateOptOutAcknowledgement confirms that a refusal of advertising was
	// received. Decree 91/2020/ND-CP Art. 16 owes a Vietnamese recipient one
	// within twenty-four hours.
	//
	// THE ONE TEMPLATE THAT CARRIES NO LINK. It goes to somebody who has just
	// told the product to stop, so anything beyond "we heard you" is the thing
	// they asked not to receive — and a link asking them to do something more
	// would read as a message that did not take the first answer.
	TemplateOptOutAcknowledgement = "optout_acknowledgement"
)

// Rendered is one template resolved into the words that will be sent.
type Rendered struct {
	Key      string
	Version  int
	Subject  string
	Body     string
	HTMLBody string
	Language string
}

// ConfirmationSend is one confirmation message, ready to stage.
type ConfirmationSend struct {
	ContactID ids.ContactID
	// LeadID is set instead of ContactID for a website lead's privacy notice,
	// so the timeline row is filed under the lead the notice went to.
	LeadID    ids.LeadID
	Recipient string
	// Category is what the engine is asked about. It is set from the template
	// rather than by a caller: the whole point of the lane is that these
	// categories cannot be claimed by anybody else.
	Category  commsauthz.Category
	LinkID    ids.UUID
	LinkRef   string
	ExpiresAt time.Time
	DueAt     *time.Time
	MessageID string
	Rendered  Rendered
}

// ConfirmationSender stages a controller message on the caller's transaction.
//
// Compose implements it: the wording is this module's, the delivery row is
// comms', and neither may import the other.
type ConfirmationSender interface {
	QueueConfirmationTx(ctx context.Context, tx pgx.Tx, in ConfirmationSend) (ids.UUID, error)
	// PresentConfirmationTx is the mail QueueConfirmationTx would stage for in,
	// refused as staging would refuse it, staging nothing.
	PresentConfirmationTx(ctx context.Context, tx pgx.Tx, in ConfirmationSend) (PresentedMail, error)
}

// controllerTemplate is one registered wording.
type controllerTemplate struct {
	version  int
	category commsauthz.Category
	// The three parts of the message, each read from the installation's own
	// catalog rather than held as a literal here.
	//
	// The wording moved to platform/mailcopy because these were the last mail
	// the product sent in hard-coded English. An installation whose screens are
	// German asked a stranger, in English, whether it might keep writing to
	// them — and that is the one message where being understood is the point,
	// since an unanswered opt-in withholds the permission.
	//
	// The rendered body still carries the link placeholder exactly once. comms
	// checks that against the material it is staged with and refuses a
	// disagreement, which is what stops a message that was meant to carry a
	// link from going out without one.
	subject func(mailcopy.Copy) string
	intro   func(mailcopy.Copy) string
	closing func(mailcopy.Copy) string
	// linkless says this template carries no one-time link, so the renderer
	// writes no placeholder and comms stages it with no material.
	linkless bool
}

// controllerTemplates is the closed catalog.
//
// The wording states what the link IS before it asks anybody to click it.
// Somebody receiving this did not ask for it, so a bare "confirm your details"
// link from a company they may not remember reads exactly like a phishing mail
// — which is both a deliverability problem and a fair reaction.
var controllerTemplates = map[string]controllerTemplate{
	TemplateRecordConfirmation: {
		version:  3,
		category: commsauthz.CategoryRecordConfirmation,
		subject:  func(w mailcopy.Copy) string { return w.ConfirmRecordSubject },
		intro:    func(w mailcopy.Copy) string { return w.ConfirmRecordBody },
		closing:  func(w mailcopy.Copy) string { return w.ConfirmRecordIgnore },
	},
	TemplateConsentConfirmation: {
		version:  5,
		category: commsauthz.CategoryConsentConfirmation,
		subject:  func(w mailcopy.Copy) string { return w.ConfirmConsentSubject },
		intro:    func(w mailcopy.Copy) string { return w.ConfirmConsentBody },
		closing:  func(w mailcopy.Copy) string { return w.ConfirmConsentIgnore },
	},
	// Version 1, because this wording has never shipped. The two above moved
	// for changes made after they had.
	TemplatePrivacyNotice: {
		version:  3,
		category: commsauthz.CategoryPrivacyNotice,
		subject:  func(w mailcopy.Copy) string { return w.NoticeSubject },
		intro:    func(w mailcopy.Copy) string { return w.NoticeBody },
		closing:  func(w mailcopy.Copy) string { return w.NoticeIgnore },
	},
	TemplateOptOutAcknowledgement: {
		version:  2,
		category: commsauthz.CategoryOptoutConfirmation,
		subject:  func(w mailcopy.Copy) string { return w.OptOutAckSubject },
		intro:    func(w mailcopy.Copy) string { return w.OptOutAckBody },
		closing:  func(w mailcopy.Copy) string { return w.OptOutAckIgnore },
		// The one template with nothing to click. comms refuses a body whose
		// placeholder count disagrees with the material it was staged with, and
		// this is staged with none.
		linkless: true,
	},
}

// linkPlaceholder is where the one-time link goes. It is comms.LinkPlaceholder
// spelled as a literal because this module may not import that one — the value
// is held to agree by TestTheTemplatePlaceholderAgreesWithTheLane.
const linkPlaceholder = "{{confirmation-link}}"

// templateRegistry answers comms' question about what this build knows.
type templateRegistry struct{}

// ControllerTemplateRegistry is what compose hands to the staging call.
//
//nolint:ireturn // returns the comms-side seam by design: the concrete type is unexported and empty
func ControllerTemplateRegistry() interface {
	Registered(key string, version int) bool
} {
	return templateRegistry{}
}

// Registered reports whether this build knows the wording at that version.
func (templateRegistry) Registered(key string, version int) bool {
	t, ok := controllerTemplates[key]
	return ok && (t.version == version || historicalControllerVersion(key, version))
}

// ControllerTemplateVersion is the version of the wording this build sends
// for a template, so a review can be pinned to the words it approved.
func ControllerTemplateVersion(key string) (int, bool) {
	t, ok := controllerTemplates[key]
	return t.version, ok
}

// RenderControllerTemplate resolves one template into the words to be sent.
//
// The link is NOT substituted here and the plaintext never reaches this
// function. The body carries a placeholder, the material rides the vault, and
// the two meet in memory at dispatch — so the link is absent from the delivery
// row, the timeline, the audit entry and the outbox event alike.
func RenderControllerTemplate(key string, expiresAt time.Time, language string) (Rendered, commsauthz.Category, error) {
	return renderTemplateContext(key, expiresAt, language, "")
}

func renderTemplateContext(key string, expiresAt time.Time, language, purpose string) (Rendered, commsauthz.Category, error) {
	t, ok := controllerTemplates[key]
	if !ok {
		return Rendered{}, "", fmt.Errorf("consent: %q is not a registered controller template", key)
	}
	language = string(mailcopy.Carried(language))
	words := mailcopy.For(language)
	var body strings.Builder
	if t.linkless {
		// NOTHING TO CLICK and nothing about a link's life, so the personal-link
		// sentence and the expiry line are both absent rather than rendered
		// about a link that does not exist.
		body.WriteString(t.intro(words) + "\n\n" + t.closing(words) + "\n")
		return Rendered{
			Key: key, Version: t.version, Subject: t.subject(words),
			Body: body.String(), Language: language,
		}, t.category, nil
	}
	subject := t.subject(words)
	body.WriteString(t.intro(words) + "\n\n")
	if key == TemplateConsentConfirmation && purpose != "" {
		subject += ": " + mailcopy.OneLine(purpose)
		body.WriteString(words.ConsentPurposeLabel + ": " + mailcopy.OneLine(purpose) + "\n\n")
	}
	body.WriteString(controllerAction(key, words) + "\n  " + linkPlaceholder + "\n\n" + words.ConfirmPersonal)
	if !expiresAt.IsZero() {
		// Appended to the personal-link sentence rather than substituted into
		// it: the shipped English replaced a phrase inside its own body text,
		// which stops working the moment a language spells that sentence
		// differently.
		fmt.Fprintf(&body, words.ConfirmExpiry, words.Instant(expiresAt.UTC()))
	}
	body.WriteString("\n\n" + t.closing(words) + "\n")
	return Rendered{Key: key, Version: t.version, Subject: subject, Body: body.String(), Language: language}, t.category, nil
}

// ControllerTemplateKeys derives previews and fixture coverage from the registry.
func ControllerTemplateKeys() []string {
	keys := make([]string, 0, len(controllerTemplates))
	for key := range controllerTemplates {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

// RenderControllerPresentation frames the frozen words without changing their version.
func RenderControllerPresentation(rendered Rendered, brand string) string {
	words := mailcopy.For(rendered.Language)
	label := controllerAction(rendered.Key, words)
	before, after, found := strings.Cut(rendered.Body, label+"\n  "+linkPlaceholder)
	if !found {
		var content emailview.Content
		for paragraph := range strings.SplitSeq(rendered.Body, "\n\n") {
			content.Paragraph(strings.TrimSpace(paragraph))
		}
		message := content.Message(rendered.Subject, rendered.Language, words.Preview(rendered.Subject))
		message.Text, message.Brand = rendered.Body, brand
		return emailview.Render(message)
	}
	var content emailview.Content
	for paragraph := range strings.SplitSeq(before, "\n\n") {
		content.Paragraph(strings.TrimSpace(paragraph))
	}
	content.Action(label, linkPlaceholder)
	for paragraph := range strings.SplitSeq(after, "\n\n") {
		content.Paragraph(strings.TrimSpace(paragraph))
	}
	message := content.Message(rendered.Subject, rendered.Language, words.Preview(rendered.Subject))
	message.Text, message.Brand, message.ActionPlaceholder = rendered.Body, brand, linkPlaceholder
	return emailview.Render(message)
}

func historicalControllerVersion(key string, version int) bool {
	switch key {
	case TemplateRecordConfirmation:
		return version == 2
	case TemplateConsentConfirmation:
		return version == 3 || version == 4
	case TemplatePrivacyNotice:
		return version == 1 || version == 2
	case TemplateOptOutAcknowledgement:
		return version == 1
	}
	return false
}
