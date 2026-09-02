// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package draftreply reads the {subject, body} reply the message-drafting
// sites take, and refuses what a reader must not be handed.
//
// It exists because two sites wrote this rule independently and both wrote it
// wrong the same way. The introduction REQUEST (org360, addressed to a
// colleague) and the introduction NOTE (network, forwarded to a customer) are
// two prompts with two registers, deliberately — each says so beside its own
// wording table, and merging them would produce a customer email in a
// colleague's voice. What they never had reason to spell twice is the contract
// underneath: the same three refusals, over the same reply shape, from the same
// primitives.
//
// The cost of writing it twice was not drift. It was that the DEFECT was
// copied: both refused an empty subject while neither prompt asked for one, and
// both required names in the body that neither prompt demanded. A model
// obeying either prompt was refused, the reader silently got the template, and
// fixing one copy left the other live. One parse makes that one bug.
package draftreply

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/kernel/draftfloor"
)

// Parse reads a model reply and returns the message it carries.
//
// mustName is who the BODY has to name — a draft that never names its reader is
// not addressed to them, and one that never names its subject asks for nothing
// in particular. Both read as a message the recipient must rewrite, which is
// worse than the template they would otherwise have been handed. An empty name
// asks nothing, so a caller may pass one it does not have.
//
// This is a SHAPE check, not a grounding filter: it says a message was written
// to the right people, and claims nothing about what it says about them. What a
// draft may CLAIM is scored by a rubric, because no substring test can.
//
// Errors carry no package prefix — the caller wraps with its own, because the
// site a reader was refused by is the site that has to name itself.
func Parse(raw string, mustName ...string) (subject, body string, err error) {
	var answer struct {
		Subject string `json:"subject"`
		Body    string `json:"body"`
	}
	if err := json.Unmarshal([]byte(ai.Unfence(raw)), &answer); err != nil {
		return "", "", fmt.Errorf("the reply is not the shape this site takes: %w", err)
	}
	subject = strings.TrimSpace(ai.PlainText(answer.Subject))
	body = strings.TrimSpace(ai.PlainText(answer.Body))
	if subject == "" || body == "" {
		return "", "", fmt.Errorf("the reply carries no message to send")
	}
	for _, needed := range mustName {
		if !draftfloor.NamesPerson(body, needed) {
			return "", "", fmt.Errorf("the draft never names %q", needed)
		}
	}
	return subject, body, nil
}
