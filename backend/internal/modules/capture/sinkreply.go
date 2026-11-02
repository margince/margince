// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// The reply half of the capture Sink (CAP-FORMULA-1): an INBOUND message in a
// thread we previously wrote OUTBOUND in is a reply, and the engagement signal
// scoring feeds on that fact.
//
// The formula keys on nothing but thread_key and direction, so it holds for any
// threaded medium — mail and the channel connectors alike. What differs between
// them is only how the record NAMES its human, and counterpartyShapeOf
// (sinkchannel.go) is already the one place capture asks that question. This
// file routes through it rather than assuming an answer.

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// channelEmail is the medium every mail connector reports. It is the MEDIUM and
// not the source system on purpose: gmail, imap and graph are three ways of
// reaching one inbox, and a consumer routing a reply back has the same job for
// all three. A channel connector reports its provider instead, which is the
// same rule — the provider IS the medium there.
const channelEmail = "email"

// replyOrigin is the single answer to "what medium did this reply arrive on,
// and who is already on file for it". Both halves come out of ONE switch over
// counterpartyShapeOf, so a new channel provider is one arm rather than a grep
// across every site that had re-derived the answer for itself.
type replyOrigin struct {
	// channel is the medium a reply must be SENT BACK on, which is why it names
	// the provider rather than a medium class: an automation answering an
	// inbound reply routes on this value alone, and a class would make it
	// re-derive the provider from the activity row anyway.
	channel string
	// contactID is the counterparty when they already resolve to a contact.
	// Nil is the ordinary first-contact case, not a fault: the ensure that
	// creates them runs AFTER this transaction commits, so a first-ever sender
	// has no contact yet on either medium.
	contactID *ids.ContactID
}

