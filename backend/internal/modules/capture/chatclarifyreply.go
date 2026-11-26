// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// A colleague's answer to "who wrote this?", and the provider's echo of the
// question. Neither is a customer's message. Both are used up where they
// arrive: nothing is filed for them, they create no contact, and their stored
// original is deleted. FileChatMessage asks this before it routes anything.

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/capture/telegram"
	"github.com/margince/margince/backend/internal/modules/capture/whatsapp"
	"github.com/margince/margince/backend/internal/modules/capture/zalo"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/pendingforward"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// ErrChatMessageConsumed answers a message that was a colleague's answer to a
// question, or the provider's echo of one. It is skipped, so the stored original
// is destroyed and nothing is filed.
var ErrChatMessageConsumed = fmt.Errorf("capture: the message answers or echoes a question about forwarded messages: %w", connector.ErrSkip)

// ErrHeldForwardGone answers a filing of a held forward that was archived, erased
// or filed already when the ensure took its lock. Nothing was written, and the
// caller skips the forward.
var ErrHeldForwardGone = errors.New("capture: the forward is no longer held")

// clarificationHandler is implemented by a Sink that can recognise a question's
// answer or echo.
type clarificationHandler interface {
	HandleClarification(ctx context.Context, binding connector.ChatBinding, msg connector.ChatMessage) (bool, error)
}

// handleClarification reports whether the message was used up as an answer or an
// echo. A sink that cannot tell files every message as ordinary correspondence.
func handleClarification(ctx context.Context, sink connector.Sink, binding connector.ChatBinding, msg connector.ChatMessage) (bool, error) {
	handler, ok := sink.(clarificationHandler)
	if !ok {
		return false, nil
	}
	return handler.HandleClarification(ctx, binding, msg)
}

// HandleClarification recognises the provider's echo of a question this system
// sent and a member's answer to one. It reports whether the message is used up,
// and then the caller files nothing.
func (s *Sink) HandleClarification(ctx context.Context, binding connector.ChatBinding, msg connector.ChatMessage) (bool, error) {
	if msg.Direction == connector.DirectionOutbound {
		return s.consumeEcho(ctx, binding, msg)
	}
	if msg.Forward != nil || s.members == nil || binding.Ownership != connector.ChatOwnedByWorkspace {
		return false, nil
	}
	if identity, name, named := customerNamedBy(binding, msg); named && s.channelEnsurer != nil {
		if answered, err := s.consumeAnswer(ctx, binding, msg, identity, name); err != nil || answered {
			return answered, err
		}
	}
	return s.consumeOwnMessage(ctx, binding, msg)
}

// consumeOwnMessage uses up a verified colleague's message that is no code, no
// forward and no usable answer. It is the colleague's talk with the account and
// no customer's correspondence, so it files nothing and makes no contact. A
// colleague with a batch waiting is owed one hint about how to answer.
func (s *Sink) consumeOwnMessage(ctx context.Context, binding connector.ChatBinding, msg connector.ChatMessage) (bool, error) {
	var member bool
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		who, isMember, err := s.members.MemberOf(ctx, tx, binding.Provider, binding.AccountScope, msg.Counterparty.UserID)
		if err != nil || !isMember {
			return err
		}
		member = true
		if err := noteHintOwedTx(ctx, tx, who, binding.Connection); err != nil {
			return err
		}
		return deleteReply(ctx, tx, msg.StoredOriginal)
	})
	return member, err
}

// noteHintOwedTx flags the asked batches of one colleague on one account, whose
// hint has not been sent, as owed their one hint.
func noteHintOwedTx(ctx context.Context, tx pgx.Tx, member, connection ids.UUID) error {
	if _, err := tx.Exec(ctx, `
		UPDATE raw_capture r SET ask_hint_due_at = now()
		  FROM activity a
		 WHERE a.raw_capture_id = r.id AND r.held_by = $1 AND r.held_connection IS NOT DISTINCT FROM $2
		   AND (r.ask_sent_at IS NOT NULL OR r.ask_claimed_at IS NOT NULL) AND r.ask_hinted_at IS NULL AND `+pendingforward.Predicate("a"),
		member, nullable(connection)); err != nil {
		return fmt.Errorf("capture: noting that a hint is owed: %w", err)
	}
	return nil
}

