// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import type { components } from "../api/schema";
import { suggestionsFor } from "./ai-models";

type ModelRate = components["schemas"]["AiModelRate"];

// The sheet's shape, minus the four prices this function never reads. Written
// out rather than mocked: the filter is the whole behaviour, and a double that
// answered the filter would prove nothing about the wire shape it filters.
function rate(
  provider: string,
  model_id: string,
  lane: ModelRate["lane"],
): ModelRate {
  return {
    provider,
    model_id,
    lane,
    input_per_mtok: "1",
    output_per_mtok: "1",
    cache_read_per_mtok: "0",
    cache_write_per_mtok: "0",
    effective_date: "2026-08-01",
  };
}

const SHEET: readonly ModelRate[] = [
  rate("gemini", "gemini-3.5-flash", "chat"),
  rate("gemini", "gemini-3.1-flash-lite", "chat"),
  rate("gemini", "gemini-embedding-001", "embeddings"),
  rate("anthropic", "claude-opus-4-8", "chat"),
  rate("openai_compatible", "openai/text-embedding-3-small", "embeddings"),
];

describe("suggestionsFor", () => {
  it("offers one provider's models in one lane", () => {
    expect(suggestionsFor(SHEET, "gemini", "chat")).toEqual([
      { value: "gemini-3.1-flash-lite" },
      { value: "gemini-3.5-flash" },
    ]);
  });

  // The reason the lane exists: an embedder on a chat tier cannot serve a call,
  // and a chat model on the embed lane cannot either.
  it("keeps the lanes apart", () => {
    expect(suggestionsFor(SHEET, "gemini", "embeddings")).toEqual([
      { value: "gemini-embedding-001" },
    ]);
    expect(suggestionsFor(SHEET, "anthropic", "embeddings")).toEqual([]);
  });

  // A provider the sheet prices nothing for, and a read that never landed. Both
  // are an empty list, which the field renders as a plain text box.
  it("is empty for a provider the sheet does not price", () => {
    expect(suggestionsFor(SHEET, "ollama", "chat")).toEqual([]);
  });

  it("is empty when the catalogue never arrived", () => {
    expect(suggestionsFor(undefined, "gemini", "chat")).toEqual([]);
  });
});
