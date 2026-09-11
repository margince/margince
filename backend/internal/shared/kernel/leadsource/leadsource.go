// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package leadsource says which lead sources are a RECORD rather than a
// request to act.
//
// A lead the product read off a public web page is a note that the person
// exists. Nobody wrote in, nobody asked for anything, and nothing is owed —
// so the automations that turn a new lead into somebody's work must not fire
// for one.
//
// It lives in the kernel because two modules ask it and a module never imports
// a sibling: `automation` decides whether to mint the follow-up task, and
// `people` decides whether to assign an owner. Spelled in one of them, the
// other would grow a second copy, and the two would answer differently the
// first time a source was added.
package leadsource

import "slices"

// The source_system values a captured lead carries. Named rather than spelled
// at each test: these strings are written by the capture path and read by two
// automations, and a typo in any of them fails open — the automation fires,
// which is exactly the behaviour being suppressed.
const (
	// SiteRead is a person published on a company's own website, found by the
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
	return slices.Contains(passive, sourceSystem)
}

// PassiveDiscoverySources is the same set for a caller that has to ask the
// question of many rows at once — a SQL sweep binding it as one parameter
// rather than reading each payload.
//
// Cloned, so a caller cannot rewrite the rule for everybody else by holding
// onto the slice: this set decides whether work is minted and whether a breach
// is escalated, and neither reader owns it.
func PassiveDiscoverySources() []string {
	return slices.Clone(passive)
}