// namesColleague says the customer an answer names is a verified member of the
// workspace: the colleague's own card, another colleague's, or a colleague's
// number. Filing on one would make a colleague a contact.
func (s *Sink) namesColleague(ctx context.Context, identity connector.ChannelIdentity) (bool, error) {
	var colleague bool
	err := s.db.Tx(ctx, func(tx pgx.Tx) (err error) {
		_, colleague, err = s.members.MemberOf(ctx, tx, identity.Provider, identity.AccountScope, identity.ChannelUserID)
		return err
	})
	return colleague, err
}

// refuseColleagueAnswer uses up an answer that names a colleague. The batch
// stays held and the sender is owed one hint.
func (s *Sink) refuseColleagueAnswer(ctx context.Context, member, connection, reply ids.UUID) error {
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		if err := noteHintOwedTx(ctx, tx, member, connection); err != nil {
			return err
		}
		return deleteReply(ctx, tx, reply)
	})
}

// consumeEcho uses up the provider's copy of a question this system sent.
func (s *Sink) consumeEcho(ctx context.Context, binding connector.ChatBinding, msg connector.ChatMessage) (bool, error) {
	var echoed bool
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM raw_capture
			 WHERE held_by IS NOT NULL AND held_connection IS NOT DISTINCT FROM $1 AND $2 = ANY(ask_message_ids)
			   AND coalesce(ask_sent_at, ask_claimed_at) > now() - make_interval(secs => $3))`,
			nullable(binding.Connection), msg.IdempotencyKey, pendingforward.Window.Seconds()).Scan(&echoed); err != nil {
			return fmt.Errorf("capture: looking for the question an echo copies: %w", err)
		}
		if !echoed || msg.StoredOriginal.IsZero() {
			return nil
		}
		return DeleteRawCaptureTx(ctx, tx, msg.StoredOriginal)
	})
	return echoed, err
}

func nullable(id ids.UUID) *ids.UUID {
	if id.IsZero() {
		return nil
	}
	return &id
}

// askedMessage is one held forward of a batch the account has asked about.
type askedMessage struct {
	batch      ids.UUID
	askIDs     []string
	askedAt    time.Time
	activity   ids.UUID
	capturedBy string
	source     string
	pending    bool
}

// consumeAnswer files the batch a member's answer names, or asks the member
// which one they mean. It reports false when the message is no answer: the
// sender is no member, or no question about their forwards is waiting.
func (s *Sink) consumeAnswer(ctx context.Context, binding connector.ChatBinding, msg connector.ChatMessage,
	identity connector.ChannelIdentity, name string,
) (bool, error) {
	var member ids.UUID
	var asked []askedMessage
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		var isMember bool
		var err error
		member, isMember, err = s.members.MemberOf(ctx, tx, binding.Provider, binding.AccountScope, msg.Counterparty.UserID)
		if err != nil || !isMember {
			return err
		}
		asked, err = askedMessagesTx(ctx, tx, member, binding.Connection)
		return err
	})
	if err != nil || member.IsZero() || len(asked) == 0 {
		return false, err
	}
	target, quotesAsked, ambiguous := answeredBatch(asked, msg.ReplyToID)
	switch {
	case ambiguous:
		return true, s.askWhichBatch(ctx, member, binding.Connection, msg.StoredOriginal)
	case len(target) == 0 && quotesAsked:
		// The member filed the batch by hand, or answered it already.
		return true, s.db.Tx(ctx, func(tx pgx.Tx) error { return deleteReply(ctx, tx, msg.StoredOriginal) })
	case len(target) == 0:
		return false, nil
	}
	colleague, err := s.namesColleague(ctx, identity)
	if err != nil {
		return false, err
	}
	if colleague {
		return true, s.refuseColleagueAnswer(ctx, member, binding.Connection, msg.StoredOriginal)
	}
	return true, s.fileAnswered(ctx, binding, member, target, identity, name, msg.StoredOriginal)
}

// askedMessagesTx lists the forwards of one member on one account that a
// question was sent about, newest first. It reads back as far as a held forward
// lives.
func askedMessagesTx(ctx context.Context, tx pgx.Tx, member, connection ids.UUID) ([]askedMessage, error) {
	rows, err := tx.Query(ctx, `
		SELECT r.ask_batch_id, r.ask_message_ids, coalesce(r.ask_sent_at, r.ask_claimed_at), a.id, a.captured_by, coalesce(a.source, ''),
		       (`+pendingforward.Predicate("a")+`)
		  FROM raw_capture r JOIN activity a ON a.raw_capture_id = r.id
		 WHERE r.held_by = $1 AND r.held_connection IS NOT DISTINCT FROM $2 AND (r.ask_sent_at IS NOT NULL OR r.ask_claimed_at IS NOT NULL)
		   AND coalesce(r.ask_sent_at, r.ask_claimed_at) > now() - make_interval(secs => $3)
		 ORDER BY coalesce(r.ask_sent_at, r.ask_claimed_at) DESC, a.id`, member, nullable(connection), pendingforward.Window.Seconds())
	if err != nil {
		return nil, fmt.Errorf("capture: reading the questions asked about forwards: %w", err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (askedMessage, error) {
		var m askedMessage
		err := row.Scan(&m.batch, &m.askIDs, &m.askedAt, &m.activity, &m.capturedBy, &m.source, &m.pending)
		return m, err
	})
}

// answeredBatch picks the messages an answer is for. An answer that quotes a
// question is bound to that question's batch, filed or not. An answer that
// quotes none is bound to the one batch that waits. Several waiting batches and
// no quote bind to nothing, and the caller asks which is meant. The messages it
// returns are the ones still held. quotesAsked says the answer quoted a question
// of ours, so an empty target means the batch was filed already.
func answeredBatch(asked []askedMessage, quoted string) (target []askedMessage, quotesAsked, ambiguous bool) {
	if quoted != "" {
		for _, m := range asked {
			if slices.Contains(m.askIDs, quoted) {
				return pendingOf(asked, m.batch), true, false
			}
		}
	}
	waiting := map[ids.UUID]bool{}
	var only ids.UUID
	for _, m := range asked {
		if m.pending {
			waiting[m.batch] = true
			only = m.batch
		}
	}
	switch len(waiting) {
	case 0:
		return nil, false, false
	case 1:
		return pendingOf(asked, only), false, false
	}
	return nil, false, true
}

func pendingOf(asked []askedMessage, batch ids.UUID) []askedMessage {
	var out []askedMessage
	for _, m := range asked {
		if m.batch == batch && m.pending {
			out = append(out, m)
		}
	}
	return out
}

// askWhichBatch notes that the member is owed one question that lists the
// batches waiting for them, and uses up their answer. The question goes out
// from the sweep, not from the webhook.
func (s *Sink) askWhichBatch(ctx context.Context, member, connection, reply ids.UUID) error {
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
			UPDATE raw_capture r SET ask_followup_at = coalesce(r.ask_followup_at, now())
			  FROM activity a
			 WHERE a.raw_capture_id = r.id AND r.held_by = $1 AND r.held_connection IS NOT DISTINCT FROM $2
			   AND (r.ask_sent_at IS NOT NULL OR r.ask_claimed_at IS NOT NULL) AND `+pendingforward.Predicate("a"), member, nullable(connection)); err != nil {
			return fmt.Errorf("capture: noting that a follow-up question is owed: %w", err)
		}
		return deleteReply(ctx, tx, reply)
	})
}

