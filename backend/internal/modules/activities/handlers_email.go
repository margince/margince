// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/convstate"
	"github.com/margince/margince/backend/internal/shared/kernel/draftfloor"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// ConsentGate is the outbound suppression seam (B-EP07.12): the
// consent module implements it, the composition root injects it. A
// send path constructed WITHOUT one fails closed — absence of the gate
// must never read as consent.
//
// ONE gate serves both transports, with two spellings of the same question,
// because the alternative is two default-deny checks — and the one that stopped
// applying would look exactly like the one that passes. Mail asks in addresses
// because that is what a mail surface holds; a channel recipient has no address,
// so the channel reply asks in recipients (connector.Recipient), which is the
// union of the two vocabularies.
type ConsentGate interface {
	RequireGrantedForEmails(ctx context.Context, recipients []string, purposeKey string) error
	RequireGrantedForRecipients(ctx context.Context, recipients []connector.Recipient, purposeKey string) error
}

// WithConsent returns handlers whose send path consults the given
// authority. Compose calls this; the zero Handlers value keeps sends
// suppressed.
func (h Handlers) WithConsent(gate ConsentGate) Handlers {
	h.consent = gate
	return h
}

// WithDelivery returns handlers whose send path records an accepted message
// for transmission. Compose calls this; the zero Handlers value refuses to
// send rather than log an activity claiming a message went out.
func (h Handlers) WithDelivery(stager DeliveryStager) Handlers {
	h.delivery = stager
	return h
}

// WithSendAuthority returns handlers whose send paths pre-flight the credential
// they are about to transmit through, so a sender with no send-capable mailbox —
// or a workspace with no bot bound — is refused with an actionable message
// instead of accepting a message that can only park.
func (h Handlers) WithSendAuthority(authority SendAuthority) Handlers {
	h.store = h.store.WithSendAuthority(authority)
	return h
}

// WithRecipientDirectory returns handlers whose account-started sends resolve
// every typed address to a person the sender can read, so a rep is told which
// address is not on file instead of mailing someone the record cannot name.
func (h Handlers) WithRecipientDirectory(dir RecipientDirectory) Handlers {
	h.store = h.store.WithRecipientDirectory(dir)
	return h
}

// GetReplyRecipient answers who a reply to this message would go to, without
// drafting one. A composer opening on a thread shows the recipient straight
// away rather than after a model call that takes tens of seconds.
//
// The name and the address come from two resolvers because they answer two
// questions. The greeting names whoever the message was WITH; the address must
// be a counterparty and never one of our own, so on our own outbound mail the
// sender we greet is us and the address we answer is the addressee's.
// ReplyAddressFor owns that distinction, and this asks it rather than keeping
// a second opinion about who may be written to.
func (h Handlers) GetReplyRecipient(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	ctx := r.Context()
	anchor := ids.From[ids.ActivityKind](ids.UUID(id))
	recipient, err := h.store.ReplyRecipientFor(ctx, anchor)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	address, err := h.replyAddress(ctx, anchor)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, crmcontracts.ReplyRecipient{
		FullName:  recipient.FullName,
		FirstName: recipient.FirstName,
		Address:   address,
	})
}

// replyAddress is the counterparty address for this anchor, empty when the
// thread carries none to answer.
//
// A thread with nobody outside the company to write to is an ANSWER, not a
// fault: ReplyAddressFor refuses it as NoReplyAddressError, and a composer
// that shows an empty field there is telling the truth. Every other failure
// is the caller's to see.
func (h Handlers) replyAddress(ctx context.Context, anchor ids.ActivityID) (string, error) {
	if h.colleagues == nil {
		return "", nil
	}
	covers, err := h.colleagues.Covers(ctx)
	if err != nil {
		return "", err
	}
	address, err := h.store.ReplyAddressFor(ctx, anchor, covers)
	var none *NoReplyAddressError
	if errors.As(err, &none) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return address, nil
}

// replyAddresses names where a draft's reply is sent, or nil.
//
// Nil on any failure: drafting is the caller's request and addressing is a
// convenience on top of it, so a draft whose recipient cannot be resolved is
// still served with the field left for the reader to fill. The reason is
// logged rather than swallowed — a permission failure and a thread with no
// counterparty produce the same empty field, and only the log tells them
// apart afterwards.
func (h Handlers) replyAddresses(ctx context.Context, anchor ids.ActivityID) *[]openapi_types.Email {
	address, err := h.replyAddress(ctx, anchor)
	if err != nil {
		slog.WarnContext(ctx, "reply address unavailable; drafting without a recipient", "err", err)
		return nil
	}
	if address == "" {
		return nil
	}
	return &[]openapi_types.Email{openapi_types.Email(address)}
}

