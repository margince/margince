// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import type { components } from "../api/schema";
import {
  suggestionsFor,
  type VendorCatalogue,
  type VendorModel,
  vendorSuggestions,
} from "./ai-models";

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

// The vendor catalogue is the OTHER half of the same field: what the vendor
// says it serves today, offered beside what this installation can price. Its
// own three questions are the sheet's, asked of a list nobody has agreed a
// price for — so the two must not drift into different answers about the same
// model.
function vendorModel(
  id: string,
  input_per_mtok?: string,
  output_per_mtok?: string,
): VendorModel {
  return { id, input_per_mtok, output_per_mtok };
}

// A vendor that answered. `rankedBy` and `unavailable` travel with every
// catalogue, and neither reaches the suggestions — naming them once here keeps
// that true of the fixtures too.
function answered(models: readonly VendorModel[]): VendorCatalogue {
  return { models, rankedBy: "throughput", unavailable: false };
}

describe("vendorSuggestions", () => {
  // A model the sheet already prices is the sheet's to offer, with the number
  // this installation agreed to. Offering it twice would put two hints on one
  // id and leave a reader to guess which price they are binding at.
  it("leaves out what the price sheet already offers, for this provider", () => {
    const vendor = answered([
      vendorModel("gemini-3.5-flash"),
      vendorModel("gemini-4-pro"),
    ]);
    expect(
      vendorSuggestions(vendor, SHEET, "gemini", "en").map((s) => s.value),
    ).toEqual(["gemini-4-pro"]);
  });

  // Scoped to the provider being bound: an id the sheet prices for ANOTHER
  // vendor says nothing about this one, and dropping it here would hide a model
  // that is genuinely on offer.
  it("does not let another provider's sheet entry hide a model", () => {
    const vendor = answered([vendorModel("claude-opus-4-8", "3.00", "15.00")]);
    expect(vendorSuggestions(vendor, SHEET, "gemini", "en")).toEqual([
      { value: "claude-opus-4-8", hint: "US$3.00 → US$15.00" },
    ]);
  });

  // Half a price is not a price. The vendor states each side only where it
  // publishes one, so a model priced on one side offers the field without a
  // hint rather than printing a number that means something else.
  it.each([
    ["neither side", undefined, undefined],
    ["input only", "2.00", undefined],
    ["output only", undefined, "8.00"],
    ["an unreadable figure", "n/a", "8.00"],
  ])("offers a model priced on %s, without a hint", (_, input, output) => {
    const vendor = answered([vendorModel("gemini-4-pro", input, output)]);
    expect(vendorSuggestions(vendor, SHEET, "gemini", "en")).toEqual([
      { value: "gemini-4-pro", hint: undefined },
    ]);
  });

  // A read that never landed is an empty list, the same as a vendor that
  // publishes nothing: the field is a plain text box and the admin types an id.
  it("is empty when the vendor never answered", () => {
    expect(vendorSuggestions(undefined, SHEET, "gemini", "en")).toEqual([]);
  });
});