// fileAnswered files each held message of the batch on the customer the answer
// names, as the member who forwarded them, then uses up the answer. A message
// that expired or that the member filed by hand since it was read is left alone.
// An answer with nothing left to file is used up all the same, since the
// forwards it answers are gone.
func (s *Sink) fileAnswered(ctx context.Context, binding connector.ChatBinding, member ids.UUID, batch []askedMessage,
	identity connector.ChannelIdentity, name string, reply ids.UUID,
) error {
	ctx, err := memberActing(ctx, binding, member)
	if err != nil {
		return err
	}
	for _, m := range batch {
		// A held forward may have expired or been filed by hand since it was read.
		if held, err := s.stillHeld(ctx, m.activity); err != nil || !held {
			if err != nil {
				return err
			}
			continue
		}
		if _, err := s.channelEnsurer.EnsureChannelCounterparty(ctx, EnsureChannelRequest{
			Identity: identity, DisplayName: name, ActivityID: m.activity, Source: m.source, CapturedBy: m.capturedBy, RequireHeld: true,
		}); err != nil && !errors.Is(err, ErrHeldForwardGone) {
			return fmt.Errorf("capture: filing a held forward on the customer the member named: %w", err)
		}
	}
	return s.db.Tx(ctx, func(tx pgx.Tx) error { return deleteReply(ctx, tx, reply) })
}

