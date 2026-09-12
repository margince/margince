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
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

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
)

// Rendered is one template resolved into the words that will be sent.
type Rendered struct {
	Key     string
	Version int
	Subject string
	Body    string
}

// ConfirmationSend is one confirmation message, ready to stage.
type ConfirmationSend struct {
	ContactID ids.ContactID
	Recipient string
	// Category is what the engine is asked about. It is set from the template
	// rather than by a caller: the whole point of the lane is that these
	// categories cannot be claimed by anybody else.
	Category  commsauthz.Category
	LinkID    ids.UUID
	LinkRef   string
	ExpiresAt time.Time
	MessageID string
	Rendered  Rendered
}

// ConfirmationSender stages a controller message on the caller's transaction.
//
// Compose implements it: the wording is this module's, the delivery row is
// comms', and neither may import the other.
type ConfirmationSender interface {
	QueueConfirmationTx(ctx context.Context, tx pgx.Tx, in ConfirmationSend) (ids.UUID, error)
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
}

// controllerTemplates is the closed catalog.
//
// The wording states what the link IS before it asks anybody to click it.
// Somebody receiving this did not ask for it, so a bare "confirm your details"
// link from a company they may not remember reads exactly like a phishing mail
// — which is both a deliverability problem and a fair reaction.
var controllerTemplates = map[string]controllerTemplate{
	TemplateRecordConfirmation: {
		version:  2,
		category: commsauthz.CategoryRecordConfirmation,
		subject:  func(w mailcopy.Copy) string { return w.ConfirmRecordSubject },
		intro:    func(w mailcopy.Copy) string { return w.ConfirmRecordBody },
		closing:  func(w mailcopy.Copy) string { return w.ConfirmRecordIgnore },
	},
	TemplateConsentConfirmation: {
		version:  2,
		category: commsauthz.CategoryConsentConfirmation,
		subject:  func(w mailcopy.Copy) string { return w.ConfirmConsentSubject },
		intro:    func(w mailcopy.Copy) string { return w.ConfirmConsentBody },
		closing:  func(w mailcopy.Copy) string { return w.ConfirmConsentIgnore },
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
	return ok && t.version == version
}

// RenderControllerTemplate resolves one template into the words to be sent.
//
// The link is NOT substituted here and the plaintext never reaches this
// function. The body carries a placeholder, the material rides the vault, and
// the two meet in memory at dispatch — so the link is absent from the delivery
// row, the timeline, the audit entry and the outbox event alike.
func RenderControllerTemplate(key string, expiresAt time.Time, language string) (Rendered, commsauthz.Category, error) {
	t, ok := controllerTemplates[key]
	if !ok {
		return Rendered{}, "", fmt.Errorf("consent: %q is not a registered controller template", key)
	}
	words := mailcopy.For(language)
	var body strings.Builder
	body.WriteString(t.intro(words) + "\n\n  " + linkPlaceholder + "\n\n" + words.ConfirmPersonal)
	if !expiresAt.IsZero() {
		// Appended to the personal-link sentence rather than substituted into
		// it: the shipped English replaced a phrase inside its own body text,
		// which stops working the moment a language spells that sentence
		// differently.
		fmt.Fprintf(&body, words.ConfirmExpiry, expiresAt.Format(mailcopy.DateLayout))
	}
	body.WriteString("\n\n" + t.closing(words) + "\n")
	return Rendered{Key: key, Version: t.version, Subject: t.subject(words), Body: body.String()}, t.category, nil
}
