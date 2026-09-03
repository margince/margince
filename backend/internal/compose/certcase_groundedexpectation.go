// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The expectation vocabulary the two GROUNDING certification sites share:
// site_extract/profile and site_fact_extract/page_facts.
//
// Both read a crawl and file named fields, and both have scenarios whose whole
// content is a NEGATIVE — that a page must file nothing under a field. A bare
// field-to-value map cannot make that claim: it can only pin what should be
// there, which leaves the absence to the judge's rubric, and a rubric band is
// HardPass independent. So both sites take a second spelling, and it is one
// spelling rather than two because two would drift and a scenario author would
// have to remember which site read which.
//
// Declared here rather than in whichever file needed it first: it arrived with
// the second caller, which is the point at which it stopped being one site's
// vocabulary.

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// The two keys the explicit form is spelled with. Neither is a field name at
// either site, which is what lets one mapping carry both forms without a
// discriminator: an expectation using either key is the explicit form, and
// anything else is the bare field-to-value map.
const (
	groundedKey    = "grounded"
	notGroundedKey = "not_grounded"
)

// groundedExpectation is what a grounding scenario asserts about one crawl: the
// fields the read must ground with the values they must carry, and the fields it
// must not ground at all.
type groundedExpectation struct {
	grounded    map[string]string
	notGrounded []string
}

// parseGroundedExpectation reads either spelling. `site` names the caller in
// every refusal, because a scenario author reads the message and not the stack.
//
// The bare field-to-value map stays legal because it is the whole claim of a
// scenario that only says what a page grounds, and rewriting those into a
// wrapper would buy nothing but churn.
//
// The explicit form refuses an unknown key rather than ignoring it, because a
// mistyped `not_ground:` would otherwise load as an expectation that forbids
// nothing and pass whatever the model filed — the exact failure the negative
// claim exists to prevent.
func parseGroundedExpectation(site string, expected json.RawMessage) (groundedExpectation, error) {
	var keyed map[string]json.RawMessage
	if err := json.Unmarshal(expected, &keyed); err != nil {
		return groundedExpectation{}, fmt.Errorf("%s: the expected answer is not a mapping: %w", site, err)
	}
	_, hasGrounded := keyed[groundedKey]
	_, hasNotGrounded := keyed[notGroundedKey]
	if !hasGrounded && !hasNotGrounded {
		var bare map[string]string
		if err := json.Unmarshal(expected, &bare); err != nil {
			return groundedExpectation{}, fmt.Errorf(
				"%s: the expected answer is neither a field to value map nor a %s/%s mapping: %w",
				site, groundedKey, notGroundedKey, err)
		}
		return groundedExpectation{grounded: bare}, nil
	}
	var explicit struct {
		Grounded    map[string]string `json:"grounded"`
		NotGrounded []string          `json:"not_grounded"`
	}
	dec := json.NewDecoder(bytes.NewReader(expected))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&explicit); err != nil {
		return groundedExpectation{}, fmt.Errorf(
			"%s: the expected answer carries %s or %s but is not that form: %w",
			site, groundedKey, notGroundedKey, err)
	}
	return groundedExpectation{grounded: explicit.Grounded, notGrounded: explicit.NotGrounded}, nil
}
