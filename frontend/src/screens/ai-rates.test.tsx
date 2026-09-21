/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it } from "vitest";
import { LocaleProvider } from "../i18n";
import { en } from "../i18n/en";
import { ModelRatePlate } from "./ai-rates";

afterEach(cleanup);

function row(
  model_id: string,
  lane: "chat" | "embeddings",
  input_per_mtok: string,
  output_per_mtok: string,
) {
  return {
    provider: "gemini",
    model_id,
    lane,
    input_per_mtok,
    output_per_mtok,
    cache_read_per_mtok: "0",
    cache_write_per_mtok: "0",
    effective_date: "2026-08-01",
  };
}

const SHEET = [
  row("gemini-2.5-flash", "chat", "0.30", "2.50"),
  row("text-embedding-004", "embeddings", "0.15", "0"),
];

function plate(props: Partial<Parameters<typeof ModelRatePlate>[0]> = {}) {
  render(
    <LocaleProvider>
      <ModelRatePlate
        catalogue={SHEET}
        provider="gemini"
        chatModel="gemini-2.5-flash"
        embedModel="text-embedding-004"
        locale="en"
        {...props}
      />
    </LocaleProvider>,
  );
}

describe("ModelRatePlate", () => {
  it("prices a chat model on both sides, because output is the larger number", () => {
    plate();
    expect(screen.getByText("US$0.30 → US$2.50")).toBeTruthy();
  });

  it("prices an embedder on one side, because it has no output to charge for", () => {
    plate();
    expect(screen.getByText("US$0.15")).toBeTruthy();
    expect(screen.queryByText(/US\$0\.15 →/)).toBeNull();
  });

  it("says a model has no price rather than showing it as free", () => {
    plate({ chatModel: "gemini-3.0-preview" });
    expect(screen.getByText(en["aiRates.unpriced"])).toBeTruthy();
    // The one number that must never appear for an unknown model: `Number("")`
    // is 0, so an absent price rendered naively reads as free.
    expect(screen.queryByText(/US\$0\.00/)).toBeNull();
    // And the id is still shown, because the binding itself is legitimate. It
    // shares its line with what an unpriced binding costs a reader.
    expect(screen.getByText(/gemini-3\.0-preview/)).toBeTruthy();
    expect(
      screen.getByText(new RegExp(en["aiRates.unpricedDetail"])),
    ).toBeTruthy();
    expect(screen.getByText(en["aiRates.unpricedConsequence"])).toBeTruthy();
  });

  it("folds the repair for an unpriced lane into the reading's receipt", async () => {
    const user = userEvent.setup();
    plate({ chatModel: "gemini-3.0-preview" });

    // A caption holds two lines and the consequence takes both, so where the
    // rate is actually entered is one fold away rather than dropped.
    await user.click(await screen.findByRole("button", { name: "Evidence" }));

    expect(await screen.findByText(en["aiRates.unpricedBasis"])).toBeTruthy();
  });

  it("says a model has no price when the sheet's own row is unreadable", () => {
    plate({ catalogue: [row("gemini-2.5-flash", "chat", "", ""), SHEET[1]] });
    expect(screen.getByText(en["aiRates.unpriced"])).toBeTruthy();
    // The lane whose row IS readable is unaffected: one unstateable price does
    // not blank the plate.
    expect(screen.getByText("US$0.15")).toBeTruthy();
  });

  it("draws no plate before either model has been named", () => {
    plate({ chatModel: "  ", embedModel: "" });
    expect(screen.queryByTestId("ai-rate-plate")).toBeNull();
  });

  it("draws one slot when only one lane has been named", () => {
    plate({ embedModel: "" });
    expect(screen.queryByText(en["aiRates.embedLane"])).toBeNull();
    expect(screen.getByText(en["aiRates.chatLane"])).toBeTruthy();
  });

  it("names a vendor's asking price as the vendor's, and says what binding it would do", async () => {
    const user = userEvent.setup();
    plate({
      chatModel: "gemini-3.0-preview",
      vendor: {
        models: [
          {
            id: "gemini-3.0-preview",
            input_per_mtok: "0.40",
            output_per_mtok: "3.00",
          },
        ],
        rankedBy: "",
        unavailable: false,
      },
    });

    expect(screen.getByText("US$0.40 → US$3.00")).toBeTruthy();
    // Two lines and no more: the model with what its figure is per, then the
    // caveat that nobody has agreed to this number.
    expect(
      screen.getByText(`gemini-3.0-preview · ${en["aiRates.perMTokInOut"]}`),
    ).toBeTruthy();
    expect(screen.getByText(en["aiRates.proposedDetail"])).toBeTruthy();

    await user.click(await screen.findByRole("button", { name: "Evidence" }));

    expect(await screen.findByText(en["aiRates.proposedBasis"])).toBeTruthy();
  });
});
