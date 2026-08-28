// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import type { components } from "../api/schema";
import { suggestionsFor } from "./ai-models";

type ModelRate = components["schemas"]["AiModelRate"];

// The sheet's wire shape. Written out rather than mocked: the filter and the
// price hint are the whole behaviour, and a double would prove nothing about
// the shape they read.
function rate(
  provider: string,
  model_id: string,
  lane: ModelRate["lane"],
  input_per_mtok = "1",
  output_per_mtok = "5",
): ModelRate {
  return {
    provider,
    model_id,
    lane,
    input_per_mtok,
    output_per_mtok,
    cache_read_per_mtok: "0",
    cache_write_per_mtok: "0",
    effective_date: "2026-08-01",
  };
}

const SHEET: readonly ModelRate[] = [
  rate("gemini", "gemini-3.5-flash", "chat", "1.50", "9.00"),
  rate("gemini", "gemini-3.1-flash-lite", "chat", "0.25", "1.50"),
  rate("gemini", "gemini-embedding-001", "embeddings", "0.15", "0"),
  rate("anthropic", "claude-opus-4-8", "chat", "5.00", "25.00"),
  rate("openai_compatible", "openai/text-embedding-3-small", "embeddings"),
];

describe("suggestionsFor", () => {
  // A chat model is priced on both sides and the output rate is the larger
  // one, so the hint that ranks the list has to carry both.
  it("offers one provider's models in one lane, priced in → out", () => {
    expect(suggestionsFor(SHEET, "gemini", "chat", "en")).toEqual([
      { value: "gemini-3.1-flash-lite", hint: "US$0.25 → US$1.50" },
      { value: "gemini-3.5-flash", hint: "US$1.50 → US$9.00" },
    ]);
  });

  // An embedding lane has no output at all, so a second figure would be a zero
  // meaning "not applicable" rendered as if it were a price.
  it("prices an embedder on its one side only", () => {
    expect(suggestionsFor(SHEET, "gemini", "embeddings", "en")).toEqual([
      { value: "gemini-embedding-001", hint: "US$0.15" },
    ]);
  });

  it("prices in the reader's conventions", () => {
    expect(suggestionsFor(SHEET, "anthropic", "chat", "de")).toEqual([
      { value: "claude-opus-4-8", hint: "5,00\u00a0$ → 25,00\u00a0$" },
    ]);
  });

  // A sheet we cannot read is a model offered without a price, never a `NaN`
  // in the list — and never a throw inside a render, which takes the whole
  // settings page down with it. Blank is in here on purpose: `Number("")` is 0,
  // so an absent price would otherwise read as free.
  it.each([
    ["blank", ""],
    ["not a number", "n/a"],
  ])("offers a model whose %s price it cannot read, without one", (_, bad) => {
    const broken = [rate("gemini", "gemini-3.5-flash", "chat", bad, bad)];
    expect(suggestionsFor(broken, "gemini", "chat", "en")).toEqual([
      { value: "gemini-3.5-flash", hint: undefined },
    ]);
  });

  // The reason the lane exists: an embedder on a chat tier cannot serve a call,
  // and a chat model on the embed lane cannot either.
  it("keeps the lanes apart", () => {
    expect(
      suggestionsFor(SHEET, "gemini", "embeddings", "en").map((s) => s.value),
    ).toEqual(["gemini-embedding-001"]);
    expect(suggestionsFor(SHEET, "anthropic", "embeddings", "en")).toEqual([]);
  });

  // A provider the sheet prices nothing for, and a read that never landed. Both
  // are an empty list, which the field renders as a plain text box.
  it("is empty for a provider the sheet does not price", () => {
    expect(suggestionsFor(SHEET, "ollama", "chat", "en")).toEqual([]);
  });

  it("is empty when the catalogue never arrived", () => {
    expect(suggestionsFor(undefined, "gemini", "chat", "en")).toEqual([]);
  });
});