// DeterministicEmailDraft is the shared no-model floor for every drafting
// transport. Compose calls it when the model lane is absent or unavailable,
// so HTTP, MCP, and automation cannot drift into different fallback text.
//
// The prose skeleton comes from the shared floor table rather than from string
// literals here, so this path writes the same German a model-backed draft would
// (DRAFT-AC-E-1). answering is the correspondence the draft replies into: its
// text decides the language, and its shape decides whether there is a thread to
// reply to at all.
func DeterministicEmailDraft(answering DraftContext, intent string) (subject, body string) {
	lang := answering.language()
	band := answering.Band

	topic := strings.TrimSpace(answering.Topic)
	subject = draftfloor.Subject(lang, band, topic, answering.Threaded)

	phrases := draftfloor.For(lang, band)
	lines := []string{draftfloor.Greeting(lang, band, answering.Recipient), ""}
	if phrases.Opener != "" {
		lines = append(lines, phrases.Opener, "")
	}
	// The topic is the only substance this floor has, and dropping it would
	// leave a draft that says nothing about what it is answering.
	if topic != "" {
		lines = append(lines, draftfloor.Fill(draftfloor.SubstanceFor(lang).Thread, topic), "")
	}
	if intent := strings.TrimSpace(intent); intent != "" {
		lines = append(lines, intent, "")
	}
	return subject, strings.Join(append(lines, phrases.Ask), "\n")
}

// DraftContext is what the floor knows about the correspondence it is writing
// into. Everything is optional: the zero value describes a first message to
// somebody with no history, which is the honest reading of "we were told
// nothing".
type DraftContext struct {
	// Topic is what the message is about: the subject of the thread being
	// answered, or a deal name the caller chose. Empty when nothing is known.
	Topic string
	// Recipient is who the draft is addressed to, by name. Empty opens the
	// message without a name, which is right when nobody is known - greeting
	// whoever is nearest is how a draft ends up addressed to its own author.
	Recipient string
	// Threaded says the topic is the subject of a real INBOUND MAIL thread
	// rather than a name the caller picked. Only that earns the reply prefix:
	// "Re:" on a deal name, or on a meeting somebody titled "Quarterly
	// review", claims a message that was never written to us.
	Threaded bool
	// Body is the text of the message being answered, used to detect the
	// language of the correspondence. Empty falls back to the topic.
	Body string
	// Band is where the correspondence stands. The zero value is BandNone.
	Band convstate.Band
}

// IsMailThread reports whether an activity is an inbound mail thread, which is
// the only thing a reply prefix may be built on.
//
// Spelled once rather than at each call site: three callers derive the same
// flag, and a fourth that guessed would put "Re:" on a meeting title. Kind and
// direction both, because an email WE sent is not a message to reply to either.
func IsMailThread(kind crmcontracts.ActivityKind, direction *crmcontracts.ActivityDirection) bool {
	return kind == crmcontracts.ActivityKindEmail &&
		direction != nil && *direction == crmcontracts.ActivityDirectionInbound
}

// language resolves the correspondence language from whatever text there is,
// preferring the body because a subject line rarely carries enough words to
// clear the detector's floor.
func (c DraftContext) language() textlang.Lang {
	if lang := textlang.Detect(c.Body); lang != textlang.Unknown {
		return lang
	}
	if lang := textlang.Detect(c.Topic); lang != textlang.Unknown {
		return lang
	}
	return draftfloor.DefaultLang
}

// SendAccountEmail starts a NEW conversation from a record rather than
// answering one. It differs from SendEmail in exactly two places — the origin
// it builds and the links that origin carries — and shares the send itself,
// so the consent gate, deliverability and the staging transaction cannot
// drift between the two surfaces (ADR-0087 §1).
func (h Handlers) SendAccountEmail(w http.ResponseWriter, r *http.Request, _ crmcontracts.SendAccountEmailParams) {
	var req crmcontracts.SendAccountEmailRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	links := linkInputsOf(&req.Links)
	if len(links) == 0 {
		// A message filed under nothing is one nobody finds again, which is
		// the gap this operation exists to close. The contract says minItems,
		// and nothing in this stack validates a request against the schema.
		writeStoreErr(w, r, &RequiredFieldError{Field: fieldLinks})
		return
	}

	sched, err := scheduleFrom(req.ScheduledAt, req.ScheduledTz)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	in, err := accountSendInput(req)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	out, err := h.store.SendOrSchedule(r.Context(), FromAccount(links), in,
		sched, h.consent, h.delivery, h.timer)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	writeSendOutcome(w, out)
}

