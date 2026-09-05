// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package orgscan

// What the scan reads: the account as the reader sees it, and the recent
// exchanges' own words. The account half is the brief's input — the same
// projection of the same composite read, so the two surfaces cannot
// disagree about what a deal or a task is called — and the words are this
// package's own read, under the content gate, because the brief never
// needed them.

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/margince/margince/backend/internal/compose/orgbrief"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/promptfence"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// floorVersion names the shape of the input and the merge. It is hand-bumped:
// nothing about it is a prompt to hash, and a change to what the scan reads
// is a change to every stored fingerprint.
const floorVersion = "org-scan-floor-v1"

// promptVersion is derived from the prompt AS SENT, at one fixed language —
// the language is its own fingerprint component. A rewording re-reads every
// account rather than serving findings a prompt no longer produces.
var promptVersion = ai.PromptDigest(func(fence promptfence.Fence) string {
	return scanSystemFor(fence, string(textlang.English))
})

// Input is everything the model is handed, in the order it is handed it.
// Declaration order is the fingerprint's stability guarantee, exactly as it
// is for the brief.
type Input struct {
	// Account is the 360 as this reader sees it: contacts, open deals, open
	// tasks, the recent activity by subject, and the sections the reader was
	// not shown — which the prompt tells the model to stay silent about.
	Account orgbrief.Input `json:"account"`
	// Messages are the recent exchanges with their own words, oldest first
	// so the model reads them as a conversation. Empty when the reader may
	// read none of them, in which case nothing is asked of the model.
	Messages []MessageIn `json:"messages"`
}

// MessageIn is one exchange as the model reads it.
type MessageIn struct {
	ID        ids.UUID  `json:"id"`
	Kind      string    `json:"kind"`
	Direction string    `json:"direction,omitempty"`
	Subject   string    `json:"subject,omitempty"`
	At        time.Time `json:"at"`
	// Text is the body's opening, bounded by scanBodyChars; UnreadChars says
	// how much was cut, so the model — and a reader of the trace — knows a
	// quote can only come from what was shown.
	Text        string `json:"text,omitempty"`
	UnreadChars int    `json:"unread_chars,omitempty"`
}

// Fingerprint identifies what a scan was read from: the versions of the
// floor, the prompt and the routing, the language, and the encoded input.
// Two readers of one account get two fingerprints when their grants differ,
// which is the point — a stored scan is served only to the reader whose
// input it was read from.
func Fingerprint(in Input, routingVersion, lang string) (string, error) {
	encoded, err := json.Marshal(in)
	if err != nil {
		return "", fmt.Errorf("fingerprint the scan input: %w", err)
	}
	sum := sha256.Sum256([]byte(
		floorVersion + "\x00" + promptVersion + "\x00" + routingVersion + "\x00" + lang + "\x00" + string(encoded)))
	return hex.EncodeToString(sum[:]), nil
}

// message finds the exchange a finding cites, by the id the model was given.
func (in Input) message(id string) (MessageIn, bool) {
	for _, message := range in.Messages {
		if message.ID.String() == id {
			return message, true
		}
	}
	return MessageIn{}, false
}
