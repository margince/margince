// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package comms

// Staging a message the INSTALLATION sends, and retiring the one-time material
// it carries.
//
// Its own file beside store.go and storechannel.go, because the package splits
// staging by the shape of the thing being staged and a controller message is a
// shape of its own: no sending user, no caller-chosen purpose, no arbitrary
// subject or body.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// StageControllerInput is one message from the installation itself.
type StageControllerInput struct {
	// ActivityID is the timeline row this send hangs off. Required, and not an
	// accident of the schema: privacy's erasure and the subject access export
	// both reach comms_outbound THROUGH the activity, so a delivery with no
	// activity would be invisible to both.
	ActivityID ids.ActivityID
	// Recipient is one address, chosen by the server. A controller message goes
	// where the record says, never where a caller asks.
	Recipient string
	// TemplateKey and TemplateVersion name the registered wording. Staging
	// refuses a pair the registry does not know.
	TemplateKey     string
	TemplateVersion int
	// Subject and Body are the RENDERED template. Body carries the link
	// placeholder when PayloadRef is set; the plaintext link is never here.
	Subject string
	Body    string
	// MessageID is the RFC822 id this send will claim.
	MessageID string
	// PayloadRef and PayloadExpiresAt reference the one-time link material in
	// the key vault, and say when it stops being worth sending.
	PayloadRef       string
	PayloadExpiresAt *time.Time
	// LinkID is the confirm_token this message carries, when it carries one.
	LinkID ids.UUID
}

// ControllerTemplates answers whether a template key and version are
// registered. Compose supplies it: the wording is consent's, the sending is
// this module's, and neither may import the other.
type ControllerTemplates interface {
	Registered(key string, version int) bool
}

// StageControllerTx records a delivery the installation sends, on the caller's
// transaction.
//
// It refuses three things a user send is allowed, and each refusal is the point
// of the lane rather than a precaution:
//
//   - an unregistered template, because the installation's own words are fixed
//     in code and versioned, never composed at a call site;
//   - a body whose placeholder count disagrees with the material, because a
//     message that was supposed to carry a link and does not is a dead end for
//     the person who receives it, and one that carries two is a bug nobody sees
//     until it ships;
//   - HTML, because the operator relay is text-only and pretending otherwise
//     would silently drop the markup.
func (s *Store) StageControllerTx(ctx context.Context, tx pgx.Tx, in StageControllerInput, templates ControllerTemplates) (ids.UUID, error) {
	if in.Recipient == "" {
		return ids.UUID{}, ErrNoAddressee
	}
	if in.MessageID == "" {
		// Non-NULL and empty passes the row's shape CHECK, so the SECOND such
		// send collides on the message-id uniqueness index and reads back as a
		// duplicate. Named here as the caller defect it is.
		return ids.UUID{}, fmt.Errorf("comms: a controller delivery needs a message id: %w", ErrTemplateShape)
	}
	if templates == nil || !templates.Registered(in.TemplateKey, in.TemplateVersion) {
		return ids.UUID{}, fmt.Errorf("comms: %q@%d is not a registered controller template: %w",
			in.TemplateKey, in.TemplateVersion, ErrTemplateNotRegistered)
	}
	if err := checkPlaceholder(in); err != nil {
		return ids.UUID{}, err
	}
	recipients, err := marshalList([]string{in.Recipient})
	if err != nil {
		return ids.UUID{}, fmt.Errorf("comms: encoding the controller recipient: %w", err)
	}
	empty, err := marshalList(nil)
	if err != nil {
		return ids.UUID{}, fmt.Errorf("comms: encoding the empty lists: %w", err)
	}
	files, err := json.Marshal(orEmptyFiles(nil))
	if err != nil {
		return ids.UUID{}, fmt.Errorf("comms: encoding the attachment snapshot: %w", err)
	}
	id := ids.NewV7()
	if _, err := tx.Exec(ctx, `
		INSERT INTO comms_outbound
		  (id, activity_id, provider, message_id, recipients, cc, bcc,
		   subject, body, references_chain, status, created_at, attachments,
		   sender_kind, template_key, template_version, payload_ref, payload_expires_at, link_id)
		VALUES ($1, $2, $3, $4, $5, $6, $6,
		        $7, $8, $6, 'pending', $9, $10,
		        'controller', $11, $12, NULLIF($13,''), $14, $15)`,
		id, in.ActivityID, ProviderOperatorRelay, in.MessageID, recipients, empty,
		in.Subject, in.Body, s.now().UTC(), files,
		in.TemplateKey, in.TemplateVersion, in.PayloadRef, in.PayloadExpiresAt,
		nullableID(in.LinkID)); err != nil {
		if storekit.IsUniqueViolation(err) {
			return ids.UUID{}, ErrDuplicateMessage
		}
		return ids.UUID{}, fmt.Errorf("comms: staging the controller delivery: %w", err)
	}
	return id, nil
}

