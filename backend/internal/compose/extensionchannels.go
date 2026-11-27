// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"maps"
	"slices"
	"sort"

	"github.com/margince/margince/backend/internal/modules/comms"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// channelVocabulary is a composition's channel transports, as its boot
// registered them. sendable is what a reply can leave on: the reconcile records
// it from the core connectors and the units that declared a sender. Nil until
// then, and comms.PreRegistryChannelSending answers meanwhile.
//
// registered and sending are the directory snapshot the discovery endpoint
// serves. registered is every transport a message may name, with the row's own
// display facts. sending maps the subset a reply can leave on to what it can
// carry. The two differ (whatsapp is registered and unsendable), so they are
// held apart.
type channelVocabulary struct {
	sendable   map[string]bool
	registered []channelProviderFacts
	sending    map[string]connector.Carriage
}

func (x *Extensions) setSendable(providers []string) {
	set := make(map[string]bool, len(providers))
	for _, p := range providers {
		set[p] = true
	}
	x.mu.Lock()
	defer x.mu.Unlock()
	x.channels.sendable = set
}

// sendsOn reports whether a reply can leave this composition on the provider.
func (x *Extensions) sendsOn(provider string) bool {
	if x == nil {
		return comms.PreRegistryChannelSending(provider)
	}
	x.mu.RLock()
	defer x.mu.RUnlock()
	if x.channels.sendable == nil {
		return comms.PreRegistryChannelSending(provider)
	}
	return x.channels.sendable[provider]
}

// sendableProviders lists the transports sendsOn answers true for, sorted.
func (x *Extensions) sendableProviders() []string {
	if x == nil {
		return comms.PreRegistryChannels()
	}
	x.mu.RLock()
	defer x.mu.RUnlock()
	if x.channels.sendable == nil {
		return comms.PreRegistryChannels()
	}
	out := slices.Collect(maps.Keys(x.channels.sendable))
	sort.Strings(out)
	return out
}

func (x *Extensions) setDirectory(registered []channelProviderFacts, sending map[string]connector.Carriage) {
	x.mu.Lock()
	defer x.mu.Unlock()
	x.channels.registered = slices.Clone(registered)
	x.channels.sending = maps.Clone(sending)
}

// channelDirectory returns the registered transports and, for the subset that
// can carry an outbound message, what each one can carry.
func (x *Extensions) channelDirectory() (registered []channelProviderFacts, sending map[string]connector.Carriage) {
	if x == nil {
		return nil, nil
	}
	x.mu.RLock()
	defer x.mu.RUnlock()
	return slices.Clone(x.channels.registered), maps.Clone(x.channels.sending)
}
