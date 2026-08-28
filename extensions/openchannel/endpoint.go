// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package openchannel

// The endpoint a member owns, and the operations they drive from the screen:
// open it, pause or resume it, and say where this connector talks back to.
//
// None of them names a member. All of them take one from the INVOCATION, which
// is the rule that keeps this unit's own surface from forging the consent the
// anonymous edge reads: the owner recorded on the endpoint is whose sealed
// secret verifies every arriving request, so an operation that let one member
// open or re-point ANOTHER member's endpoint would be handing a stranger's
// name to whatever arrives.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/margince/margince/backend/pkg/extension"
)

// endpointColumns is the projection every read and every write returns, in one
// place so a column added to the table is one edit rather than five.
const endpointColumns = `id::text, user_id::text, slug, coalesce(url, ''), enabled,
	inbound_received, outbound_sent, last_inbound_at, last_outbound_at, version`

// endpoint is one member's edge, as this unit reads and renders it.
//
// IT CARRIES NO SECRET, and cannot: the column does not exist. What a screen
// shows about the signing material is that some was minted, which is what the
// last-minted state of a row means.
type endpoint struct {
	ID     string `json:"id"`
	UserID string `json:"user_id"`
	// Slug is which declared anonymous edge this row owns — the last segment of
	// the public path senders POST to. It is not a credential: the path appears
	// in access logs, and the signing secret is the only thing that admits a
	// request.
	Slug string `json:"slug"`
	// URL is where this connector talks back to, empty until a member registers
	// one. Empty is the ordinary state of an endpoint that only receives.
	URL     string `json:"url"`
	Enabled bool   `json:"enabled"`
	// InboundReceived and OutboundSent are what a screen renders about traffic.
	// The queue table holds the individual requests, so anything finer is a
	// query over it rather than a counter that could disagree with one.
	InboundReceived int64  `json:"inbound_received"`
	OutboundSent    int64  `json:"outbound_sent"`
	LastInboundAt   string `json:"last_inbound_at,omitempty"`
	LastOutboundAt  string `json:"last_outbound_at,omitempty"`
	Version         int    `json:"version"`
}

// scanEndpoint reads endpointColumns off one row.
//
// The two timestamps are scanned as TIMES and rendered afterwards, not scanned
// as text: the columns are timestamptz and the driver refuses to put one into a
// string, which no unit test catches because a fake hands back whatever the
// fixture scripted. The rendering is RFC 3339 because that is what the contract
// declares and what a screen's formatter parses.
func scanEndpoint(scan func(...any) error) (endpoint, error) {
	var (
		e                         endpoint
		lastInbound, lastOutbound *time.Time
	)
	err := scan(&e.ID, &e.UserID, &e.Slug, &e.URL, &e.Enabled,
		&e.InboundReceived, &e.OutboundSent, &lastInbound, &lastOutbound, &e.Version)
	if err != nil {
		return endpoint{}, err
	}
	e.LastInboundAt = renderTime(lastInbound)
	e.LastOutboundAt = renderTime(lastOutbound)
	return e, nil
}

// renderTime formats an optional timestamp for this unit's answers: absent
// reads as an empty string, and present reads as RFC 3339, which is what the
// contract declares and what a screen's formatter parses.
func renderTime(at *time.Time) string {
	if at == nil {
		return ""
	}
	return at.UTC().Format(time.RFC3339)
}

// open claims the unit's declared edge for the CALLER and records it.
//
// It mints nothing. Opening an endpoint and holding the credential that admits
// requests to it are two acts, and separating them is what makes minting
// idempotent to ask for and destructive only when asked: an open that also
// minted would silently stop every already-registered sender each time a member
// re-opened the screen.
//
// Re-opening is the same endpoint, not a second one. The row is unique on the
// member and on the slug, and the slug is the unit's own declared literal, so
// there is exactly one endpoint to claim and exactly one member who holds it.
func open(ctx context.Context, rt extension.Runtime, in json.RawMessage) (json.RawMessage, error) {
	if _, err := extension.DecodeArgs[struct{}](in); err != nil {
		return nil, err
	}
	member, err := callingMember(rt, "opening an endpoint")
	if err != nil {
		return nil, err
	}
	var stored endpoint
	err = rt.Tx(ctx, func(ctx context.Context, tx extension.Tx) error {
		mine, err := endpointOf(ctx, tx, member)
		if err != nil {
			return err
		}
		if mine != nil {
			stored = *mine
			return nil
		}
		// The declared slug is the installation's ONE anonymous path for this
		// unit, so a second member cannot also hold it. Checked here so the
		// refusal is a sentence a person can act on; the UNIQUE constraint is
		// what actually holds under two simultaneous opens.
		held, err := endpointBySlug(ctx, tx, inboundSlug)
		if err != nil {
			return err
		}
		if held != nil {
			return fmt.Errorf("%w: this installation's open channel is already held by another member — they have to release it before it can be opened again", extension.ErrConflict)
		}
		stored, err = scanEndpoint(tx.QueryRow(ctx,
			`INSERT INTO `+endpointTable+` (user_id, slug) VALUES ($1::uuid, $2)
			 RETURNING `+endpointColumns, member, inboundSlug).Scan)
		if err != nil {
			return err
		}
		return recordEndpoint(ctx, tx, extension.AuditCreate, eventOpened, nil, &stored)
	})
	if err != nil {
		return nil, err
	}
	return json.Marshal(stored)
}

