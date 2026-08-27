// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// What the ingress seam refuses, and why. Separate from the declarations in
// ingress.go because the shapes are what a unit author reads and these are what
// a unit author trips over, and the two are read at different times.
//
//margince:extension-surface

package extension

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

// Validate checks everything decidable without knowing which unit is calling.
// Whether System is one the CALLER declared, and whether the caller may ingest
// at all, are the core's checks — made at the port against the invocation, for
// the same reason a Change's entity namespace is.
func (r Record) Validate() error {
	if err := r.validateKey(); err != nil {
		return err
	}
	if err := r.Activity.validate(); err != nil {
		return err
	}
	if err := r.validateAddresses(); err != nil {
		return err
	}
	if err := r.Counterparty.validate(); err != nil {
		return err
	}
	if len(r.ThreadKey) > MaxThreadKeyLength {
		return fmt.Errorf("extension: the thread key is %d bytes, over the %d-byte cap", len(r.ThreadKey), MaxThreadKeyLength)
	}
	if len(r.Raw) > MaxRawBytes {
		return fmt.Errorf("extension: the raw record is %d bytes, over the %d-byte cap every landed record pays for", len(r.Raw), MaxRawBytes)
	}
	return nil
}

func (r Record) validateKey() error {
	switch {
	case strings.TrimSpace(r.System) == "":
		return errors.New("extension: the record names no ingress system")
	case strings.TrimSpace(r.Key) == "":
		return errors.New("extension: the record carries no key — unkeyed capture cannot be idempotent, so a replay would land a second copy")
	case len(r.Key) > MaxKeyLength:
		return fmt.Errorf("extension: the record key is %d bytes, over the %d-byte cap", len(r.Key), MaxKeyLength)
	}
	return nil
}

func (r Record) validateAddresses() error {
	// A record that names no ADDRESS at all may name no addresses: a chat
	// message can have none anywhere in it, and the core's own shape already
	// says so — connector.NormalizedRecord treats an empty set as "I cannot
	// enumerate the parties", which the internal-message gate reads as NOT
	// internal and keeps. Refusing it here would have turned away every record
	// from a provider that issues opaque account ids and no mail, before it
	// reached a core that would have accepted it.
	//
	// The condition is the counterparty's EMAIL rather than "is this a channel
	// record", because that is the partition the core itself draws: a record
	// naming its human by a channel account and one naming nobody at all are
	// the same case here, and both are legal.
	if len(r.Addresses) == 0 {
		if r.Counterparty.Email == "" {
			return nil
		}
		return errors.New("extension: the record names an address for its counterparty and no addresses at all — the internal-message gate reads every party from that set, and over an empty one it answers \"not internal\" and keeps the record, so leaving it empty disables the gate rather than passing it")
	}
	if len(r.Addresses) > MaxAddresses {
		return fmt.Errorf("extension: the record names %d addresses, over the cap of %d", len(r.Addresses), MaxAddresses)
	}
	for _, addr := range r.Addresses {
		switch {
		case strings.TrimSpace(addr) == "":
			return errors.New("extension: the record names a blank address — the internal-message gate skips a party it cannot read, so a blank one is a party the gate never judges")
		case len(addr) > MaxAddressLength:
			return fmt.Errorf("extension: an address is %d bytes, over the %d-byte cap", len(addr), MaxAddressLength)
		}
	}
	return nil
}

func (c Counterparty) validate() error {
	if len(c.Email) > MaxAddressLength {
		return fmt.Errorf("extension: the counterparty address is %d bytes, over the %d-byte cap", len(c.Email), MaxAddressLength)
	}
	if utf8.RuneCountInString(c.DisplayName) > MaxDisplayNameRunes {
		return fmt.Errorf("extension: the counterparty display name is over the %d-rune cap", MaxDisplayNameRunes)
	}
	if c.Direction != "" && c.Direction != DirectionInbound && c.Direction != DirectionOutbound {
		return fmt.Errorf("extension: %q is not a direction (%q, %q, or empty for a record with no honest direction)",
			c.Direction, DirectionInbound, DirectionOutbound)
	}
	// A counterparty carrying BOTH is well-formed here, and deliberately so: the
	// channel identity NAMES the human and the address CORROBORATES them, which
	// is a shape only the core can admit or refuse, because only the core knows
	// which merge keys this source declared. Refusing it here would refuse it
	// for every source alike and put the decision in the wrong place.
	return c.ChannelIdentity.validate()
}

// validate refuses a HALF-stated channel identity, which is the shape that
// looks populated and routes nowhere.
//
// Either both the provider and the account id are present or neither is. One
// without the other reaches the core as a binding it cannot key — and the core
// would drop it rather than fail, so the record would land, look ordinary, and
// carry no reply address, which is indistinguishable from a provider that
// simply does not identify its senders.
func (c ChannelIdentity) validate() error {
	switch {
	case c.Provider == "" && c.ChannelUserID == "":
		return nil
	case c.Provider == "":
		return errors.New("extension: the channel identity names an account with no provider — the binding is keyed on the pair, so half of it binds nothing")
	case c.ChannelUserID == "":
		return errors.New("extension: the channel identity names a provider with no account id — a party identified only by transport cannot be replied to")
	case !providerGrammar.MatchString(c.Provider):
		return fmt.Errorf("extension: channel identity provider %q must start with a letter and contain only lower-case letters, digits and underscores", c.Provider)
	case len(c.ChannelUserID) > MaxChannelUserIDLength:
		return fmt.Errorf("extension: the channel account id is %d bytes, over the %d-byte cap", len(c.ChannelUserID), MaxChannelUserIDLength)
	case utf8.RuneCountInString(c.DisplayName) > MaxDisplayNameRunes:
		return fmt.Errorf("extension: the channel identity display name is over the %d-rune cap", MaxDisplayNameRunes)
	}
	return nil
}

func (a ActivityFields) validate() error {
	if strings.TrimSpace(a.Kind) == "" {
		return errors.New("extension: the activity names no kind")
	}
	if utf8.RuneCountInString(a.Subject) > MaxSubjectRunes {
		return fmt.Errorf("extension: the subject is over the %d-rune cap", MaxSubjectRunes)
	}
	if utf8.RuneCountInString(a.Body) > MaxBodyRunes {
		return fmt.Errorf("extension: the body is over the %d-rune cap", MaxBodyRunes)
	}
	if a.OccurredAt.IsZero() {
		return errors.New("extension: the activity has no occurred-at — a timeline ordered by when a poll noticed is a timeline of this system's own scheduling")
	}
	if a.Direction != "" && a.Direction != DirectionInbound && a.Direction != DirectionOutbound {
		return fmt.Errorf("extension: %q is not a direction (%q, %q, or empty)", a.Direction, DirectionInbound, DirectionOutbound)
	}
	return nil
}
