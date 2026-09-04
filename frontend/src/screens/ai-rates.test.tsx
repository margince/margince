/** @vitest-environment jsdom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { LocaleProvider } from "../i18n";
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
    expect(screen.getByText("No price on file")).toBeTruthy();
    // The one number that must never appear for an unknown model: `Number("")`
    // is 0, so an absent price rendered naively reads as free.
    expect(screen.queryByText(/US\$0\.00/)).toBeNull();
    // And the id is still shown, because the binding itself is legitimate.
    expect(screen.getByText("gemini-3.0-preview")).toBeTruthy();
  });

  it("says a model has no price when the sheet's own row is unreadable", () => {
    plate({ catalogue: [row("gemini-2.5-flash", "chat", "", ""), SHEET[1]] });
    expect(screen.getByText("No price on file")).toBeTruthy();
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
    expect(screen.queryByText("What it remembers with")).toBeNull();
    expect(screen.getByText("What it thinks with")).toBeTruthy();
  });
});