// stillHeld locks one held forward and says it is still held. The lock waits
// out a purge that has begun on it.
func (s *Sink) stillHeld(ctx context.Context, activity ids.UUID) (bool, error) {
	var held bool
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		var id ids.UUID
		err := tx.QueryRow(ctx, `SELECT a.id FROM activity a WHERE a.id = $1 AND `+pendingforward.Predicate("a")+` FOR UPDATE OF a`, activity).Scan(&id)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		held = err == nil
		return err
	})
	return held, err
}

func deleteReply(ctx context.Context, tx pgx.Tx, reply ids.UUID) error {
	if reply.IsZero() {
		return nil
	}
	return DeleteRawCaptureTx(ctx, tx, reply)
}

// memberActing is the context the member's forwards are filed under, so each
// names them as its submitter.
func memberActing(ctx context.Context, binding connector.ChatBinding, member ids.UUID) (context.Context, error) {
	actor, ok := principal.Actor(ctx)
	if !ok {
		return ctx, errors.New("capture: an answer arrived with no connector on the context")
	}
	actor.UserID = member
	return chatActingContext(principal.WithActor(ctx, actor), binding)
}

// phoneInText finds a phone number as people write one.
var phoneInText = regexp.MustCompile(`\+?\d[\d\s().-]{5,18}\d`)

// The shortest and longest phone number the answer may name, in digits.
const (
	minPhoneDigits = 7
	maxPhoneDigits = 15
	// maxAnswerRunes bounds a text that is only a phone number.
	maxAnswerRunes = 40
)

// customerNamedBy reads the customer an answer names, in the network's own
// identity. WhatsApp knows a customer by phone number, from a contact card or a
// text that is one number. Telegram and Zalo know one by account id, which only a
// card carries.
func customerNamedBy(binding connector.ChatBinding, msg connector.ChatMessage) (connector.ChannelIdentity, string, bool) {
	card := msg.SharedContact
	switch binding.Provider {
	case whatsapp.Provider:
		digits, name := "", ""
		if card != nil && len(card.Phones) > 0 {
			digits, name = phoneDigits(card.Phones[0]), card.DisplayName
		} else if card == nil {
			digits = phoneInTextOnly(msg.Body)
		}
		if digits == "" {
			return connector.ChannelIdentity{}, "", false
		}
		return connector.ChannelIdentity{Provider: binding.Provider, ChannelUserID: digits}, name, true
	case telegram.Provider, zalo.Provider:
		if card == nil || card.UserID == "" {
			return connector.ChannelIdentity{}, "", false
		}
		scope := ""
		if binding.Provider == zalo.Provider {
			scope = binding.AccountScope
		}
		return connector.ChannelIdentity{Provider: binding.Provider, AccountScope: scope, ChannelUserID: card.UserID}, card.DisplayName, true
	}
	return connector.ChannelIdentity{}, "", false
}

// phoneDigits is a number's digits when it has as many as a phone number does.
func phoneDigits(number string) string {
	digits := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, number)
	if len(digits) < minPhoneDigits || len(digits) > maxPhoneDigits {
		return ""
	}
	return digits
}

// phoneInTextOnly is the number a short text names, when the text names one
// number and nothing else of weight.
func phoneInTextOnly(text string) string {
	text = strings.TrimSpace(text)
	if text == "" || len([]rune(text)) > maxAnswerRunes {
		return ""
	}
	found := phoneInText.FindAllString(text, -1)
	if len(found) != 1 {
		return ""
	}
	return phoneDigits(found[0])
}
