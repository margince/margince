// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package leadsource distinguishes records imported or discovered in bulk from
// live intake. A historical record does not create a new follow-up obligation.
package leadsource

import (
	"slices"

	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

// The source_system values a captured lead carries. Named rather than spelled
// at each test: these strings are written by the capture path and read by two
// automations, and a typo in any of them fails open — the automation fires,
// which is exactly the behaviour being suppressed.
const (
	// SiteRead is a contact published on a company's own website, found by the
	// deep-read pass and admitted by a human accepting the proposal.
	SiteRead = "siteread"
	// Crawl is the broader web-crawl source, the same shape of evidence.
	Crawl = "crawl"
)

// passive is the set both readers below take their answer from: the payload
// test, which one lead.created at a time asks whether to mint work, and the
// SQL parameter beneath it, which the breach sweep asks of every open row. A
// source added here therefore reaches the automations and the sweep together —
// which matters because the sweep would otherwise keep escalating a source the
// automations had just learned to decline.
var passive = []string{SiteRead, Crawl}

// IsPassiveDiscovery reports whether a lead from this source arrived without
// anybody asking us for anything.
//
// The empty string is FALSE, deliberately: a direct create sets no source
// system, and a lead somebody typed in by hand is the clearest case of work to
// do. Failing open here matches the automations' own defensive reading of a
// missing payload.
func IsPassiveDiscovery(sourceSystem string) bool {
	return provenance.ImporterNamespace(sourceSystem) || slices.Contains(passive, sourceSystem)
}

// PassiveDiscoverySources returns the named discovery sources. SQL readers
// exclude the import namespace separately through provenance's shared prefix.
func PassiveDiscoverySources() []string {
	return slices.Clone(passive)
}
