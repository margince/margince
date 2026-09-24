// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// A conversation the model is sent alternates user and assistant turns.
//
// Several open-weight models refuse anything else: the chat template shipped
// with Mistral's and Gemma's instruction models raises "conversation roles must
// alternate user/assistant/user/assistant", and a server that renders the
// model's own template (vLLM does) answers the whole call with a 400. The
// onboarding conversations are the requests that break it, because each opens
// with a user turn carrying the fenced context and then replays the history and
// the new message as user turns of their own. Ollama renders its own template
// and never refused them, which is why nothing said so until a vLLM binding did.

import (
	"strings"

	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// alternatingTurns joins each run of adjacent turns with the same role into one
// turn, their contents separated by a blank line and kept in order.
//
// Joined rather than dropped or reordered: every turn carries something the
// model is meant to read, and a fenced context block keeps its own boundary
// inside a joined turn, because the fence is the nonce and not the turn edge.
func alternatingTurns(turns []model.Message) []model.Message {
	joined := make([]model.Message, 0, len(turns))
	for _, turn := range turns {
		last := len(joined) - 1
		if last >= 0 && joined[last].Role == turn.Role {
			joined[last].Content = strings.Join([]string{joined[last].Content, turn.Content}, "\n\n")
			continue
		}
		joined = append(joined, turn)
	}
	return joined
}