// emitReply records an inbound reply against a prior outbound on its thread.
// Composed mail waits for publication; its durable ledger prevents replay duplicates.
func (s *Sink) emitReply(ctx context.Context, tx pgx.Tx, auditID ids.UUID, id ids.ActivityID, rec connector.NormalizedRecord, fields ActivityFields) error {
	if rec.NaturalKey.SourceSystem == connector.EmailSourceSystem && s.threadJoin.complete() {
		if fields.Direction != connector.DirectionInbound {
			return nil
		}
		live, err := threadSourceLiveTx(ctx, tx, id)
		if err != nil || !live {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO capture_thread_reply(activity_id) VALUES(@id) ON CONFLICT DO NOTHING`, pgx.NamedArgs{"id": id})
		return err
	}
	return emitReplyNow(ctx, tx, auditID, id, rec, fields)
}

func emitReplyNow(ctx context.Context, tx pgx.Tx, auditID ids.UUID, id ids.ActivityID, rec connector.NormalizedRecord, fields ActivityFields) error {
	if fields.Direction != connector.DirectionInbound || rec.ThreadKey == "" {
		return nil
	}
	var matched ids.UUID
	// Deferred detection must not turn an earlier inbound into a reply to
	// later mail. Native ancestry resolves equal timestamps for late parents.
	err := tx.QueryRow(ctx, `
		SELECT outbound.id FROM activity outbound
		 JOIN activity inbound ON inbound.id=@id AND activity_thread_available(inbound.thread_key)
		 WHERE outbound.thread_key=@thread AND outbound.direction='outbound' AND outbound.kind=@kind
		  AND outbound.channel_provider IS NOT DISTINCT FROM NULLIF(@provider, '')
		  AND outbound.archived_at IS NULL AND activity_thread_available(outbound.thread_key)
		  AND outbound.id<>inbound.id
		  AND (outbound.occurred_at<inbound.occurred_at -- occurred_at raw: relative ordering of two messages, not whether either happened by now
		    OR (outbound.occurred_at=inbound.occurred_at AND (outbound.created_at<inbound.created_at
		      OR EXISTS (SELECT 1 FROM activity_mail_reference reference
		        JOIN activity_identity identity ON identity.identity_key=reference.referenced_id
		         AND identity.identity_kind='mail' AND identity.activity_id=outbound.id
		        WHERE reference.activity_id=inbound.id))))
		 ORDER BY outbound.occurred_at DESC,outbound.id LIMIT 1`, pgx.NamedArgs{
		"id": id, "thread": rec.ThreadKey, auditKeyKind: fields.Kind, IdentitySourceProvider: fields.ChannelProvider,
	}).Scan(&matched)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("capture: reply detection: %w", err)
	}
	origin, ok, err := replyOriginForShape(ctx, tx, rec.Counterparty, counterpartyShapeOf(rec.Counterparty))
	if err != nil {
		return err
	}
	if !ok {
		// A thread-matched inbound whose record names no counterparty at all.
		// The thread match is real, so this is narrower than the formula, which
		// emits on the match alone: what it refuses is a reply fact asserting a
		// medium nothing in the record attests to. Which of the two CAP-FORMULA-1
		// means is an open question against the spec, not a settled reading — no
		// live producer reaches this arm either way, because every mail connector
		// drops a From-less message before capture.
		return nil
	}
	idempotencyKey := rec.NaturalKey.SourceSystem + ":" + rec.NaturalKey.SourceID
	payload := engagementReplyPayload(matched, origin, occurredday.Moment{At: fields.OccurredAt, DateOnly: fields.DateOnly}, idempotencyKey)
	if auditID == ids.Nil {
		auditID, err = storekit.AuditEvent(ctx, tx, "update", "activity", id.UUID, map[string]string{"matched_outbound_activity_id": matched.String()})
		if err != nil {
			return err
		}
	}
	if rec.NaturalKey.SourceSystem == connector.EmailSourceSystem {
		if _, err := tx.Exec(ctx, `UPDATE capture_thread_reply SET emitted=true WHERE activity_id=@id`, pgx.NamedArgs{"id": id}); err != nil {
			return err
		}
	}
	return storekit.EmitEvent(ctx, tx, auditID, id.UUID, payload)
}

// replyOriginOf resolves the medium a reply arrived on and the contact already
// on file for it, in one switch over the shape the record names its human by.
// It reports ok=false when there is no origin to publish at all.
//
// The two malformed shapes return their sentinel rather than a bare miss.
// Upsert refuses both before any of this runs (sink.go), so reaching them here
// means that guard has been bypassed — an invariant break the reply path must
// state, not absorb into a silent no-event.
func (s *Sink) replyOriginOf(ctx context.Context, tx pgx.Tx, cp connector.Counterparty) (replyOrigin, bool, error) {
	return s.replyOriginForShape(ctx, tx, cp, counterpartyShapeOf(cp))
}

// replyOriginForShape is replyOriginOf with the classification handed in, so the
// arms can be walked across the whole enum rather than only the shapes a
// Counterparty can be built to produce. Nothing but the seam above and that walk
// calls it: a caller choosing its own shape would be re-deciding the one
// question counterpartyShapeOf exists to answer.
func (s *Sink) replyOriginForShape(ctx context.Context, tx pgx.Tx, cp connector.Counterparty, shape counterpartyShape) (replyOrigin, bool, error) {
	return replyOriginForShape(ctx, tx, cp, shape)
}

func replyOriginForShape(ctx context.Context, tx pgx.Tx, cp connector.Counterparty, shape counterpartyShape) (replyOrigin, bool, error) {
	switch shape {
	case shapeMail:
		contact, found, err := mailReplyContact(ctx, tx, cp.Email)
		if err != nil {
			return replyOrigin{}, false, err
		}
		return withContact(replyOrigin{channel: channelEmail}, contact, found), true, nil
	case shapeChannel:
		contact, found, err := channelReplyContact(ctx, tx, cp.ChannelIdentity)
		if err != nil {
			return replyOrigin{}, false, err
		}
		return withContact(replyOrigin{channel: cp.ChannelIdentity.Provider}, contact, found), true, nil
	case shapeNone:
		return replyOrigin{}, false, nil
	case shapeHalfChannel:
		return replyOrigin{}, false, ErrChannelIdentityIncomplete
	default:
		// A shape added to the enum without an arm here. Naming it is the whole
		// value of this arm: the alternative is a new medium silently publishing
		// replies with an empty channel.
		return replyOrigin{}, false, fmt.Errorf("capture: reply origin: unhandled counterparty shape %d", shape)
	}
}

// withContact carries a resolved contact onto the origin, and leaves it absent
// when the lookup found none. It exists so both arms above spell "found or not"
// the same way rather than each re-deriving a pointer from a flag.
func withContact(origin replyOrigin, contact ids.ContactID, found bool) replyOrigin {
	if found {
		origin.contactID = &contact
	}
	return origin
}

// mailReplyContact resolves a mail counterparty to a contact already on file.
// A miss is not an error: the ensure that would create them runs after this
// transaction commits, so the normal first-contact reply simply has no contact
// to name yet.
func mailReplyContact(ctx context.Context, tx pgx.Tx, email string) (ids.ContactID, bool, error) {
	normalized := strings.ToLower(strings.TrimSpace(email))
	if normalized == "" {
		return ids.ContactID{}, false, nil
	}
	var contactID ids.ContactID
	// The CONTACT's own archived_at is checked, not only the binding's. Archiving
	// a contact does cascade to their satellites today, so this join changes no
	// live answer — it stops the reply fact from depending on that cascade. A
	// half-archived record (a retention pass, a merge, a repair) would otherwise
	// name a contact the consumer cannot read back, and the same reasoning holds
	// for the channel twin below.
	err := tx.QueryRow(ctx, `
		SELECT pe.contact_id FROM contact_email pe
		JOIN contact p ON p.id = pe.contact_id AND p.archived_at IS NULL
		WHERE pe.email = $1 AND pe.archived_at IS NULL
		ORDER BY pe.is_primary DESC LIMIT 1`, normalized).Scan(&contactID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ids.ContactID{}, false, nil
	}
	if err != nil {
		return ids.ContactID{}, false, fmt.Errorf("capture: reply contact lookup: %w", err)
	}
	return contactID, true, nil
}

// channelReplyContact is the same resolution on a channel identity — the key a
// messaging connector names its human by, where mail names an address. The
// binding is unique per live identity (uq_contact_channel_identity is partial on
// archived_at IS NULL), so this needs no ordering tiebreak the way the mail
// lookup needs is_primary.
func channelReplyContact(ctx context.Context, tx pgx.Tx, ci connector.ChannelIdentity) (ids.ContactID, bool, error) {
	if ci.Provider == "" || ci.ChannelUserID == "" {
		// shapeChannel already guarantees both halves, exactly as shapeMail
		// guarantees a non-empty address for the sibling lookup. Both refuse an
		// empty key anyway rather than probing on one: a half key matches by
		// accident or matches nothing, and neither is an answer.
		return ids.ContactID{}, false, nil
	}
	var contactID ids.ContactID
	err := tx.QueryRow(ctx, `
		SELECT pci.contact_id FROM contact_channel_identity pci
		JOIN contact p ON p.id = pci.contact_id AND p.archived_at IS NULL
		WHERE pci.provider = $1 AND pci.channel_user_id = $2 AND pci.archived_at IS NULL`,
		ci.Provider, ci.ChannelUserID).Scan(&contactID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ids.ContactID{}, false, nil
	}
	if err != nil {
		return ids.ContactID{}, false, fmt.Errorf("capture: reply channel contact lookup: %w", err)
	}
	return contactID, true, nil
}

// engagementReplyPayload builds the engagement.reply event from the origin the
// switch above resolved — the reply's channel and, when the counterparty is
// already a known contact, their id (absent, not null, otherwise).
func engagementReplyPayload(matched ids.UUID, origin replyOrigin, occurred occurredday.Moment, idempotencyKey string) crmcontracts.PublicEventEngagementReply {
	precision := crmcontracts.ReplyTimeIsAnInstant
	if occurred.DateOnly {
		precision = crmcontracts.ReplyTimeIsADay
	}
	payload := crmcontracts.PublicEventEngagementReply{
		MatchedOutboundActivityId: openapi_types.UUID(matched),
		Channel:                   origin.channel,
		OccurredAt:                occurred.At,
		OccurredPrecision:         &precision,
		IdempotencyKey:            idempotencyKey,
	}
	if origin.contactID != nil {
		id := openapi_types.UUID(origin.contactID.UUID)
		payload.ContactId = &id
	}
	return payload
}
