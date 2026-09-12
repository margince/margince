// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// Putting a confirm link on the durable send lane.
//
// Its own file beside confirmtoken.go, which mints the link, because this is
// the half that leaves the module: the wording is here, the delivery row is
// comms', and compose joins them. Keeping it separate is what lets confirmtoken.go
// stay about the credential and this file about the message.
//
// WHY A LANE AND NOT AN SMTP CALL. The direct call this replaces returned before
// any mailbox saw the message, so "delivered" meant "the relay did not refuse
// it" — and a later bounce could not travel back to correct that. Worse, the
// message existed nowhere: no delivery row, no authorization decision, no
// timeline entry, so it appeared in no subject-access export and no erasure
// reached it. A contact asking "what have you sent me" was answered with silence
// about the one message the installation had written entirely on its own.

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/mailcopy"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ConfirmLinkVault holds the plaintext confirm link between staging and
// dispatch, so the delivery row, the timeline, the audit entry and the outbox
// event carry a placeholder instead of a live credential.
type ConfirmLinkVault interface {
	Put(ctx context.Context, secret string) (string, error)
}

// WithConfirmationLane wires the durable lane the installation's own mail rides.
//
// Both halves or neither: a sender with no vault could stage a message whose
// link nothing can supply, which reaches somebody as a mail with a placeholder
// where the link should be.
func (s *Store) WithConfirmationLane(sender ConfirmationSender, vault ConfirmLinkVault, base string) *Store {
	// ALL THREE or none. A lane with no base URL would mail a link built on an
	// empty origin — an unusable URL that still spent the one token the contact
	// was issued, and superseded whatever earlier link they still had.
	if sender == nil || vault == nil || base == "" {
		return s
	}
	s.publicBaseURL = strings.TrimRight(base, "/")
	s.confirmSender = sender
	s.vault = vault
	return s
}

// confirmMailInput is one confirm link, ready to be turned into a message.
type confirmMailInput struct {
	contactID  ids.ContactID
	recipient  string
	kind       string
	tokenRowID ids.UUID
	link       string
	expiresAt  time.Time
}

// stageConfirmMail renders the registered wording and stages it, on the
// caller's transaction.
//
// It reports FALSE rather than failing when no lane is wired. The token has
// already been minted and audited by the time this runs, and answering with an
// error would roll that back and invite a retry that mints another — so an
// installation with no relay gets a link it can see was not sent, which is what
// the screen tells an operator to fix.
func (s *Store) stageConfirmMail(ctx context.Context, tx pgx.Tx, in confirmMailInput) (bool, error) {
	if s.confirmSender == nil || s.vault == nil {
		return false, nil
	}
	rendered, category, err := RenderControllerTemplate(
		templateForLinkKind(in.kind), in.expiresAt, s.mailLanguage(ctx, tx))
	if err != nil {
		return false, err
	}
	// The plaintext goes to the vault, never onto the row. What the delivery
	// carries is a placeholder and a reference; the two meet in memory at
	// dispatch, so the link is absent from the delivery row, the timeline body,
	// the audit payload and the outbox event alike.
	ref, err := s.vault.Put(ctx, in.link)
	if err != nil {
		return false, fmt.Errorf("consent: sealing the one-time confirm link: %w", err)
	}
	if _, err := s.confirmSender.QueueConfirmationTx(ctx, tx, ConfirmationSend{
		ContactID: in.contactID,
		Recipient: in.recipient,
		Category:  category,
		LinkID:    in.tokenRowID,
		LinkRef:   ref,
		ExpiresAt: in.expiresAt,
		MessageID: confirmMessageID(in.tokenRowID),
		Rendered:  rendered,
	}); err != nil {
		return false, err
	}
	return true, nil
}

// templateForLinkKind maps a token kind to the wording that carries it.
//
// Total over the two link kinds, and it returns the record-confirmation wording
// for anything else rather than an error: every caller passes one of the two
// Link* constants, and a wrong template would be caught at staging anyway —
// comms refuses one whose placeholder count disagrees with the material it was
// staged with.
func templateForLinkKind(kind string) string {
	if kind == LinkConsentConfirmation {
		return TemplateConsentConfirmation
	}
	return TemplateRecordConfirmation
}

// confirmMessageID is the RFC822 identity this message claims.
//
// Derived from the TOKEN row rather than minted fresh, so it is stable for the
// life of the link: the delivery's message-id uniqueness index then refuses a
// second staging of the same link outright, which is the property that makes a
// retried request incapable of mailing somebody two live links.
func confirmMessageID(tokenRowID ids.UUID) string {
	return "confirm-" + strings.ReplaceAll(tokenRowID.String(), "-", "") + "@margince.invalid"
}

// confirmRoute is the SPA route a confirm link points at. The token rides the
// FRAGMENT for the reason the deal-room invitation states at length: a browser
// does not put a fragment on the wire, so it stays out of access logs and out of
// the Referer a click sends onward. Containment rather than a guarantee — what
// bounds the exposure is that the token is single-use and expires.
const confirmRoute = "/#/confirm/"

// confirmLink puts the token in the URL's FRAGMENT, never its path.
func (s *Store) confirmLink(token string) string {
	return s.publicBaseURL + confirmRoute + token
}

// canSendConfirm reports whether this installation can put a confirm link on the
// lane at all.
func (s *Store) canSendConfirm() bool {
	return s.confirmSender != nil && s.vault != nil && s.publicBaseURL != ""
}

// WithConfirmationLane injects the durable lane the installation's own mail
// rides: what stages the message, what seals the one-time link, and the
// canonical origin the link is built on.
//
// Compose supplies all three. The stager lives there because it joins comms and
// this module, and neither may import the other.
func (h Handlers) WithConfirmationLane(sender ConfirmationSender, vault ConfirmLinkVault, base string) Handlers {
	h.store = h.store.WithConfirmationLane(sender, vault, base)
	return h
}

// mailLanguage is the language this installation's controller mail is written
// in, or the fallback when nothing is wired or nothing is set.
//
// An unwired reader answers the FALLBACK rather than failing. Refusing to send
// a confirm link because a settings read failed trades the whole message for a
// formatting preference, and for the consent link that is worse than it sounds:
// the permission stays withheld until the contact answers a mail that never
// arrived. mailcopy.For treats an unknown answer the same way, so the two
// agree without either having to know the other's list.
func (s *Store) mailLanguage(ctx context.Context, tx pgx.Tx) string {
	if s.language == nil {
		return string(mailcopy.Fallback)
	}
	language, err := s.language.MailLanguageTx(ctx, tx)
	if err != nil {
		slog.WarnContext(ctx, "the installation's mail language could not be read; sending in the fallback",
			"fallback", mailcopy.Fallback, "cause", err)
		return string(mailcopy.Fallback)
	}
	return language
}