// setEnabled pauses or resumes the caller's own endpoint.
//
// Pausing keeps the row, the owner and the sealed secret. It is a state a
// member undoes, and the alternative — deleting the endpoint — would destroy
// the credential every registered sender already holds, so a pause and a
// resume would not be inverses of each other.
func setEnabled(ctx context.Context, rt extension.Runtime, in json.RawMessage) (json.RawMessage, error) {
	args, err := extension.DecodeArgs[struct {
		Enabled bool `json:"enabled"`
	}](in)
	if err != nil {
		return nil, err
	}
	verb := eventEnabled
	if !args.Enabled {
		verb = eventDisabled
	}
	stored, err := updateOwnEndpoint(ctx, rt, verb, "enabled = $2", args.Enabled)
	if err != nil {
		return nil, err
	}
	return json.Marshal(stored)
}

// registerURL records where this connector talks back to.
//
// The address is validated where a person can still read the refusal, against
// what they typed, rather than at the moment something tries to dial it.
func registerURL(ctx context.Context, rt extension.Runtime, in json.RawMessage) (json.RawMessage, error) {
	args, err := extension.DecodeArgs[struct {
		URL string `json:"url"`
	}](in)
	if err != nil {
		return nil, err
	}
	dialable, err := registrableURL(args.URL)
	if err != nil {
		return nil, err
	}
	stored, err := updateOwnEndpoint(ctx, rt, eventURLRegistered, "url = $2", dialable)
	if err != nil {
		return nil, err
	}
	return json.Marshal(stored)
}

// updateOwnEndpoint applies one assignment to the caller's own endpoint and
// records the change, before-image and all.
//
// One helper rather than a statement per operation, because the shape is the
// same every time and the parts that must not vary are the ones a copy would
// eventually drop: the owner predicate, the version bump, and the ledger row
// carrying both images. The assignment is a compile-time literal from this
// file — never a string off a request — and its one bound value arrives as $2.
//
// The value is constrained to the two column types the governed operations set,
// so a caller cannot bind something the schema has no place for and find out
// from the driver.
func updateOwnEndpoint[T bool | string](ctx context.Context, rt extension.Runtime, verb, assignment string, value T) (endpoint, error) {
	member, err := callingMember(rt, "changing an endpoint")
	if err != nil {
		return endpoint{}, err
	}
	var stored endpoint
	err = rt.Tx(ctx, func(ctx context.Context, tx extension.Tx) error {
		before, err := endpointOf(ctx, tx, member)
		if err != nil {
			return err
		}
		if before == nil {
			return errNoEndpoint()
		}
		stored, err = scanEndpoint(tx.QueryRow(ctx,
			`UPDATE `+endpointTable+` SET `+assignment+`, version = version + 1, updated_at = now()
			 WHERE user_id = $1::uuid RETURNING `+endpointColumns, member, value).Scan)
		if err != nil {
			return err
		}
		return recordEndpoint(ctx, tx, extension.AuditUpdate, verb, before, &stored)
	})
	return stored, err
}

// errNoEndpoint is what every operation on an endpoint that was never opened
// answers, worded for the person reading a screen.
//
// A function rather than a package-level var: a unit's declaration is read out
// of its AST without compiling it, and an initializer that calls out would run
// at import — before the declaration has been judged at all.
func errNoEndpoint() error {
	return fmt.Errorf("%w: you have not opened an endpoint yet — open one before changing it", extension.ErrNotFound)
}

// callingMember is the member this invocation runs as. A job tick and a bus
// delivery both answer the zero Caller, and neither can own an endpoint,
// because there is nobody whose endpoint it would be.
func callingMember(rt extension.Runtime, doing string) (string, error) {
	member := rt.Caller().UserID
	if member == "" {
		return "", fmt.Errorf("%w: %s is something a person does, and this invocation has nobody behind it", extension.ErrForbidden, doing)
	}
	return member, nil
}

// endpointOf reads one member's endpoint, or nothing.
func endpointOf(ctx context.Context, tx extension.Tx, member string) (*endpoint, error) {
	return oneEndpoint(tx.QueryRow(ctx,
		`SELECT `+endpointColumns+` FROM `+endpointTable+` WHERE user_id = $1::uuid`, member).Scan)
}

// endpointBySlug reads whoever holds one declared slug, or nothing.
func endpointBySlug(ctx context.Context, tx extension.Tx, slug string) (*endpoint, error) {
	return oneEndpoint(tx.QueryRow(ctx,
		`SELECT `+endpointColumns+` FROM `+endpointTable+` WHERE slug = $1`, slug).Scan)
}

// oneEndpoint turns a single-row read into a row or an absence.
//
// The absence is the PUBLISHED sentinel and not the driver's own wording: the
// core translates the driver's empty result into extension.ErrNoRows precisely
// so a unit does not match on a driver's text, and a unit that matched the text
// would report "you have not opened one yet" as a fault.
func oneEndpoint(scan func(...any) error) (*endpoint, error) {
	found, err := scanEndpoint(scan)
	if err != nil {
		if errors.Is(err, extension.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &found, nil
}
