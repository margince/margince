// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package openchannel

// The endpoint a member owns, and the operations they drive from the screen:
// open it, read it back, pause or resume it, and say where this connector talks
// back to.
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
const endpointColumns = `id::text, user_id::text, slug, ref, coalesce(url, ''), enabled,
	inbound_received, outbound_sent, last_inbound_at, last_outbound_at, version`

// endpoint is one member's edge, as this unit reads and renders it.
//
// IT CARRIES NO SECRET, and cannot: the column does not exist. What a screen
// shows about the signing material is that some was minted, which is what the
// last-minted state of a row means.
type endpoint struct {
	ID     string `json:"id"`
	UserID string `json:"user_id"`
	// Slug and Ref are the last two segments of the public path senders POST to:
	// the declared edge, and this member's own handle on it. NEITHER IS A
	// CREDENTIAL — both reach every access log a request passes through, and the
	// signing secret is the only thing that admits one.
	Slug string `json:"slug"`
	Ref  string `json:"ref"`
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
	err := scan(&e.ID, &e.UserID, &e.Slug, &e.Ref, &e.URL, &e.Enabled,
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

// open gives the CALLER their own address on the unit's declared edge.
//
// EVERY MEMBER GETS ONE. The declared slug is a literal and therefore the same
// for everybody, so the row that tells an arriving request whose it is carries a
// minted ref instead — one URL per member on one mounted edge.
//
// It mints no SECRET. Opening an endpoint and holding the credential that
// admits requests to it are two acts, and separating them is what makes minting
// idempotent to ask for and destructive only when asked: an open that also
// minted would silently stop every already-registered sender each time a member
// re-opened the screen. The ref is not that credential and is minted here,
// because an endpoint without an address is not an endpoint.
//
// Re-opening is the same endpoint, not a second one, and it keeps the same ref
// — a new one would break every sender already pointed at the old URL, which is
// exactly what re-opening must not do.
//
// TWO OVERLAPPING FIRST-OPENS ARE SAFE, and that is a claim this file's own
// contract makes ("asking twice returns the same endpoint") — one that a plain
// existence check followed by a plain insert cannot keep: two calls that both
// read no row before either commits would both attempt the insert, and the
// second would hit ext_openchannel_endpoint_one_per_member as a bare
// constraint violation rather than the same endpoint the first call already
// created. ON CONFLICT DO NOTHING makes the insert itself the arbiter — the
// loser's RETURNING yields no row, which is how it learns to go read what the
// winner committed instead of failing.
func open(ctx context.Context, rt extension.Runtime, in json.RawMessage) (json.RawMessage, error) {
	if _, err := extension.DecodeArgs[struct{}](in); err != nil {
		return nil, err
	}
	member, err := callingMember(rt, "opening an endpoint")
	if err != nil {
		return nil, err
	}
	ref, err := newEndpointRef()
	if err != nil {
		return nil, err
	}
	var stored endpoint
	err = rt.Tx(ctx, func(ctx context.Context, tx extension.Tx) error {
		created, err := scanEndpoint(tx.QueryRow(ctx,
			`INSERT INTO `+endpointTable+` (user_id, slug, ref) VALUES ($1::uuid, $2, $3)
			 ON CONFLICT (user_id, slug) DO NOTHING
			 RETURNING `+endpointColumns, member, inboundSlug, ref).Scan)
		switch {
		case err == nil:
			stored = created
			return recordEndpoint(ctx, tx, extension.AuditCreate, eventOpened, nil, &stored)
		case errors.Is(err, extension.ErrNoRows):
			// The conflict means the row already exists — an earlier open
			// from this member, or one that just raced this call — and asking
			// again is asking for THAT endpoint. The ref minted above is
			// discarded either way: this call did not create anything, so it
			// records nothing.
			mine, err := endpointOf(ctx, tx, member)
			if err != nil {
				return err
			}
			if mine == nil {
				return fmt.Errorf("openchannel: opening conflicted with a concurrent open, but the endpoint it created cannot be found")
			}
			stored = *mine
			return nil
		default:
			return err
		}
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
	stored, err := updateOwnEndpoint(ctx, rt, "", verb, "enabled = $2", args.Enabled)
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
		EndpointID string `json:"endpoint_id"`
		URL        string `json:"url"`
	}](in)
	if err != nil {
		return nil, err
	}
	dialable, err := registrableURL(args.URL)
	if err != nil {
		return nil, err
	}
	stored, err := updateOwnEndpoint(ctx, rt, args.EndpointID, eventURLRegistered, "url = $2", dialable)
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
// file — never a string off a request — and its one bound value arrives as $2,
// beside the owner and the declared edge the row is keyed by.
//
// The value is constrained to the two column types the governed operations set,
// so a caller cannot bind something the schema has no place for and find out
// from the driver.
//
// expectedID is empty for an operation whose request names no endpoint at all
// (setEnabled). A confirm-first operation's request DOES carry one — a staged
// approval needs a row to show the approver — and the id is checked against
// the caller's own before anything is written: a mismatch answers exactly as
// no endpoint at all, because an operation that told a caller "that one exists
// but is not yours" would be confirming a stranger's endpoint id back to them.
func updateOwnEndpoint[T bool | string](ctx context.Context, rt extension.Runtime, expectedID, verb, assignment string, value T) (endpoint, error) {
	member, err := callingMember(rt, "changing an endpoint")
	if err != nil {
		return endpoint{}, err
	}
	var stored endpoint
	err = rt.Tx(ctx, func(ctx context.Context, tx extension.Tx) error {
		before, err := lockedEndpointOf(ctx, tx, member)
		if err != nil {
			return err
		}
		if before == nil {
			return errNoEndpoint()
		}
		if expectedID != "" && before.ID != expectedID {
			return errNoEndpoint()
		}
		stored, err = scanEndpoint(tx.QueryRow(ctx,
			`UPDATE `+endpointTable+` SET `+assignment+`, version = version + 1, updated_at = now()
			 WHERE user_id = $1::uuid AND slug = $3
			 RETURNING `+endpointColumns, member, value, inboundSlug).Scan)
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

// endpointOf reads one member's endpoint on the unit's declared edge, or
// nothing. Both halves of the key are named: a member holds one endpoint PER
// EDGE, and a read on the member alone would answer an arbitrary one of them the
// day this unit declares a second.
//
// A PLAIN READ — no lock. This is the shape every read-only caller wants
// (readEndpoint, listOutbound, listInbound, and the ownership checks that
// precede a seal), and taking a row lock for all of them would serialize
// callers that never write anything. A caller about to UPDATE the row wants
// lockedEndpointOf instead — see there for why.
func endpointOf(ctx context.Context, tx extension.Tx, member string) (*endpoint, error) {
	return oneEndpoint(tx.QueryRow(ctx,
		`SELECT `+endpointColumns+` FROM `+endpointTable+`
		 WHERE user_id = $1::uuid AND slug = $2`, member, inboundSlug).Scan)
}

// lockedEndpointOf reads one member's endpoint exactly as endpointOf does, FOR
// UPDATE.
//
// EVERY CALLER THAT RECORDS A BEFORE-IMAGE AND THEN UPDATES THE ROW MUST READ
// THROUGH THIS, not endpointOf. Two overlapping calls that both read first and
// both update after — updateOwnEndpoint pausing an endpoint while
// registerURL re-points it, or two overlapping secret mints — can otherwise
// both read the SAME before-image, both write, and the second writer's ledger
// row then names a "before" that was never the state its own UPDATE actually
// replaced: the first writer's change vanished from the trail between two
// audit rows that both claim to have started from the row as it stood before
// either ran. FOR UPDATE serializes them: the second caller's read blocks
// until the first's transaction commits, so it reads what that transaction
// actually left rather than what was there before either began.
func lockedEndpointOf(ctx context.Context, tx extension.Tx, member string) (*endpoint, error) {
	return oneEndpoint(tx.QueryRow(ctx,
		`SELECT `+endpointColumns+` FROM `+endpointTable+`
		 WHERE user_id = $1::uuid AND slug = $2
		 FOR UPDATE`, member, inboundSlug).Scan)
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

// readEndpoint answers the CALLER's own endpoint, or the absence of one.
//
// Their own, and not one named in the arguments: the row carries the address
// senders are pointed at and the traffic that has passed through it, which
// together say who has been messaging that member. That is not a fact this unit
// hands to a colleague because they hold the same RBAC object.
//
// Having no endpoint is `opened: false` and not an error — not having opened one
// is the ordinary state of this screen, and it is exactly what the caller is
// asking about.
func readEndpoint(ctx context.Context, rt extension.Runtime, in json.RawMessage) (json.RawMessage, error) {
	if _, err := extension.DecodeArgs[struct{}](in); err != nil {
		return nil, err
	}
	member, err := callingMember(rt, "reading an endpoint")
	if err != nil {
		return nil, err
	}
	var found *endpoint
	err = rt.Tx(ctx, func(ctx context.Context, tx extension.Tx) error {
		found, err = endpointOf(ctx, tx, member)
		return err
	})
	if err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		Opened   bool      `json:"opened"`
		Endpoint *endpoint `json:"endpoint,omitempty"`
	}{Opened: found != nil, Endpoint: found})
}
