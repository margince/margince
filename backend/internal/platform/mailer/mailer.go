// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package mailer is the transactional-email transport (A74/ADR-0056):
// ONE outbound channel for product-originated mail, configured by the
// operator. Its first consumer is password-reset delivery (A107); the
// consent module's marketing lanes are a separate, gated concern and do
// not ride this seam.
package mailer

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/margince/margince/backend/internal/platform/emailview"
	"github.com/margince/margince/backend/internal/platform/outbound"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// Mailer sends one plain-text transactional message. Implementations
// must never log the body: reset mail carries a live credential.
type Mailer interface {
	Send(ctx context.Context, to, subject, textBody string) error
}

// IdentifiedSender sends one message under a Message-ID the caller chose.
//
// A delivery report quotes the returned message's own Message-ID. A bounce is
// matched back to its delivery row by that key alone, so a relay left to mint
// the identity makes every bounce unmatchable.
type IdentifiedSender interface {
	SendIdentified(ctx context.Context, to, subject, textBody, messageID string) error
}

// SMTP is the operator-relay transport. STARTTLS is negotiated when the
// relay offers it; credentials are optional (an authenticated submission
// port vs. a local relay).
type SMTP struct {
	Host        string
	Port        int
	Username    string
	Password    string
	FromAddress string
	// Now stamps the Date header; the wall clock when nil.
	Now func() time.Time
	// Letterhead is who product mail comes from: the installation's own
	// company, with its logo when one is stored. Send and SendBranded refuse
	// without one, so a relay built field by field fails at its first send
	// rather than mailing as nobody.
	Letterhead LetterheadSource
}

// LetterheadSource reads the letterhead product mail is drawn under, in the
// mail's language, with the inline images its markup refers to. It returns
// no error: a source that cannot read the logo in time answers with the name
// alone, because a reset mail must not wait on a slow object store.
type LetterheadSource interface {
	Letterhead(ctx context.Context, language string) (emailview.Letterhead, []connector.InlineImage)
}

// BrandedSender sends product mail under the installation's letterhead.
type BrandedSender interface {
	SendBranded(ctx context.Context, to, subject string, message emailview.Message) error
}

// ErrNoLetterhead is the refusal of a relay that was built without a
// letterhead source.
var ErrNoLetterhead = errors.New("mailer: no letterhead source is wired")

// Send submits the message through the configured relay. TLS is
// REQUIRED: a reset mail carries a live credential, so a relay that
// offers no STARTTLS refuses the send instead of downgrading to
// cleartext — except a loopback relay (a local forwarder such as a
// docker mailhog or a host postfix), where the wire never leaves the
// machine.
func (s SMTP) Send(ctx context.Context, to, subject, textBody string) error {
	return s.SendBranded(ctx, to, subject, emailview.Message{Title: subject, Text: textBody})
}

// SendBranded renders message under the installation's letterhead and sends
// it with the letterhead's logo as an inline part.
func (s SMTP) SendBranded(ctx context.Context, to, subject string, message emailview.Message) error {
	if s.Letterhead == nil {
		return ErrNoLetterhead
	}
	head, inline := s.Letterhead.Letterhead(ctx, message.Language)
	return s.send(ctx, to, subject, textLetterhead(head)+message.Text, emailview.Render(message, head), "", inline)
}

// textLetterhead heads the plain-text alternative the way the logo heads the
// markup: the company's name, then the attribution, then a blank line. A
// reader whose client shows text only still sees who the mail comes from.
// The message's own text is left as the builder wrote it.
func textLetterhead(head emailview.Letterhead) string {
	var lines []string
	for _, line := range []string{head.Name, head.Attribution} {
		if line = strings.TrimSpace(line); line != "" {
			lines = append(lines, line)
		}
	}
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n") + "\n\n"
}

// SendIdentified is Send with the Message-ID header set to messageID. An
// empty messageID gets one minted on the sender's domain.
//
// Once the relay accepts the DATA terminator the message is sent, so a QUIT
// that fails after it is logged and the send still succeeds. Reporting it
// would make the dispatcher retry and mail a second copy.
func (s SMTP) SendIdentified(ctx context.Context, to, subject, textBody, messageID string) error {
	return s.SendAlternative(ctx, to, subject, textBody, "", messageID)
}

// SendAlternative submits readable text and HTML under one message identity.
func (s SMTP) SendAlternative(ctx context.Context, to, subject, textBody, htmlBody, messageID string) error {
	return s.send(ctx, to, subject, textBody, htmlBody, messageID, nil)
}

// SendRelated is SendAlternative with the inline images the markup shows by
// content id.
func (s SMTP) SendRelated(ctx context.Context, to, subject, textBody, htmlBody, messageID string, inline []connector.InlineImage) error {
	return s.send(ctx, to, subject, textBody, htmlBody, messageID, inline)
}

func (s SMTP) send(ctx context.Context, to, subject, textBody, htmlBody, messageID string, inline []connector.InlineImage) error {
	if strings.ContainsAny(to, "\r\n") || strings.ContainsAny(subject, "\r\n") {
		return errors.New("mailer: recipient and subject must be single-line (header injection)")
	}
	if messageID != "" && !connector.ValidMessageID(messageID) {
		return connector.ErrInvalidMessageID
	}
	msg, err := s.composeAlternative(to, subject, textBody, htmlBody, messageID, inline)
	if err != nil {
		return err
	}
	return s.submission().Submit(ctx, s.FromAddress, []string{to}, msg)
}

// submission is the operator relay as one SMTP exchange. Its credentials are
// optional: an authenticated submission port, or a local relay that takes
// mail without a login.
func (s SMTP) submission() Submission {
	return Submission{Host: s.Host, Port: s.Port, Username: s.Username, Password: s.Password, HelloName: s.helloName()}
}

// helloName is what this installation announces to the relay.
//
// The sender's own domain, which is the honest answer and the one the relay is
// about to see in MAIL FROM anyway — so it needs no configuration of its own
// and differs per installation, which is the whole point. The product token
// stands in only when the configured sender carries no domain to announce.
func (s SMTP) helloName() string {
	// The LAST `@`, because a quoted local part may hold one — `"a@b"@x.test`
	// is a legal address whose domain is `x.test`, and cutting at the first
	// would announce `b"@x.test`, which is not a name any relay will take.
	if at := strings.LastIndex(s.FromAddress, "@"); at >= 0 {
		if domain := strings.TrimSpace(s.FromAddress[at+1:]); domain != "" {
			return domain
		}
	}
	return outbound.MailProduct
}

// AlternativeSender preserves the already-authorized HTML and text alternatives.
type AlternativeSender interface {
	SendAlternative(ctx context.Context, to, subject, textBody, htmlBody, messageID string) error
}

// RelatedSender is an AlternativeSender that also attaches the images the
// markup shows inline.
type RelatedSender interface {
	SendRelated(ctx context.Context, to, subject, textBody, htmlBody, messageID string, inline []connector.InlineImage) error
}
