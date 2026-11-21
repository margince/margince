// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

// The Messages API reuses a request prefix only up to a block marked
// cache_control. The prefix runs tools, then system, then messages. Every
// system prompt puts its fixed rules before the boundary sentence, whose nonce
// is minted per call. So the rules are marked and the sentence goes unmarked.

import "github.com/margince/margince/backend/internal/shared/kernel/promptfence"

// anthropicSystemBlock is one text block of the system field in block form.
type anthropicSystemBlock struct {
	Type         string                 `json:"type"`
	Text         string                 `json:"text"`
	CacheControl *anthropicCacheControl `json:"cache_control,omitempty"`
}

type anthropicCacheControl struct {
	Type string `json:"type"`
}

// anthropicCacheable marks the end of a prefix the provider may reuse for its
// default lifetime of five minutes.
var anthropicCacheable = &anthropicCacheControl{Type: "ephemeral"}

// anthropicCacheMinBytes is the smallest prefix any model caches, 512 tokens,
// at four bytes a token. Below it a marker can never be read. Above it a model
// with a higher minimum writes nothing and bills nothing extra for the marker.
const anthropicCacheMinBytes = 512 * 4

// anthropicSystem splits the system prompt at its boundary sentence and marks
// the stable rules before it. A prompt that declares no boundary is stable as a
// whole. A prefix under anthropicCacheMinBytes goes as one unmarked block.
func anthropicSystem(system string) []anthropicSystemBlock {
	if system == "" {
		return nil
	}
	stable, perCall := system, ""
	if start, ok := promptfence.RuleStart(system); ok {
		stable, perCall = system[:start], system[start:]
	}
	if len(stable) < anthropicCacheMinBytes {
		return []anthropicSystemBlock{{Type: anthropicBlockText, Text: system}}
	}
	blocks := []anthropicSystemBlock{{Type: anthropicBlockText, Text: stable, CacheControl: anthropicCacheable}}
	if perCall != "" {
		blocks = append(blocks, anthropicSystemBlock{Type: anthropicBlockText, Text: perCall})
	}
	return blocks
}