// ClearPayloadRef retires the one-time material once it can no longer be sent.
//
// The reference is cleared and not the row, because the delivery is still the
// record that a message went out. What must not survive is a live credential
// pointing at somebody's mailbox.
//
// Two callers, and they are the two halves of the same obligation. The
// dispatcher calls it when the relay has taken the message or the delivery
// reached a terminal state (closePayload), and the payload sweep calls it for
// the material no terminal outcome ever reached. Art. 17 erasure retires the
// material too, on its own path: it destroys the vault entry and nulls the
// reference in one transaction on every arm that scrubs a delivery.
//
// Every caller destroys the vault entry BEFORE calling this. Clearing first
// would leave live material with nothing left pointing at it, which no later
// pass and no erasure can find by reference again.
func (s *Store) ClearPayloadRef(ctx context.Context, id ids.UUID) error {
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx,
			`UPDATE comms_outbound SET payload_ref = NULL WHERE id = $1`, id)
		if err != nil {
			return fmt.Errorf("comms: clearing the payload reference: %w", err)
		}
		return nil
	})
}

// ExpiredControllerPayloads names the controller deliveries still holding
// one-time link material past its expiry.
//
// Deliberately NOT filtered by status. A payload outliving payload_expires_at
// is a live credential to somebody's mailbox whatever the row's disposition
// says, and the two dispositions that retire it — accepted and parked — already
// cleared payload_ref, so a row reaching this read has by definition not been
// retired by either. Adding a status predicate would only be able to exclude
// rows that should be swept.
//
// Ordered by expiry so the oldest material goes first: a bounded pass that
// cannot drain its backlog should shorten the longest exposure, not an
// arbitrary one.
//
// `after` is what stops that ordering starving the newer rows. A batch of rows
// that will never delete — a vault rejecting them, a ref the store cannot
// resolve — would otherwise be re-selected by every cadence forever, and every
// payload behind them would sit undestroyed while the pass reported itself busy.
// The sweep pages past what it has already tried in this run, so one poisoned
// batch costs one batch rather than the whole backlog.
func (s *Store) ExpiredControllerPayloads(
	ctx context.Context, now time.Time, after time.Time, limit int,
) ([]ids.UUID, time.Time, error) {
	var out []ids.UUID
	var last time.Time
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		// payload_expires_at IS NOT NULL is not restated: the table's own
		// payload_needs_expiry CHECK already makes it implied by payload_ref
		// being present, and a reader who saw it here would go looking for the
		// case it handles.
		rows, err := tx.Query(ctx, `
			SELECT id, payload_expires_at FROM comms_outbound
			 WHERE payload_ref IS NOT NULL
			   AND payload_expires_at < $1
			   AND payload_expires_at > $2
			 ORDER BY payload_expires_at
			 LIMIT $3`, now, after, limit)
		if err != nil {
			return fmt.Errorf("comms: reading expired link material: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var id ids.UUID
			var expires time.Time
			if err := rows.Scan(&id, &expires); err != nil {
				return fmt.Errorf("comms: reading an expired payload row: %w", err)
			}
			out = append(out, id)
			last = expires
		}
		return rows.Err()
	})
	return out, last, err
}

// PayloadRefFor reads the material reference a delivery still holds, so a sweep
// can destroy the vault entry before clearing the column that names it.
//
// Order matters and is the same one erasure uses: clear the column first and a
// failure leaves an orphan in the vault that nothing can find again.
func (s *Store) PayloadRefFor(ctx context.Context, id ids.UUID) (string, error) {
	var ref string
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx,
			`SELECT coalesce(payload_ref, '') FROM comms_outbound WHERE id = $1`, id).Scan(&ref)
		if err != nil {
			return fmt.Errorf("comms: reading the payload reference: %w", err)
		}
		return nil
	})
	return ref, err
}

// checkPlaceholder holds the body and the material to the same story.
func checkPlaceholder(in StageControllerInput) error {
	n := placeholderCount(in.Body)
	if in.PayloadRef == "" {
		if n != 0 {
			return fmt.Errorf("comms: a template with no link material carries %d placeholder(s): %w",
				n, ErrTemplateShape)
		}
		return nil
	}
	if n != 1 {
		return fmt.Errorf("comms: a template carrying link material has %d placeholder(s), want exactly 1: %w",
			n, ErrTemplateShape)
	}
	return nil
}

func nullableID(id ids.UUID) *ids.UUID {
	if id == (ids.UUID{}) {
		return nil
	}
	return &id
}

// ProviderOperatorRelay is the provider name a controller delivery carries. It
// is not one of the mail providers a user connects: there is no OAuth grant
// behind it and no scope to intersect, which is why profileFor asks neither.
const ProviderOperatorRelay = "operator_relay"

// LinkPlaceholder is where a rendered controller template expects its one-time
// link. It is substituted in memory at dispatch and never stored.
const LinkPlaceholder = "{{confirmation-link}}"

// ErrTemplateNotRegistered refuses a controller send whose wording this build
// does not know. It is an ANSWER about the request, not an outage.
var ErrTemplateNotRegistered = errors.New("comms: the controller template is not registered")

// ErrTemplateShape refuses a rendered body whose placeholder count disagrees
// with the material it was staged with.
var ErrTemplateShape = errors.New("comms: the rendered template does not match its link material")

func placeholderCount(body string) int { return strings.Count(body, LinkPlaceholder) }