// writeSendOutcome answers a send with what actually happened: the activity
// when it went now, the scheduled record when it will go later.
//
// Spelled once so both surfaces answer a deferred send identically — a 201
// on one and a 202 on the other would make "did it send?" depend on which
// button the rep pressed.
func writeSendOutcome(w http.ResponseWriter, out SendOutcome) {
	if out.Scheduled != nil {
		// 201: a new scheduled-send resource exists. Nothing is on the
		// timeline and nothing has reached a provider.
		httperr.WriteJSON(w, http.StatusCreated, scheduledSendResponse(*out.Scheduled))
		return
	}
	// 202: accepted for delivery, the activity is the durable fact.
	httperr.WriteJSON(w, http.StatusAccepted, out.Activity)
}

// sendInputFrom builds the send's input from the fields both mail surfaces
// carry. One spelling, because the merged consent list is a rule rather than
// a convenience: consent is owed to every addressee, so Recipients is To+Cc
// and Cc is a subset of it. A second hand-rolled copy would eventually merge
// one of them differently and mail somebody the gate never asked about.
func sendInputFrom(to []openapi_types.Email, cc, bcc *[]openapi_types.Email, subject, body string, htmlBody *string, attachments *[]openapi_types.UUID, purpose string, draftRef *string) SendEmailInput {
	var ccAddresses []string
	if cc != nil {
		ccAddresses = make([]string, 0, len(*cc))
		for _, addr := range *cc {
			ccAddresses = append(ccAddresses, string(addr))
		}
	}
	recipients := make([]string, 0, len(to)+len(ccAddresses))
	for _, addr := range to {
		recipients = append(recipients, string(addr))
	}
	bccAddresses := addressesOf(bcc)
	// Every addressee joins the merged list. A blind copy is blind to the
	// RECIPIENTS and never to the consent gate: they receive the message, so
	// consent is owed to them exactly as it is to a To or a Cc.
	recipients = append(recipients, ccAddresses...)
	recipients = append(recipients, bccAddresses...)

	ref := ""
	if draftRef != nil {
		ref = *draftRef
	}
	html := ""
	if htmlBody != nil {
		html = *htmlBody
	}
	return SendEmailInput{
		Recipients:     recipients,
		Cc:             ccAddresses,
		Bcc:            bccAddresses,
		Subject:        subject,
		Body:           body,
		HTMLBody:       html,
		AttachmentIDs:  attachmentIDsFrom(attachments),
		ConsentPurpose: purpose,
		DraftRef:       ref,
	}
}

// SendEmail answers an existing conversation: the activity in the path is the
// anchor whose threading chain the reply continues and whose record links it
// inherits. Its account-started twin above shares everything after the origin.
func (h Handlers) SendEmail(w http.ResponseWriter, r *http.Request, id crmcontracts.Id, _ crmcontracts.SendEmailParams) {
	var req crmcontracts.SendEmailRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	// Deliverability — the RFC 8058 header and the visible footer — is
	// derived by the store, on the message, where the MCP send tool reaches
	// it too. It belongs on the mail, not on this response to the API
	// caller, who is not the recipient and has nothing to unsubscribe from.
	sched, err := scheduleFrom(req.ScheduledAt, req.ScheduledTz)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	// The anchor's own links, plus whatever the caller named beyond them. A
	// reply with none of its own is unchanged, which is every reply that was
	// sent before this field existed.
	origin := FromActivity(pathID[ids.ActivityKind](id)).AlsoFiledUnder(linkInputsOf(req.AlsoLinks))
	in, err := replySendInput(req)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	out, err := h.store.SendOrSchedule(r.Context(), origin, in,
		sched, h.consent, h.delivery, h.timer)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	writeSendOutcome(w, out)
}

// addressesOf flattens an optional address list, which both cc and bcc are.
func addressesOf(list *[]openapi_types.Email) []string {
	if list == nil {
		return nil
	}
	out := make([]string, 0, len(*list))
	for _, addr := range *list {
		out = append(out, string(addr))
	}
	return out
}

// linkInputsOf converts the contract's link list to the store's, and answers
// nothing for a list that was never sent.
//
// One converter for both send paths: the account-started list the caller must
// supply, and the reply's optional additions. They were separate spellings of
// the same three lines, and a difference between them would be a difference in
// what a record link MEANS on two operations that write the same column.
func linkInputsOf(list *[]crmcontracts.ActivityLinkInput) []ActivityLinkInput {
	if list == nil {
		return nil
	}
	out := make([]ActivityLinkInput, 0, len(*list))
	for _, l := range *list {
		out = append(out, ActivityLinkInput{EntityType: string(l.EntityType), EntityID: ids.UUID(l.EntityId)})
	}
	return out
}
