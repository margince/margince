// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

// cacheWithin bounds a wire's prompt-cache report by the prompt it itemizes.
//
// Every wire that calls it reports a cache-INCLUSIVE prompt total — OpenAI's
// input_tokens, a broker's prompt_tokens, Ollama's prompt_eval_count — with the
// cache read and the cache write as parts of it; OpenRouter billing a 12,610-
// token prompt reported 12,601 of them as cache_write_tokens on the first call
// and as cached_tokens on the second. The port holds the same shape
// (model.Response.InputTokens), so the parts can never outgrow the whole: a
// read above the prompt is clamped to it, and the write to what the read left.
// It is reasoningWithin's rule for the output side, applied to the input side,
// and for the same reason — a pricer that trusted an oversized part would bill
// cache rates for tokens the prompt never had.
func cacheWithin(prompt, read, write int) (int, int) {
	read = cacheReadWithin(prompt, read)
	return read, min(max(write, 0), max(prompt, 0)-read)
}

// cacheReadWithin is cacheWithin for a wire that reports a cache read and no
// cache write.
func cacheReadWithin(prompt, read int) int {
	return min(max(read, 0), max(prompt, 0))
}
